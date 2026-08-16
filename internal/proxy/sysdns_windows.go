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
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"resultproxy-wails/internal/system"
)

const dnsSnapshotFile = "dns-snapshot.json"

// dnsAdminCheck is overridden in tests.
var dnsAdminCheck = system.IsAdmin

type adapterDNS struct {
	InterfaceIndex  int      `json:"InterfaceIndex"`
	InterfaceAlias  string   `json:"InterfaceAlias"`
	ServerAddresses []string `json:"ServerAddresses"`
}

type dnsSnapshot struct {
	Adapters []adapterDNS `json:"adapters"`
}

type windowsSystemDNS struct {
	snapshotPath string
	timings      dnsPhaseTimings
}

func newSystemDNS() SystemDNS {
	return &windowsSystemDNS{
		snapshotPath: filepath.Join(resultProxyDataDir(), dnsSnapshotFile),
	}
}

// SnapshotExists reports whether a DNS snapshot is on disk — a previous run
// overrode the system resolver and never restored it (crash / force-kill).
func (w *windowsSystemDNS) SnapshotExists() bool {
	_, err := os.Stat(w.snapshotPath)
	return err == nil
}

func (w *windowsSystemDNS) Override(servers []string) error {
	if len(servers) == 0 {
		return errors.New("system dns override: empty server list")
	}
	if !dnsAdminCheck() {
		return ErrDNSRequiresAdmin
	}

	w.timings.reset()

	tList := time.Now()
	adapters, listPS, err := listAdapterDNS()
	w.timings.recordList(time.Since(tList), listPS)
	if err != nil {
		return fmt.Errorf("enumerate adapter dns: %w", err)
	}

	// Persist BEFORE mutating anything. If we crash after persisting but
	// before applying, Restore() at next start is a no-op on adapters that
	// were never touched. If we crash mid-apply, Restore reads the snapshot
	// and reverts what we did touch — also safe.
	tSnapshot := time.Now()
	err = writeSnapshot(w.snapshotPath, adapters)
	w.timings.recordSnapshot(time.Since(tSnapshot))
	if err != nil {
		return fmt.Errorf("save dns snapshot: %w", err)
	}

	var firstErr error
	for _, a := range adapters {
		if skipAdapterDNS(a.InterfaceAlias) {
			continue
		}
		tSet := time.Now()
		usedPS, setErr := setAdapterDNS(a.InterfaceIndex, servers)
		w.timings.recordSet(time.Since(tSet), usedPS)
		if setErr != nil && firstErr == nil {
			firstErr = fmt.Errorf("set dns on %q: %w", a.InterfaceAlias, setErr)
		}
	}
	return firstErr
}

// TakeDNSTimings returns the per-step breakdown of the last override and clears
// it. Implements dnsTimingSource. See dnsPhaseTimings.
func (w *windowsSystemDNS) TakeDNSTimings() string {
	return w.timings.take()
}

// OverrideTunnelAdapter sets the resolver on the sing-tun adapter (located by
// its interface address) so Windows DNS queries are routed into the TUN and
// answered by sing-box's hijack-dns rule through the tunnel. Requires admin —
// tunnel mode already is. No snapshot: the adapter vanishes on disconnect.
func (w *windowsSystemDNS) OverrideTunnelAdapter(adapterIP, dnsIP string) error {
	if !dnsAdminCheck() {
		return ErrDNSRequiresAdmin
	}
	idx, err := findInterfaceIndexByIP(adapterIP)
	if err != nil {
		return err
	}
	tSet := time.Now()
	usedPS, err := setAdapterDNS(idx, []string{dnsIP})
	w.timings.recordTun(time.Since(tSet), usedPS)
	return err
}

// findInterfaceIndexByIP returns the OS interface index of the adapter that
// carries ifaceIP as one of its unicast addresses.
func findInterfaceIndexByIP(ifaceIP string) (int, error) {
	want := net.ParseIP(ifaceIP)
	if want == nil {
		return 0, fmt.Errorf("bad interface ip %q", ifaceIP)
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return 0, fmt.Errorf("enumerate interfaces: %w", err)
	}
	for _, ifi := range ifaces {
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && ipnet.IP.Equal(want) {
				return ifi.Index, nil
			}
		}
	}
	return 0, fmt.Errorf("adapter with ip %s not found", ifaceIP)
}

