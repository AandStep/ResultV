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
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/proxy"
)

// autoCandidate is one ranked member. Entry is the marshaled ProxyEntry the
// caller feeds straight back to BuildSingBoxConfigFromEntryV2 — Kotlin never
// rebuilds it, so a candidate cannot drift from what was measured.
type autoCandidate struct {
	Key      string          `json:"key"`
	Name     string          `json:"name"`
	RTTms    int64           `json:"rttMs"`
	RTTKnown bool            `json:"rttKnown"`
	Entry    json.RawMessage `json:"entry"`
}

type autoResolveResult struct {
	Candidates []autoCandidate `json:"candidates"`
}

var autoStatsOnce struct {
	sync.Mutex
	dataDir string
}

// InitAutoStats points the node-statistics store at the app's data dir. Safe to
// call repeatedly; only the first call for a given dir does any work. Kotlin
// calls it during service init so a store is in place before any connect.
func InitAutoStats(dataDir string) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return
	}
	autoStatsOnce.Lock()
	defer autoStatsOnce.Unlock()
	if autoStatsOnce.dataDir == dataDir {
		return
	}
	proxy.SetNodeStatStore(proxy.NewNodeStatStore(dataDir))
	autoStatsOnce.dataDir = dataDir
}

// ResolveAutoCandidates probes an AUTO group and returns its members best-first
// as JSON: {"candidates":[{"key","name","rttMs","entry"},...]}.
//
// The list is empty when nothing in the group is reachable; the caller then
// falls back to the group head rather than failing the connect outright — a
// probe sweep that saw nothing is not proof the tunnel would not come up.
//
// timeoutMs bounds the whole sweep. On mobile this is on the connect critical
// path, so the caller passes a budget rather than letting a group of dead nodes
// hold the connect for the sum of their timeouts.
func ResolveAutoCandidates(entryJSON, dataDir string, timeoutMs int) (string, error) {
	var entry config.ProxyEntry
	if err := json.Unmarshal([]byte(entryJSON), &entry); err != nil {
		return "", fmt.Errorf("resolve auto: parsing entry: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(entry.Type), "AUTO") {
		return "", fmt.Errorf("resolve auto: entry is %q, not AUTO", entry.Type)
	}
	members, err := decodeAutoMembers(entry.Extra)
	if err != nil {
		return "", fmt.Errorf("resolve auto: members: %w", err)
	}
	if len(members) == 0 {
		return "", fmt.Errorf("resolve auto: group has no members")
	}
	InitAutoStats(dataDir)

	if timeoutMs <= 0 {
		timeoutMs = 15000
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	ranked, diag := proxy.RankAutoCandidates(ctx, members, previousAutoKey())

	// candidateRTT pairs the millisecond figure with whether it is a real
	// measurement, so the two cannot drift apart as they are seeded and
	// re-seeded below (a bare map[string]int64 would silently drop RTTKnown).
	type candidateRTT struct {
		ms    int64
		known bool
	}
	rtt := map[string]candidateRTT{}
	// Defensive seed, not the decisive value: every candidate
	// RankAutoCandidates actually returns carries a successful phase-2 entry
	// (internal/proxy/autoselect.go:281-334 only forwards OK results), so this
	// phase-1 pass only matters if that contract ever loosens to let a
	// phase-1-only result through.
	for _, r := range diag.Phase1 {
		rtt[r.Key] = candidateRTT{ms: r.RTTms, known: r.RTTKnown}
	}
	// The figure that actually reaches the caller for every returned
	// candidate today.
	for _, r := range diag.Phase2 {
		if r.OK {
			rtt[r.Key] = candidateRTT{ms: r.RTTms, known: r.RTTKnown}
		}
	}

	out := autoResolveResult{Candidates: make([]autoCandidate, 0, len(ranked))}
	for _, m := range ranked {
		raw, err := json.Marshal(m)
		if err != nil {
			continue
		}
		key := proxy.AutoNodeKey(m)
		out.Candidates = append(out.Candidates, autoCandidate{
			Key:      key,
			Name:     m.Name,
			RTTms:    rtt[key].ms,
			RTTKnown: rtt[key].known,
			Entry:    raw,
		})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("resolve auto: marshaling result: %w", err)
	}
	return string(b), nil
}

// lastAutoKey is the key of the node whose last connect actually succeeded
// (set only by RecordAutoConnectOutcome, never by ResolveAutoCandidates
// itself), fed back into the next ranking as previousKey so the hysteresis
// head start applies to the node actually in use.
var lastAutoKey struct {
	sync.Mutex
	key string
}

func previousAutoKey() string {
	lastAutoKey.Lock()
	defer lastAutoKey.Unlock()
	return lastAutoKey.key
}

// RecordAutoConnectOutcome folds the result of a real connect attempt into the
// node's history — the strongest signal the store has, and the one that makes a
// node which will not connect sink regardless of how fast it probes.
func RecordAutoConnectOutcome(key string, ok bool, reason string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	if ok {
		lastAutoKey.Lock()
		lastAutoKey.key = key
		lastAutoKey.Unlock()
	}
	proxy.RecordConnectOutcome(key, ok, reason)
}
