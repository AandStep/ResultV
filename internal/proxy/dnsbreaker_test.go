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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mDNS "github.com/miekg/dns"
	"github.com/sagernet/sing-box/adapter"
	sbdns "github.com/sagernet/sing-box/dns"
)

type fakeDNSTransport struct {
	sbdns.TransportAdapter
	calls atomic.Int32
	mu    sync.Mutex
	err   error
	rcode int
}

func newFakeDNSTransport(tag string) *fakeDNSTransport {
	return &fakeDNSTransport{TransportAdapter: sbdns.NewTransportAdapter("fake", tag, nil)}
}

func (f *fakeDNSTransport) set(err error, rcode int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err, f.rcode = err, rcode
}

func (f *fakeDNSTransport) Start(adapter.StartStage) error { return nil }
func (f *fakeDNSTransport) Close() error                   { return nil }
func (f *fakeDNSTransport) Reset()                         {}
func (f *fakeDNSTransport) Exchange(_ context.Context, m *mDNS.Msg) (*mDNS.Msg, error) {
	f.calls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	resp := new(mDNS.Msg)
	resp.SetRcode(m, f.rcode)
	return resp, nil
}
func (f *fakeDNSTransport) ExchangeAsync(ctx context.Context, m *mDNS.Msg, cb func(*mDNS.Msg, error)) {
	cb(f.Exchange(ctx, m))
}

func dnsQuery() *mDNS.Msg {
	m := new(mDNS.Msg)
	m.SetQuestion("example.com.", mDNS.TypeA)
	return m
}

func TestDNSBreakerStopsWaitingForADeadPrimary(t *testing.T) {
	primary, backup := newFakeDNSTransport("cfg"), newFakeDNSTransport("backup")
	primary.set(errors.New("deadline exceeded"), 0)
	clock := time.Unix(1000, 0)
	breaker := newDNSBreaker("custom-1", primary, backup, func() time.Time { return clock })

	for i := 0; i < dnsBreakerTrips; i++ {
		if _, err := breaker.Exchange(context.Background(), dnsQuery()); err != nil {
			t.Fatalf("запасной должен был ответить: %v", err)
		}
	}
	before := primary.calls.Load()
	for i := 0; i < 10; i++ {
		breaker.Exchange(context.Background(), dnsQuery())
	}
	if extra := primary.calls.Load() - before; extra > 1 {
		t.Fatalf("после %d провалов мёртвый резолвер спросили ещё %d раз — каждое имя снова ждёт его", dnsBreakerTrips, extra)
	}
	if backup.calls.Load() < int32(dnsBreakerTrips+10) {
		t.Fatalf("запасной спрошен %d раз", backup.calls.Load())
	}
}

func TestDNSBreakerReturnsToARecoveredPrimary(t *testing.T) {
	primary, backup := newFakeDNSTransport("cfg"), newFakeDNSTransport("backup")
	primary.set(errors.New("deadline exceeded"), 0)
	clock := time.Unix(1000, 0)
	breaker := newDNSBreaker("custom-1", primary, backup, func() time.Time { return clock })
	for i := 0; i < dnsBreakerTrips; i++ {
		breaker.Exchange(context.Background(), dnsQuery())
	}

	primary.set(nil, mDNS.RcodeNameError)
	clock = clock.Add(dnsBreakerOpenFor + time.Second)
	breaker.Exchange(context.Background(), dnsQuery())

	backupBefore := backup.calls.Load()
	for i := 0; i < 5; i++ {
		resp, err := breaker.Exchange(context.Background(), dnsQuery())
		if err != nil || resp.Rcode != mDNS.RcodeNameError {
			t.Fatalf("ответ ожившего резолвера (NXDOMAIN) должен быть окончательным: %v %v", resp, err)
		}
	}
	if backup.calls.Load() != backupBefore {
		t.Fatal("ожившему резолверу из конфига не вернули запросы")
	}
}

func TestDNSBreakerTreatsServfailAsFailure(t *testing.T) {
	primary, backup := newFakeDNSTransport("cfg"), newFakeDNSTransport("backup")
	primary.set(nil, mDNS.RcodeServerFailure)
	breaker := newDNSBreaker("custom-1", primary, backup, time.Now)
	resp, err := breaker.Exchange(context.Background(), dnsQuery())
	if err != nil || resp.Rcode != mDNS.RcodeSuccess {
		t.Fatalf("на SERVFAIL должен ответить запасной: %v %v", resp, err)
	}
}

func TestDNSBreakerDoesNotBlamePrimaryForACancelledLookup(t *testing.T) {
	primary, backup := newFakeDNSTransport("cfg"), newFakeDNSTransport("backup")
	primary.set(context.Canceled, 0)
	breaker := newDNSBreaker("custom-1", primary, backup, time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < dnsBreakerTrips+1; i++ {
		breaker.Exchange(ctx, dnsQuery())
	}
	primary.set(nil, mDNS.RcodeSuccess)
	before := backup.calls.Load()
	breaker.Exchange(context.Background(), dnsQuery())
	if backup.calls.Load() != before {
		t.Fatal("отменённые вызывающим запросы засчитаны резолверу как провалы")
	}
}
