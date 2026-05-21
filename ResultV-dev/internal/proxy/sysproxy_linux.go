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

//go:build linux

package proxy

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// killSwitchSinkhole is an unreachable address used to break all proxied
// traffic when the kill switch is engaged.
const killSwitchSinkhole = "127.0.0.1:65535"

// linuxBackend applies system proxy settings to a single Linux subsystem
// (GNOME, KDE, environment files, ...). Multiple backends are applied in
// parallel because users may run mixed environments (e.g., GTK apps under KDE
// or vice versa) and apps that read $http_proxy ignore both DE settings.
type linuxBackend interface {
	Name() string
	Available() bool
	Apply(host, port string, bypass []string) error
	Disable() error
}

// LinuxSystemProxy fans out proxy configuration to every available backend on
// the host. A backend that is not installed (e.g., gsettings on a KDE-only
// system) is silently skipped; if no backend is available, Set returns an
// error so the caller can surface it to the user.
type LinuxSystemProxy struct {
	router   *Router
	backends []linuxBackend

	mu sync.Mutex
}

var _ SystemProxy = (*LinuxSystemProxy)(nil)

func newSystemProxy(router *Router) SystemProxy {
	return &LinuxSystemProxy{
		router: router,
		backends: []linuxBackend{
			&gnomeBackend{},
			&kdeBackend{},
			&envBackend{},
		},
	}
}

func (l *LinuxSystemProxy) Set(addr string, bypass []string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid proxy address %q: %w", addr, err)
	}

	safe := bypass
	if l.router != nil && len(bypass) > 0 {
		safe = l.router.GetSafeOSWhitelist(bypass)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	var firstErr error
	applied := 0
	for _, b := range l.backends {
		if !b.Available() {
			continue
		}
		if err := b.Apply(host, port, safe); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", b.Name(), err)
			}
			continue
		}
		applied++
	}
	if applied == 0 {
		if firstErr != nil {
			return firstErr
		}
		return fmt.Errorf("no system proxy backend available (install gsettings, kwriteconfig, or use environment variables)")
	}
	return nil
}

func (l *LinuxSystemProxy) Disable() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	var firstErr error
	for _, b := range l.backends {
		if !b.Available() {
			continue
		}
		if err := b.Disable(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("%s: %w", b.Name(), err)
		}
	}
	return firstErr
}

func (l *LinuxSystemProxy) DisableSync() {
	_ = l.Disable()
}

func (l *LinuxSystemProxy) ApplyKillSwitch() error {
	return l.Set(killSwitchSinkhole, nil)
}

// --- GNOME / GTK backend (gsettings) ---

type gnomeBackend struct{}

func (gnomeBackend) Name() string { return "gnome" }

func (gnomeBackend) Available() bool {
	_, err := exec.LookPath("gsettings")
	return err == nil
}

func (gnomeBackend) Apply(host, port string, bypass []string) error {
	steps := [][]string{
		{"set", "org.gnome.system.proxy", "mode", "manual"},
		{"set", "org.gnome.system.proxy.http", "host", host},
		{"set", "org.gnome.system.proxy.http", "port", port},
		{"set", "org.gnome.system.proxy.https", "host", host},
		{"set", "org.gnome.system.proxy.https", "port", port},
		{"set", "org.gnome.system.proxy.socks", "host", host},
		{"set", "org.gnome.system.proxy.socks", "port", port},
	}
	ignore := []string{"localhost", "127.0.0.0/8", "::1"}
	for _, d := range bypass {
		ignore = append(ignore, d, "*."+d)
	}
	steps = append(steps, []string{
		"set", "org.gnome.system.proxy", "ignore-hosts", formatGSettingsList(ignore),
	})
	for _, s := range steps {
		if err := runGsettings(s...); err != nil {
			return err
		}
	}
	return nil
}

func (gnomeBackend) Disable() error {
	return runGsettings("set", "org.gnome.system.proxy", "mode", "none")
}

