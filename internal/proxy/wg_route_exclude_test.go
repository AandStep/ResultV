// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"slices"
	"testing"
)

// Без исключения собственный UDP узла входит в TUN и выпускается обратно
// правилом маршрутизации, совпадающим с адресом сервера, — каждый байт
// пересекает инбаунд дважды: как полезная нагрузка и как несущий её
// зашифрованный пакет. На 1.13 это тратило работу, на sing-tun 0.9 это душит
// сессию: загрузки не стартуют, мелкие запросы истекают, tcp_established не
// уходит с нуля.
func TestWireGuardServerAddressIsExcludedFromTun(t *testing.T) {
	for _, pt := range []string{"WIREGUARD", "AMNEZIAWG"} {
		in := tunInboundFor(t, pt)
		if !slices.Contains(in.RouteExcludeAddress, "203.0.113.7/32") {
			t.Errorf("%s: route_exclude_address = %v, нет адреса узла 203.0.113.7/32",
				pt, in.RouteExcludeAddress)
		}
	}
}

// Прежнее поведение остальных протоколов не меняется — оно и было правильным.
func TestPlainNodeAddressStaysExcluded(t *testing.T) {
	in := tunInboundFor(t, "VLESS")
	if !slices.Contains(in.RouteExcludeAddress, "203.0.113.7/32") {
		t.Errorf("route_exclude_address = %v, нет адреса узла", in.RouteExcludeAddress)
	}
}
