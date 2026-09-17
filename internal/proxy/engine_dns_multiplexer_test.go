// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import "testing"

// sing-box 1.14 поставил мультиплексор запросов перед транспортами tcp, tls и
// udp: как только фоновая проба решает, что резолвер поддерживает
// переиспользование, все запросы переезжают на одно общее долгоживущее
// соединение, чья проверка живости — `conn != nil`. Через прокси-аутбаунд это
// соединение заклинивает, и запросы перестают возвращаться совсем: ни ответа,
// ни ошибки. Транспорт https — единственный удалённый, которого мультиплексор
// не касается, поэтому DoH идёт первым.
func TestTunnelDNSLeadsWithDoH(t *testing.T) {
	dns := buildDNS(EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{Type: "VLESS", IP: "203.0.113.7", Port: 443},
	})
	if dns == nil {
		t.Fatal("buildDNS вернул nil")
	}

	var sawHTTPS, sawFallback bool
	for _, s := range dns.Servers {
		switch s.Type {
		case "https":
			if s.Detour != "" {
				sawHTTPS = true
			}
		case "fallback":
			sawFallback = true
			if len(s.Servers) < 2 {
				t.Errorf("fallback %q ссылается на %d сервер(ов), ожидалось минимум два", s.Tag, len(s.Servers))
			}
			if s.Timeout == "" {
				t.Errorf("fallback %q без таймаута — заклинившая нога будет висеть вечно", s.Tag)
			}
		}
	}
	if !sawHTTPS {
		t.Error("среди туннельных серверов DNS нет ни одного https — на 1.14 tcp через прокси замолкает")
	}
	if !sawFallback {
		t.Error("нет обёртки fallback — правила некуда направлять")
	}
}

// Мультиплексируемых транспортов через детур не должно остаться в одиночестве:
// каждый такой сервер обязан быть ногой обёртки, а не целью правила.
func TestNoBareMultiplexedTransportIsAddressableThroughDetour(t *testing.T) {
	dns := buildDNS(EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{Type: "VLESS", IP: "203.0.113.7", Port: 443},
	})
	if dns == nil {
		t.Fatal("buildDNS вернул nil")
	}

	legs := map[string]bool{}
	for _, s := range dns.Servers {
		if s.Type == "fallback" {
			for _, leg := range s.Servers {
				legs[leg] = true
			}
		}
	}
	for _, s := range dns.Servers {
		if s.Detour == "" {
			continue
		}
		if s.Type != "tcp" && s.Type != "tls" && s.Type != "udp" {
			continue
		}
		if !legs[s.Tag] {
			t.Errorf("сервер %q типа %s идёт через детур и не является ногой обёртки — на 1.14 он замолчит",
				s.Tag, s.Type)
		}
	}
}

// Каждое правило DNS обязано указывать на существующий тег. Тег, которого нет
// в списке серверов, — это мёртвый движок, а не предупреждение.
func TestEveryDNSRuleNamesARegisteredServer(t *testing.T) {
	dns := buildDNS(EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{Type: "VLESS", IP: "node.example.com", Port: 443},
	})
	if dns == nil {
		t.Fatal("buildDNS вернул nil")
	}
	tags := map[string]bool{}
	for _, s := range dns.Servers {
		tags[s.Tag] = true
	}
	for i, r := range dns.Rules {
		if r.Server == "" {
			continue
		}
		if !tags[r.Server] {
			t.Errorf("правило %d указывает на сервер %q, которого нет в списке", i, r.Server)
		}
	}
	if dns.Final != "" && !tags[dns.Final] {
		t.Errorf("final указывает на сервер %q, которого нет в списке", dns.Final)
	}
}
