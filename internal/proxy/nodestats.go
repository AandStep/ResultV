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
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Per-node health statistics, persisted across launches.
//
// The single most honest signal about a node is whether Connect actually
// succeeded, and until now that was thrown away: every connect re-probed from
// scratch and a node that failed three times yesterday could still win today on
// a 20ms head start. This store keeps that history so future selection can
// weigh it (this file only stores and reports, it does not score).
//
// Entries are keyed by AutoNodeKey (opaque hashed strings from autoprobe.go).
// This store never parses or interprets a key; it just remembers whatever
// string it is given.
//
// This file holds only opaque hashed keys and timings, so it carries no
// plaintext liability and is NOT encrypted — a promise `sanitizeStatReason`
// enforces.

const nodeStatsFile = "node_stats.json"

// nodeStatAlpha weights new samples in the EWMA. 0.3 tracks genuine route
// changes within a few probes while still damping one-off spikes.
const nodeStatAlpha = 0.3

// NodeStat is one node's rolling health record.
type NodeStat struct {
	EWMARTTms     float64   `json:"ewmaRttMs"`
	JitterMs      float64   `json:"jitterMs"`
	ConnectOK     int       `json:"connectOk"`
	ConnectFail   int       `json:"connectFail"`
	ConsecFails   int       `json:"consecFails"`
	LastSuccessAt time.Time `json:"lastSuccessAt"`
	LastFailAt    time.Time `json:"lastFailAt"`
	LastReason    string    `json:"lastReason"`
}

// NodeStatStore holds per-node stats in memory and persists them to a plain
// JSON file on explicit Flush. Safe for concurrent use.
type NodeStatStore struct {
	mu      sync.Mutex
	dataDir string
	stats   map[string]NodeStat
	dirty   bool
}

// NewNodeStatStore loads any existing stats from dataDir/node_stats.json (or
// starts empty if the file is missing or corrupt) and returns a ready store.
func NewNodeStatStore(dataDir string) *NodeStatStore {
	s := &NodeStatStore{dataDir: dataDir, stats: map[string]NodeStat{}}
	s.load()
	return s
}

func (s *NodeStatStore) path() string {
	return filepath.Join(s.dataDir, nodeStatsFile)
}

// load reads the stats file if present. Losing statistics is recoverable;
// refusing to start over a corrupt or unreadable file is not — so any error
// here just leaves the store empty rather than propagating.
func (s *NodeStatStore) load() {
	if s.dataDir == "" {
		return
	}
	raw, err := os.ReadFile(s.path())
	if err != nil {
		return
	}
	loaded := map[string]NodeStat{}
	if err := json.Unmarshal(raw, &loaded); err != nil {
		return
	}
	s.stats = loaded
}

// Get returns the stat record for key, or the zero value if never recorded.
func (s *NodeStatStore) Get(key string) NodeStat {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats[key]
}

// RecordProbe folds a latency probe result into the node's rolling stats.
//
// A failed probe still updates LastReason and LastFailAt (something worth
// remembering happened) but must leave EWMARTTms untouched: a dead probe
// reports 0ms, and blending that into the average would read as "got faster"
// when the truth is "didn't answer at all".
//
// The very first successful sample seeds EWMARTTms/JitterMs outright instead
// of being blended against the zero-value starting point — otherwise every
// node's first reading would come out halved.
//
// reason is sanitized exactly like RecordConnect's, even though the current
// caller (RankAutoCandidates, fed by pingTCPProbe/pingHysteria2Probe/
// pingWireGuardProbe/autoTLSProbe) only ever passes fixed vocabulary strings
// today. This is defence in depth, not a fix for a known leak: node_stats.json
// is unencrypted on the promise that it holds no addresses, and that promise
// should not depend on every present and future probe function remembering
// never to interpolate one.
func (s *NodeStatStore) RecordProbe(key string, rttMs, jitterMs int64, ok bool, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.stats[key]
	st.LastReason = sanitizeStatReason(reason)
	if ok {
		if st.EWMARTTms == 0 {
			st.EWMARTTms = float64(rttMs)
			st.JitterMs = float64(jitterMs)
		} else {
			st.EWMARTTms = st.EWMARTTms*(1-nodeStatAlpha) + float64(rttMs)*nodeStatAlpha
			st.JitterMs = st.JitterMs*(1-nodeStatAlpha) + float64(jitterMs)*nodeStatAlpha
		}
	} else {
		st.LastFailAt = time.Now()
	}
	s.stats[key] = st
	s.dirty = true
}

