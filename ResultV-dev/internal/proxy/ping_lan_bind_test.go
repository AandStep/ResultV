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
	"errors"
	"net"
	"testing"
)

func TestIsEngineTunIPv4(t *testing.T) {
	if !isEngineTunIPv4(net.ParseIP("172.19.0.1")) {
		t.Fatal("expected 172.19.0.1 to match engine TUN subnet")
	}
	if isEngineTunIPv4(net.ParseIP("172.19.1.1")) {
		t.Fatal("expected 172.19.1.1 outside /30")
	}
	if isEngineTunIPv4(net.ParseIP("10.0.0.1")) {
		t.Fatal("expected 10.0.0.1 not to match")
	}
}

func TestLooksLikeTunnelInterface(t *testing.T) {
	if !looksLikeTunnelInterface("Wintun Userspace Tunnel") {
		t.Fatal("wintun")
	}
	if looksLikeTunnelInterface("Ethernet") {
		t.Fatal("ethernet")
	}
}

func TestPingProxyLANBind_ReturnsLanBindUnavailableWhenNoInterface(t *testing.T) {
	oldPick := pickLANBindIPv4
	defer func() { pickLANBindIPv4 = oldPick }()
	pickLANBindIPv4 = func() (net.IP, error) {
		return nil, errors.New("no iface")
	}

	latency, reachable, reason := PingProxyLANBind("1.2.3.4", 443)
	if reachable || latency != 0 || reason != "lan_bind_unavailable" {
		t.Fatalf("unexpected result: latency=%d reachable=%v reason=%q", latency, reachable, reason)
	}
}

func TestPingProxyUDPLANBind_ReturnsLanBindUnavailableWhenNoInterface(t *testing.T) {
	oldPick := pickLANBindIPv4
	defer func() { pickLANBindIPv4 = oldPick }()
	pickLANBindIPv4 = func() (net.IP, error) {
		return nil, errors.New("no iface")
	}

	latency, reachable, reason := PingProxyUDPLANBind("1.2.3.4", 443)
	if reachable || latency != 0 || reason != "lan_bind_unavailable" {
		t.Fatalf("unexpected result: latency=%d reachable=%v reason=%q", latency, reachable, reason)
	}
}
