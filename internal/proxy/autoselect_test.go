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
	"encoding/json"
	"sync"
	"testing"
	"time"

	"resultproxy-wails/internal/config"
)

func mkNodes(ips ...string) []config.ProxyEntry {
	out := make([]config.ProxyEntry, 0, len(ips))
	for _, ip := range ips {
		out = append(out, config.ProxyEntry{
			ID: ip, IP: ip, Port: 443, Type: "VLESS",
			Name: ip, SubscriptionURL: "https://sub.example",
			// security=tls, not an empty Extra: a real VLESS node parsed
			// from a URI always carries an explicit security value
			// (parseVLESSURI defaults it to "none" — never absent), so an
			// empty Extra here would be unrepresentative fixture data.
			// These ranking tests need DepthFull's TLS stage to actually
			// run (that's where autoTLSProbe below is consulted), which
			// requires wantTLS=true per autoProbeTLSParams/autoProbeWantsTLS.
			Extra: json.RawMessage(`{"security":"tls"}`),
		})
	}
	return out
}

func TestRankAutoCandidates_OrdersByRTTAndCapsAtFive(t *testing.T) {
	oldBind := autoProbeBindsToLAN
	defer func() { autoProbeBindsToLAN = oldBind }()
	autoProbeBindsToLAN = func() bool { return false }

	oldTCP, oldTLS := pingTCPProbe, autoTLSProbe
	defer func() { pingTCPProbe, autoTLSProbe = oldTCP, oldTLS }()

	rtt := map[string]int64{
		"a": 90, "b": 10, "c": 50, "d": 20, "e": 70, "f": 30, "g": 40,
	}
	pingTCPProbe = func(ip string, _ int) (int64, bool, string) { return rtt[ip], true, "" }
	autoTLSProbe = func(ip string, _ int, _ string, _ []string) (int64, bool, string) {
		return rtt[ip], true, ""
	}

	got, _ := RankAutoCandidates(context.Background(),
		mkNodes("a", "b", "c", "d", "e", "f", "g"), "")

	if len(got) != AutoMaxCandidates {
		t.Fatalf("ожидали не более %d кандидатов, получили %d", AutoMaxCandidates, len(got))
	}
	if got[0].IP != "b" {
		t.Errorf("быстрейший узел должен быть первым, получили %q", got[0].IP)
	}
	if got[1].IP != "d" {
		t.Errorf("вторым ожидали d (20ms), получили %q", got[1].IP)
	}
}

func TestRankAutoCandidates_DropsUnreachableNodes(t *testing.T) {
	// DepthFull (phase 2) probes the shortlist with a real TLS handshake for
	// any type not in autoProbeTLSParams' no-TLS list — VLESS (mkNodes'
	// default) needs one. Without mocking autoTLSProbe too, "live" would hit
	// the real network via an unresolvable host and be dropped as a false
	// negative, which is not what this test is checking.
	oldBind := autoProbeBindsToLAN
	defer func() { autoProbeBindsToLAN = oldBind }()
	autoProbeBindsToLAN = func() bool { return false }

	oldTCP, oldTLS := pingTCPProbe, autoTLSProbe
	defer func() { pingTCPProbe, autoTLSProbe = oldTCP, oldTLS }()

	pingTCPProbe = func(ip string, _ int) (int64, bool, string) {
		if ip == "dead" {
			return 0, false, "timeout"
		}
		return 30, true, ""
	}
	autoTLSProbe = func(ip string, _ int, _ string, _ []string) (int64, bool, string) {
		return 30, true, ""
	}

	got, _ := RankAutoCandidates(context.Background(), mkNodes("dead", "live"), "")

	if len(got) != 1 || got[0].IP != "live" {
		t.Fatalf("недоступный узел не должен попадать в кандидаты, получили %+v", got)
	}
}

