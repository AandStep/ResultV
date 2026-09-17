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
			byTag := serversByTag(dns.Servers)
			for _, s := range dns.Servers {
				if s.Tag == "local" {
					continue // bootstrap resolver, direct on purpose
				}
				assertDialsThroughTunnel(t, byTag, s)
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
	byTag := serversByTag(dns.Servers)
	for _, s := range dns.Servers {
		if s.Tag == "local" {
			continue
		}
		assertDialsThroughTunnel(t, byTag, s)
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

// serversByTag индексирует список для проверки ног fallback-обёртки.
func serversByTag(servers []SBDNSServer) map[string]SBDNSServer {
	byTag := make(map[string]SBDNSServer, len(servers))
	for _, s := range servers {
		byTag[s.Tag] = s
	}
	return byTag
}

// assertDialsThroughTunnel требует детур от сервера, который набирает сам, и
// разворачивает fallback-обёртку до её ног.
//
// У обёртки детур пуст намеренно: своего адреса у неё нет, она не набирает
// ничего, ядро добирается до членов по тегу. Требование «через туннель»
// относится к ногам, и здесь оно с них и спрашивается — то есть проверка после
// перехода на DoH стала строже, а не слабее.
func assertDialsThroughTunnel(t *testing.T, byTag map[string]SBDNSServer, s SBDNSServer) {
	t.Helper()
	if s.Type == "fallback" {
		if len(s.Servers) == 0 {
			t.Fatalf("fallback %q без ног — направлять запросы некуда", s.Tag)
		}
		for _, legTag := range s.Servers {
			leg, ok := byTag[legTag]
			if !ok {
				t.Fatalf("fallback %q ссылается на незарегистрированный сервер %q", s.Tag, legTag)
			}
			if leg.Detour != "proxy" {
				t.Fatalf("нога %q обёртки %q должна набираться через туннель, получено detour=%q",
					legTag, s.Tag, leg.Detour)
			}
		}
		return
	}
	if s.Detour != "proxy" {
		t.Fatalf("DNS server %q (type %s) must be dialled through the tunnel, got detour=%q",
			s.Tag, s.Type, s.Detour)
	}
}
