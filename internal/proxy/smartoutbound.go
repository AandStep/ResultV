// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package proxy

import (
	"context"
	"net"
	"sync"
	"time"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/outbound"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"

	"resultproxy-wails/internal/verdict"
)

// smartOutboundTag is both the type name and the tag. One group, one instance,
// no reason for the two to differ.
const smartOutboundTag = "smart"

// smartOutboundOptions is the config shape of the "smart" outbound. Only the
// member list: everything else it needs comes from the service context, which
// is how it reaches state that cannot survive a trip through JSON.
type smartOutboundOptions struct {
	Outbounds []string `json:"outbounds,omitempty"`
}

var (
	_ adapter.OutboundGroup             = (*smartOutbound)(nil)
	_ adapter.ConnectionHandlerEx       = (*smartOutbound)(nil)
	_ adapter.PacketConnectionHandlerEx = (*smartOutbound)(nil)
)

type smartOutbound struct {
	outbound.Adapter
	ctx        context.Context
	outbound   adapter.OutboundManager
	connection adapter.ConnectionManager
	logger     logger.ContextLogger

	tags     []string
	direct   adapter.Outbound
	proxy    adapter.Outbound
	store    *verdict.Store
	traffic  *trafficTracker
	health   *directHealth
	probes   *probeGate
	udpAlive nodeUDPCheck

	// probe is probeHost, indirected so tests can answer without a network.
	probe func(ctx context.Context, host string, gate *probeGate) verdict.Decision
	// inflight holds the hosts a probe is already running for. A page load
	// opens dozens of connections to one host at once; without this each of
	// them would start its own probe and one page would spend the whole
	// twenty-per-minute budget the gate exists to enforce.
	inflight sync.Map
}

// nodeUDPCheck reports whether the node currently in use has been measured
// carrying UDP. A function rather than a value because the measurement arrives
// a few seconds after connect (startUDPRelayProbe) — long after this outbound
// was built — and can flip again on a reload.
type nodeUDPCheck func() bool

// nodeUDPCheckFor binds the check to one node.
//
// AutoNodeKeyOf, and nothing else, because that is the key startUDPRelayProbe
// files the verdict under. A key derived any other way would never match, the
// check would answer "not measured" forever, and HTTP/3 would stay off for
// every learned name with nothing in the logs to say why.
func nodeUDPCheckFor(p ProxyConfig) nodeUDPCheck {
	key := AutoNodeKeyOf(p)
	return func() bool { return NodeCarriesUDP(key) }
}

func newSmartOutbound(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options smartOutboundOptions) (adapter.Outbound, error) {
	return &smartOutbound{
		Adapter:    outbound.NewAdapter(smartOutboundTag, tag, []string{N.NetworkTCP, N.NetworkUDP}, options.Outbounds),
		ctx:        ctx,
		outbound:   service.FromContext[adapter.OutboundManager](ctx),
		connection: service.FromContext[adapter.ConnectionManager](ctx),
		logger:     logger,
		tags:       options.Outbounds,
		store:      service.FromContext[*verdict.Store](ctx),
		traffic:    service.FromContext[*trafficTracker](ctx),
		health:     newDirectHealth(nil),
		probes:     newProbeGate(nil),
		udpAlive:   service.FromContext[nodeUDPCheck](ctx),
		probe:      probeHost,
	}, nil
}

// nodeCarriesUDP is the measured answer, and "not measured" is not the same as
// "measured working": an engine that registered no check at all has to read as
// unproven, or the gate it feeds would open on every node nobody ever probed.
func (s *smartOutbound) nodeCarriesUDP() bool {
	if s.udpAlive == nil {
		return false
	}
	return s.udpAlive()
}

// Start resolves the two members once. Resolving them per connection would put
// a map lookup on the hot path for a pair that cannot change while the engine
// is up.
func (s *smartOutbound) Start() error {
	for _, tag := range s.tags {
		detour, loaded := s.outbound.Outbound(tag)
		if !loaded {
			return E.New("smart: outbound not found: ", tag)
		}
		switch tag {
		case "direct":
			s.direct = detour
		case "proxy":
			s.proxy = detour
		}
	}
	if s.direct == nil || s.proxy == nil {
		return E.New("smart: needs both a direct and a proxy member, got ", s.tags)
	}
	return nil
}