func TestRankAutoCandidates_NoReachableNodesReturnsNil(t *testing.T) {
	oldBind := autoProbeBindsToLAN
	defer func() { autoProbeBindsToLAN = oldBind }()
	autoProbeBindsToLAN = func() bool { return false }

	oldTCP := pingTCPProbe
	defer func() { pingTCPProbe = oldTCP }()
	pingTCPProbe = func(_ string, _ int) (int64, bool, string) { return 0, false, "timeout" }

	if got, _ := RankAutoCandidates(context.Background(), mkNodes("a", "b"), ""); got != nil {
		t.Fatalf("ожидали nil когда живых узлов нет, получили %+v", got)
	}
}

func TestRankAutoCandidates_ReturnsPhase1EvenWhenNoneReachable(t *testing.T) {
	// This is the diagnostic case that matters most: a dead AUTO group is
	// exactly when the caller's per-member RTT/reason table needs data, so
	// phase1 must not come back empty just because candidates did.
	oldBind := autoProbeBindsToLAN
	defer func() { autoProbeBindsToLAN = oldBind }()
	autoProbeBindsToLAN = func() bool { return false }

	oldTCP := pingTCPProbe
	defer func() { pingTCPProbe = oldTCP }()
	pingTCPProbe = func(ip string, _ int) (int64, bool, string) { return 0, false, "timeout: " + ip }

	got, diag := RankAutoCandidates(context.Background(), mkNodes("a", "b"), "")
	if got != nil {
		t.Fatalf("ожидали nil кандидатов, получили %+v", got)
	}
	if len(diag.Phase1) != 2 {
		t.Fatalf("ожидали 2 строки диагностики фазы 1 даже без доступных узлов, получили %d: %+v", len(diag.Phase1), diag.Phase1)
	}
	for _, r := range diag.Phase1 {
		if r.OK {
			t.Errorf("узел %q помечен как OK, хотя ping должен был провалиться", r.Key)
		}
		if r.Reason == "" {
			t.Errorf("узел %q: пустая причина отказа в диагностике фазы 1", r.Key)
		}
	}
}

func TestRankAutoCandidates_IncludesPreviousPickInShortlistEvenIfSlow(t *testing.T) {
	oldBind := autoProbeBindsToLAN
	defer func() { autoProbeBindsToLAN = oldBind }()
	autoProbeBindsToLAN = func() bool { return false }

	oldTCP, oldTLS := pingTCPProbe, autoTLSProbe
	defer func() { pingTCPProbe, autoTLSProbe = oldTCP, oldTLS }()

	rtt := map[string]int64{"a": 10, "b": 20, "c": 30, "d": 40, "e": 50, "slow": 900}
	pingTCPProbe = func(ip string, _ int) (int64, bool, string) { return rtt[ip], true, "" }

	// ProbeAutoNodes runs the shortlist through a bounded worker pool, so this
	// closure is invoked from multiple goroutines concurrently. Go maps are not
	// safe for concurrent writes even to distinct keys (the runtime detects the
	// write-write race and calls fatal, which -race or not aborts the whole test
	// binary) — a mutex is required, not just good practice.
	var fullProbedMu sync.Mutex
	fullProbed := map[string]bool{}
	autoTLSProbe = func(ip string, _ int, _ string, _ []string) (int64, bool, string) {
		fullProbedMu.Lock()
		fullProbed[ip] = true
		fullProbedMu.Unlock()
		return rtt[ip], true, ""
	}

	nodes := mkNodes("a", "b", "c", "d", "e", "slow")
	prev := AutoNodeKey(nodes[5])

	RankAutoCandidates(context.Background(), nodes, prev)

	if !fullProbed["slow"] {
		t.Error("прошлый выбор должен попадать в фазу 2 даже если он вне топ-5")
	}
}

