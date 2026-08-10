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

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/proxy"
)

func TestFormatAutoMemberTable_ListsEveryMemberWithRTTAndReason(t *testing.T) {
	rows := []autoMemberProbe{
		{Name: "DE #1", Addr: "1.2.3.4:443", Type: "VLESS", RTTms: 42, OK: true},
		{Name: "RU #2", Addr: "5.6.7.8:443", Type: "TROJAN", RTTms: 1, OK: true},
		{Name: "NL #3", Addr: "9.9.9.9:443", Type: "VLESS", OK: false, Reason: "timeout"},
	}

	got := formatAutoMemberTable("impVPN Auto", rows)

	if len(got) != 4 {
		t.Fatalf("ожидали заголовок + 3 строки, получили %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "impVPN Auto") || !strings.Contains(got[0], "3") {
		t.Errorf("заголовок должен называть группу и число членов, получили %q", got[0])
	}
	if !strings.Contains(got[2], "RU #2") || !strings.Contains(got[2], "1ms") {
		t.Errorf("строка члена должна содержать имя и RTT, получили %q", got[2])
	}
	if !strings.Contains(got[3], "timeout") {
		t.Errorf("недоступный член должен показывать reason, получили %q", got[3])
	}
}

func TestFormatAutoMemberTable_EmptyMembersStillReportsGroup(t *testing.T) {
	got := formatAutoMemberTable("Auto", nil)
	if len(got) != 1 {
		t.Fatalf("ожидали только заголовок, получили %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "0") {
		t.Errorf("заголовок должен сообщать 0 членов, получили %q", got[0])
	}
}

// TestFormatAutoMemberTable_MarksTLSStageFailureDistinctly is the regression
// test for finding #4: a member that dies specifically at the TLS handshake
// (Stage=="tls", the SNI/DPI-blocking signature — see
// autoProbeTLSParams/probeOne) must read differently from a plain connect
// failure, so the pending manual verification step this table exists for can
// tell "never even connected" apart from "connected fine, TLS killed it".
func TestFormatAutoMemberTable_MarksTLSStageFailureDistinctly(t *testing.T) {
	rows := []autoMemberProbe{
		{Name: "A", Addr: "1.1.1.1:443", Type: "VLESS", OK: false, Stage: "tls", Reason: "timeout"},
		{Name: "B", Addr: "2.2.2.2:443", Type: "VLESS", OK: false, Stage: "tcp", Reason: "timeout"},
	}

	got := formatAutoMemberTable("Auto", rows)
	if len(got) != 3 {
		t.Fatalf("ожидали заголовок + 2 строки, получили %d: %v", len(got), got)
	}
	if !strings.Contains(got[1], "TLS") {
		t.Errorf("отказ на TLS-этапе должен быть явно помечен, получили %q", got[1])
	}
	if strings.Contains(got[2], "TLS") {
		t.Errorf("обычный отказ на TCP-этапе не должен упоминать TLS, получили %q", got[2])
	}
}

// TestBuildAutoMemberRows_Phase2VerdictOverridesPhase1 is the regression test
// for finding #4: ResolveAutoCandidates used to feed the diagnostic table
// from phase 1 (DepthFast, TCP-only) alone. A member that passes phase 1 and
// then dies at phase 2's TLS handshake (DepthFull, shortlist only — the
// exact fallout of finding #1's bug) showed up as a healthy "NNms" row while
// being silently absent from RankAutoCandidates' output. buildAutoMemberRows
// must show phase 2's verdict — the FINAL one — for any member phase 2
// actually re-probed, and leave phase 1's verdict alone for members phase 2
// never touched.
func TestBuildAutoMemberRows_Phase2VerdictOverridesPhase1(t *testing.T) {
	members := []config.ProxyEntry{
		{ID: "a", Name: "A", IP: "1.1.1.1", Port: 443, Type: "VLESS"},
		{ID: "b", Name: "B", IP: "2.2.2.2", Port: 443, Type: "VLESS"},
	}
	keyA, keyB := proxy.AutoNodeKey(members[0]), proxy.AutoNodeKey(members[1])

	phase1 := []proxy.AutoProbeResult{
		{Key: keyA, OK: true, RTTms: 30, Stage: "tcp"},
		{Key: keyB, OK: true, RTTms: 40, Stage: "tcp"},
	}
	// A passes TCP in phase 1 but dies on TLS in phase 2. B is never
	// re-probed (e.g. it fell outside the shortlist).
	phase2 := []proxy.AutoProbeResult{
		{Key: keyA, OK: false, Stage: "tls", Reason: "timeout"},
	}

	rows := buildAutoMemberRows(members, phase1, phase2)

	if len(rows) != 2 {
		t.Fatalf("ожидали 2 строки, получили %d: %+v", len(rows), rows)
	}
	if rows[0].OK || rows[0].Stage != "tls" || rows[0].Reason != "timeout" {
		t.Errorf("строка A должна отражать отказ фазы 2 на TLS, а не успех фазы 1: %+v", rows[0])
	}
	if !rows[1].OK || rows[1].RTTms != 40 {
		t.Errorf("строка B (фаза 2 её не трогала) должна сохранить вердикт фазы 1: %+v", rows[1])
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
