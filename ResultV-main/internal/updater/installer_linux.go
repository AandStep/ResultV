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

package updater

import (
	"fmt"
	"os"
	"syscall"
)

// currentPlatformKey returns the platforms map key for Linux (AppImage).
func currentPlatformKey() string {
	return "linux-amd64-appimage"
}

// installUpdate replaces the current AppImage in-place and exec's the new binary,
// replacing the current process without a visible restart.
func installUpdate(appImagePath string) error {
	currentExe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get current exe: %w", err)
	}

	if err := os.Chmod(appImagePath, 0o755); err != nil {
		return fmt.Errorf("chmod new AppImage: %w", err)
	}

	// Backup the current AppImage before replacing it.
	backupPath := currentExe + ".bak"
	_ = os.Rename(currentExe, backupPath)

	if err := os.Rename(appImagePath, currentExe); err != nil {
		// Restore backup on failure.
		_ = os.Rename(backupPath, currentExe)
		return fmt.Errorf("replace AppImage: %w", err)
	}

	// Replace the current process in-place (no visible restart gap).
	return syscall.Exec(currentExe, os.Args, os.Environ())
}
