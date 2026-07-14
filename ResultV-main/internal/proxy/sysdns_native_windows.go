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

//go:build windows

package proxy

// Native (PowerShell-free) adapter DNS list/set/reset.
//
// The PowerShell-based implementation spawns powershell.exe once per call plus
// once per adapter; a cold interpreter start is ~300-600ms, which dominated both
// connect (Override) and disconnect (Restore). These functions call the iphlpapi
// equivalents directly — no process spawn, sub-millisecond.
//
// They are the *fast path*: the dispatchers in sysdns_windows.go fall back to the
// PowerShell implementation if a native call errors or (for apply) fails a
// read-back verification, so a subtle Win32 mistake degrades to "as slow as
// before", never to a broken leak-protection feature.

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modIPHLPAPI                     = windows.NewLazySystemDLL("iphlpapi.dll")
	procSetInterfaceDnsSettings     = modIPHLPAPI.NewProc("SetInterfaceDnsSettings")
	procConvertInterfaceIndexToLuid = modIPHLPAPI.NewProc("ConvertInterfaceIndexToLuid")
	procConvertInterfaceLuidToGuid  = modIPHLPAPI.NewProc("ConvertInterfaceLuidToGuid")
)

const (
	dnsInterfaceSettingsVersion1 = 1
	dnsSettingNameServer         = 0x0002 // DNS_SETTING_NAMESERVER (IPv4 servers)
)

// dnsInterfaceSettings mirrors Win32 DNS_INTERFACE_SETTINGS (netioapi.h). On
// amd64 Go inserts the same 4-byte gap between Version and the 8-byte-aligned
// Flags that the C compiler does, so the layout matches.
type dnsInterfaceSettings struct {
	Version             uint32
	Flags               uint64
	Domain              *uint16
	NameServer          *uint16
	SearchList          *uint16
	RegistrationEnabled uint32
	RegisterAdapterName uint32
	EnableLLMNR         uint32
	QueryAdapterName    uint32
	ProfileNameServer   *uint16
}

// interfaceGUIDForIndex resolves an interface index to the interface GUID that
// SetInterfaceDnsSettings expects (index → LUID → GUID).
func interfaceGUIDForIndex(ifIdx int) (windows.GUID, error) {
	var luid uint64
	if ret, _, _ := procConvertInterfaceIndexToLuid.Call(
		uintptr(uint32(ifIdx)), uintptr(unsafe.Pointer(&luid)),
	); ret != 0 {
		return windows.GUID{}, fmt.Errorf("ConvertInterfaceIndexToLuid(%d): status %d", ifIdx, ret)
	}
	var guid windows.GUID
	if ret, _, _ := procConvertInterfaceLuidToGuid.Call(
		uintptr(unsafe.Pointer(&luid)), uintptr(unsafe.Pointer(&guid)),
	); ret != 0 {
		return windows.GUID{}, fmt.Errorf("ConvertInterfaceLuidToGuid: status %d", ret)
	}
	return guid, nil
}

// setInterfaceDNSNative applies (reset=false) or clears to DHCP (reset=true) the
// IPv4 nameservers on the interface identified by guid. The GUID is 16 bytes, so
// the x64 ABI passes it by reference — hence &guid, not the value.
func setInterfaceDNSNative(guid windows.GUID, servers []string, reset bool) error {
	settings := dnsInterfaceSettings{
		Version: dnsInterfaceSettingsVersion1,
		Flags:   dnsSettingNameServer,
	}
	var ns *uint16
	if !reset && len(servers) > 0 {
		p, err := windows.UTF16PtrFromString(strings.Join(servers, ","))
		if err != nil {
			return err
		}
		ns = p
		settings.NameServer = ns
	}
	// reset: NameServer stays nil → SetInterfaceDnsSettings reverts to DHCP.
	ret, _, _ := procSetInterfaceDnsSettings.Call(
		uintptr(unsafe.Pointer(&guid)),
		uintptr(unsafe.Pointer(&settings)),
	)
	runtime.KeepAlive(ns)
	runtime.KeepAlive(&settings)
	if ret != 0 {
		return fmt.Errorf("SetInterfaceDnsSettings: status %d", ret)
	}
	return nil
}

func setAdapterDNSNative(ifIdx int, servers []string) error {
	clean := make([]string, 0, len(servers))
	for _, s := range servers {
		s = strings.TrimSpace(s)
		if !isSafeDNSToken(s) {
			return fmt.Errorf("unsafe dns server token: %q", s)
		}
		clean = append(clean, s)
	}
	guid, err := interfaceGUIDForIndex(ifIdx)
	if err != nil {
		return err
	}
	return setInterfaceDNSNative(guid, clean, false)
}

func resetAdapterDNSNative(ifIdx int) error {
	guid, err := interfaceGUIDForIndex(ifIdx)
	if err != nil {
		return err
	}
	return setInterfaceDNSNative(guid, nil, true)
}

// listAdapterDNSNative enumerates IPv4 nameservers per adapter via
// GetAdaptersAddresses, returning the same shape as the PowerShell lister so the
// snapshot format is unchanged. Tunnel/virtual adapters are dropped by
// filterPhysicalAdapterDNS, matching the PowerShell path.
func listAdapterDNSNative() ([]adapterDNS, error) {
	const family = uint32(windows.AF_INET)
	flags := uint32(windows.GAA_FLAG_SKIP_UNICAST | windows.GAA_FLAG_SKIP_ANYCAST | windows.GAA_FLAG_SKIP_MULTICAST)

	var size uint32
	err := windows.GetAdaptersAddresses(family, flags, 0, nil, &size)
	if err != nil && !errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
		return nil, err
	}
	if size == 0 {
		return nil, nil
	}
	buf := make([]byte, size)
	aa := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
	if err := windows.GetAdaptersAddresses(family, flags, 0, aa, &size); err != nil {
		return nil, err
	}

	var out []adapterDNS
	for cur := aa; cur != nil; cur = cur.Next {
		var servers []string
		for dns := cur.FirstDnsServerAddress; dns != nil; dns = dns.Next {
			ip := dns.Address.IP()
			if ip == nil {
				continue
			}
			if v4 := ip.To4(); v4 != nil {
				servers = append(servers, v4.String())
			}
		}
		if len(servers) == 0 {
			continue
		}
		out = append(out, adapterDNS{
			InterfaceIndex:  int(cur.IfIndex),
			InterfaceAlias:  windows.UTF16PtrToString(cur.FriendlyName),
			ServerAddresses: servers,
		})
	}
	runtime.KeepAlive(buf)
	return filterPhysicalAdapterDNS(out), nil
}

// verifyAdapterDNS confirms the interface's live IPv4 nameservers match want (as
// a set). Used after a native apply so a silent failure (API returns success but
// nothing actually changed, or it touched the wrong interface) is caught and the
// caller falls back to PowerShell — this is a leak-protection feature, so a
// confirmed-applied state matters more than raw speed.
func verifyAdapterDNS(ifIdx int, want []string) bool {
	adapters, err := listAdapterDNSNative()
	if err != nil {
		return false
	}
	wantSet := make(map[string]struct{}, len(want))
	for _, s := range want {
		wantSet[strings.TrimSpace(s)] = struct{}{}
	}
	for _, a := range adapters {
		if a.InterfaceIndex != ifIdx {
			continue
		}
		if len(a.ServerAddresses) != len(wantSet) {
			return false
		}
		for _, s := range a.ServerAddresses {
			if _, ok := wantSet[s]; !ok {
				return false
			}
		}
		return true
	}
	return false
}