func (w *windowsSystemDNS) Restore() error {
	data, err := os.ReadFile(w.snapshotPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read dns snapshot: %w", err)
	}

	var snap dnsSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		// Bad snapshot is worse than a stale one — drop it so we don't
		// re-enter restore forever on every start.
		_ = os.Remove(w.snapshotPath)
		return fmt.Errorf("parse dns snapshot: %w", err)
	}

	var firstErr error
	for _, a := range snap.Adapters {
		if skipAdapterDNS(a.InterfaceAlias) {
			continue
		}
		var err error
		if len(a.ServerAddresses) == 0 {
			_, err = resetAdapterDNS(a.InterfaceIndex)
		} else {
			_, err = setAdapterDNS(a.InterfaceIndex, a.ServerAddresses)
		}
		if err != nil {
			// TUN is torn down before Restore runs; a stale snapshot may still
			// reference its old InterfaceIndex — treat as success.
			if isAdapterGoneError(err) {
				continue
			}
			if firstErr == nil {
				firstErr = fmt.Errorf("restore dns on %q: %w", a.InterfaceAlias, err)
			}
		}
	}

	// Remove the snapshot only on success; otherwise the next start gets
	// another shot at restoring.
	if firstErr != nil && isPermissionDeniedError(firstErr) && !dnsAdminCheck() {
		if err := w.restoreElevated(snap.Adapters); err == nil {
			_ = os.Remove(w.snapshotPath)
			return nil
		}
		return fmt.Errorf("%w: %v", ErrDNSRequiresAdmin, firstErr)
	}

	if firstErr == nil {
		_ = os.Remove(w.snapshotPath)
	}
	return firstErr
}

func writeSnapshot(path string, adapters []adapterDNS) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(dnsSnapshot{Adapters: adapters})
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// listAdapterDNS / setAdapterDNS / resetAdapterDNS dispatch to the native
// (iphlpapi) fast path and fall back to PowerShell on error — and, for the
// apply path, on a read-back verification mismatch. Native is sub-millisecond;
// PowerShell costs a process spawn per call. Worst case is therefore today's
// speed, never a broken leak-protection feature. Whether native or PowerShell
// ran is visible via the "dns=Xms" figure in the connect timing line.
// listAdapterDNS enumerates per-adapter DNS. The bool reports whether the
// PowerShell fallback was used — it costs 300-700 ms, so the connect log
// surfaces it (see dnsPhaseTimings).
func listAdapterDNS() ([]adapterDNS, bool, error) {
	if adapters, err := listAdapterDNSNative(); err == nil {
		return adapters, false, nil
	}
	adapters, err := listAdapterDNSPowerShell()
	return adapters, true, err
}

// adapterDNSVerify* bound the read-back retry. A freshly created TUN adapter
// does not always report its new nameservers on the first GetAdaptersAddresses
// call, and treating that as failure sent every tunnel connect through
// PowerShell — a process spawn worth ~3.5s against ~26ms for the native path on
// a settled adapter. Three quick looks cost at most 100ms and remove the
// fallback in the normal case without weakening the "applied and confirmed"
// guarantee: if it never confirms, PowerShell still runs.
const (
	adapterDNSVerifyAttempts = 3
	adapterDNSVerifyDelay    = 50 * time.Millisecond
)

// Indirections so the retry policy is testable without touching real adapters.
var (
	setAdapterDNSNativeFn     = setAdapterDNSNative
	verifyAdapterDNSFn        = verifyAdapterDNS
	setAdapterDNSPowerShellFn = setAdapterDNSPowerShell
)

// setAdapterDNS applies servers to one adapter. The bool reports whether the
// PowerShell fallback was used.
func setAdapterDNS(ifIdx int, servers []string) (bool, error) {
	if err := setAdapterDNSNativeFn(ifIdx, servers); err == nil {
		for attempt := 0; attempt < adapterDNSVerifyAttempts; attempt++ {
			if attempt > 0 {
				time.Sleep(adapterDNSVerifyDelay)
			}
			if verifyAdapterDNSFn(ifIdx, servers) {
				return false, nil
			}
		}
	}
	return true, setAdapterDNSPowerShellFn(ifIdx, servers)
}

