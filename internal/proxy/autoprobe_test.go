package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"resultproxy-wails/internal/config"
)

func TestProbeAutoNodes_FastReturnsResultPerNodeInInputOrder(t *testing.T) {
	old := pingTCPProbe
	defer func() { pingTCPProbe = old }()

	pingTCPProbe = func(ip string, _ int) (int64, bool, string) {
		switch ip {
		case "1.1.1.1":
			return 40, true, ""
		case "2.2.2.2":
			return 0, false, "timeout"
		}
		return 90, true, ""
	}

	nodes := []config.ProxyEntry{
		{ID: "a", IP: "1.1.1.1", Port: 443, Type: "VLESS"},
		{ID: "b", IP: "2.2.2.2", Port: 443, Type: "TROJAN"},
		{ID: "c", IP: "3.3.3.3", Port: 443, Type: "VLESS"},
	}

	got := ProbeAutoNodes(context.Background(), nodes, DepthFast)

	if len(got) != 3 {
		t.Fatalf("ожидали 3 результата, получили %d", len(got))
	}
	if got[0].RTTms != 40 || !got[0].OK {
		t.Errorf("узел a: ожидали 40ms/OK, получили %+v", got[0])
	}
	if got[1].OK || got[1].Reason != "timeout" {
		t.Errorf("узел b: ожидали отказ с timeout, получили %+v", got[1])
	}
	if got[2].RTTms != 90 {
		t.Errorf("узел c: ожидали 90ms, получили %+v", got[2])
	}
	if got[0].Stage != "tcp" {
		t.Errorf("ожидали stage=tcp, получили %q", got[0].Stage)
	}
}

func TestProbeAutoNodes_SkipsSectionAndAddresslessEntries(t *testing.T) {
	old := pingTCPProbe
	defer func() { pingTCPProbe = old }()

	var calls int32
	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		atomic.AddInt32(&calls, 1)
		return 10, true, ""
	}

	nodes := []config.ProxyEntry{
		{ID: "s", Type: "SECTION", Name: "Когда глушат"},
		{ID: "z", IP: "", Port: 0, Type: "VLESS"},
		{ID: "ok", IP: "1.1.1.1", Port: 443, Type: "VLESS"},
	}

	got := ProbeAutoNodes(context.Background(), nodes, DepthFast)

	if len(got) != 1 || got[0].Key == "" {
		t.Fatalf("ожидали один результат для единственного адресуемого узла, получили %+v", got)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("проба должна вызываться ровно один раз, вызвана %d раз", n)
	}
}

func TestProbeAutoNodes_UsesHysteria2ProbeForHysteria2(t *testing.T) {
	oldTCP, oldHY := pingTCPProbe, pingHysteria2Probe
	defer func() { pingTCPProbe, pingHysteria2Probe = oldTCP, oldHY }()

	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		t.Error("для HYSTERIA2 должна использоваться QUIC-проба, а не TCP")
		return 0, false, ""
	}
	pingHysteria2Probe = func(_ string, _ int) (int64, bool, string, string) {
		return 25, true, "", "quic_handshake"
	}

	got := ProbeAutoNodes(context.Background(),
		[]config.ProxyEntry{{ID: "h", IP: "1.1.1.1", Port: 443, Type: "HYSTERIA2"}},
		DepthFast)

	if len(got) != 1 || got[0].RTTms != 25 || got[0].Stage != "quic_handshake" {
		t.Fatalf("ожидали 25ms через quic_handshake, получили %+v", got)
	}
}

// sampleProxyEntry is a fully-populated entry used as the baseline for
// AutoNodeKey tests below. Tests mutate a copy of exactly one field and check
// that the key does (or does not) change, so every field that isn't the one
// under test must be pre-populated and non-empty — an empty field could hide
// a bug where that field is silently ignored.
func sampleProxyEntry() config.ProxyEntry {
	return config.ProxyEntry{
		ID:              "orig-id",
		IP:              "1.2.3.4",
		Port:            443,
		Type:            "VLESS",
		Username:        "user-a",
		Password:        "pass-a",
		SubscriptionURL: "https://example.com/sub/Aa",
		Extra:           json.RawMessage(`{"uuid":"11111111-1111-1111-1111-111111111111"}`),
	}
}

// TestAutoNodeKey_DiffersOnExtra is the regression test for the finding:
// AutoNodeKey used to hash only subscription|ip|port|type, so two nodes on the
// same CDN-fronted host:port that differ only by UUID/SNI/path (carried in
// Extra) collided onto one key. frontend/src/utils/proxyParser.js:657 already
// documents hitting this exact collision from the other side.
func TestAutoNodeKey_DiffersOnExtra(t *testing.T) {
	a := sampleProxyEntry()
	b := sampleProxyEntry()
	b.Extra = json.RawMessage(`{"uuid":"22222222-2222-2222-2222-222222222222"}`)

	if AutoNodeKey(a) == AutoNodeKey(b) {
		t.Fatal("entries differing only in Extra (uuid) produced the same key — CDN/multi-account collision is not fixed")
	}
}

