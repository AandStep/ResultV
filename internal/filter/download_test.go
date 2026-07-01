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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadFirstOK_FallsThroughToSecondURL(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("x", minFilterBytes+10)))
	}))
	defer good.Close()

	dest := filepath.Join(t.TempDir(), "list.txt")
	client := newFilterHTTPClient()
	ctx := context.Background()
	if err := downloadFirstOK(ctx, client, []string{bad.URL, good.URL}, dest); err != nil {
		t.Fatalf("expected fallthrough to succeed, got %v", err)
	}
	b, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < minFilterBytes {
		t.Fatalf("expected at least %d bytes, got %d", minFilterBytes, len(b))
	}
}

func TestDownloadFirstOK_TooSmallResponseRejected(t *testing.T) {
	tiny := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("too small"))
	}))
	defer tiny.Close()

	dest := filepath.Join(t.TempDir(), "list.txt")
	client := newFilterHTTPClient()
	ctx := context.Background()
	err := downloadFirstOK(ctx, client, []string{tiny.URL}, dest)
	if err == nil {
		t.Fatal("expected an error for a too-small response")
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		t.Fatal("dest file should not exist after a rejected download")
	}
}

func TestWriteEmbeddedFallback_WritesValidFile(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "fallback.txt")
	if err := writeEmbeddedFallback(dest); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "doubleclick.net") {
		t.Fatalf("expected fallback rules to mention doubleclick.net, got: %s", string(b))
	}
}
