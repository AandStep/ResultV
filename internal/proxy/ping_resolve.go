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
	"net"
	"strings"
	"sync"
	"time"
)

// pingResolveTimeout bounds the OS-resolver attempt before falling back to DoH.
// Short on purpose: during a tunnel session the OS lookup does not fail fast, it
// hangs until its own timeout, and the ping grid in the UI fires per server.
const pingResolveTimeout = 2 * time.Second

// pingResolveTTL is how long a successful hostname→IPv4 answer is reused. The
// UI pings the whole server list at once and on every refresh; without a cache
// each pass would fire a DoH query per domain server.
const pingResolveTTL = 5 * time.Minute

// pingDoHResolve is resolveServerIPsViaDoH behind a var so tests can substitute
// it without touching the network.
var pingDoHResolve = resolveServerIPsViaDoH

type pingResolveEntry struct {
	ip      string
	expires time.Time
}

var (
	pingResolveMu    sync.Mutex
	pingResolveCache = map[string]pingResolveEntry{}
)

func lookupPingResolveCache(host string, now time.Time) (string, bool) {
	pingResolveMu.Lock()
	defer pingResolveMu.Unlock()
	e, ok := pingResolveCache[host]
	if !ok || now.After(e.expires) {
		return "", false
	}
	return e.ip, true
}

func storePingResolveCache(host, ip string, now time.Time) {
	pingResolveMu.Lock()
	defer pingResolveMu.Unlock()
	pingResolveCache[host] = pingResolveEntry{ip: ip, expires: now.Add(pingResolveTTL)}
}

// resetPingResolveCache clears the cache. Test-only.
func resetPingResolveCache() {
	pingResolveMu.Lock()
	defer pingResolveMu.Unlock()
	pingResolveCache = map[string]pingResolveEntry{}
}

// resolvePingHost turns a server address into a literal IPv4 for the ping
// probes. Literal input passes straight through.
//
// The OS resolver is tried first and DoH-over-IP second, because an active
// tunnel session breaks the OS resolver for the app itself: the system-DNS
// override pins the physical adapters to resolvers reachable only inside the
// tunnel, while the app's own traffic is self-direct. A domain server then
// failed every ping with "no such host", which the reason classifier collapsed
// to probe_error and the UI rendered as "Error" until the app was restarted.
//
// Returns "" when neither path answers; the caller decides what to report.
// Only successes are cached — the OS resolver recovers when the session ends.
func resolvePingHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if net.ParseIP(host) != nil {
		return host
	}
	now := time.Now()
	if ip, ok := lookupPingResolveCache(host, now); ok {
		return ip
	}

	ctx, cancel := context.WithTimeout(context.Background(), pingResolveTimeout)
	defer cancel()
	if addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host); err == nil {
		for _, a := range addrs {
			if v4 := a.IP.To4(); v4 != nil {
				ip := v4.String()
				storePingResolveCache(host, ip, now)
				return ip
			}
		}
	}

	if ips := pingDoHResolve(host); len(ips) > 0 {
		storePingResolveCache(host, ips[0], now)
		return ips[0]
	}
	return ""
}
