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
	"testing"

	"github.com/sagernet/sing-box/adapter"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

// A WireGuard node in tunnel mode no longer reaches RoutedConnection at all on
// sing-box 1.14: the TUN asks the router about every new flow, the WireGuard
// endpoint answers PreMatchFlow unconditionally and implements tun.Port, so the
// packets are forwarded at L3 and never become a net.Conn this process wraps.
// The byte counters behind the speed indicator and the traffic veto would read
// zero for the whole session. RoutedFlow is where those bytes are booked now.
func TestRoutedFlowCountsBytesForForwardedFlows(t *testing.T) {
	tr := trackerForTest()
	flow := tr.RoutedFlow(context.Background(), adapter.InboundContext{
		Network:     N.NetworkTCP,
		Source:      M.ParseSocksaddr("10.0.0.2:51820"),
		Destination: M.ParseSocksaddr("203.0.113.7:443"),
	}, nil, stubOutbound{tag: "proxy"})
	if flow == nil {
		t.Fatal("RoutedFlow returned nil for a TCP flow — a WireGuard session would be invisible to the speed indicator")
	}
	flow.CountForward(1500)
	flow.CountReverse(9000)
	if got := tr.upload.Load(); got != 1500 {
		t.Fatalf("upload = %d, want 1500", got)
	}
	if got := tr.download.Load(); got != 9000 {
		t.Fatalf("download = %d, want 9000", got)
	}
}

// ICMP is the one flow the core asks about even on a plain node: `direct`
// implements FlowOutbound solely for it. It never fed the counters before 1.14
// — pings are not the traffic the indicator is about — and it must not start
// now, so the answer stays nil rather than a tracker that books ping bytes.
func TestRoutedFlowIgnoresICMP(t *testing.T) {
	tr := trackerForTest()
	if flow := tr.RoutedFlow(context.Background(), adapter.InboundContext{
		Network:     N.NetworkICMP,
		Source:      M.ParseSocksaddr("10.0.0.2:0"),
		Destination: M.ParseSocksaddr("1.1.1.1:0"),
	}, nil, stubOutbound{tag: "direct"}); flow != nil {
		t.Fatal("RoutedFlow must stay out of ICMP accounting")
	}
}
