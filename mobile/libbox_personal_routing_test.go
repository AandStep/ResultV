package mobile

import (
	"encoding/json"
	"testing"

	"resultproxy-wails/internal/proxy"
)

func TestPersonalRulesSupportBothDirectionsInEachMode(t *testing.T) {
	const uri = "vless://11111111-1111-4111-8111-111111111111@203.0.113.7:443?encryption=none#test"
	for _, tc := range []struct{ name, opts string }{
		{"smart", `{"smartMode":true,"bidirectionalRules":true,"excludedDomains":"example.org","intoVpnDomains":"vpn.example.org","intoVpnApps":"com.example.vpn"}`},
		{"global", `{"smartMode":false,"bidirectionalRules":true,"excludedDomains":"example.org","intoVpnDomains":"vpn.example.org","intoVpnApps":"com.example.vpn"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := BuildSingBoxConfigV2(uri, t.TempDir(), tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			var cfg proxy.SingBoxConfig
			if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
				t.Fatal(err)
			}
			var vpn, direct, app int
			for i, r := range cfg.Route.Rules {
				if len(r.DomainSuffix) == 1 && r.DomainSuffix[0] == "vpn.example.org" && r.Outbound == "proxy" {
					vpn = i + 1
				}
				if len(r.DomainSuffix) == 1 && r.DomainSuffix[0] == "example.org" && r.Outbound == "direct" {
					direct = i + 1
				}
				if len(r.PackageName) == 1 && r.PackageName[0] == "com.example.vpn" && r.Outbound == "proxy" {
					app = i + 1
				}
			}
			if vpn == 0 || direct == 0 || app == 0 || vpn > direct {
				t.Fatalf("want package and child VPN override ahead of direct parent: vpn=%d direct=%d app=%d", vpn, direct, app)
			}
		})
	}
}