// RecordConnect folds in the outcome of a real connection attempt — the
// strongest signal this store has, since it reflects an actual end-to-end
// connect rather than a lightweight probe.
//
// ConsecFails resets to 0 on success and increments on failure, answering
// "is it broken right now". ConnectFail only ever accumulates, answering the
// separate question "how unreliable has this node been overall". Both are
// kept because the scoring logic needs to tell a node mid-outage apart from
// one that is merely historically flaky.
//
// reason is sanitized here, not left to callers, for the same reason
// RecordProbe sanitizes internally (see its comment): node_stats.json is
// unencrypted on the promise it holds no addresses, and that promise must
// hold by construction rather than depend on every present and future caller
// remembering to sanitize before calling in.
func (s *NodeStatStore) RecordConnect(key string, ok bool, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.stats[key]
	if ok {
		st.ConnectOK++
		st.ConsecFails = 0
		st.LastSuccessAt = time.Now()
		st.LastReason = ""
	} else {
		st.ConnectFail++
		st.ConsecFails++
		st.LastFailAt = time.Now()
		st.LastReason = sanitizeStatReason(reason)
	}
	s.stats[key] = st
	s.dirty = true
}

// Flush persists the store to disk if and only if something changed since
// the last successful flush. RecordProbe/RecordConnect only mark the store
// dirty in memory; a sweep of dozens of probes must cost exactly one
// WriteFile when the caller decides to flush, not one per record, and there
// is no background timer — callers own when a flush happens.
func (s *NodeStatStore) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.dirty || s.dataDir == "" {
		return nil
	}
	raw, err := json.Marshal(s.stats)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dataDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(s.path(), raw, 0o600); err != nil {
		return err
	}
	s.dirty = false
	return nil
}

// Package-level store. RankAutoCandidates is called from both the tray and the
// frontend and has no natural owner to thread the store through; a guarded
// package-level instance keeps the call sites unchanged.
var (
	nodeStatsMu    sync.Mutex
	nodeStatsStore *NodeStatStore
)

// SetNodeStatStore installs the active store. `mobile.InitAutoStats` calls
// this with a disk-backed store before anything can resolve an AUTO group;
// tests install a `t.TempDir()`-backed one and restore the previous value.
func SetNodeStatStore(s *NodeStatStore) {
	nodeStatsMu.Lock()
	defer nodeStatsMu.Unlock()
	nodeStatsStore = s
}

// nodeStats returns the active store, creating an in-memory one if the app has
// not installed a persistent store yet, so callers never nil-check.
func nodeStats() *NodeStatStore {
	nodeStatsMu.Lock()
	defer nodeStatsMu.Unlock()
	if nodeStatsStore == nil {
		nodeStatsStore = &NodeStatStore{stats: map[string]NodeStat{}}
	}
	return nodeStatsStore
}

// sanitizeStatReason reduces a failure description to a fixed vocabulary before
// it can reach node_stats.json.
//
// That file is deliberately NOT encrypted, on the stated grounds that it holds
// only opaque hashed keys and timings. Free-text reasons would break that
// promise silently: Go's dial errors embed the remote address ("dial tcp
// 203.0.113.5:443: i/o timeout"), so storing err.Error() verbatim would write
// real backend IPs to a plaintext file on disk — exactly the data
// server_pins.json is encrypted to protect. Anything unrecognised collapses to
// "error" rather than being stored.
func sanitizeStatReason(reason string) string {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "":
		return ""
	case "timeout", "connection_refused", "network_unreachable", "no_route_to_host",
		"connection_closed", "connection_reset", "probe_error", "lan_bind_unavailable":
		return reason
	}
	return "error"
}

// RecordConnectOutcome folds a real connection attempt into the active store.
// reason sanitization happens inside RecordConnect itself — see its comment.
func RecordConnectOutcome(key string, ok bool, reason string) {
	nodeStats().RecordConnect(key, ok, reason)
	_ = nodeStats().Flush()
}

// LookupNodeStat reads the active store's record for key, or the zero value
// if key was never recorded. the package-level store has no exported read
// access otherwise, and the mobile bindings need one without reaching into
// unexported state.
func LookupNodeStat(key string) NodeStat {
	return nodeStats().Get(key)
}
