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
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"testing"
)

// wintunGUIDForName reimplements sing-tun's generateGUIDByDeviceName
// (sing-tun/tun_windows.go:636): the raw 16 md5 bytes of "wintun"+name are
// reinterpreted as a windows.GUID, i.e. Data1/Data2/Data3 are little-endian and
// Data4 is the trailing 8 bytes verbatim. Recomputed here rather than compared
// against a second copy of the constant on purpose — the whole point is to
// catch a rename that forgets to recompute the GUID.
func wintunGUIDForName(name string) string {
	sum := md5.Sum([]byte("wintun" + name))
	return fmt.Sprintf("{%08X-%04X-%04X-%02X%02X-%X}",
		binary.LittleEndian.Uint32(sum[0:4]),
		binary.LittleEndian.Uint16(sum[4:6]),
		binary.LittleEndian.Uint16(sum[6:8]),
		sum[8], sum[9], sum[10:16])
}

// The cleanup paths look the adapter up by GUID. If someone changes
// tunInterfaceName without recomputing tunAdapterGUID, every one of them
// silently searches for a device that never exists — which is the exact bug
// this whole change fixes, returned in an indistinguishable form.
func TestTunAdapterGUIDMatchesInterfaceName(t *testing.T) {
	if got := wintunGUIDForName(tunInterfaceName); got != tunAdapterGUID {
		t.Fatalf("tunAdapterGUID is stale: name %q derives %s, constant says %s",
			tunInterfaceName, got, tunAdapterGUID)
	}
}

// Legacy GUID must stay pinned to sing-box's default name: users upgrading from
// a pre-rename build can still have that adapter (or its ghost) on disk.
func TestLegacyTunAdapterGUIDMatchesDefaultName(t *testing.T) {
	if got := wintunGUIDForName("tun0"); got != tunAdapterGUIDLegacy {
		t.Fatalf("tunAdapterGUIDLegacy is wrong: %q derives %s, constant says %s",
			"tun0", got, tunAdapterGUIDLegacy)
	}
}

// looksLikeTunnelInterface classifies adapters by the "tun" substring, and three
// call sites depend on OUR adapter being classified as a tunnel: skipAdapterDNS
// (sysdns_windows.go) keeps it out of the physical-adapter DNS snapshot,
// systemHasIPv6 (engine.go) must not count it as host IPv6, and the Ping LAN
// bind (ping_lan_bind.go) must not bind to it. A name without "tun" breaks all
// three at once.
func TestTunInterfaceNameLooksLikeTunnel(t *testing.T) {
	if !looksLikeTunnelInterface(tunInterfaceName) {
		t.Fatalf("tunInterfaceName %q must be recognised by looksLikeTunnelInterface", tunInterfaceName)
	}
}

// The name must not collide with sing-box's default, or we are back to sharing a
// devnode with every other sing-box client on the machine.
func TestTunInterfaceNameIsNotTheSingBoxDefault(t *testing.T) {
	if tunInterfaceName == "tun0" || tunInterfaceName == "tun" {
		t.Fatalf("tunInterfaceName %q is the shared sing-box default", tunInterfaceName)
	}
}

func TestIsOurTunAdapterGUID(t *testing.T) {
	cases := []struct {
		name string
		guid string
		want bool
	}{
		{"current", "{1F2204B3-7F00-E47C-7441-6763D2F86416}", true},
		{"current lowercase", "{1f2204b3-7f00-e47c-7441-6763d2f86416}", true},
		{"legacy tun0", "{0DCCC63E-5622-3880-1E09-7CC9C46AD7B4}", true},
		{"legacy lowercase", "{0dccc63e-5622-3880-1e09-7cc9c46ad7b4}", true},
		// Another sing-box client that DID set its own interface_name carries the
		// same "sing-tun Tunnel" description but a different GUID. The old
		// description match swept it up; the GUID match must not.
		{"foreign sing-tun client", "{7B3C1A55-1111-2222-3333-444455556666}", false},
		{"wireguard", "{A1B2C3D4-0000-0000-0000-000000000000}", false},
		{"empty", "", false},
		{"guid without braces", "1F2204B3-7F00-E47C-7441-6763D2F86416", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isOurTunAdapterGUID(tc.guid); got != tc.want {
				t.Fatalf("isOurTunAdapterGUID(%q) = %v, want %v", tc.guid, got, tc.want)
			}
		})
	}
}

func TestTunnelModeConfigPinsInterfaceName(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy: ProxyConfig{Type: "vless", IP: "203.0.113.7", Port: 443, Password: "p"},
		Mode:  ProxyModeTunnel,
	})
	var found bool
	for _, in := range cfg.Inbounds {
		if in.Tag != "tun-in" {
			continue
		}
		found = true
		if in.InterfaceName != tunInterfaceName {
			t.Fatalf("tun inbound interface_name = %q, want %q", in.InterfaceName, tunInterfaceName)
		}
	}
	if !found {
		t.Fatal("tunnel config has no tun-in inbound")
	}
	// The pinned core decodes options with DisallowUnknownFields, so a field it
	// does not know is not an ignored knob — it is a dead engine for every node.
	assertCoreAcceptsConfig(t, cfg)
}
