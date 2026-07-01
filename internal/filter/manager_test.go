// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package filter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestManager_Update_PopulatesFilterPathsMap(t *testing.T) {
	list := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("! test list\n", 50)))
	}))
	defer list.Close()

	orig := DefaultSources
	DefaultSources = []ListSource{{ID: 1, Name: "adguard-base", URLs: []string{list.URL}}}
	defer func() { DefaultSources = orig }()

	m := NewManager(t.TempDir())
	if err := m.Update(context.Background(), nil); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	paths := m.FilterPathsMap()
	if len(paths) != 1 {
		t.Fatalf("expected 1 cached list, got %d: %+v", len(paths), paths)
	}
}

func TestManager_Update_AllSourcesFail_WritesEmbeddedFallback(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer dead.Close()

	orig := DefaultSources
	DefaultSources = []ListSource{{ID: 1, Name: "adguard-base", URLs: []string{dead.URL}}}
	defer func() { DefaultSources = orig }()

	m := NewManager(t.TempDir())
	if err := m.Update(context.Background(), nil); err != nil {
		t.Fatalf("Update should degrade to the embedded fallback, not fail: %v", err)
	}
	paths := m.FilterPathsMap()
	if len(paths) != 1 {
		t.Fatalf("expected embedded fallback to populate 1 list, got %d", len(paths))
	}
}

func TestManager_Status_ReflectsReadyCounts(t *testing.T) {
	m := NewManager(t.TempDir())
	st := m.Status()
	if st.ListsTotal != len(DefaultSources) {
		t.Fatalf("expected ListsTotal=%d, got %d", len(DefaultSources), st.ListsTotal)
	}
	if st.ListsReady != 0 {
		t.Fatalf("expected ListsReady=0 before any Update, got %d", st.ListsReady)
	}
}
