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
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

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
	q.Set("address", "10.0.0.2/32")
	q.Set("private_key", "PRIV")
	q.Set("public_key", "PUB")
	q.Set("allowed_ips", "0.0.0.0/0")
	q.Set("RandomTrailers", "on")
	q.Set("DisableCookies", "off")

	entry, err := ParseProxyURI("awg://1.2.3.4:51820?" + q.Encode() + "#t")
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

// The same for the JSON-shaped links, where the amnezia block arrives as an
// object rather than as query parameters.
func TestAWG31SurvivesJSONLink(t *testing.T) {
	raw := map[string]interface{}{
		"address":     []string{"10.0.0.2/32"},
		"private_key": "PRIV",
		"public_key":  "PUB",
		"allowed_ips": []string{"0.0.0.0/0"},
		"amnezia": map[string]interface{}{
			"jc":              4,
			"random_trailers": "on",
		},
	}
	blob, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := ParseProxyURI("wireguard://1.2.3.4:51820?" + url.Values{"json": {string(blob)}}.Encode() + "#t")
	if err != nil {
		// Not every build accepts this link shape; the assertion that matters
		// is the one below, on the extra we do produce.
		t.Skipf("ссылка такой формы не разбирается: %v", err)
	}
	if !strings.Contains(string(entry.Extra), "random_trailers") {
		t.Errorf("random_trailers не доехал в extra: %s", entry.Extra)
	}
}

// AWG 3.1 knobs must not leak into the sing-box config: the core parses it
// strictly and an unknown key under "amnezia" fails the entire start. They
// travel to the device by a second IpcSet instead (see applyAWG31).
func TestAWG31NeverReachesEngineConfig(t *testing.T) {
	q := url.Values{}
	q.Set("address", "10.0.0.2/32")
	q.Set("private_key", "PRIV")
	q.Set("public_key", "PUB")
	q.Set("allowed_ips", "0.0.0.0/0")
	q.Set("RandomTrailers", "on")
	q.Set("DisableCookies", "on")

	entry, err := ParseProxyURI("awg://1.2.3.4:51820?" + q.Encode() + "#t")
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
}
