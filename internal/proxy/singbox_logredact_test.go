// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"strings"
	"testing"

	sblog "github.com/sagernet/sing-box/log"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/logger"
)

// TestEngineLogRedactsWireGuardKeys covers the sing-box-extended failure path
// in transport/wireguard/endpoint.go, which reports IpcSet errors as
// "setup wireguard: \n" + the entire ipcConf. That dump carries private_key
// and, now that the AWG 3.0 knobs are wired through, header_protection_key in
// the clear.
func TestEngineLogRedactsWireGuardKeys(t *testing.T) {
	log := logger.New()
	w := newSingBoxLogWriter(log, ProxyConfig{})

	const priv = "e8bd3f19a0c74d2b5f6a1c8e93b47d05a2f6c1e84b9d70f3a5c2e6b18d4f90a7"
	const hdr = "7b1e4c9a2f6d80b3e5a7c14f9d2b6e08a3f5c7d19b4e60a2c8f3d5b7e9a1c04f"
	msg := "setup wireguard: \nprivate_key=" + priv +
		"\nlisten_port=51820\nheader_protection_key=" + hdr +
		"\njc=8\npublic_key=deadbeef"

	w.WriteMessage(sblog.LevelError, msg)

	entries := log.GetAll()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	got := entries[0].Msg

	if strings.Contains(got, priv) {
		t.Fatalf("private_key value leaked into log: %s", got)
	}
	if strings.Contains(got, hdr) {
		t.Fatalf("header_protection_key value leaked into log: %s", got)
	}
	// Non-secret context must survive so the message stays diagnosable.
	if !strings.Contains(got, "setup wireguard") {
		t.Fatalf("lost error context: %s", got)
	}
	if !strings.Contains(got, "listen_port=51820") {
		t.Fatalf("redaction ate a non-secret field: %s", got)
	}
	if !strings.Contains(got, "jc=8") {
		t.Fatalf("redaction ate a non-secret field: %s", got)
	}
	// public_key is not a secret, but preshared_key is; both end in "_key".
	if !strings.Contains(got, "public_key=deadbeef") {
		t.Fatalf("public_key must stay visible: %s", got)
	}
}

