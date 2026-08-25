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

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseIfIndexLines(t *testing.T) {
	got := parseIfIndexLines([]byte("3\r\n14\n  7  \n\nnotnum\n"))
	want := []int{3, 14, 7}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestBuildTunCleanupScript(t *testing.T) {
	s := buildTunCleanupScript([]int{3, 9})
	for _, want := range []string{
		"Remove-NetRoute -InterfaceIndex 3 -Confirm:$false",
		"Set-DnsClientServerAddress -InterfaceIndex 3 -ResetServerAddresses",
		"Remove-NetRoute -InterfaceIndex 9 -Confirm:$false",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("script missing %q:\n%s", want, s)
		}
	}
	// Regression: auto_route with route_exclude_address (server-IP pin) installs
	// a disaggregated prefix set with NO 0.0.0.0/0 entry, so a cleanup filtered
	// to the default route deletes nothing and leaves the internet black-holed.
	// The script must remove ALL routes bound to the adapter.
	if strings.Contains(s, "-DestinationPrefix") {
		t.Fatalf("cleanup must remove all adapter routes, found prefix filter:\n%s", s)
	}
}

// A leftover sing-tun adapter keeps its IPv6 ULA (fdfe:dcba:9876::1/126) bound.
// clearLeftoverTun stripping only routes/DNS leaves that address in place, so the
// next sing-tun boot re-adds the same ULA and dies with "set ipv6 address: The
// object already exists" — fixable today only by a reboot. The cleanup must also
// release the IPv6 address (synchronously, unlike async pnputil device removal).
func TestBuildTunCleanupScript_RemovesIPv6Address(t *testing.T) {
	s := buildTunCleanupScript([]int{3, 9})
	for _, want := range []string{
		"Remove-NetIPAddress -InterfaceIndex 3 -AddressFamily IPv6",
		"Remove-NetIPAddress -InterfaceIndex 9 -AddressFamily IPv6",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("script missing IPv6 release %q:\n%s", want, s)
		}
	}
}

func TestBuildTunCleanupCommands_RemovesIPv6Address(t *testing.T) {
	cmds := buildTunCleanupCommands([]int{3})
	want := "Remove-NetIPAddress -InterfaceIndex 3 -AddressFamily IPv6"
	found := false
	for _, cmd := range cmds {
		if strings.Contains(cmd, want) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected an IPv6-release command containing %q in %v", want, cmds)
	}
}

func TestClearLeftoverTun_AdminReleasesIPv6Address(t *testing.T) {
	ran, _ := withTunStubs(t, true, "3\n")
	if err := clearLeftoverTun(); err != nil {
		t.Fatalf("clearLeftoverTun: %v", err)
	}
	if !strings.Contains(*ran, "Remove-NetIPAddress -InterfaceIndex 3 -AddressFamily IPv6") {
		t.Fatalf("admin cleanup must release the adapter's IPv6 address: %q", *ran)
	}
}

func TestClearLeftoverTun_NonAdminReleasesIPv6Address(t *testing.T) {
	_, elevated := withTunStubs(t, false, "5\n")
	if err := clearLeftoverTun(); err != nil {
		t.Fatalf("clearLeftoverTun: %v", err)
	}
	if !strings.Contains(*elevated, "Remove-NetIPAddress -InterfaceIndex 5 -AddressFamily IPv6") {
		t.Fatalf("elevated cleanup must release the adapter's IPv6 address: %q", *elevated)
	}
}

// withTunStubs swaps the PowerShell runners + admin check; detect returns the
// given ifIndex output for the Get-NetAdapter probe.
func withTunStubs(t *testing.T, admin bool, detectOut string) (ran *string, elevated *string) {
	t.Helper()
	prevDetect, prevReset, prevRunCmd, prevElev, prevAdmin := leftoverTunIfIndexesFn, resetAdapterDNSNativeFn, runCmdFn, tunPSRunElevated, tunIsAdmin
	t.Cleanup(func() {
		leftoverTunIfIndexesFn = prevDetect
		resetAdapterDNSNativeFn = prevReset
		runCmdFn = prevRunCmd
		tunPSRunElevated = prevElev
		tunIsAdmin = prevAdmin
	})
	ran, elevated = new(string), new(string)
	tunIsAdmin = func() bool { return admin }

	idxs := parseIfIndexLines([]byte(detectOut))
	leftoverTunIfIndexesFn = func() ([]int, error) {
		return idxs, nil
	}

	var cmds []string
	resetAdapterDNSNativeFn = func(ifIdx int) error {
		cmds = append(cmds, fmt.Sprintf("Set-DnsClientServerAddress -InterfaceIndex %d -ResetServerAddresses", ifIdx))
		*ran = strings.Join(cmds, "\n")
		return nil
	}
	runCmdFn = func(name string, args ...string) error {
		cmds = append(cmds, name+" "+strings.Join(args, " "))
		*ran = strings.Join(cmds, "\n")
		return nil
	}
	tunPSRunElevated = func(script string) error {
		*elevated = script
		return nil
	}
	return ran, elevated
}

