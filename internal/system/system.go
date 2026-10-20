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

package system

import (
	"encoding/base64"
	"strconv"
	"strings"
)

// TrafficStats reports cumulative interface counters in bytes. The numbers come
// from the OS — implementations live in system_windows.go / system_unix.go.
type TrafficStats struct {
	Received int64 `json:"received"`
	Sent     int64 `json:"sent"`
}

// ArgsStartInTray reports whether the process was launched in tray-only mode.
// Cross-platform: parsed from os.Args by main, no syscalls.
func ArgsStartInTray(args []string) bool {
	for _, a := range args[1:] {
		if a == "--autostart" || a == "--tray" {
			return true
		}
	}
	return false
}

// ElevatedFromFlag carries the pid of the instance that relaunched us elevated.
const ElevatedFromFlag = "--elevated-from="

// ConnectAfterElevationFlag carries the server the user was connecting to when
// the instance had to restart elevated. The id is base64url so no quoting layer
// (PowerShell, AppleScript, pkexec) ever sees anything but [A-Za-z0-9_-].
const ConnectAfterElevationFlag = "--connect="

// ModeAfterElevationFlag carries the mode the user was switching to when the
// instance had to restart elevated.
const ModeAfterElevationFlag = "--mode="

// ElevationRelaunchArgs builds the arguments that make the elevated instance
// resume what the user started: connect to proxyID, optionally in mode.
func ElevationRelaunchArgs(proxyID, mode string) []string {
	var args []string
	if proxyID != "" {
		args = append(args, ConnectAfterElevationFlag+base64.RawURLEncoding.EncodeToString([]byte(proxyID)))
	}
	if mode == "proxy" || mode == "tunnel" {
		args = append(args, ModeAfterElevationFlag+mode)
	}
	return args
}

// ArgsResumeAfterElevation parses what ElevationRelaunchArgs produced.
func ArgsResumeAfterElevation(args []string) (proxyID, mode string) {
	for _, a := range args {
		if v, ok := strings.CutPrefix(a, ConnectAfterElevationFlag); ok {
			if b, err := base64.RawURLEncoding.DecodeString(v); err == nil {
				proxyID = string(b)
			}
		}
		if v, ok := strings.CutPrefix(a, ModeAfterElevationFlag); ok && (v == "proxy" || v == "tunnel") {
			mode = v
		}
	}
	return proxyID, mode
}

// ArgsElevatedFrom reports whether this process is itself an elevated relaunch.
func ArgsElevatedFrom(args []string) bool { return elevatedFromPID(args) > 0 }

func elevatedFromPID(args []string) int {
	for _, a := range args {
		if v, ok := strings.CutPrefix(a, ElevatedFromFlag); ok {
			if pid, err := strconv.Atoi(v); err == nil {
				return pid
			}
		}
	}
	return 0
}
