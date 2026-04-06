package proxy

import "testing"

func TestBuildRoute_NestedDomainException_ProducesProxyOverride(t *testing.T) {
	cfg := EngineConfig{
		RoutingMode: ModeWhitelist,
		Whitelist:   []string{".ru", "2ip.ru"},
	}

	route := buildRoute(cfg)
	if route == nil {
		t.Fatal("expected non-nil route")
	}

	var ruDirect bool
	var twoIPProxy bool
	var twoIPRuleIndex = -1
	var ruRuleIndex = -1

	for i, r := range route.Rules {
		if len(r.DomainSuffix) != 1 {
			continue
		}
		switch r.DomainSuffix[0] {
		case "ru":
			if r.Outbound == "direct" {
				ruDirect = true
				ruRuleIndex = i
			}
		case "2ip.ru":
			if r.Outbound == "proxy" {
				twoIPProxy = true
				twoIPRuleIndex = i
			}
		}
	}

	if !ruDirect {
		t.Fatalf("expected direct rule for ru suffix, rules=%+v", route.Rules)
	}
	if !twoIPProxy {
		t.Fatalf("expected proxy override rule for 2ip.ru suffix, rules=%+v", route.Rules)
	}
	if twoIPRuleIndex > ruRuleIndex {
		t.Fatalf("expected more specific rule (2ip.ru) before ru: twoIP=%d ru=%d", twoIPRuleIndex, ruRuleIndex)
	}
}
