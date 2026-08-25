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

import "strings"

// hasLeftoverTunFn / clearLeftoverTunFn indirect through package vars so the
// Manager's recovery path can be unit-tested without a real sing-tun adapter or
// touching the OS routing table. Production wiring points at the per-platform
// implementations (systun_windows.go / systun_other.go).
var (
	hasLeftoverTunFn        = hasLeftoverTun
	clearLeftoverTunFn      = clearLeftoverTun
	removeStaleTunAdapterFn = removeStaleTunAdapter
)

// tunInterfaceName is the Windows TUN interface name we hand sing-box via
// `interface_name`. Two hard requirements bind this constant:
//
//  1. It must be UNIQUE across sing-box clients. sing-tun derives the Wintun
//     adapter's GUID deterministically from this name — generateGUIDByDeviceName
//     is md5("wintun"+name) reinterpreted as a GUID — so every client that leaves
//     interface_name empty lands on sing-box's default "tun0" and therefore on
//     the same GUID {0DCCC63E-...}. A leftover ghost device from ANY of them then
//     squats our devnode and CreateAdapter degrades into "configure tun
//     interface: set ipv6 address: Element not found", with no way out but a
//     reboot.
//
//  2. It must contain the substring "tun". looksLikeTunnelInterface recognises
//     tunnel adapters by that substring, and three call sites depend on it
//     classifying OUR adapter as a tunnel: skipAdapterDNS (else our TUN lands in
//     the physical-adapter DNS snapshot, gets pinned to public resolvers, and
//     Restore fails on the vanished interface index), systemHasIPv6 (else a
//     leftover of ours fakes host IPv6 support and brings back the very
//     "set ipv6 address" failure), and the Ping LAN bind (else Ping binds to our
//     own TUN).
//
// Changing this name WITHOUT recomputing tunAdapterGUID silently blinds every
// cleanup path — TestTunAdapterGUIDMatchesInterfaceName exists to stop that.
const tunInterfaceName = "rvtun0"

// tunAdapterGUID is the Wintun adapter GUID sing-tun derives from
// tunInterfaceName. tunAdapterGUIDLegacy is the one our pre-rename builds got
// from sing-box's default "tun0"; it is kept forever, because an upgrading user
// can still have that adapter — or its ghost — wedged on disk.
const (
	tunAdapterGUID       = "{1F2204B3-7F00-E47C-7441-6763D2F86416}"
	tunAdapterGUIDLegacy = "{0DCCC63E-5622-3880-1E09-7CC9C46AD7B4}"
)

// isOurTunAdapterGUID reports whether a Windows adapter GUID string — the
// NetCfgInstanceId, which GetAdaptersAddresses reports in AdapterName — is one
// of ours. Matching the GUID rather than the "sing-tun Tunnel" description is
// what keeps us off other people's tunnels: that description is a hardcoded
// constant inside sing-tun and is identical for every app built on the core.
func isOurTunAdapterGUID(adapterGUID string) bool {
	return strings.EqualFold(adapterGUID, tunAdapterGUID) ||
		strings.EqualFold(adapterGUID, tunAdapterGUIDLegacy)
}

// tunRemovedMarker prefixes the line ghostTunRemovalScript prints for every PnP
// device it actually tore down. We print our OWN marker rather than parsing
// pnputil's output because that output is localised — on a Russian Windows it
// comes back as cp866 Cyrillic — so any parse of it would be a locale trap.
const tunRemovedMarker = "RVTUN-REMOVED "

// parseRemovedTunDevices extracts what ghostTunRemovalScript reported removing,
// one entry per device ("<instance id> status=<s> rc=<n>").
//
// This exists so the retry path can tell the two outcomes apart in the log.
// Before it, removeStaleTunAdapter returned only an error, so "cleanup tore down
// a wedged device" and "cleanup found nothing at all" produced the identical
// line — and a silent no-op reading as success is precisely what let the ghost
// bug survive in the field for so long.
func parseRemovedTunDevices(out []byte) []string {
	var removed []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, tunRemovedMarker) {
			continue
		}
		if detail := strings.TrimSpace(strings.TrimPrefix(line, tunRemovedMarker)); detail != "" {
			removed = append(removed, detail)
		}
	}
	return removed
}
