// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package mobile

import (
	"strings"
	"testing"
)

// The knobs are applied to whatever node the engine was actually started with,
// and that is the node the last config was built from — including the AUTO
// path, which swaps the member without touching the user's profile. So the
// memory has to be refreshed by the one funnel every build goes through.
func TestBuiltNodeFollowsTheLastBuiltConfig(t *testing.T) {
	dir := t.TempDir()
	opts := encodeOptions(BuildOptions{})

	awg := "awg://1.2.3.4:51820?private_key=aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8%3D" +
		"&public_key=WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE%3D" +
		"&address=10.0.0.2%2F32&allowed_ips=0.0.0.0%2F0&RandomTrailers=on#awg"
	if _, err := BuildSingBoxConfigV2(awg, dir, opts); err != nil {
		t.Fatalf("сборка конфига AWG: %v", err)
	}
	if got := builtNode(); !strings.EqualFold(got.Type, "AMNEZIAWG") {
		t.Fatalf("после сборки AWG узел = %q", got.Type)
	}

	vless := "vless://11111111-1111-1111-1111-111111111111@5.6.7.8:443?encryption=none&security=tls&type=tcp#v"
	if _, err := BuildSingBoxConfigV2(vless, dir, opts); err != nil {
		t.Fatalf("сборка конфига VLESS: %v", err)
	}
	if got := builtNode(); strings.EqualFold(got.Type, "AMNEZIAWG") {
		t.Error("память об узле не обновилась — ключи 3.1 уехали бы на чужой узел")
	}
}

// Without a server there is nothing to apply to, and that must be an ordinary
// error rather than a crash on the first connect after a process restart.
func TestApplyAWG31WithoutServer(t *testing.T) {
	if _, err := ApplyAWG31(nil); err == nil {
		t.Error("без сервера должна быть ошибка, а не тишина")
	}
}
