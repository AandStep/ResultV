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
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func knownFolderPath(folder *windows.KNOWNFOLDERID, env, fallback string) string {
	if p, err := windows.KnownFolderPath(folder, windows.KF_FLAG_DEFAULT); err == nil && p != "" {
		return p
	}
	if p := os.Getenv(env); p != "" {
		return p
	}
	if p, err := os.UserHomeDir(); err == nil && p != "" {
		return filepath.Join(p, fallback)
	}
	return fallback
}

func UserDataDir() string {
	newPath, _ := userDataPaths()
	return newPath
}

func WebviewUserDataPath() string {
	newPath, _ := webviewDataPaths()
	return newPath
}

func userDataPaths() (string, string) {
	base := knownFolderPath(windows.FOLDERID_RoamingAppData, "APPDATA", filepath.Join("AppData", "Roaming"))
	newPath := filepath.Join(base, "ResultV")
	legacyPath := filepath.Join(base, "ResultProxy")
	return newPath, legacyPath
}

func webviewDataPaths() (string, string) {
	base := knownFolderPath(windows.FOLDERID_LocalAppData, "LOCALAPPDATA", filepath.Join("AppData", "Local"))
	newPath := filepath.Join(base, "ResultV", "webview")
	legacyPath := filepath.Join(base, "ResultProxy", "webview")
	return newPath, legacyPath
}
