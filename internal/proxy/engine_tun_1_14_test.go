// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import "testing"

func tunInboundFor(t *testing.T, proxyType string) SBInbound {
	t.Helper()
	cfg := EngineConfig{
		Mode: ProxyModeTunnel,
		Proxy: ProxyConfig{
			Type: proxyType,
			IP:   "203.0.113.7",
			Port: 443,
		},
	}
	sb := BuildTunnelModeConfig(cfg)
	for _, in := range sb.Inbounds {
		if in.Type == "tun" {
			return in
		}
	}
	t.Fatalf("в конфиге для %s нет tun-инбаунда", proxyType)
	return SBInbound{}
}

// На 1.13 отсутствие udp_mapping/udp_filtering означало симметричный NAT, на
// 1.14 — endpoint-independent. Поведение обычного узла выбрано осознанно
// (штормы QUIC-ретраев дают меньше слотов при endpoint-independent), поэтому
// оно записано, а не унаследовано.
func TestTunNATBehaviourIsStatedForPlainNodes(t *testing.T) {
	in := tunInboundFor(t, "VLESS")
	if in.UDPMapping != "endpoint_independent" {
		t.Errorf("udp_mapping = %q, ожидалось endpoint_independent", in.UDPMapping)
	}
	if in.UDPFiltering != "endpoint_independent" {
		t.Errorf("udp_filtering = %q, ожидалось endpoint_independent", in.UDPFiltering)
	}
}

// У WG/AWG инбаунд кормит пакетами эндпоинт, который держит своё состояние
// сессии. Здесь сохраняется ровно то поведение, которое ветка имела до 1.14,
// — теперь сказанное вслух, потому что дефолт ядра из-под него ушёл.
func TestTunNATBehaviourIsStatedForWireGuard(t *testing.T) {
	for _, pt := range []string{"WIREGUARD", "AMNEZIAWG"} {
		in := tunInboundFor(t, pt)
		if in.UDPMapping != "address_and_port_dependent" {
			t.Errorf("%s: udp_mapping = %q, ожидалось address_and_port_dependent", pt, in.UDPMapping)
		}
		if in.UDPFiltering != "address_and_port_dependent" {
			t.Errorf("%s: udp_filtering = %q, ожидалось address_and_port_dependent", pt, in.UDPFiltering)
		}
	}
}

// dns_mode в 1.14 по умолчанию hijack — ровно то, на что клиент опирался
// всегда. Записано здесь, чтобы будущий дефолт не сдвинул это молча, как
// сдвинул endpoint_independent_nat. DNSAddress не задаётся намеренно: ядро
// выводит адрес перехвата из адреса TUN, как было до появления опции.
func TestTunDNSModeIsStated(t *testing.T) {
	if in := tunInboundFor(t, "VLESS"); in.DNSMode != "hijack" {
		t.Errorf("dns_mode = %q, ожидалось hijack", in.DNSMode)
	}
}

// Потолок ставится только обычным узлам. WG и AWG держат своё состояние сессии,
// и голодание таблицы инбаунда у них — та же ошибка, что однажды уронила живой
// туннель таймаутом.
func TestUDPNATCeilingIsSetForPlainNodesOnly(t *testing.T) {
	if in := tunInboundFor(t, "VLESS"); in.UDPNATMax != 8192 {
		t.Errorf("udp_nat_max = %d, ожидалось 8192", in.UDPNATMax)
	}
	for _, pt := range []string{"WIREGUARD", "AMNEZIAWG"} {
		if in := tunInboundFor(t, pt); in.UDPNATMax != 0 {
			t.Errorf("%s: udp_nat_max = %d, у эндпоинтов потолок не ставится", pt, in.UDPNATMax)
		}
	}
}

// Стек TUN у WG и AWG был прибит к "system" ещё десктопным коммитом v3.0.0, без
// обоснования под Android; на ПК этой ветки давно нет — стек там общий. На
// sing-tun 0.9 системный стек на телефоне перестал обслуживать TCP: DNS и QUIC
// идут, TCP не открывается вовсе (замер на живом AWG-узле: curl к 1.1.1.1:443 —
// таймаут 15 с, 25 МБ — ноль байт за 60 с, при том что на VLESS через gvisor те
// же 25 МБ качаются за 3.5 с).
func TestWireGuardUsesTheSameTunStackAsEveryoneElse(t *testing.T) {
	plain := tunInboundFor(t, "VLESS").Stack
	if plain != "gvisor" {
		t.Fatalf("обычный узел получил стек %q, тест написан в расчёте на gvisor", plain)
	}
	for _, pt := range []string{"WIREGUARD", "AMNEZIAWG"} {
		if got := tunInboundFor(t, pt).Stack; got != plain {
			t.Errorf("%s: стек TUN = %q, ожидался %q — на системном стеке TCP не открывается", pt, got, plain)
		}
	}
}
