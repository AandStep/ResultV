// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build linux

package system

import (
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// On Linux, Wails uses WebKitGTK. Multiple parallel-installable ABIs exist;
// we probe them in order of how recent the API is, since newer Wails builds
// link against newer ones. pkg-config is the canonical source — falls back
// silently if the dev tools aren't installed (most end-user systems won't
// have pkg-config), in which case the version is unknown but the app still
// works fine.
var webKitGTKPackages = []string{
	"webkitgtk-6.0",   // GTK4, current
	"webkit2gtk-4.1",  // GTK3 with libsoup3
	"webkit2gtk-4.0",  // GTK3 with libsoup2, legacy
}

var (
	webViewVersionOnce sync.Once
	webViewVersion     string
)

func detectWebViewVersion() string {
	for _, pkg := range webKitGTKPackages {
		out, err := exec.Command("pkg-config", "--modversion", pkg).Output()
		if err != nil {
			continue
		}
		if v := strings.TrimSpace(string(out)); v != "" {
			return v
		}
	}
	return ""
}

// WebView2Version returns the installed WebKitGTK version (e.g. "2.42.5") or
// "" when neither pkg-config nor any known webkit-gtk package is present.
// Name kept WebView2-prefixed for cross-platform symmetry; on Linux the
// engine is WebKitGTK, not WebView2.
func WebView2Version() string {
	webViewVersionOnce.Do(func() {
		webViewVersion = detectWebViewVersion()
	})
	return webViewVersion
}

// WebViewFingerprint returns "safari" — WebKitGTK is the Linux port of the
// same WebKit engine that powers Safari, so its TLS behaviour through
// libsoup/GnuTLS is closer to Safari than to Chrome. sing-box maps "safari"
// to utls.HelloSafari_Auto.
func WebViewFingerprint() string { return "safari" }

// WebView2MajorVersion parses the major component of the WebKitGTK version
// (e.g. 2 from "2.42.5"). Returns 0 when detection failed.
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
