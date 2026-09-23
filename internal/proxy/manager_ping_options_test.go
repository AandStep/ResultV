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
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	M "github.com/sagernet/sing/common/metadata"

	"resultproxy-wails/internal/config"
)

func TestPingHTTPTypeUsesNodeProbe(t *testing.T) {
	old := pingThroughNodeProbe
	defer func() { pingThroughNodeProbe = old }()

	var gotMethod, gotURL string
	pingThroughNodeProbe = func(_ context.Context, _ ProxyConfig, method, testURL, _ string) (int64, bool, string) {
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

type wireGuardDialStub struct {
	stubEngine
	addr  string
	dials atomic.Int32
}

func (s *wireGuardDialStub) DialWireGuard(ctx context.Context, _ string, _ M.Socksaddr) (net.Conn, error) {
	s.dials.Add(1)
	var d net.Dialer
	return d.DialContext(ctx, "tcp", s.addr)
}

func liveWireGuardManager(t *testing.T, node ProxyConfig) *Manager {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	live := node
	return &Manager{connected: true, proxy: &live, engine: &wireGuardDialStub{addr: srv.Listener.Addr().String()}}
}

func stubWireGuardHandshake(t *testing.T, fn func(context.Context, ProxyConfig, string) (int64, bool, string)) {
	old := pingWireGuardHandshakeProbe
	t.Cleanup(func() { pingWireGuardHandshakeProbe = old })
	pingWireGuardHandshakeProbe = fn
}

func stubICMP(t *testing.T, ms int64, ok bool) {
	old := pingICMPProbe
	t.Cleanup(func() { pingICMPProbe = old })
	pingICMPProbe = func(_, _ string, _ time.Duration) (int64, bool) { return ms, ok }
}

func TestPingHTTPTypeLiveWireGuardNodeGoesThroughSession(t *testing.T) {
	node := ProxyConfig{ID: "n1", IP: "1.2.3.4", Port: 51820, Type: "AMNEZIAWG"}
	m := liveWireGuardManager(t, node)
	stubWireGuardHandshake(t, func(context.Context, ProxyConfig, string) (int64, bool, string) {
		t.Fatal("the live node must not be handshaken by a probe")
		return 0, false, ""
	})
	oldEngine := pingThroughNodeProbe
	t.Cleanup(func() { pingThroughNodeProbe = oldEngine })
	pingThroughNodeProbe = func(context.Context, ProxyConfig, string, string, string) (int64, bool, string) {
		t.Fatal("the live node must not get a probe engine")
		return 0, false, ""
	}
	res := m.Ping("1.2.3.4", 51820, "AMNEZIAWG", node, PingOptions{
		Type:    config.PingTypeHTTPGet,
		URL:     "https://127.0.0.1/generate_204",
		Timeout: 3 * time.Second,
	})
	if res.CheckType != "http_get" || m.engine.(*wireGuardDialStub).dials.Load() == 0 {
		t.Fatalf("got %+v", res)
	}
}

func TestPingAutoWireGuardHandshakesWhenICMPBlocked(t *testing.T) {
	stubICMP(t, 0, false)
	var probed ProxyConfig
	stubWireGuardHandshake(t, func(_ context.Context, node ProxyConfig, _ string) (int64, bool, string) {
		probed = node
		return 42, true, ""
	})
	node := ProxyConfig{ID: "n1", IP: "1.2.3.4", Port: 51820, Type: "AMNEZIAWG"}
	res := (&Manager{}).Ping("1.2.3.4", 51820, "AMNEZIAWG", node, PingOptions{Type: config.PingTypeAuto, Timeout: 3 * time.Second})
	if !res.Reachable || res.LatencyMs != 42 || res.CheckType != "handshake" || probed.ID != "n1" {
		t.Fatalf("got %+v, probed %+v", res, probed)
	}
}

func TestPingAutoWireGuardPrefersICMP(t *testing.T) {
	stubICMP(t, 17, true)
	stubWireGuardHandshake(t, func(context.Context, ProxyConfig, string) (int64, bool, string) {
		t.Fatal("ICMP answered, no handshake needed")
		return 0, false, ""
	})
	node := ProxyConfig{ID: "n1", IP: "1.2.3.4", Port: 51820, Type: "WIREGUARD"}
	res := (&Manager{}).Ping("1.2.3.4", 51820, "WIREGUARD", node, PingOptions{Type: config.PingTypeAuto, Timeout: 3 * time.Second})
	if !res.Reachable || res.LatencyMs != 17 || res.CheckType != "icmp" {
		t.Fatalf("got %+v", res)
	}
}

func TestPingAutoLiveWireGuardNodeIsNotHandshaken(t *testing.T) {
	stubICMP(t, 0, false)
	stubWireGuardHandshake(t, func(context.Context, ProxyConfig, string) (int64, bool, string) {
		t.Fatal("the live node must not be handshaken by a probe")
		return 0, false, ""
	})
	node := ProxyConfig{ID: "n1", IP: "1.2.3.4", Port: 51820, Type: "AMNEZIAWG"}
	m := liveWireGuardManager(t, node)
	res := m.Ping("1.2.3.4", 51820, "AMNEZIAWG", node, PingOptions{
		Type: config.PingTypeAuto, URL: "http://127.0.0.1/generate_204", Timeout: 3 * time.Second,
	})
	if !res.Reachable || res.CheckType != "tunnel_http" {
		t.Fatalf("got %+v", res)
	}
}

func TestPingAutoWireGuardHandshakeTimeoutIsReported(t *testing.T) {
	stubICMP(t, 0, false)
	stubWireGuardHandshake(t, func(ctx context.Context, _ ProxyConfig, _ string) (int64, bool, string) {
		<-ctx.Done()
		return 0, false, "timeout"
	})
	node := ProxyConfig{ID: "n1", IP: "1.2.3.4", Port: 51820, Type: "AMNEZIAWG"}
	start := time.Now()
	res := (&Manager{}).Ping("1.2.3.4", 51820, "AMNEZIAWG", node, PingOptions{Type: config.PingTypeAuto, Timeout: time.Second})
	if res.Reachable || res.Reason != "timeout" || time.Since(start) > 1500*time.Millisecond {
		t.Fatalf("got %+v after %v", res, time.Since(start))
	}
}

func TestUAPIHandshakeTime(t *testing.T) {
	dump := "private_key=aa\nlisten_port=1\npublic_key=bb\nlast_handshake_time_sec=0\nlast_handshake_time_nsec=0\n"
	if _, ok := uapiHandshakeTime(dump); ok {
		t.Fatal("no handshake yet")
	}
	if uapiValue(dump, "public_key") != "bb" {
		t.Fatal("peer key")
	}
	got, ok := uapiHandshakeTime("last_handshake_time_sec=1700000000\nlast_handshake_time_nsec=500\n")
	if !ok || !got.Equal(time.Unix(1700000000, 500)) {
		t.Fatalf("got %v %v", got, ok)
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
	pingThroughNodeProbe = func(_ context.Context, _ ProxyConfig, _, _, _ string) (int64, bool, string) {
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

func TestPingAutoWireGuardProbeGetsBudgetWithinTimeout(t *testing.T) {
	old := pingWireGuardProbe
	defer func() { pingWireGuardProbe = old }()
	var got time.Duration
	pingWireGuardProbe = func(_ string, _ int, budget time.Duration) (int64, bool, string) {
		got = budget
		return -1, true, ""
	}
	m := &Manager{}
	res := m.Ping("1.2.3.4", 51820, "AMNEZIAWG", ProxyConfig{}, PingOptions{Type: config.PingTypeAuto, Timeout: 3 * time.Second})
	if !res.Reachable {
		t.Fatalf("got %+v", res)
	}
	if got <= 0 || got > 3*time.Second {
		t.Fatalf("budget %v", got)
	}
}

func TestWireGuardProbeBudgetsFitTotal(t *testing.T) {
	for _, total := range []time.Duration{time.Second, 3 * time.Second, 10 * time.Second} {
		icmp, udp := wireGuardProbeBudgets(total)
		if icmp <= 0 || udp <= 0 || icmp+udp+pingBudgetMargin > total {
			t.Fatalf("total %v: icmp %v udp %v", total, icmp, udp)
		}
	}
	icmp, udp := wireGuardProbeBudgets(wireGuardProbeDefaultBudget)
	if icmp != 2*time.Second || udp != time.Second {
		t.Fatalf("default: icmp %v udp %v", icmp, udp)
	}
}

// A host that answers neither ICMP nor UDP — the AmneziaWG server that
// reported a timeout on every auto ping — must get its verdict inside the budget.
func TestPingWireGuardSilentHostFinishesWithinBudget(t *testing.T) {
	old := pingICMPProbe
	defer func() { pingICMPProbe = old }()
	pingICMPProbe = func(_, _ string, timeout time.Duration) (int64, bool) {
		time.Sleep(timeout)
		return 0, false
	}
	start := time.Now()
	_, ok, reason := PingWireGuard("192.0.2.1", 51820, time.Second)
	if elapsed := time.Since(start); elapsed >= time.Second {
		t.Fatalf("took %v", elapsed)
	}
	if !ok {
		t.Fatalf("reason=%q", reason)
	}
}

func stubWireGuardLiveness(t *testing.T) *atomic.Int32 {
	var calls atomic.Int32
	old := pingWireGuardProbe
	t.Cleanup(func() { pingWireGuardProbe = old })
	pingWireGuardProbe = func(string, int, time.Duration) (int64, bool, string) {
		calls.Add(1)
		return -1, true, ""
	}
	return &calls
}

func TestPingConnectingWireGuardNodeIsNotHandshaken(t *testing.T) {
	stubICMP(t, 0, false)
	liveness := stubWireGuardLiveness(t)
	stubWireGuardHandshake(t, func(context.Context, ProxyConfig, string) (int64, bool, string) {
		t.Fatal("a node being connected must not be handshaken by a probe")
		return 0, false, ""
	})
	oldEngine := pingThroughNodeProbe
	t.Cleanup(func() { pingThroughNodeProbe = oldEngine })
	pingThroughNodeProbe = func(context.Context, ProxyConfig, string, string, string) (int64, bool, string) {
		t.Fatal("a node being connected must not get a probe engine")
		return 0, false, ""
	}
	node := ProxyConfig{ID: "n1", IP: "1.2.3.4", Port: 51820, Type: "AMNEZIAWG"}
	m := &Manager{}
	m.setPendingLocked(node, ProxyModeTunnel)
	for _, pingType := range []string{config.PingTypeAuto, config.PingTypeHTTPGet} {
		res := m.Ping("1.2.3.4", 51820, "AMNEZIAWG", node, PingOptions{
			Type: pingType, URL: "https://example.com/x", Timeout: 3 * time.Second,
		})
		if !res.Reachable || res.LatencyMs != -1 {
			t.Fatalf("%s: got %+v", pingType, res)
		}
	}
	if liveness.Load() != 2 {
		t.Fatalf("liveness probe calls %d", liveness.Load())
	}
}

func TestConnectCancelsHandshakeProbeOfSameNode(t *testing.T) {
	stubICMP(t, 0, false)
	node := ProxyConfig{ID: "n1", IP: "1.2.3.4", Port: 51820, Type: "AMNEZIAWG"}
	m := &Manager{}
	started := make(chan struct{})
	stubWireGuardHandshake(t, func(ctx context.Context, _ ProxyConfig, _ string) (int64, bool, string) {
		close(started)
		<-ctx.Done()
		return 0, false, "cancelled"
	})
	result := make(chan PingResultDTO, 1)
	go func() {
		result <- m.Ping("1.2.3.4", 51820, "AMNEZIAWG", node, PingOptions{Type: config.PingTypeAuto, Timeout: 10 * time.Second})
	}()
	<-started
	m.mu.Lock()
	m.setPendingLocked(node, ProxyModeTunnel)
	m.mu.Unlock()
	select {
	case res := <-result:
		if res.Reachable {
			t.Fatalf("got %+v", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("connect did not cancel the probe")
	}
}

func TestPingAutoWireGuardBudgetStartsAfterQueue(t *testing.T) {
	stubICMP(t, 0, false)
	stubWireGuardHandshake(t, func(ctx context.Context, _ ProxyConfig, _ string) (int64, bool, string) {
		if ctx.Err() != nil {
			return 0, false, "timeout"
		}
		return 30, true, ""
	})
	for i := 0; i < pingEngineMaxConcurrency; i++ {
		pingEngineSem <- struct{}{}
	}
	go func() {
		time.Sleep(1500 * time.Millisecond)
		for i := 0; i < pingEngineMaxConcurrency; i++ {
			<-pingEngineSem
		}
	}()
	node := ProxyConfig{ID: "n1", IP: "1.2.3.4", Port: 51820, Type: "AMNEZIAWG"}
	res := (&Manager{}).Ping("1.2.3.4", 51820, "AMNEZIAWG", node, PingOptions{Type: config.PingTypeAuto, Timeout: time.Second})
	if !res.Reachable || res.CheckType != "handshake" {
		t.Fatalf("a node that waited its turn got %+v", res)
	}
}
