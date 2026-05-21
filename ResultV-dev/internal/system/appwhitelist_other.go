// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build !windows && !darwin

package system

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// normalizeAppEntry on Linux/BSD reduces the input to the basename of the
// referenced executable.
//
// Special handling:
//   - .desktop entries: parse the Exec= line and use the first token's
//     basename (a common shape for users that drag entries from menus).
//   - everything else: plain basename of the path.
func normalizeAppEntry(s string) string {
	if strings.HasSuffix(strings.ToLower(s), ".desktop") {
		if exe, ok := execFromDesktopFile(s); ok {
			return exe
		}
	}
	return basenameNoExt(s)
}

func execFromDesktopFile(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	inDesktopEntry := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inDesktopEntry = line == "[Desktop Entry]"
			continue
		}
		if !inDesktopEntry {
			continue
		}
		if !strings.HasPrefix(line, "Exec=") {
			continue
		}
		cmd := strings.TrimPrefix(line, "Exec=")
		token := firstExecToken(cmd)
		if token == "" {
			return "", false
		}
		return filepath.Base(token), true
	}
	return "", false
}

// firstExecToken extracts the first whitespace-delimited token from a Desktop
// Entry Exec= value, honouring double-quoting per the spec.
func firstExecToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if s[0] == '"' {
		end := strings.IndexByte(s[1:], '"')
		if end == -1 {
			return strings.TrimPrefix(s, `"`)
		}
		return s[1 : 1+end]
	}
	if i := strings.IndexAny(s, " \t"); i != -1 {
		return s[:i]
	}
	return s
}
