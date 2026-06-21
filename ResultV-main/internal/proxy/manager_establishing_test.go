// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"testing"

	"resultproxy-wails/internal/logger"
)

func TestGetStatus_Establishing(t *testing.T) {
	eng := &stubEngine{running: true}
	m := &Manager{
		log:    logger.New(),
		engine: eng,
	}
	p := ProxyConfig{ID: "1", IP: "1.2.3.4", Port: 443, Type: "VLESS"}
	m.setPendingLocked(p, ProxyModeTunnel)

	status := m.GetStatus()
	if !status.IsEstablishing {
		t.Fatalf("expected IsEstablishing=true, got %+v", status)
	}
	if status.IsConnected {
		t.Fatalf("expected IsConnected=false during establishing")
	}
	if status.CurrentProxy == nil || status.CurrentProxy.IP != "1.2.3.4" {
		t.Fatalf("expected pending proxy in status, got %+v", status.CurrentProxy)
	}
	if status.Mode != ProxyModeTunnel {
		t.Fatalf("expected pending mode tunnel, got %q", status.Mode)
	}
}

func TestGetStatus_NotEstablishingWhenConnected(t *testing.T) {
	eng := &stubEngine{running: true}
	m := &Manager{
		log:       logger.New(),
		engine:    eng,
		connected: true,
		proxy:     &ProxyConfig{ID: "1", IP: "1.2.3.4", Port: 443, Type: "VLESS"},
		mode:      ProxyModeProxy,
	}
	status := m.GetStatus()
	if status.IsEstablishing {
		t.Fatalf("expected IsEstablishing=false when connected")
	}
	if !status.IsConnected {
		t.Fatalf("expected IsConnected=true")
	}
}
