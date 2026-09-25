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

package system

import (
	"net"
	"testing"
)

// TestCheckHostsAreLiteralIPs pins the invariant that keeps "Интернет-соединение
// потеряно" honest. A hostname target is resolved by the OS resolver, which the
// app's own tunnel session breaks: the system-DNS override pins the physical
// adapters to resolvers reachable only inside the tunnel, while the app's
// traffic is self-direct. The monitor then reports the machine offline while
// the browser streams video.
func TestCheckHostsAreLiteralIPs(t *testing.T) {
	if len(checkHosts) == 0 {
		t.Fatal("checkHosts is empty")
	}
	for _, hp := range checkHosts {
		host, port, err := net.SplitHostPort(hp)
		if err != nil {
			t.Fatalf("checkHosts entry %q is not host:port: %v", hp, err)
		}
		if port == "" {
			t.Fatalf("checkHosts entry %q has no port", hp)
		}
		if net.ParseIP(host) == nil {
			t.Fatalf("checkHosts entry %q uses a hostname; the OS resolver dies "+
				"during a tunnel session — use a literal IP", hp)
		}
	}
}

// Teredo drops and re-adds its link-local address on its own schedule, and
// our TUN comes and goes with every session. Neither moves the physical
// address probes leave through, so neither may read as a network change.
func TestAddrSignatureIgnoresVirtualAndLinkLocal(t *testing.T) {
	mustCIDR := func(s string) net.Addr {
		ip, n, err := net.ParseCIDR(s)
		if err != nil {
			t.Fatal(err)
		}
		n.IP = ip
		return n
	}
	base := map[string][]net.Addr{
		"Ethernet": {mustCIDR("192.168.0.11/24"), mustCIDR("fe80::5fdd:2162:71f1:268e/64")},
	}
	noisy := map[string][]net.Addr{
		"Ethernet": {mustCIDR("192.168.0.11/24"), mustCIDR("fe80::5fdd:2162:71f1:268e/64")},
		"Teredo Tunneling Pseudo-Interface": {mustCIDR("fe80::30de:d3af:a542:51a6/64")},
		"rvtun0":                            {mustCIDR("172.19.0.1/30")},
	}
	if a, b := addrSignature(base), addrSignature(noisy); a != b {
		t.Fatalf("виртуальные адаптеры меняют сигнатуру:\n%s\n%s", a, b)
	}

	roamed := map[string][]net.Addr{
		"Ethernet": {mustCIDR("192.168.1.20/24")},
	}
	if addrSignature(base) == addrSignature(roamed) {
		t.Fatal("смена физического адреса не видна")
	}
}
