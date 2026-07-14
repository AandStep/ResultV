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
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"resultproxy-wails/internal/system"

	"golang.org/x/sys/windows"
)

// singTunAdapterDescription is the fixed NIC description sing-tun gives the
// Wintun adapter it creates on Windows. Matching on this EXACT string (not a
// broad "tun" substring) is critical: it guarantees we only ever touch OUR
// leftover adapter and never the user's other tunnels (Radmin VPN, OpenVPN
// TAP, Tailscale, WireGuard, …), whose default routes we must not delete.
const singTunAdapterDescription = "sing-tun Tunnel"

// Indirections for tests so the cleanup can be exercised without a real adapter.
var (
	leftoverTunIfIndexesFn  = leftoverTunIfIndexesNative
	resetAdapterDNSNativeFn = resetAdapterDNSNative
	runCmdFn                = runCommandHidden
	tunPSRunElevated        = powerShellRunElevated
	tunIsAdmin              = system.IsAdmin
)

func runCommandHidden(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}

// leftoverTunIfIndexes returns the interface indexes of any sing-tun adapter
// currently present. At startup — before we have connected — a present sing-tun
// adapter can only be a leftover from a prior run that was force-killed or
// crashed: the in-process sing-box died, but Windows kept its Wintun adapter and
// (the real problem) the auto_route default route bound to it, black-holing all
// traffic. Read-only enumeration; works without admin.
func leftoverTunIfIndexes() []int {
	idxs, err := leftoverTunIfIndexesFn()
	if err != nil {
		return nil
	}
	return idxs
}

func leftoverTunIfIndexesNative() ([]int, error) {
	const family = uint32(windows.AF_UNSPEC)
	flags := uint32(windows.GAA_FLAG_SKIP_UNICAST | windows.GAA_FLAG_SKIP_ANYCAST | windows.GAA_FLAG_SKIP_MULTICAST | windows.GAA_FLAG_SKIP_DNS_SERVER)

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

	var idxs []int
	for cur := aa; cur != nil; cur = cur.Next {
		desc := windows.UTF16PtrToString(cur.Description)
		if desc == singTunAdapterDescription {
			idxs = append(idxs, int(cur.IfIndex))
		}
	}
	runtime.KeepAlive(buf)
	return idxs, nil
}

func parseIfIndexLines(out []byte) []int {
	out = stripBOM(out)
	var idxs []int
	for _, line := range strings.Split(string(out), "\n") {
		s := strings.TrimSpace(line)
		if s == "" {
			continue
		}
		if n, err := strconv.Atoi(s); err == nil {
			idxs = append(idxs, n)
		}
	}
	return idxs
}

// hasLeftoverTun reports whether a leftover sing-tun adapter is present.
func hasLeftoverTun() bool {
	return len(leftoverTunIfIndexes()) > 0
}

// clearLeftoverTun severs every leftover sing-tun adapter from the network: it
// deletes EVERY route bound to the adapter (so traffic falls back to the
// physical default route and the internet works again), releases the adapter's
// IPv6 ULA (so sing-tun can re-add it on the next boot instead of dying with
// "set ipv6 address: The object already exists" — see removeTunIPv6PS), and
// resets the adapter's DNS (so Windows' Smart Multi-Homed resolution stops
// querying the dead tunnel resolver). Routing/DNS/address changes require admin;
// without it we run the cleanup through an elevated (UAC) PowerShell, mirroring
// the DNS snapshot restore path. The dead adapter itself is left in place —
// sing-box reclaims it on the next connect — but with no routes it is inert.
//
// ALL routes, not just 0.0.0.0/0: auto_route with route_exclude_address (the
// server-IP pin sets it for every non-endpoint protocol) makes sing-tun install
// a disaggregated covering set (~32 prefixes: 0.0.0.0/1, 128.0.0.0/4, …) with
// NO 0.0.0.0/0 entry at all. A default-route-only delete removes nothing of
// that set, every metric-0 prefix keeps pointing at the dead adapter and the
// internet stays black-holed even though cleanup "succeeded". The adapter is
// exclusively ours (matched by the exact "sing-tun Tunnel" description), so
// removing all its routes is safe.
func clearLeftoverTun() error {
	idxs := leftoverTunIfIndexes()
	if len(idxs) == 0 {
		return nil
	}
	if tunIsAdmin() {
		var firstErr error
		for _, idx := range idxs {
			if err := resetAdapterDNSNativeFn(idx); err != nil && firstErr == nil {
				firstErr = err
			}
			_ = runCmdFn("powershell", "-NoProfile", "-NonInteractive", "-Command",
				removeAllRoutesPS(idx))
			_ = runCmdFn("powershell", "-NoProfile", "-NonInteractive", "-Command",
				removeTunIPv6PS(idx))
		}
		return firstErr
	}
	script := buildTunCleanupScript(idxs)
	return tunPSRunElevated(script)
}

