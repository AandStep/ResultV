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
	"time"

	"resultproxy-wails/internal/verdict"
)

// probeRecheckTimeout caps one host's whole re-check. Both halves run in
// parallel, so this is the wall-clock cost of learning that one name is walled
// off — and nothing waits on it.
const probeRecheckTimeout = 12 * time.Second

// probeHost asks one host the same question twice — directly and through the
// node — and returns what the pair of answers proves.
//
// It exists because the race cannot see a geo-block: claude.ai answers a
// Russian address with a 302 to a region page in 407 ms over a perfectly
// healthy TCP connection, so the direct path wins the race while the site stays
// broken. Only reading the answer tells the two apart.
func probeHost(ctx context.Context, host string, gate *probeGate) verdict.Decision {
	if host == "" || gate == nil {
		return verdict.Unknown
	}
	if !gate.allow(host) {
		return verdict.Unknown
	}
	ctx, cancel := context.WithTimeout(ctx, probeRecheckTimeout)
	defer cancel()

	url := "https://" + host + "/"
	type half struct {
		out     probeOutcome
		viaNode bool
	}
	results := make(chan half, 2)
	go func() { results <- half{probeFetch(ctx, url, false), false} }()
	go func() { results <- half{probeFetch(ctx, url, true), true} }()

	var direct, viaNode probeOutcome
	for i := 0; i < 2; i++ {
		r := <-results
		if r.viaNode {
			viaNode = r.out
		} else {
			direct = r.out
		}
	}

	decision := classifyProbe(direct, viaNode)
	// The gate's notion of failure is "this probe told us nothing", not "the
	// site is blocked": a host that keeps teaching nothing is the one worth
	// backing off from.
	gate.record(host, decision != verdict.Unknown)
	return decision
}
