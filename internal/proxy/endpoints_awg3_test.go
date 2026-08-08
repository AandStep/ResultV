// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Since sing-box-extended v1.13.16-extended-2.6.1 the AmneziaWG 3.0 knobs are
// carried by option.WireGuardAmnezia and written to the IPC string, so they are
// no longer something we drop and warn about. Only the junk-packet knobs are.
func TestAWG3KnobsAreNoLongerReportedAsUnsupported(t *testing.T) {
	extra := map[string]interface{}{
		"amnezia": map[string]interface{}{
			"jc": 8, "s1": 15, "s2": 15, "s3": 15, "s4": 15,
			"header_protection_key":    "AwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwM=",
			"content_padding_addition": "10-50",
			"rekey_after_time":         120,
			"max_handshake_attempts":   "18",
			// Still unsupported, and still the only thing worth warning about.
			"j1": "<b 0xf1>", "itime": 30,
		},
	}
	got := unsupportedAmneziaKnobs(extra)
	want := []string{"j1", "itime"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
}

// A zero range means "unset" to AmneziaWG and must stay out of the config —
// upstream would otherwise emit "rekey_timeout=0" into the IPC string.
func TestAWG3ZeroKnobsAreDropped(t *testing.T) {
	am := amneziaFromExtra(map[string]interface{}{
		"amnezia": map[string]interface{}{
			"jc": 8, "rekey_timeout": 0, "keepalive_timeout": "",
		},
	})
	if am == nil {
		t.Fatal("amnezia block went missing entirely")
	}
	if am.RekeyTimeout != "" || am.KeepaliveTimeout != "" {
		t.Errorf("zero knobs should be dropped: %+v", am)
	}
}

// The whole point of 2.6.1: these now reach the engine config.
func TestBuiltConfigCarriesAWG3Knobs(t *testing.T) {
	cfg := BuildTunnelModeConfig(EngineConfig{
		Mode: ProxyModeTunnel,
		Proxy: ProxyConfig{
			IP: "example.com", Port: 51820, Type: "AMNEZIAWG",
			Extra: json.RawMessage(`{
				"private_key":"aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=",
				"public_key":"WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=",
				"amnezia":{
					"jc":8,"s1":15,"s2":15,"s3":15,"s4":15,
					"header_protection_key":"AwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwM=",
					"content_padding_addition":"10-50",
					"rekey_after_time":120,"rekey_timeout":5,
					"reject_after_time":180,"keepalive_timeout":10,
					"max_handshake_attempts":18
				}
			}`),
		},
	})
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range awg3DeviceKnobs {
		if !strings.Contains(string(raw), `"`+key+`"`) {
			t.Errorf("config lost %q — the tunnel would fall back to AWG 2.0:\n%s", key, raw)
		}
	}
	// The key must stay base64: the core base64-decodes it before hex-encoding
	// it into the IPC string, so handing it hex would corrupt the key.
	if !strings.Contains(string(raw), `"header_protection_key":"AwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwM="`) {
		t.Errorf("header protection key was not passed through as base64:\n%s", raw)
	}
	// Ranges must survive as strings badoption.Range can parse.
	if !strings.Contains(string(raw), `"content_padding_addition":"10-50"`) {
		t.Errorf("range knob mangled:\n%s", raw)
	}
	if !strings.Contains(string(raw), `"jc"`) || !strings.Contains(string(raw), `"s1"`) {
		t.Errorf("supported AWG 2.0 knobs were lost:\n%s", raw)
	}
}

// The config path no longer second-guesses the key: validateAmneziaOptions
// refuses the bad combination by name before the engine starts, so quietly
// dropping it here would only turn a legible error into a handshake timeout.
// The rejection itself is covered by TestHeaderProtectionRequiresAllPaddings-
// AtNonceSize; this pins the other half — that nothing swallows it silently.
func TestConfigPathDoesNotSilentlyDropHeaderProtectionKey(t *testing.T) {
	extra := map[string]interface{}{
		"amnezia": map[string]interface{}{
			// Deliberately unusable: s3 is below the nonce size.
			"s1": 15, "s2": 15, "s3": 11, "s4": 15,
			"header_protection_key": awgTestKey,
		},
	}

	am := amneziaFromExtra(extra)
	if am == nil {
		t.Fatal("amnezia block went missing entirely")
	}
	if am.HeaderProtectionKey != awgTestKey {
		t.Errorf("key was dropped instead of being rejected upstream: %+v", am)
	}
	// And it must not be reported as a "dropped knob" — it is an error now.
	if slices.Contains(unsupportedAmneziaKnobs(extra), "header_protection_key") {
		t.Error("an unusable key should be a hard error, not a dropped-knob warning")
	}
	// The guard that actually stops it:
	if err := validateAmneziaOptions(extra); err == nil {
		t.Error("validation accepted a key with s3 below the nonce size")
	}
}

// The probe builds its own UAPI, and must apply the same rule — otherwise the
// device refuses to start and every ping reads as probe_error.
func TestProbeOmitsUnusableHeaderProtectionKey(t *testing.T) {
	var b strings.Builder
	writeAmneziaUAPI(&b, map[string]any{
		"amnezia": map[string]any{
			"s1": 15, "s2": 15, "s3": 11, "s4": 15,
			"header_protection_key": "AwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwM=",
		},
	})
	if got := b.String(); strings.Contains(got, "header_protection_key") {
		t.Errorf("probe would send an unusable key:\n%s", got)
	}
}

// extended-1.5.0 parses s1-s4 as uint16 and jc/jmin/jmax as uint32. A value
// past the ceiling fails the entire IpcSet, so it has to be clamped rather
// than forwarded.
func TestNormalizeAmneziaClampsPaddingToUint16(t *testing.T) {
	am := &SBWireGuardAmnezia{S1: 100000, S2: -1, S3: 65535, S4: 12}
	normalizeAmnezia(am)

	if am.S1 != maxAmneziaPadding {
		t.Errorf("S1 = %d, want clamp to %d", am.S1, maxAmneziaPadding)
	}
	if am.S2 != 0 {
		t.Errorf("S2 = %d, want 0", am.S2)
	}
	// Values already in range must be left exactly as they are — S4=12 is the
	// minimum header protection needs, and rounding it would break AWG 3.0.
	if am.S3 != 65535 || am.S4 != 12 {
		t.Errorf("in-range paddings damaged: S3=%d S4=%d", am.S3, am.S4)
	}
}

// The junk ceiling is uint32, which only exceeds int on 64-bit builds. On
// armeabi-v7a every non-negative int is already inside the range, so there is
// nothing to clamp and the constant itself would not fit in an int literal.
func TestNormalizeAmneziaClampsJunkToUint32(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("int is 32-bit here; every non-negative value is within uint32")
	}
	overCeiling := int(int64(maxAmneziaJunk) + 1)

	am := &SBWireGuardAmnezia{JC: overCeiling, JMin: -5, JMax: overCeiling}
	normalizeAmnezia(am)

	if int64(am.JC) != maxAmneziaJunk {
		t.Errorf("JC = %d, want clamp to %d", am.JC, int64(maxAmneziaJunk))
	}
	if am.JMin != 0 {
		t.Errorf("JMin = %d, want 0", am.JMin)
	}
	if int64(am.JMax) != maxAmneziaJunk {
		t.Errorf("JMax = %d, want clamp to %d", am.JMax, int64(maxAmneziaJunk))
	}
}
