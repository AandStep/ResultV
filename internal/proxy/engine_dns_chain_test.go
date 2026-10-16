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

func TestTunnelDNSAsksConfiguredResolversInParallel(t *testing.T) {
	cfg := EngineConfig{
		Mode:        ProxyModeTunnel,
		RoutingMode: ModeGlobal,
		Proxy:       ProxyConfig{Type: "amneziawg", IP: "1.2.3.4", Port: 51820},
		DNSServers:  []string{"10.8.0.1", "9.9.9.9"},
	}
	dns := buildDNS(cfg)

	byTag := make(map[string]SBDNSServer, len(dns.Servers))
	for _, server := range dns.Servers {
		byTag[server.Tag] = server
	}

	if dns.Final != "custom-1" {
		t.Fatalf("Final должен указывать на обёртку резолверов из конфига, получено %q", dns.Final)
	}
	top := byTag[dns.Final]
	if top.Type != "fallback" || top.Strategy != "parallel" {
		t.Fatalf("резолверы должны спрашиваться параллельно, получено %s/%s", top.Type, top.Strategy)
	}
	if top.Timeout == "" {
		t.Error("у верхней обёртки нет timeout")
	}

	want := []string{"10.8.0.1", "9.9.9.9"}
	if len(top.Servers) != len(want) {
		t.Fatalf("члены обёртки: %v, ожидалось по одному на резолвер %v", top.Servers, want)
	}
	for i, member := range top.Servers {
		resolver := byTag[member]
		if resolver.Type != "fallback" || resolver.Strategy != "sequential" || len(resolver.Servers) != 2 {
			t.Fatalf("%s: ожидалась последовательная обёртка из двух ног, получено %+v", member, resolver)
		}
		doh, plain := byTag[resolver.Servers[0]], byTag[resolver.Servers[1]]
		if doh.Type != "https" || plain.Type != "tcp" {
			t.Errorf("%s: ноги %s/%s, ожидались https/tcp", member, doh.Type, plain.Type)
		}
		if doh.Server != want[i] || plain.Server != want[i] {
			t.Errorf("%s: ноги идут к %s/%s, ожидался %s", member, doh.Server, plain.Server, want[i])
		}
	}

	seen := make(map[string]bool)
	for _, server := range dns.Servers {
		for _, member := range server.Servers {
			if !seen[member] {
				t.Errorf("обёртка %q ссылается на %q раньше, чем он объявлен", server.Tag, member)
			}
		}
		seen[server.Tag] = true
	}
}
