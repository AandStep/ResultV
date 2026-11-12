package proxy

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"slices"
	"sync"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/outbound"
	"github.com/sagernet/sing-box/common/dialer"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/group"
	"github.com/sagernet/sing-box/route/rule"
	"github.com/sagernet/sing-tun"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/protocol/socks"
	"github.com/sagernet/sing/protocol/socks/socks5"
	"github.com/sagernet/sing/service"

	"resultproxy-wails/internal/config"
)

const (
	socks5ExitOutboundType = "resultv-socks5-exit"
	socks5ExitSelectorType = "resultv-socks5-exit-selector"
	socks5ExitTag          = "socks5-exit"
	socks5ExitNextTag      = "socks5-exit-next"
	socks5ExitSelector     = "socks5-exit-switch"
)

var errSocks5ExitUnavailable = errors.New("SOCKS5 exit is unavailable on this connection")

// A session without an active exit is built exactly as it was before the SOCKS5
// exit existed.
func socks5ExitWired(profile *config.Socks5ExitConfig) bool {
	return profile.IsActive()
}

func addSocks5Exit(sb *SingBoxConfig, cfg EngineConfig) {
	if !socks5ExitWired(cfg.Socks5Exit) {
		return
	}
	p := cfg.Socks5Exit
	exit := SBOutbound{
		Type: socks5ExitOutboundType, Tag: socks5ExitTag, Version: "5", Network: "tcp", Detour: "proxy",
		Server: p.Host, ServerPort: p.Port, Username: p.Username, Password: p.Password,
	}
	exit.DomainResolver = firstDetourServerTag(sb.DNS.Servers, "proxy")
	next := exit
	next.Tag, next.Server, next.ServerPort, next.Username, next.Password = socks5ExitNextTag, "", 0, "", ""
	sb.Outbounds = append(sb.Outbounds, exit, next, SBOutbound{
		Type: socks5ExitSelectorType, Tag: socks5ExitSelector, Outbounds: []string{"proxy", socks5ExitTag, socks5ExitNextTag},
		Default: socks5ExitTag, InterruptExistConnections: true,
	})
	if !socks5ExitListActive(cfg) {
		routeVPNThroughSocks5Exit(sb)
	}
	if cfg.Mode == ProxyModeTunnel || cfg.FirstHopProbePort != 0 {
		probe := []SBRouteRule{{Inbound: []string{probeInboundTag}, Action: "route", Outbound: "proxy"}}
		if proxyIsEndpoint(cfg.Proxy) {
			probe = resolveBeforeEndpoint(probe, firstDetourServerTag(sb.DNS.Servers, "proxy"))
		}
		sb.Route.Rules = append(probe, sb.Route.Rules...)
	}
}

func routeVPNThroughSocks5Exit(sb *SingBoxConfig) {
	for i := range sb.Route.Rules {
		rule := &sb.Route.Rules[i]
		if rule.Outbound != "proxy" || slices.Contains(rule.Inbound, updateInboundTag) || slices.Contains(rule.Inbound, probeInboundTag) {
			continue
		}
		rule.Outbound = socks5ExitSelector
	}
	if sb.Route.Final == "proxy" {
		sb.Route.Final = socks5ExitSelector
	}
	for i := range sb.Outbounds {
		if sb.Outbounds[i].Tag == smartOutboundTag {
			sb.Outbounds[i].Outbounds = []string{"direct", socks5ExitSelector}
		}
	}
}

func registerSocks5ExitOutbounds(registry *outbound.Registry) {
	outbound.Register[option.SelectorOutboundOptions](registry, socks5ExitSelectorType, newSocks5ExitSelector)
	outbound.Register[option.SOCKSOutboundOptions](registry, socks5ExitOutboundType, newSocks5ExitOutbound)
}