func TestClearLeftoverTun_AdminRunsDirectly(t *testing.T) {
	ran, elevated := withTunStubs(t, true, "3\n")
	if err := clearLeftoverTun(); err != nil {
		t.Fatalf("clearLeftoverTun: %v", err)
	}
	if *elevated != "" {
		t.Fatalf("admin path must not elevate, got: %q", *elevated)
	}
	if !strings.Contains(*ran, "InterfaceIndex 3") {
		t.Fatalf("cleanup did not target detected adapter: %q", *ran)
	}
	if !strings.Contains(*ran, "Remove-NetRoute -InterfaceIndex 3") {
		t.Fatalf("cleanup must remove the adapter's routes: %q", *ran)
	}
	// See TestBuildTunCleanupScript: a default-route-only delete misses the
	// disaggregated auto_route prefix set and strands the internet.
	if strings.Contains(*ran, "-DestinationPrefix") || strings.Contains(*ran, "0.0.0.0/0") {
		t.Fatalf("cleanup must remove all adapter routes, not just the default route: %q", *ran)
	}
}

func TestClearLeftoverTun_NonAdminElevates(t *testing.T) {
	ran, elevated := withTunStubs(t, false, "5\n")
	if err := clearLeftoverTun(); err != nil {
		t.Fatalf("clearLeftoverTun: %v", err)
	}
	if *ran != "" {
		t.Fatalf("non-admin path must not run cleanup directly, got: %q", *ran)
	}
	if !strings.Contains(*elevated, "InterfaceIndex 5") {
		t.Fatalf("elevated cleanup did not target detected adapter: %q", *elevated)
	}
	if strings.Contains(*elevated, "-DestinationPrefix") {
		t.Fatalf("elevated cleanup must remove all adapter routes, found prefix filter: %q", *elevated)
	}
}

func TestClearLeftoverTun_NoAdapterIsNoOp(t *testing.T) {
	ran, elevated := withTunStubs(t, true, "\n")
	if err := clearLeftoverTun(); err != nil {
		t.Fatalf("clearLeftoverTun: %v", err)
	}
	if *ran != "" || *elevated != "" {
		t.Fatalf("no adapter detected → no cleanup; ran=%q elevated=%q", *ran, *elevated)
	}
}

func TestGhostTunRemovalScript_TargetsOurInstanceIDs(t *testing.T) {
	s := ghostTunRemovalScript()
	// Must enumerate PnP devices, not net adapters: Get-NetAdapter and
	// GetAdaptersAddresses both omit non-present ("ghost") devices, and a ghost is
	// exactly what wedges CreateAdapter.
	if !strings.Contains(s, "Get-PnpDevice") {
		t.Fatalf("removal must enumerate PnP devices to see ghosts, got: %s", s)
	}
	if strings.Contains(s, "Get-NetAdapter") {
		t.Fatalf("Get-NetAdapter cannot see ghost devices, got: %s", s)
	}
	if !strings.Contains(s, tunPnpInstanceID(tunAdapterGUID)) {
		t.Fatalf("removal must target our instance id, got: %s", s)
	}
	if !strings.Contains(s, tunPnpInstanceID(tunAdapterGUIDLegacy)) {
		t.Fatalf("removal must target the legacy instance id, got: %s", s)
	}
	if !strings.Contains(s, "pnputil /remove-device") {
		t.Fatalf("removal must delete the PnP device, got: %s", s)
	}
}

