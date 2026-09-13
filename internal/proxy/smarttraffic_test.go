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
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"

	"resultproxy-wails/internal/logger"
)

type stubOutbound struct {
	adapter.Outbound
	tag string
}

func (s stubOutbound) Tag() string  { return s.tag }
func (s stubOutbound) Type() string { return s.tag }

// newPipeConnPair is two ends of an in-memory connection. net.Pipe is
// synchronous and unbuffered, which makes it a precise stand-in for a server
// that says nothing until it is spoken to.
func newPipeConnPair() (net.Conn, net.Conn) {
	return net.Pipe()
}

func trackerForTest() *trafficTracker {
	return &trafficTracker{
		upload:        new(atomic.Int64),
		download:      new(atomic.Int64),
		proxyUpload:   new(atomic.Int64),
		proxyDownload: new(atomic.Int64),
		log:           logger.New(),
	}
}

// Before the smart outbound has chosen, nothing may be attributed to the node:
// the tracker runs first (fork route/route.go:158) and at that moment the
// answer does not exist yet.
func TestSmartOutboundIsNotCountedAsProxyTrafficUpFront(t *testing.T) {
	tr := trackerForTest()
	_, _, shouldTrack := tr.logConnection(
		adapter.InboundContext{Domain: "example.com", Destination: M.ParseSocksaddr("example.com:443")},
		stubOutbound{tag: smartOutboundTag},
	)
	if shouldTrack {
		t.Fatal("traffic was booked to the node before the outbound had chosen")
	}
}

// The pre-existing outbounds must keep behaving exactly as before.
func TestPlainOutboundsKeepTheirAttribution(t *testing.T) {
	meta := adapter.InboundContext{Domain: "example.com", Destination: M.ParseSocksaddr("example.com:443")}
	tr := trackerForTest()
	if _, _, ok := tr.logConnection(meta, stubOutbound{tag: "proxy"}); !ok {
		t.Error("proxy traffic stopped being counted as proxy traffic")
	}
	tr = trackerForTest()
	if _, _, ok := tr.logConnection(meta, stubOutbound{tag: "direct"}); ok {
		t.Error("direct traffic started being counted as proxy traffic")
	}
}

// Once the choice is proxy, the bytes have to land in the node counters — the
// speed indicator is the only place the user can see this.
func TestAttributingAConnectionFeedsTheProxyCounters(t *testing.T) {
	tr := trackerForTest()
	left, right := newPipeConnPair()
	defer left.Close()
	defer right.Close()

	wrapped := tr.attributeProxyConn(left)
	go func() {
		right.Write([]byte("0123456789"))
	}()
	buf := make([]byte, 10)
	if _, err := wrapped.Read(buf); err != nil {
		t.Fatal(err)
	}
	if tr.proxyDownload.Load() == 0 && tr.proxyUpload.Load() == 0 {
		t.Fatal("ten bytes through an attributed connection moved no node counter")
	}
}

// The node's share of the traffic was measured for TCP and not for UDP: the
// packet path only wrote its [CONN] line. In Smart mode with the adaptive
// engine on, everything HTTP/3 sends through the node therefore vanished from
// the speed indicator.
func TestAttributingAPacketConnectionFeedsTheProxyCounters(t *testing.T) {
	tr := trackerForTest()

	left, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no loopback UDP socket available: %v", err)
	}
	defer left.Close()
	right, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no loopback UDP socket available: %v", err)
	}
	defer right.Close()

	wrapped := tr.attributeProxyPacketConn(bufio.NewPacketConn(left))
	if _, err := right.WriteTo([]byte("0123456789"), left.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	if err := left.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	buffer := buf.NewSize(64)
	defer buffer.Release()
	if _, err := wrapped.ReadPacket(buffer); err != nil {
		t.Fatal(err)
	}

	if tr.proxyDownload.Load() == 0 && tr.proxyUpload.Load() == 0 {
		t.Fatal("ten bytes through an attributed packet connection moved no node counter")
	}
	// Same orientation as the TCP wrapper and as RoutedPacketConnection: an
	// inbound packet is download. Getting this backwards would make the node's
	// share move in the opposite direction from the total.
	if tr.proxyDownload.Load() == 0 {
		t.Fatalf("an inbound packet was booked as upload: down=%d up=%d",
			tr.proxyDownload.Load(), tr.proxyUpload.Load())
	}
}
