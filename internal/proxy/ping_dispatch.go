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
	"net/http"
	"strings"
	"time"

	"resultproxy-wails/internal/config"
)

// PingOptions is what the user asked the ping to measure. It arrives already
// normalised (see config.EffectivePing*): an unreadable type still falls back
// to auto here, because a binding is a public edge and must not trust its
// caller.
type PingOptions struct {
	// Type is one of config.PingType*.
	Type string
	// TestURL is fetched by the http_* types. Ignored by the others.
	TestURL string
	// Timeout bounds one measurement. For http_* and icmp it is the exact
	// budget; for auto it is an outer ceiling on top of each probe's own
	// internal limit, so it can shorten a wait but not extend one.
	Timeout time.Duration
}

// PingNode measures latency to one node the way the user asked for.
//
// This is the dispatcher the desktop keeps in Manager.Ping. There is no
// Manager on Android — the engine belongs to libbox and the app talks to it
// through bindings — so the dispatch lives here, where the probe seams are,
// and the binding above only parses JSON.
//
// keyedWGAllowed is passed in rather than read here because the tunnel's state
// is Kotlin's knowledge, not this package's: the keyed WireGuard handshake
// probe attacks the live session and must not run while it is up.
func PingNode(entryJSON string, opts PingOptions, keyedWGAllowed bool) (latencyMs int64, reachable bool, reason, checkType string) {
	var entry config.ProxyEntry
	if err := json.Unmarshal([]byte(entryJSON), &entry); err != nil {
		return 0, false, "probe_error", ""
	}
	if entry.IP == "" || entry.Port <= 0 {
		return 0, false, "probe_error", ""
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	switch strings.TrimSpace(opts.Type) {
	case config.PingTypeHTTPGet:
		return pingViaNode(entry, http.MethodGet, opts.TestURL, timeout)
	case config.PingTypeHTTPHead:
		return pingViaNode(entry, http.MethodHead, opts.TestURL, timeout)
	case config.PingTypeICMP:
		return pingViaICMP(entry, timeout)
	default:
		return pingByProtocol(entry, keyedWGAllowed)
	}
}

// pingViaNode fetches the test URL through the node itself.
func pingViaNode(entry config.ProxyEntry, method, testURL string, timeout time.Duration) (int64, bool, string, string) {
	url := strings.TrimSpace(testURL)
	if config.ValidatePingTestURL(url) != nil {
		// A stored URL that cannot work is the user's own setting, and saying
		// so beats silently measuring something else.
		return 0, false, "bad_test_url", pingCheckTypeFor(method)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ms, ok, reason := pingThroughNodeProbe(ctx, proxyConfigFromEntry(entry), method, url)
	return ms, ok, reason, pingCheckTypeFor(method)
}

func pingCheckTypeFor(method string) string {
	if method == http.MethodHead {
		return config.PingTypeHTTPHead
	}
	return config.PingTypeHTTPGet
}

// pingViaICMP measures the path to the node's address, whatever it carries on
// top. Source binding is not used on Android: the app is excluded from its own
// VPN, so the echo already leaves over the physical link.
func pingViaICMP(entry config.ProxyEntry, timeout time.Duration) (int64, bool, string, string) {
	ms, ok := pingICMPProbe(entry.IP, "", timeout)
	if !ok {
		// Not "timeout": the probe answers "no" both when the host stays
		// silent and when the socket could not be opened at all, and calling
		// that a timeout would send the user looking at the node instead of
		// at ICMP.
		return 0, false, "icmp_unavailable", config.PingTypeICMP
	}
	return ms, true, "", config.PingTypeICMP
}

// pingByProtocol is today's behaviour: the probe is picked by what the node
// speaks.
func pingByProtocol(entry config.ProxyEntry, keyedWGAllowed bool) (int64, bool, string, string) {
	switch strings.ToUpper(strings.TrimSpace(entry.Type)) {
	case "WIREGUARD", "AMNEZIAWG":
		if keyedWGAllowed {
			raw, err := json.Marshal(entry)
			if err != nil {
				return 0, false, "probe_error", "wg_handshake"
			}
			ms, ok, reason := PingWireGuardHandshake(string(raw))
			return ms, ok, reason, "wg_handshake"
		}
		// Tunnel is up — the keyed probe would attack the live session. Fall
		// back to the keyless liveness check.
		ms, ok, reason := pingWireGuardProbe(entry.IP, entry.Port)
		return ms, ok, reason, "wg_liveness"
	case "HYSTERIA2":
		return pingHysteria2Probe(entry.IP, entry.Port)
	default:
		ms, ok, reason := pingTCPProbe(entry.IP, entry.Port)
		return ms, ok, reason, "tcp"
	}
}

// proxyConfigFromEntry narrows a stored profile entry to what the config
// builders read.
func proxyConfigFromEntry(entry config.ProxyEntry) ProxyConfig {
	return ProxyConfig{
		IP:       entry.IP,
		Port:     entry.Port,
		Type:     entry.Type,
		Username: entry.Username,
		Password: entry.Password,
		URI:      entry.URI,
		Extra:    entry.Extra,
	}
}
