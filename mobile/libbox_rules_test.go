package mobile

import (
	"encoding/json"
	"testing"
)

const rulesTestURI = "vless://11111111-1111-1111-1111-111111111111@1.2.3.4:443?security=tls&type=tcp#rules"

func buildRules(t *testing.T, opts BuildOptions) []map[string]any {
	t.Helper()
	out, err := BuildSingBoxConfigV2(rulesTestURI, t.TempDir(), encodeOptions(opts))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	route, _ := m["route"].(map[string]any)
	raw, _ := route["rules"].([]any)
	rules := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		if rm, ok := r.(map[string]any); ok {
			rules = append(rules, rm)
		}
	}
	return rules
}

// indexOfPackageRule returns the position of the first rule carrying a
// package_name matcher with the given action, or -1.
func indexOfPackageRule(rules []map[string]any, action string) int {
	for i, r := range rules {
		if r["action"] != action {
			continue
		}
		if _, ok := r["package_name"].([]any); ok {
			return i
		}
	}
	return -1
}

func indexOfAction(rules []map[string]any, action string) int {
	for i, r := range rules {
		if r["action"] == action {
			return i
		}
	}
	return -1
}

func TestBlockedAppsEmitRejectRule(t *testing.T) {
	rules := buildRules(t, BuildOptions{BlockedApps: "com.google.android.youtube"})
	i := indexOfPackageRule(rules, "reject")
	if i < 0 {
		t.Fatal("no package_name reject rule emitted")
	}
	pkgs, _ := rules[i]["package_name"].([]any)
	if len(pkgs) != 1 || pkgs[0] != "com.google.android.youtube" {
		t.Fatalf("package_name = %v", pkgs)
	}
}

// The zero-overhead guarantee: sing-box only resolves the connection owner
// when a rule needs it, so empty lists must emit no package_name rule at all.
func TestEmptyListsEmitNoPackageRules(t *testing.T) {
	rules := buildRules(t, BuildOptions{})
	for _, r := range rules {
		if _, ok := r["package_name"]; ok {
			t.Fatalf("package_name rule emitted for empty lists: %v", r)
		}
	}
}

func TestIntoVpnAppsOnlyInSmart(t *testing.T) {
	global := buildRules(t, BuildOptions{IntoVpnApps: "com.discord"})
	if i := indexOfPackageRule(global, "route"); i >= 0 {
		t.Fatalf("into-VPN rule emitted in Global mode: %v", global[i])
	}
	smart := buildRules(t, BuildOptions{SmartMode: true, IntoVpnApps: "com.discord"})
	i := indexOfPackageRule(smart, "route")
	if i < 0 {
		t.Fatal("no into-VPN package rule in Smart mode")
	}
	if smart[i]["outbound"] != "proxy" {
		t.Fatalf("outbound = %v, want proxy", smart[i]["outbound"])
	}
}

func TestExcludedDomainsOnlyInGlobal(t *testing.T) {
	global := buildRules(t, BuildOptions{ExcludedDomains: "yandex.ru"})
	if !hasDomainSuffix(global, "yandex.ru") {
		t.Fatal("excluded domain rule missing in Global mode")
	}
	smart := buildRules(t, BuildOptions{SmartMode: true, ExcludedDomains: "yandex.ru"})
	if hasDomainSuffix(smart, "yandex.ru") {
		t.Fatal("excluded domain rule leaked into Smart mode")
	}
}

func hasDomainSuffix(rules []map[string]any, want string) bool {
	for _, r := range rules {
		sufs, _ := r["domain_suffix"].([]any)
		for _, s := range sufs {
			if s == want {
				return true
			}
		}
	}
	return false
}

// Restrictive-first, and after sniff: user rules must sit behind the sniff /
// hijack-dns / port-853 prologue but ahead of the built-in ad-block and
// Smart-list rules, or a built-in rule would win the match first.
func TestUserRuleOrder(t *testing.T) {
	rules := buildRules(t, BuildOptions{
		SmartMode:   true,
		AdBlock:     true,
		BlockedApps: "com.blocked",
		IntoVpnApps: "com.discord",
	})
	sniff := indexOfAction(rules, "sniff")
	block := indexOfPackageRule(rules, "reject")
	route := indexOfPackageRule(rules, "route")
	if sniff < 0 || block < 0 || route < 0 {
		t.Fatalf("missing rules: sniff=%d block=%d route=%d", sniff, block, route)
	}
	if !(sniff < block && block < route) {
		t.Fatalf("want sniff < block < route, got %d %d %d", sniff, block, route)
	}
	// The port-853 DoT reject must keep firing before an into-VPN rule could
	// send port 853 to the proxy and re-introduce the 5s stall it prevents.
	for i, r := range rules {
		if p, ok := r["port"].([]any); ok && len(p) > 0 && r["action"] == "reject" {
			if i > block {
				t.Fatalf("port-853 reject at %d is behind user rules at %d", i, block)
			}
		}
	}
}

func TestBlockedDomainsEmitRejectInBothModes(t *testing.T) {
	for _, smart := range []bool{false, true} {
		rules := buildRules(t, BuildOptions{SmartMode: smart, BlockedDomains: "ads.example"})
		found := false
		for _, r := range rules {
			if r["action"] != "reject" {
				continue
			}
			sufs, _ := r["domain_suffix"].([]any)
			for _, s := range sufs {
				if s == "ads.example" {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("blocked domain rule missing (smart=%v)", smart)
		}
	}
}
