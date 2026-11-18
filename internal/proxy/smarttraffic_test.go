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

// Once the choice is proxy, the bytes have to land in the node counters — the
// speed indicator is the only place the user can see this.
func TestAttributingAConnectionFeedsTheProxyCounters(t *testing.T) {
	tr := trackerForTest()
	exchangeTrackedTCP(t, tr.attributeProxyConn)
	if up, down := tr.proxyUpload.Load(), tr.proxyDownload.Load(); up != 3 || down != 7 {
		t.Fatalf("adaptive TCP upload=%d download=%d, want 3 and 7", up, down)
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

	if err := wrapped.WritePacket(buf.As([]byte("reply")), M.SocksaddrFromNet(right.LocalAddr())); err != nil {
		t.Fatal(err)
	}
	if up, down := tr.proxyUpload.Load(), tr.proxyDownload.Load(); up != 10 || down != 5 {
		t.Fatalf("adaptive UDP upload=%d download=%d, want 10 and 5", up, down)
	}
}