func runGsettings(args ...string) error {
	cmd := exec.Command("gsettings", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gsettings %s: %s: %w",
			strings.Join(args, " "),
			strings.TrimSpace(string(out)),
			err,
		)
	}
	return nil
}

// formatGSettingsList renders a Go slice as a GSettings array literal:
// ['localhost', '127.0.0.0/8', ...].
func formatGSettingsList(items []string) string {
	var b strings.Builder
	b.WriteString("[")
	for i, it := range items {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString("'")
		b.WriteString(strings.ReplaceAll(it, "'", `\'`))
		b.WriteString("'")
	}
	b.WriteString("]")
	return b.String()
}

// --- KDE backend (kwriteconfig5/kwriteconfig6) ---

type kdeBackend struct {
	binary string
}

func (k *kdeBackend) Name() string { return "kde" }

func (k *kdeBackend) Available() bool {
	for _, b := range []string{"kwriteconfig6", "kwriteconfig5"} {
		if _, err := exec.LookPath(b); err == nil {
			k.binary = b
			return true
		}
	}
	return false
}

func (k *kdeBackend) Apply(host, port string, bypass []string) error {
	// KDE encodes proxy as "scheme://host port" (note: space, not colon, before the port).
	value := fmt.Sprintf("http://%s %s", host, port)
	noProxyParts := append([]string{"localhost", "127.0.0.1", "::1"}, bypass...)
	noProxy := strings.Join(noProxyParts, ",")

	// ProxyType: 0 = none, 1 = manual, 2 = PAC, 3 = WPAD/system.
	steps := [][]string{
		{"--file", "kioslaverc", "--group", "Proxy Settings", "--key", "ProxyType", "1"},
		{"--file", "kioslaverc", "--group", "Proxy Settings", "--key", "httpProxy", value},
		{"--file", "kioslaverc", "--group", "Proxy Settings", "--key", "httpsProxy", value},
		{"--file", "kioslaverc", "--group", "Proxy Settings", "--key", "ftpProxy", value},
		{"--file", "kioslaverc", "--group", "Proxy Settings", "--key", "socksProxy", value},
		{"--file", "kioslaverc", "--group", "Proxy Settings", "--key", "NoProxyFor", noProxy},
	}
	for _, s := range steps {
		if err := k.run(s...); err != nil {
			return err
		}
	}
	return nil
}

func (k *kdeBackend) Disable() error {
	return k.run("--file", "kioslaverc", "--group", "Proxy Settings", "--key", "ProxyType", "0")
}

func (k *kdeBackend) run(args ...string) error {
	cmd := exec.Command(k.binary, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %s: %w",
			k.binary,
			strings.Join(args, " "),
			strings.TrimSpace(string(out)),
			err,
		)
	}
	return nil
}

// --- Environment variable backend (~/.config/environment.d/) ---
//
// systemd reads ~/.config/environment.d/*.conf when starting a user session,
// and many CLI/curl-style tools honour http_proxy/https_proxy/no_proxy. This
// backend provides a fallback that works regardless of desktop environment,
// at the cost of requiring re-login for newly-spawned processes to pick up
// the change.

type envBackend struct{}

func (envBackend) Name() string { return "env" }

func (envBackend) Available() bool {
	if _, err := os.UserHomeDir(); err != nil {
		return false
	}
	return true
}

func envFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "environment.d", "99-resultv.conf"), nil
}

func (envBackend) Apply(host, port string, bypass []string) error {
	path, err := envFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	url := fmt.Sprintf("http://%s:%s", host, port)
	noParts := append([]string{"localhost", "127.0.0.1", "::1"}, bypass...)
	no := strings.Join(noParts, ",")
	body := fmt.Sprintf(
		"http_proxy=%s\nhttps_proxy=%s\nHTTP_PROXY=%s\nHTTPS_PROXY=%s\nno_proxy=%s\nNO_PROXY=%s\n",
		url, url, url, url, no, no,
	)
	return os.WriteFile(path, []byte(body), 0o644)
}

func (envBackend) Disable() error {
	path, err := envFilePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
