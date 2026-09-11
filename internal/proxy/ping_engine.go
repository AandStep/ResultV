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
	"errors"
	"fmt"
	"net"
)

// errPingProbeUnsupported marks a node whose protocol cannot carry an
// arbitrary TCP request at all — WireGuard and AmneziaWG, which buildOutbounds
// answers with direct+block and no proxy outbound.
var errPingProbeUnsupported = errors.New("protocol carries no proxy outbound")

// pingProbeInboundTag names the probe engine's only listener. It exists purely
// so the JSON is readable in a bug report.
const pingProbeInboundTag = "ping-probe-in"

// BuildPingProbeConfig assembles the whole sing-box config for one throwaway
// measurement: a loopback mixed inbound, the node's own outbound, nothing else.
//
// It deliberately reuses buildOutbounds rather than growing a second, simpler
// outbound builder. buildOutbounds already knows every protocol and its Extra
// quirks; a parallel implementation would drift apart from it on the next
// protocol we add, and the probe would then measure a node the real session
// builds differently. Its direct/block outbounds ride along unused — harmless
// next to that risk.
//
// bindIPv4, when non-empty, pins the node dial to the physical adapter.
func BuildPingProbeConfig(proxy ProxyConfig, listenPort int, bindIPv4 string) (SingBoxConfig, error) {
	outbounds := buildOutbounds(proxy)
	found := false
	for i := range outbounds {
		if outbounds[i].Tag != "proxy" {
			continue
		}
		found = true
		if bindIPv4 != "" {
			outbounds[i].Inet4BindAddress = bindIPv4
		}
	}
	if !found {
		return SingBoxConfig{}, fmt.Errorf("%w: %s", errPingProbeUnsupported, proxy.Type)
	}

	cfg := SingBoxConfig{
		// error level only: a sweep over a 48-node subscription starts 48 of
		// these, and anything chattier buries the user's own log.
		Log:       &SBLog{Level: "error"},
		Inbounds:  []SBInbound{{Type: "mixed", Tag: pingProbeInboundTag, Listen: "127.0.0.1", ListenPort: listenPort}},
		Outbounds: outbounds,
		Route:     &SBRoute{Final: "proxy"},
	}

	// The host of the test URL is never resolved here: with the default
	// as-is domain strategy sing-box hands the name to the outbound and the
	// node resolves it. The only name needing an answer is the node's own,
	// and only when it is a domain. Serve it from a static hosts record built
	// from the IPs connect time already learned — the OS resolver is
	// unusable for this process during an active session (see resolvePingHost).
	if proxy.IP != "" && net.ParseIP(proxy.IP) == nil {
		if pinned := serverPinnedIPs(proxy); len(pinned) > 0 {
			cfg.DNS = &SBDNS{
				Servers: []SBDNSServer{
					{Type: "hosts", Tag: "server-pin", Predefined: map[string][]string{proxy.IP: pinned}},
					{Type: "local", Tag: "local"},
				},
				Rules: []SBDNSRule{{Domain: []string{proxy.IP}, Server: "server-pin"}},
				Final: "local",
			}
		} else {
			cfg.DNS = &SBDNS{
				Servers: []SBDNSServer{{Type: "local", Tag: "local"}},
				Final:   "local",
			}
		}
	}

	return cfg, nil
}
