package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"resultproxy-wails/internal/config"
)

func TestProbeAutoNodes_FastReturnsResultPerNodeInInputOrder(t *testing.T) {
	stubProbeResolver(t)
	oldBind := autoProbeBindsToLAN
	defer func() { autoProbeBindsToLAN = oldBind }()
	autoProbeBindsToLAN = func() bool { return false }

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
	stubProbeResolver(t)
	oldBind := autoProbeBindsToLAN
	defer func() { autoProbeBindsToLAN = oldBind }()
	autoProbeBindsToLAN = func() bool { return false }

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

// TestProbeAutoNodes_CancelledContextLeavesZeroValueSlotsWithEmptyKey
// documents the premise finding #5's context-cancellation item depends on:
// a probe slot the worker pool never reached because ctx was already
// cancelled is left at AutoProbeResult's zero value, whose Key is "". That
// zero-Key value is indistinguishable from "not yet a real result" by every
// caller (RankAutoCandidates in particular — see
// TestRankAutoCandidates_CancelledProbesAreNotRecordedUnderEmptyKey), so
// something downstream must filter it rather than record or display it.
func TestProbeAutoNodes_CancelledContextLeavesZeroValueSlotsWithEmptyKey(t *testing.T) {
	stubProbeResolver(t)
	oldBind := autoProbeBindsToLAN
	defer func() { autoProbeBindsToLAN = oldBind }()
	autoProbeBindsToLAN = func() bool { return false }

	old := pingTCPProbe
	defer func() { pingTCPProbe = old }()
	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		t.Error("проба не должна вызываться при уже отменённом контексте")
		return 0, false, ""
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got := ProbeAutoNodes(ctx, mkNodes("a", "b"), DepthFast)
	if len(got) != 2 {
		t.Fatalf("ожидали 2 (нулевых) слота, получили %d", len(got))
	}
	for _, r := range got {
		if r.Key != "" {
			t.Errorf("ожидали пустой Key у отменённого слота, получили %+v", r)
		}
	}
}

func TestProbeAutoNodes_UsesHysteria2ProbeForHysteria2(t *testing.T) {
	stubProbeResolver(t)
	oldTCP, oldHY := pingTCPProbe, pingHysteria2StrictProbe
	oldBind := autoProbeBindsToLAN
	defer func() {
		pingTCPProbe, pingHysteria2StrictProbe = oldTCP, oldHY
		autoProbeBindsToLAN = oldBind
	}()

	autoProbeBindsToLAN = func() bool { return false }
	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		t.Error("для HYSTERIA2 должна использоваться QUIC-проба, а не TCP")
		return 0, false, ""
	}
	pingHysteria2StrictProbe = func(_ string, _ int, _ string) (int64, bool, string, string) {
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

// TestAutoNodeKey_CanonicalizesExtraHTMLEscaping is the regression test for
// the finding: AutoNodeKey used to hash e.Extra's raw bytes, but Extra
// reaches the backend through two encoders that disagree on escaping — Go's
// encoding/json HTML-escapes &, <, > by default, JSON.stringify (the
// frontend, on every settings round-trip — see updateSetting) does not. A
// node whose ws/xhttp path contains "&" (e.g. "?ed=2048&v=1", a common
// early-data pattern) got one key right after connect and a DIFFERENT key
// the moment the config round-tripped through the frontend, silently
// restarting its history and breaking hysteresis.
func TestAutoNodeKey_CanonicalizesExtraHTMLEscaping(t *testing.T) {
	// htmlEscaped is what encoding/json (Go, used whenever THIS backend
	// writes Extra) actually produces for a path containing "&": it
	// HTML-escapes &, <, > by default. literalBytes is what JSON.stringify
	// (the frontend, on every settings round-trip — see updateSetting)
	// writes for the exact same logical value: no HTML-escaping at all.
	htmlEscaped, err := json.Marshal(map[string]string{"path": "/ws?a=1&b=2"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	literalBytes := json.RawMessage(`{"path":"/ws?a=1&b=2"}`)
	if string(htmlEscaped) == string(literalBytes) {
		t.Fatal("test setup invalid: encoding/json did not actually HTML-escape '&' here, so this test would prove nothing")
	}

	escaped := sampleProxyEntry()
	escaped.Extra = htmlEscaped

	literal := sampleProxyEntry()
	literal.Extra = literalBytes

	if AutoNodeKey(escaped) != AutoNodeKey(literal) {
		t.Fatal("HTML-escaped and literal encodings of the same logical Extra must hash to the same key")
	}

	// Guard against a canonicalization bug that just hashes the empty
	// string, or otherwise collapses every Extra onto one key: a genuinely
	// different value must still produce a different key.
	different := sampleProxyEntry()
	different.Extra = json.RawMessage(`{"path":"/ws?a=1&b=3"}`)
	if AutoNodeKey(escaped) == AutoNodeKey(different) {
		t.Fatal("genuinely different Extra values must still produce different keys after canonicalization")
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

// TestAutoProbeTLSParams_MirrorsEngineTLSCondition is the regression test for
// the finding: autoProbeTLSParams used to decide wantTLS from e.Type alone,
// firing a ClientHello at any non-excluded-protocol node regardless of
// whether its Extra actually asked for TLS. A plain vless://...?security=none
// node (parseVLESSURI defaults security to "none", never omits it) or a
// vmess:// node with tls!="tls" (parseVMessURI then writes no security key at
// all) would get probed anyway, time out at the TLS stage, and get deleted
// from AUTO selection while being penalised as if it had genuinely failed.
//
// Each case is checked against the exact condition applyTLSAndTransport uses
// (outbound.go:428-500) — the engine's own rule for whether a node's
// outbound gets a TLS record — so this test would catch autoProbeTLSParams
// drifting from that rule in either direction (probing a plain node, or
// skipping a TLS node).
func TestAutoProbeTLSParams_MirrorsEngineTLSCondition(t *testing.T) {
	cases := []struct {
		name    string
		e       config.ProxyEntry
		wantTLS bool
	}{
		{
			name:    "vless security=none has no TLS stage",
			e:       config.ProxyEntry{IP: "1.2.3.4", Port: 443, Type: "VLESS", Extra: []byte(`{"security":"none","network":"ws"}`)},
			wantTLS: false,
		},
		{
			name:    "vmess with no security key has no TLS stage",
			e:       config.ProxyEntry{IP: "1.2.3.4", Port: 443, Type: "VMESS", Extra: []byte(`{"network":"tcp","uuid":"x"}`)},
			wantTLS: false,
		},
		{
			name:    "socks5 has no TLS stage",
			e:       config.ProxyEntry{IP: "1.2.3.4", Port: 1080, Type: "SOCKS5"},
			wantTLS: false,
		},
		{
			name:    "http has no TLS stage",
			e:       config.ProxyEntry{IP: "1.2.3.4", Port: 8080, Type: "HTTP"},
			wantTLS: false,
		},
		{
			name:    "security=tls runs the TLS stage",
			e:       config.ProxyEntry{IP: "1.2.3.4", Port: 443, Type: "VLESS", Extra: []byte(`{"security":"tls"}`)},
			wantTLS: true,
		},
		{
			name:    "security=reality runs the TLS stage",
			e:       config.ProxyEntry{IP: "1.2.3.4", Port: 443, Type: "VLESS", Extra: []byte(`{"security":"reality"}`)},
			wantTLS: true,
		},
		{
			name:    "pbk present without security runs the TLS stage",
			e:       config.ProxyEntry{IP: "1.2.3.4", Port: 443, Type: "VLESS", Extra: []byte(`{"pbk":"REbCbLiQwWzmUHZgBc-oCO0CMtwgvWURtWkFjNfcQkk"}`)},
			wantTLS: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, got := autoProbeTLSParams(c.e)
			if got != c.wantTLS {
				t.Errorf("wantTLS = %v, хотели %v", got, c.wantTLS)
			}
		})
	}
}

func TestProbeAutoNodes_FullFailsNodeWhenTLSHandshakeFails(t *testing.T) {
	stubProbeResolver(t)
	oldBind := autoProbeBindsToLAN
	defer func() { autoProbeBindsToLAN = oldBind }()
	autoProbeBindsToLAN = func() bool { return false }

	oldTCP, oldTLS := pingTCPProbe, autoTLSProbe
	defer func() { pingTCPProbe, autoTLSProbe = oldTCP, oldTLS }()

	pingTCPProbe = func(_ string, _ int) (int64, bool, string) { return 20, true, "" }
	autoTLSProbe = func(_ string, _ int, _ string, _ []string) (int64, bool, string) {
		return 0, false, "connection_reset"
	}

	got := ProbeAutoNodes(context.Background(),
		// security=tls: wantTLS must be true for this node or DepthFull
		// would never reach the TLS stage this test is about (see
		// autoProbeWantsTLS — a VLESS node with no Extra at all no longer
		// implies TLS, that was finding #1's bug).
		[]config.ProxyEntry{{ID: "a", IP: "1.1.1.1", Port: 443, Type: "VLESS", Extra: []byte(`{"security":"tls"}`)}},
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
	stubProbeResolver(t)
	oldBind := autoProbeBindsToLAN
	defer func() { autoProbeBindsToLAN = oldBind }()
	autoProbeBindsToLAN = func() bool { return false }

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
		// security=tls — see the comment in
		// TestProbeAutoNodes_FullFailsNodeWhenTLSHandshakeFails above.
		[]config.ProxyEntry{{ID: "a", IP: "1.1.1.1", Port: 443, Type: "VLESS", Extra: []byte(`{"security":"tls"}`)}},
		DepthFull)

	if got[0].RTTms != 50 {
		t.Errorf("ожидали медиану 50, получили %d", got[0].RTTms)
	}
	if got[0].JitterMs != 60 {
		t.Errorf("ожидали джиттер 90-30=60, получили %d", got[0].JitterMs)
	}
}

// TestProbeAutoNodes_BoundedPoolProbesEveryNodeOnceWithinConcurrencyLimit
// exercises the path above autoProbeMaxConcurrency that the cap exists for.
// All other tests in this file use node counts at or below the cap, where
// pool == len(targets) and each worker claims exactly one item — the claim
// loop that hands out work to a pool smaller than the target count never
// runs a second lap, and finalPeak <= autoProbeMaxConcurrency holds no matter
// whether the cap is actually wired in. With 80 nodes and a pool capped at
// 64, the loop must run multiple laps and finalPeak can only stay <= 64 if
// the cap is genuinely enforced, so this is the only test that can catch a
// broken index handoff (double-probe or skipped node) or a pool that isn't
// actually bounded.
func TestProbeAutoNodes_BoundedPoolProbesEveryNodeOnceWithinConcurrencyLimit(t *testing.T) {
	stubProbeResolver(t)
	oldBind := autoProbeBindsToLAN
	defer func() { autoProbeBindsToLAN = oldBind }()
	autoProbeBindsToLAN = func() bool { return false }

	oldTCP := pingTCPProbe
	defer func() { pingTCPProbe = oldTCP }()

	const n = 80
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
	if finalPeak > autoProbeMaxConcurrency {
		t.Errorf("peak concurrent in-flight probes = %d, want <= autoProbeMaxConcurrency (%d)", finalPeak, autoProbeMaxConcurrency)
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

// Пробы стоят на таймаутах, а не на CPU: пул обязан покрывать весь список
// одной волной, пока список меньше потолка. При пуле 16 подписка на 48 узлов
// раскладывалась на три волны, и каждая оплачивала полный таймаут заново.
func TestProbeAutoNodes_PoolCoversAllNodesInOneWave(t *testing.T) {
	stubProbeResolver(t)
	oldTCP, oldBind := pingTCPProbe, autoProbeBindsToLAN
	defer func() { pingTCPProbe, autoProbeBindsToLAN = oldTCP, oldBind }()
	autoProbeBindsToLAN = func() bool { return false }

	const n = 48
	var inFlight, maxInFlight int32
	var mu sync.Mutex
	release := make(chan struct{})

	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		cur := atomic.AddInt32(&inFlight, 1)
		mu.Lock()
		if cur > maxInFlight {
			maxInFlight = cur
		}
		mu.Unlock()
		<-release
		atomic.AddInt32(&inFlight, -1)
		return 10, true, ""
	}

	names := make([]string, 0, n)
	for i := 0; i < n; i++ {
		names = append(names, fmt.Sprintf("node-%d", i))
	}

	done := make(chan []AutoProbeResult, 1)
	go func() { done <- ProbeAutoNodes(context.Background(), mkNodes(names...), DepthFast) }()

	deadline := time.After(5 * time.Second)
	for {
		if atomic.LoadInt32(&inFlight) >= n {
			break
		}
		select {
		case <-deadline:
			close(release)
			<-done
			t.Fatalf("одновременно в полёте было максимум %d проб из %d — пул не покрывает список одной волной",
				atomic.LoadInt32(&maxInFlight), n)
		default:
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(release)

	if got := <-done; len(got) != n {
		t.Fatalf("ожидали %d результатов, получили %d", n, len(got))
	}
}

// Потолок существует, чтобы подписка на сотни узлов не открыла сотни сокетов
// разом.
func TestProbeAutoNodes_PoolIsCappedAtMaxConcurrency(t *testing.T) {
	if autoProbeMaxConcurrency != 64 {
		t.Fatalf("ожидали потолок 64, получили %d", autoProbeMaxConcurrency)
	}
}

// stubProbeResolver заставляет свип дозваниваться ровно по тому адресу, что
// записан в узле. Фикстуры называются «a» / «node-0», а не реальными хостами,
// так что без этой подмены каждый свип в сюите выжидал бы настоящий
// DNS-таймаут на каждом из них.
func stubProbeResolver(t *testing.T) {
	t.Helper()
	old := autoProbeResolveHost
	t.Cleanup(func() { autoProbeResolveHost = old })
	autoProbeResolveHost = func(_ context.Context, host string) (string, bool) { return host, true }
}

// Провайдер выдаёт по 4-5 портов на один хост. Резолв на каждую пробу — это
// десятки лишних лукапов и, что важнее, случайное время резолва внутри
// измеряемого RTT: порты одного хоста получали несравнимые между собой числа.
func TestProbeAutoNodes_ResolvesEachHostOnce(t *testing.T) {
	oldTCP, oldBind, oldResolve := pingTCPProbe, autoProbeBindsToLAN, autoProbeResolveHost
	defer func() {
		pingTCPProbe, autoProbeBindsToLAN, autoProbeResolveHost = oldTCP, oldBind, oldResolve
	}()
	autoProbeBindsToLAN = func() bool { return false }

	var mu sync.Mutex
	lookups := map[string]int{}
	autoProbeResolveHost = func(_ context.Context, host string) (string, bool) {
		mu.Lock()
		lookups[host]++
		mu.Unlock()
		return "10.0.0.1", true
	}

	var dialed []string
	pingTCPProbe = func(ip string, _ int) (int64, bool, string) {
		mu.Lock()
		dialed = append(dialed, ip)
		mu.Unlock()
		return 10, true, ""
	}

	nodes := []config.ProxyEntry{
		{ID: "1", IP: "cdn1.example.test", Port: 443, Type: "VLESS"},
		{ID: "2", IP: "cdn1.example.test", Port: 1443, Type: "VLESS"},
		{ID: "3", IP: "cdn1.example.test", Port: 7443, Type: "TROJAN"},
	}
	ProbeAutoNodes(context.Background(), nodes, DepthFast)

	if lookups["cdn1.example.test"] != 1 {
		t.Fatalf("ожидали ровно один резолв хоста, получили %d", lookups["cdn1.example.test"])
	}
	if len(dialed) != 3 {
		t.Fatalf("ожидали 3 пробы, получили %d", len(dialed))
	}
	for _, ip := range dialed {
		if ip != "10.0.0.1" {
			t.Fatalf("проба должна дозваниваться по резолвнутому IP, получили %q", ip)
		}
	}
}

// Имена хостов регистронезависимы (RFC 4343): одна и та же машина, записанная
// в подписке в разном регистре, обязана резолвиться один раз — иначе
// собственная гарантия «каждый хост ровно однажды» не выполняется.
func TestProbeAutoNodes_ResolvesHostCaseInsensitively(t *testing.T) {
	oldTCP, oldBind, oldResolve := pingTCPProbe, autoProbeBindsToLAN, autoProbeResolveHost
	defer func() {
		pingTCPProbe, autoProbeBindsToLAN, autoProbeResolveHost = oldTCP, oldBind, oldResolve
	}()
	autoProbeBindsToLAN = func() bool { return false }

	var mu sync.Mutex
	var lookups int
	autoProbeResolveHost = func(_ context.Context, host string) (string, bool) {
		mu.Lock()
		lookups++
		mu.Unlock()
		if host != "cdn1.example.test" {
			t.Errorf("резолвить нужно приведённое к нижнему регистру имя, получили %q", host)
		}
		return "10.0.0.1", true
	}

	var dialed []string
	pingTCPProbe = func(ip string, _ int) (int64, bool, string) {
		mu.Lock()
		dialed = append(dialed, ip)
		mu.Unlock()
		return 10, true, ""
	}

	ProbeAutoNodes(context.Background(), []config.ProxyEntry{
		{ID: "1", IP: "CDN1.example.test", Port: 443, Type: "VLESS"},
		{ID: "2", IP: "cdn1.example.test", Port: 1443, Type: "VLESS"},
		{ID: "3", IP: " Cdn1.Example.Test ", Port: 7443, Type: "VLESS"},
	}, DepthFast)

	if lookups != 1 {
		t.Fatalf("ожидали ровно один резолв на три написания одного хоста, получили %d", lookups)
	}
	if len(dialed) != 3 {
		t.Fatalf("ожидали 3 пробы, получили %d", len(dialed))
	}
	for _, ip := range dialed {
		if ip != "10.0.0.1" {
			t.Fatalf("все три узла должны дозваниваться по резолвнутому IP, получили %q", ip)
		}
	}
}

// Фан-аут резолвера ограничен тем же потолком, что и пул проб, и по той же
// причине: на Windows каждый getaddrinfo блокирует поток ОС, так что подписка
// на сотни разных хостов иначе запустила бы сотни лукапов разом.
func TestResolveProbeHosts_FanOutIsCappedAtMaxConcurrency(t *testing.T) {
	oldResolve := autoProbeResolveHost
	defer func() { autoProbeResolveHost = oldResolve }()

	const n = autoProbeMaxConcurrency * 2
	var inFlight, maxInFlight int32
	release := make(chan struct{})

	autoProbeResolveHost = func(_ context.Context, _ string) (string, bool) {
		cur := atomic.AddInt32(&inFlight, 1)
		for {
			prev := atomic.LoadInt32(&maxInFlight)
			if cur <= prev || atomic.CompareAndSwapInt32(&maxInFlight, prev, cur) {
				break
			}
		}
		<-release
		atomic.AddInt32(&inFlight, -1)
		return "10.0.0.1", true
	}

	targets := make([]config.ProxyEntry, 0, n)
	for i := 0; i < n; i++ {
		targets = append(targets, config.ProxyEntry{
			ID: fmt.Sprint(i), IP: fmt.Sprintf("cdn%d.example.test", i), Port: 443, Type: "VLESS",
		})
	}

	done := make(chan map[string]string, 1)
	go func() { done <- resolveProbeHosts(context.Background(), targets) }()

	// Пул не может быть больше потолка, значит больше потолка лукапов
	// одновременно висеть не должно — ждём, пока он заполнится, и отпускаем.
	deadline := time.After(5 * time.Second)
	for atomic.LoadInt32(&inFlight) < autoProbeMaxConcurrency {
		select {
		case <-deadline:
			t.Fatalf("пул не заполнился: в полёте %d", atomic.LoadInt32(&inFlight))
		default:
			time.Sleep(time.Millisecond)
		}
	}
	if got := atomic.LoadInt32(&maxInFlight); got > autoProbeMaxConcurrency {
		t.Fatalf("одновременных лукапов %d, потолок %d", got, autoProbeMaxConcurrency)
	}
	close(release)

	out := <-done
	if got := atomic.LoadInt32(&maxInFlight); got > autoProbeMaxConcurrency {
		t.Fatalf("одновременных лукапов %d, потолок %d", got, autoProbeMaxConcurrency)
	}
	if len(out) != n {
		t.Fatalf("ожидали %d резолвнутых хостов, получили %d", n, len(out))
	}
}

// pickIPv4 — чистая часть правила отбора: обвязка проб ниже умеет только v4,
// поэтому AAAA-первый ответ обязан быть пропущен, а ответ без A-записи —
// честно провалиться, а не откатиться на v6-литерал.
func TestPickIPv4_SkipsAAAAAndFailsWithoutARecord(t *testing.T) {
	if ip, ok := pickIPv4([]net.IPAddr{
		{IP: net.ParseIP("2a01:4f8::1")},
		{IP: net.ParseIP("203.0.113.9")},
	}); !ok || ip != "203.0.113.9" {
		t.Fatalf("ожидали 203.0.113.9, получили ip=%q ok=%v", ip, ok)
	}
	if ip, ok := pickIPv4([]net.IPAddr{{IP: net.ParseIP("2a01:4f8::1")}}); ok {
		t.Fatalf("ожидали отказ без A-записи, получили ip=%q", ip)
	}
	if ip, ok := pickIPv4(nil); ok {
		t.Fatalf("ожидали отказ на пустом ответе, получили ip=%q", ip)
	}
}

// Строка лога «опрошено N узлов» считает именно то, что свип дозвонит:
// SECTION-заголовки и записи без адреса он отбрасывает.
func TestCountProbeableNodes_CountsOnlyDialableEntries(t *testing.T) {
	got := CountProbeableNodes([]config.ProxyEntry{
		{ID: "1", IP: "1.1.1.1", Port: 443, Type: "VLESS"},
		{ID: "2", IP: "", Port: 0, Type: "SECTION", Name: "🚀 когда душат"},
		{ID: "3", IP: "", Port: 443, Type: "VLESS"},
		{ID: "4", IP: "2.2.2.2", Port: 0, Type: "VLESS"},
		{ID: "5", IP: "3.3.3.3", Port: 8443, Type: "TROJAN"},
	})
	if got != 2 {
		t.Fatalf("ожидали 2 пробуемых узла, получили %d", got)
	}
}

// LAN-bind ветка сквозь весь свип: до сих пор все тесты фиксировали
// autoProbeBindsToLAN = false, так что связка «есть физический адаптер →
// берём LAN-bound вариант пробы» не проверялась целиком ни разу.
func TestProbeAutoNodes_UsesLANBoundProbesWhenAdapterAvailable(t *testing.T) {
	stubProbeResolver(t)
	oldTCP, oldLAN, oldBind := pingTCPProbe, pingLANProbe, autoProbeBindsToLAN
	defer func() {
		pingTCPProbe, pingLANProbe, autoProbeBindsToLAN = oldTCP, oldLAN, oldBind
	}()
	autoProbeBindsToLAN = func() bool { return true }

	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		t.Error("при доступном адаптере проба обязана идти LAN-bound вариантом")
		return 0, false, ""
	}
	var mu sync.Mutex
	var lanCalls []string
	pingLANProbe = func(ip string, _ int) (int64, bool, string) {
		mu.Lock()
		lanCalls = append(lanCalls, ip)
		mu.Unlock()
		return 25, true, ""
	}

	got := ProbeAutoNodes(context.Background(), mkNodes("1.1.1.1", "2.2.2.2"), DepthFast)
	if len(got) != 2 || !got[0].OK || !got[1].OK {
		t.Fatalf("ожидали два успешных результата, получили %+v", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(lanCalls) != 2 {
		t.Fatalf("ожидали 2 LAN-bound пробы, получили %d", len(lanCalls))
	}
}

// Loopback — единственное исключение: Windows отклоняет дозвон к 127.0.0.1 с
// не-loopback источника, так что бинд обязан отключаться, даже когда адаптер
// доступен.
func TestProbeAutoNodes_SkipsLANBindForLoopbackTarget(t *testing.T) {
	stubProbeResolver(t)
	oldTCP, oldLAN, oldBind := pingTCPProbe, pingLANProbe, autoProbeBindsToLAN
	defer func() {
		pingTCPProbe, pingLANProbe, autoProbeBindsToLAN = oldTCP, oldLAN, oldBind
	}()
	autoProbeBindsToLAN = func() bool { return true }

	pingLANProbe = func(_ string, _ int) (int64, bool, string) {
		t.Error("для loopback-цели LAN-bind обязан быть пропущен")
		return 0, false, ""
	}
	called := ""
	pingTCPProbe = func(ip string, _ int) (int64, bool, string) { called = ip; return 5, true, "" }

	ProbeAutoNodes(context.Background(), mkNodes("127.0.0.1"), DepthFast)
	if called != "127.0.0.1" {
		t.Fatalf("ожидали небинденную пробу к 127.0.0.1, получили %q", called)
	}
}

// Литеральный IP резолвить нечего.
func TestProbeAutoNodes_DoesNotResolveLiteralIPs(t *testing.T) {
	oldTCP, oldBind, oldResolve := pingTCPProbe, autoProbeBindsToLAN, autoProbeResolveHost
	defer func() {
		pingTCPProbe, autoProbeBindsToLAN, autoProbeResolveHost = oldTCP, oldBind, oldResolve
	}()
	autoProbeBindsToLAN = func() bool { return false }

	autoProbeResolveHost = func(_ context.Context, host string) (string, bool) {
		t.Errorf("литеральный IP %q резолвить не нужно", host)
		return "", false
	}
	got := ""
	pingTCPProbe = func(ip string, _ int) (int64, bool, string) { got = ip; return 10, true, "" }

	ProbeAutoNodes(context.Background(),
		[]config.ProxyEntry{{ID: "1", IP: "203.0.113.7", Port: 443, Type: "VLESS"}}, DepthFast)

	if got != "203.0.113.7" {
		t.Fatalf("ожидали дозвон по 203.0.113.7, получили %q", got)
	}
}

// Не резолвится — пробуем по исходному имени, пусть с ним разбирается диалер.
// Терять узел из-за одного неудачного лукапа нельзя.
func TestProbeAutoNodes_FallsBackToHostnameWhenResolveFails(t *testing.T) {
	oldTCP, oldBind, oldResolve := pingTCPProbe, autoProbeBindsToLAN, autoProbeResolveHost
	defer func() {
		pingTCPProbe, autoProbeBindsToLAN, autoProbeResolveHost = oldTCP, oldBind, oldResolve
	}()
	autoProbeBindsToLAN = func() bool { return false }

	autoProbeResolveHost = func(_ context.Context, _ string) (string, bool) { return "", false }
	got := ""
	pingTCPProbe = func(ip string, _ int) (int64, bool, string) { got = ip; return 10, true, "" }

	ProbeAutoNodes(context.Background(),
		[]config.ProxyEntry{{ID: "1", IP: "cdn9.example.test", Port: 443, Type: "VLESS"}}, DepthFast)

	if got != "cdn9.example.test" {
		t.Fatalf("ожидали дозвон по исходному имени, получили %q", got)
	}
}

// Резолв не имеет права влиять на ключ узла: он хеширует e.IP, и подмена
// исходного имени на IP расколола бы историю узла в node_stats.json надвое.
func TestProbeAutoNodes_ResolveDoesNotChangeNodeKey(t *testing.T) {
	oldTCP, oldBind, oldResolve := pingTCPProbe, autoProbeBindsToLAN, autoProbeResolveHost
	defer func() {
		pingTCPProbe, autoProbeBindsToLAN, autoProbeResolveHost = oldTCP, oldBind, oldResolve
	}()
	autoProbeBindsToLAN = func() bool { return false }
	autoProbeResolveHost = func(_ context.Context, _ string) (string, bool) { return "10.0.0.1", true }
	pingTCPProbe = func(_ string, _ int) (int64, bool, string) { return 10, true, "" }

	node := config.ProxyEntry{ID: "1", IP: "cdn1.example.test", Port: 443, Type: "VLESS"}
	got := ProbeAutoNodes(context.Background(), []config.ProxyEntry{node}, DepthFast)

	if len(got) != 1 || got[0].Key != AutoNodeKey(node) {
		t.Fatalf("ключ должен считаться по исходной записи, получили %+v", got)
	}
}

// SNI берётся из исходного имени, а не из резолвнутого IP — иначе TLS-фаза
// перестанет видеть блокировку по SNI, ради которой она и существует.
func TestProbeOne_TLSPhaseKeepsOriginalSNIAfterResolve(t *testing.T) {
	oldTCP, oldTLS, oldBind := pingTCPProbe, autoTLSProbe, autoProbeBindsToLAN
	defer func() { pingTCPProbe, autoTLSProbe, autoProbeBindsToLAN = oldTCP, oldTLS, oldBind }()
	autoProbeBindsToLAN = func() bool { return false }
	pingTCPProbe = func(_ string, _ int) (int64, bool, string) { return 10, true, "" }

	var gotHost, gotSNI string
	autoTLSProbe = func(host string, _ int, sni string, _ []string) (int64, bool, string) {
		gotHost, gotSNI = host, sni
		return 30, true, ""
	}

	node := config.ProxyEntry{ID: "1", IP: "cdn1.example.test", Port: 443, Type: "TROJAN"}
	probeOne(node, DepthFull, "10.0.0.1")

	if gotHost != "10.0.0.1" {
		t.Fatalf("TLS-проба должна дозваниваться по резолвнутому IP, получили %q", gotHost)
	}
	if gotSNI != "cdn1.example.test" {
		t.Fatalf("SNI должен остаться исходным именем, получили %q", gotSNI)
	}
}

// Резолвер ОС может вернуть AAAA раньше A (на Windows getaddrinfo сортирует
// по RFC 6724, и на машине с рабочим IPv6 v6-запись часто оказывается
// первой). Вся проб-обвязка ниже — TCP-адрес, LAN-bind, форс ip4 у WireGuard
// — рассчитана только на v4, так что резолв обязан явно выбрать v4-литерал,
// а не слепо брать первый ответ.
func TestAutoProbeResolveHost_PrefersIPv4WhenAAAAReturnedFirst(t *testing.T) {
	oldLookup := autoProbeLookupIPAddr
	defer func() { autoProbeLookupIPAddr = oldLookup }()
	autoProbeLookupIPAddr = func(_ context.Context, _ string) ([]net.IPAddr, error) {
		return []net.IPAddr{
			{IP: net.ParseIP("2a01:4f8::1")},
			{IP: net.ParseIP("203.0.113.9")},
		}, nil
	}

	ip, ok := autoProbeResolveHost(context.Background(), "cdn1.example.test")
	if !ok || ip != "203.0.113.9" {
		t.Fatalf("ожидали IPv4-литерал 203.0.113.9 несмотря на AAAA первым в ответе, получили ip=%q ok=%v", ip, ok)
	}
}

// Если A-записи вообще нет, резолв обязан провалиться, а не откатиться на
// v6-литерал: проб-обвязка ниже не умеет его дозвонить, и откат на v6 лишь
// гарантировал бы отказ там, где дозвон по исходному имени мог бы сработать.
func TestAutoProbeResolveHost_NoARecordReportsFailure(t *testing.T) {
	oldLookup := autoProbeLookupIPAddr
	defer func() { autoProbeLookupIPAddr = oldLookup }()
	autoProbeLookupIPAddr = func(_ context.Context, _ string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("2a01:4f8::1")}}, nil
	}

	if ip, ok := autoProbeResolveHost(context.Background(), "v6only.example.test"); ok {
		t.Fatalf("ожидали отказ резолва при отсутствии A-записи, получили ip=%q ok=true", ip)
	}
}

// Сквозной сценарий: домен отвечает только AAAA (или временно без A), резолв
// внутри свипа честно проваливается — и свип обязан пробовать дозвон по
// исходному имени вместо того, чтобы молча терять узел.
func TestProbeAutoNodes_NoIPv4AnswerFallsBackToHostnameDial(t *testing.T) {
	oldTCP, oldBind, oldLookup := pingTCPProbe, autoProbeBindsToLAN, autoProbeLookupIPAddr
	defer func() {
		pingTCPProbe, autoProbeBindsToLAN, autoProbeLookupIPAddr = oldTCP, oldBind, oldLookup
	}()
	autoProbeBindsToLAN = func() bool { return false }
	autoProbeLookupIPAddr = func(_ context.Context, _ string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("2a01:4f8::1")}}, nil
	}

	got := ""
	pingTCPProbe = func(ip string, _ int) (int64, bool, string) { got = ip; return 10, true, "" }

	ProbeAutoNodes(context.Background(),
		[]config.ProxyEntry{{ID: "1", IP: "v6only.example.test", Port: 443, Type: "VLESS"}}, DepthFast)

	if got != "v6only.example.test" {
		t.Fatalf("ожидали дозвон по исходному имени при отсутствии A-записи, получили %q", got)
	}
}

// autoProbeHysteria2SNI обязан подставлять SNI из Extra, если он там задан —
// иначе проба всегда бьёт по голому адресу узла вместо настоящего фронта,
// которым домен на самом деле прикрыт.
func TestAutoProbeHysteria2SNI_PrefersExplicitSNIFromExtra(t *testing.T) {
	e := config.ProxyEntry{
		IP:    "1.2.3.4",
		Type:  "HYSTERIA2",
		Extra: json.RawMessage(`{"sni":"front.example.com"}`),
	}
	if got := autoProbeHysteria2SNI(e); got != "front.example.com" {
		t.Fatalf("ожидали SNI из extra, получили %q", got)
	}
}

// Падение по цепочке: sni -> server_name (snake_case — так называет это поле
// именно HYSTERIA2-ветка buildProxyOutboundRaw) -> адрес узла как крайний
// случай.
func TestAutoProbeHysteria2SNI_FallsBackToServerNameThenAddress(t *testing.T) {
	withServerName := config.ProxyEntry{IP: "1.2.3.4", Extra: json.RawMessage(`{"server_name":"alt.example.com"}`)}
	if got := autoProbeHysteria2SNI(withServerName); got != "alt.example.com" {
		t.Fatalf("ожидали server_name как SNI, получили %q", got)
	}

	bare := config.ProxyEntry{IP: "hy2.example.test"}
	if got := autoProbeHysteria2SNI(bare); got != "hy2.example.test" {
		t.Fatalf("ожидали адрес узла как SNI по умолчанию, получили %q", got)
	}
}