// removeAllRoutesPS returns the PowerShell command that deletes every route
// bound to the given interface (both address families; "no routes" is silenced).
func removeAllRoutesPS(idx int) string {
	return fmt.Sprintf("Remove-NetRoute -InterfaceIndex %d -Confirm:$false -ErrorAction SilentlyContinue", idx)
}

// removeTunIPv6PS returns the PowerShell command that releases the adapter's
// IPv6 unicast addresses. The leftover sing-tun adapter keeps its ULA
// (fdfe:dcba:9876::1/126) bound; sing-tun reopens that same-named adapter on the
// next boot and re-adds the identical ULA, which Windows rejects with
// "set ipv6 address: The object already exists" (ERROR_OBJECT_ALREADY_EXISTS).
// Removing the address is synchronous — unlike async pnputil device removal,
// whose teardown can lag past the retry window and collide again — so by the time
// sing-tun re-adds the ULA the slot is free. Safe: the adapter is exclusively
// ours (matched by the exact "sing-tun Tunnel" description) and sing-tun
// reconfigures whatever addresses it needs when it reclaims the adapter.
func removeTunIPv6PS(idx int) string {
	return fmt.Sprintf("Remove-NetIPAddress -InterfaceIndex %d -AddressFamily IPv6 -Confirm:$false -ErrorAction SilentlyContinue", idx)
}

func buildTunCleanupCommands(idxs []int) []string {
	var cmds []string
	for _, idx := range idxs {
		cmds = append(cmds, fmt.Sprintf(`powershell -NoProfile -NonInteractive -Command "%s"`, removeAllRoutesPS(idx)))
		cmds = append(cmds, fmt.Sprintf(`powershell -NoProfile -NonInteractive -Command "%s"`, removeTunIPv6PS(idx)))
		cmds = append(cmds, fmt.Sprintf("netsh interface ipv4 set dnsservers name=%d source=dhcp", idx))
		cmds = append(cmds, fmt.Sprintf("netsh interface ipv6 set dnsservers name=%d source=dhcp", idx))
	}
	return cmds
}

func buildTunCleanupScript(idxs []int) string {
	var b strings.Builder
	for _, idx := range idxs {
		fmt.Fprintf(&b, "%s\n", removeAllRoutesPS(idx))
		fmt.Fprintf(&b, "%s\n", removeTunIPv6PS(idx))
		fmt.Fprintf(&b, "Set-DnsClientServerAddress -InterfaceIndex %d -ResetServerAddresses -ErrorAction SilentlyContinue\n", idx)
	}
	return b.String()
}

// staleTunRemovalScript returns the PowerShell that deletes the sing-tun Wintun
// *device* (not just its routes) from Windows. It selects the adapter by the
// exact "sing-tun Tunnel" description — never a broad "tun" match — so it can
// only ever remove OUR adapter, then hands the device's PnP instance id to
// pnputil. ErrorAction/2>$null keep it silent when no such adapter exists, so
// the script is a safe no-op on a clean machine.
func staleTunRemovalScript() string {
	return fmt.Sprintf(
		"Get-NetAdapter -InterfaceDescription '%s' -ErrorAction SilentlyContinue | "+
			"ForEach-Object { & pnputil /remove-device $_.PnpDeviceID 2>$null }",
		singTunAdapterDescription,
	)
}

// removeStaleTunAdapter deletes a wedged sing-tun adapter device so the next
// CreateAdapter builds a fresh one. clearLeftoverTun only strips routes/DNS and
// leaves the adapter for sing-box to reclaim — but when the adapter's backing
// Wintun session is dead (the device lingers in the stack yet its handle is
// gone), reopening it is what fails with "open interface take too much time" →
// "cannot find the file specified". Reusing that husk on every retry is why the
// tunnel used to come back only after a reboot; removing the device breaks that
// loop. No-op when no such adapter is present. Admin removes it directly;
// without admin we go through the elevated (UAC) PowerShell, mirroring
// clearLeftoverTun — though tunnel mode is admin-gated up front, so the direct
// path is the one that runs in practice.
func removeStaleTunAdapter() error {
	if len(leftoverTunIfIndexes()) == 0 {
		return nil
	}
	script := staleTunRemovalScript()
	if tunIsAdmin() {
		return runCmdFn("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	}
	return tunPSRunElevated(script)
}
