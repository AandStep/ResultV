package proxy

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/logger"
)

func TestPingHTTPTypeUsesNodeProbe(t *testing.T) {
	old := pingThroughNodeProbe
	defer func() { pingThroughNodeProbe = old }()

	var gotMethod, gotURL string
	pingThroughNodeProbe = func(_ context.Context, _ ProxyConfig, method, testURL, _ string, _ *logger.Logger) (int64, bool, string) {
		gotMethod, gotURL = method, testURL
		return 42, true, ""
	}

	m := &Manager{}
	node := ProxyConfig{ID: "n1", IP: "1.2.3.4", Port: 443, Type: "VLESS"}
	res := m.Ping("1.2.3.4", 443, "VLESS", node, PingOptions{
		Type:    config.PingTypeHTTPHead,
		URL:     "https://example.com/x",
		Timeout: 3 * time.Second,
	})

	if !res.Reachable || res.LatencyMs != 42 {
		t.Fatalf("got %+v", res)
	}
	if res.CheckType != "http_head" {
		t.Fatalf("checkType %q", res.CheckType)
	}
	if gotMethod != "HEAD" || gotURL != "https://example.com/x" {
		t.Fatalf("probe got method=%q url=%q", gotMethod, gotURL)
	}
}

func TestPingHTTPTypeRejectsWireGuard(t *testing.T) {
	m := &Manager{}
	node := ProxyConfig{ID: "n1", IP: "1.2.3.4", Port: 51820, Type: "WIREGUARD"}
	res := m.Ping("1.2.3.4", 51820, "WIREGUARD", node, PingOptions{
		Type:    config.PingTypeHTTPGet,
		URL:     "https://example.com/x",
		Timeout: 3 * time.Second,
	})
	if res.Reachable || res.Reason != "unsupported_for_protocol" {
		t.Fatalf("got %+v", res)
	}
}

func TestPingHTTPTypeReportsMissingNode(t *testing.T) {
	// App could not find the entry by id — the list was rewritten between the
	// sweep and this call. The HTTP types cannot build an outbound without it,
	// and must say so instead of reporting a dead node.
	m := &Manager{}
	res := m.Ping("1.2.3.4", 443, "VLESS", ProxyConfig{}, PingOptions{
		Type:    config.PingTypeHTTPGet,
		URL:     "https://example.com/x",
		Timeout: 3 * time.Second,
	})
	if res.Reachable || res.Reason != "node_not_found" {
		t.Fatalf("got %+v", res)
	}
}

func TestPingHTTPTypeRejectsPlainHTTPURL(t *testing.T) {
	m := &Manager{}
	node := ProxyConfig{ID: "n1", IP: "1.2.3.4", Port: 443, Type: "VLESS"}
	res := m.Ping("1.2.3.4", 443, "VLESS", node, PingOptions{
		Type:    config.PingTypeHTTPGet,
		URL:     "http://example.com/x",
		Timeout: 3 * time.Second,
	})
	if res.Reachable || res.Reason != "bad_test_url" {
		t.Fatalf("got %+v", res)
	}
}

func TestPingICMPTypeDoesNotFallBack(t *testing.T) {
	old := pingICMPProbe
	defer func() { pingICMPProbe = old }()
	pingICMPProbe = func(_, _ string, _ time.Duration) (int64, bool) { return 0, false }

	m := &Manager{}
	node := ProxyConfig{ID: "n1", IP: "1.2.3.4", Port: 443, Type: "VLESS"}
	res := m.Ping("1.2.3.4", 443, "VLESS", node, PingOptions{
		Type:    config.PingTypeICMP,
		Timeout: 2 * time.Second,
	})
	// The user asked for ICMP specifically. Silently answering with a TCP
	// number would misreport what was measured.
	if res.Reachable || res.Reason != "icmp_unavailable" {
		t.Fatalf("got %+v", res)
	}
	if res.CheckType != "icmp" {
		t.Fatalf("checkType %q", res.CheckType)
	}
}

func TestPingAutoTypeUnchanged(t *testing.T) {
	oldTCP := pingTCPProbe
	defer func() { pingTCPProbe = oldTCP }()
	pingTCPProbe = func(_ string, _ int) (int64, bool, string) { return 17, true, "" }

	m := &Manager{}
	node := ProxyConfig{ID: "n1", IP: "1.2.3.4", Port: 443, Type: "VLESS"}
	res := m.Ping("1.2.3.4", 443, "VLESS", node, PingOptions{
		Type:    config.PingTypeAuto,
		Timeout: 5 * time.Second,
	})
	if !res.Reachable || res.LatencyMs != 17 || res.CheckType != "tcp" {
		t.Fatalf("auto must keep today's behaviour, got %+v", res)
	}
}

