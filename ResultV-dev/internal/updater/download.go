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
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const maxDownloadBytes = 200 * 1024 * 1024 // 200 MB hard ceiling

// downloadFile fetches rawURL to destPath, calling fn with download progress.
// Writes to a sibling temp file and renames atomically on success.
// Partial downloads are cleaned up automatically on failure.
func downloadFile(ctx context.Context, rawURL, destPath string, expectedSize int64, fn ProgressFn) error {
	client := &http.Client{}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	contentLen := resp.ContentLength
	if contentLen > maxDownloadBytes {
		return fmt.Errorf("Content-Length %d exceeds the 200 MB limit", contentLen)
	}
	if expectedSize > 0 && contentLen > 0 && contentLen != expectedSize {
		return fmt.Errorf("Content-Length %d does not match manifest size %d", contentLen, expectedSize)
	}
	total := contentLen
	if total <= 0 && expectedSize > 0 {
		total = expectedSize
	}

	dir := filepath.Dir(destPath)
	if dir == "" {
		dir = os.TempDir()
	}
	tmp, err := os.CreateTemp(dir, "resultv-update-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := os.Chmod(tmpPath, 0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}

	var downloaded int64
	start := time.Now()
	buf := make([]byte, 32*1024)
	limited := io.LimitReader(resp.Body, maxDownloadBytes+1)

	for {
		n, readErr := limited.Read(buf)
		if n > 0 {
			if _, writeErr := tmp.Write(buf[:n]); writeErr != nil {
				tmp.Close()
				return fmt.Errorf("write to temp: %w", writeErr)
			}
			downloaded += int64(n)
			if downloaded > maxDownloadBytes {
				tmp.Close()
				return fmt.Errorf("download exceeded the 200 MB limit")
			}
			if fn != nil {
				elapsed := time.Since(start).Seconds()
				var speedBps float64
				if elapsed > 0 {
					speedBps = float64(downloaded) / elapsed
				}
				fn(downloaded, total, speedBps)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			tmp.Close()
			return fmt.Errorf("read body: %w", readErr)
		}
		// Check cancellation between chunks
		select {
		case <-ctx.Done():
			tmp.Close()
			return ctx.Err()
		default:
		}
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("rename to destination: %w", err)
	}
	return nil
}
