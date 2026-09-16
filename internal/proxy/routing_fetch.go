// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

// Fetching what a routing profile points at: rule lists and geo databases.
//
// Every URL here arrives from outside — a deep link anyone can send, or a
// provider's subscription. So the fetch is bounded three ways: only http(s),
// plaintext only with explicit consent, and never to a private or loopback
// address.
//
// The last check sits on the dialer's Control hook, AFTER name resolution. The
// string check this tree already had (isPrivateOrLoopbackHost in mobile) only
// looks at the URL, so a hostname that resolves to 127.0.0.1 walks straight
// past it and reaches whatever the device is running locally.

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"
)

const (
	// routingFetchMaxBytes bounds a response body. Real geo databases are
	// ~0.5 MiB and rule lists smaller; 8 MiB is roomy and still finite.
	routingFetchMaxBytes = 8 << 20
	// A list host (often a CDN edge) occasionally stalls before sending
	// headers. Retry a few times with a short per-attempt timeout so a one-off
	// stall recovers in a second or two instead of failing the whole compile.
	routingFetchAttempts       = 3
	routingFetchAttemptTimeout = 15 * time.Second
)

// safeRoutingDialer refuses to connect to private, loopback, link-local or
// unspecified addresses. Control fires after the address is resolved and before
// the socket connects, which is the only point where the real destination is
// known.
func safeRoutingDialer() *net.Dialer {
	return &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("routing fetch: bad address %q", address)
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("routing fetch: cannot parse %q", host)
			}
			if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
				ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
				return fmt.Errorf("routing fetch: blocked private/loopback target %s", ip)
			}
			return nil
		},
	}
}

// FetchRoutingPayload downloads one rule list or geo database.
//
// allowInsecure carries down the consent the user already gave for a provider;
// it never grants itself one.
func FetchRoutingPayload(ctx context.Context, rawURL string, allowInsecure bool) ([]byte, error) {
	client := &http.Client{
		Timeout:   routingFetchAttemptTimeout,
		Transport: &http.Transport{DialContext: safeRoutingDialer().DialContext},
	}
	return fetchRoutingPayloadVia(ctx, client, rawURL, allowInsecure)
}

// fetchRoutingPayloadVia is FetchRoutingPayload with the client supplied.
//
// It exists for the tests. httptest always listens on loopback, which is
// exactly what safeRoutingDialer blocks — so retries, status handling and the
// size cap could not be covered at all without a way in. Production callers use
// FetchRoutingPayload and get the guarded client.
func fetchRoutingPayloadVia(ctx context.Context, client *http.Client, rawURL string, allowInsecure bool) ([]byte, error) {
	// Rewrite a GitHub blob (web page) URL to its raw form so an ordinary
	// github.com link fetches the file, not the HTML page. Idempotent, so it
	// also fixes blob URLs stored by an older build on refresh.
	u := NormalizeRoutingListURL(strings.TrimSpace(rawURL))
	if u == "" {
		return nil, fmt.Errorf("routing fetch: empty URL")
	}
	lower := strings.ToLower(u)
	isHTTP := strings.HasPrefix(lower, "http://")
	isHTTPS := strings.HasPrefix(lower, "https://")
	if isHTTP && !allowInsecure {
		return nil, fmt.Errorf("routing fetch: insecure http:// requires explicit consent")
	}
	if !isHTTP && !isHTTPS {
		return nil, fmt.Errorf("routing fetch: unsupported URL scheme")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var lastErr error
	for attempt := 1; attempt <= routingFetchAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		body, status, err := tryFetchRoutingPayload(ctx, client, u)
		if err == nil && status == http.StatusOK {
			return body, nil
		}
		if err != nil {
			lastErr = err
			continue
		}
		lastErr = fmt.Errorf("routing fetch: http %d", status)
		// A definitive client error (not found / auth / gone) will not fix
		// itself on retry. Retry only on 5xx and 429.
		if status >= 400 && status < 500 && status != http.StatusTooManyRequests {
			break
		}
	}
	return nil, lastErr
}

func tryFetchRoutingPayload(ctx context.Context, client *http.Client, u string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, routingFetchMaxBytes))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}
