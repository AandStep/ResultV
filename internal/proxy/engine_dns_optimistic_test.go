// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import "testing"

// Окно названо, а не унаследовано: ядро отдавало бы протухший ответ трое суток,
// что переживает любую смену сети. Шесть часов покрывают ночь со спящим
// телефоном, а переход между Wi-Fi и мобильной сетью обновляет кэш задолго до
// исхода окна.
func TestDNSCacheIsOptimisticWithSixHourWindow(t *testing.T) {
	for _, mode := range []ProxyMode{ProxyModeTunnel, ProxyModeProxy} {
		dns := buildDNS(EngineConfig{
			Mode:  mode,
			Proxy: ProxyConfig{Type: "VLESS", IP: "203.0.113.7", Port: 443},
		})
		if dns == nil {
			t.Fatalf("%v: buildDNS вернул nil", mode)
		}
		if dns.Optimistic == nil {
			t.Fatalf("%v: optimistic не задан — резолвер будет блокироваться на протухшей записи", mode)
		}
		if !dns.Optimistic.Enabled {
			t.Errorf("%v: optimistic.enabled = false", mode)
		}
		if dns.Optimistic.Timeout != "6h" {
			t.Errorf("%v: optimistic.timeout = %q, ожидалось 6h", mode, dns.Optimistic.Timeout)
		}
	}
}