func TestRankAutoCandidates_RecordsProbeResultsInStore(t *testing.T) {
	oldBind := autoProbeBindsToLAN
	defer func() { autoProbeBindsToLAN = oldBind }()
	autoProbeBindsToLAN = func() bool { return false }

	oldTCP, oldTLS, oldStore := pingTCPProbe, autoTLSProbe, nodeStats()
	defer func() {
		pingTCPProbe, autoTLSProbe = oldTCP, oldTLS
		SetNodeStatStore(oldStore)
	}()

	SetNodeStatStore(NewNodeStatStore(t.TempDir()))
	pingTCPProbe = func(ip string, _ int) (int64, bool, string) {
		if ip == "dead" {
			return 0, false, "timeout"
		}
		return 60, true, ""
	}
	autoTLSProbe = func(_ string, _ int, _ string, _ []string) (int64, bool, string) {
		return 60, true, ""
	}

	nodes := mkNodes("live", "dead")
	RankAutoCandidates(context.Background(), nodes, "")

	live := nodeStats().Get(AutoNodeKey(nodes[0]))
	if live.EWMARTTms == 0 {
		t.Error("успешная проба должна попадать в стор")
	}
	dead := nodeStats().Get(AutoNodeKey(nodes[1]))
	if dead.LastReason != "timeout" {
		t.Errorf("неудачная проба должна сохранять reason, получили %q", dead.LastReason)
	}
}

// TestDropEmptyKeyResults_RemovesOnlyZeroValueSlots is a focused unit test of
// the helper RankAutoCandidates uses to filter cancelled probe slots (see
// ProbeAutoNodes: a slot the worker pool never reached is left at its zero
// value, whose Key is ""). It must remove exactly the empty-Key entries and
// nothing else, preserving both OK and failed real results.
func TestDropEmptyKeyResults_RemovesOnlyZeroValueSlots(t *testing.T) {
	in := []AutoProbeResult{
		{Key: "a", OK: true, RTTms: 10},
		{}, // zero-value: a cancelled slot
		{Key: "b", OK: false, Reason: "timeout"},
	}

	got := dropEmptyKeyResults(in)

	if len(got) != 2 {
		t.Fatalf("ожидали 2 результата после фильтрации, получили %d: %+v", len(got), got)
	}
	for _, r := range got {
		if r.Key == "" {
			t.Errorf("запись с пустым Key пережила фильтрацию: %+v", r)
		}
	}
}

// TestRankAutoCandidates_CancelledProbesAreNotRecordedUnderEmptyKey is the
// regression test for the finding: probes skipped because ctx was already
// cancelled used to be recorded under the empty key "" (nodeStats().
// RecordProbe("", ...)), writing a permanent junk entry to node_stats.json
// and surfacing as a meaningless "— :0" row in the diagnostic table built
// from phase1/diag.Phase1. Passing an already-cancelled context makes every
// probe slot arrive at its zero value (see
// TestProbeAutoNodes_CancelledContextLeavesZeroValueSlotsWithEmptyKey), which
// is exactly the scenario dropEmptyKeyResults exists to filter before either
// recording or returning.
func TestRankAutoCandidates_CancelledProbesAreNotRecordedUnderEmptyKey(t *testing.T) {
	oldBind := autoProbeBindsToLAN
	defer func() { autoProbeBindsToLAN = oldBind }()
	autoProbeBindsToLAN = func() bool { return false }

	oldTCP, oldStore := pingTCPProbe, nodeStats()
	defer func() {
		pingTCPProbe = oldTCP
		SetNodeStatStore(oldStore)
	}()
	SetNodeStatStore(NewNodeStatStore(t.TempDir()))

	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		t.Error("проба не должна вызываться при уже отменённом контексте")
		return 0, false, ""
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, diag := RankAutoCandidates(ctx, mkNodes("a", "b", "c"), "")

	if got != nil {
		t.Fatalf("ожидали nil кандидатов, получили %+v", got)
	}
	for _, r := range diag.Phase1 {
		if r.Key == "" {
			t.Fatalf("diag.Phase1 содержит запись с пустым ключом: %+v", diag.Phase1)
		}
	}
	if st := nodeStats().Get(""); st != (NodeStat{}) {
		t.Errorf("под пустым ключом не должно быть записи в сторе, получили %+v", st)
	}
}

func TestScoreNode_PenalisesJitterOverRawLatency(t *testing.T) {
	now := time.Now()
	steady := scoreNode(AutoProbeResult{RTTms: 60, JitterMs: 2}, NodeStat{}, false, 1.0, now)
	jumpy := scoreNode(AutoProbeResult{RTTms: 50, JitterMs: 40}, NodeStat{}, false, 1.0, now)

	if jumpy <= steady {
		t.Errorf("нестабильный узел (50ms±40) не должен обыгрывать стабильный (60ms±2): %v vs %v", jumpy, steady)
	}
}

