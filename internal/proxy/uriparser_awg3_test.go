// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/common/json/badoption"
)

// testAWG3Key is a base64-encoded 32-byte key. sing-box-extended decodes
// header_protection_key as base64 and hex-encodes it for the UAPI, exactly
// like private_key and public_key.
const testAWG3Key = "QGg8AFRn6qKfTB7cT3FWH1WGx3np+OKzlNuQUrqIBmI="

// TestAmneziaDropsKeysUnsupportedByEngine guards a hard failure mode: the
// wireguard-go fork behind sing-box-extended has no j1/j2/j3/itime device
// keys in its UAPI (device/uapi.go ends at i5, and the default branch returns
// "invalid UAPI device key"). sing-box-extended nonetheless writes them into
// ipcConf when present, so a subscription carrying any of them makes IpcSet
// fail and the endpoint never comes up. We must never emit them.
func TestAmneziaDropsKeysUnsupportedByEngine(t *testing.T) {
	q := url.Values{}
	q.Set("address", "10.0.0.2/32")
	q.Set("private_key", "PRIV")
	q.Set("public_key", "PUB")
	q.Set("allowed_ips", "0.0.0.0/0")
	q.Set("jc", "5")
	q.Set("i1", "<b 0x01>")
	q.Set("i5", "<r 32>")
	// Unsupported by the engine — must be dropped.
	q.Set("j1", "<b 0xAA>")
	q.Set("j2", "<r 16>")
	q.Set("j3", "<c>")
	q.Set("itime", "300")

	entry, err := ParseProxyURI("awg://1.2.3.4:51820?" + q.Encode() + "#test")
	if err != nil {
		t.Fatalf("ParseProxyURI: %v", err)
	}

	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy: ProxyConfig{IP: entry.IP, Port: entry.Port, Type: entry.Type, Extra: entry.Extra},
		Mode:  ProxyModeTunnel,
	})
	if len(cfg.Endpoints) != 1 || cfg.Endpoints[0].Amnezia == nil {
		t.Fatalf("expected endpoint with amnezia, got %+v", cfg.Endpoints)
	}

	blob, err := json.Marshal(cfg.Endpoints[0])
	if err != nil {
		t.Fatal(err)
	}
	js := string(blob)
	for _, banned := range []string{`"j1"`, `"j2"`, `"j3"`, `"itime"`} {
		if strings.Contains(js, banned) {
			t.Fatalf("endpoint JSON still carries engine-unsupported key %s: %s", banned, js)
		}
	}
	// The supported obfuscation knobs must survive untouched.
	if a := cfg.Endpoints[0].Amnezia; a.JC != 5 || a.I1 != "<b 0x01>" || a.I5 != "<r 32>" {
		t.Fatalf("supported keys damaged: %+v", a)
	}
}

// TestAWG3KnobsReachEngine covers AmneziaWG 3.0 support end to end: our
// config must carry the knobs as first-class fields of the amnezia block and
// survive the upstream option parser, which is where a wrong key name or
// encoding would surface as a hard engine-start failure.
func TestAWG3KnobsReachEngine(t *testing.T) {
	extra := `{
		"private_key": "aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=",
		"public_key": "WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=",
		"address": ["10.8.1.2/24"],
		"allowed_ips": ["0.0.0.0/0"],
		"amnezia": {
			"jc": 8, "jmin": 64, "jmax": 900,
			"s1": 88, "s2": 155, "s3": 32, "s4": 16,
			"header_protection_key": "` + testAWG3Key + `",
			"content_padding_addition": "16-64",
			"rekey_after_time": "100-140",
			"rekey_timeout": "5-8",
			"reject_after_time": "160-200",
			"keepalive_timeout": "10-14",
			"max_handshake_attempts": "18-22"
		}
	}`

	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy: ProxyConfig{IP: "1.2.3.4", Port: 51820, Type: "AMNEZIAWG", Extra: json.RawMessage(extra)},
		Mode:  ProxyModeTunnel,
	})

	full, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	for _, want := range []string{
		`"header_protection_key":"` + testAWG3Key + `"`,
		`"content_padding_addition":"16-64"`,
		`"rekey_after_time":"100-140"`,
		`"max_handshake_attempts":"18-22"`,
	} {
		if !strings.Contains(string(full), want) {
			t.Fatalf("config JSON is missing %s: %s", want, string(full))
		}
	}

	// The upstream parse is the real gate: header_protection_key must decode
	// as base64 and every timing must land in a *badoption.Range.
	ctx := include.Context(context.Background())
	var opts option.Options
	if err := singjson.UnmarshalContext(ctx, full, &opts); err != nil {
		t.Fatalf("upstream sing-box-extended rejected AWG3 config: %v\nJSON: %s", err, string(full))
	}
	wgEp, ok := opts.Endpoints[0].Options.(*option.WireGuardEndpointOptions)
	if !ok {
		t.Fatalf("endpoint options type: %T", opts.Endpoints[0].Options)
	}
	am := wgEp.Amnezia
	if am == nil {
		t.Fatal("upstream Amnezia is nil after parse")
	}
	if am.HeaderProtectionKey != testAWG3Key {
		t.Fatalf("header_protection_key = %q", am.HeaderProtectionKey)
	}
	for _, tc := range []struct {
		name     string
		got      *badoption.Range[uint32]
		from, to uint32
	}{
		{"content_padding_addition", am.ContentPaddingAddition, 16, 64},
		{"rekey_after_time", am.RekeyAfterTime, 100, 140},
		{"rekey_timeout", am.RekeyTimeout, 5, 8},
		{"reject_after_time", am.RejectAfterTime, 160, 200},
		{"keepalive_timeout", am.KeepaliveTimeout, 10, 14},
		{"max_handshake_attempts", am.MaxHandshakeAttempts, 18, 22},
	} {
		if tc.got == nil {
			t.Fatalf("%s stayed nil after upstream parse", tc.name)
		}
		if tc.got.From != tc.from || tc.got.To != tc.to {
			t.Fatalf("%s parsed as %v, want %d-%d", tc.name, tc.got, tc.from, tc.to)
		}
	}
}

