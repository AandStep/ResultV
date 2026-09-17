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

func proxyOutboundFor(t *testing.T, sb SingBoxConfig) SBOutbound {
	t.Helper()
	for _, out := range sb.Outbounds {
		if out.Tag == "proxy" {
			return out
		}
	}
	t.Fatal("в конфиге нет аутбаунда с тегом proxy")
	return SBOutbound{}
}

func marshalConfigForTest(t *testing.T, sb SingBoxConfig) string {
	t.Helper()
	b, err := json.MarshalIndent(sb, "", "  ")
	if err != nil {
		t.Fatalf("маршалинг конфига: %v", err)
	}
	return string(b)
}

// Узел, адресованный именем, на 1.14 не получает дайлера, пока ему не назван
// резолвер. В туннельном режиме ответ повторяет правило, которое buildDNS уже
// эмитит для того же домена: системный резолвер. Он не должен ехать по
// туннелю — этот туннель и открывается данным вызовом.
func TestDomainAddressedNodeNamesItsResolverInTunnel(t *testing.T) {
	sb := BuildTunnelModeConfig(EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{Type: "VLESS", IP: "node.example.com", Port: 443},
	})
	if got := proxyOutboundFor(t, sb).DomainResolver; got != "local" {
		t.Errorf("domain_resolver = %q, ожидалось local", got)
	}
}

// У литерального адреса резолвить нечего, и ядро вообще не строит
// resolve-дайлер — поле остаётся пустым, иначе оно назвало бы сервер без нужды.
func TestLiteralAddressedNodeNamesNoResolver(t *testing.T) {
	sb := BuildTunnelModeConfig(EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{Type: "VLESS", IP: "203.0.113.7", Port: 443},
	})
	if got := proxyOutboundFor(t, sb).DomainResolver; got != "" {
		t.Errorf("domain_resolver = %q, у литерального адреса поле должно быть пустым", got)
	}
}

// В proxy-режиме нет TUN, системный резолвер никуда не перенаправлен, и
// назвать "local" значило бы отдать домен узла провайдеру открытым текстом.
// Тег — тот, который ядро выбрало бы само: первый зарегистрированный транспорт.
func TestDomainAddressedNodeUsesFirstTransportInProxyMode(t *testing.T) {
	sb := BuildProxyModeConfig(EngineConfig{
		Mode:  ProxyModeProxy,
		Proxy: ProxyConfig{Type: "VLESS", IP: "node.example.com", Port: 443},
	})
	if sb.DNS == nil || len(sb.DNS.Servers) == 0 {
		t.Fatal("в proxy-режиме пустой список серверов DNS — тег назвать не из чего")
	}
	want := sb.DNS.Servers[0].Tag
	if got := proxyOutboundFor(t, sb).DomainResolver; got != want {
		t.Errorf("domain_resolver = %q, ожидалось %q", got, want)
	}
}

// Эндпоинт WireGuard набирает свой адрес сам и упирается в то же требование
// 1.14, что и обычный аутбаунд.
func TestDomainAddressedWireGuardEndpointNamesItsResolver(t *testing.T) {
	sb := BuildTunnelModeConfig(EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{Type: "WIREGUARD", IP: "wg.example.com", Port: 51820},
	})
	if len(sb.Endpoints) == 0 {
		t.Fatal("в конфиге WireGuard нет эндпоинтов")
	}
	if got := sb.Endpoints[0].DomainResolver; got != "local" {
		t.Errorf("domain_resolver эндпоинта = %q, ожидалось local", got)
	}
}

// route.default_domain_resolver не задаётся намеренно. Названный там резолвер
// увёл бы каждый внутренний резолв прямо в этот транспорт, мимо dns.rules, — а
// обход правил и есть то, чем заблокированный домен резолвится через туннель,
// а не через цензурируемый локальный резолвер.
func TestRouteNeverNamesADefaultDomainResolver(t *testing.T) {
	sb := BuildTunnelModeConfig(EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{Type: "VLESS", IP: "node.example.com", Port: 443},
	})
	if strings.Contains(marshalConfigForTest(t, sb), "default_domain_resolver") {
		t.Error("в конфиге появился default_domain_resolver — внутренние резолвы пойдут мимо dns.rules")
	}
}
