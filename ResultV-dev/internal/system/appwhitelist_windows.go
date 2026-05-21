// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build windows

package system

import (
	"path/filepath"
	"strings"
)

// normalizeAppEntry on Windows extracts the basename and ensures it ends in
// .exe. The routing engine matches the basename of the OS process path, so
// "C:\Program Files\Foo\bar.exe" and "bar.exe" both reduce to "bar.exe".
func normalizeAppEntry(s string) string {
	// Accept both forward and backward slashes.
	s = strings.ReplaceAll(s, "/", `\`)
	base := filepath.Base(s)
	if base == "." || base == `\` {
		return ""
	}
	if !strings.HasSuffix(strings.ToLower(base), ".exe") {
		base += ".exe"
	}
	return base
}
