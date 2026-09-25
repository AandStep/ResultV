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
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
	tun "github.com/sagernet/sing-tun"
	singjson "github.com/sagernet/sing/common/json"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"

	mDNS "github.com/miekg/dns"
)

const endpointListDomain = "vpnlist.example"

var endpointListAddr = netip.MustParseAddr("192.0.2.10")

func randomWGKey(t *testing.T) string {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(key)
}

func adaptiveWireGuardConfig(t *testing.T) EngineConfig {
	t.Helper()
	extra, err := json.Marshal(map[string]interface{}{
		"private_key": randomWGKey(t),
		"public_key":  randomWGKey(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	list := filepath.Join(dir, "vpn-list.json")
	if err := os.WriteFile(list, []byte(`{"version":3,"rules":[{"domain_suffix":["`+endpointListDomain+`"]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := adaptiveTunnelConfig()
	cfg.DataDir = dir
	cfg.Proxy = ProxyConfig{Type: "WIREGUARD", IP: "203.0.113.7", Port: 51820, Extra: extra}
	cfg.RoutingLists = []RoutingListSpec{{Tag: "user-vpn", Path: list, Action: "proxy"}}
	return cfg
}

// startEndpointCore runs the built config in-process without the TUN inbound,
// with the tunnel resolver replaced by a static record so that "resolved
// through the tunnel" is observable without a live peer.
func startEndpointCore(t *testing.T, cfg SingBoxConfig) context.Context {
	t.Helper()
	tunnelTag := firstDetourServerTag(cfg.DNS.Servers, "proxy")
	if tunnelTag == "" {
		t.Fatal("no tunnel resolver in the built config")
	}
	for i, srv := range cfg.DNS.Servers {
		if srv.Tag == tunnelTag {
			cfg.DNS.Servers[i] = SBDNSServer{
				Type:       "hosts",
				Tag:        tunnelTag,
				Predefined: map[string][]string{endpointListDomain: {endpointListAddr.String()}},
			}
		}
	}
	var inbounds []SBInbound
	for _, in := range cfg.Inbounds {
		if in.Type != "tun" {
			inbounds = append(inbounds, in)
		}
	}
	cfg.Inbounds = inbounds

	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	boxCtx := extendedBoxContext(ctx)
	var opts option.Options
	if err := singjson.UnmarshalContext(boxCtx, raw, &opts); err != nil {
		t.Fatalf("core rejected the config: %v\n%s", err, raw)
	}
	instance, err := box.New(box.Options{Context: boxCtx, Options: opts})
	if err != nil {
		t.Fatalf("box.New: %v", err)
	}
	if err := instance.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		done := make(chan struct{})
		go func() { _ = instance.Close(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
		}
		cancel()
	})
	return boxCtx
}

func fakeAddressFor(t *testing.T, boxCtx context.Context, name string) netip.Addr {
	t.Helper()
	msg := new(mDNS.Msg)
	msg.SetQuestion(mDNS.Fqdn(name), mDNS.TypeA)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := service.FromContext[adapter.DNSRouter](boxCtx).Exchange(ctx, msg, adapter.DNSQueryOptions{})
	if err != nil {
		t.Fatalf("exchange %s: %v", name, err)
	}
	for _, answer := range resp.Answer {
		if a, isA := answer.(*mDNS.A); isA {
			addr, _ := netip.AddrFromSlice(a.A.To4())
			if !isFakeIPAddr(a.A) {
				t.Fatalf("%s answered with a real address %s, the test needs a fake one", name, addr)
			}
			return addr
		}
	}
	t.Fatalf("%s: no A record", name)
	return netip.Addr{}
}

// A name the user sent to the VPN reaches the router as a fake address. For UDP
// the WireGuard endpoint takes it on the pre-match path, where packets go
// straight into the tunnel and there is no dial to resolve the name at; with
// nothing resolving it first the core refuses the flow outright.
func TestEndpointTakesAFakeUDPDestinationFromTheUsersVPNList(t *testing.T) {
	boxCtx := startEndpointCore(t, mustBuildTunnelModeConfig(t, adaptiveWireGuardConfig(t)))
	fake := fakeAddressFor(t, boxCtx, endpointListDomain)

	res := service.FromContext[adapter.Router](boxCtx).PreMatch(adapter.InboundContext{
		Inbound:     "tun-in",
		InboundType: "tun",
		Network:     N.NetworkUDP,
		Source:      M.SocksaddrFrom(netip.MustParseAddr("172.19.0.1"), 50000),
		Destination: M.SocksaddrFrom(fake, 27015),
	}, []byte("not a sniffable protocol"))

	if res.Action != adapter.PreMatchFlow {
		t.Fatalf("pre-match action = %v, want a flow into the endpoint", res.Action)
	}
	if want := netip.AddrPortFrom(endpointListAddr, 27015); res.Destination != want {
		t.Fatalf("flow destination = %v, want %v resolved through the tunnel", res.Destination, want)
	}
}

type routedMetadata chan adapter.InboundContext

func (r routedMetadata) RoutedConnection(_ context.Context, conn net.Conn, metadata adapter.InboundContext, _ adapter.Rule, _ adapter.Outbound) net.Conn {
	r <- metadata
	return conn
}

func (r routedMetadata) RoutedPacketConnection(_ context.Context, conn N.PacketConn, metadata adapter.InboundContext, _ adapter.Rule, _ adapter.Outbound) N.PacketConn {
	r <- metadata
	return conn
}

func (routedMetadata) RoutedFlow(context.Context, adapter.InboundContext, adapter.Rule, adapter.Outbound) tun.FlowTracker {
	return nil
}

// TCP never reaches the pre-match path (the sniff rule sends it to the
// connection path first), so the name survives to the endpoint's own dial, and
// the endpoint resolves it on dns.final — the system resolver in Smart. For a
// name the user sent to the VPN that is the resolver it was sent away from.
func TestEndpointGetsTunnelAddressesForAFakeTCPDestination(t *testing.T) {
	boxCtx := startEndpointCore(t, mustBuildTunnelModeConfig(t, adaptiveWireGuardConfig(t)))
	fake := fakeAddressFor(t, boxCtx, endpointListDomain)

	router := service.FromContext[adapter.Router](boxCtx)
	routed := make(routedMetadata, 1)
	router.AppendTracker(routed)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, server := net.Pipe()
	defer client.Close()
	go router.RouteConnectionEx(ctx, server, adapter.InboundContext{
		Inbound:     "tun-in",
		InboundType: "tun",
		Network:     N.NetworkTCP,
		Source:      M.SocksaddrFrom(netip.MustParseAddr("172.19.0.1"), 50001),
		Destination: M.SocksaddrFrom(fake, 443),
	}, func(error) {})

	select {
	case md := <-routed:
		if md.RouteOutbound != "proxy" {
			t.Fatalf("routed to %q, want the endpoint", md.RouteOutbound)
		}
		if len(md.DestinationAddresses) != 1 || md.DestinationAddresses[0] != endpointListAddr {
			t.Fatalf("endpoint handed addresses %v, want [%v] from the tunnel resolver", md.DestinationAddresses, endpointListAddr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("connection was never routed")
	}
}

// routeFromOwnProcess routes a connection that the process searcher attributes
// to this test binary, the way the prober's own requests are attributed to the
// app, and reports what the router decided.
func routeFromOwnProcess(t *testing.T, boxCtx context.Context, inbound string, destination M.Socksaddr) adapter.InboundContext {
	t.Helper()
	router := service.FromContext[adapter.Router](boxCtx)
	routed := make(routedMetadata, 1)
	router.AppendTracker(routed)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		if c, _ := ln.Accept(); c != nil {
			defer c.Close()
			time.Sleep(3 * time.Second)
		}
	}()
	own, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer own.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, server := net.Pipe()
	defer client.Close()
	go router.RouteConnectionEx(ctx, server, adapter.InboundContext{
		Inbound:     inbound,
		InboundType: "mixed",
		Network:     N.NetworkTCP,
		Source:      M.SocksaddrFromNet(own.LocalAddr()),
		Destination: destination,
	}, func(error) {})
	select {
	case md := <-routed:
		return md
	case <-time.After(5 * time.Second):
		t.Fatal("connection was never routed")
	}
	return adapter.InboundContext{}
}

// The block prober's "through the node" half asks the probe inbound for hosts
// no list mentions. The request comes from the app's own process, so the
// self-direct rule used to claim it and the prober compared the direct path
// with itself.
func TestProbeInboundAsksTheNodeForAnyHost(t *testing.T) {
	cfg := adaptiveTunnelConfig()
	cfg.DataDir = t.TempDir()
	boxCtx := startEndpointCore(t, mustBuildTunnelModeConfig(t, cfg))

	md := routeFromOwnProcess(t, boxCtx, probeInboundTag, M.ParseSocksaddrHostPort("unlisted.example", 443))
	if md.ProcessInfo == nil {
		t.Fatal("the process searcher did not attribute the connection; the test proves nothing")
	}
	if md.RouteOutbound != "proxy" {
		t.Fatalf("probe inbound routed to %q by %q, want the node", md.RouteOutbound, md.RouteRule)
	}
}

// Through a WireGuard node the probe's name has to be resolved by the tunnel
// resolver too, or the node would be asked for the address the local resolver
// handed out.
func TestProbeInboundThroughAnEndpointUsesTheTunnelResolver(t *testing.T) {
	boxCtx := startEndpointCore(t, mustBuildTunnelModeConfig(t, adaptiveWireGuardConfig(t)))

	md := routeFromOwnProcess(t, boxCtx, probeInboundTag, M.ParseSocksaddrHostPort(endpointListDomain, 443))
	if md.RouteOutbound != "proxy" {
		t.Fatalf("probe inbound routed to %q by %q, want the endpoint", md.RouteOutbound, md.RouteRule)
	}
	if len(md.DestinationAddresses) != 1 || md.DestinationAddresses[0] != endpointListAddr {
		t.Fatalf("endpoint handed addresses %v, want [%v] from the tunnel resolver", md.DestinationAddresses, endpointListAddr)
	}
}

// A name no rule has an opinion about goes to the smart group. On the
// pre-match path the core replaces a group by whatever Now() names, and a
// WireGuard endpoint takes every flow it is offered there — so if Now() ever
// named the endpoint, all of this traffic would go into the tunnel without the
// group deciding anything.
func TestUnlistedNameOnAnEndpointNodeIsLeftToTheSmartGroup(t *testing.T) {
	boxCtx := startEndpointCore(t, mustBuildTunnelModeConfig(t, adaptiveWireGuardConfig(t)))
	fake := fakeAddressFor(t, boxCtx, "unlisted.example")

	res := service.FromContext[adapter.Router](boxCtx).PreMatch(adapter.InboundContext{
		Inbound:     "tun-in",
		InboundType: "tun",
		Network:     N.NetworkUDP,
		Source:      M.SocksaddrFrom(netip.MustParseAddr("172.19.0.1"), 50002),
		Destination: M.SocksaddrFrom(fake, 27015),
	}, []byte("not a sniffable protocol"))

	if res.Action != adapter.PreMatchContinue {
		t.Fatalf("pre-match action = %v via %v, want the connection path to the smart group", res.Action, res.Outbound)
	}
}
