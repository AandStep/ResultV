// Copyright (C) 2026 ResultV
//
// Licensed under the terms of the GNU General Public License v3 or later.

package main

import (
	"encoding/json"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/proxy"
)

// TestProxyLogLabel_NeverLeaksSubscriptionAddress is the regression test for
// the leak the AUTO logs shipped with: the per-member probe table and the
// "первый — host:port" line printed provider backend addresses
// (cdn34.example.digital:8443) straight into a log the user can export and
// share. internal/proxy has guarded this all along — every address there sits
// behind `SubscriptionURL == ""`, and newSingBoxLogWriter redacts the server
// out of engine output — but app.go had no such guard.
func TestProxyLogLabel_NeverLeaksSubscriptionAddress(t *testing.T) {
	cases := []struct {
		name  string
		entry config.ProxyEntry
		want  []string
		deny  []string
	}{
		{
			name: "подписка: имя и страна вместо адреса",
			entry: config.ProxyEntry{
				Name: "Netherlands #12", Country: "NL",
				IP: "cdn34.example.digital", Port: 8443,
				SubscriptionURL: "https://provider.example/sub",
			},
			want: []string{"Netherlands #12", "NL"},
			deny: []string{"cdn34.example.digital", "8443"},
		},
		{
			name: "подписка без страны: только имя",
			entry: config.ProxyEntry{
				Name: "Node 7", IP: "1.2.3.4", Port: 443,
				SubscriptionURL: "https://provider.example/sub",
			},
			want: []string{"Node 7"},
			deny: []string{"1.2.3.4", "443"},
		},
		{
			name: "подписка без имени: страна, но не адрес",
			entry: config.ProxyEntry{
				Country: "DE", IP: "5.6.7.8", Port: 443,
				SubscriptionURL: "https://provider.example/sub",
			},
			want: []string{"DE"},
			deny: []string{"5.6.7.8"},
		},
		{
			name: "подписка без имени и страны: адрес всё равно скрыт",
			entry: config.ProxyEntry{
				IP: "9.9.9.9", Port: 443,
				SubscriptionURL: "https://provider.example/sub",
			},
			deny: []string{"9.9.9.9"},
		},
		{
			// Manual servers keep full detail — same policy as manager.Connect
			// and newSingBoxLogWriter: the user owns that address, already sees
			// it in the UI, and here it is the diagnostic payload.
			name:  "ручной сервер: адрес сохраняется",
			entry: config.ProxyEntry{Name: "Personal", IP: "4.4.4.4", Port: 8443},
			want:  []string{"Personal", "4.4.4.4", "8443"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := proxyLogLabel(tc.entry)
			if strings.TrimSpace(got) == "" {
				t.Fatal("метка не должна быть пустой — иначе строка лога теряет смысл")
			}
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("ожидали %q в метке, получили %q", w, got)
				}
			}
			for _, d := range tc.deny {
				if strings.Contains(got, d) {
					t.Errorf("адрес подписки утёк в лог: %q содержит %q", got, d)
				}
			}
		})
	}
}

func TestFormatAutoPickLine_NamesGroupAndPickWithoutAddress(t *testing.T) {
	pick := config.ProxyEntry{
		Name: "Netherlands #12", Country: "NL",
		IP: "cdn34.example.digital", Port: 8443,
		SubscriptionURL: "https://provider.example/sub",
	}

	got := formatAutoPickLine("🚀 impVPN Auto", 5, pick)

	for _, want := range []string{"impVPN Auto", "5", "Netherlands #12", "NL"} {
		if !strings.Contains(got, want) {
			t.Errorf("ожидали %q в строке, получили %q", want, got)
		}
	}
	for _, deny := range []string{"cdn34.example.digital", "8443"} {
		if strings.Contains(got, deny) {
			t.Errorf("адрес подписки утёк в лог: %q содержит %q", got, deny)
		}
	}
}

