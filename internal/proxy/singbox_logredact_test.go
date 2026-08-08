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
	w := &singBoxLogWriter{log: log}

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