func TestScoreNode_PenalisesConsecutiveFailuresAndCapsAtThree(t *testing.T) {
	now := time.Now()
	clean := scoreNode(AutoProbeResult{RTTms: 100}, NodeStat{}, false, 1.0, now)
	three := scoreNode(AutoProbeResult{RTTms: 100}, NodeStat{ConsecFails: 3}, false, 1.0, now)
	ten := scoreNode(AutoProbeResult{RTTms: 100}, NodeStat{ConsecFails: 10}, false, 1.0, now)

	if three <= clean {
		t.Error("серия отказов должна ухудшать скор")
	}
	if three != ten {
		t.Errorf("штраф должен упираться в потолок на 3 отказах: %v vs %v", three, ten)
	}
}

func TestScoreNode_RecentConnectFailurePenaltyExpires(t *testing.T) {
	now := time.Now()
	fresh := scoreNode(AutoProbeResult{RTTms: 100}, NodeStat{LastFailAt: now.Add(-1 * time.Minute)}, false, 1.0, now)
	stale := scoreNode(AutoProbeResult{RTTms: 100}, NodeStat{LastFailAt: now.Add(-30 * time.Minute)}, false, 1.0, now)

	if fresh <= stale {
		t.Error("свежий провал должен штрафоваться сильнее старого")
	}
}

func TestScoreNode_CurrentPickGetsToleranceCredit(t *testing.T) {
	now := time.Now()
	current := scoreNode(AutoProbeResult{RTTms: 100}, NodeStat{}, true, 1.0, now)
	other := scoreNode(AutoProbeResult{RTTms: 100}, NodeStat{}, false, 1.0, now)

	if other-current != autoTolerance {
		t.Errorf("текущий выбор должен получать кредит ровно в autoTolerance, разница %v", other-current)
	}
}

// TestScoreNode_ToleranceCreditIsExactlyFifty pins autoTolerance's effect to a
// concrete number instead of comparing against the constant's own name (as
// TestScoreNode_CurrentPickGetsToleranceCredit does). That symmetry check
// stays green no matter how absurd autoTolerance becomes — e.g. a comparator
// bug of `if isCurrent { score -= 1e18 }` would still satisfy "other minus
// current equals autoTolerance" if autoTolerance were redefined to 1e18. This
// test asserts the actual arithmetic against a literal, so it catches both an
// absurd constant and a dropped tolerance credit.
func TestScoreNode_ToleranceCreditIsExactlyFifty(t *testing.T) {
	if autoTolerance != 50 {
		t.Fatalf("тест ожидает autoTolerance=50, но константа равна %v — обновите литерал в тесте, если допуск изменён намеренно", autoTolerance)
	}

	now := time.Now()
	const rtt = 100.0
	got := scoreNode(AutoProbeResult{RTTms: rtt}, NodeStat{}, true, 1.0, now)
	want := rtt - 50.0

	if got != want {
		t.Errorf("текущий выбор при RTT=%vms и пустой статистике должен получить скор %v, получили %v", rtt, want, got)
	}
}

func TestRankAutoCandidates_StaysOnCurrentPickWithinTolerance(t *testing.T) {
	oldBind := autoProbeBindsToLAN
	defer func() { autoProbeBindsToLAN = oldBind }()
	autoProbeBindsToLAN = func() bool { return false }

	oldTCP, oldTLS, oldStore := pingTCPProbe, autoTLSProbe, nodeStats()
	defer func() {
		pingTCPProbe, autoTLSProbe = oldTCP, oldTLS
		SetNodeStatStore(oldStore)
	}()
	SetNodeStatStore(NewNodeStatStore(t.TempDir()))

	rtt := map[string]int64{"cur": 100, "new": 70}
	pingTCPProbe = func(ip string, _ int) (int64, bool, string) { return rtt[ip], true, "" }
	autoTLSProbe = func(ip string, _ int, _ string, _ []string) (int64, bool, string) {
		return rtt[ip], true, ""
	}

	nodes := mkNodes("cur", "new")
	got, _ := RankAutoCandidates(context.Background(), nodes, AutoNodeKey(nodes[0]))

	if got[0].IP != "cur" {
		t.Errorf("узел быстрее лишь на 30ms при tolerance=50 не должен вытеснять текущий, получили %q", got[0].IP)
	}
}

