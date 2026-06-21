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

package updater

import (
	"fmt"
	"os/exec"
)

// currentPlatformKey returns the platforms map key for macOS.
func currentPlatformKey() string {
	return "darwin-universal"
}

// installUpdate mounts the DMG, copies the app bundle, then relaunches from
// /Applications. Returns so the caller can gracefully quit via Wails.
func installUpdate(dmgPath string) error {
	const mountPoint = "/tmp/resultv-update"

	if err := exec.Command(
		"hdiutil", "attach", dmgPath,
		"-nobrowse", "-mountpoint", mountPoint,
	).Run(); err != nil {
		return fmt.Errorf("mount dmg: %w", err)
	}

	// Best-effort detach on exit.
	defer exec.Command("hdiutil", "detach", mountPoint, "-force").Run()

	if err := exec.Command(
		"ditto",
		mountPoint+"/ResultV.app",
		"/Applications/ResultV.app",
	).Run(); err != nil {
		return fmt.Errorf("copy app bundle: %w", err)
	}

	if err := exec.Command("open", "/Applications/ResultV.app").Start(); err != nil {
		return fmt.Errorf("launch new version: %w", err)
	}

	return nil
}
