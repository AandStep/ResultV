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
	"sync/atomic"
	"testing"
)

// TestTrafficTracker_ProxyCountersOnlyWhenTracked verifies the watchdog veto's
// liveness signal is sound: proxy-only byte counters are fed ONLY by the proxy
// outbound (shouldTrack), never by direct/split-tunnel traffic — otherwise
// direct traffic could mask a genuinely dead upstream and suppress a real trip.
func TestTrafficTracker_ProxyCountersOnlyWhenTracked(t *testing.T) {
	var up, down, pUp, pDown atomic.Int64
	tr := &trafficTracker{upload: &up, download: &down, proxyUpload: &pUp, proxyDownload: &pDown}

	// Proxy outbound: both the global and the proxy-only counters are incremented.
	dc := tr.downCounters(true)
	if len(dc) != 2 || dc[0] != &down || dc[1] != &pDown {
		t.Fatalf("tracked down counters = %v, want [global, proxy]", dc)
	}
	uc := tr.upCounters(true)
	if len(uc) != 2 || uc[0] != &up || uc[1] != &pUp {
		t.Fatalf("tracked up counters = %v, want [global, proxy]", uc)
	}

	// Direct/block outbound: only the global counter — the proxy-only counter is
	// never touched, so it reflects upstream traffic exclusively.
	if dc2 := tr.downCounters(false); len(dc2) != 1 || dc2[0] != &down {
		t.Fatalf("untracked down counters = %v, want [global] only", dc2)
	}
	if uc2 := tr.upCounters(false); len(uc2) != 1 || uc2[0] != &up {
		t.Fatalf("untracked up counters = %v, want [global] only", uc2)
	}
}
