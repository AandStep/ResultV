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
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"

	"resultproxy-wails/internal/verdict"
)

// The asymmetry this closes: a name the LIST says is blocked has its udp/443
// knocked back onto TCP by a route rule, while a name the engine LEARNED is
// blocked went straight out over the node's UDP. When the node does not carry
// UDP — a measured property of this project, not a hypothetical — that second
// path is a black hole, and the client waits out the full HTTP/3 timeout
// instead of falling back to TCP in milliseconds.
func TestSmartUDPRefusesHTTP3ThroughANodeWhoseUDPIsNotProven(t *testing.T) {
	if got := decideSmartUDP(chooseProxy, true, 443, false); got != udpRefuse {
		t.Fatalf("h3 to a learned-blocked name on an unproven node = %v, want refuse", got)
	}
}

// With the node's UDP actually measured, h3 is the better path and there is no
// reason to spend a TCP fallback on it.
func TestSmartUDPCarriesHTTP3WhenTheNodesUDPIsProven(t *testing.T) {
	if got := decideSmartUDP(chooseProxy, true, 443, true); got != udpViaProxy {
		t.Fatalf("h3 to a learned-blocked name on a proven node = %v, want proxy", got)
	}
}

// Only 443 has a fallback to fall back to. A game or a voice flow refused here
// has nowhere else to go, so an unproven node is still better than a certain
// failure — and if the measurement was stale or wrong, this is the path that
// keeps working.
func TestSmartUDPStillUsesTheNodeOffPort443(t *testing.T) {
	if got := decideSmartUDP(chooseProxy, true, 19302, false); got != udpViaProxy {
		t.Fatalf("non-443 UDP on an unproven node = %v, want proxy", got)
	}
}

// Unchanged from before the node flag existed: an unknown name on 443 is
// refused so the client drops to TCP, where the race can classify it. One
// bounce to learn, rather than a permanent ban on HTTP/3.
func TestSmartUDPRefusesAnUnknownNameOn443(t *testing.T) {
	if got := decideSmartUDP(chooseDirect, false, 443, true); got != udpRefuse {
		t.Fatalf("h3 to an unknown name = %v, want refuse", got)
	}
}

// A name known to work directly never touches the node, so the node's UDP
// health has no say over it.
func TestSmartUDPSendsAKnownDirectNameOutDirectly(t *testing.T) {
	if got := decideSmartUDP(chooseDirect, true, 443, false); got != udpViaDirect {
		t.Fatalf("h3 to a known-direct name = %v, want direct", got)
	}
}

// Nothing known and not port 443: the pre-feature behaviour, which is to let
// it out the way it would have gone without this engine.
func TestSmartUDPLetsAnUnknownNonHTTP3FlowOutDirectly(t *testing.T) {
	if got := decideSmartUDP(chooseDirect, false, 53, false); got != udpViaDirect {
		t.Fatalf("unknown udp/53 = %v, want direct", got)
	}
}

// The node-UDP flag is read from the same record the auto-selector scores, and
// it means "measured, and recently enough to still be true".
func TestNodeUDPRelayConfirmedOnlyOnAFreshOKVerdict(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

	fresh := NodeStat{UDPRelay: UDPRelayOK, UDPRelayCheckedAt: now.Add(-time.Hour)}
	if !udpRelayConfirmed(fresh, now) {
		t.Error("a node measured carrying UDP an hour ago is not confirmed")
	}

	stale := NodeStat{UDPRelay: UDPRelayOK, UDPRelayCheckedAt: now.Add(-autoUDPRelayVerdictTTL - time.Minute)}
	if udpRelayConfirmed(stale, now) {
		t.Error("a verdict older than the TTL the scorer honours is still counted")
	}

	failed := NodeStat{UDPRelay: UDPRelayFail, UDPRelayCheckedAt: now.Add(-time.Hour)}
	if udpRelayConfirmed(failed, now) {
		t.Error("a node measured NOT carrying UDP is counted as confirmed")
	}

	// The state every node is in for the first seconds of its first session.
	if udpRelayConfirmed(NodeStat{}, now) {
		t.Error("a node that was never probed is counted as confirmed")
	}

	// A verdict with no timestamp cannot be aged, so it cannot be trusted.
	if udpRelayConfirmed(NodeStat{UDPRelay: UDPRelayOK}, now) {
		t.Error("an undated verdict is counted as confirmed")
	}
}

// The flag has to reach the outbound the same way the store and the tracker
// do: the core builds this outbound out of JSON, so the service context is the
// only channel for state that cannot survive the trip.
func TestSmartOutboundReadsTheNodeUDPFlagFromTheServiceContext(t *testing.T) {
	alive := false
	ctx := service.ContextWith[nodeUDPCheck](context.Background(), func() bool { return alive })

	built, err := newSmartOutbound(ctx, nil, log.NewNOPFactory().NewLogger("test"), smartOutboundTag, smartOutboundOptions{})
	if err != nil {
		t.Fatal(err)
	}
	s := built.(*smartOutbound)

	if s.nodeCarriesUDP() {
		t.Error("the outbound reported UDP alive while the measurement said otherwise")
	}
	// Read live, not cached: the verdict is written seconds after connect, long
	// after this outbound was constructed.
	alive = true
	if !s.nodeCarriesUDP() {
		t.Error("a verdict that arrived after the outbound was built never reached it")
	}
}

