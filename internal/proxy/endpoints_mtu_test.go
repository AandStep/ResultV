// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import "testing"

func TestWireGuardMTUOverride(t *testing.T) {
	cases := []struct {
		name       string
		override   int
		configured int
		want       int
	}{
		{"без переопределения берётся значение узла", 0, 1408, 1408},
		{"переопределение перекрывает узел", 1280, 1408, 1280},
		{"работает и при нулевом значении узла", 1280, 0, 1280},
		// Below the IPv4 minimum a path is not required to carry anything, and
		// above 1500 the override would create the very problem it exists to
		// test for.
		{"слишком маленькое игнорируется", 500, 1408, 1408},
		{"слишком большое игнорируется", 9000, 1408, 1408},
		{"граница снизу принимается", 576, 1408, 576},
		{"граница сверху принимается", 1500, 1408, 1500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := wireguardMTU(tc.configured, tc.override); got != tc.want {
				t.Errorf("wireguardMTU(%d, %d) = %d, ожидалось %d",
					tc.configured, tc.override, got, tc.want)
			}
		})
	}
}

// Переопределение должно доезжать до конфига, а не только до функции.
func TestWireGuardMTUReachesEndpoint(t *testing.T) {
	entry := ProxyConfig{
		IP: "1.2.3.4", Port: 51820, Type: "WIREGUARD",
		Extra: []byte(`{"address":["10.0.0.2/32"],` +
			`"private_key":"aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=",` +
			`"public_key":"WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=",` +
			`"allowed_ips":["0.0.0.0/0"],"mtu":1408}`),
	}
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{Proxy: entry, Mode: ProxyModeTunnel, WGMTU: 1280})
	if len(cfg.Endpoints) != 1 {
		t.Fatalf("ожидался один эндпоинт, получено %d", len(cfg.Endpoints))
	}
	if cfg.Endpoints[0].MTU != 1280 {
		t.Errorf("MTU эндпоинта = %d, ожидалось 1280", cfg.Endpoints[0].MTU)
	}
	assertCoreAcceptsConfig(t, cfg)
}
