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
	"net/netip"
	"testing"
	"time"

	"github.com/sagernet/gvisor/pkg/tcpip"
	"github.com/sagernet/gvisor/pkg/tcpip/adapters/gonet"
	"github.com/sagernet/gvisor/pkg/tcpip/link/channel"
	"github.com/sagernet/gvisor/pkg/tcpip/network/ipv4"
	"github.com/sagernet/gvisor/pkg/tcpip/stack"
	"github.com/sagernet/gvisor/pkg/tcpip/transport/tcp"
)

// blackholeStack is a gVisor stack whose peer never answers: every SYN goes
// out and nothing comes back, as with a WireGuard bind that is already down.
func blackholeStack(t *testing.T) *stack.Stack {
	t.Helper()
	s := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol},
	})
	link := channel.New(1024, 1500, "")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		for {
			if pkt := link.ReadContext(ctx); pkt == nil {
				return
			} else {
				pkt.DecRef()
			}
		}
	}()
	if err := s.CreateNIC(1, link); err != nil {
		t.Fatalf("CreateNIC: %v", err)
	}
	if err := s.AddProtocolAddress(1, tcpip.ProtocolAddress{
		Protocol:          ipv4.ProtocolNumber,
		AddressWithPrefix: tcpip.AddrFrom4([4]byte{10, 0, 0, 1}).WithPrefix(),
	}, stack.AddressProperties{}); err != nil {
		t.Fatalf("AddProtocolAddress: %v", err)
	}
	s.SetRouteTable([]tcpip.Route{{Destination: header4Any(), NIC: 1}})
	return s
}

func header4Any() tcpip.Subnet {
	subnet, _ := tcpip.NewSubnet(tcpip.AddrFrom4([4]byte{}), tcpip.MaskFromBytes([]byte{0, 0, 0, 0}))
	return subnet
}

// A dial that arrives after Stack.Close has taken its snapshot is what held
// Close for minutes on 24.09.2026. Its context is never cancelled, like the
// one http.Transport uses for plain HTTP through the mixed inbound.
func TestAbortStackEndpointsReleasesLateDial(t *testing.T) {
	s := blackholeStack(t)
	s.Close()

	dialDone := make(chan struct{})
	go func() {
		defer close(dialDone)
		addr := netip.MustParseAddr("10.0.0.2").As4()
		conn, err := gonet.DialContextTCP(context.Background(), s, tcpip.FullAddress{
			NIC: 1, Addr: tcpip.AddrFrom4(addr), Port: 80,
		}, ipv4.ProtocolNumber)
		if err == nil {
			conn.Close()
		}
	}()
	deadline := time.Now().Add(2 * time.Second)
	for len(s.RegisteredEndpoints()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("дозвон так и не появился на стеке")
		}
		time.Sleep(10 * time.Millisecond)
	}

	waitDone := make(chan struct{})
	go func() {
		s.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
		t.Fatal("Stack.Wait вернулся без прерывания — сценарий зависания не воспроизведён")
	case <-time.After(time.Second):
	}

	stop := abortStackEndpointsUntil(s, 50*time.Millisecond)
	defer stop()
	select {
	case <-waitDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Stack.Wait висит и после прерывания эндпоинтов")
	}
	select {
	case <-dialDone:
	case <-time.After(time.Second):
		t.Fatal("дозвон не получил ошибку после прерывания")
	}
}