func (s *smartOutbound) Now() string   { return "direct" }
func (s *smartOutbound) All() []string { return s.tags }

// member maps a decision onto the outbound that carries it out. chooseRace has
// no member of its own; until the race exists it behaves as the pre-feature
// default did.
func (s *smartOutbound) member(c smartChoice) adapter.Outbound {
	if c == chooseProxy {
		return s.proxy
	}
	return s.direct
}

// decide answers for one connection. Racing is switched off here and turned on
// by the task that implements it.
func (s *smartOutbound) decide(metadata *adapter.InboundContext) smartChoice {
	return decideSmart(s.store, smartHost(metadata), metadata.Destination.Addr, s.raceAllowed())
}

// raceAllowed is the breaker (spec §6.5). While the direct path is failing on
// everything at once, nothing is raced and nothing is learned.
func (s *smartOutbound) raceAllowed() bool { return s.health.healthy() }

// smartHost is the name this connection is for, or "" when there is none.
// FakeIP puts the name in Destination.Fqdn before any rule is matched (fork
// route/route.go:422); the sniffer, when it ran, puts it in Domain.
func smartHost(metadata *adapter.InboundContext) string {
	if metadata.Domain != "" {
		return metadata.Domain
	}
	return metadata.Destination.Fqdn
}

func (s *smartOutbound) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	choice := chooseDirect
	if metadata := adapter.ContextFrom(ctx); metadata != nil {
		choice = s.decide(metadata)
	}
	return s.member(choice).DialContext(ctx, network, destination)
}

func (s *smartOutbound) ListenPacket(ctx context.Context, destination M.Socksaddr) (net.PacketConn, error) {
	choice := chooseDirect
	if metadata := adapter.ContextFrom(ctx); metadata != nil {
		choice = s.decide(metadata)
	}
	return s.member(choice).ListenPacket(ctx, destination)
}

// attributeProxy books this connection to the node, both in the counters the
// speed indicator reads and in the log line the user sees. Called only when the
// proxy member actually carried the connection.
func (s *smartOutbound) attributeProxy(conn net.Conn, metadata adapter.InboundContext) net.Conn {
	if s.traffic == nil {
		return conn
	}
	s.traffic.logProxyConnection(metadata)
	return s.traffic.attributeProxyConn(conn)
}

func (s *smartOutbound) NewConnectionEx(ctx context.Context, conn net.Conn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	rec, known := s.lookupAndRefresh(&metadata)
	choice := choiceFrom(rec, known, s.raceAllowed())
	if choice == chooseRace {
		s.raceConnection(ctx, conn, metadata, onClose)
		return
	}
	if choice == chooseProxy {
		conn = s.attributeProxy(conn, metadata)
	}
	chosen := s.member(choice)
	if handler, isHandler := chosen.(adapter.ConnectionHandlerEx); isHandler {
		handler.NewConnectionEx(ctx, conn, metadata, onClose)
		return
	}
	s.connection.NewConnection(ctx, chosen, conn, metadata, onClose)
}