func newSocks5ExitOutbound(ctx context.Context, _ adapter.Router, _ log.ContextLogger, tag string, options option.SOCKSOutboundOptions) (adapter.Outbound, error) {
	o := &tcpOnlySocks5Exit{
		Adapter: outbound.NewAdapterWithDialerOptions("socks", tag, options.Network.Build(), options.DialerOptions),
	}
	initial := config.Socks5ExitConfig{
		Host: options.Server, Port: int(options.ServerPort), Username: options.Username, Password: options.Password,
	}
	options.Server, options.ServerPort, options.Username, options.Password = "", 0, "", ""
	o.configure = func(profile *config.Socks5ExitConfig) (N.Dialer, error) {
		if profile == nil || profile.Host == "" {
			return nil, nil
		}
		next := options
		next.Server, next.ServerPort = profile.Host, uint16(profile.Port)
		upstream, err := dialer.New(ctx, next.DialerOptions, next.ServerIsDomain())
		if err != nil {
			return nil, err
		}
		method := byte(socks5.AuthTypeNotRequired)
		if profile.Username != "" {
			method = socks5.AuthTypeUsernamePassword
		}
		return socks.NewClient(socks5MethodDialer{Dialer: upstream, method: method}, next.ServerOptions.Build(),
			socks.Version5, profile.Username, profile.Password), nil
	}
	err := o.setProfile(&initial)
	return o, err
}

// Network filtering in the router can race with selecting a new exit. Reject
// UDP again at the SOCKS5 dial boundary so it never gets an ASSOCIATE request.
type tcpOnlySocks5Exit struct {
	outbound.Adapter
	mu        sync.RWMutex
	dialer    N.Dialer
	configure func(*config.Socks5ExitConfig) (N.Dialer, error)
}

func (o *tcpOnlySocks5Exit) setProfile(profile *config.Socks5ExitConfig) error {
	dialer, err := o.configure(profile)
	if err != nil {
		return err
	}
	o.mu.Lock()
	o.dialer = dialer
	o.mu.Unlock()
	return nil
}

func (o *tcpOnlySocks5Exit) DialContext(ctx context.Context, network string, target M.Socksaddr) (net.Conn, error) {
	if N.NetworkName(network) != N.NetworkTCP {
		return nil, N.ErrUnknownNetwork
	}
	o.mu.RLock()
	dialer := o.dialer
	o.mu.RUnlock()
	if dialer == nil {
		return nil, errSocks5ExitUnavailable
	}
	ctx, metadata := adapter.ExtendContext(ctx)
	metadata.Outbound, metadata.Destination = o.Tag(), target
	return dialer.DialContext(ctx, network, target)
}

func (o *tcpOnlySocks5Exit) ListenPacket(context.Context, M.Socksaddr) (net.PacketConn, error) {
	return nil, N.ErrUnknownNetwork
}

func newSocks5ExitSelector(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.SelectorOutboundOptions) (adapter.Outbound, error) {
	// The manager chooses the initial exit for each session. The stock selector
	// gives cached choices priority over Default, even before the TUN can start.
	// Keep the shared cache available to DNS and other outbounds.
	ctx = service.ContextWith[adapter.CacheFile](service.ExtendContext(ctx), nil)
	inner, err := group.NewSelector(ctx, router, logger, tag, options)
	if err != nil {
		return nil, err
	}
	return &socks5ExitSwitch{
		Outbound: inner, selector: inner.(*group.Selector),
		outbounds:  service.FromContext[adapter.OutboundManager](ctx),
		connection: service.FromContext[adapter.ConnectionManager](ctx),
	}, nil
}

var _ adapter.Lifecycle = (*socks5ExitSwitch)(nil)

// Do not expose OutboundGroup here. TUN pre-match unwraps groups and would
// cache a WireGuard raw flow that the selector cannot interrupt on a switch.
type socks5ExitSwitch struct {
	adapter.Outbound
	selector   *group.Selector
	outbounds  adapter.OutboundManager
	connection adapter.ConnectionManager
	raw        adapter.FlowOutbound
	mu         sync.RWMutex
	revision   uint64
}

func (s *socks5ExitSwitch) Start(stage adapter.StartStage, scope *adapter.Scope) error {
	if stage == adapter.StartStateStart {
		firstHop, _ := s.outbounds.Outbound("proxy")
		s.raw, _ = firstHop.(adapter.FlowOutbound)
	}
	return s.selector.Start(stage, scope)
}

func (s *socks5ExitSwitch) Now() string { return s.selector.Now() }

