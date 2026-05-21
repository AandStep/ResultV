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

//go:build darwin || linux

package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// IsAdmin reports whether the current process has root privileges.
//
// macOS/Linux use POSIX euid; the Windows variant in system_windows.go uses
// `net session`. This split is the load-bearing one — the previous unified
// implementation called `net session` on every platform, so on macOS the check
// always returned false and Tunnel mode was unreachable even under sudo.
func IsAdmin() bool {
	return os.Geteuid() == 0
}

// RestartAsAdmin re-launches the application with root privileges and returns
// nil on success so the caller can quit the unprivileged instance.
//
//   - macOS: drives osascript "do shell script ... with administrator privileges",
//     which shows the standard system password dialog. We launch the binary
//     detached (`&`) so osascript returns immediately; without that the script
//     blocks on the GUI process and we can't quit cleanly.
//   - Linux: prefers pkexec (PolicyKit GUI prompt). Requires DISPLAY/XAUTHORITY
//     to be propagated so the relaunched GUI can find the X/Wayland session.
//
// GetNetworkTraffic exists for parity with Windows; the netstat/proc parsing on
// Unix is noisy and we don't surface those numbers in the UI today, so it
// returns zeros rather than guessing.
func RestartAsAdmin(exePath string, args ...string) error {
	if exePath == "" {
		return fmt.Errorf("RestartAsAdmin: empty exePath")
	}

	switch runtime.GOOS {
	case "darwin":
		return restartAsAdminDarwin(exePath, args)
	case "linux":
		return restartAsAdminLinux(exePath, args)
	default:
		return fmt.Errorf("RestartAsAdmin: unsupported platform %s", runtime.GOOS)
	}
}

func restartAsAdminDarwin(exePath string, args []string) error {
	parts := []string{shellQuote(exePath)}
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	// Detach so the AppleScript doesn't wait on the GUI process; redirect
	// stdio so a daemonized child doesn't keep the script's pipes open.
	shellCmd := strings.Join(parts, " ") + " >/dev/null 2>&1 &"

	appleEscaped := strings.ReplaceAll(shellCmd, `\`, `\\`)
	appleEscaped = strings.ReplaceAll(appleEscaped, `"`, `\"`)
	script := fmt.Sprintf(
		`do shell script "%s" with administrator privileges`,
		appleEscaped,
	)
	if err := exec.Command("osascript", "-e", script).Run(); err != nil {
		return fmt.Errorf("osascript elevation failed: %w", err)
	}
	return nil
}

func restartAsAdminLinux(exePath string, args []string) error {
	if _, err := exec.LookPath("pkexec"); err != nil {
		return fmt.Errorf("pkexec not found; install PolicyKit or relaunch via `sudo %s`",
			filepath.Base(exePath))
	}
	// Propagate the GUI session env so the elevated process can attach to
	// the user's display server.
	pkArgs := []string{"env"}
	for _, k := range []string{"DISPLAY", "XAUTHORITY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR"} {
		if v, ok := os.LookupEnv(k); ok {
			pkArgs = append(pkArgs, k+"="+v)
		}
	}
	pkArgs = append(pkArgs, exePath)
	pkArgs = append(pkArgs, args...)

	cmd := exec.Command("pkexec", pkArgs...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("pkexec start: %w", err)
	}
	// Don't Wait — pkexec stays alive for the lifetime of the elevated
	// process, and the caller is about to quit this instance anyway.
	return nil
}

// GetNetworkTraffic returns zeros on Unix. The Windows implementation parses
// `netstat -e`; the macOS/Linux equivalents (`netstat -ib`, /proc/net/dev) are
// per-interface and require selecting the right one, which the UI doesn't
// currently consume.
func GetNetworkTraffic() TrafficStats {
	return TrafficStats{}
}
