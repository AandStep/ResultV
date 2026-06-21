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
	"runtime"
	"strings"
)

// runPrivileged runs argv with root privileges and returns combined stdout+stderr.
//
// Resolution order:
//  1. If we are already root (euid == 0), exec directly.
//  2. Try `sudo -n` (non-interactive — succeeds only if the user already has a
//     valid sudo timestamp or NOPASSWD configured).
//  3. Fall back to a GUI elevation prompt:
//     - macOS: osascript "do shell script ... with administrator privileges"
//     - Linux: pkexec
//
// The GUI fallback may pop a password dialog, which is acceptable for
// kill-switch toggling. If neither sudo nor a GUI prompter is available, the
// error message hints at the missing tool so the UI can surface it.
func runPrivileged(argv []string) ([]byte, error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("runPrivileged: empty argv")
	}

	if os.Geteuid() == 0 {
		return exec.Command(argv[0], argv[1:]...).CombinedOutput()
	}

	if _, err := exec.LookPath("sudo"); err == nil {
		sudoArgs := append([]string{"-n"}, argv...)
		out, err := exec.Command("sudo", sudoArgs...).CombinedOutput()
		if err == nil {
			return out, nil
		}
		// sudo -n failed (likely requires password); fall through to GUI.
	}

	return runPrivilegedGUI(argv)
}

func runPrivilegedGUI(argv []string) ([]byte, error) {
	switch runtime.GOOS {
	case "darwin":
		// osascript expects a single shell command. Quote each argument so
		// embedded whitespace and shell metachars survive the round-trip.
		quoted := make([]string, 0, len(argv))
		for _, a := range argv {
			quoted = append(quoted, shellQuote(a))
		}
		shellCmd := strings.Join(quoted, " ")
		// AppleScript double-quote escaping: \ and " inside the literal must
		// be escaped.
		appleEscaped := strings.ReplaceAll(shellCmd, `\`, `\\`)
		appleEscaped = strings.ReplaceAll(appleEscaped, `"`, `\"`)
		script := fmt.Sprintf(
			`do shell script "%s" with administrator privileges`,
			appleEscaped,
		)
		return exec.Command("osascript", "-e", script).CombinedOutput()
	case "linux":
		if _, err := exec.LookPath("pkexec"); err == nil {
			pkArgs := append([]string{"--disable-internal-agent"}, argv...)
			return exec.Command("pkexec", pkArgs...).CombinedOutput()
		}
		return nil, fmt.Errorf("privilege escalation requires pkexec or a passwordless sudo")
	default:
		return nil, fmt.Errorf("privilege escalation not supported on %s", runtime.GOOS)
	}
}

// shellQuote returns a POSIX-shell single-quoted form of s. Embedded single
// quotes are handled by closing the quote, inserting an escaped quote, and
// reopening: foo'bar -> 'foo'\”bar'.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n\"'\\$`*?[]{}()<>|&;#~!") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
