// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package filter

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDownloadFirstOK_GitHubMirror(t *testing.T) {
	if os.Getenv("RESULTV_FILTER_NETWORK_TEST") == "" {
		t.Skip("set RESULTV_FILTER_NETWORK_TEST=1 to run")
	}
	dir := t.TempDir()
	dest := filepath.Join(dir, "base.txt")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client := newFilterHTTPClient()
	err := downloadFirstOK(ctx, client, DefaultSources[0].URLs, dest)
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(dest)
	if err != nil || st.Size() < minFilterBytes {
		t.Fatalf("file too small: %v", err)
	}
}

func TestWriteEmbeddedFallback(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "fallback.txt")
	if err := writeEmbeddedFallback(dest); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(dest)
	if err != nil || st.Size() < minFilterBytes {
		t.Fatalf("fallback file: %v", err)
	}
}