func TestRedactEngineSecretsCoversEverySecretKey(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"private", "private_key=AAAA", "private_key=<скрыто>"},
		{"preshared", "preshared_key=BBBB", "preshared_key=<скрыто>"},
		{"pre_shared", "pre_shared_key=CCCC", "pre_shared_key=<скрыто>"},
		{"header_protection", "header_protection_key=DDDD", "header_protection_key=<скрыто>"},
		{"public_kept", "public_key=EEEE", "public_key=EEEE"},
		{"midline", "setup: private_key=FFFF trailing", "setup: private_key=<скрыто> trailing"},
		{"no_secret", "jc=8", "jc=8"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := redactEngineSecrets(tc.in); got != tc.want {
				t.Fatalf("redactEngineSecrets(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// A provider's backend address is not the user's to leak. sing-box narrates it
// constantly — "lookup <domain>", "open connection ... using outbound" — so the
// writer masks it for subscription servers.
func TestEngineLogRedactsSubscriptionServerAddress(t *testing.T) {
	log := logger.New()
	w := newSingBoxLogWriter(log, ProxyConfig{
		IP:              "backend-07.provider.example",
		Port:            443,
		SubscriptionURL: "https://provider.example/sub",
	})

	w.WriteMessage(sblog.LevelError, "dns: lookup backend-07.provider.example: no such host")

	entries := log.GetAll()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	got := entries[0].Msg

	if strings.Contains(got, "backend-07.provider.example") {
		t.Fatalf("provider backend address leaked: %s", got)
	}
	if !strings.Contains(got, "<сервер>") {
		t.Errorf("address should be replaced, not deleted: %s", got)
	}
	// The rest of the message has to survive or the log stops being useful.
	if !strings.Contains(got, "no such host") {
		t.Errorf("lost the diagnosis: %s", got)
	}
}

// A manually added server is the user's own and its address is already on
// screen; masking it would only make their own logs harder to read.
func TestEngineLogKeepsManualServerAddress(t *testing.T) {
	log := logger.New()
	w := newSingBoxLogWriter(log, ProxyConfig{IP: "203.0.113.9", Port: 443})

	w.WriteMessage(sblog.LevelError, "dial tcp 203.0.113.9:443: i/o timeout")

	entries := log.GetAll()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if !strings.Contains(entries[0].Msg, "203.0.113.9") {
		t.Errorf("manual server address should stay visible: %s", entries[0].Msg)
	}
}

// Both redactions have to survive each other: the secret masking runs on the
// already-server-masked string, and neither may swallow the other's work.
func TestServerAndSecretRedactionCompose(t *testing.T) {
	log := logger.New()
	w := newSingBoxLogWriter(log, ProxyConfig{
		IP:              "backend-07.provider.example",
		SubscriptionURL: "https://provider.example/sub",
	})

	w.WriteMessage(sblog.LevelError,
		"setup wireguard: \nprivate_key=deadbeefcafe\nendpoint=backend-07.provider.example:51820")

	got := log.GetAll()[0].Msg
	if strings.Contains(got, "deadbeefcafe") {
		t.Errorf("private key leaked: %s", got)
	}
	if strings.Contains(got, "backend-07.provider.example") {
		t.Errorf("server address leaked: %s", got)
	}
}

// Known gap, pinned so it is a decision rather than a surprise: the desktop
// writer also masks the pinned resolved IPs, but this branch has no server-pin
// machinery to supply them, so an address sing-box resolved on its own still
// appears. Closing it means plumbing resolution results into ProxyConfig.
func TestResolvedIPIsNotYetRedacted(t *testing.T) {
	log := logger.New()
	w := newSingBoxLogWriter(log, ProxyConfig{
		IP:              "backend-07.provider.example",
		SubscriptionURL: "https://provider.example/sub",
	})

	w.WriteMessage(sblog.LevelError, "dial tcp 198.51.100.77:443: i/o timeout")

	if !strings.Contains(log.GetAll()[0].Msg, "198.51.100.77") {
		t.Skip("resolved IPs are now redacted — update this test and the comment on newSingBoxLogWriter")
	}
}

// The probe path builds its own UAPI string and hands IpcSet failures back as
// an opaque "probe_error", but any future logging of that string has to be
// safe too — the same private key is in it. Guards the whole-config shape our
// own builder produces, not just upstream's.
func TestRedactionCoversOurOwnUAPIString(t *testing.T) {
	entry := config.ProxyEntry{
		IP: "203.0.113.7", Port: 51820, Type: "AMNEZIAWG",
		Extra: []byte(`{
			"private_key":"` + b64key(0x01) + `",
			"public_key":"` + b64key(0x02) + `",
			"amnezia":{
				"s1":15,"s2":15,"s3":15,"s4":15,
				"header_protection_key":"` + b64key(0x03) + `"
			}
		}`),
	}

	uapi, _, err := buildWGUAPI(entry)
	if err != nil {
		t.Fatalf("buildWGUAPI: %v", err)
	}
	if !strings.Contains(uapi, "private_key=") {
		t.Fatalf("fixture did not produce a private key:\n%s", uapi)
	}

	got := redactEngineSecrets(uapi)
	if strings.Contains(got, hexkey(0x01)) {
		t.Errorf("private_key leaked:\n%s", got)
	}
	if strings.Contains(got, hexkey(0x03)) {
		t.Errorf("header_protection_key leaked:\n%s", got)
	}
	if !strings.Contains(got, "public_key="+hexkey(0x02)) {
		t.Errorf("public_key should stay visible:\n%s", got)
	}
}
