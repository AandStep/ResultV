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

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/outbound"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/log"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"

	"resultproxy-wails/internal/verdict"
)

// smartOutboundTag is both the type name and the tag. One group, one instance,
// no reason for the two to differ.
const smartOutboundTag = "smart"

// smartOutboundOptions is the config shape of the "smart" outbound. Only the
// member list: everything else it needs comes from the service context, which
// is how it reaches state that cannot survive a trip through JSON.
type smartOutboundOptions struct {
	Outbounds []string `json:"outbounds,omitempty"`
}

var (
	_ adapter.OutboundGroup             = (*smartOutbound)(nil)
	_ adapter.ConnectionHandlerEx       = (*smartOutbound)(nil)
	_ adapter.PacketConnectionHandlerEx = (*smartOutbound)(nil)
)

type smartOutbound struct {
	outbound.Adapter
	ctx        context.Context
	outbound   adapter.OutboundManager
	connection adapter.ConnectionManager
	logger     logger.ContextLogger

	tags   []string
	direct adapter.Outbound
	proxy  adapter.Outbound
	store  *verdict.Store
}

func newSmartOutbound(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options smartOutboundOptions) (adapter.Outbound, error) {
	return &smartOutbound{
		Adapter:    outbound.NewAdapter(smartOutboundTag, tag, []string{N.NetworkTCP, N.NetworkUDP}, options.Outbounds),
		ctx:        ctx,
		outbound:   service.FromContext[adapter.OutboundManager](ctx),
		connection: service.FromContext[adapter.ConnectionManager](ctx),
		logger:     logger,
		tags:       options.Outbounds,
		store:      service.FromContext[*verdict.Store](ctx),
	}, nil
}

// Start resolves the two members once. Resolving them per connection would put
// a map lookup on the hot path for a pair that cannot change while the engine
// is up.
func (s *smartOutbound) Start() error {
	for _, tag := range s.tags {
		detour, loaded := s.outbound.Outbound(tag)
		if !loaded {
			return E.New("smart: outbound not found: ", tag)
		}
		switch tag {
		case "direct":
			s.direct = detour
		case "proxy":
			s.proxy = detour
		}
	}
	if s.direct == nil || s.proxy == nil {
		return E.New("smart: needs both a direct and a proxy member, got ", s.tags)
	}
	return nil
}

func (s *smartOutbound) Now() string   { return "direct" }
func (s *smartOutbound) All() []string { return s.tags }

// member maps a decision onto the outbound that carries it out. chooseRace has
// no member of its own; until the race exists it behaves as the pre-feature
// default did.
func (s *smartOutbound) member(c smartChoice) adapter.Outbound {
	if c == chooseProxy {
		return s.proxy
	}
	return s.direct
}

// decide answers for one connection. Racing is switched off here and turned on
// by the task that implements it.
func (s *smartOutbound) decide(metadata *adapter.InboundContext) smartChoice {
	return decideSmart(s.store, smartHost(metadata), metadata.Destination.Addr, false)
}

// smartHost is the name this connection is for, or "" when there is none.
// FakeIP puts the name in Destination.Fqdn before any rule is matched (fork
// route/route.go:422); the sniffer, when it ran, puts it in Domain.
func smartHost(metadata *adapter.InboundContext) string {
	if metadata.Domain != "" {
		return metadata.Domain
	}
	return metadata.Destination.Fqdn
}

func (s *smartOutbound) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	choice := chooseDirect
	if metadata := adapter.ContextFrom(ctx); metadata != nil {
		choice = s.decide(metadata)
	}
	return s.member(choice).DialContext(ctx, network, destination)
}

func (s *smartOutbound) ListenPacket(ctx context.Context, destination M.Socksaddr) (net.PacketConn, error) {
	choice := chooseDirect
	if metadata := adapter.ContextFrom(ctx); metadata != nil {
		choice = s.decide(metadata)
	}
	return s.member(choice).ListenPacket(ctx, destination)
}

func (s *smartOutbound) NewConnectionEx(ctx context.Context, conn net.Conn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	chosen := s.member(s.decide(&metadata))
	if handler, isHandler := chosen.(adapter.ConnectionHandlerEx); isHandler {
		handler.NewConnectionEx(ctx, conn, metadata, onClose)
		return
	}
	s.connection.NewConnection(ctx, chosen, conn, metadata, onClose)
}

func (s *smartOutbound) NewPacketConnectionEx(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	chosen := s.member(s.decide(&metadata))
	if handler, isHandler := chosen.(adapter.PacketConnectionHandlerEx); isHandler {
		handler.NewPacketConnectionEx(ctx, conn, metadata, onClose)
		return
	}
	s.connection.NewPacketConnection(ctx, chosen, conn, metadata, onClose)
}

// extendedBoxContext is include.Context plus our own outbound type.
//
// It has to be spelled out rather than wrapped: include.Context builds all six
// registries and hands them to box.Context in one call
// (include/registry.go:60-62), so there is no seam to slip a type into
// afterwards.
func extendedBoxContext(ctx context.Context) context.Context {
	outbounds := include.OutboundRegistry()
	outbound.Register[smartOutboundOptions](outbounds, smartOutboundTag, newSmartOutbound)
	return box.Context(
		ctx,
		include.InboundRegistry(),
		outbounds,
		include.EndpointRegistry(),
		include.ProviderRegistry(),
		include.DNSTransportRegistry(),
		include.ServiceRegistry(),
	)
}
