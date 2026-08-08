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
)

const awgTestKey = "AwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwM=" // 32 bytes of 0x03

func awgExtra(am map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"amnezia": am}
}

// The injection this blocks: sing-box writes I1-I5 and the AWG 3.0 knobs into
// ipcConf verbatim, and ipcConf is newline-separated "key=value". A hostile or
// merely broken subscription that slips a line break into one of those slots
// gets to append device keys of its own — including the ones we deliberately
// refuse to emit, like j1, which kills the tunnel outright.
//
// Length clipping does not help here: the payload needs only a few bytes.
func TestAmneziaValidationRejectsLineBreakInjection(t *testing.T) {
	slots := append([]string{"i1", "i2", "i3", "i4", "i5"}, awg3DeviceKnobs...)
	for _, slot := range slots {
		t.Run(slot, func(t *testing.T) {
			err := validateAmneziaOptions(awgExtra(map[string]interface{}{
				slot: "<b 0xf1>\nj1=<b 0xff>",
			}))
			if err == nil {
				t.Fatalf("%s accepted a line break — a subscription could inject device keys", slot)
			}
			if !strings.Contains(err.Error(), slot) {
				t.Errorf("error should name the offending slot, got: %v", err)
			}
		})
	}

	// A carriage return alone is enough on the platforms that accept it.
	if err := validateAmneziaOptions(awgExtra(map[string]interface{}{
		"i3": "<b 0xf1>\rlisten_port=1",
	})); err == nil {
		t.Error("bare carriage return accepted")
	}
}

func TestAmneziaValidationAcceptsCleanConfig(t *testing.T) {
	err := validateAmneziaOptions(awgExtra(map[string]interface{}{
		"jc": 8, "jmin": 64, "jmax": 900,
		"s1": 15, "s2": 15, "s3": 15, "s4": 15,
		"h1": "122163117-125750861",
		"i1": "<b 0xf1>",
		"header_protection_key":    awgTestKey,
		"content_padding_addition": "10-50",
		"rekey_after_time":         120,
		"max_handshake_attempts":   "18",
	}))
	if err != nil {
		t.Fatalf("clean AWG 3.0 config rejected: %v", err)
	}
}

// A plain WireGuard or AWG 2.0 profile must not acquire new ways to fail.
func TestAmneziaValidationIgnoresNonAmneziaConfigs(t *testing.T) {
	if err := validateAmneziaOptions(map[string]interface{}{"private_key": "x"}); err != nil {
		t.Errorf("plain WireGuard rejected: %v", err)
	}
	if err := validateAmneziaOptions(awgExtra(map[string]interface{}{
		"jc": 8, "s1": 88, "h1": "1-5",
	})); err != nil {
		t.Errorf("AWG 2.0 profile rejected: %v", err)
	}
}

func TestAWGHeaderProtectionKeyValidation(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		wantErr string
	}{
		{"valid base64", awgTestKey, ""},
		{"not base64", "not a key!!", "not valid base64"},
		{"too short", "AwMDAwMD", "must decode to 32 bytes"},
		// 64 hex characters are also valid base64 — they decode to 48 bytes.
		// This is the exact shape that used to pass the probe and kill the
		// tunnel, so it has to be caught by size, not by format alone.
		{
			"hex spelling",
			"6805c549c1c0e6d000f66530a756810d5f5c01b1e3d6649fc1d7352124ec666f",
			"must decode to 32 bytes",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAWGHeaderProtectionKey(tc.key)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("got %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

// Header protection reuses S1-S4 as its nonce, so all four must clear the nonce
// size. Failing loudly beats dropping the key: a server that requires header
// protection will not answer an unprotected peer, so a silent drop would only
// convert a nameable error into a handshake timeout.
func TestHeaderProtectionRequiresAllPaddingsAtNonceSize(t *testing.T) {
	base := map[string]interface{}{"header_protection_key": awgTestKey}

	for _, short := range []string{"s1", "s2", "s3", "s4"} {
		t.Run(short, func(t *testing.T) {
			am := map[string]interface{}{}
			for k, v := range base {
				am[k] = v
			}
			for _, s := range []string{"s1", "s2", "s3", "s4"} {
				am[s] = 15
			}
			am[short] = 11

			err := validateAmneziaOptions(awgExtra(am))
			if err == nil {
				t.Fatalf("%s=11 accepted with header protection on", short)
			}
			if !strings.Contains(err.Error(), short) {
				t.Errorf("error should name %s, got: %v", short, err)
			}
		})
	}

	// Exactly at the boundary is fine.
	am := map[string]interface{}{"header_protection_key": awgTestKey}
	for _, s := range []string{"s1", "s2", "s3", "s4"} {
		am[s] = awgHeaderCipherNonceSize
	}
	if err := validateAmneziaOptions(awgExtra(am)); err != nil {
		t.Errorf("S1-S4 exactly at the nonce size rejected: %v", err)
	}
}

func TestAWGRangeValidation(t *testing.T) {
	good := []string{"0", "120", "10-50", "4294967295", "7-7"}
	for _, v := range good {
		if err := validateAWGRange(v); err != nil {
			t.Errorf("validateAWGRange(%q) = %v, want nil", v, err)
		}
	}
	bad := []string{"", "abc", "10-", "-50", "50-10", "1-2-3", "4294967296", "-1"}
	for _, v := range bad {
		if err := validateAWGRange(v); err == nil {
			t.Errorf("validateAWGRange(%q) = nil, want an error", v)
		}
	}
}

// The knob names have to appear in the message: "amneziawg rekey_timeout:
// invalid value" is actionable, "setup wireguard" is not.
func TestAmneziaValidationNamesTheBadKnob(t *testing.T) {
	err := validateAmneziaOptions(awgExtra(map[string]interface{}{
		"rekey_timeout": "50-10",
	}))
	if err == nil {
		t.Fatal("inverted range accepted")
	}
	if !strings.Contains(err.Error(), "rekey_timeout") {
		t.Errorf("error should name the knob, got: %v", err)
	}
}
