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
	"sync"
	"time"

	mDNS "github.com/miekg/dns"
	"github.com/sagernet/sing-box/adapter"
	sbdns "github.com/sagernet/sing-box/dns"
	"github.com/sagernet/sing-box/log"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/service"
)

const dnsBreakerType = "resultv-breaker"

const (
	dnsBreakerTrips        = 2
	dnsBreakerOpenFor      = 60 * time.Second
	dnsBreakerProbeGap     = 30 * time.Second
	dnsBreakerProbeTimeout = 5 * time.Second
)

type dnsBreakerOptions struct {
	Servers []string `json:"servers"`
}

// dnsBreaker asks the configured resolvers first and the backup after them,
// and stops waiting for the configured ones once they fail dnsBreakerTrips
// times in a row: for dnsBreakerOpenFor lookups go straight to the backup,
// while a background probe every dnsBreakerProbeGap checks whether they are
// back.
type dnsBreaker struct {
	sbdns.TransportAdapter
	primary, backup adapter.DNSTransport
	now             func() time.Time

	mu        sync.Mutex
	failures  int
	openUntil time.Time
	nextProbe time.Time
	probing   bool
}

func registerDNSBreaker(registry *sbdns.TransportRegistry) {
	sbdns.RegisterTransport[dnsBreakerOptions](registry, dnsBreakerType, newDNSBreakerTransport)
}

func newDNSBreakerTransport(ctx context.Context, _ log.ContextLogger, tag string, options dnsBreakerOptions) (adapter.DNSTransport, error) {
	if len(options.Servers) != 2 {
		return nil, E.New("breaker needs a primary and a backup server")
	}
	manager := service.FromContext[adapter.DNSTransportManager](ctx)
	if manager == nil {
		return nil, E.New("missing DNS transport manager")
	}
	var members [2]adapter.DNSTransport
	for i, member := range options.Servers {
		transport, loaded := manager.Transport(member)
		if !loaded {
			return nil, E.New("server ", member, " not found")
		}
		members[i] = transport
	}
	breaker := newDNSBreaker(tag, members[0], members[1], time.Now)
	breaker.TransportAdapter = sbdns.NewTransportAdapter(dnsBreakerType, tag, options.Servers)
	return breaker, nil
}

func newDNSBreaker(tag string, primary, backup adapter.DNSTransport, now func() time.Time) *dnsBreaker {
	return &dnsBreaker{
		TransportAdapter: sbdns.NewTransportAdapter(dnsBreakerType, tag, []string{primary.Tag(), backup.Tag()}),
		primary:          primary,
		backup:           backup,
		now:              now,
	}
}

func (b *dnsBreaker) Start(adapter.StartStage) error { return nil }
func (b *dnsBreaker) Close() error                   { return nil }
func (b *dnsBreaker) Reset()                         {}

func (b *dnsBreaker) Exchange(ctx context.Context, message *mDNS.Msg) (*mDNS.Msg, error) {
	if b.skipPrimary(message) {
		return b.backup.Exchange(ctx, message)
	}
	response, err := dnsAnswered(b.primary.Exchange(ctx, message))
	if err == nil {
		b.record(true)
		return response, nil
	}
	if ctx.Err() != nil {
		return nil, err
	}
	b.record(false)
	return b.backup.Exchange(ctx, message)
}

func (b *dnsBreaker) ExchangeAsync(ctx context.Context, message *mDNS.Msg, callback func(response *mDNS.Msg, err error)) {
	callback(b.Exchange(ctx, message))
}

func (b *dnsBreaker) skipPrimary(message *mDNS.Msg) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	if !now.Before(b.openUntil) {
		return false
	}
	if !b.probing && !now.Before(b.nextProbe) {
		b.probing = true
		b.nextProbe = now.Add(dnsBreakerProbeGap)
		go b.probe(message.Copy())
	}
	return true
}

func (b *dnsBreaker) probe(message *mDNS.Msg) {
	ctx, cancel := context.WithTimeout(context.Background(), dnsBreakerProbeTimeout)
	defer cancel()
	_, err := dnsAnswered(b.primary.Exchange(ctx, message))
	b.mu.Lock()
	defer b.mu.Unlock()
	b.probing = false
	if err == nil {
		b.failures = 0
		b.openUntil = time.Time{}
	}
}

func (b *dnsBreaker) record(ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ok {
		b.failures = 0
		b.openUntil = time.Time{}
		return
	}
	b.failures++
	if b.failures >= dnsBreakerTrips {
		now := b.now()
		b.openUntil = now.Add(dnsBreakerOpenFor)
		b.nextProbe = now.Add(dnsBreakerProbeGap)
	}
}

func dnsAnswered(response *mDNS.Msg, err error) (*mDNS.Msg, error) {
	if err != nil {
		return nil, err
	}
	if response.Rcode != mDNS.RcodeSuccess && response.Rcode != mDNS.RcodeNameError {
		return nil, E.New("bad response rcode: ", mDNS.RcodeToString[response.Rcode])
	}
	return response, nil
}
