// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package filter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManager_LoadCacheEmpty(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	if err := m.LoadCache(); err != nil {
		t.Fatal(err)
	}
	if m.NeedsUpdate() != true {
		t.Fatal("expected needs update on empty cache")
	}
}

func TestManager_SaveMetaRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	path := filepath.Join(m.FilterDir(), "easylist.txt")
	if err := os.MkdirAll(m.FilterDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	m.meta.Sources = map[string]string{"easylist": path}
	payload := strings.Repeat("||ads.example.com^\n", 30)
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	m.meta.UpdatedAt = 12345
	if err := m.saveMeta(); err != nil {
		t.Fatal(err)
	}
	m2 := NewManager(dir)
	if err := m2.LoadCache(); err != nil {
		t.Fatal(err)
	}
	if m2.meta.UpdatedAt != 12345 {
		t.Fatalf("updatedAt: got %d", m2.meta.UpdatedAt)
	}
	if len(m2.meta.Sources) != 1 {
		t.Fatalf("sources: %+v", m2.meta.Sources)
	}
}