// TestRankAutoCandidates_SwitchesToClearlyFasterRival is the mirror of
// TestRankAutoCandidates_StaysOnCurrentPickWithinTolerance: it proves the
// other half of hysteresis actually works — a rival that beats the current
// pick by far more than autoTolerance must take over the top rank. Without
// this test a comparator bug like `if isCurrent { score -= 1e18 }` (making
// the current pick effectively unbeatable forever) would pass the whole
// package: nothing else asserts AUTO ever moves off a node once picked.
func TestNodeClass_ClassifiesByProtocolAndTransport(t *testing.T) {
	cases := []struct {
		name string
		e    config.ProxyEntry
		want string
	}{
		{"reality", config.ProxyEntry{Type: "VLESS", Extra: []byte(`{"security":"reality"}`)}, "obfs"},
		// Transport is stored under "network" (parseVLESSURI, outbound.go),
		// never under "type" — "type" is only the URI query parameter name.
		{"xhttp", config.ProxyEntry{Type: "VLESS", Extra: []byte(`{"network":"xhttp"}`)}, "obfs"},
		{"grpc", config.ProxyEntry{Type: "VLESS", Extra: []byte(`{"network":"grpc"}`)}, "obfs"},
		// A public key with no explicit security is still Reality — the engine
		// (outbound.go's applyTLSAndTransport) auto-detects it that way too.
		{"pbk without explicit security", config.ProxyEntry{Type: "VLESS", Extra: []byte(`{"pbk":"REbCbLiQwWzmUHZgBc-oCO0CMtwgvWURtWkFjNfcQkk"}`)}, "obfs"},
		// Hysteria2 obfuscation is always a flat string under "obfs_type"
		// (parseHysteria2URI); there is no object-shaped "obfs" key any real
		// producer writes.
		{"hysteria2 obfs", config.ProxyEntry{Type: "HYSTERIA2", Extra: []byte(`{"obfs_type":"salamander"}`)}, "obfs"},
		{"hysteria2 no obfs", config.ProxyEntry{Type: "HYSTERIA2"}, "plain"},
		{"plain vless tls", config.ProxyEntry{Type: "VLESS", Extra: []byte(`{"security":"tls"}`)}, "plain"},
		{"trojan", config.ProxyEntry{Type: "TROJAN"}, "plain"},
		{"shadowsocks", config.ProxyEntry{Type: "SS"}, "plain"},
	}
	for _, c := range cases {
		if got := nodeClass(c.e); got != c.want {
			t.Errorf("%s: ожидали %q, получили %q", c.name, c.want, got)
		}
	}
}

func TestDetectClassWeights_PenalisesPlainWhenPlainIsBeingCut(t *testing.T) {
	members := []config.ProxyEntry{
		{IP: "p1", Port: 443, Type: "TROJAN"},
		{IP: "p2", Port: 443, Type: "TROJAN"},
		{IP: "p3", Port: 443, Type: "TROJAN"},
		{IP: "o1", Port: 443, Type: "VLESS", Extra: []byte(`{"security":"reality"}`)},
		{IP: "o2", Port: 443, Type: "VLESS", Extra: []byte(`{"security":"reality"}`)},
		{IP: "o3", Port: 443, Type: "VLESS", Extra: []byte(`{"security":"reality"}`)},
	}
	fast := []AutoProbeResult{
		{Key: AutoNodeKey(members[0]), OK: false},
		{Key: AutoNodeKey(members[1]), OK: false},
		{Key: AutoNodeKey(members[2]), OK: false},
		{Key: AutoNodeKey(members[3]), OK: true},
		{Key: AutoNodeKey(members[4]), OK: true},
		{Key: AutoNodeKey(members[5]), OK: true},
	}

	w := detectClassWeights(members, fast)

	if w["plain"] != autoBlockedClassPenalty {
		t.Errorf("простые узлы мертвы, обфусцированные живы — ожидали штраф %v, получили %v",
			autoBlockedClassPenalty, w["plain"])
	}
	if w["obfs"] != 1.0 {
		t.Errorf("обфусцированные узлы не должны штрафоваться, получили %v", w["obfs"])
	}
}

