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
	"net"
	"testing"

	"github.com/sagernet/sing-box/adapter"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"
)

type fakeConnectionManager struct {
	closedAll bool
}

func (f *fakeConnectionManager) Start(adapter.StartStage) error { return nil }
func (f *fakeConnectionManager) Close() error                   { return nil }
func (f *fakeConnectionManager) Count() int                     { return 0 }
func (f *fakeConnectionManager) CloseAll()                      { f.closedAll = true }
func (f *fakeConnectionManager) TrackConn(c net.Conn) net.Conn  { return c }
func (f *fakeConnectionManager) TrackPacketConn(c net.PacketConn) net.PacketConn {
	return c
}
func (f *fakeConnectionManager) NewConnection(context.Context, N.Dialer, net.Conn, adapter.InboundContext, N.CloseHandlerFunc) {
}
func (f *fakeConnectionManager) NewPacketConnection(context.Context, N.Dialer, N.PacketConn, adapter.InboundContext, N.CloseHandlerFunc) {
}

// The goroutine dumps taken on 14.09.2026 (diag/close-hang-*.txt) show exactly
// one thing holding the disconnect: box.Close reaches the WireGuard endpoint,
// whose gVisor stack calls Stack.Wait, which sits in tcp.Endpoint.Wait waiting
// for a HUp that never comes. Those TCP endpoints belong to connections the
// connection manager owns — and box.Close shuts the connection manager down
// SIX entries after the endpoint, so nothing ever closes them. Closing them
// first is what lets Wait return.
func TestCloseTrackedConnectionsClosesEverythingBeforeTeardown(t *testing.T) {
	cm := &fakeConnectionManager{}
	ctx := service.ContextWith[adapter.ConnectionManager](context.Background(), adapter.ConnectionManager(cm))
	closeTrackedConnections(ctx, nil)
	if !cm.closedAll {
		t.Fatal("CloseAll was not called — the WireGuard stack would wait for these connections forever")
	}
}

// A context without the service (or none at all) must not panic: this runs on
// the disconnect path, where crashing is strictly worse than a slow close.
func TestCloseTrackedConnectionsToleratesMissingManager(t *testing.T) {
	closeTrackedConnections(context.Background(), nil)
	closeTrackedConnections(nil, nil)
}