func (s *socks5ExitSwitch) Network() []string {
	networks := s.selector.Network()
	// Accept UDP into the switch so the router can hand it to our rejection
	// path instead of logging an unsupported outbound for every QUIC retry.
	if !slices.Contains(networks, N.NetworkUDP) {
		networks = append(slices.Clone(networks), N.NetworkUDP)
	}
	if s.raw != nil && !slices.Contains(networks, N.NetworkICMP) {
		return append(slices.Clone(networks), N.NetworkICMP)
	}
	return networks
}

// Preserve real echo requests for WireGuard when SOCKS5 is off. TCP and UDP
// use connection forwarding so switching can interrupt their old route.
func (s *socks5ExitSwitch) PreMatchFlow(network string, dest netip.Addr) adapter.PreMatchAction {
	if network == N.NetworkUDP && !s.canForwardUDP() {
		return adapter.PreMatchReject
	}
	if s.raw == nil || network != N.NetworkICMP {
		return adapter.PreMatchContinue
	}
	if s.Now() != "proxy" {
		return adapter.PreMatchDrop
	}
	return s.raw.PreMatchFlow(network, dest)
}

func (s *socks5ExitSwitch) canForwardUDP() bool {
	return slices.Contains(s.selector.Network(), N.NetworkUDP)
}

func (s *socks5ExitSwitch) NewPacketConnection(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	if !s.canForwardUDP() {
		N.CloseOnHandshakeFailure(conn, onClose, &rule.RejectedError{Cause: rule.ErrReset})
		return
	}
	s.connection.NewPacketConnection(ctx, s, conn, metadata, onClose)
}

func (s *socks5ExitSwitch) PortAddresses() (netip.Addr, netip.Addr) {
	return s.raw.PortAddresses()
}

func (s *socks5ExitSwitch) PortMTU() uint32 {
	return s.raw.PortMTU()
}

func (s *socks5ExitSwitch) AttachReturn(path tun.Return) error {
	return s.raw.AttachReturn(path)
}

func (s *socks5ExitSwitch) DetachReturn(path tun.Return) error {
	return s.raw.DetachReturn(path)
}

func (s *socks5ExitSwitch) WritePackets(packets [][]byte) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Now() != "proxy" {
		return nil
	}
	return s.raw.WritePackets(packets)
}

func (s *socks5ExitSwitch) SelectOutbound(tag string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.Now()
	if !s.selector.SelectOutbound(tag) {
		return false
	}
	if old != tag {
		s.revision++
	}
	return true
}

func (s *socks5ExitSwitch) currentRevision() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.revision
}

func (s *socks5ExitSwitch) DialContext(ctx context.Context, network string, target M.Socksaddr) (net.Conn, error) {
	rev := s.currentRevision()
	conn, err := s.selector.DialContext(ctx, network, target)
	if err == nil && s.currentRevision() != rev {
		_ = conn.Close()
		return nil, net.ErrClosed
	}
	return conn, err
}

func (s *socks5ExitSwitch) ListenPacket(ctx context.Context, target M.Socksaddr) (net.PacketConn, error) {
	rev := s.currentRevision()
	conn, err := s.selector.ListenPacket(ctx, target)
	if err == nil && s.currentRevision() != rev {
		_ = conn.Close()
		return nil, net.ErrClosed
	}
	return conn, err
}

func (e *SingBoxEngine) SetSocks5Exit(profile *config.Socks5ExitConfig) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.instance == nil || !e.running.Load() {
		return errSocks5ExitUnavailable
	}
	out, _ := e.instance.Outbound().Outbound(socks5ExitSelector)
	selector, ok := out.(*socks5ExitSwitch)
	if !ok {
		return errSocks5ExitUnavailable
	}
	tag := "proxy"
	if profile.IsActive() {
		tag = socks5ExitTag
		if selector.Now() == tag {
			tag = socks5ExitNextTag
		}
		out, _ := e.instance.Outbound().Outbound(tag)
		slot, ok := out.(*tcpOnlySocks5Exit)
		if !ok {
			return errSocks5ExitUnavailable
		}
		if err := slot.setProfile(profile); err != nil {
			return err
		}
	}
	if !selector.SelectOutbound(tag) {
		return errors.New("SOCKS5 selector could not apply the requested exit")
	}
	e.savedCfg.Socks5Exit = profile
	return nil
}
