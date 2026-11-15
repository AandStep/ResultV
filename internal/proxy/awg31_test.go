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
	"testing"
)

// AmneziaWG writes these as on/off in its config files, JSON subscriptions send
// booleans, and some providers send 1/0. All three mean the same switch.
func TestAWG31ParsesEverySpelling(t *testing.T) {
	cases := []struct {
		raw  string
		want awg31Knobs
	}{
		{`{"random_trailers":"on","disable_cookies":"off"}`, awg31Knobs{RandomTrailers: boolPtr(true), DisableCookies: boolPtr(false)}},
		{`{"RandomTrailers":true,"DisableCookies":true}`, awg31Knobs{RandomTrailers: boolPtr(true), DisableCookies: boolPtr(true)}},
		{`{"random-trailers":1}`, awg31Knobs{RandomTrailers: boolPtr(true)}},
		{`{"random_trailers":"enabled"}`, awg31Knobs{RandomTrailers: boolPtr(true)}},
		{`{"jc":4,"s1":16}`, awg31Knobs{}},
	}
	for _, tc := range cases {
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(tc.raw), &m); err != nil {
			t.Fatalf("%s: %v", tc.raw, err)
		}
		got := awg31FromExtra(m)
		if !sameKnob(got.RandomTrailers, tc.want.RandomTrailers) ||
			!sameKnob(got.DisableCookies, tc.want.DisableCookies) {
			t.Errorf("%s: получено {rt:%s cookies:%s}, ожидалось {rt:%s cookies:%s}",
				tc.raw, knobString(got.RandomTrailers), knobString(got.DisableCookies),
				knobString(tc.want.RandomTrailers), knobString(tc.want.DisableCookies))
		}
	}
}

// An unreadable value must stay unstated. Turning it into "off" would quietly
// pick the other protocol: with random trailers mismatched, every handshake the
// peer sends is dropped for being the wrong size.
func TestAWG31IgnoresUnreadableValues(t *testing.T) {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(`{"random_trailers":"maybe","disable_cookies":"да"}`), &m); err != nil {
		t.Fatal(err)
	}
	got := awg31FromExtra(m)
	if !got.empty() {
		t.Errorf("нечитаемые значения не должны становиться выключателями: %+v", got)
	}
}

// Only WireGuard-shaped nodes carry these switches.
func TestAWG31OnlyForWireGuardNodes(t *testing.T) {
	extra := json.RawMessage(`{"amnezia":{"random_trailers":"on"}}`)
	if got := awg31KnobsFor(ProxyConfig{Type: "vless", Extra: extra}); !got.empty() {
		t.Errorf("VLESS-узел не должен отдавать параметры AWG: %+v", got)
	}
	got := awg31KnobsFor(ProxyConfig{Type: "amneziawg", Extra: extra})
	if got.RandomTrailers == nil || !*got.RandomTrailers {
		t.Errorf("AWG-узел должен отдать random_trailers=on, получено %+v", got)
	}
}

func awg31TestNode(t *testing.T, amnezia map[string]interface{}) ProxyConfig {
	t.Helper()
	extra, err := json.Marshal(map[string]interface{}{
		"private_key": randomWGKey(t),
		"public_key":  randomWGKey(t),
		"address":     []string{"10.0.0.2/32"},
		"allowed_ips": []string{"0.0.0.0/0"},
		"amnezia":     amnezia,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ProxyConfig{Type: "AMNEZIAWG", IP: "203.0.113.7", Port: 51820, Extra: extra}
}

func onlyEndpoint(t *testing.T, cfg SingBoxConfig) SBEndpoint {
	t.Helper()
	if len(cfg.Endpoints) != 1 {
		t.Fatalf("ожидался один эндпоинт, получено %d", len(cfg.Endpoints))
	}
	return cfg.Endpoints[0]
}

// The switches travel in the endpoint's amnezia block, and the pinned core has
// to accept them there: it decodes strictly, and a key it does not know fails
// the whole start.
func TestAWG31SwitchesReachTheEngineConfig(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy: awg31TestNode(t, map[string]interface{}{"jc": 4, "random_trailers": "on", "disable_cookies": "on"}),
		Mode:  ProxyModeTunnel,
	})
	am := onlyEndpoint(t, cfg).Amnezia
	if am == nil || !am.RandomTrailers || !am.DisableCookies {
		t.Fatalf("переключатели 3.1 не попали в эндпоинт: %+v", am)
	}
	assertCoreAcceptsConfig(t, cfg)
}

// A plain WireGuard node that only turns trailers on still needs the block.
func TestAWG31AloneStillEmitsTheBlock(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy: awg31TestNode(t, map[string]interface{}{"random_trailers": true}),
		Mode:  ProxyModeTunnel,
	})
	am := onlyEndpoint(t, cfg).Amnezia
	if am == nil || !am.RandomTrailers {
		t.Fatalf("random_trailers без остальных параметров AWG потерялся: %+v", am)
	}
	assertCoreAcceptsConfig(t, cfg)
}

// Off is the device default, so a node that only says "off" changes nothing.
func TestAWG31StatedOffAddsNothing(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy: awg31TestNode(t, map[string]interface{}{"random_trailers": "off", "disable_cookies": "off"}),
		Mode:  ProxyModeTunnel,
	})
	if am := onlyEndpoint(t, cfg).Amnezia; am != nil {
		t.Fatalf("выключенные переключатели не должны создавать блок amnezia: %+v", am)
	}
}

// The ping probe runs its own engine from its own config builder, and a node
// with trailers on cannot complete a handshake without them.
func TestAWG31PingProbeCarriesTheSwitches(t *testing.T) {
	cfg, err := BuildPingProbeConfig(awg31TestNode(t, map[string]interface{}{"random_trailers": "on"}), getFreeLocalPort(0), "")
	if err != nil {
		t.Fatal(err)
	}
	am := onlyEndpoint(t, cfg).Amnezia
	if am == nil || !am.RandomTrailers {
		t.Fatalf("проба пинга идёт без random_trailers: %+v", am)
	}
}

// The core writes the switches in the same IpcSet as the peers, so the device
// has them before its first initiation leaves.
func TestAWG31DeviceStartsWithTheSwitches(t *testing.T) {
	boxCtx := startEndpointCore(t, mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy:   awg31TestNode(t, map[string]interface{}{"random_trailers": "on", "disable_cookies": "on"}),
		Mode:    ProxyModeTunnel,
		DataDir: t.TempDir(),
	}))
	dump, err := wgDeviceDump(boxCtx)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range awg31Keys {
		if got := uapiValue(dump, key); got != "1" {
			t.Errorf("устройство стартовало с %s=%q", key, got)
		}
	}
}

func boolPtr(v bool) *bool { return &v }

func sameKnob(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func knobString(v *bool) string {
	if v == nil {
		return "—"
	}
	if *v {
		return "on"
	}
	return "off"
}
