// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.

package proxy

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestAmneziaWGURIRoundTripAWG2Fields(t *testing.T) {
	q := url.Values{}
	q.Set("address", "10.0.0.2/32")
	q.Set("private_key", "PRIV")
	q.Set("public_key", "PUB")
	q.Set("allowed_ips", "0.0.0.0/0")
	// AWG 1 (lowercase from URI)
	q.Set("jc", "5")
	q.Set("jmin", "10")
	q.Set("jmax", "50")
	q.Set("s1", "16")
	q.Set("s2", "32")
	q.Set("s3", "8")
	q.Set("s4", "12")
	q.Set("h1", "1")
	q.Set("h2", "2")
	q.Set("h3", "3")
	q.Set("h4", "4")
	// AWG 2.0
	q.Set("i1", "<b 0x01>")
	q.Set("i2", "<r 32>")
	q.Set("i3", "<c>")
	q.Set("i4", "<t>")
	q.Set("i5", "<wt 1000>")
	q.Set("j1", "<b 0xAA>")
	q.Set("j2", "<r 16>")
	q.Set("j3", "<c>")
	q.Set("itime", "300")

	uri := "awg://1.2.3.4:51820?" + q.Encode() + "#test"
	entry, err := ParseProxyURI(uri)
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
	for _, k := range []string{"jc", "jmin", "jmax", "s1", "s2", "s3", "s4", "h1", "h2", "h3", "h4", "itime", "i1", "i2", "i3", "i4", "i5", "j1", "j2", "j3"} {
		if _, ok := am[k]; !ok {
			t.Fatalf("amnezia missing key %q: %+v", k, am)
		}
	}

	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy: ProxyConfig{IP: entry.IP, Port: entry.Port, Type: entry.Type, Extra: entry.Extra},
		Mode:  ProxyModeTunnel,
	})
	if len(cfg.Endpoints) != 1 {
		t.Fatalf("endpoints: %d", len(cfg.Endpoints))
	}
	ep := cfg.Endpoints[0]
	if ep.Amnezia == nil {
		t.Fatal("expected amnezia in endpoint")
	}
	a := ep.Amnezia
	if a.JC != 5 || a.JMin != 10 || a.JMax != 50 {
		t.Fatalf("jitter: %+v", a)
	}
	if a.S1 != 16 || a.S2 != 32 || a.S3 != 8 || a.S4 != 12 {
		t.Fatalf("S1-S4: %+v", a)
	}
	if a.H1 != "1" || a.H2 != "2" || a.H3 != "3" || a.H4 != "4" {
		t.Fatalf("H1-H4: %+v", a)
	}
	if a.I1 != "<b 0x01>" || a.I5 != "<wt 1000>" {
		t.Fatalf("I*: %+v", a)
	}
	if a.J1 != "<b 0xAA>" || a.J3 != "<c>" {
		t.Fatalf("J*: %+v", a)
	}
	if a.ITime != 300 {
		t.Fatalf("itime: %d", a.ITime)
	}

	j, err := json.Marshal(ep)
	if err != nil {
		t.Fatal(err)
	}
	js := string(j)
	for _, want := range []string{`"jc":5`, `"jmin":10`, `"jmax":50`, `"s1":16`, `"h1":"1"`, `"itime":300`} {
		if !strings.Contains(js, want) {
			t.Fatalf("missing %q in marshalled JSON: %s", want, js)
		}
	}
	// Round-trip the JSON to verify string fields survive marshal/unmarshal
	// (json.Marshal HTML-escapes < and > but the value is preserved).
	var rt struct {
		Amnezia struct {
			I1 string `json:"i1"`
			J1 string `json:"j1"`
		} `json:"amnezia"`
	}
	if err := json.Unmarshal(j, &rt); err != nil {
		t.Fatal(err)
	}
	if rt.Amnezia.I1 != a.I1 || rt.Amnezia.J1 != a.J1 {
		t.Fatalf("string round-trip mismatch: i1=%q j1=%q", rt.Amnezia.I1, rt.Amnezia.J1)
	}
}

func TestAmneziaWGURICaseInsensitiveCapitalKeys(t *testing.T) {
	// AmneziaVPN clients emit Capitalized parameter names.
	raw := "awg://1.2.3.4:51820?" +
		"address=10.0.0.2%2F32&private_key=PRIV&public_key=PUB&allowed_ips=0.0.0.0%2F0" +
		"&Jc=4&Jmin=20&Jmax=10&S1=15&H1=7&I1=%3Cb+0x01%3E&J1=%3Cc%3E&Itime=120"

	entry, err := ParseProxyURI(raw)
	if err != nil {
		t.Fatal(err)
	}
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy: ProxyConfig{IP: entry.IP, Port: entry.Port, Type: entry.Type, Extra: entry.Extra},
		Mode:  ProxyModeTunnel,
	})
	if len(cfg.Endpoints) != 1 || cfg.Endpoints[0].Amnezia == nil {
		t.Fatalf("expected endpoint with amnezia, got %+v", cfg.Endpoints)
	}
	a := cfg.Endpoints[0].Amnezia
	if a.JC != 4 || a.S1 != 15 || a.H1 != "7" || a.ITime != 120 {
		t.Fatalf("capitalized keys: %+v", a)
	}
	if a.I1 != "<b 0x01>" || a.J1 != "<c>" {
		t.Fatalf("capitalized string keys: %+v", a)
	}
	// Jmin > Jmax should be normalized (swap).
	if a.JMin != 10 || a.JMax != 20 {
		t.Fatalf("expected jmin/jmax swap, got jmin=%d jmax=%d", a.JMin, a.JMax)
	}
}

