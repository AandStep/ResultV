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

	"resultproxy-wails/internal/config"
)

// The handshake probe builds its UAPI string itself instead of going through
// sing-box's option struct, so it is the one path that can speak full
// AmneziaWG 3.0. That matters: a server configured with header protection
// answers nothing unless the probe encrypts its headers the same way, so
// without these lines ping against an AWG 3.0 server reads as "unreachable".
func TestWriteAmneziaUAPIEmitsAWG3Knobs(t *testing.T) {
	var b strings.Builder
	writeAmneziaUAPI(&b, map[string]any{
		"amnezia": map[string]any{
			"jc": 8, "s1": 15, "s2": 15, "s3": 15, "s4": 15,
			"h1":                       "1-5",
			"content_padding_addition": "10-50",
			"rekey_after_time":         float64(120),
			"rekey_timeout":            "5",
			"reject_after_time":        "180",
			"keepalive_timeout":        "10",
			"max_handshake_attempts":   "18",
		},
	})
	got := b.String()

	for _, want := range []string{
		"jc=8\n", "s1=15\n", "h1=1-5\n",
		"content_padding_addition=10-50\n",
		"rekey_after_time=120\n",
		"rekey_timeout=5\n",
		"reject_after_time=180\n",
		"keepalive_timeout=10\n",
		"max_handshake_attempts=18\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// Unset knobs must stay out of the string entirely — the fork treats a missing
// key as "use the WireGuard default", which is not the same as a zero range.
func TestWriteAmneziaUAPIOmitsUnsetAWG3Knobs(t *testing.T) {
	var b strings.Builder
	writeAmneziaUAPI(&b, map[string]any{
		"amnezia": map[string]any{"jc": 8, "rekey_timeout": 0, "keepalive_timeout": ""},
	})
	got := b.String()

	for _, unwanted := range []string{"rekey_timeout", "keepalive_timeout", "header_protection_key"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("emitted unset knob %q in:\n%s", unwanted, got)
		}
	}
}

// HeaderCipherKey.FromHex only takes hex, but `awg genkey` prints base64 like
// every other WireGuard key, so both spellings have to reach the UAPI as hex.
func TestAmneziaKeyHexAcceptsBase64AndHex(t *testing.T) {
	const wantHex = "6805c549c1c0e6d000f66530a756810d5f5c01b1e3d6649fc1d7352124ec666f"
	const asBase64 = "aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsZm8="

	if got := amneziaKeyHex(asBase64); got != wantHex {
		t.Errorf("base64 key: got %q, want %q", got, wantHex)
	}
	if got := amneziaKeyHex(wantHex); got != wantHex {
		t.Errorf("hex key: got %q, want %q", got, wantHex)
	}
	if got := amneziaKeyHex(strings.ToUpper(wantHex)); got != wantHex {
		t.Errorf("uppercase hex key: got %q, want %q", got, wantHex)
	}
	if got := amneziaKeyHex(""); got != "" {
		t.Errorf("empty key: got %q, want empty", got)
	}
	if got := amneziaKeyHex("not a key"); got != "" {
		t.Errorf("garbage key: got %q, want empty", got)
	}
}

func TestWriteAmneziaUAPIEmitsHeaderProtectionKeyAsHex(t *testing.T) {
	var b strings.Builder
	writeAmneziaUAPI(&b, map[string]any{
		"amnezia": map[string]any{
			// S1-S4 must clear the nonce size or the key is deliberately
			// withheld — see headerProtectionUsable.
			"s1": 15, "s2": 15, "s3": 15, "s4": 15,
			"header_protection_key": "aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsZm8=",
		},
	})
	want := "header_protection_key=6805c549c1c0e6d000f66530a756810d5f5c01b1e3d6649fc1d7352124ec666f\n"
	if got := b.String(); !strings.Contains(got, want) {
		t.Errorf("missing %q in:\n%s", want, got)
	}
}

// Device-level keys must all precede the peer block; the UAPI parser switches
// to peer mode at the first public_key= line and rejects device keys after it.
func TestBuildWGUAPIKeepsAWG3KnobsInTheDeviceBlock(t *testing.T) {
	extra := `{
		"private_key":"` + b64key(0x01) + `",
		"public_key":"` + b64key(0x02) + `",
		"amnezia":{
			"jc":8,"s1":15,"s2":15,"s3":15,"s4":15,
			"header_protection_key":"` + b64key(0x03) + `",
			"content_padding_addition":"10-50",
			"rekey_after_time":120,"rekey_timeout":5,
			"reject_after_time":180,"keepalive_timeout":10,
			"max_handshake_attempts":18
		}
	}`
	entry := config.ProxyEntry{
		IP:    "203.0.113.7",
		Port:  51820,
		Type:  "AMNEZIAWG",
		Extra: []byte(extra),
	}

	uapi, _, err := buildWGUAPI(entry)
	if err != nil {
		t.Fatalf("buildWGUAPI: %v", err)
	}
	peerAt := strings.Index(uapi, "public_key=")
	if peerAt < 0 {
		t.Fatalf("no peer block in:\n%s", uapi)
	}
	for _, key := range awg3DeviceKnobs {
		at := strings.Index(uapi, key+"=")
		if at < 0 {
			t.Errorf("%s never reached the UAPI string:\n%s", key, uapi)
			continue
		}
		if at > peerAt {
			t.Errorf("%s appears after the peer block — IpcSet would reject it:\n%s", key, uapi)
		}
	}
}
