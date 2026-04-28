// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"errors"
	"fmt"
	urlpkg "net/url"
	"strings"
)

// DeepLinkScheme is the custom URL scheme handled by ResultV. Browsers and
// the OS open `resultv://...` URLs in the running app instance.
const DeepLinkScheme = "resultv://"

// deepLinkSchemeOpaque is the opaque form (no `//`) for browsers that choke on
// hierarchical URLs containing colons in unexpected places.
const deepLinkSchemeOpaque = "resultv:"

// IsDeepLink reports whether s starts with the resultv:// or resultv: scheme.
func IsDeepLink(s string) bool {
	low := strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(low, DeepLinkScheme) || strings.HasPrefix(low, deepLinkSchemeOpaque)
}

// DecodeDeepLink unwraps a resultv:// URL into a subscription payload that
// the app can hand off to the regular subscription pipeline. The returned
// payload is either:
//   - a single-line http(s) URL (the caller fetches it), or
//   - a multi-line text body containing proxy URIs / base64 / JSON.
//
// Supported URL shapes (preferred form first):
//   - resultv://import/<base64-of-RVSUB1-aes-gcm-ciphertext>   (preferred)
//   - resultv:import/<base64>                                  (opaque)
//   - resultv://crypt4/<base64>                                (happ-compat)
//   - resultv://RVSUB1:<base64>                                (legacy)
//   - resultv://<base64>                                       (raw)
//   - resultv://plain/<url-or-uri-list>                        (debug)
//
// The encrypted variants reuse the same key as RVSUB1 subscription bodies,
// so a deeplink and a copy-pasted RVSUB1 string decrypt with the same key.
//
// IMPORTANT for browser compatibility: prefer the `resultv://import/<base64>`
// shape. URLs with a colon directly after the scheme host (`resultv://RVSUB1:...`)
// are treated by Chrome's URL parser as `userinfo:password@host` syntax with a
// missing host, and silently rejected with `about:blank#blocked`.
func DecodeDeepLink(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	// Some shells append NULs / control bytes / a trailing slash to %1.
	rawURL = strings.TrimRight(rawURL, "/\x00\r\n\t ")
	if !IsDeepLink(rawURL) {
		return "", errors.New("not a resultv:// deep link")
	}
	low := strings.ToLower(rawURL)
	var body string
	switch {
	case strings.HasPrefix(low, DeepLinkScheme):
		body = rawURL[len(DeepLinkScheme):]
	case strings.HasPrefix(low, deepLinkSchemeOpaque):
		body = rawURL[len(deepLinkSchemeOpaque):]
	}
	body = strings.TrimLeft(body, "/")
	body = strings.TrimRight(body, "/\x00\r\n\t ")

	// Strip optional path/format marker. We accept "crypt4/" for compat with
	// happ-style links, "RVSUB1:" for direct copy-pastes of an encrypted body,
	// and treat anything else as the raw ciphertext.
	lowBody := strings.ToLower(body)
	switch {
	case strings.HasPrefix(lowBody, "plain/"):
		plain := strings.TrimSpace(body[len("plain/"):])
		if plain == "" {
			return "", errors.New("resultv://plain/ payload is empty")
		}
		return plain, nil
	case strings.HasPrefix(lowBody, "import/"):
		body = body[len("import/"):]
	case strings.HasPrefix(lowBody, "crypt4/"):
		body = body[len("crypt4/"):]
	case strings.HasPrefix(lowBody, "i/"):
		body = body[len("i/"):]
	case strings.HasPrefix(body, subscriptionMagic):
		body = body[len(subscriptionMagic):]
	case len(body) > len(subscriptionMagic) &&
		strings.EqualFold(body[:len(subscriptionMagic)], subscriptionMagic):
		body = body[len(subscriptionMagic):]
	}

	body = strings.TrimSpace(body)
	if body == "" {
		return "", errors.New("resultv:// payload is empty")
	}

	// Some browsers / shells percent-encode the URL before passing it on.
	if decoded, err := urlpkg.QueryUnescape(body); err == nil && decoded != "" {
		body = decoded
	}

	// Strip anything that is not a valid base64 character. Whitespace, line
	// breaks, NULs, stray slashes and percent-encoded artefacts can sneak in
	// when the URL travels through a shell argument or copy/paste.
	body = sanitizeBase64(body)
	if body == "" {
		return "", errors.New("resultv:// payload contains no base64 characters")
	}

	// Reuse the RVSUB1 decryption pipeline by re-prefixing the magic header.
	plain, err := tryDecryptSubscription(subscriptionMagic + body)
	if err != nil {
		return "", fmt.Errorf("decoding resultv:// payload: %w", err)
	}
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return "", errors.New("resultv:// payload decrypted to empty content")
	}
	return plain, nil
}

func sanitizeBase64(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z',
			r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '+', r == '/', r == '=', r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	return b.String()
}