func TestDetectClassWeights_NoPenaltyWhenBothClassesHealthy(t *testing.T) {
	members := []config.ProxyEntry{
		{IP: "p1", Port: 443, Type: "TROJAN"},
		{IP: "p2", Port: 443, Type: "TROJAN"},
		{IP: "p3", Port: 443, Type: "TROJAN"},
		{IP: "o1", Port: 443, Type: "VLESS", Extra: []byte(`{"security":"reality"}`)},
	}
	fast := []AutoProbeResult{
		{Key: AutoNodeKey(members[0]), OK: true},
		{Key: AutoNodeKey(members[1]), OK: true},
		{Key: AutoNodeKey(members[2]), OK: true},
		{Key: AutoNodeKey(members[3]), OK: true},
	}

	w := detectClassWeights(members, fast)
	if w["plain"] != 1.0 || w["obfs"] != 1.0 {
		t.Errorf("оба класса живы — штрафов быть не должно, получили %v", w)
	}
}

func TestDetectClassWeights_TooFewPlainNodesIsNotEvidence(t *testing.T) {
	members := []config.ProxyEntry{
		{IP: "p1", Port: 443, Type: "TROJAN"},
		{IP: "o1", Port: 443, Type: "VLESS", Extra: []byte(`{"security":"reality"}`)},
		{IP: "o2", Port: 443, Type: "VLESS", Extra: []byte(`{"security":"reality"}`)},
	}
	fast := []AutoProbeResult{
		{Key: AutoNodeKey(members[0]), OK: false},
		{Key: AutoNodeKey(members[1]), OK: true},
		{Key: AutoNodeKey(members[2]), OK: true},
	}

	w := detectClassWeights(members, fast)
	if w["plain"] != 1.0 {
		t.Errorf("один мёртвый простой узел — статистически не вывод, ожидали 1.0, получили %v", w["plain"])
	}
}

func TestRankAutoCandidates_SwitchesToClearlyFasterRival(t *testing.T) {
	oldBind := autoProbeBindsToLAN
	defer func() { autoProbeBindsToLAN = oldBind }()
	autoProbeBindsToLAN = func() bool { return false }

	oldTCP, oldTLS, oldStore := pingTCPProbe, autoTLSProbe, nodeStats()
	defer func() {
		pingTCPProbe, autoTLSProbe = oldTCP, oldTLS
		SetNodeStatStore(oldStore)
	}()
	SetNodeStatStore(NewNodeStatStore(t.TempDir()))

	// Gap is 400ms, far more than the 50ms tolerance credit the current pick
	// gets, so "new" must win even after the credit is applied.
	rtt := map[string]int64{"cur": 500, "new": 100}
	pingTCPProbe = func(ip string, _ int) (int64, bool, string) { return rtt[ip], true, "" }
	autoTLSProbe = func(ip string, _ int, _ string, _ []string) (int64, bool, string) {
		return rtt[ip], true, ""
	}

	nodes := mkNodes("cur", "new")
	got, _ := RankAutoCandidates(context.Background(), nodes, AutoNodeKey(nodes[0]))

	if len(got) == 0 || got[0].IP != "new" {
		t.Errorf("узел быстрее текущего на 400ms (много больше tolerance=50) должен вытеснить текущий выбор, получили %+v", got)
	}
}

