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
	"net/netip"
	"slices"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/outbound"
	"github.com/sagernet/sing-box/log"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"
)

// appDirectTag is the outbound excluded app families leave through: direct,
// but addressed by the system resolver. Windows resolves names in the DNS
// Client service, so an excluded app's lookup carries no process and in Global
// is answered through the node, with a server near the node. On the
// connection the process is known, and the name comes from the sniffer,
// FakeIP or the reverse mapping, so it is resolved again locally. The original
// address stays as the last fallback.
const appDirectTag = "app-direct"

type appDirectOptions struct{}

var (
	_ adapter.ConnectionHandler       = (*appDirectOutbound)(nil)
	_ adapter.PacketConnectionHandler = (*appDirectOutbound)(nil)
)

type appDirectOutbound struct {
	outbound.Adapter
	ctx        context.Context
	outbounds  adapter.OutboundManager
	connection adapter.ConnectionManager
	logger     logger.ContextLogger

	direct adapter.Outbound
	lookup func(ctx context.Context, fqdn string) ([]netip.Addr, error)
}

func newAppDirectOutbound(ctx context.Context, _ adapter.Router, logger log.ContextLogger, tag string, _ appDirectOptions) (adapter.Outbound, error) {
	return &appDirectOutbound{
		Adapter:    outbound.NewAdapter(appDirectTag, tag, []string{N.NetworkTCP, N.NetworkUDP}, []string{"direct"}),
		ctx:        ctx,
		outbounds:  service.FromContext[adapter.OutboundManager](ctx),
		connection: service.FromContext[adapter.ConnectionManager](ctx),
		logger:     logger,
	}, nil
}

func (a *appDirectOutbound) Start() error {
	direct, loaded := a.outbounds.Outbound("direct")
	if !loaded {
		return E.New("app-direct: outbound not found: direct")
	}
	a.direct = direct
	if transport, loaded := service.FromContext[adapter.DNSTransportManager](a.ctx).Transport("local"); loaded {
		dns := service.FromContext[adapter.DNSRouter](a.ctx)
		a.lookup = func(ctx context.Context, fqdn string) ([]netip.Addr, error) {
			return dns.Lookup(ctx, fqdn, adapter.DNSQueryOptions{Transport: transport})
		}
	}
	return nil
}

func (a *appDirectOutbound) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	return a.direct.DialContext(ctx, network, destination)
}

func (a *appDirectOutbound) ListenPacket(ctx context.Context, destination M.Socksaddr) (net.PacketConn, error) {
	return a.direct.ListenPacket(ctx, destination)
}

func (a *appDirectOutbound) NewConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	a.localize(ctx, &metadata, true)
	a.connection.NewConnection(ctx, a.direct, conn, metadata, onClose)
}

func (a *appDirectOutbound) NewPacketConnection(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	// UDP has no failed dial to fall back from: the first address is the one
	// the flow is pinned to.
	a.localize(ctx, &metadata, false)
	a.connection.NewPacketConnection(ctx, a.direct, conn, metadata, onClose)
}

// localize points the connection at what the system resolver says the name
// is. A name it cannot resolve leaves the connection as it came.
func (a *appDirectOutbound) localize(ctx context.Context, metadata *adapter.InboundContext, keepOriginal bool) {
	host := appDirectHost(metadata)
	if host == "" || a.lookup == nil {
		return
	}
	addrs, err := a.lookup(ctx, host)
	if err != nil || len(addrs) == 0 {
		a.logger.DebugContext(ctx, "app-direct: local lookup of ", host, " failed, keeping the original address: ", err)
		return
	}
	metadata.DestinationAddresses = withOriginal(addrs, metadata.Destination, keepOriginal)
}

// appDirectHost is the name worth resolving again, or "" when there is none or
// when the destination is on the local network.
func appDirectHost(metadata *adapter.InboundContext) string {
	if metadata.Destination.IsDomain() {
		return metadata.Destination.Fqdn
	}
	if !metadata.Destination.IsIP() || !isPublicAddr(metadata.Destination.Addr) {
		return ""
	}
	if M.IsDomainName(metadata.Domain) {
		return metadata.Domain
	}
	return ""
}

func isPublicAddr(addr netip.Addr) bool {
	return addr.IsGlobalUnicast() && !addr.IsPrivate()
}

func withOriginal(addrs []netip.Addr, destination M.Socksaddr, keepOriginal bool) []netip.Addr {
	if !keepOriginal || !destination.IsIP() || slices.Contains(addrs, destination.Addr) {
		return addrs
	}
	return append(append([]netip.Addr(nil), addrs...), destination.Addr)
}
