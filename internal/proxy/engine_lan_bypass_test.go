// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"net"
	"testing"
)

func TestLanBypassCIDRs_ExcludesEngineTUNDNS(t *testing.T) {
	tunDNS := net.ParseIP("172.19.0.2")
	if lanBypassContainsIP(tunDNS) {
		t.Fatal("172.19.0.2 (in-tunnel DNS) must not match LAN bypass — breaks DNS hijack")
	}
	homeLAN := net.ParseIP("192.168.1.1")
	if !lanBypassContainsIP(homeLAN) {
		t.Fatal("192.168.1.1 should still match LAN bypass")
	}
	corpLAN := net.ParseIP("172.16.0.1")
	if !lanBypassContainsIP(corpLAN) {
		t.Fatal("172.16.0.1 should still match LAN bypass")
	}
}

func TestBuildRoute_BypassLAN_UsesCarvedOutPrivateRanges(t *testing.T) {
	route := buildRoute(EngineConfig{Mode: ProxyModeTunnel, BypassLAN: true})
	var lanRule *SBRouteRule
	for i := range route.Rules {
		if route.Rules[i].Outbound == "direct" && len(route.Rules[i].IPCidr) > 0 {
			lanRule = &route.Rules[i]
			break
		}
	}
	if lanRule == nil {
		t.Fatal("expected a LAN bypass ip_cidr rule")
	}
	has17216Whole := false
	for _, c := range lanRule.IPCidr {
		if c == "172.16.0.0/12" {
			has17216Whole = true
		}
	}
	if has17216Whole {
		t.Fatalf("LAN bypass must not use bare 172.16.0.0/12 (covers TUN DNS), got %+v", lanRule.IPCidr)
	}
}
