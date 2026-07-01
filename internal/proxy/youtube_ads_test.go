// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import "testing"

func TestBuildDNS_AdBlock_BypassesGooglevideoBeforeReject(t *testing.T) {
	cfg := EngineConfig{Mode: ProxyModeTunnel, AdBlock: true, Proxy: ProxyConfig{Type: "vless"}}
	dns := buildDNS(cfg)
	rejectIdx, gvBypassIdx := -1, -1
	for i, r := range dns.Rules {
		if r.Action == "reject" {
			rejectIdx = i
		}
		for _, sfx := range r.DomainSuffix {
			if sfx == ".googlevideo.com" && r.Server != "" {
				gvBypassIdx = i
			}
		}
	}
	if rejectIdx == -1 || gvBypassIdx == -1 || gvBypassIdx >= rejectIdx {
		t.Fatalf("expected googlevideo DNS bypass before reject, rules=%+v", dns.Rules)
	}
}
