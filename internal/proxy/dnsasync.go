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

	mDNS "github.com/miekg/dns"
	"github.com/sagernet/sing-box/adapter"
	sbdns "github.com/sagernet/sing-box/dns"
	"github.com/sagernet/sing-box/log"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/service"
)

const dnsAsyncType = "resultv-async"

type dnsAsyncOptions struct {
	Servers []string `json:"servers"`
}

// dnsAsync fronts a server whose ExchangeAsync answers on the caller's
// goroutine. The core calls ExchangeAsync for hijacked DNS from the TUN read
// loop, so such a server stalls every packet until it answers.
type dnsAsync struct {
	sbdns.TransportAdapter
	upstream adapter.DNSTransport
}

func registerDNSAsync(registry *sbdns.TransportRegistry) {
	sbdns.RegisterTransport[dnsAsyncOptions](registry, dnsAsyncType, newDNSAsyncTransport)
}

func newDNSAsyncTransport(ctx context.Context, _ log.ContextLogger, tag string, options dnsAsyncOptions) (adapter.DNSTransport, error) {
	if len(options.Servers) != 1 {
		return nil, E.New("async front needs exactly one server")
	}
	manager := service.FromContext[adapter.DNSTransportManager](ctx)
	if manager == nil {
		return nil, E.New("missing DNS transport manager")
	}
	upstream, loaded := manager.Transport(options.Servers[0])
	if !loaded {
		return nil, E.New("server ", options.Servers[0], " not found")
	}
	return newDNSAsync(tag, upstream), nil
}

func newDNSAsync(tag string, upstream adapter.DNSTransport) *dnsAsync {
	return &dnsAsync{
		TransportAdapter: sbdns.NewTransportAdapter(dnsAsyncType, tag, []string{upstream.Tag()}),
		upstream:         upstream,
	}
}

func (a *dnsAsync) Start(adapter.StartStage, *adapter.Scope) error { return nil }
func (a *dnsAsync) Reset()                                         {}

func (a *dnsAsync) Exchange(ctx context.Context, message *mDNS.Msg) (*mDNS.Msg, error) {
	return a.upstream.Exchange(ctx, message)
}

func (a *dnsAsync) ExchangeAsync(ctx context.Context, message *mDNS.Msg, callback func(response *mDNS.Msg, err error)) {
	go func() {
		callback(a.Exchange(ctx, message))
	}()
}
