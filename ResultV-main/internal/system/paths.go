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

//go:build !windows

package system

import (
	"os"
	"path/filepath"
	"runtime"
)

func UserDataDir() string {
	newPath, _ := userDataPaths()
	return newPath
}

func WebviewUserDataPath() string {
	newPath, _ := webviewDataPaths()
	return newPath
}

func userConfigBase() string {
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, "Library", "Application Support")
		}
		return filepath.Join(home, ".config")
	}
	return "."
}

func userCacheBase() string {
	if dir, err := os.UserCacheDir(); err == nil && dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, "Library", "Caches")
		}
		return filepath.Join(home, ".cache")
	}
	return "."
}

func userDataPaths() (string, string) {
	base := userConfigBase()
	return filepath.Join(base, "ResultV"), filepath.Join(base, "ResultProxy")
}

func webviewDataPaths() (string, string) {
	base := userCacheBase()
	return filepath.Join(base, "ResultV", "webview"), filepath.Join(base, "ResultProxy", "webview")
}