// raceConnection handles a destination nothing is known about.
//
// The client's opening bytes are usually already waiting in the connection: the
// router pushes the sniffed buffer back before handing it over (fork
// route/route.go:155-157), so on the normal path this read costs nothing.
func (s *smartOutbound) raceConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	first := make([]byte, smartFirstReadBudget)
	_ = conn.SetReadDeadline(time.Now().Add(smartRaceHeadStart))
	n, readErr := conn.Read(first)
	_ = conn.SetReadDeadline(time.Time{})
	if n == 0 {
		// A client that says nothing cannot be raced: there is nothing to replay
		// and no way to tell the two paths apart. Send it the way it would have
		// gone before this feature existed.
		if readErr != nil {
			s.logger.DebugContext(ctx, "smart: client sent nothing, falling back to direct: ", readErr)
		}
		s.connection.NewConnection(ctx, s.direct, conn, metadata, onClose)
		return
	}

	res := runSmartRace(ctx, first[:n],
		func(dialCtx context.Context) (net.Conn, error) {
			return s.direct.DialContext(dialCtx, N.NetworkTCP, metadata.Destination)
		},
		func(dialCtx context.Context) (net.Conn, error) {
			return s.proxy.DialContext(dialCtx, N.NetworkTCP, metadata.Destination)
		})
	if res.Err != nil {
		s.health.record(smartHost(&metadata), false)
		N.CloseOnHandshakeFailure(conn, onClose, res.Err)
		s.logger.ErrorContext(ctx, res.Err)
		return
	}

	// A race the direct path won is proof the link works; one the proxy won is
	// one more site that did not answer directly.
	s.health.record(smartHost(&metadata), !res.ViaProxy)
	s.learn(metadata, res.ViaProxy)
	if res.ViaProxy {
		conn = s.attributeProxy(conn, metadata)
	} else {
		// Direct won on bytes. Whether those bytes were the site or a wall is a
		// different question, and only a probe can answer it.
		s.recheckAsync(smartHost(&metadata))
	}
	// The server's first bytes are already off the socket, so they are handed
	// back in front of it; from here this is an ordinary relayed pair and the
	// core's own copy loop owns it.
	server := bufio.NewCachedConn(res.Conn, buf.As(res.Head))
	s.connection.NewConnection(ctx, constantDialer{conn: server}, conn, metadata, onClose)
}

// learn writes down what the race proved. A name is worth remembering; a bare
// address is remembered under the address, which is all Telegram's MTProto and
// Discord's voice media ever give us.
func (s *smartOutbound) learn(metadata adapter.InboundContext, viaProxy bool) {
	if s.store == nil {
		return
	}
	if !s.health.healthy() {
		// The link itself is down. Anything written here would be a guess with a
		// seven-day lifetime.
		return
	}
	d := verdict.Direct
	if viaProxy {
		d = verdict.Proxy
	}
	if host := smartHost(&metadata); host != "" {
		s.store.Learn(host, d)
		return
	}
	if metadata.Destination.Addr.IsValid() {
		s.store.LearnIP(metadata.Destination.Addr, d)
	}
}

// recheckAsync re-examines a name the race decided in favour of the direct
// path. The race only knows whether bytes arrived; a region wall arrives as
// bytes too. Always asynchronous — a connection is already being served and
// must never wait on this.
func (s *smartOutbound) recheckAsync(host string) {
	if host == "" || s.store == nil || s.probe == nil {
		return
	}
	if _, busy := s.inflight.LoadOrStore(host, struct{}{}); busy {
		return
	}
	go func() {
		defer s.inflight.Delete(host)
		if d := s.probe(context.Background(), host, s.probes); d != verdict.Unknown {
			s.store.Learn(host, d)
		}
	}()
}

// verdictRefreshDivisor sets how early a learned verdict is renewed: once less
// than a TTL/verdictRefreshDivisor of its life is left. A fraction rather than
// a fixed lead because the two TTLs differ by a factor of seven — three hours
// of warning on a Direct verdict, twenty-one on a Proxy one.
const verdictRefreshDivisor = 8

// verdictNeedsRefresh reports whether this record is close enough to expiry to
// be worth renewing now, while it is still valid.
//
// Only learned records age, and only they may be re-derived: a user rule is
// not a guess to be second-guessed, and re-deriving a floor entry would put
// the data plane above the hand-proven answer that the source order exists to
// keep it below. Both of those carry no expiry at all, so the check is the
// same either way — but it is spelled out, because the day someone gives a
// floor entry a TTL this must not quietly start probing it.
func verdictNeedsRefresh(rec verdict.Record, now time.Time) bool {
	if rec.Source != verdict.SourceLearned || rec.ExpiresAt.IsZero() {
		return false
	}
	ttl := verdict.TTLDirect
	if rec.Decision == verdict.Proxy {
		ttl = verdict.TTLProxy
	}
	return rec.ExpiresAt.Sub(now) < ttl/verdictRefreshDivisor
}

// refreshExpiringAsync is the second of the prober's three triggers (spec §7).
// The first is the race; this one renews a verdict before it dies, so the user
// never pays for the same classification twice.
//
// Nothing waits on it: the record still in force answers the connection that
// triggered this, and the probe replaces it behind the user's back.
func (s *smartOutbound) refreshExpiringAsync(host string, rec verdict.Record) {
	if !verdictNeedsRefresh(rec, time.Now()) {
		return
	}
	s.recheckAsync(host)
}

