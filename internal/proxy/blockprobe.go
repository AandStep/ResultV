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
	"strings"
	"time"

	"resultproxy-wails/internal/verdict"
)

// probeOutcome is one observation of a host: what happened when we asked it a
// question ourselves, rather than what we inferred from someone else's traffic.
type probeOutcome struct {
	Err      error
	Status   int
	Location string
	Bytes    int64
	Elapsed  time.Duration
}

func (o probeOutcome) reachable() bool { return o.Err == nil && o.Status > 0 }

// regionBlockMarkers are the paths and hosts services redirect a censored
// region to. Kept as substrings because the exact URL differs per service and
// per week; the shape does not.
var regionBlockMarkers = []string{
	"unavailable-in-region",
	"unavailable_in_region",
	"not-available-in-your-country",
	"region-block",
	"geo-block",
	"geoblock",
}

// looksLikeRegionBlock reports whether a reachable answer is a wall rather than
// a page. 403 and 451 say it outright; a redirect has to be read, because an
// ordinary 301 to www is not a wall.
func looksLikeRegionBlock(o probeOutcome) bool {
	if !o.reachable() {
		return false
	}
	if o.Status == 403 || o.Status == 451 {
		return true
	}
	if o.Status < 300 || o.Status >= 400 {
		return false
	}
	loc := strings.ToLower(o.Location)
	for _, marker := range regionBlockMarkers {
		if strings.Contains(loc, marker) {
			return true
		}
	}
	return false
}

// classifyProbe turns two observations into a verdict.
//
// The node is the control, not a second opinion: unless it answered, nothing
// the direct side did is evidence about censorship. That asymmetry is the
// guard against a bad minute on the user's own line being written down as a
// week of "blocked".
func classifyProbe(direct, viaNode probeOutcome) verdict.Decision {
	if !viaNode.reachable() {
		return verdict.Unknown
	}
	if direct.Err != nil {
		return verdict.Proxy
	}
	if looksLikeRegionBlock(direct) {
		return verdict.Proxy
	}
	if !direct.reachable() {
		return verdict.Unknown
	}
	return verdict.Direct
}

// probeFetch performs one observation. Declared as a var so tests can hand the
// classifier synthetic outcomes without opening a socket — the same seam
// autoprobe.go already uses for autoProbeLookupIPAddr.
//
// The real HTTP measurement lands with the outbound that needs it: until a
// verdict can change a route, a probe would only be traffic with nowhere to go.
var probeFetch = func(ctx context.Context, url string, viaNode bool) probeOutcome {
	return probeOutcome{Err: context.Canceled}
}
