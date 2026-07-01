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
	filterproxy "github.com/AdguardTeam/urlfilter/proxy"
	"github.com/AdguardTeam/urlfilter/rules"
)

// Config configures the local MITM filtering proxy.
type Config struct {
	ListenPort  int
	RootCert    *x509.Certificate
	RootKey     *rsa.PrivateKey
	FilterPaths map[rules.ListID]string
	OnBlocked   func(cosmetic bool)
}

// Server wraps urlfilter's MITM proxy.
type Server struct {
	inner *filterproxy.Server
}

// NewServer builds a MITM proxy listening on 127.0.0.1:ListenPort. Traffic
// exits directly to the internet from this process — Android's own
// per-UID VPN routing (the TUN this app already owns) still applies to
// this app's own outbound sockets exactly like any other app on the
// device, so packets naturally continue through the existing tunnel.
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
		InjectionHost: "injections.resultv.local",
		ProxyConfig: gomitmproxy.Config{
			ListenAddr:     addr,
			MITMConfig:     mitmConfig,
			MITMExceptions: defaultMITMExceptions(),
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
		"google.com", "googleapis.com", "gstatic.com", "youtube.com", "ytimg.com",
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