// TestRankAutoCandidates_ClassPenaltyChangesFinalOrder proves the class weight
// computed by detectClassWeights actually reaches scoreNode's classWeight
// argument inside RankAutoCandidates, not just detectClassWeights' own return
// map. Every other TestRankAutoCandidates_* test uses mkNodes(), which builds
// uniform VLESS nodes with no Extra — they all classify "plain", so
// total["obfs"] is always 0 and detectClassWeights always short-circuits to
// {1.0, 1.0} before ever reaching the ratio check. The scoring loop's
// weights[classByKey[r.Key]] lookup could be reverted to a literal 1.0 and
// every one of those tests would stay green. This test is the one that would
// catch that regression: it puts the plain node in front on raw RTT alone and
// checks the penalty flips the order.
func TestRankAutoCandidates_ClassPenaltyChangesFinalOrder(t *testing.T) {
	oldBind := autoProbeBindsToLAN
	defer func() { autoProbeBindsToLAN = oldBind }()
	autoProbeBindsToLAN = func() bool { return false }

	oldTCP, oldTLS, oldStore := pingTCPProbe, autoTLSProbe, nodeStats()
	defer func() {
		pingTCPProbe, autoTLSProbe = oldTCP, oldTLS
		SetNodeStatStore(oldStore)
	}()
	SetNodeStatStore(NewNodeStatStore(t.TempDir()))

	members := []config.ProxyEntry{
		{ID: "p1", IP: "p1", Port: 443, Type: "TROJAN", Name: "p1", SubscriptionURL: "https://sub.example"},
		{ID: "p2", IP: "p2", Port: 443, Type: "TROJAN", Name: "p2", SubscriptionURL: "https://sub.example"},
		{ID: "p3", IP: "p3", Port: 443, Type: "TROJAN", Name: "p3", SubscriptionURL: "https://sub.example"},
		{ID: "o1", IP: "o1", Port: 443, Type: "VLESS", Name: "o1", SubscriptionURL: "https://sub.example", Extra: []byte(`{"security":"reality"}`)},
		{ID: "o2", IP: "o2", Port: 443, Type: "VLESS", Name: "o2", SubscriptionURL: "https://sub.example", Extra: []byte(`{"security":"reality"}`)},
	}

	// Phase 1 (TCP connect): p2 and p3 are dead, so the plain class' survival
	// ratio is 1/3 (<0.5) while obfs' is 2/2 (>=0.5) — detectClassWeights'
	// exact trigger condition. p1's own phase-1 RTT does not matter for the
	// final score (DepthFull overwrites RTT/Jitter with the TLS samples
	// below) — it only needs to survive so it reaches phase 2 at all. Both
	// probes are mocked because TROJAN and VLESS both want a phase-2 TLS
	// handshake (see autoProbeTLSParams); leaving autoTLSProbe real would
	// dial the live network.
	tcpAlive := map[string]bool{"p1": true, "p2": false, "p3": false, "o1": true, "o2": true}
	pingTCPProbe = func(ip string, _ int) (int64, bool, string) {
		if tcpAlive[ip] {
			return 5, true, ""
		}
		return 0, false, "timeout"
	}

	// Phase 2 (TLS handshake, DepthFull, 3 samples averaged): p1 is genuinely
	// the fastest node (80ms), faster than either obfs node (100ms/110ms) —
	// but plain is the class currently reported as being cut. Unpenalised,
	// p1 (base 80) wins outright. Penalised 3x (autoBlockedClassPenalty), its
	// score is 240 — worse than both obfs nodes' unpenalised 100 and 110.
	tlsRTT := map[string]int64{"p1": 80, "o1": 100, "o2": 110}
	autoTLSProbe = func(ip string, _ int, _ string, _ []string) (int64, bool, string) {
		return tlsRTT[ip], true, ""
	}

	got, _ := RankAutoCandidates(context.Background(), members, "")

	if len(got) != 3 {
		t.Fatalf("ожидали 3 живых кандидата (p1, o1, o2), получили %d: %+v", len(got), got)
	}
	if got[0].IP != "o1" {
		t.Errorf("штраф x%v за вымирающий plain-класс должен вывести более медленный, но живой obfs-узел первым, получили %+v", autoBlockedClassPenalty, got)
	}
	if got[len(got)-1].IP != "p1" {
		t.Errorf("оштрафованный p1 (80ms*%v=%vms) должен проигрывать обоим obfs-узлам (100ms, 110ms без штрафа), получили %+v", autoBlockedClassPenalty, 80*autoBlockedClassPenalty, got)
	}
}