// resetAdapterDNS reverts one adapter to DHCP. The bool reports whether the
// PowerShell fallback was used.
func resetAdapterDNS(ifIdx int) (bool, error) {
	// Reset reverts to DHCP, whose servers we can't predict, so there's nothing
	// to verify by value — trust the native status and fall back on error only.
	// A silent reset miss is recoverable (snapshot restore at next start) and is
	// not a leak.
	if err := resetAdapterDNSNative(ifIdx); err == nil {
		return false, nil
	}
	return true, resetAdapterDNSPowerShell(ifIdx)
}

func listAdapterDNSPowerShell() ([]adapterDNS, error) {
	// Statically-configured adapters report their servers in ServerAddresses;
	// DHCP-configured ones also do (the resolved value). PowerShell surfaces
	// them the same way, so we snapshot whatever's effective right now and
	// restore that exact set on disconnect.
	script := `Get-DnsClientServerAddress -AddressFamily IPv4 | ` +
		`Where-Object { $_.ServerAddresses.Count -gt 0 } | ` +
		`Select-Object InterfaceIndex,InterfaceAlias,ServerAddresses | ` +
		`ConvertTo-Json -Compress`
	out, err := powerShellRun(script)
	if err != nil {
		return nil, fmt.Errorf("powershell: %w (out=%s)", err, strings.TrimSpace(string(out)))
	}
	adapters, err := parseAdapterDNS(out)
	if err != nil {
		return nil, err
	}
	return filterPhysicalAdapterDNS(adapters), nil
}

// skipAdapterDNS returns true for virtual tunnel adapters (tun0, Wintun, …).
// Their DNS is managed by sing-box and the adapter vanishes on disconnect —
// snapshotting them causes Restore to fail with InterfaceIndex not found.
func skipAdapterDNS(alias string) bool {
	return looksLikeTunnelInterface(alias)
}

func filterPhysicalAdapterDNS(adapters []adapterDNS) []adapterDNS {
	if len(adapters) == 0 {
		return nil
	}
	out := make([]adapterDNS, 0, len(adapters))
	for _, a := range adapters {
		if skipAdapterDNS(a.InterfaceAlias) {
			continue
		}
		out = append(out, a)
	}
	return out
}

// isAdapterGoneError detects PowerShell/CIM failures when the interface
// index from an older snapshot no longer exists (typical after TUN teardown).
func isPermissionDeniedError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrDNSRequiresAdmin) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "permissiondenied") ||
		strings.Contains(msg, "access is denied") ||
		strings.Contains(msg, "access denied") ||
		strings.Contains(msg, "отказано в доступе") // localized PowerShell on RU Windows
}

func isAdapterGoneError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "notfound_interfaceindex") ||
		strings.Contains(msg, "interfacenotfound") ||
		strings.Contains(msg, "objectnotfound") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "не найден") // localized PowerShell on RU Windows
}

// parseAdapterDNS accepts the output of `ConvertTo-Json -Compress`, which
// emits a JSON object for a single record and a JSON array for >1. Empty
// pipeline → empty output. We also tolerate Windows' UTF-16 LE BOM.
func parseAdapterDNS(raw []byte) ([]adapterDNS, error) {
	raw = stripBOM(raw)
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return nil, nil
	}

	switch raw[0] {
	case '[':
		var list []adapterDNS
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, fmt.Errorf("decode array: %w", err)
		}
		return list, nil
	case '{':
		var single adapterDNS
		if err := json.Unmarshal(raw, &single); err != nil {
			return nil, fmt.Errorf("decode object: %w", err)
		}
		return []adapterDNS{single}, nil
	default:
		return nil, fmt.Errorf("unexpected powershell output: %q", string(raw))
	}
}

func stripBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		return b[2:]
	}
	return b
}

