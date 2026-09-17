// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import "testing"

// Дефолт на Android — gvisor, и это не вкусовщина: на sing-tun 0.9 системный
// стек на телефоне перестал обслуживать TCP (приёмка блока 1, раздел 11.1
// спека). Переключатель нужен, чтобы это можно было сравнить, а не чтобы
// выбирать.
func TestEffectiveTunStack(t *testing.T) {
	cases := []struct {
		name     string
		selected string
		want     string
	}{
		{"пусто — дефолт gvisor", "", "gvisor"},
		{"system выбирается явно", "system", "system"},
		{"gvisor выбирается явно", "gvisor", "gvisor"},
		{"регистр и пробелы не важны", "  SYSTEM ", "system"},
		{"неизвестное значение падает в дефолт", "mystack", "gvisor"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := effectiveTunStack(tc.selected); got != tc.want {
				t.Errorf("effectiveTunStack(%q) = %q, ожидалось %q", tc.selected, got, tc.want)
			}
		})
	}
}

// И выбор должен доезжать до TUN-инбаунда, а не только до функции.
func TestTunStackReachesInbound(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy:    ProxyConfig{IP: "1.2.3.4", Port: 443, Type: "VLESS", Password: "11111111-1111-1111-1111-111111111111"},
		Mode:     ProxyModeTunnel,
		TunStack: "system",
	})
	if len(cfg.Inbounds) == 0 {
		t.Fatal("нет инбаундов")
	}
	if cfg.Inbounds[0].Stack != "system" {
		t.Errorf("стек TUN = %q, ожидалось system", cfg.Inbounds[0].Stack)
	}
	assertCoreAcceptsConfig(t, cfg)
}
