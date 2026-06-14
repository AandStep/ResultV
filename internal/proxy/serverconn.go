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

// Capturing the proxy server's real IP from the live connection.
//
// For a domain-addressed VPN server in a censored network, the OS resolver
// cannot resolve the server hostname at all (that is what the tunnel is for) —
// only sing-box can, via its own DNS path. So our connect-time pin
// (resolvePinnedServerIP, which uses the OS resolver) comes back empty, the
// server domain stays in the sing-box outbound, and sing-box re-resolves it per
// connection through the fragile `local` resolver — a transient failure there
// trips a false kill switch, and the kill-switch firewall has no IP to allow
// ("no proxy IP to allow").
//
// sing-box, however, IS connected — so the OS holds a live socket from our own
// process to the server's real IP. Reading that socket gives the exact address
// sing-box is actually using (the ground truth), which is strictly better than
// re-resolving: a CDN-fronted server has many A records, and a fresh lookup can
// return a different pool member than the one in use, so a firewall rule built
// from it would allow an IP the live connection never touches.
//
// establishedServerIP returns the remote IP of an ESTABLISHED TCP connection
// owned by THIS process whose remote port is `serverPort`, or "" when none is
// found (no live socket yet, a UDP transport, or a non-Windows build). It is
// best-effort: an empty return simply leaves the existing domain behaviour
// unchanged, never a regression.

// tcpConnRow is the platform-independent projection of one OS TCP table entry
// the picker needs. The Windows syscall layer fills these from the raw
// MIB_TCPROW_OWNER_PID rows; keeping the selection logic off the syscall struct
// makes it unit-testable on any platform.
type tcpConnRow struct {
	remoteIP    string
	remotePort  int
	pid         uint32
	established bool
}

// pickServerIP selects the server IP from a snapshot of the OS TCP table. It
// keeps only ESTABLISHED rows owned by ourPID whose remote port matches the
// server port, then returns the remote IP shared by the most connections.
//
// The "most connections" tie-break matters for transports that open several
// sockets to the server (XHTTP up/down streams, connection pools): they all
// share the one server IP, so the dominant IP is the server even if the
// process also has an unrelated single connection to the same port number
// (the port==443 ambiguity). When the winner is ambiguous (a tie between
// distinct IPs, e.g. exactly one connection each), it returns "" rather than
// guess — a wrong pin is worse than none.
func pickServerIP(rows []tcpConnRow, ourPID uint32, serverPort int) string {
	counts := make(map[string]int)
	for _, r := range rows {
		if !r.established || r.pid != ourPID || r.remotePort != serverPort {
			continue
		}
		if r.remoteIP == "" {
			continue
		}
		counts[r.remoteIP]++
	}
	bestIP := ""
	bestN := 0
	tie := false
	for ip, n := range counts {
		switch {
		case n > bestN:
			bestIP, bestN, tie = ip, n, false
		case n == bestN:
			tie = true
		}
	}
	if bestIP == "" || tie {
		return ""
	}
	return bestIP
}
