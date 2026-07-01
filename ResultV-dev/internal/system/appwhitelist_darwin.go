// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build darwin

package system

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var bundleExecKeyRe = regexp.MustCompile(
	`(?s)<key>\s*CFBundleExecutable\s*</key>\s*<string>\s*([^<\s][^<]*?)\s*</string>`,
)

// normalizeAppEntry on macOS resolves an .app bundle (or a path inside one)
// to the actual binary basename that appears in process paths. For non-bundle
// inputs it falls back to the basename.
//
// Resolution order for a bundle path:
//  1. Read Contents/Info.plist and pull CFBundleExecutable. This is the
//     authoritative name and handles renames like
//     "Visual Studio Code.app" -> "Electron".
//  2. Fall back to the bundle directory name without the .app suffix.
func normalizeAppEntry(s string) string {
	if bundle := bundleRoot(s); bundle != "" {
		if exe, ok := bundleExecutableFromPlist(bundle); ok {
			return exe
		}
		base := filepath.Base(bundle)
		return strings.TrimSuffix(base, ".app")
	}
	return basenameNoExt(s)
}

// bundleRoot returns the .app directory path for s, or "" if s does not
// reference an .app bundle. Handles both "/Applications/Foo.app" and a deep
// path inside the bundle.
func bundleRoot(s string) string {
	idx := strings.Index(s, ".app")
	if idx == -1 {
		return ""
	}
	end := idx + len(".app")
	// Accept ".app" at end of string, followed by '/', or followed by other
	// characters that form a parent path (e.g., ".app/Contents/...").
	if end < len(s) && s[end] != '/' {
		return ""
	}
	return s[:end]
}

func bundleExecutableFromPlist(bundle string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(bundle, "Contents", "Info.plist"))
	if err != nil {
		return "", false
	}
	m := bundleExecKeyRe.FindSubmatch(data)
	if len(m) < 2 {
		return "", false
	}
	name := strings.TrimSpace(string(m[1]))
	if name == "" {
		return "", false
	}
	return name, true
}
