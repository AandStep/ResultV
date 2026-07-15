// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package mitm

import (
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"net"
	"time"

	"github.com/AdguardTeam/gomitmproxy"
	"github.com/AdguardTeam/gomitmproxy/mitm"
	filterproxy "resultproxy-wails/internal/filter/mitm/vendoredproxy"
	"github.com/AdguardTeam/urlfilter"
	"github.com/AdguardTeam/urlfilter/rules"
)

// Config configures the local MITM filtering proxy.
type Config struct {
	ListenPort  int
	RootCert    *x509.Certificate
	RootKey     *rsa.PrivateKey
	FilterPaths map[rules.ListID]string
	OnBlocked   func(cosmetic bool)
	// Engine, when non-nil, is reused instead of rebuilding from FilterPaths.
	// The Manager caches it across proxy restarts to keep the browser ad-block
	// attach fast on every connect after the first.
	Engine *urlfilter.Engine
	// UpstreamDial, when non-nil, is used for every connection the proxy makes
	// to an origin server. On Android the MITM proxy runs inside the app
	// process, which is excluded from the VPN (addDisallowedApplication), so a
	// plain net.Dial bypasses the tunnel — fatal for RKN-blocked sites in the
	// browser. The caller passes a SOCKS5 dialer aimed at the engine's loopback
	// inbound so upstream traffic re-enters the tunnel. Nil = direct dial
	// (desktop, where the process is not tunnel-excluded).
	UpstreamDial func(network, addr string) (net.Conn, error)
}

// BuildEngine builds (and lets the caller cache) a urlfilter engine from the
// given filter files, so the expensive list parse happens once, not on every
// proxy restart.
func BuildEngine(paths map[rules.ListID]string) (*urlfilter.Engine, error) {
	return filterproxy.BuildEngine(paths)
}

// Server wraps urlfilter's MITM proxy.
type Server struct {
	inner *filterproxy.Server
}

// NewServer builds a MITM proxy listening on 127.0.0.1:ListenPort. Upstream
// connections use cfg.UpstreamDial when set — required on Android, where this
// app's process is EXCLUDED from its own VPN (BoxPlatform.applyAppRouting adds
// the package to the disallowed list so the engine's proxy dial doesn't
// recurse into the tunnel). A plain net.Dial from here would therefore bypass
// the tunnel entirely and every RKN-blocked site would fail in the browser
// while the VPN is up. The caller aims UpstreamDial at the engine's loopback
// SOCKS inbound so filtered browser traffic re-enters the tunnel. When nil
// (desktop), gomitmproxy's default direct dialer is used.
func NewServer(cfg Config) (*Server, error) {
	if cfg.RootCert == nil || cfg.RootKey == nil {
		return nil, fmt.Errorf("root CA required")
	}
	if len(cfg.FilterPaths) == 0 {
		return nil, fmt.Errorf("filter paths required")
	}
	mitmConfig, err := mitm.NewConfig(cfg.RootCert, cfg.RootKey, nil)
	if err != nil {
		return nil, err
	}
	mitmConfig.SetValidity(7 * 24 * time.Hour)
	mitmConfig.SetOrganization("ResultV")

	addr := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: cfg.ListenPort}

	s := &Server{}
	inner, err := filterproxy.NewServer(filterproxy.Config{
		FiltersPaths:  cfg.FilterPaths,
		Engine:        cfg.Engine,
		InjectionHost: "injections.resultv.local",
		ProxyConfig: gomitmproxy.Config{
			ListenAddr:     addr,
			MITMConfig:     mitmConfig,
			MITMExceptions: defaultMITMExceptions(),
			// Route upstream through the tunnel (see UpstreamDial doc). Nil
			// leaves gomitmproxy's default direct dialer in place.
			Dial: cfg.UpstreamDial,
		},
	})
	if err != nil {
		return nil, err
	}
	s.inner = inner
	return s, nil
}

// defaultMITMExceptions are domains never intercepted, even inside browser
// traffic that otherwise flows through this proxy. youtube.com/googleapis.com
// are here because Google's own network_security_config enforces
// Certificate Transparency + system-only trust anchors (proven empirically —
// see docs/superpowers/specs/2026-07-01-android-mitm-adblock-design.md) —
// attempting MITM on them cannot succeed and would only break sign-in.
// Banking domains are excluded out of caution: breaking a bank's TLS for a
// user who happens to browse it in Chrome has no upside for an ad blocker.
func defaultMITMExceptions() []string {
	return []string{
		"google.com", "googleapis.com", "gstatic.com", "youtube.com", "ytimg.com", "googlevideo.com",
		"apple.com", "icloud.com", "mzstatic.com",
		"microsoft.com", "windows.com", "windowsupdate.com",
		"gosuslugi.ru", "sberbank.ru", "vtb.ru", "tinkoff.ru",
	}
}

func (s *Server) Start() error {
	return s.inner.Start()
}

func (s *Server) Close() {
	if s.inner != nil {
		s.inner.Close()
	}
}
