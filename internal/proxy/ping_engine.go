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
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
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
	// The probe engine's DNS block below has the same shape tunnel mode builds —
	// the hosts record when the server is pinned, the system resolver when it is
	// not — so it asks for the tag the same way.
	outbounds := buildOutbounds(proxy, serverDomainResolverTag(proxy, ProxyModeTunnel, nil), "")
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
					{Type: "hosts", Tag: serverPinDNSTag, Predefined: map[string][]string{proxy.IP: pinned}},
					{Type: "local", Tag: "local"},
				},
				Rules: []SBDNSRule{{Domain: []string{proxy.IP}, Server: serverPinDNSTag}},
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

// pingProbeEngineCeiling bounds how long the throwaway engine is given to shut
// down. Mirrors the main engine's teardown ceiling: a leaked instance is
// collected eventually, a frozen sweep is not.
const pingProbeEngineCeiling = 5 * time.Second

// classifyPingFetch turns one HTTP attempt into a verdict.
//
// Any status counts as success on purpose. The request goes out over HTTPS as
// a CONNECT and the certificate is verified inside this process, so a response
// arriving at all proves the bytes reached the real host — which is exactly
// what the measurement is asking. That is also why the plain-HTTP probes
// elsewhere in this package cannot be this permissive: there, a dead outbound
// is answered by our own inbound with a forged 502, and only an exact expected
// status tells the two apart (see probeResponseMatches).
func classifyPingFetch(resp *http.Response, err error) (bool, string) {
	if err != nil {
		return false, pingReasonFromError(err)
	}
	if resp == nil {
		return false, "no response"
	}
	if resp.StatusCode == http.StatusProxyAuthRequired {
		return false, "proxy_auth_required"
	}
	return true, ""
}

// pingThroughNode measures how long the node takes to deliver testURL.
//
// The clock covers the node handshake, the CONNECT, the TLS session to the
// target and the wait for response headers — everything the user is actually
// waiting on. Starting the engine is NOT in the figure: that is our cost, not
// the node's.
func pingThroughNode(ctx context.Context, proxy ProxyConfig, method, testURL, bindIPv4 string) (latencyMs int64, reachable bool, reason string) {
	port := getFreeLocalPort(0)
	cfg, err := BuildPingProbeConfig(proxy, port, bindIPv4)
	if err != nil {
		if errors.Is(err, errPingProbeUnsupported) {
			return 0, false, "unsupported_for_protocol"
		}
		return 0, false, "engine_config_failed"
	}

	configJSON, err := json.Marshal(cfg)
	if err != nil {
		return 0, false, "engine_config_failed"
	}

	boxCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	boxCtx = extendedBoxContext(boxCtx)

	var options option.Options
	if err := singjson.UnmarshalContext(boxCtx, configJSON, &options); err != nil {
		return 0, false, "engine_config_failed"
	}

	// No PlatformLogWriter, no traffic tracker and no logger on the teardown
	// path: this engine's bytes are our own measurement, not the user's
	// session, and must not land in either the visible log or the traffic
	// counters. The logger is not merely unused here, it is absent from the
	// signature — closeInstanceBounded writes a line per teardown, and a sweep
	// that probes every node once a cycle turned that into a wall of
	// "Закрываем N соединений перед остановкой" in the user's own log.
	instance, err := box.New(box.Options{Context: boxCtx, Options: options})
	if err != nil {
		return 0, false, "engine_start_failed"
	}
	if err := instance.Start(); err != nil {
		closeInstanceBounded(instance, boxCtx, pingProbeEngineCeiling, nil)
		return 0, false, "engine_start_failed"
	}
	defer closeInstanceBounded(instance, boxCtx, pingProbeEngineCeiling, nil)

	proxyURL, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	if err != nil {
		return 0, false, "engine_config_failed"
	}
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:             http.ProxyURL(proxyURL),
			DisableKeepAlives: true,
		},
		// Redirects are not followed: the first answer already proves the node
		// carried the request, and chasing a redirect would measure a second
		// host instead of this one.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, method, testURL, nil)
	if err != nil {
		return 0, false, "bad_test_url"
	}

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)
	if resp != nil {
		defer resp.Body.Close()
	}
	ok, reason := classifyPingFetch(resp, err)
	if !ok {
		return 0, false, reason
	}
	ms := elapsed.Milliseconds()
	if ms <= 0 {
		ms = 1
	}
	return ms, true, ""
}

// pingThroughNodeProbe is a var so Manager tests can measure the dispatch
// logic without standing up a real engine, matching the pingTCPProbe pattern.
var pingThroughNodeProbe = pingThroughNode