func TestTunPnpInstanceID(t *testing.T) {
	if got, want := tunPnpInstanceID(tunAdapterGUID), `SWD\WINTUN\`+tunAdapterGUID; got != want {
		t.Fatalf("tunPnpInstanceID = %q, want %q", got, want)
	}
}

func TestGhostTunRemovalScript_UsesExactMatchOnly(t *testing.T) {
	s := ghostTunRemovalScript()
	// -eq is exact and has no wildcard semantics; -like would treat [ ] as
	// wildcards, and a description match would sweep up other people's tunnels
	// (Tailscale, WireGuard, another sing-box client).
	if !strings.Contains(s, "-eq") {
		t.Fatalf("removal must match InstanceId with -eq, got: %s", s)
	}
	for _, bad := range []string{"-like", "*tun*", "InterfaceDescription", "sing-tun Tunnel"} {
		if strings.Contains(s, bad) {
			t.Fatalf("removal must not use a broad match (%q), got: %s", bad, s)
		}
	}
}

// A live "tun0" can belong to ANOTHER running sing-box client: deleting its
// devnode tears down someone else's session irreversibly. A ghost tun0 belongs
// to nobody. Our own GUID has no such ambiguity - nobody else can hold it.
func TestGhostTunRemovalScript_LegacyGUIDOnlyWhenNotPresent(t *testing.T) {
	s := ghostTunRemovalScript()
	var ourLine, legacyLine string
	for _, ln := range strings.Split(s, "\n") {
		if strings.Contains(ln, tunAdapterGUID) {
			ourLine = ln
		}
		if strings.Contains(ln, tunAdapterGUIDLegacy) {
			legacyLine = ln
		}
	}
	if ourLine == "" || legacyLine == "" {
		t.Fatalf("expected one line per GUID, got: %s", s)
	}
	if !strings.Contains(legacyLine, "Status -ne 'OK'") {
		t.Fatalf("legacy GUID must be gated on the device not being present, got: %s", legacyLine)
	}
	if strings.Contains(ourLine, "Status -ne 'OK'") {
		t.Fatalf("our own GUID must be removed regardless of presence, got: %s", ourLine)
	}
}

func TestRemoveStaleTunAdapter_AdminRunsDirectly(t *testing.T) {
	ran, elevated := withTunStubs(t, true, "3\n")
	if err := removeStaleTunAdapter(); err != nil {
		t.Fatalf("removeStaleTunAdapter: %v", err)
	}
	if *elevated != "" {
		t.Fatalf("admin path must not elevate, got: %q", *elevated)
	}
	if !strings.Contains(*ran, "pnputil /remove-device") {
		t.Fatalf("admin path must run the removal directly, got: %q", *ran)
	}
}

func TestRemoveStaleTunAdapter_NonAdminElevates(t *testing.T) {
	ran, elevated := withTunStubs(t, false, "5\n")
	if err := removeStaleTunAdapter(); err != nil {
		t.Fatalf("removeStaleTunAdapter: %v", err)
	}
	if *ran != "" {
		t.Fatalf("non-admin path must not run removal directly, got: %q", *ran)
	}
	if !strings.Contains(*elevated, "pnputil /remove-device") {
		t.Fatalf("non-admin path must elevate the removal, got: %q", *elevated)
	}
}

// THE regression test for the reported failure. removeStaleTunAdapter used to
// bail out when GetAdaptersAddresses reported no present adapter - which is
// precisely the ghost case - so the retry path logged "removing the wedged
// adapter and retrying", removed nothing, and the second attempt died on the
// same ghost. Removal must run even with zero present adapters.
func TestRemoveStaleTunAdapter_RunsWithNoPresentAdapter(t *testing.T) {
	ran, elevated := withTunStubs(t, true, "\n")
	if err := removeStaleTunAdapter(); err != nil {
		t.Fatalf("removeStaleTunAdapter: %v", err)
	}
	if *elevated != "" {
		t.Fatalf("admin path must not elevate, got: %q", *elevated)
	}
	if !strings.Contains(*ran, "pnputil /remove-device") {
		t.Fatalf("ghost removal must run even when no adapter is present, ran=%q", *ran)
	}
	if !strings.Contains(*ran, "Get-PnpDevice") {
		t.Fatalf("ghost removal must enumerate PnP devices, ran=%q", *ran)
	}
}

func TestRemoveStaleTunAdapter_NonAdminElevatesWithNoPresentAdapter(t *testing.T) {
	ran, elevated := withTunStubs(t, false, "\n")
	if err := removeStaleTunAdapter(); err != nil {
		t.Fatalf("removeStaleTunAdapter: %v", err)
	}
	if *ran != "" {
		t.Fatalf("non-admin path must not run removal directly, got: %q", *ran)
	}
	if !strings.Contains(*elevated, "pnputil /remove-device") {
		t.Fatalf("non-admin path must elevate the ghost removal, got: %q", *elevated)
	}
}

func TestBuildTunCleanupCommands(t *testing.T) {
	cmds := buildTunCleanupCommands([]int{3, 7})
	expected := []string{
		`powershell -NoProfile -NonInteractive -Command "Remove-NetRoute -InterfaceIndex 3 -Confirm:$false -ErrorAction SilentlyContinue"`,
		"netsh interface ipv4 set dnsservers name=3 source=dhcp",
		"netsh interface ipv6 set dnsservers name=3 source=dhcp",
		`powershell -NoProfile -NonInteractive -Command "Remove-NetRoute -InterfaceIndex 7 -Confirm:$false -ErrorAction SilentlyContinue"`,
	}
	for _, exp := range expected {
		found := false
		for _, cmd := range cmds {
			if cmd == exp {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected command %q not found in %v", exp, cmds)
		}
	}
	for _, cmd := range cmds {
		if strings.Contains(cmd, "delete route 0.0.0.0/0") {
			t.Fatalf("cleanup must remove all adapter routes, not just the default route: %q", cmd)
		}
	}
}
