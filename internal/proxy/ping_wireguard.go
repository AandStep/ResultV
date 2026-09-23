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
	"context"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

// wgHandshakePollInterval is how often the device is asked whether the
// handshake finished. It does not bound the precision: the device stamps the
// moment itself.
const wgHandshakePollInterval = 10 * time.Millisecond

// pingWireGuardHandshake measures a WireGuard/AmneziaWG node by the only thing
// its server answers — a handshake with the node's own keys — made by a
// throwaway engine. The figure runs from the initiation leaving to the
// response being accepted.
//
// Not for the node of the live session: the server moves the peer to whoever
// handshook last, and the session's replies would go to this probe.
func pingWireGuardHandshake(ctx context.Context, proxy ProxyConfig, bindIPv4 string) (latencyMs int64, reachable bool, reason string) {
	boxCtx, stop, reason := startPingProbeEngine(ctx, proxy, getFreeLocalPort(0), bindIPv4)
	if reason != "" {
		return 0, false, reason
	}
	defer stop()

	endpoint, err := wgEndpointFrom(boxCtx)
	if err != nil {
		return 0, false, "engine_start_failed"
	}
	deviceValue, err := awg31DeviceInterface(endpoint)
	if err != nil {
		return 0, false, "engine_start_failed"
	}
	device, ok := deviceValue.(interface {
		ipcGetter
		ipcSetter
	})
	if !ok {
		return 0, false, "engine_start_failed"
	}
	dump, err := device.IpcGet()
	if err != nil {
		return 0, false, "engine_start_failed"
	}
	peerKey := uapiValue(dump, "public_key")
	if peerKey == "" {
		return 0, false, "engine_start_failed"
	}

	// Turning keepalive on for a running device sends one at once, and with no
	// session yet that is a handshake initiation.
	start := time.Now()
	if err := device.IpcSet("public_key=" + peerKey + "\npersistent_keepalive_interval=1\n"); err != nil {
		return 0, false, "engine_start_failed"
	}

	ticker := time.NewTicker(wgHandshakePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return 0, false, "timeout"
		case <-ticker.C:
		}
		dump, err := device.IpcGet()
		if err != nil {
			continue
		}
		done, ok := uapiHandshakeTime(dump)
		if !ok {
			continue
		}
		ms := done.Sub(start).Milliseconds()
		if ms < 1 {
			ms = 1
		}
		return ms, true, ""
	}
}

// uapiValue returns the first value of key in a UAPI dump. The interface
// section lists private_key, never public_key, so the first public_key is the
// peer's.
func uapiValue(dump, key string) string {
	for _, line := range strings.Split(dump, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && k == key {
			return v
		}
	}
	return ""
}

// uapiHandshakeTime reads the peer's last completed handshake; ok is false
// while there has been none.
func uapiHandshakeTime(dump string) (time.Time, bool) {
	sec, err := strconv.ParseInt(uapiValue(dump, "last_handshake_time_sec"), 10, 64)
	if err != nil || sec == 0 {
		return time.Time{}, false
	}
	nsec, _ := strconv.ParseInt(uapiValue(dump, "last_handshake_time_nsec"), 10, 64)
	return time.Unix(sec, nsec), true
}

// wireGuardDialer is an engine whose live session runs a WireGuard endpoint.
type wireGuardDialer interface {
	DialWireGuard(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error)
}

// pingThroughLiveWireGuard measures the live session's node over the session
// itself. The test host is resolved here and dialled by address: the
// session's own DNS may answer with a FakeIP that means nothing past the
// tunnel.
func pingThroughLiveWireGuard(ctx context.Context, dialer wireGuardDialer, method, testURL string, resolveBudget time.Duration) (int64, bool, string) {
	target, err := url.Parse(testURL)
	if err != nil || target.Hostname() == "" {
		return 0, false, "bad_test_url"
	}
	port := target.Port()
	if port == "" {
		port = "443"
		if target.Scheme == "http" {
			port = "80"
		}
	}
	ip := resolvePingHostBounded(target.Hostname(), resolveBudget)
	if ip == "" {
		return 0, false, "dns_unresolved"
	}
	destination := M.ParseSocksaddr(net.JoinHostPort(ip, port))
	return fetchPingVia(ctx, &http.Transport{
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialWireGuard(ctx, N.NetworkTCP, destination)
		},
	}, method, testURL)
}
