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

package mobile

import (
	"encoding/json"
	"strings"
	"testing"

	"resultproxy-wails/internal/proxy"
)

func autoEnvelope(t *testing.T) string {
	t.Helper()
	// Two unroutable members: TEST-NET-3 addresses never answer, so the sweep
	// finishes on timeouts and returns no candidates — which is exactly the
	// shape the caller must handle, and it keeps the test off the network.
	return `{"type":"AUTO","name":"grp","extra":{"members":[
		{"ip":"203.0.113.9","port":443,"type":"VLESS"},
		{"ip":"203.0.113.10","port":443,"type":"VLESS"}]}}`
}

func TestResolveAutoCandidatesReturnsWellFormedJSON(t *testing.T) {
	raw, err := ResolveAutoCandidates(autoEnvelope(t), t.TempDir(), 2000, false)
	if err != nil {
		t.Fatalf("ResolveAutoCandidates: %v", err)
	}
	var got struct {
		Candidates []struct {
			Key   string          `json:"key"`
			Name  string          `json:"name"`
			RTTms int64           `json:"rttMs"`
			Entry json.RawMessage `json:"entry"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("payload is not valid JSON: %v (%s)", err, raw)
	}
	// Unreachable members yield an empty list, never a nil field the Kotlin
	// parser would trip over.
	if !strings.Contains(raw, `"candidates"`) {
		t.Fatalf("payload must always carry a candidates array, got %s", raw)
	}
}

func TestResolveAutoCandidatesRejectsNonAutoEntry(t *testing.T) {
	_, err := ResolveAutoCandidates(`{"type":"VLESS","ip":"203.0.113.9","port":443}`, t.TempDir(), 2000, false)
	if err == nil {
		t.Fatal("a non-AUTO entry must be rejected, not silently resolved")
	}
}

// TestResolveAutoCandidatesRestoresKeyedWGGate pins the "restore afterwards"
// half of Fix 2: ResolveAutoCandidates overrides proxy's keyed-WireGuard-probe
// gate for the duration of its own sweep, and must put back whatever value it
// found — not the value implied by its own engineRunning argument — so a
// concurrent caller of SetAutoKeyedWGProbe is never overwritten by a sweep
// that has nothing to do with it.
func TestResolveAutoCandidatesRestoresKeyedWGGate(t *testing.T) {
	prev := proxy.AutoKeyedWGProbeAllowed()
	t.Cleanup(func() { proxy.SetAutoKeyedWGProbe(prev) })

	for _, before := range []bool{true, false} {
		proxy.SetAutoKeyedWGProbe(before)
		// engineRunning is the opposite of `before` on purpose: if restore were
		// buggy and just re-derived the value from engineRunning instead of
		// snapshotting the prior state, this would still coincidentally pass
		// when the two happen to agree.
		if _, err := ResolveAutoCandidates(autoEnvelope(t), t.TempDir(), 500, !before); err != nil {
			t.Fatalf("ResolveAutoCandidates: %v", err)
		}
		if got := proxy.AutoKeyedWGProbeAllowed(); got != before {
			t.Fatalf("gate not restored: started at %v, got %v after resolve", before, got)
		}
	}
}

func TestRecordAutoConnectOutcomeSurvivesUninitialisedStore(t *testing.T) {
	// The JSON test above resolves a group and, via InitAutoStats, leaves a
	// real store installed for the package's lifetime. Force a truly nil
	// store here so the lazy-init branch in proxy.nodeStats is what actually
	// runs, then restore what was there so later tests in this package are
	// not affected by this test's own manipulation.
	autoStatsOnce.Lock()
	oldDataDir := autoStatsOnce.dataDir
	autoStatsOnce.Unlock()
	defer func() {
		if oldDataDir == "" {
			proxy.SetNodeStatStore(nil)
			return
		}
		proxy.SetNodeStatStore(proxy.NewNodeStatStore(oldDataDir))
	}()
	proxy.SetNodeStatStore(nil)

	// A Kotlin caller can report an outcome before any resolve has run — a
	// reload path, say — and that must not panic.
	RecordAutoConnectOutcome("deadbeef", false, "timeout")
}
