// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build !no_mitm

// Browser ad-block (MITM) bindings. Built into the `full` distribution only:
// the `play` build is compiled with -tags=no_mitm, which swaps this file for
// libbox_filter_stub.go and drops internal/filter — and with it the CA
// generation and TLS-interception code — out of the linked .so entirely.
//
// The SOCKS port constant deliberately stays in libbox.go: the engine config
// builder references it in both distributions.

package mobile

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	socksproxy "golang.org/x/net/proxy"
	"resultproxy-wails/internal/filter"
)

var (
	filterMu      sync.Mutex
	filterManager *filter.Manager
)

func getFilterManager(dataDir string) *filter.Manager {
	filterMu.Lock()
	defer filterMu.Unlock()
	if filterManager == nil {
		filterManager = filter.NewManager(dataDir)
	}
	return filterManager
}

// FetchFilterLists downloads the EasyList/AdGuard text filter lists used by
// the browser MITM ad blocker into dataDir/filter. Returns JSON:
//
//	{ "ready": 5, "total": 6, "error": "" }
//
// Safe to call repeatedly. Degrades to an embedded minimal fallback list if
// every remote source fails, so this never leaves the feature completely
// unusable — see filter.Manager.Update.
func FetchFilterLists(dataDir string) (string, error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", fmt.Errorf("dataDir is required for filter list cache")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	m := getFilterManager(dataDir)
	err := m.Update(ctx, nil)
	st := m.Status()
	out := map[string]interface{}{
		"ready": st.ListsReady,
		"total": st.ListsTotal,
		"error": "",
	}
	if err != nil {
		out["error"] = err.Error()
	}
	data, jsonErr := json.Marshal(out)
	if jsonErr != nil {
		return "", fmt.Errorf("marshaling filter list result: %w", jsonErr)
	}
	return string(data), nil
}

// FilterCARootPath returns the absolute path to the (PEM-encoded) root CA
// certificate used by the browser MITM ad blocker. The Android side reads
// this file, parses it with java.security.cert.CertificateFactory, and
// hands the DER bytes to android.security.KeyChain.createInstallIntent.
func FilterCARootPath(dataDir string) (string, error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", fmt.Errorf("dataDir is required")
	}
	return getFilterManager(dataDir).CARootPath()
}

// SetFilterCASeed records a stable, device-scoped seed (the Android side
// passes Settings.Secure.ANDROID_ID) so the root CA is generated
// deterministically. Reinstalling the app then recreates the byte-identical
// CA already trusted by the system, avoiding a re-install prompt and
// duplicate trust-store entries. Must be called before the first
// FilterCARootPath/StartFilterProxy. A blank seed leaves generation random.
func SetFilterCASeed(dataDir, seed string) error {
	if strings.TrimSpace(dataDir) == "" {
		return fmt.Errorf("dataDir is required")
	}
	getFilterManager(dataDir).SetCASeed(seed)
	return nil
}

// StartFilterProxy starts the local MITM proxy on 127.0.0.1:listenPort.
// Fails if FetchFilterLists hasn't successfully populated at least one
// list yet. Returns JSON { "started": true } on success so the Kotlin
// caller has a uniform JSON-or-error contract like the rest of this file.
//
// The MITM proxy's upstream connections are routed through the engine's
// loopback SOCKS inbound (BrowserAdBlockSocksPort) so filtered browser traffic
// re-enters the tunnel — see the BrowserAdBlock docs. The engine config MUST
// have been built with BuildOptions.BrowserAdBlock=true (which adds that
// inbound); Kotlin gates both on the same setting.
func StartFilterProxy(dataDir string, listenPort int) (string, error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", fmt.Errorf("dataDir is required")
	}
	if err := getFilterManager(dataDir).StartMITM(listenPort, browserAdBlockUpstreamDial); err != nil {
		return "", err
	}
	return `{"started":true}`, nil
}

// browserAdBlockUpstreamDial dials origin servers through the engine's loopback
// SOCKS inbound so the MITM proxy's upstream traffic goes through the tunnel
// instead of leaking direct (the app process is excluded from its own VPN).
// The address is passed verbatim (hostname:port) so sing-box's SOCKS inbound
// receives the origin domain and applies the same domain route rules
// (Smart-list, exclusions, ad-block) as any other flow.
func browserAdBlockUpstreamDial(network, addr string) (net.Conn, error) {
	dialer, err := socksproxy.SOCKS5("tcp",
		fmt.Sprintf("127.0.0.1:%d", BrowserAdBlockSocksPort), nil, socksproxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("building socks dialer: %w", err)
	}
	return dialer.Dial(network, addr)
}

// StopFilterProxy stops the local MITM proxy if running. Safe to call when
// it isn't running.
func StopFilterProxy() {
	filterMu.Lock()
	m := filterManager
	filterMu.Unlock()
	if m != nil {
		m.StopMITM()
	}
}

// FilterStatus returns the browser MITM ad blocker's current status as
// JSON, for the Android Settings screen.
func FilterStatus(dataDir string) (string, error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", fmt.Errorf("dataDir is required")
	}
	st := getFilterManager(dataDir).Status()
	data, err := json.Marshal(st)
	if err != nil {
		return "", fmt.Errorf("marshaling filter status: %w", err)
	}
	return string(data), nil
}
