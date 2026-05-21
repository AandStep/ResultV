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

package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- ValidateAssetURL --------------------------------------------------------

func TestValidateAssetURL(t *testing.T) {
	allowed := []string{"github.com", "objects.githubusercontent.com", "result-proxy.ru"}

	cases := []struct {
		url     string
		wantErr bool
	}{
		{"https://github.com/owner/repo/releases/download/v1/app.exe", false},
		{"https://objects.githubusercontent.com/file.exe", false},
		{"https://result-proxy.ru/update.exe", false},
		{"https://GITHUB.COM/file.exe", false}, // case-insensitive host match
		{"http://github.com/evil.exe", true},    // not HTTPS
		{"https://evil.com/malware.exe", true},  // host not in list
		{"javascript:evil()", true},
		{"ftp://github.com/file", true},
		{"", true},
	}
	for _, tc := range cases {
		err := ValidateAssetURL(tc.url, allowed)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidateAssetURL(%q): err=%v, wantErr=%v", tc.url, err, tc.wantErr)
		}
	}
}

// ---- Manifest ----------------------------------------------------------------

func makeManifest() *Manifest {
	sha := strings.Repeat("ab", 32) // 64-char hex string
	return &Manifest{
		Version: "9.9.9",
		Platforms: map[string]*PlatformAsset{
			"windows-amd64-portable":  {URL: "https://github.com/a.exe", SHA256: sha, Size: 1024},
			"windows-amd64-installer": {URL: "https://github.com/a-setup.exe", SHA256: sha, Size: 2048},
			"darwin-universal":        {URL: "https://github.com/a.dmg", SHA256: sha, Size: 3000},
			"linux-amd64-appimage":    {URL: "https://github.com/a.AppImage", SHA256: sha, Size: 4000},
		},
	}
}

func TestManifestResolveAsset_CurrentPlatform(t *testing.T) {
	m := makeManifest()
	key := currentPlatformKey()
	asset := m.ResolveAsset()
	if key == "" {
		if asset != nil {
			t.Errorf("expected nil asset for unsupported platform, got %+v", asset)
		}
		return
	}
	if asset == nil {
		t.Fatalf("expected asset for platform key %q, got nil", key)
	}
	if asset.URL == "" {
		t.Errorf("asset URL is empty")
	}
}

func TestManifestResolveAsset_NilPlatforms(t *testing.T) {
	m := &Manifest{Version: "1.0.0"}
	if asset := m.ResolveAsset(); asset != nil {
		t.Errorf("expected nil when platforms is nil, got %+v", asset)
	}
}

func TestManifestResolveAsset_EmptyURL(t *testing.T) {
	m := &Manifest{
		Platforms: map[string]*PlatformAsset{
			currentPlatformKey(): {URL: "", SHA256: "abc"},
		},
	}
	if currentPlatformKey() == "" {
		t.Skip("no platform key on this OS")
	}
	if asset := m.ResolveAsset(); asset != nil {
		t.Errorf("expected nil for empty URL, got %+v", asset)
	}
}

// ---- SHA-256 verify ----------------------------------------------------------

func TestVerifySHA256_Match(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bin")
	content := []byte("hello update content")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(content)
	if err := verifySHA256(path, hex.EncodeToString(h[:])); err != nil {
		t.Errorf("correct hash: %v", err)
	}
}

func TestVerifySHA256_Mismatch_DeletesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tampered.bin")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	wrong := strings.Repeat("00", 32)
	if err := verifySHA256(path, wrong); err == nil {
		t.Error("expected error for wrong hash, got nil")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("tampered file should have been deleted after mismatch")
	}
}

// ---- FetchManifest -----------------------------------------------------------

func TestFetchManifest_OK(t *testing.T) {
	m := Manifest{Version: "9.9.9"}
	body, _ := json.Marshal(m)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	got, err := FetchManifest(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchManifest: %v", err)
	}
	if got.Version != "9.9.9" {
		t.Errorf("version: got %q, want %q", got.Version, "9.9.9")
	}
}

func TestFetchManifest_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := FetchManifest(context.Background(), srv.URL)
	if err == nil {
		t.Error("expected error for 404, got nil")
	}
}

func TestFetchManifest_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	_, err := FetchManifest(context.Background(), srv.URL)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

// ---- downloadFile ------------------------------------------------------------

func TestDownloadFile_OK(t *testing.T) {
	content := []byte("fake binary content for testing the downloader")
	h := sha256.Sum256(content)
	expectedSHA := hex.EncodeToString(h[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.Write(content)
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "ResultV-update-abcd1234.exe")

	var progressCalled bool
	err := downloadFile(context.Background(), srv.URL, dest, int64(len(content)),
		func(d, total int64, speed float64) { progressCalled = true })
	if err != nil {
		t.Fatalf("downloadFile: %v", err)
	}
	if !progressCalled {
		t.Error("progress callback was never called")
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Error("downloaded content mismatch")
	}
	// Verify the sha256 of the download
	if err := verifySHA256(dest, expectedSHA); err != nil {
		t.Errorf("sha256 after download: %v", err)
	}
}

func TestDownloadFile_ContentLengthExceedsLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", maxDownloadBytes+1))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	err := downloadFile(context.Background(), srv.URL, filepath.Join(dir, "test.exe"), maxDownloadBytes+1, nil)
	if err == nil {
		t.Error("expected error for over-limit Content-Length, got nil")
	}
}

func TestDownloadFile_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	dir := t.TempDir()
	err := downloadFile(context.Background(), srv.URL, filepath.Join(dir, "test.exe"), 0, nil)
	if err == nil {
		t.Error("expected error for 500, got nil")
	}
}

func TestDownloadFile_ContextCancel(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		// Block until client disconnects
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	dir := t.TempDir()

	done := make(chan error, 1)
	go func() {
		done <- downloadFile(ctx, srv.URL, filepath.Join(dir, "test.exe"), 0, nil)
	}()

	<-started
	cancel()
	err := <-done
	if err == nil {
		t.Error("expected error after context cancel, got nil")
	}
}
