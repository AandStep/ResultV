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
	"fmt"
	"strings"
	"sync"
	"time"
)

// ErrDNSRequiresAdmin is returned when Windows adapter DNS cannot be changed
// without elevation (Set-DnsClientServerAddress requires administrator).
var ErrDNSRequiresAdmin = errors.New("system dns: administrator privileges required")

// dnsPhaseTimings accumulates the per-step cost of one system DNS override so
// the connect log can show where the phase spends its time. The override runs
// off the connect goroutine, so every field is mutex-guarded.
//
// Why this exists: the DNS phase is the single largest slice of a connect
// (~1.5 s of ~3.5 s) and has two very different code paths — a fast native
// iphlpapi call and a PowerShell fallback that costs 300-700 ms per
// invocation. Without the breakdown we cannot tell which one we are paying for.
type dnsPhaseTimings struct {
	mu       sync.Mutex
	list     time.Duration
	listPS   bool
	snapshot time.Duration
	set      time.Duration
	adapters int
	setPS    int
	tun      time.Duration
	tunPS    bool
	touched  bool
}

func (t *dnsPhaseTimings) recordList(d time.Duration, usedPowerShell bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.list = d
	t.listPS = usedPowerShell
	t.touched = true
}

func (t *dnsPhaseTimings) recordSnapshot(d time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.snapshot = d
	t.touched = true
}

func (t *dnsPhaseTimings) recordSet(d time.Duration, usedPowerShell bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.set += d
	t.adapters++
	if usedPowerShell {
		t.setPS++
	}
	t.touched = true
}

func (t *dnsPhaseTimings) recordTun(d time.Duration, usedPowerShell bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tun = d
	t.tunPS = usedPowerShell
	t.touched = true
}

func (t *dnsPhaseTimings) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.resetLocked()
}

func (t *dnsPhaseTimings) resetLocked() {
	t.list, t.listPS = 0, false
	t.snapshot = 0
	t.set, t.adapters, t.setPS = 0, 0, 0
	t.tun, t.tunPS = 0, false
	t.touched = false
}

// take returns the formatted breakdown and clears the accumulator, so the next
// connect starts from zero. Returns "" when nothing was recorded.
func (t *dnsPhaseTimings) take() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.touched {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "list=%dms(%s)", t.list.Milliseconds(), dnsPathLabel(t.listPS))
	fmt.Fprintf(&b, " snapshot=%dms", t.snapshot.Milliseconds())
	fmt.Fprintf(&b, " adapters=%d set=%dms(ps=%d)", t.adapters, t.set.Milliseconds(), t.setPS)
	fmt.Fprintf(&b, " tun=%dms(%s)", t.tun.Milliseconds(), dnsPathLabel(t.tunPS))
	out := b.String()
	t.resetLocked()
	return out
}

func dnsPathLabel(usedPowerShell bool) string {
	if usedPowerShell {
		return "ps"
	}
	return "native"
}

// dnsTimingSource is implemented by platform SystemDNS impls that record a
// per-step breakdown. Only Windows does; the manager type-asserts and stays
// silent elsewhere.
type dnsTimingSource interface {
	TakeDNSTimings() string
}

// SystemDNS overrides the OS resolver list for the duration of a VPN session.
//
// Why: in Proxy mode sing-box doesn't intercept DNS at the OS level. Apps
// that don't honor the system HTTP/SOCKS proxy (SSH/SFTP clients like
// WinSCP, native socket TCP, some VOIP/games) keep querying the
// DHCP-provided resolver — that's the ISP, and every TLS hostname leaks
// in plaintext UDP/53. Tunnel mode plugs this via auto_route + hijack-dns;
// Proxy mode needs an explicit override.
//
// Lifecycle:
//   - Override(servers): snapshot current per-adapter DNS to disk, apply
//     the supplied resolvers (e.g. 1.1.1.1, 8.8.8.8) to every active
//     adapter. Disk snapshot is the crash-recovery anchor.
//   - Restore(): re-read the snapshot, re-apply the original DNS, delete
//     the snapshot. Idempotent — safe to call without a prior Override.
type SystemDNS interface {
	Override(servers []string) error
	Restore() error

	// OverrideTunnelAdapter points the tunnel adapter's resolver at dnsIP — an
	// address inside the TUN subnet, answered by sing-box's hijack-dns rule
	// through the tunnel. The adapter is located by its interface address
	// (adapterIP). Deliberately NOT snapshotted: the adapter is destroyed on
	// disconnect, so there is nothing to restore.
	OverrideTunnelAdapter(adapterIP, dnsIP string) error

	// SnapshotExists reports whether a DNS snapshot is present on disk — i.e.
	// a previous run applied an override and did not cleanly restore it
	// (crash / force-kill). It is the detection anchor for startup leftover
	// cleanup; non-Windows platforms return false.
	SnapshotExists() bool

	// RestoreCommands returns the list of CLI commands (netsh) to restore DNS.
	RestoreCommands() ([]string, error)
	// DeleteSnapshot deletes the on-disk snapshot.
	DeleteSnapshot() error
}

// NewSystemDNS returns the platform implementation. On non-Windows it's
// a no-op (we only ship Windows Proxy-mode; macOS/Linux use Tunnel-mode
// where sing-box already covers DNS).
func NewSystemDNS() SystemDNS {
	return newSystemDNS()
}