func TestAutoNodeKey_DiffersOnUsername(t *testing.T) {
	a := sampleProxyEntry()
	b := sampleProxyEntry()
	b.Username = "user-b"

	if AutoNodeKey(a) == AutoNodeKey(b) {
		t.Fatal("entries differing only in Username produced the same key")
	}
}

func TestAutoNodeKey_DiffersOnPassword(t *testing.T) {
	a := sampleProxyEntry()
	b := sampleProxyEntry()
	b.Password = "pass-b"

	if AutoNodeKey(a) == AutoNodeKey(b) {
		t.Fatal("entries differing only in Password produced the same key")
	}
}

// TestAutoNodeKey_StableAcrossCallsAndIgnoresID checks both halves of the
// contract that makes AutoNodeKey useful as a persistent key: repeated calls
// on an unchanged entry must agree with each other, and ID churn (the backend
// reassigns ProxyEntry.ID on every subscription fetch, app.go) must not.
func TestAutoNodeKey_StableAcrossCallsAndIgnoresID(t *testing.T) {
	a := sampleProxyEntry()

	k1 := AutoNodeKey(a)
	k2 := AutoNodeKey(a)
	if k1 != k2 {
		t.Fatalf("AutoNodeKey is not stable across repeated calls on the same entry: %q vs %q", k1, k2)
	}

	b := sampleProxyEntry()
	b.ID = "a-different-id-assigned-on-refresh"
	if AutoNodeKey(a) != AutoNodeKey(b) {
		t.Fatal("entries differing only in ID produced different keys — the key must survive ID churn on refresh")
	}
}

// TestAutoNodeKey_HostCaseAndWhitespaceIgnored_PathCaseSignificant covers the
// asymmetric normalization documented on AutoNodeKey: host and protocol are
// case-insensitive (DNS/scheme convention), but the subscription URL is only
// trimmed, never lowercased, because URL paths are case-sensitive per RFC 3986.
func TestAutoNodeKey_HostCaseAndWhitespaceIgnored_PathCaseSignificant(t *testing.T) {
	a := sampleProxyEntry()
	a.IP = "Example.COM"
	a.Type = " vless "

	b := sampleProxyEntry()
	b.IP = "  example.com  "
	b.Type = "VLESS"

	if AutoNodeKey(a) != AutoNodeKey(b) {
		t.Fatal("host case/whitespace and type case/whitespace should not change the key")
	}

	c := sampleProxyEntry()
	c.SubscriptionURL = "https://example.com/sub/AA" // path segment differs only in case from "Aa"
	if AutoNodeKey(sampleProxyEntry()) == AutoNodeKey(c) {
		t.Fatal("subscription URL path is case-sensitive per RFC 3986 and must change the key")
	}
}

func TestAutoProbeTLSParams_ReadsSNIAndALPNFromExtra(t *testing.T) {
	e := config.ProxyEntry{
		IP:    "1.2.3.4",
		Port:  443,
		Type:  "VLESS",
		Extra: []byte(`{"sni":"www.example.com","alpn":"h2,http/1.1","security":"reality"}`),
	}

	sni, alpn, wantTLS := autoProbeTLSParams(e)

	if !wantTLS {
		t.Fatal("reality-узел должен требовать TLS-этап")
	}
	if sni != "www.example.com" {
		t.Errorf("ожидали SNI из extra, получили %q", sni)
	}
	if len(alpn) != 2 || alpn[0] != "h2" {
		t.Errorf("ожидали ALPN [h2 http/1.1], получили %v", alpn)
	}
}

func TestAutoProbeTLSParams_FallsBackToHostAndSkipsPlainProtocols(t *testing.T) {
	tls := config.ProxyEntry{IP: "cdn.example.com", Port: 443, Type: "TROJAN"}
	sni, _, wantTLS := autoProbeTLSParams(tls)
	if !wantTLS || sni != "cdn.example.com" {
		t.Errorf("TROJAN без extra: ожидали TLS с SNI=host, получили sni=%q wantTLS=%v", sni, wantTLS)
	}

	ss := config.ProxyEntry{IP: "1.2.3.4", Port: 8388, Type: "SS"}
	if _, _, want := autoProbeTLSParams(ss); want {
		t.Error("SS не использует TLS — этап должен пропускаться")
	}
}