func TestPingOuterDeadlineFiresWithoutWaitingForProbe(t *testing.T) {
	oldTCP := pingTCPProbe
	defer func() { pingTCPProbe = oldTCP }()
	release := make(chan struct{})
	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		<-release
		return 1, true, ""
	}
	defer close(release)

	m := &Manager{}
	node := ProxyConfig{ID: "n1", IP: "1.2.3.4", Port: 443, Type: "VLESS"}
	start := time.Now()
	res := m.Ping("1.2.3.4", 443, "VLESS", node, PingOptions{
		Type:    config.PingTypeAuto,
		Timeout: 300 * time.Millisecond,
	})
	elapsed := time.Since(start)

	if res.Reachable || res.Reason != "timeout" {
		t.Fatalf("got %+v", res)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("deadline did not fire: waited %v", elapsed)
	}
}

func TestPingHTTPConcurrencyIsCapped(t *testing.T) {
	old := pingThroughNodeProbe
	defer func() { pingThroughNodeProbe = old }()

	var inFlight, peak int64
	var mu sync.Mutex
	pingThroughNodeProbe = func(_ context.Context, _ ProxyConfig, _, _, _ string, _ *logger.Logger) (int64, bool, string) {
		n := atomic.AddInt64(&inFlight, 1)
		mu.Lock()
		if n > peak {
			peak = n
		}
		mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt64(&inFlight, -1)
		return 1, true, ""
	}

	m := &Manager{}
	node := ProxyConfig{ID: "n1", IP: "1.2.3.4", Port: 443, Type: "VLESS"}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.Ping("1.2.3.4", 443, "VLESS", node, PingOptions{
				Type:    config.PingTypeHTTPGet,
				URL:     "https://example.com/x",
				Timeout: 5 * time.Second,
			})
		}()
	}
	wg.Wait()

	mu.Lock()
	got := peak
	mu.Unlock()
	if got > pingEngineMaxConcurrency {
		t.Fatalf("peak concurrency %d exceeds cap %d", got, pingEngineMaxConcurrency)
	}
}

func TestPingICMPVerdictBeatsTheOuterDeadline(t *testing.T) {
	old := pingICMPProbe
	defer func() { pingICMPProbe = old }()
	// A host that blocks ICMP consumes the whole budget it was handed and then
	// reports "no answer" — that is the common case, not an exotic one. Its
	// verdict has to reach the user: "this host does not answer echo" and "we
	// gave up waiting" are different facts, and the UI prints only the latter
	// as "Timeout".
	pingICMPProbe = func(_, _ string, budget time.Duration) (int64, bool) {
		time.Sleep(budget)
		return 0, false
	}

	m := &Manager{}
	res := m.Ping("1.2.3.4", 443, "VLESS", ProxyConfig{}, PingOptions{
		Type:    config.PingTypeICMP,
		Timeout: 1 * time.Second,
	})
	if res.Reason != "icmp_unavailable" {
		t.Fatalf("the ICMP verdict must beat the outer deadline, got %+v", res)
	}
}

func TestPingICMPResolveCannotEatTheProbeBudget(t *testing.T) {
	oldLookup := pingLookupIPAddr
	oldDoH := pingDoHResolve
	oldICMP := pingICMPProbe
	defer func() {
		pingLookupIPAddr = oldLookup
		pingDoHResolve = oldDoH
		pingICMPProbe = oldICMP
	}()

	// A resolver that never answers must not spend the probe's share of the
	// budget: worst case today is a 2s OS lookup plus 1.5s per DoH endpoint,
	// which alone overruns any timeout the user can choose.
	pingLookupIPAddr = func(ctx context.Context, _ string) ([]net.IPAddr, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	pingDoHResolve = func(string) []string { time.Sleep(2 * time.Second); return nil }

	probed := make(chan struct{}, 1)
	pingICMPProbe = func(_, _ string, _ time.Duration) (int64, bool) {
		probed <- struct{}{}
		return 1, true
	}

	m := &Manager{}
	start := time.Now()
	res := m.Ping("vpn.example.invalid", 443, "VLESS", ProxyConfig{}, PingOptions{
		Type:    config.PingTypeICMP,
		Timeout: 1 * time.Second,
	})
	elapsed := time.Since(start)

	if res.Reason != "dns_unresolved" {
		t.Fatalf("an unresolvable host must be reported as such, got %+v", res)
	}
	if elapsed > 900*time.Millisecond {
		t.Fatalf("resolve was allowed to spend the whole budget: %v", elapsed)
	}
	select {
	case <-probed:
		t.Fatal("nothing to ping: the probe must not run without an address")
	default:
	}
}
