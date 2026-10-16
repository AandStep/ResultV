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
	"testing"
	"time"
)

func TestTunnelDNSFallsThroughToNextConfiguredResolver(t *testing.T) {
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
		t.Fatalf("Final должен указывать на обёртку первого резолвера, получено %q", dns.Final)
	}
	wrapper := byTag[dns.Final]
	var addrs []string
	for _, leg := range wrapper.Servers {
		addrs = append(addrs, byTag[leg].Server)
	}
	want := []string{"10.8.0.1", "10.8.0.1", "9.9.9.9", "9.9.9.9"}
	if len(addrs) != len(want) {
		t.Fatalf("ноги обёртки: %v, ожидались адреса %v", wrapper.Servers, want)
	}
	for i := range want {
		if addrs[i] != want[i] {
			t.Fatalf("ноги обёртки идут к %v, ожидались %v", addrs, want)
		}
	}

	timeout, err := time.ParseDuration(wrapper.Timeout)
	if err != nil {
		t.Fatalf("timeout %q: %v", wrapper.Timeout, err)
	}
	if perLeg := timeout / time.Duration(len(wrapper.Servers)); perLeg < 2500*time.Millisecond {
		t.Errorf("первой ноге досталось %v — меньше, чем у одиночного резолвера", perLeg)
	}

	seen := make(map[string]bool)
	for _, server := range dns.Servers {
		for _, leg := range server.Servers {
			if !seen[leg] {
				t.Errorf("обёртка %q ссылается на %q раньше, чем он объявлен", server.Tag, leg)
			}
		}
		seen[server.Tag] = true
	}
}
