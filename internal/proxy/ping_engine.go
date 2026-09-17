// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

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
	"github.com/sagernet/sing-box/include"
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

// pingProbeHostsTag names the static record that answers the node's own name.
const pingProbeHostsTag = "ping-probe-hosts"


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
// Two things the desktop version has are deliberately absent here:
//
//   - No bind-to-adapter parameter. The desktop pins the node dial to the
//     physical adapter so a probe during a live session does not measure the
//     tunnel to itself. On Android the app is excluded from its own VPN
//     (BoxModule passes its own package to addDisallowedApplication), so this
//     process never enters the tunnel and there is nothing to pin. Adding the
//     field to SBOutbound just to pass an always-empty value would be dead
//     weight in the config every node builds.
//   - A different reason for the pinned-IP DNS record. The desktop needs one
//     because its own resolver is unusable during a session. Ours needs one
//     because the throwaway engine has no platform interface: the real engine
//     resolves through libbox's LocalDNSTransport, which Kotlin implements,
//     and a "local" server without it has nothing to read — Android has no
//     /etc/resolv.conf. So the caller resolves the node's name with the Go
//     resolver (which does work here, same as the WireGuard handshake probe)
//     and hands the answer in as nodeIPs.
func BuildPingProbeConfig(proxy ProxyConfig, listenPort int, nodeIPs []string) (SingBoxConfig, error) {
	// A name-addressed node needs someone to resolve it; a literal one needs
	// nobody, and serverDomainResolverTag says which case this is by returning
	// an empty tag.
	//
	// When the caller already resolved the name, the outbound is pointed
	// straight at the static record. Naming a server matters in 1.14: an
	// outbound with domain_resolver set goes to THAT server and never consults
	// the DNS rules, so leaving it on "local" sent every lookup to a resolver
	// the probe does not have — on the phone that is ::1:53, and the engine
	// answered "connection refused" before it ever dialled the node.
	resolverTag := serverDomainResolverTag(proxy, ProxyModeTunnel, nil)
	if resolverTag != "" && len(nodeIPs) > 0 {
		resolverTag = pingProbeHostsTag
	}
	outbounds := buildOutbounds(proxy, resolverTag)
	found := false
	for i := range outbounds {
		if outbounds[i].Tag == "proxy" {
			found = true
		}
	}
	if !found {
		return SingBoxConfig{}, fmt.Errorf("%w: %s", errPingProbeUnsupported, proxy.Type)
	}

	cfg := SingBoxConfig{
		// error level only: a sweep over a 32-node subscription starts one of
		// these per node, and anything chattier buries the user's own log.
		Log:       &SBLog{Level: "error"},
		Inbounds:  []SBInbound{{Type: "mixed", Tag: pingProbeInboundTag, Listen: "127.0.0.1", ListenPort: listenPort}},
		Outbounds: outbounds,
		Route:     &SBRoute{Final: "proxy"},
	}

	// The host of the test URL is never resolved here: with the default as-is
	// domain strategy sing-box hands the name to the outbound and the node
	// resolves it. The only name needing an answer is the node's own.
	if proxy.IP != "" && net.ParseIP(proxy.IP) == nil {
		if len(nodeIPs) > 0 {
			cfg.DNS = &SBDNS{
				Servers: []SBDNSServer{
					{Type: "hosts", Tag: pingProbeHostsTag, Predefined: map[string][]string{proxy.IP: nodeIPs}},
					{Type: "local", Tag: "local"},
				},
				Rules: []SBDNSRule{{Domain: []string{proxy.IP}, Server: pingProbeHostsTag}},
				Final: "local",
			}
		} else {
			// Nothing resolved: leave the local server so the config stays
			// valid, and let the dial fail with its own reason rather than
			// inventing one here.
			cfg.DNS = &SBDNS{
				Servers: []SBDNSServer{{Type: "local", Tag: "local"}},
				Final:   "local",
			}
		}
	}

	return cfg, nil
}

// pingProbeEngineCeiling bounds how long the throwaway engine is given to shut
// down. A leaked instance is collected eventually, a frozen sweep is not.
const pingProbeEngineCeiling = 5 * time.Second

