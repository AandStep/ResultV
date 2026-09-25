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
	"encoding/json"
	"net"
	"net/netip"
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
)

const endpointListDomain = "vpnlist.example"

var endpointListAddr = netip.MustParseAddr("192.0.2.10")

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
