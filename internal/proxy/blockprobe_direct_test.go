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
	"strconv"
	"testing"
)

// The prober compares the direct path against the node. If its "direct" half
// resolves through the OS resolver it gets a fake address, dials into the TUN,
// and is routed by the very engine it is supposed to be measuring — so both
// halves would describe the tunnel and classifyProbe would compare a thing with
// itself. It must refuse rather than measure the wrong thing.
func TestProbeDirectDialRefusesAFakeAddress(t *testing.T) {
	resetPingResolveCache()
	t.Cleanup(resetPingResolveCache)

	prevLookup := pingLookupIPAddr
	pingLookupIPAddr = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("198.18.0.224")}}, nil
	}
	t.Cleanup(func() { pingLookupIPAddr = prevLookup })

	prevDoH := pingDoHResolve
	pingDoHResolve = func(host string) []string { return nil }
	t.Cleanup(func() { pingDoHResolve = prevDoH })

	dialed := false
	prevBind := pickLANBindIPv4
	pickLANBindIPv4 = func() (net.IP, error) {
		dialed = true
		return net.ParseIP("192.168.0.11"), nil
	}
	t.Cleanup(func() { pickLANBindIPv4 = prevBind })

	conn, err := probeDirectDial(context.Background(), "tcp", "example.com:443")
	if err == nil {
		conn.Close()
		t.Fatal("the direct half dialled an address that only exists inside the tunnel")
	}
	if dialed {
		t.Fatal("it got as far as picking a bind address for an unusable destination")
	}
}

// Without an address to bind to there is no way to guarantee the dial bypasses
// the tunnel, and a probe that silently measures the tunnel twice is worse than
// no probe at all.
func TestProbeDirectDialRefusesWithoutABindAddress(t *testing.T) {
	resetPingResolveCache()
	t.Cleanup(resetPingResolveCache)

	prevLookup := pingLookupIPAddr
	pingLookupIPAddr = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("203.0.113.7")}}, nil
	}
	t.Cleanup(func() { pingLookupIPAddr = prevLookup })

	prevBind := pickLANBindIPv4
	pickLANBindIPv4 = func() (net.IP, error) { return nil, errors.New("no suitable LAN IPv4 for bind") }
	t.Cleanup(func() { pickLANBindIPv4 = prevBind })

	if conn, err := probeDirectDial(context.Background(), "tcp", "example.com:443"); err == nil {
		conn.Close()
		t.Fatal("the direct half dialled without binding to the physical adapter")
	}
}

// The happy path still has to reach a real server, bound to the real adapter.
func TestProbeDirectDialReachesARealListener(t *testing.T) {
	resetPingResolveCache()
	t.Cleanup(resetPingResolveCache)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		if c, err := ln.Accept(); err == nil {
			c.Close()
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)

	prevLookup := pingLookupIPAddr
	pingLookupIPAddr = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	t.Cleanup(func() { pingLookupIPAddr = prevLookup })

	prevBind := pickLANBindIPv4
	pickLANBindIPv4 = func() (net.IP, error) { return net.ParseIP("127.0.0.1"), nil }
	t.Cleanup(func() { pickLANBindIPv4 = prevBind })

	conn, err := probeDirectDial(context.Background(), "tcp", net.JoinHostPort("probe.example", strconv.Itoa(addr.Port)))
	if err != nil {
		t.Fatalf("a reachable listener was not reached: %v", err)
	}
	conn.Close()
}
