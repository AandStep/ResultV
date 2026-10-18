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

import (
	"strings"
	"testing"
)

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
	top := byTag[byTag[dns.Final].Servers[0]]
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

func TestTunnelDNSFallsBackToPublicResolversAfterConfigOnes(t *testing.T) {
	cfg := EngineConfig{
		Mode:        ProxyModeTunnel,
		RoutingMode: ModeGlobal,
		Proxy:       ProxyConfig{Type: "amneziawg", IP: "1.2.3.4", Port: 51820},
		DNSServers:  []string{"10.8.0.1"},
	}
	dns := buildDNS(cfg)

	byTag := make(map[string]SBDNSServer, len(dns.Servers))
	for _, server := range dns.Servers {
		byTag[server.Tag] = server
	}

	top := byTag[dns.Final]
	if dns.Final != "custom-1" || top.Strategy != "sequential" || len(top.Servers) != 2 {
		t.Fatalf("Final %q: ожидалась последовательная пара «конфиг, запасной», получено %+v", dns.Final, top)
	}

	configured := byTag[top.Servers[0]]
	if configured.Timeout != "1s" {
		t.Errorf("резолверу из конфига отведено %q, ожидалась 1s", configured.Timeout)
	}
	if addrs := dnsLegAddrs(byTag, configured); len(addrs) != 1 || addrs[0] != "10.8.0.1" {
		t.Errorf("первым должен спрашиваться только резолвер из конфига, получено %v", addrs)
	}

	backup := byTag[top.Servers[1]]
	if addrs := dnsLegAddrs(byTag, backup); len(addrs) != 2 || addrs[0] != "8.8.8.8" || addrs[1] != "1.1.1.1" {
		t.Errorf("запасные резолверы: %v, ожидались 8.8.8.8 и 1.1.1.1", addrs)
	}
	for _, server := range dns.Servers {
		if server.Detour != "" && server.Detour != "proxy" {
			t.Errorf("%s идёт мимо узла: detour=%q", server.Tag, server.Detour)
		}
	}
}

func TestTunnelDNSBackupSkipsResolversAlreadyConfigured(t *testing.T) {
	cfg := EngineConfig{
		Mode:        ProxyModeTunnel,
		RoutingMode: ModeGlobal,
		Proxy:       ProxyConfig{Type: "amneziawg", IP: "1.2.3.4", Port: 51820},
		DNSServers:  []string{"8.8.8.8", "1.1.1.1"},
	}
	dns := buildDNS(cfg)
	for _, server := range dns.Servers {
		if strings.HasPrefix(server.Tag, backupDNSGroupTag) {
			t.Errorf("при 8.8.8.8 и 1.1.1.1 в конфиге запасной не нужен, а появился %q", server.Tag)
		}
	}
}

func TestTunnelDNSWithoutConfiguredResolversHasNoBackupLayer(t *testing.T) {
	cfg := EngineConfig{
		Mode:        ProxyModeTunnel,
		RoutingMode: ModeGlobal,
		Proxy:       ProxyConfig{Type: "amneziawg", IP: "1.2.3.4", Port: 51820},
	}
	dns := buildDNS(cfg)
	for _, server := range dns.Servers {
		if server.Timeout == "1s" {
			t.Errorf("без DNS в конфиге никому не урезают время, а %q получил 1s", server.Tag)
		}
	}
}

// Без dns.final ядро берёт первый зарегистрированный транспорт — голую DoH-ногу,
// и TCP-нога за обёрткой недостижима. Резолвер из конфига AWG (100.64.0.1) DoH
// не держит, так что каждое имя падало бы на x509.
func TestBuildDNSFinalNamesTheFallbackWrapper(t *testing.T) {
	for _, routing := range []RoutingMode{ModeGlobal, ModeSmart} {
		for _, servers := range [][]string{nil, {"100.64.0.1"}, {"10.8.0.1", "9.9.9.9"}} {
			dns := buildDNS(EngineConfig{
				Mode:        ProxyModeTunnel,
				RoutingMode: routing,
				Proxy:       ProxyConfig{Type: "amneziawg", IP: "1.2.3.4", Port: 51820},
				DNSServers:  servers,
			})
			var final *SBDNSServer
			for i := range dns.Servers {
				if dns.Servers[i].Tag == dns.Final {
					final = &dns.Servers[i]
				}
			}
			if final == nil || final.Type != "fallback" || final.throughDetour != "proxy" {
				t.Fatalf("%s dns=%v: Final=%q must name the tunnel wrapper", routing, servers, dns.Final)
			}
		}
	}
}

func dnsLegAddrs(byTag map[string]SBDNSServer, group SBDNSServer) []string {
	var addrs []string
	seen := map[string]bool{}
	var walk func(SBDNSServer)
	walk = func(s SBDNSServer) {
		if s.Type != "fallback" {
			if !seen[s.Server] {
				seen[s.Server] = true
				addrs = append(addrs, s.Server)
			}
			return
		}
		for _, member := range s.Servers {
			walk(byTag[member])
		}
	}
	walk(group)
	return addrs
}
