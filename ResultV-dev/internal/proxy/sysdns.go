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
}

// NewSystemDNS returns the platform implementation. On non-Windows it's
// a no-op (we only ship Windows Proxy-mode; macOS/Linux use Tunnel-mode
// where sing-box already covers DNS).
func NewSystemDNS() SystemDNS {
	return newSystemDNS()
}
