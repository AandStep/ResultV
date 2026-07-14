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
	ListenPort   int
	UpstreamPort int // sing-box mixed inbound; 0 = direct
	RootCert     *x509.Certificate
	RootKey      *rsa.PrivateKey
	FilterPaths  map[rules.ListID]string
	OnBlocked    func(cosmetic bool) // reserved for future metrics hooks
}

// Server wraps urlfilter's MITM proxy.
type Server struct {
	inner *filterproxy.Server
}

// NewServer builds a MITM proxy.
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

	// Plain HTTP proxy only. sing-box http outbound speaks HTTP CONNECT without
	// TLS to the proxy port; TLSConfig here caused immediate "unexpected EOF".
	s := &Server{}

	inner, err := filterproxy.NewServer(filterproxy.Config{
		FiltersPaths: cfg.FilterPaths,
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

func defaultMITMExceptions() []string {
	return []string{
		"apple.com", "icloud.com", "mzstatic.com",
		"microsoft.com", "windows.com", "windowsupdate.com",
		"google.com", "googleapis.com", "gstatic.com",
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
