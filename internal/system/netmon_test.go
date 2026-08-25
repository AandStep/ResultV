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
