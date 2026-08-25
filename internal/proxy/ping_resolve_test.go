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
	"testing"
	"time"
)

func TestResolvePingHostPassesThroughLiteralIP(t *testing.T) {
	calls := 0
	orig := pingDoHResolve
	pingDoHResolve = func(string) []string { calls++; return nil }
	defer func() { pingDoHResolve = orig }()

	if got := resolvePingHost("203.0.113.7"); got != "203.0.113.7" {
		t.Fatalf("resolvePingHost = %q, want the literal back", got)
	}
	if calls != 0 {
		t.Fatalf("DoH called %d times for a literal IP, want 0", calls)
	}
}

// The point of the whole task: when the OS resolver is dead (the tunnel's own
// DNS override made it unreachable) a domain server must still resolve, via
// DoH-over-IP, instead of every ping rendering as "Error".
func TestResolvePingHostFallsBackToDoH(t *testing.T) {
	resetPingResolveCache()
	orig := pingDoHResolve
	pingDoHResolve = func(host string) []string {
		if host != "vpn.example.invalid" {
			t.Fatalf("DoH asked for %q", host)
		}
		return []string{"198.51.100.9"}
	}
	defer func() { pingDoHResolve = orig }()

	// .invalid never resolves in the OS resolver (RFC 2606), so this exercises
	// the fallback without depending on the machine's DNS.
	if got := resolvePingHost("vpn.example.invalid"); got != "198.51.100.9" {
		t.Fatalf("resolvePingHost = %q, want the DoH answer", got)
	}
}

func TestResolvePingHostCachesSuccess(t *testing.T) {
	resetPingResolveCache()
	calls := 0
	orig := pingDoHResolve
	pingDoHResolve = func(string) []string { calls++; return []string{"198.51.100.9"} }
	defer func() { pingDoHResolve = orig }()

	for i := 0; i < 3; i++ {
		if got := resolvePingHost("vpn.example.invalid"); got != "198.51.100.9" {
			t.Fatalf("call %d: resolvePingHost = %q", i, got)
		}
	}
	if calls != 1 {
		t.Fatalf("DoH called %d times, want 1 — the UI pings the whole server list", calls)
	}
}

func TestResolvePingHostEmptyWhenNothingResolves(t *testing.T) {
	resetPingResolveCache()
	orig := pingDoHResolve
	pingDoHResolve = func(string) []string { return nil }
	defer func() { pingDoHResolve = orig }()

	if got := resolvePingHost("vpn.example.invalid"); got != "" {
		t.Fatalf("resolvePingHost = %q, want empty", got)
	}
	// A failed lookup must not be cached: the resolver comes back when the
	// session ends, and the next ping should try again.
	if _, ok := lookupPingResolveCache("vpn.example.invalid", time.Now()); ok {
		t.Fatal("failed resolution was cached")
	}
}

// Manager.Ping must hand the probes a literal IP, never the hostname: the
// probes dial by name and would go back through the dead OS resolver.
func TestManagerPingDialsResolvedIP(t *testing.T) {
	resetPingResolveCache()
	origDoH := pingDoHResolve
	pingDoHResolve = func(string) []string { return []string{"198.51.100.9"} }
	defer func() { pingDoHResolve = origDoH }()

	var dialed string
	origTCP := pingTCPProbe
	pingTCPProbe = func(host string, port int) (int64, bool, string) {
		dialed = host
		return 12, true, ""
	}
	defer func() { pingTCPProbe = origTCP }()

	m := &Manager{}
	res := m.Ping("vpn.example.invalid", 443, "VLESS")
	if dialed != "198.51.100.9" {
		t.Fatalf("probe dialed %q, want the resolved IP", dialed)
	}
	if !res.Reachable || res.LatencyMs != 12 {
		t.Fatalf("result = %+v, want reachable 12ms", res)
	}
}

// When nothing resolves, the user needs to see WHY — "Error" (probe_error) sent
// them looking at a healthy server.
func TestManagerPingReportsUnresolvedDomain(t *testing.T) {
	resetPingResolveCache()
	origDoH := pingDoHResolve
	pingDoHResolve = func(string) []string { return nil }
	defer func() { pingDoHResolve = origDoH }()

	called := false
	origTCP := pingTCPProbe
	pingTCPProbe = func(host string, port int) (int64, bool, string) {
		called = true
		return 0, false, "probe_error"
	}
	defer func() { pingTCPProbe = origTCP }()

	m := &Manager{}
	res := m.Ping("vpn.example.invalid", 443, "VLESS")
	if called {
		t.Fatal("probe dialed an unresolvable hostname; the OS resolver is the thing we're avoiding")
	}
	if res.Reachable || res.Reason != "dns_unresolved" {
		t.Fatalf("result = %+v, want unreachable dns_unresolved", res)
	}
}
