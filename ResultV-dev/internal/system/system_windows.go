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

package system

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func GetNetworkTraffic() TrafficStats {
	out, err := command("netstat", "-e").Output()
	if err != nil {
		return TrafficStats{}
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		parts := strings.Fields(strings.TrimSpace(line))
		if len(parts) >= 3 {
			received, err1 := strconv.ParseInt(parts[len(parts)-2], 10, 64)
			sent, err2 := strconv.ParseInt(parts[len(parts)-1], 10, 64)
			if err1 == nil && err2 == nil && received > 0 {
				return TrafficStats{Received: received, Sent: sent}
			}
		}
	}

	return TrafficStats{}
}

// IsAdmin reports whether the current process is running elevated.
//
// It queries the process token's elevation flag directly rather than shelling
// out to `net session`. `net session` talks to the Server (LanmanServer)
// service, so when that service is stopped/disabled (common on "debloated"
// systems) or merely slow to start right after boot, it returns an error — and
// we would report a genuine administrator as non-elevated, surfacing a futile
// "restart as administrator" prompt even though the process is already elevated.
// TokenElevation has no such external dependency and is what UAC itself checks.
func IsAdmin() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

func RestartAsAdmin(exePath string, args ...string) error {
	psArgs := fmt.Sprintf(`Start-Process -FilePath '%s' -Verb RunAs`, exePath)
	if len(args) > 0 {
		psArgs += fmt.Sprintf(` -ArgumentList '%s'`, strings.Join(args, " "))
	}
	return command("powershell", "-Command", psArgs).Start()
}

var (
	dllShell32          = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteExW = dllShell32.NewProc("ShellExecuteExW")
)

const seeMaskNoCloseProcess = 0x00000040

type shellExecuteInfo struct {
	cbSize         uint32
	fMask          uint32
	hwnd           windows.Handle
	lpVerb         *uint16
	lpFile         *uint16
	lpParameters   *uint16
	lpDirectory    *uint16
	nShow          int32
	hInstApp       windows.Handle
	lpIDList       uintptr
	lpClass        *uint16
	hkeyClass      windows.Handle
	dwHotKey       uint32
	hIcon          windows.Handle
	hProcess       windows.Handle
}

// ExecuteElevatedCommands writes commands to a temporary batch file and executes
// it elevated (UAC prompt) via ShellExecuteExW with cmd.exe, hiding the window.
// It waits up to 10 seconds for execution to finish.
func ExecuteElevatedCommands(commands []string) error {
	if len(commands) == 0 {
		return nil
	}

	dir := filepath.Join(UserDataDir(), "elevated")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create elevated dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "cleanup-*.cmd")
	if err != nil {
		return fmt.Errorf("create temp batch file: %w", err)
	}
	path := tmp.Name()

	var b strings.Builder
	b.WriteString("@echo off\nchcp 65001 >nul\n")
	for _, cmd := range commands {
		b.WriteString(cmd)
		b.WriteString("\n")
	}

	if _, err := tmp.WriteString(b.String()); err != nil {
		tmp.Close()
		_ = os.Remove(path)
		return fmt.Errorf("write commands to batch: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("close temp batch file: %w", err)
	}
	defer os.Remove(path)

	verbPtr, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	filePtr, err := windows.UTF16PtrFromString("cmd.exe")
	if err != nil {
		return err
	}
	argsPtr, err := windows.UTF16PtrFromString(fmt.Sprintf("/c \"%s\"", path))
	if err != nil {
		return err
	}

	var info shellExecuteInfo
	info.cbSize = uint32(unsafe.Sizeof(info))
	info.fMask = seeMaskNoCloseProcess
	info.lpVerb = verbPtr
	info.lpFile = filePtr
	info.lpParameters = argsPtr
	info.nShow = 0 // SW_HIDE

	ret, _, err := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info)))
	if ret == 0 {
		return fmt.Errorf("ShellExecuteExW failed: %w", err)
	}

	if info.hProcess != 0 {
		defer windows.CloseHandle(info.hProcess)
		event, err := windows.WaitForSingleObject(info.hProcess, 10000)
		if err != nil {
			return fmt.Errorf("WaitForSingleObject: %w", err)
		}
		if event == uint32(windows.WAIT_TIMEOUT) {
			return fmt.Errorf("elevated commands timed out")
		}
		var exitCode uint32
		if err := windows.GetExitCodeProcess(info.hProcess, &exitCode); err == nil {
			if exitCode != 0 {
				return fmt.Errorf("elevated process exited with code %d", exitCode)
			}
		}
	}
	return nil
}
