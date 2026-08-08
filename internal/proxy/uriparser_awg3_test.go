// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

// awg:// carries the AmneziaWG 3.0 knobs through to Extra, where the handshake
// probe can pick them up. Losing them at parse time would make ping report
// "unreachable" against any server using header protection, since the probe
// would answer with unencrypted headers.
func TestAmneziaWGURICarriesAWG3Knobs(t *testing.T) {
	q := url.Values{}
	q.Set("private_key", "aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=")
	q.Set("public_key", "WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=")
	q.Set("s1", "15")
	q.Set("header_protection_key", b64key(0x03))
	q.Set("content_padding_addition", "10-50")
	q.Set("rekey_after_time", "110-130")
	q.Set("max_handshake_attempts", "18")

	entry, err := ParseProxyURI("awg://1.2.3.4:51820?" + q.Encode() + "#awg3")
	if err != nil {
		t.Fatalf("ParseProxyURI: %v", err)
	}
	if entry.Type != "AMNEZIAWG" {
		t.Fatalf("type: %q", entry.Type)
	}

	var ex map[string]interface{}
	if err := json.Unmarshal(entry.Extra, &ex); err != nil {
		t.Fatal(err)
	}
	am, ok := ex["amnezia"].(map[string]interface{})
	if !ok {
		t.Fatalf("amnezia missing in extra: %+v", ex)
	}
	for _, k := range []string{
		"header_protection_key", "content_padding_addition",
		"rekey_after_time", "max_handshake_attempts",
	} {
		if _, ok := am[k]; !ok {
			t.Fatalf("amnezia missing key %q: %+v", k, am)
		}
	}
	// The range form must arrive intact — UintRange.FromString takes "a-b".
	if got := am["content_padding_addition"]; got != "10-50" {
		t.Errorf("content_padding_addition = %v, want 10-50", got)
	}
}

// AmneziaVPN writes these in the .conf as CamelCase with no separators, and
// our Kotlin importer forwards whatever it read, so both spellings must land.
func TestAmneziaWGURIAcceptsCamelCaseAWG3Keys(t *testing.T) {
	q := url.Values{}
	q.Set("private_key", "aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=")
	q.Set("public_key", "WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=")
	q.Set("HeaderProtectionKey", b64key(0x03))
	q.Set("RekeyAfterTime", "120")

	entry, err := ParseProxyURI("awg://1.2.3.4:51820?" + q.Encode() + "#awg3")
	if err != nil {
		t.Fatalf("ParseProxyURI: %v", err)
	}
	var ex map[string]interface{}
	if err := json.Unmarshal(entry.Extra, &ex); err != nil {
		t.Fatal(err)
	}
	am, _ := ex["amnezia"].(map[string]interface{})
	if am["header_protection_key"] == nil {
		t.Errorf("HeaderProtectionKey did not normalize: %+v", am)
	}
	if am["rekey_after_time"] == nil {
		t.Errorf("RekeyAfterTime did not normalize: %+v", am)
	}
}

// End to end for the one path that actually works today: a parsed awg:// URI
// must produce a UAPI string the forked device accepts, AWG 3.0 knobs included.
func TestAWG3URIProducesUAPITheForkAccepts(t *testing.T) {
	q := url.Values{}
	q.Set("private_key", "aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=")
	q.Set("public_key", "WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=")
	q.Set("s1", "15")
	q.Set("s2", "15")
	q.Set("s3", "15")
	q.Set("s4", "15")
	q.Set("header_protection_key", b64key(0x03))
	q.Set("content_padding_addition", "10-50")
	q.Set("rekey_after_time", "110-130")

	entry, err := ParseProxyURI("awg://203.0.113.7:51820?" + q.Encode() + "#awg3")
	if err != nil {
		t.Fatalf("ParseProxyURI: %v", err)
	}
	uapi, _, err := buildWGUAPI(entry)
	if err != nil {
		t.Fatalf("buildWGUAPI: %v", err)
	}
	if !strings.Contains(uapi, "header_protection_key=") {
		t.Fatalf("header protection lost on the way to UAPI:\n%s", uapi)
	}
	if err := newProbeDevice(t).IpcSet(uapi); err != nil {
		t.Fatalf("wireguard-go rejected the UAPI built from an awg:// URI: %v\n%s", err, uapi)
	}
}
