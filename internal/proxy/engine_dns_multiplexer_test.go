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

// The tunnel's DNS must not reach the node through a bare tcp/tls/udp server.
// Engine 1.14 put a query multiplexer in front of exactly those three
// transports, and through a proxy outbound it wedges: measured 15.09.2026,
// 294 of 295 lookups routed to the node never returned — no answer, no error —
// so no YouTube host resolved at all. Only the https transport escapes it.
//
// These tests exist because the failure is silent. Nothing logs, nothing
// errors; the only symptom is that names stop resolving. If someone folds the
// pair back into a single tcp server, this is what says so.
func TestTunnelDNSReachesNodeOverDoHNotBareTCP(t *testing.T) {
	for _, tc := range []struct {
		name        string
		dnsServers  []string
		wantWrapper string
	}{
		{"дефолтные резолверы", nil, "google"},
		{"пользовательский резолвер", []string{"8.8.8.8"}, "custom-1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := smartDNSConfig()
			cfg.DNSServers = tc.dnsServers
			dns := buildDNS(cfg)

			byTag := make(map[string]SBDNSServer, len(dns.Servers))
			for _, server := range dns.Servers {
				byTag[server.Tag] = server
			}

			// Whatever the rules point at must be the wrapper, never a leg.
			tunnelTag := firstDetourServerTag(dns.Servers, "proxy")
			if tunnelTag != tc.wantWrapper {
				t.Fatalf("правила должны указывать на обёртку %q, получено %q", tc.wantWrapper, tunnelTag)
			}
			wrapper, ok := byTag[tunnelTag]
			if !ok {
				t.Fatalf("сервер %q не объявлен", tunnelTag)
			}
			if wrapper.Type != "fallback" {
				t.Errorf("обёртка должна быть fallback, получено %q", wrapper.Type)
			}
			if wrapper.Timeout == "" {
				t.Error("без timeout зависшая нога висит вечно — ровно то, что чинится")
			}
			if len(wrapper.Servers) != 2 {
				t.Fatalf("ожидались две ноги, получено %v", wrapper.Servers)
			}

			// DoH leads: it is the one remote transport the multiplexer does
			// not touch.
			doh := byTag[wrapper.Servers[0]]
			if doh.Type != "https" {
				t.Errorf("первой ногой должен идти DoH, получено %q (%s)", doh.Type, wrapper.Servers[0])
			}
			if doh.Detour != "proxy" {
				t.Errorf("DoH обязан идти через узел, detour=%q", doh.Detour)
			}
			// DoH speaks HTTPS and must reach 443; carrying a DNS port here
			// would point it at a port that serves no HTTPS.
			if doh.ServerPort != 0 && doh.ServerPort != 443 {
				t.Errorf("у DoH не должно быть DNS-порта, получено %d", doh.ServerPort)
			}

			// The plain leg stays as the second chance for a resolver that
			// speaks no DoH — deleting it would strip such a resolver of any
			// path at all.
			plain := byTag[wrapper.Servers[1]]
			if plain.Type != "tcp" {
				t.Errorf("второй ногой должен идти tcp, получено %q", plain.Type)
			}
			if plain.Detour != "proxy" {
				t.Errorf("запасная нога обязана идти через узел, detour=%q", plain.Detour)
			}

			// Every multiplexed transport reaching the node must be a leg of
			// some wrapper — reachable only after DoH was tried and timed out.
			// One standing on its own is the bug returning.
			legs := make(map[string]bool)
			for _, server := range dns.Servers {
				if server.Type != "fallback" {
					continue
				}
				for _, leg := range server.Servers {
					legs[leg] = true
				}
			}
			for _, server := range dns.Servers {
				if server.Detour != "proxy" || legs[server.Tag] {
					continue
				}
				switch server.Type {
				case "tcp", "tls", "udp":
					t.Errorf("сервер %q типа %q идёт через узел голым мультиплексируемым транспортом",
						server.Tag, server.Type)
				}
			}
		})
	}
}
