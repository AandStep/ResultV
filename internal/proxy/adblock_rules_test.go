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
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/common/srs"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
)

// validSRSBytes builds a real, parseable sing-box binary rule-set — the same
// on-disk format sing-box loads — so tests exercise the true validation path
// instead of a stand-in byte blob. It carries enough distinct domains to clear
// the minLocalSRSBytes floor even after zlib compression (a real ad list is far
// larger; a one-rule SRS would be smaller than the floor).
func validSRSBytes(t *testing.T) []byte {
	t.Helper()
	domains := make([]string, 0, 4000)
	for i := 0; i < 4000; i++ {
		domains = append(domains, fmt.Sprintf("ad-%d-%x.tracker-example.com", i, i*2654435761))
	}
	rs := option.PlainRuleSet{
		Rules: []option.HeadlessRule{{
			Type: C.RuleTypeDefault,
			DefaultOptions: option.DefaultHeadlessRule{
				Domain: badoption.Listable[string](domains),
			},
		}},
	}
	var buf bytes.Buffer
	if err := srs.Write(&buf, rs, C.RuleSetVersionCurrent); err != nil {
		t.Fatalf("building test SRS: %v", err)
	}
	if buf.Len() < minLocalSRSBytes {
		t.Fatalf("test SRS smaller than min gate: %d < %d", buf.Len(), minLocalSRSBytes)
	}
	return buf.Bytes()
}

// TestDownloadAdBlockSRS_FailoverToMirror pins Bug C: when the first URL
// (github raw, often blocked in RU) fails, the downloader must fall over to the
// next mirror and still write the SRS.
func TestDownloadAdBlockSRS_FailoverToMirror(t *testing.T) {
	payload := validSRSBytes(t)
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

// TestValidateSRS_AcceptsValid confirms a well-formed SRS passes validation.
func TestValidateSRS_AcceptsValid(t *testing.T) {
	if err := validateSRS(validSRSBytes(t)); err != nil {
		t.Fatalf("valid SRS rejected: %v", err)
	}
}

// TestValidateSRS_RejectsTruncated reproduces the field failure: a SRS whose
// tail was lost (interrupted download) fails the zlib checksum on read. This is
// exactly the "restore cached rule-set: read rule[0] zlib invalid checksum"
// error that bricked ad-block startup.
func TestValidateSRS_RejectsTruncated(t *testing.T) {
	full := validSRSBytes(t)
	truncated := full[:len(full)-16] // drop the zlib Adler-32 tail + some data
	if err := validateSRS(truncated); err == nil {
		t.Fatal("truncated SRS accepted; want rejection")
	}
}

// TestValidateSRS_RejectsGarbage rejects a non-SRS body (e.g. an HTML error
// page or a redirect the CDN served with a 200).
func TestValidateSRS_RejectsGarbage(t *testing.T) {
	garbage := bytes.Repeat([]byte("<html>not an srs</html>"), 64)
	if err := validateSRS(garbage); err == nil {
		t.Fatal("garbage accepted as SRS; want rejection")
	}
}

// TestFetchAdBlockSRS_RejectsCorrupt ensures the downloader refuses a corrupt
// body so it never lands on disk as a local rule-set (and fails over to a
// mirror instead).
func TestFetchAdBlockSRS_RejectsCorrupt(t *testing.T) {
	corrupt := validSRSBytes(t)
	corrupt = corrupt[:len(corrupt)-16]
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(corrupt)
	}))
	defer srv.Close()
	_, err := fetchAdBlockSRS(context.Background(), &http.Client{Timeout: 5 * time.Second}, srv.URL)
	if err == nil {
		t.Fatal("fetch accepted a corrupt SRS; want error")
	}
}

// TestBuildAdBlockRuleSets_CorruptLocalDropped guards a corrupt SRS cached from
// before validation existed: it must NOT be referenced as a local rule-set
// (which fails sing-box startup), must NOT emit a remote fallback, and must be
// removed so a fresh download can replace it.
func TestBuildAdBlockRuleSets_CorruptLocalDropped(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(adBlockRuleSetsDir(dir), 0o700); err != nil {
		t.Fatal(err)
	}
	// A corrupt file that clears the size floor but fails SRS parsing.
	corruptPath := filepath.Join(adBlockRuleSetsDir(dir), defaultAdBlockRuleSets[0].fileName)
	if err := os.WriteFile(corruptPath, bytes.Repeat([]byte("x"), minLocalSRSBytes+16), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := buildAdBlockRuleSets(dir); len(got) != 0 {
		t.Fatalf("corrupt local SRS must be dropped, got %+v", got)
	}
	if _, err := os.Stat(corruptPath); !os.IsNotExist(err) {
		t.Fatalf("corrupt local SRS not removed (self-heal); stat err=%v", err)
	}
}

func TestAvailableAdBlockRuleSetTags(t *testing.T) {
	dir := t.TempDir()
	if got := availableAdBlockRuleSetTags(dir); len(got) != 0 {
		t.Fatalf("expected no tags with an empty cache, got %v", got)
	}
	if err := os.MkdirAll(adBlockRuleSetsDir(dir), 0o700); err != nil {
		t.Fatal(err)
	}
	// Cache only the first list → only its tag is available.
	if err := os.WriteFile(filepath.Join(adBlockRuleSetsDir(dir), defaultAdBlockRuleSets[0].fileName), validSRSBytes(t), 0o600); err != nil {
		t.Fatal(err)
	}
	got := availableAdBlockRuleSetTags(dir)
	if len(got) != 1 || got[0] != defaultAdBlockRuleSets[0].tag {
		t.Fatalf("expected only %q, got %v", defaultAdBlockRuleSets[0].tag, got)
	}
}

// TestBuildAdBlockRuleSets_ValidLocalUsedLocally confirms a valid cached SRS is
// referenced locally (offline, and bypassing the poisonable remote cache).
func TestBuildAdBlockRuleSets_ValidLocalUsedLocally(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(adBlockRuleSetsDir(dir), 0o700); err != nil {
		t.Fatal(err)
	}
	tag := defaultAdBlockRuleSets[0].tag
	path := filepath.Join(adBlockRuleSetsDir(dir), defaultAdBlockRuleSets[0].fileName)
	if err := os.WriteFile(path, validSRSBytes(t), 0o600); err != nil {
		t.Fatal(err)
	}
	sets := buildAdBlockRuleSets(dir)
	for i := range sets {
		if sets[i].Tag == tag {
			if sets[i].Type != "local" {
				t.Fatalf("valid local SRS emitted as %q; want local", sets[i].Type)
			}
			return
		}
	}
	t.Fatalf("no rule-set emitted for tag %q", tag)
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
