// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package filter

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AdguardTeam/urlfilter/rules"
)

// embeddedExtraRules is ResultV's built-in supplement to the downloaded
// filter lists: ad hosts confirmed to leak through every public source we
// subscribe to (see extraAdDeliveryDomains in internal/proxy for the
// engine-level twin that rejects them for non-browser apps too). Unlike
// embeddedFallbackRules this is ALWAYS merged into the MITM engine, not just
// when downloads fail.
const embeddedExtraRules = `! ResultV built-in supplement (always on)
||pubserv.pro^
`

// extraListID is the urlfilter list ID for the supplement. Downloaded
// sources use 1–8 and 99 (fallback); dynamically numbered ones start at 100.
const extraListID rules.ListID = 90

const extraListFileName = "resultv-extra.txt"

// mitmFilterPaths returns the cached downloaded lists plus the embedded
// supplement, writing/refreshing the supplement file on disk. Errors when no
// downloaded list is cached — the supplement alone must not satisfy
// StartMITM's precondition. A failed supplement write is non-fatal: the MITM
// still runs on the downloaded lists.
func (m *Manager) mitmFilterPaths() (map[rules.ListID]string, error) {
	paths := m.FilterPathsMap()
	if len(paths) == 0 {
		return nil, fmt.Errorf("no cached filters; call Update first")
	}
	dest := filepath.Join(m.FilterDir(), extraListFileName)
	if err := ensureFileContent(dest, []byte(embeddedExtraRules)); err == nil {
		paths[extraListID] = dest
	}
	return paths, nil
}

// ensureFileContent makes dest hold exactly content, leaving the file
// untouched (mtime preserved) when it already matches — engineCacheKey
// fingerprints size+mtime, so a no-op here keeps the cached urlfilter engine
// valid across restarts. Writes go through a temp file + rename so a crash
// never leaves a half-written list behind.
func ensureFileContent(dest string, content []byte) error {
	if existing, err := os.ReadFile(dest); err == nil && bytes.Equal(existing, content) {
		return nil
	}
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, content, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}