// Nothing registered the flag — an engine built before this existed, or a
// path that never measured. The conservative answer is the one that keeps
// HTTP/3 off an unproven node.
func TestSmartOutboundWithoutTheFlagTreatsTheNodeAsUnproven(t *testing.T) {
	built, err := newSmartOutbound(context.Background(), nil, log.NewNOPFactory().NewLogger("test"), smartOutboundTag, smartOutboundOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if built.(*smartOutbound).nodeCarriesUDP() {
		t.Fatal("a node nobody measured was reported as carrying UDP")
	}
}

// fakePacketConn is the client side of a UDP flow: it never carries anything,
// it only records whether the outbound closed it.
type fakePacketConn struct {
	closed atomic.Bool
}

func (c *fakePacketConn) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	return M.Socksaddr{}, io.EOF
}
func (c *fakePacketConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	return nil
}
func (c *fakePacketConn) Close() error                     { c.closed.Store(true); return nil }
func (c *fakePacketConn) LocalAddr() net.Addr              { return &net.UDPAddr{IP: net.IPv4zero} }
func (c *fakePacketConn) SetDeadline(time.Time) error      { return nil }
func (c *fakePacketConn) SetReadDeadline(time.Time) error  { return nil }
func (c *fakePacketConn) SetWriteDeadline(time.Time) error { return nil }

// recordingPacketOutbound stands in for a member of the group and remembers
// that the flow reached it.
type recordingPacketOutbound struct {
	adapter.Outbound
	tag  string
	took atomic.Bool
}

func (o *recordingPacketOutbound) Tag() string  { return o.tag }
func (o *recordingPacketOutbound) Type() string { return o.tag }
func (o *recordingPacketOutbound) NewPacketConnectionEx(
	ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc,
) {
	o.took.Store(true)
	if onClose != nil {
		onClose(nil)
	}
}

// udpTestOutbound is a smart outbound with both members stubbed out and one
// name already learned as blocked.
func udpTestOutbound(t *testing.T, nodeUDPAlive bool) (*smartOutbound, *recordingPacketOutbound, *recordingPacketOutbound) {
	t.Helper()
	store := choiceStore(t)
	store.Learn("blocked.example", verdict.Proxy)

	direct := &recordingPacketOutbound{tag: "direct"}
	proxy := &recordingPacketOutbound{tag: "proxy"}
	return &smartOutbound{
		ctx:      context.Background(),
		logger:   log.NewNOPFactory().NewLogger("test"),
		direct:   direct,
		proxy:    proxy,
		store:    store,
		traffic:  trackerForTest(),
		health:   newDirectHealth(nil),
		probes:   newProbeGate(nil),
		udpAlive: func() bool { return nodeUDPAlive },
	}, direct, proxy
}

func udpMetadata(host string, port uint16) adapter.InboundContext {
	return adapter.InboundContext{
		Domain:      host,
		Destination: M.ParseSocksaddrHostPort(host, port),
	}
}

// End to end for the gate: the flow the list would have rejected is now
// rejected for a learned name too, instead of disappearing into a node whose
// UDP nobody has proven.
func TestPacketConnectionRefusesHTTP3OnAnUnprovenNode(t *testing.T) {
	s, _, proxy := udpTestOutbound(t, false)
	conn := &fakePacketConn{}

	s.NewPacketConnectionEx(context.Background(), conn, udpMetadata("blocked.example", 443), nil)

	if proxy.took.Load() {
		t.Fatal("HTTP/3 was handed to a node whose UDP was never measured")
	}
	if !conn.closed.Load() {
		t.Fatal("the flow was neither tunnelled nor refused — the client is left waiting")
	}
}

// And with the node measured, the same flow goes through and its bytes are
// booked to the node.
func TestPacketConnectionCarriesHTTP3OnAProvenNodeAndCountsIt(t *testing.T) {
	s, _, proxy := udpTestOutbound(t, true)
	conn := &fakePacketConn{}

	s.NewPacketConnectionEx(context.Background(), conn, udpMetadata("blocked.example", 443), nil)

	if !proxy.took.Load() {
		t.Fatal("HTTP/3 to a learned-blocked name never reached the node")
	}
	if conn.closed.Load() {
		t.Fatal("the flow was refused on a node measured carrying UDP")
	}
}

// The flag is only worth anything if it reads the record the probe actually
// wrote. app.go files the verdict under AutoNodeKeyOf(proxyDTO); if the engine
// looked it up under anything else the answer would be a permanent "not
// measured", and HTTP/3 would be off for every learned name forever — silently.
func TestNodeUDPCheckReadsTheKeyTheProbeWroteUnder(t *testing.T) {
	SetNodeStatStore(&NodeStatStore{stats: map[string]NodeStat{}})
	t.Cleanup(func() { SetNodeStatStore(nil) })

	p := ProxyConfig{IP: "203.0.113.7", Port: 443, Type: "vless", Username: "node-a"}
	// Exactly what startUDPRelayProbe does after a successful STUN exchange.
	RecordUDPRelayOutcome(AutoNodeKeyOf(p), true)

	if !nodeUDPCheckFor(p)() {
		t.Fatal("the engine could not find the verdict the probe had just written")
	}
}

// A different node's verdict must not answer for this one.
func TestNodeUDPCheckIsPerNode(t *testing.T) {
	SetNodeStatStore(&NodeStatStore{stats: map[string]NodeStat{}})
	t.Cleanup(func() { SetNodeStatStore(nil) })

	measured := ProxyConfig{IP: "203.0.113.7", Port: 443, Type: "vless", Username: "node-a"}
	other := ProxyConfig{IP: "198.51.100.9", Port: 443, Type: "vless", Username: "node-b"}
	RecordUDPRelayOutcome(AutoNodeKeyOf(measured), true)

	if nodeUDPCheckFor(other)() {
		t.Fatal("one node's UDP verdict answered for another")
	}
}