// TestAWG3LeavesSignaturePacketsAlone makes sure adding the AWG 3.0 knobs
// never disturbs the i1-i5 slots the provider configured.
func TestAWG3LeavesSignaturePacketsAlone(t *testing.T) {
	extra := `{
		"private_key": "PRIV", "public_key": "PUB",
		"address": ["10.0.0.2/32"], "allowed_ips": ["0.0.0.0/0"],
		"amnezia": {
			"s1": 12, "s2": 12, "s3": 12, "s4": 12,
			"i5": "<b 0xc0ffee><r 16>",
			"header_protection_key": "` + testAWG3Key + `"
		}
	}`

	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy: ProxyConfig{IP: "1.2.3.4", Port: 51820, Type: "AMNEZIAWG", Extra: json.RawMessage(extra)},
		Mode:  ProxyModeTunnel,
	})

	am := cfg.Endpoints[0].Amnezia
	if am.I5 != "<b 0xc0ffee><r 16>" {
		t.Fatalf("i5 was modified: %q", am.I5)
	}
	if am.HeaderProtectionKey != testAWG3Key {
		t.Fatalf("header_protection_key = %q", am.HeaderProtectionKey)
	}
}

// TestAWG3FromURIConfDotStyleKeys covers awg:// links generated from an
// amneziawg .conf, where the knobs keep their CamelCase spelling. The plain
// case-insensitive lookup does not fold separators, so "HeaderProtectionKey"
// has to resolve to "header_protection_key" as well.
func TestAWG3FromURIConfDotStyleKeys(t *testing.T) {
	q := url.Values{}
	q.Set("address", "10.0.0.2/32")
	q.Set("private_key", "PRIV")
	q.Set("public_key", "PUB")
	q.Set("allowed_ips", "0.0.0.0/0")
	q.Set("S1", "16")
	q.Set("S2", "16")
	q.Set("S3", "16")
	q.Set("S4", "16")
	q.Set("HeaderProtectionKey", testAWG3Key)
	q.Set("RekeyAfterTime", "100-140")
	q.Set("max_handshake_attempts", "20")

	entry, err := ParseProxyURI("awg://1.2.3.4:51820?" + q.Encode() + "#t")
	if err != nil {
		t.Fatal(err)
	}
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy: ProxyConfig{IP: entry.IP, Port: entry.Port, Type: entry.Type, Extra: entry.Extra},
		Mode:  ProxyModeTunnel,
	})

	am := cfg.Endpoints[0].Amnezia
	if am.HeaderProtectionKey != testAWG3Key {
		t.Fatalf("header_protection_key = %q", am.HeaderProtectionKey)
	}
	if am.RekeyAfterTime != "100-140" {
		t.Fatalf("rekey_after_time = %q", am.RekeyAfterTime)
	}
	if am.MaxHandshakeAttempts != "20" {
		t.Fatalf("max_handshake_attempts = %q", am.MaxHandshakeAttempts)
	}
	if am.RekeyTimeout != "" || am.KeepaliveTimeout != "" {
		t.Fatalf("unset knobs must stay empty: %+v", am)
	}
}

// TestAWG3AbsentLeavesAmneziaClean guards against emitting empty AWG 3.0
// fields for plain AWG 1.0/2.0 configs.
func TestAWG3AbsentLeavesAmneziaClean(t *testing.T) {
	q := url.Values{}
	q.Set("address", "10.0.0.2/32")
	q.Set("private_key", "PRIV")
	q.Set("public_key", "PUB")
	q.Set("allowed_ips", "0.0.0.0/0")
	q.Set("jc", "4")

	entry, err := ParseProxyURI("awg://1.2.3.4:51820?" + q.Encode() + "#t")
	if err != nil {
		t.Fatal(err)
	}
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy: ProxyConfig{IP: entry.IP, Port: entry.Port, Type: entry.Type, Extra: entry.Extra},
		Mode:  ProxyModeTunnel,
	})
	if got := cfg.Endpoints[0].Amnezia.I5; got != "" {
		t.Fatalf("i5 must stay empty without AWG3 knobs, got %q", got)
	}
}
