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
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"resultproxy-wails/internal/system"

	"golang.org/x/sys/windows"
)

// Indirections for tests so the cleanup can be exercised without a real adapter.
var (
	leftoverTunIfIndexesFn  = leftoverTunIfIndexesNative
	resetAdapterDNSNativeFn = resetAdapterDNSNative
	runCmdFn                = runCommandHidden
	runCmdOutFn             = runCommandHiddenOut
	tunPSRunElevated        = powerShellRunElevated
	tunIsAdmin              = system.IsAdmin
)

// tunCommandTimeout bounds every external command this file runs.
//
// pnputil's device removal can block indefinitely on a wedged network devnode,
// and exec.Command + cmd.Run() carry no deadline — connectCtx is deliberately
// NOT threaded into the engine-start path either, so a hang here stalled the
// whole connect with no further log line and nothing the Cancel button could
// reach. A var so tests can shorten it.
var tunCommandTimeout = 30 * time.Second

func runCommandHidden(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), tunCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return tunCommandError(ctx, name, cmd.Run())
}

// tunCommandError names the timeout that fired. CommandContext kills the child
// on expiry, and the bare "exit status 1" / "signal: killed" that comes back
// tells a reader nothing about why.
func tunCommandError(ctx context.Context, name string, err error) error {
	if err != nil && ctx.Err() != nil {
		return fmt.Errorf("%s не завершилась за %s и была снята", name, tunCommandTimeout)
	}
	return err
}

// runCommandHiddenOut is runCommandHidden with stdout captured, for the one
// caller that needs to know what the script actually did rather than just
// whether it exited cleanly. Output is returned even on a non-zero exit, so a
// partial run still reports the devices it managed to remove.
func runCommandHiddenOut(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tunCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	return stdout.Bytes(), tunCommandError(ctx, name, err)
}

// Windows CONFIGRET / CM_LOCATE_DEVNODE constants (cfgmgr32.h).
const (
	crSuccess              = 0
	cmLocateDevNodePhantom = 0x00000001
)

var (
	modCfgmgr32          = windows.NewLazySystemDLL("cfgmgr32.dll")
	procCMLocateDevNodeW = modCfgmgr32.NewProc("CM_Locate_DevNodeW")
)

// tunDevNodeExists reports whether a PnP device node is still in the device
// tree. CM_LOCATE_DEVNODE_PHANTOM is the whole point of using cfgmgr32 here: a
// non-present ("phantom") node is invisible to GetAdaptersAddresses and
// Get-NetAdapter, and it is precisely what blocks the next CreateAdapter — a
// readiness check that cannot see one would report "clear" while the node that
// wedges the retry is still sitting there.
//
// Native rather than another PowerShell spawn because this runs in a poll loop.
func tunDevNodeExists(instanceID string) bool {
	if err := procCMLocateDevNodeW.Find(); err != nil {
		// Without the probe we cannot tell. Report "gone" so the caller falls back
		// to its plain settle delay instead of burning the entire wait budget on
		// every retry.
		return false
	}
	idPtr, err := windows.UTF16PtrFromString(instanceID)
	if err != nil {
		return false
	}
	var devInst uint32
	ret, _, _ := procCMLocateDevNodeW.Call(
		uintptr(unsafe.Pointer(&devInst)),
		uintptr(unsafe.Pointer(idPtr)),
		uintptr(cmLocateDevNodePhantom),
	)
	runtime.KeepAlive(idPtr)
	return ret == crSuccess
}

