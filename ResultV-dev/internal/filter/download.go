// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package filter

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	minFilterBytes = 256
	maxRetries     = 3
	maxSRSRetries  = 2
)

func newRuleSetHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 45 * time.Second,
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			ForceAttemptHTTP2:   true,
			MaxIdleConns:        8,
			IdleConnTimeout:     30 * time.Second,
			TLSHandshakeTimeout: 15 * time.Second,
		},
	}
}

func newFilterHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 90 * time.Second,
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			ForceAttemptHTTP2:   true,
			MaxIdleConns:        8,
			IdleConnTimeout:     30 * time.Second,
			TLSHandshakeTimeout: 20 * time.Second,
		},
	}
}

func downloadSRSFirstOK(ctx context.Context, client *http.Client, urls []string, dest string) error {
	return downloadFirstOKWithMin(ctx, client, urls, dest, minSRSBytes, maxSRSRetries)
}

func downloadFirstOK(ctx context.Context, client *http.Client, urls []string, dest string) error {
	return downloadFirstOKWithMin(ctx, client, urls, dest, minFilterBytes, maxRetries)
}

func downloadFirstOKWithMin(ctx context.Context, client *http.Client, urls []string, dest string, minBytes int64, retries int) error {
	var lastErr error
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		for attempt := 0; attempt < retries; attempt++ {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			err := downloadFileOnce(ctx, client, u, dest, minBytes)
			if err == nil {
				return nil
			}
			lastErr = err
			if attempt+1 < retries {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Duration(attempt+1) * time.Second):
				}
			}
		}
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("no urls to download")
}

func downloadFileOnce(ctx context.Context, client *http.Client, url, dest string, minBytes int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "ResultV/1.0 (+https://result-proxy.ru)")
	req.Header.Set("Accept", "*/*")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d: %s", resp.StatusCode, url)
	}

	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, maxListBytes))
	closeErr := f.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if n < minBytes {
		_ = os.Remove(tmp)
		return fmt.Errorf("response too small (%d bytes): %s", n, url)
	}
	return os.Rename(tmp, dest)
}

func writeEmbeddedFallback(dest string) error {
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, []byte(embeddedFallbackRules), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}