func TestProbeAutoNodes_FullFailsNodeWhenTLSHandshakeFails(t *testing.T) {
	oldTCP, oldTLS := pingTCPProbe, autoTLSProbe
	defer func() { pingTCPProbe, autoTLSProbe = oldTCP, oldTLS }()

	pingTCPProbe = func(_ string, _ int) (int64, bool, string) { return 20, true, "" }
	autoTLSProbe = func(_ string, _ int, _ string, _ []string) (int64, bool, string) {
		return 0, false, "connection_reset"
	}

	got := ProbeAutoNodes(context.Background(),
		[]config.ProxyEntry{{ID: "a", IP: "1.1.1.1", Port: 443, Type: "VLESS"}},
		DepthFull)

	if len(got) != 1 {
		t.Fatalf("ожидали 1 результат, получили %d", len(got))
	}
	if got[0].OK {
		t.Error("живой TCP при мёртвом TLS должен считаться отказом — это и есть SNI-блокировка")
	}
	if got[0].Stage != "tls" || got[0].Reason != "connection_reset" {
		t.Errorf("ожидали stage=tls reason=connection_reset, получили %+v", got[0])
	}
}

func TestProbeAutoNodes_FullTakesMedianAndJitterOfThreeSamples(t *testing.T) {
	oldTCP, oldTLS := pingTCPProbe, autoTLSProbe
	defer func() { pingTCPProbe, autoTLSProbe = oldTCP, oldTLS }()

	pingTCPProbe = func(_ string, _ int) (int64, bool, string) { return 5, true, "" }
	samples := []int64{30, 90, 50}
	var i int
	autoTLSProbe = func(_ string, _ int, _ string, _ []string) (int64, bool, string) {
		v := samples[i%len(samples)]
		i++
		return v, true, ""
	}

	got := ProbeAutoNodes(context.Background(),
		[]config.ProxyEntry{{ID: "a", IP: "1.1.1.1", Port: 443, Type: "VLESS"}},
		DepthFull)

	if got[0].RTTms != 50 {
		t.Errorf("ожидали медиану 50, получили %d", got[0].RTTms)
	}
	if got[0].JitterMs != 60 {
		t.Errorf("ожидали джиттер 90-30=60, получили %d", got[0].JitterMs)
	}
}

// TestProbeAutoNodes_BoundedPoolProbesEveryNodeOnceWithinConcurrencyLimit
// exercises the >16-node path that autoProbeConcurrency exists for. All other
// tests in this file use <=3 nodes, where pool == len(targets) and each worker
// claims exactly one item — the claim loop that hands out work to a pool
// smaller than the target count never runs a second lap. With 40 nodes and a
// pool of 16, the loop must run multiple laps, so this is the only test that
// can catch a broken index handoff (double-probe or skipped node) or a pool
// that isn't actually bounded.
func TestProbeAutoNodes_BoundedPoolProbesEveryNodeOnceWithinConcurrencyLimit(t *testing.T) {
	oldTCP := pingTCPProbe
	defer func() { pingTCPProbe = oldTCP }()

	const n = 40
	nodes := make([]config.ProxyEntry, n)
	ipToIndex := make(map[string]int, n)
	for i := 0; i < n; i++ {
		ip := fmt.Sprintf("10.0.0.%d", i+1)
		nodes[i] = config.ProxyEntry{ID: fmt.Sprintf("node-%d", i), IP: ip, Port: 443, Type: "VLESS"}
		ipToIndex[ip] = i
	}

	var callCounts [n]int32
	var inFlight, peak int32

	// ipToIndex is built once above and never written again, so concurrent
	// reads from worker goroutines are race-free.
	pingTCPProbe = func(ip string, _ int) (int64, bool, string) {
		idx, known := ipToIndex[ip]
		if !known {
			t.Errorf("probed unexpected ip %q", ip)
			return 0, false, "unexpected"
		}
		atomic.AddInt32(&callCounts[idx], 1)

		cur := atomic.AddInt32(&inFlight, 1)
		for {
			p := atomic.LoadInt32(&peak)
			if cur <= p {
				break
			}
			if atomic.CompareAndSwapInt32(&peak, p, cur) {
				break
			}
		}

		// Hold the slot briefly so overlapping in-flight probes are actually
		// observable instead of the mock returning before the next worker starts.
		time.Sleep(5 * time.Millisecond)

		atomic.AddInt32(&inFlight, -1)
		return 10, true, ""
	}

	got := ProbeAutoNodes(context.Background(), nodes, DepthFast)

	if len(got) != n {
		t.Fatalf("expected %d results, got %d", n, len(got))
	}

	for i, c := range callCounts {
		if c != 1 {
			t.Errorf("node %d probed %d times, want exactly 1", i, c)
		}
	}

	finalPeak := atomic.LoadInt32(&peak)
	if finalPeak > autoProbeConcurrency {
		t.Errorf("peak concurrent in-flight probes = %d, want <= autoProbeConcurrency (%d)", finalPeak, autoProbeConcurrency)
	}
	if finalPeak <= 1 {
		t.Errorf("peak concurrent in-flight probes = %d, want > 1 — a serial implementation would also pass at 1, proving nothing about the pool", finalPeak)
	}

	for i := range got {
		want := AutoNodeKey(nodes[i])
		if got[i].Key != want {
			t.Errorf("result[%d].Key = %q, want %q (results not in input order)", i, got[i].Key, want)
		}
	}
}
