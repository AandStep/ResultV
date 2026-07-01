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
	"path/filepath"
	"strings"
)

// NormalizeAppEntry converts a user-supplied app whitelist input into the
// canonical form consumed by the routing engine.
//
// The routing engine matches against the basename of the OS process path
// (regex (^|[\/])<entry>$). Different platforms surface "the application" to
// the user in different ways:
//
//   - Windows: a .exe file path or the bare exe name.
//   - macOS:   an .app bundle (a folder) whose actual binary lives at
//     Contents/MacOS/<CFBundleExecutable>.
//   - Linux:   a binary path (e.g., /usr/bin/firefox) or a .desktop file.
//
// The platform-specific implementation lives in appwhitelist_<os>.go and is
// responsible for resolving these conventions to the binary basename.
func NormalizeAppEntry(input string) string {
	s := strings.TrimSpace(input)
	if s == "" {
		return ""
	}
	return normalizeAppEntry(s)
}

// NormalizeAppList returns a deduplicated, normalized copy of list. Entries
// that normalize to the empty string are dropped. Order is preserved.
func NormalizeAppList(list []string) []string {
	if len(list) == 0 {
		return list
	}
	seen := make(map[string]struct{}, len(list))
	out := make([]string, 0, len(list))
	for _, raw := range list {
		n := NormalizeAppEntry(raw)
		if n == "" {
			continue
		}
		key := strings.ToLower(n)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, n)
	}
	return out
}

// basenameNoExt returns the basename of p with the final extension stripped.
// Used by Linux/BSD normalization to convert paths like /usr/bin/firefox to
// "firefox" without altering names that have meaningful dots in them.
func basenameNoExt(p string) string {
	base := filepath.Base(p)
	if base == "." || base == string(filepath.Separator) {
		return ""
	}
	return base
}
