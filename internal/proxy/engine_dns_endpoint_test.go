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

import "testing"

// A DNS server with no detour is dialled by sing-box's *default* dialer
// (common/dialer/dialer.go: `if Detour != "" { NewDetour } else { NewDefault }`),
// which is a plain protected socket on the underlying network — route rules are
// never consulted. So an endpoint transport built without a detour sends every
// lookup in the clear to 8.8.8.8 past the tunnel, which on a censored network is
// exactly the resolver the tunnel exists to avoid.
//
// The endpoint carries tag "proxy" and OutboundManager.Outbound() falls back to
// the endpoint registry (adapter/outbound/manager.go), so "proxy" is a valid
// detour for WireGuard/AmneziaWG just like it is for VLESS.
func TestEndpointDNSGoesThroughTheTunnel(t *testing.T) {
	for _, pt := range []string{"WIREGUARD", "AMNEZIAWG"} {
		t.Run(pt, func(t *testing.T) {
			dns := buildDNS(EngineConfig{
				Mode:      ProxyModeTunnel,
				Proxy:     ProxyConfig{IP: "203.0.113.9", Port: 51820, Type: pt},
				DataDir:   t.TempDir(),
				IsAndroid: true,
			})
			for _, s := range dns.Servers {
				if s.Tag == "local" {
					continue // bootstrap resolver, direct on purpose
				}
				if s.Detour != "proxy" {
					t.Fatalf("DNS server %q (type %s) must be dialled through the tunnel, got detour=%q",
						s.Tag, s.Type, s.Detour)
				}
			}
		})
	}
}

// Plain UDP :53 to a public resolver is the first thing a censoring middlebox
// rewrites. Every other protocol already asks for TCP/TLS; endpoints must not be
// the exception now that their lookups ride the tunnel.
func TestEndpointDNSUsesTCPNotUDP(t *testing.T) {
	dns := buildDNS(EngineConfig{
		Mode:      ProxyModeTunnel,
		Proxy:     ProxyConfig{IP: "203.0.113.9", Port: 51820, Type: "AMNEZIAWG"},
		DataDir:   t.TempDir(),
		IsAndroid: true,
	})
	for _, s := range dns.Servers {
		if s.Tag == "local" {
			continue
		}
		if s.Type == "udp" {
			t.Fatalf("DNS server %q rides the tunnel and must not use plaintext UDP", s.Tag)
		}
	}
}

// Custom (user-entered) resolvers take the same path — the setting must not
// silently become a direct, leak-everything resolver on an endpoint profile.
func TestEndpointCustomDNSGoesThroughTheTunnel(t *testing.T) {
	dns := buildDNS(EngineConfig{
		Mode:       ProxyModeTunnel,
		Proxy:      ProxyConfig{IP: "203.0.113.9", Port: 51820, Type: "AMNEZIAWG"},
		DataDir:    t.TempDir(),
		IsAndroid:  true,
		DNSServers: []string{"9.9.9.9"},
	})
	for _, s := range dns.Servers {
		if s.Tag == "local" {
			continue
		}
		if s.Detour != "proxy" {
			t.Fatalf("custom DNS server %q must be dialled through the tunnel, got detour=%q", s.Tag, s.Detour)
		}
	}
}

// A hostname endpoint (Endpoint = vpn.example.com:51820) must keep resolving via
// the bootstrap resolver: sending that one lookup through the tunnel is the
// chicken-and-egg the "local" server exists to break.
func TestEndpointServerHostnameResolvesLocally(t *testing.T) {
	dns := buildDNS(EngineConfig{
		Mode:      ProxyModeTunnel,
		Proxy:     ProxyConfig{IP: "vpn.example.com", Port: 51820, Type: "AMNEZIAWG"},
		DataDir:   t.TempDir(),
		IsAndroid: true,
	})
	for _, r := range dns.Rules {
		for _, d := range r.Domain {
			if d == "vpn.example.com" && r.Server == "local" {
				return
			}
		}
	}
	t.Fatal("the endpoint's own hostname must resolve through the bootstrap resolver, else the tunnel can never dial its peer")
}
