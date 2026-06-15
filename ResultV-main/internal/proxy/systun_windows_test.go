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

func TestStaleTunRemovalScript_TargetsOnlyOurAdapter(t *testing.T) {
	s := staleTunRemovalScript()
	// Must select by the EXACT sing-tun description so a user's other tunnels
	// (Tailscale, WireGuard, OpenVPN TAP, …) are never removed.
	if !strings.Contains(s, "-InterfaceDescription '"+singTunAdapterDescription+"'") {
		t.Fatalf("removal must match the exact sing-tun description: %s", s)
	}
	if !strings.Contains(s, "pnputil /remove-device") {
		t.Fatalf("removal must delete the PnP device, got: %s", s)
	}
	// A broad "tun" substring match here would risk nuking unrelated adapters.
	if strings.Contains(s, "-InterfaceDescription '*tun*'") || strings.Contains(s, "Where-Object") {
		t.Fatalf("removal must not use a broad match: %s", s)
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

func TestRemoveStaleTunAdapter_NoAdapterIsNoOp(t *testing.T) {
	ran, elevated := withTunStubs(t, true, "\n")
	if err := removeStaleTunAdapter(); err != nil {
		t.Fatalf("removeStaleTunAdapter: %v", err)
	}
	if *ran != "" || *elevated != "" {
		t.Fatalf("no adapter detected → no removal; ran=%q elevated=%q", *ran, *elevated)
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
