// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build darwin

package system

import (
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// On macOS, Wails uses WKWebView, which is part of the system and ships with
// Safari/WebKit. The version we surface here is the OS version (sw_vers
// -productVersion), since that's what determines the WKWebView/Safari build
// the app is actually using. Cached because the answer never changes within
// a process.
var (
	webViewVersionOnce sync.Once
	webViewVersion     string
)

func detectWebViewVersion() string {
	out, err := exec.Command("sw_vers", "-productVersion").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// WebView2Version returns the macOS product version (e.g. "14.4.1"), which
// stands in for the WKWebView/Safari version since they're tied to the OS.
// Name kept WebView2-prefixed for cross-platform symmetry, even though the
// engine isn't WebView2 here.
func WebView2Version() string {
	webViewVersionOnce.Do(func() {
		webViewVersion = detectWebViewVersion()
	})
	return webViewVersion
}

// WebViewFingerprint returns "safari" — WKWebView is Safari's network stack
// (NSURLSession + SecureTransport/Network.framework), so the closest uTLS
// preset is HelloSafari_Auto. sing-box maps "safari" to it directly.
func WebViewFingerprint() string { return "safari" }

// WebView2MajorVersion parses the macOS major version (e.g. 14 from "14.4.1").
// Returns 0 when sw_vers failed or the string is malformed.
func WebView2MajorVersion() int {
	v := WebView2Version()
	if v == "" {
		return 0
	}
	if i := strings.IndexByte(v, '.'); i > 0 {
		v = v[:i]
	}
	n, _ := strconv.Atoi(v)
	return n
}
