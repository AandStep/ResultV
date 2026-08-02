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
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"resultproxy-wails/internal/config"
)

// autoProbeConcurrency bounds in-flight probes. Probes are network-bound and
// spend nearly all their time on timeouts, so a bounded pool turns
// nodes×timeout into ~nodes/16×timeout. Matches PING_CONCURRENCY in
// frontend/src/hooks/useDaemonPing.js so both sweeps behave the same.
const autoProbeConcurrency = 16

type AutoProbeDepth int

const (
	// DepthFast probes the transport only: TCP connect, QUIC handshake for
	// Hysteria2, UDP for WireGuard. One sample. Used to sweep the whole group.
	DepthFast AutoProbeDepth = iota
	// DepthFull adds a TLS handshake with the node's real SNI and ALPN and
	// takes three samples. Used on the shortlist only.
	DepthFull
)

type AutoProbeResult struct {
	Key      string
	RTTms    int64
	JitterMs int64
	OK       bool
	Stage    string
	Reason   string
}

// AutoNodeKey identifies a node across subscription refreshes. The backend
// reassigns ProxyEntry.ID on every fetch (app.go), so ID is useless as a
// persistent key; address plus subscription is stable.
func AutoNodeKey(e config.ProxyEntry) string {
	raw := strings.ToLower(strings.TrimSpace(e.SubscriptionURL)) + "|" +
		strings.ToLower(strings.TrimSpace(e.IP)) + "|" +
		fmt.Sprintf("%d", e.Port) + "|" +
		strings.ToUpper(strings.TrimSpace(e.Type))
	sum := sha1.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// isProbeableNode reports whether an entry has something to dial. SECTION rows
// are subscription labels whose IP/Port were blanked by normalizeSectionEntry;
// probing one dials ":0".
func isProbeableNode(e config.ProxyEntry) bool {
	if strings.EqualFold(strings.TrimSpace(e.Type), "SECTION") {
		return false
	}
	return strings.TrimSpace(e.IP) != "" && e.Port > 0
}

// ProbeAutoNodes probes every dialable node in parallel and returns one result
// per probed node, in input order. Non-dialable entries are dropped.
func ProbeAutoNodes(ctx context.Context, nodes []config.ProxyEntry, depth AutoProbeDepth) []AutoProbeResult {
	targets := make([]config.ProxyEntry, 0, len(nodes))
	for _, n := range nodes {
		if isProbeableNode(n) {
			targets = append(targets, n)
		}
	}
	if len(targets) == 0 {
		return nil
	}

	out := make([]AutoProbeResult, len(targets))
	var next int
	var mu sync.Mutex
	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for {
			mu.Lock()
			if next >= len(targets) {
				mu.Unlock()
				return
			}
			i := next
			next++
			mu.Unlock()

			if ctx.Err() != nil {
				return
			}
			out[i] = probeOne(targets[i], depth)
		}
	}

	pool := autoProbeConcurrency
	if pool > len(targets) {
		pool = len(targets)
	}
	wg.Add(pool)
	for i := 0; i < pool; i++ {
		go worker()
	}
	wg.Wait()

	return out
}

// probeTransport runs the transport-level probe for one node, mirroring the
// protocol dispatch in Manager.Ping.
func probeTransport(e config.ProxyEntry) (rtt int64, ok bool, stage, reason string) {
	switch strings.ToUpper(strings.TrimSpace(e.Type)) {
	case "HYSTERIA2":
		rtt, ok, reason, stage = pingHysteria2Probe(e.IP, e.Port)
		return rtt, ok, stage, reason
	case "WIREGUARD", "AMNEZIAWG":
		rtt, ok, reason = pingWireGuardProbe(e.IP, e.Port)
		return rtt, ok, "udp", reason
	default:
		rtt, ok, reason = pingTCPProbe(e.IP, e.Port)
		return rtt, ok, "tcp", reason
	}
}

func probeOne(e config.ProxyEntry, depth AutoProbeDepth) AutoProbeResult {
	rtt, ok, stage, reason := probeTransport(e)
	return AutoProbeResult{
		Key:    AutoNodeKey(e),
		RTTms:  rtt,
		OK:     ok,
		Stage:  stage,
		Reason: reason,
	}
}
