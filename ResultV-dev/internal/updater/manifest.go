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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PlatformAsset holds the download metadata for one release artifact.
type PlatformAsset struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Manifest is the parsed update.json.
type Manifest struct {
	Version     string                    `json:"version"`
	DownloadURL string                    `json:"downloadUrl"`
	Platforms   map[string]*PlatformAsset `json:"platforms"`
}

// FetchManifest downloads and parses update.json from manifestURL.
func FetchManifest(ctx context.Context, manifestURL string) (*Manifest, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build manifest request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch manifest: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest server returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &m, nil
}

// ResolveAsset returns the PlatformAsset for the current OS/binary type,
// or nil if no in-app update asset is available for this configuration.
// The platform key is determined by the build-tag-selected currentPlatformKey().
func (m *Manifest) ResolveAsset() *PlatformAsset {
	key := currentPlatformKey()
	if key == "" || m.Platforms == nil {
		return nil
	}
	a := m.Platforms[key]
	if a == nil || a.URL == "" || a.SHA256 == "" {
		return nil
	}
	return a
}

// ValidateAssetURL verifies rawURL is HTTPS and its hostname is in allowedHosts.
func ValidateAssetURL(rawURL string, allowedHosts []string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("download URL must use HTTPS, got %q", u.Scheme)
	}
	host := strings.ToLower(u.Hostname())
	for _, h := range allowedHosts {
		if host == strings.ToLower(h) {
			return nil
		}
	}
	return fmt.Errorf("download host %q is not in the allowed list", host)
}
