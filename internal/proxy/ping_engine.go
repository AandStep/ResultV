// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

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
// Two things the desktop version has are deliberately absent here:
//
//   - No bind-to-adapter parameter. The desktop pins the node dial to the
//     physical adapter so a probe during a live session does not measure the
//     tunnel to itself. On Android the app is excluded from its own VPN
//     (BoxModule passes its own package to addDisallowedApplication), so this
//     process never enters the tunnel and there is nothing to pin. Adding the
//     field to SBOutbound just to pass an always-empty value would be dead
//     weight in the config every node builds.
//   - No pinned-IP DNS record. The desktop needs one because its own resolver
//     is unusable during a session; ours is not, for the same reason as above,
//     so a name-addressed node is resolved by the plain local server.
func BuildPingProbeConfig(proxy ProxyConfig, listenPort int) (SingBoxConfig, error) {
	// A name-addressed node needs someone to resolve it; a literal one needs
	// nobody, and serverDomainResolverTag says which case this is by returning
	// an empty tag. Tunnel mode is asked for deliberately: it is the mode whose
	// answer is the plain "local" server, which is exactly what the probe has.
	resolverTag := serverDomainResolverTag(proxy, ProxyModeTunnel, nil)
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
		cfg.DNS = &SBDNS{
			Servers: []SBDNSServer{{Type: "local", Tag: "local"}},
			Final:   "local",
		}
	}

	return cfg, nil
}
