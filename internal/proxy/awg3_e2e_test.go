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
	"strings"
	"testing"

	"github.com/sagernet/wireguard-go/conn"
	"github.com/sagernet/wireguard-go/device"

	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"

	"resultproxy-wails/internal/config"
)

// newProbeDevice builds a device the same way realWGHandshakeProbe does, minus
// the network work: no Up(), so nothing is sent anywhere.
func newProbeDevice(t *testing.T) *device.Device {
	t.Helper()
	dev := device.NewDevice(
		context.Background(),
		newStubTUN(),
		conn.NewDefaultBind(nil),
		device.NewLogger(device.LogLevelSilent, ""),
		0, 0, false,
	)
	t.Cleanup(dev.Close)
	return dev
}

// The whole point of the extended-1.5.0 bump: the fork's UAPI now has a case
// for every AmneziaWG 3.0 device key. If any one of them were still missing,
// handleDeviceLine would fall through to `default:` and fail the *entire*
// IpcSet with "invalid UAPI device key" — which is exactly how j1-j3 and itime
// still behave. This test is what tells us the bump actually delivered AWG 3.0
// rather than just moving a version string.
func TestAWG3UAPIIsAcceptedByTheForkedDevice(t *testing.T) {
	// S1-S4 must be >= HeaderCipherNonceSize (12) for header protection;
	// mergeWithDevice rejects the config otherwise.
	extra := `{
		"private_key":"` + b64key(0x01) + `",
		"public_key":"` + b64key(0x02) + `",
		"amnezia":{
			"jc":8,"jmin":64,"jmax":900,
			"s1":15,"s2":15,"s3":15,"s4":15,
			"h1":"122163117-125750861",
			"header_protection_key":"` + b64key(0x03) + `",
			"content_padding_addition":"10-50",
			"rekey_after_time":"110-130",
			"rekey_timeout":5,
			"reject_after_time":180,
			"keepalive_timeout":10,
			"max_handshake_attempts":18
		}
	}`
	entry := config.ProxyEntry{
		IP: "203.0.113.7", Port: 51820, Type: "AMNEZIAWG",
		Extra: []byte(extra),
	}

	uapi, _, err := buildWGUAPI(entry)
	if err != nil {
		t.Fatalf("buildWGUAPI: %v", err)
	}
	if err := newProbeDevice(t).IpcSet(uapi); err != nil {
		t.Fatalf("wireguard-go rejected our AWG 3.0 UAPI: %v\n%s", err, uapi)
	}
}

// The counterpart, and the reason stripUnsupportedAmnezia still exists: the
// fork implements AWG 3.0 but *still* has no case for j1-j3 / itime, so one of
// those keys kills the whole device setup. Verified here rather than assumed,
// because it is the only thing keeping the strip in place.
func TestForkStillRejectsJunkPacketKnobs(t *testing.T) {
	for _, key := range []string{"j1", "j2", "j3", "itime"} {
		t.Run(key, func(t *testing.T) {
			value := "<b 0xf1>"
			if key == "itime" {
				value = "30"
			}
			uapi := "private_key=" + hexkey(0x01) + "\n" + key + "=" + value + "\n"

			err := newProbeDevice(t).IpcSet(uapi)
			if err == nil {
				t.Fatalf("fork now accepts %q — stripUnsupportedAmnezia can be dropped", key)
			}
			if !strings.Contains(err.Error(), "invalid UAPI device key") {
				t.Fatalf("unexpected failure for %q: %v", key, err)
			}
		})
	}
}

// Header protection needs S1-S4 >= 12. Our clamp must not silently produce a
// combination the device refuses, and the failure has to stay legible.
func TestHeaderProtectionRequiresTwelveBytePadding(t *testing.T) {
	uapi := "private_key=" + hexkey(0x01) + "\n" +
		"s1=5\ns2=5\ns3=5\ns4=5\n" +
		"header_protection_key=" + hexkey(0x03) + "\n"

	err := newProbeDevice(t).IpcSet(uapi)
	if err == nil {
		t.Fatal("expected S1-S4 < 12 to be rejected alongside header protection")
	}
	if !strings.Contains(err.Error(), "headerProtection") {
		t.Errorf("unexpected error, wanted the padding constraint: %v", err)
	}
}

// The real proof that AWG 3.0 now works end to end: our marshaled config goes
// through the actual upstream decoder — which runs with DisallowUnknownFields,
// so every key has to match option.WireGuardAmnezia exactly — and the parsed
// options must carry the knobs, not silently ignore them.
func TestAWG3ConfigStillParsesUpstream(t *testing.T) {
	cfg := BuildTunnelModeConfig(EngineConfig{
		Mode: ProxyModeTunnel,
		Proxy: ProxyConfig{
			IP: "example.com", Port: 51820, Type: "AMNEZIAWG",
			Extra: json.RawMessage(`{
				"private_key":"aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=",
				"public_key":"WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=",
				"address":["10.8.1.2/24"],
				"allowed_ips":["0.0.0.0/0"],
				"amnezia":{
					"jc":8,"s1":15,"s2":15,"s3":15,"s4":15,
					"h1":"122163117-125750861",
					"header_protection_key":"AwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwM=",
					"content_padding_addition":"10-50",
					"rekey_after_time":120,"max_handshake_attempts":18
				}
			}`),
		},
	})

	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	ctx := include.Context(context.Background())
	var parsed option.Options
	if err := singjson.UnmarshalContext(ctx, raw, &parsed); err != nil {
		t.Fatalf("upstream rejected our config: %v\n%s", err, raw)
	}
	if len(parsed.Endpoints) != 1 {
		t.Fatalf("endpoints: %d", len(parsed.Endpoints))
	}

	wg, ok := parsed.Endpoints[0].Options.(*option.WireGuardEndpointOptions)
	if !ok {
		t.Fatalf("unexpected endpoint options type %T", parsed.Endpoints[0].Options)
	}
	if wg.Amnezia == nil {
		t.Fatal("amnezia section did not survive the upstream parse")
	}
	if wg.Amnezia.HeaderProtectionKey == "" {
		t.Errorf("header protection key lost upstream: %+v", wg.Amnezia)
	}
	if wg.Amnezia.ContentPaddingAddition == nil {
		t.Fatalf("content_padding_addition lost upstream: %+v", wg.Amnezia)
	}
	if got := wg.Amnezia.ContentPaddingAddition.String(); got != "10-50" {
		t.Errorf("content_padding_addition = %q, want 10-50", got)
	}
	// A bare JSON number must land as a degenerate range, not be dropped.
	if wg.Amnezia.RekeyAfterTime == nil || wg.Amnezia.RekeyAfterTime.String() != "120" {
		t.Errorf("rekey_after_time did not round-trip: %+v", wg.Amnezia.RekeyAfterTime)
	}
	if wg.Amnezia.MaxHandshakeAttempts == nil || wg.Amnezia.MaxHandshakeAttempts.String() != "18" {
		t.Errorf("max_handshake_attempts did not round-trip: %+v", wg.Amnezia.MaxHandshakeAttempts)
	}
}
