// Vendored from github.com/AdguardTeam/urlfilter@v0.23.2 (proxy package),
// licensed GPL-3.0. Copyright (C) AdGuard Software Ltd.
//
// Modified by ResultV (2026-07-02): filterHTML now also strips
// <meta http-equiv="Content-Security-Policy"> tags from HTML bodies before
// computing the cosmetic-injection point. Upstream only strips CSP
// delivered via HTTP response headers (see the unmodified
// "TODO(ameshkov): HANDLE CSP PROPERLY!" comment below) — meta-tag CSP,
// common on modern sites, silently blocked the injected content-script
// script tag, which is why ad containers stayed visible as empty boxes
// instead of collapsing. See
// docs/superpowers/specs/2026-07-02-browser-adblock-hardening-design.md.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"bytes"
	"io"
	"math"
	"regexp"
	"strings"

	"github.com/AdguardTeam/golibs/log"
	"github.com/AdguardTeam/gomitmproxy/proxyutil"
)

// headBufferSize is the count of bytes where we'll be looking for one of injections points
const headBufferSize = 16 * 1024

// metaCSPPattern matches <meta http-equiv="Content-Security-Policy" ...> and
// the Report-Only variant, case-insensitively. ResultV addition — not
// present upstream.
var metaCSPPattern = regexp.MustCompile(`(?i)<meta[^>]+http-equiv\s*=\s*["']Content-Security-Policy(-Report-Only)?["'][^>]*>`)

// stripMetaCSP removes <meta http-equiv="Content-Security-Policy"> tags from
// decoded HTML. ResultV addition — not present upstream.
func stripMetaCSP(body string) string {
	return metaCSPPattern.ReplaceAllString(body, "")
}

// filterHTML replaces the original response with the one where the body is modified
func (s *Server) filterHTML(session *Session) error {
	res := session.HTTPResponse

	b, err := proxyutil.ReadDecompressedBody(res)
	// Close the original body
	_ = res.Body.Close()
	if err != nil {
		log.Error("urlfilter id=%s: could not read the full body: %v", session.ID, err)
		return err
	}

	// Use latin1 before modifying the body
	// Using this 1-byte encoding will let us preserve all original characters
	// regardless of what exactly is the encoding
	body, err := proxyutil.DecodeLatin1(bytes.NewReader(b))
	if err != nil {
		log.Error("urlfilter id=%s: could not decode the body: %v", session.ID, err)
		return err
	}

	// ResultV: strip <meta> CSP before computing the injection index, so a
	// removed tag can't shift byte offsets out from under
	// findBodyInjectionIndex below.
	body = stripMetaCSP(body)

	// Modifying the original body
	modifiedBody := body
	index := findBodyInjectionIndex(body)
	if index != -1 {
		// TODO(ameshkov): HANDLE CSP PROPERLY!
		session.HTTPResponse.Header.Del("Content-Security-Policy")
		session.HTTPResponse.Header.Del("Content-Security-Policy-Report-Only")
		injection := s.buildInjectionCode(session)
		modifiedBody = body[:index] + injection + body[index:]
	}

	b, err = proxyutil.EncodeLatin1(modifiedBody)
	if err != nil {
		log.Error("urlfilter id=%s: could not encode body: %v", session.ID, err)
		return err
	}

	res.Body = io.NopCloser(bytes.NewReader(b))
	res.Header.Del("Content-Encoding")
	res.ContentLength = int64(len(b))
	return nil
}

// findBodyInjectionIndex finds a place where we can inject the content script
func findBodyInjectionIndex(body string) int {
	cnt := int(math.Min(headBufferSize, float64(len(body))))
	for i := 0; i < cnt; i++ {
		if isMatchFound(body, "</head", i) ||
			isMatchFound(body, "<link", i) ||
			isMatchFound(body, "<style", i) ||
			isMatchFound(body, "<script", i) {
			return i
		}
	}

	return -1
}

// isMatchFound checks if body
func isMatchFound(body, match string, index int) bool {
	if index+len(match) > len(body) {
		return false
	}

	str := body[index : index+len(match)]
	return strings.EqualFold(str, match)
}
