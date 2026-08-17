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

// Knob values taken verbatim from a live AmneziaWG 2.0 provider config: H1-H4
// as wide ranges and an I1 signature packet that spoofs a DNS response for
// yandex.ru. Hand-written fixtures tend to be tidier than anything real, which
// is how a shape like the 103-character I1 below goes untested.
//
// The keys are dummies. The real config carried live credentials, and IpcSet
// does not care which valid 32-byte key it gets.
const realWorldAWG2Knobs = `"jc":8,"jmin":64,"jmax":900,
	"s1":88,"s2":155,"s3":32,"s4":16,
	"h1":"111326662-114201317","h2":"874613113-878446789",
	"h3":"1231205899-1232132550","h4":"2048379358-2048756464",
	"i1":"<r 2><b 0x8580000100010000000004796162730679616e6465780272750000010001c00c000100010000026d000457fa27d1>"`

func realWorldAWGEntry(t *testing.T, extraKnobs string) config.ProxyEntry {
	t.Helper()
	knobs := realWorldAWG2Knobs
	if extraKnobs != "" {
		knobs += "," + extraKnobs
	}
	return config.ProxyEntry{
		IP: "203.0.113.7", Port: 10700, Type: "AMNEZIAWG",
		Extra: []byte(`{"private_key":"` + b64key(0x01) + `",
			"public_key":"` + b64key(0x02) + `",
			"pre_shared_key":"` + b64key(0x04) + `",
			"amnezia":{` + knobs + `}}`),
	}
}

// The whole pipeline against a real device: parse → UAPI → IpcSet. Guards the
// AWG 2.0 shapes our own fixtures under-represent.
func TestRealWorldAWG2ConfigIsAcceptedByTheDevice(t *testing.T) {
	entry := realWorldAWGEntry(t, "")

	uapi, _, err := buildWGUAPI(entry)
	if err != nil {
		t.Fatalf("buildWGUAPI: %v", err)
	}
	// The header ranges must reach the device verbatim; collapsing one to a
	// single value would silently change which packets the peer accepts.
	if !strings.Contains(uapi, "h1=111326662-114201317") {
		t.Errorf("header range mangled:\n%s", uapi)
	}
	if err := newProbeDevice(t).IpcSet(uapi); err != nil {
		t.Fatalf("device rejected a real provider config: %v\n%s", err, uapi)
	}
}

// The five AmneziaWG 3.0 timing knobs are documented client-side — "not
// required to be the same on both server and client" — so they can be used
// against any AmneziaWG server, including a 2.0 one. That is what makes them
// testable without an AWG 3.0 peer, and it is worth pinning: if a future fork
// makes any of them server-side, this stops being true and the test says so.
func TestAWG3TimingsWorkOnTopOfAnAWG2Config(t *testing.T) {
	entry := realWorldAWGEntry(t, `"rekey_after_time":"110-130","rekey_timeout":5,
		"reject_after_time":180,"keepalive_timeout":10,"max_handshake_attempts":18`)

	uapi, _, err := buildWGUAPI(entry)
	if err != nil {
		t.Fatalf("buildWGUAPI: %v", err)
	}
	if err := newProbeDevice(t).IpcSet(uapi); err != nil {
		t.Fatalf("device rejected AWG 3.0 timings on an AWG 2.0 config: %v\n%s", err, uapi)
	}
}

// Header protection is the one knob that genuinely needs a cooperating server:
// the key is shared state. Locally it only requires S1-S4 to clear the nonce
// size, which this provider's 88/155/32/16 already do — so the config is one
// key away from being an AWG 3.0 test case.
func TestRealWorldPaddingsClearTheHeaderProtectionNonce(t *testing.T) {
	entry := realWorldAWGEntry(t, `"header_protection_key":"`+b64key(0x03)+`"`)

	uapi, _, err := buildWGUAPI(entry)
	if err != nil {
		t.Fatalf("buildWGUAPI: %v", err)
	}
	if !strings.Contains(uapi, "header_protection_key=") {
		t.Fatalf("key withheld — paddings should have been large enough:\n%s", uapi)
	}
	if err := newProbeDevice(t).IpcSet(uapi); err != nil {
		t.Fatalf("device rejected header protection: %v\n%s", err, uapi)
	}
}