func TestExtractAutoMembers_ParsesBothExtraEncodings(t *testing.T) {
	plain := json.RawMessage(`{"members":["m1","m2"]}`)
	if got := extractAutoMembers(plain); len(got) != 2 || got[0] != "m1" {
		t.Errorf("прямой JSON: ожидали [m1 m2], получили %v", got)
	}

	quoted := json.RawMessage(`"{\"members\":[\"m3\"]}"`)
	if got := extractAutoMembers(quoted); len(got) != 1 || got[0] != "m3" {
		t.Errorf("JSON-в-строке: ожидали [m3], получили %v", got)
	}

	if got := extractAutoMembers(nil); len(got) != 0 {
		t.Errorf("пустой extra: ожидали пусто, получили %v", got)
	}
}

// TestReportAutoConnectOutcome_IndexDisambiguatesCollidingAddress covers the
// exact collision AutoNodeKey's doc comment warns about: CDN-fronted and
// multi-account panels can issue several logically distinct nodes on one
// host:port:type. member1 and member2 below share IP and Port and differ
// only in Extra, so ReportAutoConnectOutcome's old "first IP/Port match"
// lookup would have credited whichever of them happened to sort first with
// the other's connect result. The fix looks the candidate up by index
// instead (verified against the address only as a staleness check), so this
// test asserts the outcome lands under member2's AutoNodeKey and nowhere
// near member1's.
//
// Why a real loopback listener instead of mocking the probe: pingTCPProbe and
// autoTLSProbe (internal/proxy/autoprobe.go, manager.go) are unexported
// package-level vars, substitutable only by test code inside package proxy
// (see internal/proxy/autoselect_test.go) — unreachable from this package
// main test. Exporting them just for this test would widen the production
// API for a test-only need, so instead this test opens a real 127.0.0.1
// listener and points both members at it: RankAutoCandidates' phase-1 TCP
// probe then succeeds deterministically without touching any real host or
// the network this machine actually routes through. Both members use Type
// "SS" so phase 2 never attempts a TLS handshake (autoProbeTLSParams treats
// SS/SHADOWSOCKS/WIREGUARD/AMNEZIAWG/HYSTERIA2 as wantTLS=false) — a bare
// loopback socket has no certificate to offer.
func TestReportAutoConnectOutcome_IndexDisambiguatesCollidingAddress(t *testing.T) {
	// nodeStats() falls back to an in-memory store when unset, but that
	// fallback is a shared package-level var — leaving it installed would
	// leak this test's records into whichever test runs next in the same
	// binary. Reset it afterward instead of trying to save/restore the
	// previous value: there is no exported getter, and SetNodeStatStore(nil)
	// puts the package back exactly where "never configured" would.
	t.Cleanup(func() { proxy.SetNodeStatStore(nil) })
	proxy.SetNodeStatStore(proxy.NewNodeStatStore(t.TempDir()))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	_, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	member1 := config.ProxyEntry{
		ID: "m1", Name: "Member 1", Type: "SS",
		IP: "127.0.0.1", Port: port,
		Extra: json.RawMessage(`{"password":"pw1"}`),
	}
	member2 := config.ProxyEntry{
		ID: "m2", Name: "Member 2", Type: "SS",
		IP: "127.0.0.1", Port: port,
		Extra: json.RawMessage(`{"password":"pw2"}`),
	}
	head := config.ProxyEntry{
		ID:    "auto-1",
		Name:  "Auto Group",
		Type:  "AUTO",
		Extra: mustExtra(t, []string{member1.ID, member2.ID}),
	}

	a := newTestApp(t, t.TempDir())
	cfg := a.config.GetConfig()
	cfg.Proxies = []config.ProxyEntry{head, member1, member2}
	if err := a.config.SaveConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	ranked := a.ResolveAutoCandidates(head.ID)
	if len(ranked) != 2 {
		t.Fatalf("ожидали обоих коллидирующих членов в ранжировании, получили %d: %+v", len(ranked), ranked)
	}

	// Find member2's position in the cache ReportAutoConnectOutcome will
	// consult. NOT by IP/Port — both members share the address, that is the
	// entire point of this test — but by the same AutoNodeKey the probe used,
	// which is exactly what the UI cannot compute (it only ever sees the
	// address) and exactly why the index has to come from the loop position.
	a.autoCandidatesMu.Lock()
	cached := a.autoCandidates[head.ID]
	a.autoCandidatesMu.Unlock()
	idx := -1
	for i, c := range cached {
		if proxy.AutoNodeKey(c) == proxy.AutoNodeKey(member2) {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("member2 не найден среди кэшированных кандидатов: %+v", cached)
	}

	a.ReportAutoConnectOutcome(head.ID, idx, member2.IP, member2.Port, true)

	key1, key2 := proxy.AutoNodeKey(member1), proxy.AutoNodeKey(member2)
	if got := proxy.LookupNodeStat(key2).ConnectOK; got != 1 {
		t.Errorf("результат должен был попасть под ключ member2 (ConnectOK=1), получили %d", got)
	}
	if got := proxy.LookupNodeStat(key1).ConnectOK; got != 0 {
		t.Errorf("результат НЕ должен был попасть под ключ member1, получили ConnectOK=%d", got)
	}
}

// TestReportAutoConnectOutcome_GuardsOutOfRangeAndMismatchedAddress locks in
// the two silent-no-op paths app.go's ReportAutoConnectOutcome added for
// index-based lookup: an index outside the cached slice, and an index whose
// cached entry's address disagrees with what the caller reported (the cache
// having been replaced since the UI fetched its candidate list, e.g. by a
// subscription refresh mid-attempt). Both must record nothing — misattributing
// an outcome to the wrong node is worse than losing the datapoint.
func TestReportAutoConnectOutcome_GuardsOutOfRangeAndMismatchedAddress(t *testing.T) {
	t.Cleanup(func() { proxy.SetNodeStatStore(nil) })
	proxy.SetNodeStatStore(proxy.NewNodeStatStore(t.TempDir()))

	member1 := config.ProxyEntry{ID: "m1", Name: "Member 1", Type: "VLESS", IP: "127.0.0.1", Port: 1111}
	member2 := config.ProxyEntry{ID: "m2", Name: "Member 2", Type: "VLESS", IP: "127.0.0.1", Port: 2222}

	a := newTestApp(t, t.TempDir())
	// Populate the cache directly (same package as app.go, field is
	// unexported) rather than through ResolveAutoCandidates: these two cases
	// exercise the guard clauses alone and need no real probing.
	a.autoCandidatesMu.Lock()
	a.autoCandidates["head-1"] = []config.ProxyEntry{member1, member2}
	a.autoCandidatesMu.Unlock()

	assertNothingRecorded := func(t *testing.T) {
		t.Helper()
		for _, m := range []config.ProxyEntry{member1, member2} {
			st := proxy.LookupNodeStat(proxy.AutoNodeKey(m))
			if st.ConnectOK != 0 || st.ConnectFail != 0 {
				t.Errorf("ожидали отсутствие записи для %s, получили %+v", m.ID, st)
			}
		}
	}

	// Out of range: only 2 candidates cached, index 2 is one past the end.
	a.ReportAutoConnectOutcome("head-1", 2, member1.IP, member1.Port, true)
	assertNothingRecorded(t)

	// Negative index.
	a.ReportAutoConnectOutcome("head-1", -1, member1.IP, member1.Port, true)
	assertNothingRecorded(t)

	// Valid index, but the reported address does not match what is cached at
	// that position (stale cache from the UI's point of view).
	a.ReportAutoConnectOutcome("head-1", 1, member1.IP, member1.Port, true)
	assertNothingRecorded(t)

	// Sanity check: the same call with the correct address at that index DOES
	// record, so assertNothingRecorded above was actually testing the guard
	// and not, say, a typo'd proxyID that never matched anything.
	a.ReportAutoConnectOutcome("head-1", 1, member2.IP, member2.Port, true)
	if got := proxy.LookupNodeStat(proxy.AutoNodeKey(member2)).ConnectOK; got != 1 {
		t.Fatalf("контрольный вызов с верным адресом должен был записать результат, получили ConnectOK=%d", got)
	}
}

// TestAutoOutcomeKey_NonAutoNeverRecords is the regression test for one half
// of finding #5's landmine: connectFromTray used to record connect outcomes
// and set lastAutoNodeKey for non-AUTO proxies too, unlike the frontend's
// retry loop which deliberately guards with `if (isAuto)`
// (useDaemonControl.js). Connecting to a plain server from the tray would
// overwrite the global lastAutoNodeKey with a key no AUTO group's cache
// contains — every AUTO row would then revert to showing the member minimum
// — and pile junk entries into node_stats.json. autoOutcomeKey must return ""
// for isAuto=false regardless of what is cached, so callers never record.
func TestAutoOutcomeKey_NonAutoNeverRecords(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	member := config.ProxyEntry{ID: "m1", Name: "Member 1", Type: "VLESS", IP: "1.1.1.1", Port: 443}
	a.autoCandidatesMu.Lock()
	a.autoCandidates["plain-1"] = []config.ProxyEntry{member}
	a.autoCandidatesMu.Unlock()

	if got := a.autoOutcomeKey("plain-1", false, 0); got != "" {
		t.Errorf("проксирование не-AUTO сервера не должно давать ключ, получили %q", got)
	}
}

// TestAutoOutcomeKey_UsesUnstampedCacheEntry is the regression test for the
// other half of finding #5's landmine: connectFromTray used to compute the
// node key from the identity-stamped candidate (ResolveAutoCandidates
// overwrites a blank member SubscriptionURL with the head's before handing
// candidates back — see its "Keep the AUTO head's identity" comment), while
// ReportAutoConnectOutcome always keys off the UNSTAMPED entry cached in
// a.autoCandidates. AutoNodeKey hashes SubscriptionURL, so stamping can
// change the key, and the two call sites would silently disagree on which
// key a given connect outcome belongs under. autoOutcomeKey must read the
// cache — the same thing ReportAutoConnectOutcome reads — never the stamped
// copy.
func TestAutoOutcomeKey_UsesUnstampedCacheEntry(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	// Blank SubscriptionURL, exactly as a member looks BEFORE
	// ResolveAutoCandidates' identity stamp fills it in from the head.
	member := config.ProxyEntry{ID: "m1", Name: "Member 1", Type: "VLESS", IP: "1.1.1.1", Port: 443}
	a.autoCandidatesMu.Lock()
	a.autoCandidates["auto-1"] = []config.ProxyEntry{member}
	a.autoCandidatesMu.Unlock()

	want := proxy.AutoNodeKey(member)
	if got := a.autoOutcomeKey("auto-1", true, 0); got != want {
		t.Errorf("key = %q, хотели %q (ключ некэшированной/пришпиленной копии)", got, want)
	}

	// Prove the divergence this test guards against is real: the stamped
	// copy (SubscriptionURL filled from the head) must hash to a DIFFERENT
	// key, or this test would not be exercising anything.
	stamped := member
	stamped.SubscriptionURL = "https://head.example/sub"
	if proxy.AutoNodeKey(stamped) == want {
		t.Fatal("неверная настройка теста: простановка SubscriptionURL не изменила ключ")
	}
}

// TestAutoOutcomeKey_OutOfRangeIndexYieldsEmptyKey mirrors
// ReportAutoConnectOutcome's own out-of-range guard (see
// TestReportAutoConnectOutcome_GuardsOutOfRangeAndMismatchedAddress) so the
// two call sites stay consistent about what "no such candidate" means.
func TestAutoOutcomeKey_OutOfRangeIndexYieldsEmptyKey(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	a.autoCandidatesMu.Lock()
	a.autoCandidates["auto-1"] = []config.ProxyEntry{{ID: "m1", IP: "1.1.1.1", Port: 443, Type: "VLESS"}}
	a.autoCandidatesMu.Unlock()

	if got := a.autoOutcomeKey("auto-1", true, 5); got != "" {
		t.Errorf("индекс за пределами кэша должен давать пустой ключ, получили %q", got)
	}
	if got := a.autoOutcomeKey("auto-1", true, -1); got != "" {
		t.Errorf("отрицательный индекс должен давать пустой ключ, получили %q", got)
	}
}

// TestGetAutoGroupStatus_ReportsConnectedMemberScopedToItsGroup covers three
// properties GetAutoGroupStatus depends on but a bare port of the brief's
// snippet (which ignores its proxyID parameter and checks only
// a.lastAutoNodeKey) would get wrong the moment a config has more than one
// AUTO group:
//
//  1. Before any connect, status is unknown regardless of proxyID.
//  2. Once connected, the OWNING group's row reports the resolved member's
//     name/IP and RTT.
//  3. A DIFFERENT AUTO group's row must stay unknown — lastAutoNodeKey is one
//     global value, not scoped per group, so without checking group
//     membership every group would echo whichever one connected last.
//
// It also checks that a probed-but-never-connected node still reports a
// meaningful RTT (RecordProbe alone, no RecordConnect/ReportAutoConnectOutcome
// call) — NodeStat.EWMARTTms is populated by probes, not connects.
func TestGetAutoGroupStatus_ReportsConnectedMemberScopedToItsGroup(t *testing.T) {
	t.Cleanup(func() { proxy.SetNodeStatStore(nil) })
	store := proxy.NewNodeStatStore(t.TempDir())
	proxy.SetNodeStatStore(store)

	member1 := config.ProxyEntry{ID: "m1", Name: "Member 1", Type: "VLESS", IP: "127.0.0.1", Port: 1111}
	member2 := config.ProxyEntry{ID: "m2", Name: "Member 2", Type: "VLESS", IP: "203.0.113.9", Port: 2222}
	otherGroupMember := config.ProxyEntry{ID: "o1", Name: "Other Member", Type: "VLESS", IP: "198.51.100.1", Port: 3333}

	a := newTestApp(t, t.TempDir())

	if got := a.GetAutoGroupStatus("head-1"); got.Known {
		t.Fatalf("ожидали Known=false до подключения, получили %+v", got)
	}

	a.autoCandidatesMu.Lock()
	a.autoCandidates["head-1"] = []config.ProxyEntry{member1, member2}
	a.autoCandidates["head-2"] = []config.ProxyEntry{otherGroupMember}
	a.autoCandidatesMu.Unlock()

	key2 := proxy.AutoNodeKey(member2)
	store.RecordProbe(key2, 42, 3, true, "")
	a.setLastAutoNodeKey(key2)

	got := a.GetAutoGroupStatus("head-1")
	if !got.Known || got.NodeName != member2.Name || got.NodeIP != member2.IP || got.RTTms != 42 {
		t.Fatalf("ожидали статус member2 (42ms), получили %+v", got)
	}

	if got := a.GetAutoGroupStatus("head-2"); got.Known {
		t.Fatalf("статус чужой AUTO-группы не должен знать про узел другой группы, получили %+v", got)
	}

	// Probe-only record (no connect outcome at all): RTT must still surface.
	key1 := proxy.AutoNodeKey(member1)
	store.RecordProbe(key1, 15, 1, true, "")
	a.setLastAutoNodeKey(key1)
	got = a.GetAutoGroupStatus("head-1")
	if !got.Known || got.RTTms != 15 {
		t.Fatalf("RTT из пробы без коннекта должен быть виден, получили %+v", got)
	}
}

// sweepTimingSentinelResults builds Phase1/Phase2 rows carrying host-shaped
// and IP-shaped sentinel strings in Key and Reason — AutoProbeResult.Reason is
// free-form dial-error text and can legitimately contain raw host:port (the
// per-member probe table this project deleted leaked exactly that shape of
// data). A fixture that leaves these slices nil, as an earlier version of
// this test did, would stay green through a future edit that ranges over
// diag.Phase1/Phase2 and folds Reason into the line, because it would range
// zero times. Populating them makes that regression actually reachable by
// this test.
func sweepTimingSentinelResults() []proxy.AutoProbeResult {
	return []proxy.AutoProbeResult{
		{
			Key:    "probe-host-sentinel.example.invalid:65001",
			RTTms:  30,
			OK:     false,
			Stage:  "tcp",
			Reason: "dial tcp probe-host-sentinel.example.invalid:65001: i/o timeout",
		},
		{
			Key:    "198.51.100.77:65002",
			RTTms:  20,
			OK:     false,
			Stage:  "tls",
			Reason: "dial tcp 198.51.100.77:65002: connection refused",
		},
	}
}

// Строка тайминга свипа не имеет права раскрыть адреса подписки: она уходит в
// экспортируемый пользователем лог. Denylist ниже — это подстраховка второго
// уровня; главную гарантию даёт то, что diag ниже несёт непустые Phase1/Phase2
// с адресными сентинелами (см. sweepTimingSentinelResults) — иначе тест
// остался бы зелёным сквозь регрессию, где formatAutoSweepTimingLine начинает
// перебирать эти срезы и подмешивать Reason/Key в строку.
func TestFormatAutoSweepTimingLine_ContainsNoAddresses(t *testing.T) {
	diag := proxy.AutoProbeDiagnostics{
		Phase1Dur: 1200 * time.Millisecond,
		Phase2Dur: 300 * time.Millisecond,
		Phase1:    sweepTimingSentinelResults(),
		Phase2:    sweepTimingSentinelResults(),
	}
	line := formatAutoSweepTimingLine("🚀 impVPN Auto", 48, diag)

	forbidden := []string{
		"cdn", ".top", ".digital", ".today", ":8802", "45.145",
		"probe-host-sentinel.example.invalid", "198.51.100.77", "65001", "65002",
		"i/o timeout", "connection refused",
	}
	for _, f := range forbidden {
		if strings.Contains(line, f) {
			t.Fatalf("строка тайминга содержит %q: %s", f, line)
		}
	}
	if !strings.Contains(line, "🚀 impVPN Auto") || !strings.Contains(line, "48") ||
		!strings.Contains(line, "1200") || !strings.Contains(line, "300") {
		t.Fatalf("строка тайминга не содержит имя группы, счётчик или длительности: %s", line)
	}
}

func TestFormatAutoSweepTimingLine_MarksCachedResult(t *testing.T) {
	diag := proxy.AutoProbeDiagnostics{
		FromCache: true,
		Phase1:    sweepTimingSentinelResults(),
		Phase2:    sweepTimingSentinelResults(),
	}
	line := formatAutoSweepTimingLine("🚀 impVPN Auto", 48, diag)

	forbidden := []string{
		"cdn", ".top", ".digital", ".today", ":8802", "45.145",
		"probe-host-sentinel.example.invalid", "198.51.100.77", "65001", "65002",
		"i/o timeout", "connection refused",
	}
	for _, f := range forbidden {
		if strings.Contains(line, f) {
			t.Fatalf("строка кэша содержит %q: %s", f, line)
		}
	}
	if !strings.Contains(line, "кэш") {
		t.Fatalf("результат из кэша должен быть помечен: %s", line)
	}
	if !strings.Contains(line, "🚀 impVPN Auto") || !strings.Contains(line, "48") {
		t.Fatalf("строка кэша не содержит имя группы или счётчик: %s", line)
	}
}

// «фаза 2 0ms» читается двояко: и «отработала мгновенно», и «не запускалась»
// (фаза 1 никого не нашла). Пустой Phase2 различает эти случаи — строка обязана
// сказать, какой именно, а не оставлять читателя гадать по правдоподобному, но
// невозможному нулю.
func TestFormatAutoSweepTimingLine_SaysWhenPhase2NeverRan(t *testing.T) {
	diag := proxy.AutoProbeDiagnostics{
		Phase1Dur: 1200 * time.Millisecond,
		Phase1:    sweepTimingSentinelResults(),
	}
	line := formatAutoSweepTimingLine("🚀 impVPN Auto", 48, diag)

	if strings.Contains(line, "фаза 2 0ms") {
		t.Fatalf("незапущенная фаза 2 не должна показываться как 0ms: %s", line)
	}
	if !strings.Contains(line, "не запускалась") {
		t.Fatalf("ожидали явную пометку о незапущенной фазе 2: %s", line)
	}

	// Обратная сторона: измеренная фаза 2 обязана показывать миллисекунды.
	diag.Phase2 = sweepTimingSentinelResults()
	diag.Phase2Dur = 300 * time.Millisecond
	line = formatAutoSweepTimingLine("🚀 impVPN Auto", 48, diag)
	if !strings.Contains(line, "фаза 2 300ms") {
		t.Fatalf("измеренная фаза 2 должна показывать длительность: %s", line)
	}
}
