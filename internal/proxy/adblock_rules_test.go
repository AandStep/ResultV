// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDownloadAdBlockSRS_FailoverToMirror pins Bug C: when the first URL
// (github raw, often blocked in RU) fails, the downloader must fall over to the
// next mirror and still write the SRS.
func TestDownloadAdBlockSRS_FailoverToMirror(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), minLocalSRSBytes+16)
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer good.Close()

	dir := t.TempDir()
	src := adBlockRuleSetSource{
		tag:      "ads",
		fileName: "ads.srs",
		// First URL is unroutable → must fail over to the httptest server.
		urls: []string{"http://127.0.0.1:1/nope", good.URL},
	}
	if err := downloadAdBlockSRS(context.Background(), &http.Client{Timeout: 5 * time.Second}, src, dir); err != nil {
		t.Fatalf("expected failover to succeed, got %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "ads.srs"))
	if err != nil {
		t.Fatalf("SRS not written: %v", err)
	}
	if len(got) != len(payload) {
		t.Fatalf("SRS length mismatch: got %d want %d", len(got), len(payload))
	}
}

// TestDownloadAdBlockSRS_AllFail_NoClobber verifies a failed refresh never
// clobbers a previously cached SRS.
func TestDownloadAdBlockSRS_AllFail_NoClobber(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "ads.srs")
	cached := []byte("previously-cached-srs")
	if err := os.WriteFile(dst, cached, 0o600); err != nil {
		t.Fatal(err)
	}
	src := adBlockRuleSetSource{
		tag:      "ads",
		fileName: "ads.srs",
		urls:     []string{"http://127.0.0.1:1/a", "http://127.0.0.1:1/b"},
	}
	if err := downloadAdBlockSRS(context.Background(), &http.Client{Timeout: 2 * time.Second}, src, dir); err == nil {
		t.Fatal("expected an error when every URL fails")
	}
	got, err := os.ReadFile(dst)
	if err != nil || !bytes.Equal(got, cached) {
		t.Fatalf("cached SRS was clobbered on failure: err=%v got=%q", err, got)
	}
}

// TestDefaultAdBlockRuleSets_HaveMirrors ensures every SRS source ships a
// non-github fallback (the RU resilience path).
func TestDefaultAdBlockRuleSets_HaveMirrors(t *testing.T) {
	for _, s := range defaultAdBlockRuleSets {
		if len(s.urls) < 2 {
			t.Fatalf("%s: expected >=2 URLs (github + mirror), got %v", s.tag, s.urls)
		}
	}
}