func setAdapterDNSPowerShell(ifIdx int, servers []string) error {
	quoted := make([]string, 0, len(servers))
	for _, s := range servers {
		s = strings.TrimSpace(s)
		if !isSafeDNSToken(s) {
			return fmt.Errorf("unsafe dns server token: %q", s)
		}
		quoted = append(quoted, `"`+s+`"`)
	}
	script := fmt.Sprintf(
		`Set-DnsClientServerAddress -InterfaceIndex %d -ServerAddresses @(%s)`,
		ifIdx, strings.Join(quoted, ","),
	)
	if out, err := powerShellRun(script); err != nil {
		return fmt.Errorf("powershell: %w (out=%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func resetAdapterDNSPowerShell(ifIdx int) error {
	script := fmt.Sprintf(
		`Set-DnsClientServerAddress -InterfaceIndex %d -ResetServerAddresses`,
		ifIdx,
	)
	if out, err := powerShellRun(script); err != nil {
		return fmt.Errorf("powershell: %w (out=%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// isSafeDNSToken keeps a tight whitelist (IPv4/IPv6 chars only). PowerShell
// runs the script via -Command, so anything passing through quoted strings
// is shell-interpreted; this rejects anything that isn't a numeric address.
func isSafeDNSToken(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		case r == '.' || r == ':':
		default:
			return false
		}
	}
	return true
}

func powerShellRun(script string) ([]byte, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.CombinedOutput()
}

// restoreElevated runs adapter DNS restore in an elevated PowerShell process
// (UAC). Used after a crash when a snapshot exists but the app restarted
// without admin rights.
func (w *windowsSystemDNS) restoreElevated(adapters []adapterDNS) error {
	script := buildRestorePSScript(adapters)
	if script == "" {
		_ = os.Remove(w.snapshotPath)
		return nil
	}
	return powerShellRunElevated(script)
}

func buildRestorePSScript(adapters []adapterDNS) string {
	var b strings.Builder
	for _, a := range adapters {
		if skipAdapterDNS(a.InterfaceAlias) {
			continue
		}
		if len(a.ServerAddresses) == 0 {
			fmt.Fprintf(&b, "Set-DnsClientServerAddress -InterfaceIndex %d -ResetServerAddresses\n", a.InterfaceIndex)
			continue
		}
		quoted := make([]string, 0, len(a.ServerAddresses))
		for _, s := range a.ServerAddresses {
			s = strings.TrimSpace(s)
			if !isSafeDNSToken(s) {
				continue
			}
			quoted = append(quoted, `"`+s+`"`)
		}
		if len(quoted) == 0 {
			continue
		}
		fmt.Fprintf(&b, "Set-DnsClientServerAddress -InterfaceIndex %d -ServerAddresses @(%s)\n",
			a.InterfaceIndex, strings.Join(quoted, ","))
	}
	return b.String()
}

func powerShellRunElevated(script string) error {
	dir := filepath.Join(resultProxyDataDir(), "elevated")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "dns-restore-*.ps1")
	if err != nil {
		return err
	}
	path := tmp.Name()
	if _, err := tmp.WriteString(script); err != nil {
		tmp.Close()
		_ = os.Remove(path)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	defer os.Remove(path)

	esc := strings.ReplaceAll(path, "'", "''")
	launcher := fmt.Sprintf(
		`Start-Process -FilePath 'powershell.exe' -Verb RunAs -Wait -WindowStyle Hidden -ArgumentList '-NoProfile','-NonInteractive','-ExecutionPolicy','Bypass','-File','%s'`,
		esc,
	)
	if out, err := powerShellRun(launcher); err != nil {
		return fmt.Errorf("elevated powershell: %w (out=%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (w *windowsSystemDNS) RestoreCommands() ([]string, error) {
	data, err := os.ReadFile(w.snapshotPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read dns snapshot: %w", err)
	}

	var snap dnsSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("parse dns snapshot: %w", err)
	}

	var cmds []string
	for _, a := range snap.Adapters {
		if skipAdapterDNS(a.InterfaceAlias) {
			continue
		}
		if len(a.ServerAddresses) == 0 {
			cmds = append(cmds, fmt.Sprintf("netsh interface ipv4 set dnsservers name=%d source=dhcp", a.InterfaceIndex))
			cmds = append(cmds, fmt.Sprintf("netsh interface ipv6 set dnsservers name=%d source=dhcp", a.InterfaceIndex))
		} else {
			first := a.ServerAddresses[0]
			if isSafeDNSToken(first) {
				cmds = append(cmds, fmt.Sprintf("netsh interface ipv4 set dnsservers name=%d source=static address=%s register=primary validate=no", a.InterfaceIndex, first))
			}
			for idx, s := range a.ServerAddresses[1:] {
				s = strings.TrimSpace(s)
				if isSafeDNSToken(s) {
					cmds = append(cmds, fmt.Sprintf("netsh interface ipv4 add dnsservers name=%d address=%s index=%d validate=no", a.InterfaceIndex, s, idx+2))
				}
			}
		}
	}
	return cmds, nil
}

func (w *windowsSystemDNS) DeleteSnapshot() error {
	if err := os.Remove(w.snapshotPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
