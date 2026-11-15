//go:build android_core_overlay

package mobile

import (
	"context"
	"errors"
	"testing"
	"time"

	mDNS "github.com/miekg/dns"
	"github.com/sagernet/sing-box/adapter"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/dns"
	"github.com/sagernet/sing-box/dns/transport/fallback"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/service"
)

type delayedDNS struct {
	dns.TransportAdapter
	started chan struct{}
	release chan struct{}
}

func (*delayedDNS) Start(adapter.StartStage, *adapter.Scope) error { return nil }
func (*delayedDNS) Reset()                                         {}

func (d *delayedDNS) Exchange(ctx context.Context, request *mDNS.Msg) (*mDNS.Msg, error) {
	close(d.started)
	select {
	case <-d.release:
		return new(mDNS.Msg).SetReply(request), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (d *delayedDNS) ExchangeAsync(ctx context.Context, request *mDNS.Msg, answer func(*mDNS.Msg, error)) {
	answer(d.Exchange(ctx, request))
}

// This test covers the built-in fallback transport that Android libbox uses for
// tunnel DNS. Run with the same patched dependency as the AAR build using
// `go run ./scripts/android-core-build go test -tags android_core_overlay ./mobile`.
func TestAndroidFallbackDNSDoesNotBlockPacketReader(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	upstream := &delayedDNS{
		TransportAdapter: dns.NewTransportAdapter("test-delayed", "upstream", nil),
		started:          make(chan struct{}),
		release:          make(chan struct{}),
	}
	registry := dns.NewTransportRegistry()
	fallback.RegisterTransport(registry)
	dns.RegisterTransport[struct{}](registry, upstream.Type(), func(context.Context, log.ContextLogger, string, struct{}) (adapter.DNSTransport, error) {
		return upstream, nil
	})
	manager := dns.NewTransportManager(registry, nil, "tunnel")
	ctx = service.ContextWith[adapter.DNSTransportManager](ctx, manager)
	if err := manager.Create(ctx, log.StdLogger(), upstream.Tag(), upstream.Type(), &struct{}{}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Create(ctx, log.StdLogger(), "tunnel", C.DNSTypeFallback, &option.FallbackDNSServerOptions{Servers: []string{upstream.Tag()}}); err != nil {
		t.Fatal(err)
	}

	transport, ok := manager.Transport("tunnel")
	if !ok {
		t.Fatal("fallback transport missing")
	}
	request := new(mDNS.Msg)
	request.SetQuestion("example.test.", mDNS.TypeA)
	returned := make(chan struct{})
	answered := make(chan error, 1)
	go func() {
		transport.ExchangeAsync(ctx, request, func(response *mDNS.Msg, err error) {
			if err == nil && (response == nil || response.Id != request.Id || !response.Response) {
				err = errors.New("unexpected DNS response")
			}
			answered <- err
		})
		close(returned)
	}()

	select {
	case <-upstream.started:
	case <-ctx.Done():
		t.Fatal("fallback did not query the upstream")
	}
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("fallback held the packet reader while waiting for DNS")
	}
	select {
	case <-answered:
		t.Fatal("DNS callback fired while the upstream was blocked")
	default:
	}
	close(upstream.release)
	select {
	case err := <-answered:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("fallback did not deliver the response")
	}
}
