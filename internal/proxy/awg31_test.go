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

// AmneziaWG writes these as on/off in its config files, JSON subscriptions send
// booleans, and some providers send 1/0. All three mean the same switch.
func TestAWG31ParsesEverySpelling(t *testing.T) {
	cases := []struct {
		raw  string
		want awg31Knobs
	}{
		{`{"random_trailers":"on","disable_cookies":"off"}`, awg31Knobs{RandomTrailers: awgBoolPtr(true), DisableCookies: awgBoolPtr(false)}},
		{`{"RandomTrailers":true,"DisableCookies":true}`, awg31Knobs{RandomTrailers: awgBoolPtr(true), DisableCookies: awgBoolPtr(true)}},
		{`{"random-trailers":1}`, awg31Knobs{RandomTrailers: awgBoolPtr(true)}},
		{`{"random_trailers":"enabled"}`, awg31Knobs{RandomTrailers: awgBoolPtr(true)}},
		{`{"jc":4,"s1":16}`, awg31Knobs{}},
	}
	for _, tc := range cases {
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(tc.raw), &m); err != nil {
			t.Fatalf("%s: %v", tc.raw, err)
		}
		got := awg31FromExtra(m)
		if !sameAWGKnob(got.RandomTrailers, tc.want.RandomTrailers) ||
			!sameAWGKnob(got.DisableCookies, tc.want.DisableCookies) {
			t.Errorf("%s: получено {rt:%s cookies:%s}, ожидалось {rt:%s cookies:%s}",
				tc.raw, awgKnobString(got.RandomTrailers), awgKnobString(got.DisableCookies),
				awgKnobString(tc.want.RandomTrailers), awgKnobString(tc.want.DisableCookies))
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
	if got := awg31FromExtra(m); !got.empty() {
		t.Errorf("нечитаемые значения не должны становиться выключателями: %+v", got)
	}
}

// The UAPI parses with strconv.ParseBool, which does not know on/off — so what
// reaches the device must be true/false, one key per line.
func TestAWG31IpcLinesAreUAPIShaped(t *testing.T) {
	knobs := awg31Knobs{RandomTrailers: awgBoolPtr(true), DisableCookies: awgBoolPtr(false)}
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

// The description goes into a log that exists in two languages, so it is
// written in the UAPI spelling: a knob name is the same word in both.
func TestAWG31DescribeUsesUAPINames(t *testing.T) {
	knobs := awg31Knobs{RandomTrailers: awgBoolPtr(true), DisableCookies: awgBoolPtr(false)}
	if got := knobs.describe(); got != "random_trailers=on, disable_cookies=off" {
		t.Errorf("описание = %q", got)
	}
}

func awgBoolPtr(v bool) *bool { return &v }

func sameAWGKnob(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func awgKnobString(v *bool) string {
	if v == nil {
		return "—"
	}
	if *v {
		return "on"
	}
	return "off"
}
