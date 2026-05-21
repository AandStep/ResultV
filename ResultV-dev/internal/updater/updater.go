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
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

// ProgressFn is called during download with bytes downloaded, total size, and
// current speed in bytes per second. total may be 0 if unknown.
type ProgressFn func(downloaded, total int64, speedBps float64)

// ManifestURLOverride replaces the production manifest URL when set via
// -ldflags "-X resultproxy-wails/internal/updater.ManifestURLOverride=URL".
// Used to point dev/test builds at a separate update.json on the dev branch.
var ManifestURLOverride string

const productionManifestURL = "https://raw.githubusercontent.com/AandStep/ResultV/main/update.json"

// Updater manages the in-app update lifecycle: check → download → verify → install.
type Updater struct {
	AllowedHosts []string // hostnames allowed to serve update artifacts
	ManifestURL  string   // URL of update.json
	DownloadDir  string   // directory for temporary download files
}

// New returns an Updater with production defaults.
// URL resolution priority (highest to lowest):
//  1. ManifestURLOverride — set via -ldflags at build time (CI dev builds)
//  2. RESULTV_MANIFEST_URL env var — for manual local testing without rebuilding
//  3. productionManifestURL — hardcoded production default
func New() *Updater {
	manifestURL := productionManifestURL
	if ManifestURLOverride != "" {
		manifestURL = ManifestURLOverride
	} else if env := os.Getenv("RESULTV_MANIFEST_URL"); env != "" {
		manifestURL = env
	}
	return &Updater{
		AllowedHosts: []string{
			"github.com",
			"objects.githubusercontent.com",
			"result-proxy.ru",
			"www.result-proxy.ru",
		},
		ManifestURL: manifestURL,
		DownloadDir: os.TempDir(),
	}
}

// Check fetches and returns the latest manifest.
func (u *Updater) Check(ctx context.Context) (*Manifest, error) {
	return FetchManifest(ctx, u.ManifestURL)
}

// Download downloads the artifact described by asset, reporting progress via fn.
// Returns the local path of the verified downloaded file.
func (u *Updater) Download(ctx context.Context, asset *PlatformAsset, fn ProgressFn) (string, error) {
	if err := ValidateAssetURL(asset.URL, u.AllowedHosts); err != nil {
		return "", fmt.Errorf("asset URL rejected: %w", err)
	}

	// Use first 8 hex chars of SHA-256 as filename suffix for uniqueness.
	suffix := asset.SHA256
	if len(suffix) > 8 {
		suffix = suffix[:8]
	}

	// Preserve the original file extension so installers run correctly.
	ext := filepath.Ext(assetFilename(asset.URL))
	if ext == "" {
		ext = ".bin"
	}
	destPath := filepath.Join(u.DownloadDir, "ResultV-update-"+suffix+ext)

	if err := downloadFile(ctx, asset.URL, destPath, asset.Size, fn); err != nil {
		os.Remove(destPath)
		return "", err
	}
	return destPath, nil
}

// Verify checks the SHA-256 of path against the hex string in expectedSHA256.
func (u *Updater) Verify(path, expectedSHA256 string) error {
	return verifySHA256(path, expectedSHA256)
}

// Install applies the update at path and exits the current process on success.
// It delegates to the platform-specific installUpdate function.
func (u *Updater) Install(path string) error {
	return installUpdate(path)
}

// assetFilename extracts the last path segment of rawURL (the filename).
func assetFilename(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return filepath.Base(u.Path)
}
