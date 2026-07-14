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
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Persistent server-IP pin cache (encrypted at rest).
//
// For a domain-addressed server the OS resolver cannot resolve (censored DNS),
// the connect-time pin comes back empty and sing-box re-resolves the server per
// connection via the fragile `local` resolver — the false-kill-switch cause. We
// can't resolve the domain ourselves (only sing-box can), but once connected we
// read the real server IP from the live socket (captureLiveServerIP) and store
// it here. On the next connect we pin that IP straight into the sing-box
// outbound so sing-box never re-resolves.
//
// SECURITY: the entries map a server hostname to its real backend IP — exactly
// the data that, if leaked, lets a censor block the server. So the file is
// encrypted with the app's CryptoService (hardware-ID + per-install salt), the
// same scheme that protects the main config. There is no plaintext path: with
// no codec we simply don't persist (the in-session capture still works, we just
// don't carry the pin to the next launch).
//
// The cached IP can go stale if the server's address changes (CDN/failover);
// the connect path detects that (the post-start probe fails on the dead IP),
// clears the entry, and retries on the bare domain — see Manager.Connect.

// SecretCodec is the subset of config.CryptoService the pin cache needs.
// Injected via Manager.SetSecretCodec so the proxy package stays decoupled from
// the config package (and so tests can supply a fake).
type SecretCodec interface {
	Encrypt(data any) (string, error)
	DecryptInto(rawStr string, dst any) error
}

const serverPinCacheFile = "server_pins.json"

// serverPinMu serializes the load-modify-store of the cache file. Connect is
// already serialized by opMu, but guard the file explicitly for safety.
var serverPinMu sync.Mutex

// serverPinKey normalizes a server host into a cache key. Pin entries are keyed
// by hostname only: the resolved IP is host-level, identical across ports.
func serverPinKey(host string) string {
	return strings.ToLower(strings.TrimSpace(host))
}

func serverPinPath(dataDir string) string {
	return filepath.Join(dataDir, serverPinCacheFile)
}

// loadServerPinMap reads and decrypts the whole cache. A missing file, a nil
// codec, or any decrypt failure yields an empty map. A file that fails to
// decrypt is removed: it is either corrupt or a pre-encryption plaintext
// leftover, and either way must not linger on disk.
func loadServerPinMap(dataDir string, codec SecretCodec) map[string]string {
	out := map[string]string{}
	if dataDir == "" || codec == nil {
		return out
	}
	path := serverPinPath(dataDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	if err := codec.DecryptInto(string(data), &out); err != nil || out == nil {
		_ = os.Remove(path)
		return map[string]string{}
	}
	return out
}

// writeServerPinMap encrypts and writes the cache. No codec => no write (never
// persist these IPs in the clear).
func writeServerPinMap(dataDir string, codec SecretCodec, m map[string]string) {
	if dataDir == "" || codec == nil {
		return
	}
	enc, err := codec.Encrypt(m)
	if err != nil {
		return
	}
	_ = os.WriteFile(serverPinPath(dataDir), []byte(enc), 0o600)
}

// loadServerPin returns the cached IP for host, or "" when absent. A stored
// value that is not a valid IP literal is ignored (never pin garbage).
func loadServerPin(dataDir string, codec SecretCodec, host string) string {
	key := serverPinKey(host)
	if key == "" {
		return ""
	}
	serverPinMu.Lock()
	defer serverPinMu.Unlock()
	ip := loadServerPinMap(dataDir, codec)[key]
	if net.ParseIP(ip) == nil {
		return ""
	}
	return ip
}

// saveServerPin records host -> ip, replacing any previous value.
func saveServerPin(dataDir string, codec SecretCodec, host, ip string) {
	key := serverPinKey(host)
	if key == "" || net.ParseIP(ip) == nil {
		return
	}
	serverPinMu.Lock()
	defer serverPinMu.Unlock()
	m := loadServerPinMap(dataDir, codec)
	if m[key] == ip {
		return
	}
	m[key] = ip
	writeServerPinMap(dataDir, codec, m)
}

// clearServerPin drops the entry for host (called when the pin proved stale).
func clearServerPin(dataDir string, codec SecretCodec, host string) {
	key := serverPinKey(host)
	if key == "" {
		return
	}
	serverPinMu.Lock()
	defer serverPinMu.Unlock()
	m := loadServerPinMap(dataDir, codec)
	if _, ok := m[key]; !ok {
		return
	}
	delete(m, key)
	writeServerPinMap(dataDir, codec, m)
}
