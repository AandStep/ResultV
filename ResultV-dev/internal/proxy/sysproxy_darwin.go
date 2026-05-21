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

//go:build darwin

package proxy

import (
	"bufio"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"
)

// killSwitchSinkhole is an unreachable address used to break all proxied
// traffic when the kill switch is engaged.
const killSwitchSinkhole = "127.0.0.1:65535"

// DarwinSystemProxy configures system-wide HTTP/HTTPS/SOCKS proxy on macOS via
// the `networksetup` CLI. It applies the configuration to every enabled
// network service (Wi-Fi, Ethernet, Thunderbolt Bridge, etc.) so that traffic
// from any active interface honors the proxy.
type DarwinSystemProxy struct {
	router *Router

	mu              sync.Mutex
	appliedServices []string
}

var _ SystemProxy = (*DarwinSystemProxy)(nil)

func newSystemProxy(router *Router) SystemProxy {
	return &DarwinSystemProxy{router: router}
}

func (d *DarwinSystemProxy) Set(addr string, bypass []string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid proxy address %q: %w", addr, err)
	}
	services, err := listEnabledNetworkServices()
	if err != nil {
		return err
	}
	if len(services) == 0 {
		return fmt.Errorf("no active network services found")
	}

	bypassDomains := d.buildBypass(bypass)

	d.mu.Lock()
	defer d.mu.Unlock()

	for _, svc := range services {
		if err := configureService(svc, host, port, bypassDomains); err != nil {
			return fmt.Errorf("service %q: %w", svc, err)
		}
	}
	d.appliedServices = services
	return nil
}

func (d *DarwinSystemProxy) Disable() error {
	d.mu.Lock()
	services := append([]string(nil), d.appliedServices...)
	d.appliedServices = nil
	d.mu.Unlock()

	if len(services) == 0 {
		// No memory of which services we touched (e.g., process was restarted).
		// Best-effort: turn proxies off on every currently active service.
		services, _ = listEnabledNetworkServices()
	}

	var firstErr error
	for _, svc := range services {
		if err := disableServiceProxy(svc); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (d *DarwinSystemProxy) DisableSync() {
	_ = d.Disable()
}

func (d *DarwinSystemProxy) ApplyKillSwitch() error {
	return d.Set(killSwitchSinkhole, nil)
}

func (d *DarwinSystemProxy) buildBypass(whitelist []string) []string {
	base := []string{"localhost", "127.0.0.1", "::1", "*.local", "169.254/16"}
	if len(whitelist) == 0 || d.router == nil {
		return base
	}
	safe := d.router.GetSafeOSWhitelist(whitelist)
	for _, dom := range safe {
		base = append(base, dom, "*."+dom)
	}
	return base
}

func listEnabledNetworkServices() ([]string, error) {
	out, err := exec.Command("networksetup", "-listallnetworkservices").Output()
	if err != nil {
		return nil, fmt.Errorf("networksetup -listallnetworkservices: %w", err)
	}

	var services []string
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	headerSkipped := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if !headerSkipped {
			// First non-empty line is an informational header
			// ("An asterisk (*) denotes that a network service is disabled.")
			headerSkipped = true
			if strings.HasPrefix(line, "An asterisk") {
				continue
			}
		}
		// Disabled services are prefixed with '*'.
		if strings.HasPrefix(line, "*") {
			continue
		}
		services = append(services, line)
	}
	return services, nil
}

func configureService(svc, host, port string, bypass []string) error {
	if err := runNetworksetup("-setwebproxy", svc, host, port); err != nil {
		return err
	}
	if err := runNetworksetup("-setsecurewebproxy", svc, host, port); err != nil {
		return err
	}
	if err := runNetworksetup("-setsocksfirewallproxy", svc, host, port); err != nil {
		return err
	}
	if err := runNetworksetup("-setwebproxystate", svc, "on"); err != nil {
		return err
	}
	if err := runNetworksetup("-setsecurewebproxystate", svc, "on"); err != nil {
		return err
	}
	// SOCKS state is best-effort: some services may not support it.
	_ = runNetworksetup("-setsocksfirewallproxystate", svc, "on")

	args := []string{"-setproxybypassdomains", svc}
	if len(bypass) == 0 {
		args = append(args, "Empty")
	} else {
		args = append(args, bypass...)
	}
	// Bypass list is non-fatal: failure to set bypass should not abort the
	// whole proxy configuration.
	_ = runNetworksetup(args...)
	return nil
}

func disableServiceProxy(svc string) error {
	var firstErr error
	for _, action := range []string{
		"-setwebproxystate",
		"-setsecurewebproxystate",
		"-setsocksfirewallproxystate",
	} {
		if err := runNetworksetup(action, svc, "off"); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func runNetworksetup(args ...string) error {
	cmd := exec.Command("networksetup", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("networksetup %s: %s: %w",
			strings.Join(args, " "),
			strings.TrimSpace(string(out)),
			err,
		)
	}
	return nil
}
