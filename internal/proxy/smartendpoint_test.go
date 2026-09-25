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
	"errors"
	"net"
	"net/netip"
	"sync"
	"testing"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/log"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

	"resultproxy-wails/internal/verdict"
)

// recordingMember is a group member that remembers what it was handed.
type recordingMember struct {
	adapter.Outbound
	tag string

	mu       sync.Mutex
	dialed   []M.Socksaddr
	handed   []adapter.InboundContext
	listened []M.Socksaddr
}

func (o *recordingMember) Tag() string  { return o.tag }
func (o *recordingMember) Type() string { return o.tag }

func (o *recordingMember) DialContext(_ context.Context, _ string, destination M.Socksaddr) (net.Conn, error) {
	o.mu.Lock()
	o.dialed = append(o.dialed, destination)
	o.mu.Unlock()
	client, server := net.Pipe()
	go server.Close()
	return client, nil
}

func (o *recordingMember) ListenPacket(_ context.Context, destination M.Socksaddr) (net.PacketConn, error) {
	o.mu.Lock()
	o.listened = append(o.listened, destination)
	o.mu.Unlock()
	return nil, errors.New("recording member does not listen")
}

func (o *recordingMember) NewConnection(_ context.Context, conn net.Conn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	o.mu.Lock()
	o.handed = append(o.handed, metadata)
	o.mu.Unlock()
	conn.Close()
	onClose(nil)
}

func (o *recordingMember) NewPacketConnection(_ context.Context, conn N.PacketConn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	o.mu.Lock()
	o.handed = append(o.handed, metadata)
	o.mu.Unlock()
	onClose(nil)
}

var tunnelAnswer = netip.MustParseAddr("192.0.2.44")

func endpointSmart(t *testing.T, withResolver bool) (*smartOutbound, *recordingMember) {
	t.Helper()
	store := choiceStore(t)
	store.Learn("learned.example", verdict.Proxy)
	proxy := &recordingMember{tag: "proxy"}
	s := &smartOutbound{
		ctx:      context.Background(),
		logger:   log.NewNOPFactory().NewLogger("test"),
		direct:   &recordingMember{tag: "direct"},
		proxy:    proxy,
		store:    store,
		traffic:  trackerForTest(),
		health:   newDirectHealth(nil),
		probes:   newProbeGate(nil),
		udpAlive: func() bool { return true },
	}
	if withResolver {
		s.resolveProxy = func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{tunnelAnswer}, nil
		}
	}
	return s, proxy
}

// A name already known to need the tunnel is handed to the endpoint whole. The
// endpoint would resolve it on dns.final, the system resolver in Smart, so the
// addresses have to come with it.
func TestKnownProxyNameReachesTheEndpointWithTunnelAddresses(t *testing.T) {
	s, proxy := endpointSmart(t, true)
	client, server := net.Pipe()
	defer client.Close()

	s.NewConnection(context.Background(), server, adapter.InboundContext{
		Destination: M.ParseSocksaddrHostPort("learned.example", 443),
	}, func(error) {})

	if len(proxy.handed) != 1 {
		t.Fatalf("proxy member got %d connections, want 1", len(proxy.handed))
	}
	if got := proxy.handed[0].DestinationAddresses; len(got) != 1 || got[0] != tunnelAnswer {
		t.Fatalf("endpoint handed addresses %v, want [%v]", got, tunnelAnswer)
	}
}

func TestKnownProxyUDPReachesTheEndpointWithTunnelAddresses(t *testing.T) {
	s, proxy := endpointSmart(t, true)

	s.NewPacketConnection(context.Background(), &fakePacketConn{}, adapter.InboundContext{
		Destination: M.ParseSocksaddrHostPort("learned.example", 27015),
	}, func(error) {})

	if len(proxy.handed) != 1 {
		t.Fatalf("proxy member got %d flows, want 1", len(proxy.handed))
	}
	if got := proxy.handed[0].DestinationAddresses; len(got) != 1 || got[0] != tunnelAnswer {
		t.Fatalf("endpoint handed addresses %v, want [%v]", got, tunnelAnswer)
	}
}

// The race's tunnel leg dials the endpoint itself, so it dials the address.
func TestRaceLegThroughTheEndpointDialsTheTunnelAddress(t *testing.T) {
	s, proxy := endpointSmart(t, true)
	conn, err := s.dialProxy(context.Background(), N.NetworkTCP, M.ParseSocksaddrHostPort("unknown.example", 443))
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	if len(proxy.dialed) != 1 || proxy.dialed[0].Addr != tunnelAnswer || proxy.dialed[0].Port != 443 {
		t.Fatalf("endpoint dialed %v, want %v:443", proxy.dialed, tunnelAnswer)
	}
}

// A name that does not resolve through the tunnel must not quietly fall back to
// the endpoint's own lookup on the system resolver.
func TestUnresolvableNameIsRefusedInsteadOfResolvedLocally(t *testing.T) {
	s, proxy := endpointSmart(t, false)
	s.resolveProxy = func(context.Context, string) ([]netip.Addr, error) {
		return nil, errors.New("no answer")
	}
	if _, err := s.dialProxy(context.Background(), N.NetworkTCP, M.ParseSocksaddrHostPort("unknown.example", 443)); err == nil {
		t.Fatal("dial succeeded without an address")
	}
	client, server := net.Pipe()
	defer client.Close()
	s.NewConnection(context.Background(), server, adapter.InboundContext{
		Destination: M.ParseSocksaddrHostPort("learned.example", 443),
	}, func(error) {})
	if len(proxy.dialed)+len(proxy.handed) != 0 {
		t.Fatal("the endpoint was handed a name nothing resolved")
	}
}

// A proxy outbound that carries names to the far side keeps getting them.
func TestNameStaysANameForAProxyOutbound(t *testing.T) {
	s, proxy := endpointSmart(t, false)
	conn, err := s.dialProxy(context.Background(), N.NetworkTCP, M.ParseSocksaddrHostPort("unknown.example", 443))
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	if len(proxy.dialed) != 1 || proxy.dialed[0].Fqdn != "unknown.example" {
		t.Fatalf("proxy outbound dialed %v, want the name", proxy.dialed)
	}

	client, server := net.Pipe()
	defer client.Close()
	s.NewConnection(context.Background(), server, adapter.InboundContext{
		Destination: M.ParseSocksaddrHostPort("learned.example", 443),
	}, func(error) {})
	if len(proxy.handed) != 1 || len(proxy.handed[0].DestinationAddresses) != 0 {
		t.Fatalf("proxy outbound was handed %v", proxy.handed)
	}
}

func TestSmartGroupNamesTheTunnelResolverOnlyForEndpoints(t *testing.T) {
	built := mustBuildTunnelModeConfig(t, adaptiveWireGuardConfig(t))
	smart, ok := outboundByTag(built, smartOutboundTag)
	if !ok {
		t.Fatal("no smart group on a WireGuard node with the switch on")
	}
	if want := firstDetourServerTag(built.DNS.Servers, "proxy"); smart.ProxyResolver == "" || smart.ProxyResolver != want {
		t.Fatalf("proxy_resolver = %q, want the tunnel resolver %q", smart.ProxyResolver, want)
	}

	built = mustBuildTunnelModeConfig(t, adaptiveTunnelConfig())
	smart, _ = outboundByTag(built, smartOutboundTag)
	if smart.ProxyResolver != "" {
		t.Fatalf("a VLESS node's smart group names a resolver: %q", smart.ProxyResolver)
	}
}
