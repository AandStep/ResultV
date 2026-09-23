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
// pingHysteria2HandshakeTimeout leaves room for a second flow inside the default
// 3 s ping budget when the network drops the first one.
const pingHysteria2HandshakeTimeout = "1500ms"

func BuildPingProbeConfig(proxy ProxyConfig, listenPort int, bindIPv4 string) (SingBoxConfig, error) {
	// The probe engine's DNS block below has the same shape tunnel mode builds —
	// the hosts record when the server is pinned, the system resolver when it is
	// not — so it asks for the tag the same way.
	resolverTag := serverDomainResolverTag(proxy, ProxyModeTunnel, nil)
	outbounds := buildOutbounds(proxy, resolverTag, "")
	endpoints, err := pingProbeEndpoints(proxy, resolverTag)
	if err != nil {
		return SingBoxConfig{}, err
	}
	// A WireGuard endpoint reaches its server through "direct", so that is the
	// outbound to pin.
	final, serverOut := "proxy", "proxy"
	if len(endpoints) > 0 {
		final, serverOut = wireguardEndpointTag, "direct"
	}
	found := false
	for i := range outbounds {
		if outbounds[i].Tag != serverOut {
			continue
		}
		found = true
		if bindIPv4 != "" {
			outbounds[i].Inet4BindAddress = bindIPv4
		}
		if outbounds[i].Type == "hysteria2" && outbounds[i].TLS != nil {
			outbounds[i].TLS.HandshakeTimeout = pingHysteria2HandshakeTimeout
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
		Endpoints: endpoints,
		Route:     &SBRoute{Final: final},
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

// pingProbeEndpoints is the node's WireGuard endpoint, stripped of whatever
// would collide with a live session of the same node: a system interface, its
// name and a fixed listen port. Keepalive is off so the device stays silent
// until the probe asks for a handshake.
func pingProbeEndpoints(proxy ProxyConfig, resolverTag string) ([]SBEndpoint, error) {
	endpoints, err := buildEndpoints(proxy, resolverTag)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errPingProbeUnsupported, err)
	}
	for i := range endpoints {
		endpoints[i].System = false
		endpoints[i].Name = ""
		endpoints[i].ListenPort = 0
		for j := range endpoints[i].Peers {
			endpoints[i].Peers[j].PersistentKeepaliveInterval = 0
		}
	}
	return endpoints, nil
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

// startPingProbeEngine starts the throwaway engine for proxy with its mixed
// inbound on port. reason is non-empty when it could not be started.
func startPingProbeEngine(ctx context.Context, proxy ProxyConfig, port int, bindIPv4 string) (boxCtx context.Context, stop func(), reason string) {
	cfg, err := BuildPingProbeConfig(proxy, port, bindIPv4)
	if err != nil {
		if errors.Is(err, errPingProbeUnsupported) {
			return nil, nil, "unsupported_for_protocol"
		}
		return nil, nil, "engine_config_failed"
	}

	configJSON, err := json.Marshal(cfg)
	if err != nil {
		return nil, nil, "engine_config_failed"
	}

	boxCtx, cancel := context.WithCancel(ctx)
	boxCtx = extendedBoxContext(boxCtx)

	var options option.Options
	if err := singjson.UnmarshalContext(boxCtx, configJSON, &options); err != nil {
		cancel()
		return nil, nil, "engine_config_failed"
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
		cancel()
		return nil, nil, "engine_start_failed"
	}
	stop = func() {
		closeInstanceBounded(instance, boxCtx, pingProbeEngineCeiling, nil)
		cancel()
	}
	if err := instance.Start(); err != nil {
		stop()
		return nil, nil, "engine_start_failed"
	}
	if err := applyAWG31(boxCtx, awg31KnobsFor(proxy), nil); err != nil {
		stop()
		return nil, nil, "engine_start_failed"
	}
	return boxCtx, stop, ""
}

// pingThroughNode measures how long the node takes to deliver testURL.
//
// The clock covers the node handshake, the CONNECT, the TLS session to the
// target and the wait for response headers — everything the user is actually
// waiting on. Starting the engine is NOT in the figure: that is our cost, not
// the node's.
func pingThroughNode(ctx context.Context, proxy ProxyConfig, method, testURL, bindIPv4 string) (latencyMs int64, reachable bool, reason string) {
	port := getFreeLocalPort(0)
	_, stop, reason := startPingProbeEngine(ctx, proxy, port, bindIPv4)
	if reason != "" {
		return 0, false, reason
	}
	defer stop()

	proxyURL, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	if err != nil {
		return 0, false, "engine_config_failed"
	}
	return fetchPingVia(ctx, &http.Transport{
		Proxy:             http.ProxyURL(proxyURL),
		DisableKeepAlives: true,
	}, method, testURL)
}

// fetchPingVia times testURL fetched over transport.
func fetchPingVia(ctx context.Context, transport *http.Transport, method, testURL string) (int64, bool, string) {
	client := &http.Client{
		Transport: transport,
		// Redirects are not followed: the first answer already proves the node
		// carried the request, and chasing a redirect would measure a second
		// host instead of this one.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	if _, err := http.NewRequestWithContext(ctx, method, testURL, nil); err != nil {
		return 0, false, "bad_test_url"
	}
	return fetchPing(ctx, func() (time.Duration, bool, string) {
		req, _ := http.NewRequestWithContext(ctx, method, testURL, nil)
		start := time.Now()
		resp, err := client.Do(req)
		elapsed := time.Since(start)
		if resp != nil {
			resp.Body.Close()
		}
		ok, reason := classifyPingFetch(resp, err)
		return elapsed, ok, reason
	})
}

// pingAttempts bounds how often one measurement is repeated inside its time
// budget. A fresh engine closes its first hysteria2/TUIC connection with its own
// startup ResetNetwork, and the network drops part of new QUIC flows outright;
// a live session survives both by opening the next connection, so does the ping.
const pingAttempts = 3

func fetchPing(ctx context.Context, attempt func() (time.Duration, bool, string)) (int64, bool, string) {
	var (
		elapsed time.Duration
		ok      bool
		reason  string
	)
	for i := 0; i < pingAttempts && !ok; i++ {
		if i > 0 && ctx.Err() != nil {
			break
		}
		elapsed, ok, reason = attempt()
	}
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