// pingEngineMaxConcurrency caps how many throwaway probe engines run at once.
//
// PingRepository sweeps the list with sixteen workers (PING_CONCURRENCY), and
// an engine costs orders of magnitude more than a socket, so the ceiling has
// to live here rather than in Kotlin — a phone that started sixteen sing-box
// instances at once would not be probing, it would be sabotaging itself.
const pingEngineMaxConcurrency = 4

// pingEngineSem admits probe engines. Package-level rather than per-caller
// because the cost it protects — memory and sockets — is the process's. Taken
// by pingViaNode before the measurement budget starts.
var pingEngineSem = make(chan struct{}, pingEngineMaxConcurrency)

// classifyPingFetch turns one HTTP attempt into a verdict.
//
// Any status counts as success on purpose. The request goes out over HTTPS as
// a CONNECT and the certificate is verified inside this process, so a response
// arriving at all proves the bytes reached the real host — which is exactly
// what the measurement is asking.
func classifyPingFetch(resp *http.Response, err error) (bool, string) {
	if err != nil {
		return false, pingReasonFromError(err)
	}
	if resp == nil {
		return false, "no_response"
	}
	if resp.StatusCode == http.StatusProxyAuthRequired {
		return false, "proxy_auth_required"
	}
	return true, ""
}

// pingThroughNode measures how long the node takes to deliver testURL.
//
// It does NOT take the engine seat: the caller does that first, because a
// budget that counts queue time reports a false timeout for every node that
// merely waited its turn (see pingViaNode).
//
// The clock covers the node handshake, the CONNECT, the TLS session to the
// target and the wait for response headers — everything the user is actually
// waiting on. Starting the engine is NOT in the figure: that is our cost, not
// the node's.
func pingThroughNode(ctx context.Context, proxy ProxyConfig, method, testURL string) (latencyMs int64, reachable bool, reason string) {
	port := getFreeLocalPort(0)
	cfg, err := BuildPingProbeConfig(proxy, port, resolveProbeNodeIPs(ctx, proxy.IP))
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
	// include.Context, not the extended one: the custom outbound registry
	// arrives with the adaptive Smart block. When it does, this is the line to
	// change — the probe must build a node the same way the session does.
	boxCtx = include.Context(boxCtx)

	var options option.Options
	if err := singjson.UnmarshalContext(boxCtx, configJSON, &options); err != nil {
		return 0, false, "engine_config_failed"
	}

	instance, err := box.New(box.Options{Context: boxCtx, Options: options})
	if err != nil {
		return 0, false, "engine_start_failed"
	}
	if err := instance.Start(); err != nil {
		closePingProbeBounded(instance)
		return 0, false, "engine_start_failed"
	}
	defer closePingProbeBounded(instance)

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

// pingThroughNodeProbe is a var so the dispatch can be tested without standing
// up a real engine.
var pingThroughNodeProbe = pingThroughNode

// closePingProbeBounded closes the throwaway instance without letting a stuck
// teardown hold the sweep.
//
// No logger on purpose: on the desktop a line per teardown turned a list sweep
// into a wall of "closing N connections" in the user's own log. This engine's
// lifetime is our business, not the user's.
func closePingProbeBounded(instance *box.Box) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = instance.Close()
	}()
	select {
	case <-done:
	case <-time.After(pingProbeEngineCeiling):
	}
}

// resolveProbeNodeIPs answers the node's own name for the probe engine.
//
// The engine cannot do it itself: it has no platform interface, so its "local"
// DNS server has nothing to read on Android. This process can — the Go
// resolver goes through the system one here, the same way the WireGuard
// handshake probe resolves its endpoint. An empty answer is not an error: the
// caller still builds a config, and the dial then fails with a real reason.
func resolveProbeNodeIPs(ctx context.Context, host string) []string {
	if host == "" || net.ParseIP(host) != nil {
		return nil
	}
	resolveCtx, cancel := context.WithTimeout(ctx, pingProbeResolveCeiling)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupNetIP(resolveCtx, "ip", host)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, a.Unmap().String())
	}
	return out
}

// pingProbeResolveCeiling bounds the name lookup so a silent resolver cannot
// eat the whole measurement budget before the node is even dialled.
const pingProbeResolveCeiling = 2 * time.Second
