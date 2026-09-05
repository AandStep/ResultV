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

package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Why this probe exists: everything else the selector measures is latency —
// a TCP connect, a TLS or QUIC handshake, a STUN round trip. None of it says
// how much the node can actually move. Ranking on latency alone reliably
// prefers a congested node with a short path over a free one slightly further
// away, because congestion barely shows up in a handshake and dominates every
// download the user actually cares about.
//
// The measurement is deliberately taken AFTER connect, through the live
// session, not during the sweep. Measuring throughput to a candidate would
// mean speaking its full protocol (VLESS/Reality auth, Hysteria2 handshake),
// i.e. standing up an engine instance per candidate — minutes of work and a
// storm of sessions for a number that would be stale by the time it was used.
// Measuring the node we did connect to costs one small download, and the
// result is exactly the kind of history a probe can never produce: it is
// remembered per node (NodeStat.ThroughputKBps) and weighed the next time the
// group is ranked.

// throughputProbeDomains are pinned to the proxy outbound in buildRoute,
// scoped to the probe inbound — for the same reason udpRelayProbeDomains are.
// In Smart mode Final=direct would otherwise send the download straight out of
// the local uplink and report the user's own broadband as the node's speed,
// which is worse than not measuring at all.
var throughputProbeDomains = []string{"speed.cloudflare.com"}

// throughputProbeURL asks Cloudflare's public speed endpoint for a fixed
// payload. HTTPS on purpose: it goes through the local inbound as CONNECT, so
// a dead outbound fails the tunnel instead of being answered by the inbound's
// own error page (see probeResponseMatches for how that bit us on plain http).
const throughputProbeURL = "https://speed.cloudflare.com/__down?bytes=524288"

// throughputProbeBytes is what we ask for and the ceiling on what we read.
// Half a megabyte is enough to leave slow-start behind on any link worth
// measuring, and small enough that paying it once per connect is invisible.
const throughputProbeBytes = 512 * 1024

// throughputProbeMinBytes is the least we will draw a conclusion from. A
// truncated transfer measures the truncation, not the link.
const throughputProbeMinBytes = 128 * 1024

// ThroughputResult is one measurement of how fast the live session moves data.
type ThroughputResult struct {
	OK     bool
	KBps   float64
	Bytes  int64
	Reason string
}

// ProbeThroughput downloads a fixed payload through the local inbound and
// reports the observed rate.
//
// The clock starts after the response headers arrive, so the figure describes
// the transfer and not the connect + TLS + first-byte latency that precedes
// it. Those are already measured, twice over, by the sweep.
func ProbeThroughput(ctx context.Context, proxyAddr string) ThroughputResult {
	if strings.TrimSpace(proxyAddr) == "" {
		return ThroughputResult{Reason: "no local inbound address"}
	}
	proxyURL, err := url.Parse("http://" + proxyAddr)
	if err != nil {
		return ThroughputResult{Reason: "bad proxy url"}
	}
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:             http.ProxyURL(proxyURL),
			DisableKeepAlives: true,
			// No compression: the endpoint serves incompressible bytes, but an
			// Accept-Encoding negotiated behind our back would make the figure
			// describe a codec rather than the link.
			DisableCompression: true,
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, throughputProbeURL, nil)
	if err != nil {
		return ThroughputResult{Reason: "bad probe url"}
	}
	resp, err := client.Do(req)
	if err != nil {
		return ThroughputResult{Reason: probeFailureReason(err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ThroughputResult{Reason: fmt.Sprintf("unexpected status %d", resp.StatusCode)}
	}

	start := time.Now()
	n, err := io.Copy(io.Discard, io.LimitReader(resp.Body, throughputProbeBytes))
	elapsed := time.Since(start)
	if err != nil && n < throughputProbeMinBytes {
		return ThroughputResult{Bytes: n, Reason: probeFailureReason(err)}
	}
	if n < throughputProbeMinBytes {
		return ThroughputResult{Bytes: n, Reason: "short transfer"}
	}
	if elapsed <= 0 {
		return ThroughputResult{Bytes: n, Reason: "no elapsed time"}
	}
	return ThroughputResult{
		OK:    true,
		Bytes: n,
		KBps:  float64(n) / 1024 / elapsed.Seconds(),
	}
}