// constantDialer hands the core a connection that is already open, so the
// core's copy loop, connection tracking and interrupt handling are reused
// instead of reimplemented here.
type constantDialer struct {
	conn net.Conn
}

func (d constantDialer) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	return d.conn, nil
}

func (d constantDialer) ListenPacket(ctx context.Context, destination M.Socksaddr) (net.PacketConn, error) {
	return nil, E.New("smart: constantDialer is TCP only")
}

func (s *smartOutbound) NewPacketConnectionEx(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	// A QUIC Initial is cryptographically bound to its connection ID, so there
	// is nothing to replay and no race to run: decideSmartUDP works off what is
	// already known, and refusing is its answer to everything it cannot send
	// somewhere safe. A refusal costs the client one immediate fall back to TCP
	// — where the race does work — rather than the full HTTP/3 timeout.
	rec, known := s.lookupAndRefresh(&metadata)
	choice := choiceFrom(rec, known, false)
	switch decideSmartUDP(choice, known, metadata.Destination.Port, s.nodeCarriesUDP()) {
	case udpRefuse:
		err := E.New("smart: udp to ", metadata.Destination, " refused (verdict ", choice,
			", node udp confirmed: ", s.nodeCarriesUDP(), "), falling back to TCP")
		N.CloseOnHandshakeFailure(conn, onClose, err)
		s.logger.DebugContext(ctx, err)
		return
	case udpViaProxy:
		if s.traffic != nil {
			s.traffic.logProxyConnection(metadata)
			// Booked here for the same reason the TCP path books in
			// attributeProxy: the router's tracker ran before this outbound
			// had chosen, so without this the node's share of every tunnelled
			// UDP flow is missing from the speed indicator.
			conn = s.traffic.attributeProxyPacketConn(conn)
		}
		s.dispatchPacket(ctx, s.proxy, conn, metadata, onClose)
	default:
		s.dispatchPacket(ctx, s.direct, conn, metadata, onClose)
	}
}

// dispatchPacket hands the flow to a member, preferring the member's own
// handler so the core's UDP NAT and timeouts stay where they were.
func (s *smartOutbound) dispatchPacket(
	ctx context.Context, chosen adapter.Outbound, conn N.PacketConn,
	metadata adapter.InboundContext, onClose N.CloseHandlerFunc,
) {
	if handler, isHandler := chosen.(adapter.PacketConnectionHandlerEx); isHandler {
		handler.NewPacketConnectionEx(ctx, conn, metadata, onClose)
		return
	}
	s.connection.NewPacketConnection(ctx, chosen, conn, metadata, onClose)
}

// lookupAndRefresh answers what is known about this destination and, on the
// way past, renews the record if it is nearing the end of its life.
//
// One lookup on the connection path, used for three things: the choice, the
// "known versus merely defaulted" distinction UDP needs, and the age the
// refresh trigger reads. The renewal is fired here rather than from a sweep
// because this is the moment a name is proven to still matter — a store full
// of names the user stopped visiting is not worth probing.
func (s *smartOutbound) lookupAndRefresh(metadata *adapter.InboundContext) (verdict.Record, bool) {
	host := smartHost(metadata)
	rec, known := lookupSmart(s.store, host, metadata.Destination.Addr)
	if known {
		s.refreshExpiringAsync(host, rec)
	}
	return rec, known
}

// extendedBoxContext is include.Context plus our own outbound type.
//
// It has to be spelled out rather than wrapped: include.Context builds all six
// registries and hands them to box.Context in one call
// (include/registry.go:60-62), so there is no seam to slip a type into
// afterwards.
func extendedBoxContext(ctx context.Context) context.Context {
	outbounds := include.OutboundRegistry()
	outbound.Register[smartOutboundOptions](outbounds, smartOutboundTag, newSmartOutbound)
	return box.Context(
		ctx,
		include.InboundRegistry(),
		outbounds,
		include.EndpointRegistry(),
		include.ProviderRegistry(),
		include.DNSTransportRegistry(),
		include.ServiceRegistry(),
	)
}
