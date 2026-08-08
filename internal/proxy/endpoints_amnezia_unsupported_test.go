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

// The pinned wireguard-go fork's UAPI parser has no case for j1/j2/j3/itime
// (verified against extended-1.4.3 and -1.5.0, and by running a real device in
// awg3_e2e_test.go). sing-box-extended 2.6.1 responded by removing the option
// fields entirely, so there is now nowhere to put them on either side — and
// because the root decoder uses DisallowUnknownFields, emitting one would fail
// the *whole* config rather than a single endpoint.
//
// So they must never reach the engine config.
func TestAmneziaDropsKnobsTheCoreCannotParse(t *testing.T) {
	extra := map[string]interface{}{
		"amnezia": map[string]interface{}{
			"jc": 8, "jmin": 64, "jmax": 900,
			"s1": 88, "s2": 155, "s3": 32, "s4": 16,
			"h1": "1-5", "i1": "<b 0xf1>",
			"j1": "<b 0xf1>", "j2": "<b 0xf2>", "j3": "<b 0xf3>",
			"itime": 30,
		},
	}

	am := amneziaFromExtra(extra)
	if am == nil {
		t.Fatal("amnezia block went missing entirely")
	}
	amJSON, err := json.Marshal(am)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"j1"`, `"j2"`, `"j3"`, `"itime"`} {
		if strings.Contains(string(amJSON), key) {
			t.Errorf("unsupported knob %s survived: %s", key, amJSON)
		}
	}
	// Everything the core *does* understand must be untouched.
	if am.JC != 8 || am.S1 != 88 || am.S4 != 16 || am.H1 != "1-5" || am.I1 != "<b 0xf1>" {
		t.Errorf("supported knobs damaged: %+v", am)
	}
}

func TestUnsupportedAmneziaKnobsReportsWhatWasDropped(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]interface{}
		want []string
	}{
		{"none", map[string]interface{}{"jc": 8, "h1": "1-5"}, nil},
		{"j1 only", map[string]interface{}{"j1": "<b 0x1>"}, []string{"j1"}},
		{"itime only", map[string]interface{}{"itime": 30}, []string{"itime"}},
		{
			"all four",
			map[string]interface{}{"j1": "a", "j2": "b", "j3": "c", "itime": 5},
			[]string{"j1", "j2", "j3", "itime"},
		},
		// A zero itime is simply absent, not "dropped".
		{"itime zero", map[string]interface{}{"itime": 0}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := unsupportedAmneziaKnobs(map[string]interface{}{"amnezia": tc.in})
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// A config carrying *only* unsupported knobs must come out as plain WireGuard,
// not as an empty "amnezia":{} block.
func TestAmneziaOnlyUnsupportedKnobsYieldsNoAmneziaBlock(t *testing.T) {
	extra := map[string]interface{}{
		"amnezia": map[string]interface{}{"j1": "<b 0xf1>", "itime": 30},
	}
	if am := amneziaFromExtra(extra); am != nil {
		t.Errorf("expected no amnezia block, got %+v", am)
	}
}

// End-to-end: the marshaled engine config must not contain the keys, because
// that JSON is exactly what sing-box turns into the IPC string.
func TestBuiltConfigNeverCarriesUnsupportedAmneziaKnobs(t *testing.T) {
	cfg := BuildTunnelModeConfig(EngineConfig{
		Mode: ProxyModeTunnel,
		Proxy: ProxyConfig{
			IP: "example.com", Port: 51820, Type: "AMNEZIAWG",
			Extra: json.RawMessage(`{
				"private_key":"aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=",
				"public_key":"WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=",
				"amnezia":{"jc":8,"h1":"1-5","j1":"<b 0xf1>","j2":"x","j3":"y","itime":30}
			}`),
		},
	})
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{`"j1"`, `"j2"`, `"j3"`, `"itime"`} {
		if strings.Contains(string(raw), key) {
			t.Errorf("config still carries %s — IpcSet would fail:\n%s", key, raw)
		}
	}
	if !strings.Contains(string(raw), `"jc"`) {
		t.Error("supported knob jc was lost")
	}
}
