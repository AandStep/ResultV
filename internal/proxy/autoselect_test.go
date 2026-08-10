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
		})
	}
	return out
}

func TestRankAutoCandidates_OrdersByRTTAndCapsAtFive(t *testing.T) {
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
	oldTCP := pingTCPProbe
	defer func() { pingTCPProbe = oldTCP }()
	pingTCPProbe = func(ip string, _ int) (int64, bool, string) { return 0, false, "timeout: " + ip }

	got, phase1 := RankAutoCandidates(context.Background(), mkNodes("a", "b"), "")
	if got != nil {
		t.Fatalf("ожидали nil кандидатов, получили %+v", got)
	}
	if len(phase1) != 2 {
		t.Fatalf("ожидали 2 строки диагностики фазы 1 даже без доступных узлов, получили %d: %+v", len(phase1), phase1)
	}
	for _, r := range phase1 {
		if r.OK {
			t.Errorf("узел %q помечен как OK, хотя ping должен был провалиться", r.Key)
		}
		if r.Reason == "" {
			t.Errorf("узел %q: пустая причина отказа в диагностике фазы 1", r.Key)
		}
	}
}

func TestRankAutoCandidates_IncludesPreviousPickInShortlistEvenIfSlow(t *testing.T) {
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

func TestRankAutoCandidates_StaysOnCurrentPickWithinTolerance(t *testing.T) {
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
