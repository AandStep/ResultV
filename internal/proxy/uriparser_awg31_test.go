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

// The 3.1 switches have to survive the whole way: link → extra → knobs. Losing
// them anywhere in between is silent, and with random_trailers mismatched the
// tunnel never comes up at all.
func TestAWG31SurvivesURIRoundTrip(t *testing.T) {
	q := url.Values{}
	q.Set("private_key", "aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=")
	q.Set("public_key", "WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=")
	q.Set("address", "10.0.0.2/32")
	q.Set("allowed_ips", "0.0.0.0/0")
	q.Set("RandomTrailers", "on")
	q.Set("DisableCookies", "off")

	entry, err := ParseProxyURI("awg://1.2.3.4:51820?" + q.Encode() + "#awg31")
	if err != nil {
		t.Fatal(err)
	}
	knobs := awg31KnobsFor(ProxyConfig{Type: entry.Type, Extra: entry.Extra})
	if knobs.RandomTrailers == nil || !*knobs.RandomTrailers {
		t.Errorf("random_trailers потерялся по дороге: %s", entry.Extra)
	}
	if knobs.DisableCookies == nil || *knobs.DisableCookies {
		t.Errorf("disable_cookies потерялся или перевернулся: %s", entry.Extra)
	}
}

// Ту же дорогу проходит запись из JSON-подписки, где amnezia приезжает
// объектом, а не параметрами запроса.
func TestAWG31SurvivesJSONOutbound(t *testing.T) {
	const j = `{
	  "type": "amneziawg",
	  "tag": "awg-node",
	  "server": "1.2.3.4",
	  "server_port": 51820,
	  "private_key": "aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=",
	  "public_key": "WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=",
	  "address": ["10.0.0.2/32"],
	  "allowed_ips": ["0.0.0.0/0"],
	  "amnezia": {"jc": 4, "s1": 15, "random_trailers": "on", "DisableCookies": "off"}
	}`
	entries, err := ParseSubscriptionBody(j)
	if err != nil {
		t.Fatalf("подписка не разобралась: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ожидалась одна запись, получено %d", len(entries))
	}
	knobs := awg31KnobsFor(ProxyConfig{Type: entries[0].Type, Extra: entries[0].Extra})
	if knobs.RandomTrailers == nil || !*knobs.RandomTrailers {
		t.Errorf("random_trailers не доехал в extra: %s", entries[0].Extra)
	}
	if knobs.DisableCookies == nil || *knobs.DisableCookies {
		t.Errorf("disable_cookies не доехал или перевернулся: %s", entries[0].Extra)
	}
}

// AWG 3.1 knobs must not leak into the sing-box config: the core parses it
// strictly and an unknown key under "amnezia" fails the entire start. They
// travel to the device by a second IpcSet instead (see ApplyAWG31).
func TestAWG31NeverReachesEngineConfig(t *testing.T) {
	q := url.Values{}
	q.Set("private_key", "aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=")
	q.Set("public_key", "WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=")
	q.Set("address", "10.0.0.2/32")
	q.Set("allowed_ips", "0.0.0.0/0")
	q.Set("RandomTrailers", "on")
	q.Set("DisableCookies", "on")

	entry, err := ParseProxyURI("awg://1.2.3.4:51820?" + q.Encode() + "#awg31")
	if err != nil {
		t.Fatal(err)
	}
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy: ProxyConfig{IP: entry.IP, Port: entry.Port, Type: entry.Type, Extra: entry.Extra},
		Mode:  ProxyModeTunnel,
	})
	rendered, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range awg31Keys {
		if strings.Contains(string(rendered), key) {
			t.Errorf("%s попал в конфиг ядра — ядро отвергнет неизвестный ключ целиком:\n%s", key, rendered)
		}
	}
	// Не «ключей нет», а «ядро принимает»: пустая проверка прошла бы и на
	// сломанном конфиге.
	assertCoreAcceptsConfig(t, cfg)
}
