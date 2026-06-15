/*
 * Copyright (C) 2026 ResultV
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package proxy

import (
	"runtime"
	"time"

	probing "github.com/prometheus-community/pro-bing"
)

// pingICMPHost sends a single ICMP echo to host and returns the round-trip
// latency in milliseconds. It is the only reliable way to measure latency to
// WireGuard / AmneziaWG endpoints: those speak UDP only and silently drop any
// packet that isn't a valid (and, for AmneziaWG, obfuscated) handshake, so
// neither a TCP connect nor a raw UDP byte yields an RTT. ICMP echo measures the
// path to the host regardless of which transport the VPN uses on top.
//
// source, when non-empty, binds the probe to a specific local IPv4 — used in
// tunnel mode to send the echo out the physical adapter instead of the TUN
// default route (which would otherwise measure the tunnel to itself).
//
// ok is false when ICMP produced no definitive answer (host blocks ICMP, name
// resolution failed, or the socket couldn't be created); callers fall back to a
// UDP liveness probe.
func pingICMPHost(host, source string) (latencyMs int64, ok bool) {
	pinger, err := probing.NewPinger(host)
	if err != nil {
		return 0, false
	}
	// Force IPv4: source binding below is an IPv4 LAN address and the UDP probe
	// fallback dials IPv4 too — mixing in a resolved IPv6 would measure a
	// different path.
	pinger.SetNetwork("ip4")
	pinger.Count = 1
	pinger.Timeout = 2 * time.Second
	// Windows raw ICMP sockets work without administrator rights; Linux and
	// macOS use unprivileged UDP "ping" sockets. SetPrivileged(true) off Windows
	// would require root.
	pinger.SetPrivileged(runtime.GOOS == "windows")
	if source != "" {
		pinger.Source = source
	}
	if err := pinger.Run(); err != nil {
		return 0, false
	}
	st := pinger.Statistics()
	if st.PacketsRecv == 0 {
		return 0, false
	}
	ms := st.AvgRtt.Milliseconds()
	if ms <= 0 {
		ms = 1
	}
	return ms, true
}
