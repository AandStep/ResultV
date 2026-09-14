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
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	wgprotocol "github.com/sagernet/sing-box/protocol/wireguard"
)

// The path applyAWG31 walks is made of unexported fields, so it cannot be
// checked by the compiler. This test is the guard: it fails on the next engine
// bump that renames or retypes any step, which is the moment to re-check the
// route — not months later, in a user's log, as "3.1 not applied".
func TestAWG31DeviceFieldsStillExist(t *testing.T) {
	protocolEndpoint := reflect.TypeOf(wgprotocol.Endpoint{})
	field, found := protocolEndpoint.FieldByName("endpoint")
	if !found {
		t.Fatal("protocol/wireguard.Endpoint потерял поле endpoint — путь к устройству надо искать заново")
	}
	transportEndpoint := field.Type
	if transportEndpoint.Kind() != reflect.Pointer {
		t.Fatalf("ожидался указатель на transport-эндпоинт, получено %s", transportEndpoint)
	}
	deviceField, found := transportEndpoint.Elem().FieldByName("device")
	if !found {
		t.Fatal("transport/wireguard.Endpoint потерял поле device")
	}
	if !deviceField.Type.Implements(reflect.TypeOf((*ipcSetter)(nil)).Elem()) {
		t.Fatalf("устройство %s больше не принимает IpcSet", deviceField.Type)
	}
}

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

// The UAPI parses with strconv.ParseBool, which does not know on/off — so what
// reaches the device must be true/false, one key per line.
func TestAWG31IpcLinesAreUAPIShaped(t *testing.T) {
	knobs := awg31Knobs{RandomTrailers: boolPtr(true), DisableCookies: boolPtr(false)}
	got := knobs.ipcLines()
	for _, want := range []string{"random_trailers=true\n", "disable_cookies=false\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("нет строки %q в %q", want, got)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		if strings.Count(line, "=") != 1 {
			t.Errorf("строка UAPI должна быть ровно key=value, получено %q", line)
		}
	}
}

// Only WireGuard-shaped nodes carry these switches; reading them off a VLESS
// node would apply them to an endpoint that does not exist.
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

// Nothing stated means nothing sent: a node that says nothing about 3.1 must
// not have its device touched at all.
func TestAWG31EmptyKnobsDoNothing(t *testing.T) {
	if err := applyAWG31(nil, awg31Knobs{}, nil); err != nil {
		t.Errorf("пустой набор не должен быть ошибкой: %v", err)
	}
}

// Every failure on the way to the device is reported, never panicked: a session
// with 3.0 behaviour is worth more than a crash on connect.
func TestAWG31ReportsMissingCore(t *testing.T) {
	knobs := awg31Knobs{RandomTrailers: boolPtr(true)}
	if err := applyAWG31(nil, knobs, nil); err == nil {
		t.Error("отсутствие контекста должно быть ошибкой")
	}
	if err := applyAWG31(context.Background(), knobs, nil); err == nil {
		t.Error("контекст без менеджера эндпоинтов должен быть ошибкой")
	}
}

// A zero-value endpoint has no device yet — the state a domain-addressed peer
// is in until the post-start stage. It must read as "not ready", not as a
// broken engine.
func TestAWG31DeviceNotReady(t *testing.T) {
	_, err := awg31Device(&wgprotocol.Endpoint{})
	if err == nil {
		t.Fatal("ожидалась ошибка для эндпоинта без устройства")
	}
	if !strings.Contains(err.Error(), "ещё не создано") {
		t.Errorf("ошибка должна называть состояние, получено: %v", err)
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