func TestParseJSONOutboundXrayWireGuard(t *testing.T) {
	body := `{
        "protocol": "wireguard",
        "tag": "xray-wg",
        "settings": {
            "secretKey": "PRIV",
            "address": ["10.0.0.2/32"],
            "mtu": 1420,
            "peers": [{
                "publicKey": "PUB",
                "endpoint": "1.2.3.4:51820",
                "allowedIPs": ["0.0.0.0/0"]
            }]
        }
    }`
	entries, ok := parseSubscriptionJSON(body)
	if !ok || len(entries) != 1 {
		t.Fatalf("parseSubscriptionJSON: ok=%v entries=%d", ok, len(entries))
	}
	e := entries[0]
	if e.Type != "WIREGUARD" || e.IP != "1.2.3.4" || e.Port != 51820 {
		t.Fatalf("unexpected entry: %+v", e)
	}
	var ex map[string]interface{}
	if err := json.Unmarshal(e.Extra, &ex); err != nil {
		t.Fatal(err)
	}
	if ex["private_key"] != "PRIV" || ex["public_key"] != "PUB" {
		t.Fatalf("keys: %+v", ex)
	}
	if mtu, _ := ex["mtu"].(float64); mtu != 1420 {
		t.Fatalf("mtu: %+v", ex["mtu"])
	}
}

func TestParseJSONOutboundSingBoxAmneziaWG(t *testing.T) {
	body := `{
        "type": "wireguard",
        "tag": "sb-awg",
        "server": "1.2.3.4",
        "server_port": 51820,
        "private_key": "PRIV",
        "address": ["10.0.0.2/32"],
        "peers": [{
            "public_key": "PUB",
            "allowed_ips": ["0.0.0.0/0"]
        }],
        "amnezia": {
            "jc": 5, "jmin": 10, "jmax": 50,
            "s1": 16, "h1": 1,
            "i1": "<b 0x01>", "j1": "<c>", "itime": 300
        }
    }`
	entries, ok := parseSubscriptionJSON(body)
	if !ok || len(entries) != 1 {
		t.Fatalf("parseSubscriptionJSON: ok=%v entries=%d", ok, len(entries))
	}
	e := entries[0]
	if e.Type != "AMNEZIAWG" || e.IP != "1.2.3.4" || e.Port != 51820 {
		t.Fatalf("unexpected entry: %+v", e)
	}
	var ex map[string]interface{}
	if err := json.Unmarshal(e.Extra, &ex); err != nil {
		t.Fatal(err)
	}
	am, ok := ex["amnezia"].(map[string]interface{})
	if !ok {
		t.Fatalf("amnezia: %+v", ex["amnezia"])
	}
	if am["jc"].(float64) != 5 || am["i1"].(string) != "<b 0x01>" {
		t.Fatalf("amnezia content: %+v", am)
	}
}

// TestAmneziaHeaderRangePreserved verifies that H1-H4 range strings from
// the user's config travel through amneziaFromExtra unchanged so that the
// upstream sing-box-extended (>= v1.13.11-extended-2.0.0) can parse them
// into *Xbadoption.Range and randomize per packet.
func TestAmneziaHeaderRangePreserved(t *testing.T) {
	in := map[string]interface{}{
		"h1": "122163117-125750861",
		"h2": "708517833-711678982",
		"h3": "1169962535-1174185828",
		"h4": "2133918740-2137138052",
	}
	am := amneziaFromExtra(map[string]interface{}{"amnezia": in})
	if am == nil {
		t.Fatal("nil amnezia")
	}
	if am.H1 != "122163117-125750861" {
		t.Fatalf("H1 = %q", am.H1)
	}
	if am.H2 != "708517833-711678982" {
		t.Fatalf("H2 = %q", am.H2)
	}
	if am.H3 != "1169962535-1174185828" {
		t.Fatalf("H3 = %q", am.H3)
	}
	if am.H4 != "2133918740-2137138052" {
		t.Fatalf("H4 = %q", am.H4)
	}
}

func TestAmneziaHeaderStringFromInt(t *testing.T) {
	cases := []struct {
		v    interface{}
		want string
	}{
		{"123456", "123456"},
		{123456, "123456"},
		{float64(123456), "123456"},
		{"4294967295", "4294967295"},
		{"122163117-125750861", "122163117-125750861"},
		{"", ""},
		{nil, ""},
	}
	for _, tc := range cases {
		got := amneziaHeaderString(tc.v)
		if got != tc.want {
			t.Fatalf("amneziaHeaderString(%v) = %q, want %q", tc.v, got, tc.want)
		}
	}
}

func TestAmneziaFromExtraNormalizesJMinJMax(t *testing.T) {
	in := map[string]interface{}{
		"jmin": 50,
		"jmax": 10,
		"s1":   -3,
		"jc":   -1,
	}
	am := amneziaFromExtra(map[string]interface{}{"amnezia": in})
	if am == nil {
		t.Fatal("expected non-nil amnezia")
	}
	if am.JMin != 10 || am.JMax != 50 {
		t.Fatalf("swap failed: jmin=%d jmax=%d", am.JMin, am.JMax)
	}
	if am.S1 != 0 || am.JC != 0 {
		t.Fatalf("negative clamp failed: %+v", am)
	}
}
