// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build windows

package system

import (
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sys/windows/registry"
)

// webView2ClientGUID is the well-known WebView2 Evergreen Runtime app ID used
// by the Microsoft Edge updater. The same key holds the installed runtime's
// version string under the "pv" REG_SZ value.
const webView2ClientGUID = `{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`

var (
	webViewVersionOnce sync.Once
	webViewVersion     string
)

func detectWebViewVersion() string {
	paths := []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\` + webView2ClientGUID},
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\EdgeUpdate\Clients\` + webView2ClientGUID},
		{registry.CURRENT_USER, `Software\Microsoft\EdgeUpdate\Clients\` + webView2ClientGUID},
	}
	for _, p := range paths {
		k, err := registry.OpenKey(p.root, p.path, registry.QUERY_VALUE|registry.WOW64_64KEY)
		if err != nil {
			continue
		}
		v, _, err := k.GetStringValue("pv")
		k.Close()
		if err == nil && strings.TrimSpace(v) != "" && v != "0.0.0.0" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// WebView2Version returns the installed Evergreen WebView2 Runtime version
// (e.g. "131.0.2903.86") or "" if not installed. Result is cached for the
// lifetime of the process — the runtime can't change without restarting the
// app.
func WebView2Version() string {
	webViewVersionOnce.Do(func() {
		webViewVersion = detectWebViewVersion()
	})
	return webViewVersion
}

// WebViewFingerprint returns the uTLS fingerprint name that best matches the
// runtime serving WebView2 on this machine. Edge WebView2 ships with the same
// network stack as Microsoft Edge, so we map any detected version to "edge"
// (sing-box understands this and routes it to utls.HelloEdge_Auto). Falls
// back to "chrome" when WebView2 is missing — the broader Chromium family
// fingerprint is the safest default for a Wails app.
func WebViewFingerprint() string {
	if WebView2Version() != "" {
		return "edge"
	}
	return "chrome"
}

// WebView2MajorVersion parses the leading integer from WebView2Version()
// (e.g. 131 for "131.0.2903.86"). Returns 0 when WebView2 is not present
// or the version string is malformed. Useful for log lines.
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