// staleTunDevNodeGone reports whether OUR wedged Wintun node has actually left
// the device tree after removeStaleTunAdapter handed it to pnputil.
//
// Only our own GUID is waited on. The legacy tun0 node can belong to another
// vendor's running client — which ghostTunRemovalScript deliberately never
// removes while it is healthy — so waiting for that one to vanish would burn the
// full budget on every single retry.
func staleTunDevNodeGone() bool {
	return !tunDevNodeExists(tunPnpInstanceID(tunAdapterGUID))
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
		// AdapterName is the NetCfgInstanceId — the adapter GUID string, e.g.
		// "{0DCCC63E-...}". sing-tun derives it from the interface name, so it
		// identifies OUR adapter specifically; the "sing-tun Tunnel" description
		// this used to match is shared by every client built on the same core.
		if isOurTunAdapterGUID(windows.BytePtrToString(cur.AdapterName)) {
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
// exclusively ours (matched by our own Wintun adapter GUID, not by the
// "sing-tun Tunnel" description every client on this core shares), so
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
// ours (matched by our own Wintun adapter GUID) and sing-tun
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

// tunPnpInstanceID returns the Windows PnP instance id of the Wintun device
// sing-tun creates for the given adapter GUID. Wintun enumerates its adapters on
// the software device bus, so the id is always SWD\WINTUN\{GUID}.
func tunPnpInstanceID(guid string) string {
	return `SWD\WINTUN\` + guid
}

// removalReportPS returns the PowerShell that prints one tunRemovedMarker line
// per device the pipeline just handed to pnputil, carrying the instance id, the
// device's status and pnputil's exit code. Self-authored text, never a parse of
// pnputil's own output — that output is localised and would break the moment the
// user's Windows is not English.
func removalReportPS() string {
	return fmt.Sprintf(`Write-Output ("%s" + $_.InstanceId + " status=" + $_.Status + " rc=" + $LASTEXITCODE)`, tunRemovedMarker)
}

// ghostTunRemovalScript returns the PowerShell that deletes our wedged Wintun
// *device* (not just its routes) by its exact PnP instance id.
//
// Get-PnpDevice, not Get-NetAdapter: the failure this fixes is a NON-PRESENT
// ("ghost", Status: Unknown) device left behind by an unclean exit. Neither
// Get-NetAdapter nor GetAdaptersAddresses enumerates non-present devices, so
// every previous cleanup path looked straight past it while it kept squatting
// the devnode and degrading CreateAdapter into "configure tun interface: set
// ipv6 address: Element not found".
//
// -eq on the instance id, never -like or a description match: -like would treat
// [ ] as wildcards, and the "sing-tun Tunnel" description is shared by every
// client built on this core - matching it risks pnputil-removing someone else's
// live tunnel.
//
// The legacy GUID is additionally gated on the device NOT being present. A live
// "tun0" may belong to another running sing-box client, and deleting its devnode
// tears down that session irreversibly; a ghost tun0 belongs to nobody. Our own
// GUID needs no such gate - nobody else can hold it.
//
// ErrorAction/2>$null keep the script silent, so it is a safe no-op on a clean
// machine - which is why removeStaleTunAdapter no longer pre-checks anything.
func ghostTunRemovalScript() string {
	var b strings.Builder
	b.WriteString("$dev = Get-PnpDevice -Class Net -ErrorAction SilentlyContinue\n")
	fmt.Fprintf(&b, "$dev | Where-Object { $_.InstanceId -eq '%s' } | "+
		"ForEach-Object { & pnputil /remove-device $_.InstanceId 2>$null; %s }\n",
		tunPnpInstanceID(tunAdapterGUID), removalReportPS())
	fmt.Fprintf(&b, "$dev | Where-Object { $_.InstanceId -eq '%s' -and $_.Status -ne 'OK' } | "+
		"ForEach-Object { & pnputil /remove-device $_.InstanceId 2>$null; %s }\n",
		tunPnpInstanceID(tunAdapterGUIDLegacy), removalReportPS())
	return b.String()
}

// removeStaleTunAdapter deletes a wedged sing-tun adapter device so the next
// CreateAdapter builds a fresh one. clearLeftoverTun only strips routes/DNS and
// leaves the adapter for sing-box to reclaim - but when the adapter's backing
// Wintun session is dead (the device lingers in the stack yet its handle is
// gone), reopening it is what fails with "open interface take too much time" ->
// "cannot find the file specified". Reusing that husk on every retry is why the
// tunnel used to come back only after a reboot; removing the device breaks that
// loop.
//
// There is deliberately NO pre-check for a present adapter here. The check this
// used to do went through GetAdaptersAddresses, which does not enumerate
// non-present devices - so in the ghost case, the one that actually wedges
// startup, it returned "nothing to do" and the retry died on the same ghost.
// ghostTunRemovalScript is already a silent no-op on a clean machine, so the
// only cost of always running it is one PowerShell spawn on a path that has
// just failed to start the tunnel anyway. Admin removes the device directly;
// without admin we go through the elevated (UAC) PowerShell, mirroring
// clearLeftoverTun - though tunnel mode is admin-gated up front, so the direct
// path is the one that runs in practice.
//
// Returns the devices it actually tore down (one entry per device), so the retry
// path can log "removed X" and "found nothing" as the different outcomes they
// are instead of one indistinguishable line.
func removeStaleTunAdapter() ([]string, error) {
	script := ghostTunRemovalScript()
	if tunIsAdmin() {
		out, err := runCmdOutFn("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
		return parseRemovedTunDevices(out), err
	}
	// The elevated path runs the script in a separate UAC-launched process whose
	// stdout we cannot capture, so it reports no detail. Tunnel mode is admin-gated
	// up front, so this is the exceptional path.
	return nil, tunPSRunElevated(script)
}
