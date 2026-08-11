// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"encoding/json"
	"strings"
	"testing"
)

func awg3ValidationError(t *testing.T, amnezia string) (string, error) {
	t.Helper()
	extra := `{
		"private_key": "priv", "public_key": "pub",
		"address": ["10.0.0.2/32"], "allowed_ips": ["0.0.0.0/0"],
		"amnezia": ` + amnezia + `
	}`
	return validateEngineConfig(EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{Type: "AMNEZIAWG", IP: "127.0.0.1", Port: 51820, Extra: json.RawMessage(extra)},
	})
}

// TestAWG3HeaderProtectionRequiresPadding mirrors the engine-side rule in
// wireguard-go's ipcSetDevice.mergeWithDevice: with a header protection key
// set, every one of S1-S4 must be at least HeaderCipherNonceSize (12),
// because the crypto padding doubles as the header cipher's nonce. Catching
// it here turns an opaque "setup wireguard" failure into a clear message.
func TestAWG3HeaderProtectionRequiresPadding(t *testing.T) {
	code, err := awg3ValidationError(t, `{
		"s1": 88, "s2": 155, "s3": 8, "s4": 16,
		"header_protection_key": "`+testAWG3Key+`"
	}`)
	if err == nil {
		t.Fatal("expected validation error for S3 below the nonce size")
	}
	if code != ConnectErrorInvalidConfig {
		t.Fatalf("unexpected code: %s", code)
	}
	if !strings.Contains(err.Error(), "s3") {
		t.Fatalf("error should name the offending padding, got: %v", err)
	}
}

func TestAWG3HeaderProtectionAcceptsSufficientPadding(t *testing.T) {
	if _, err := awg3ValidationError(t, `{
		"s1": 12, "s2": 12, "s3": 12, "s4": 12,
		"header_protection_key": "`+testAWG3Key+`"
	}`); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

// Padding below 12 is perfectly legal for AWG 1.0/2.0 — the rule only binds
// when header protection is actually in use.
func TestAWG3PaddingRuleOnlyAppliesWithHeaderProtection(t *testing.T) {
	if _, err := awg3ValidationError(t, `{"s1": 8, "s2": 8, "s3": 8, "s4": 8}`); err != nil {
		t.Fatalf("unexpected validation error without header protection: %v", err)
	}
}

func TestAWG3RejectsMalformedHeaderProtectionKey(t *testing.T) {
	cases := []struct{ name, key string }{
		// Valid base64, but only 16 bytes instead of 32.
		{"too_short", "QGg8AFRn6qKfTB7cT3FWHw=="},
		{"not_base64", strings.Repeat("!", 44)},
		// A 64-hex-character key, which is what AWG .conf files carry — it is
		// not what sing-box-extended expects and must be rejected loudly.
		{"hex_instead_of_base64", strings.Repeat("ab", 32)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := awg3ValidationError(t, `{
				"s1": 12, "s2": 12, "s3": 12, "s4": 12,
				"header_protection_key": "`+tc.key+`"
			}`)
			if err == nil {
				t.Fatalf("expected validation error for key %q", tc.key)
			}
			if !strings.Contains(err.Error(), "header_protection_key") {
				t.Fatalf("error should name the key, got: %v", err)
			}
		})
	}
}

func TestAWG3RejectsMalformedRanges(t *testing.T) {
	cases := []struct{ name, amnezia string }{
		{"inverted", `{"rekey_after_time": "140-100"}`},
		{"not_a_number", `{"rekey_timeout": "5-abc"}`},
		{"empty_bound", `{"keepalive_timeout": "10-"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := awg3ValidationError(t, tc.amnezia)
			if err == nil {
				t.Fatalf("expected validation error for %s", tc.amnezia)
			}
		})
	}
}

func TestAWG3AcceptsSingleValueAndRange(t *testing.T) {
	if _, err := awg3ValidationError(t, `{
		"rekey_after_time": "120",
		"content_padding_addition": "16-64",
		"max_handshake_attempts": "18-22"
	}`); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

// A newline in any AWG 3.0 value would inject an arbitrary extra UAPI line
// into the device config. The values come from remote subscriptions, so the
// injection surface we rely on must stay ours alone.
func TestAWG3RejectsNewlineInValue(t *testing.T) {
	_, err := awg3ValidationError(t, `{"rekey_after_time": "120\nlisten_port=1"}`)
	if err == nil {
		t.Fatal("expected validation error for embedded newline")
	}
}

// I1-I5 are written into ipcConf verbatim as well, which is precisely the
// property appendAWG3Lines exploits. A subscription must not get to use it:
// the only newlines in the amnezia block may be the ones we put there.
func TestAmneziaRejectsNewlineInSignaturePackets(t *testing.T) {
	for _, slot := range []string{"i1", "i3", "i5"} {
		t.Run(slot, func(t *testing.T) {
			_, err := awg3ValidationError(t, `{"`+slot+`": "<b 0x01>\nheader_protection_key=`+testAWG3Key+`"}`)
			if err == nil {
				t.Fatalf("expected validation error for newline in %s", slot)
			}
			if !strings.Contains(err.Error(), slot) {
				t.Fatalf("error should name the offending slot, got: %v", err)
			}
		})
	}
}

func TestAmneziaAcceptsOrdinarySignaturePackets(t *testing.T) {
	if _, err := awg3ValidationError(t, `{"i1": "<b 0xc0ffee><r 16>", "i5": "<t>"}`); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}
