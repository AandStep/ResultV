package mobile

import (
	"context"
	"encoding/json"
	"net/netip"
	"testing"

	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
)

const redirectTestURI = "vless://11111111-1111-1111-1111-111111111111@1.2.3.4:443?security=tls&type=tcp#redirect"

func buildRedirectRules(t *testing.T, opts BuildOptions) []map[string]any {
	t.Helper()
	out, err := BuildSingBoxConfigV2(redirectTestURI, t.TempDir(), encodeOptions(opts))
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

// indexOfRedirectRule returns the position of the browser-ad-block redirect
// rule. It is the only rule carrying a destination override, which is what
// makes the redirect work at all — see buildBrowserAdBlockRedirect.
func indexOfRedirectRule(rules []map[string]any) int {
	for i, r := range rules {
		if _, ok := r["override_address"]; ok {
			return i
		}
	}
	return -1
}

func firstString(v any) string {
	list, _ := v.([]any)
	if len(list) == 0 {
		return ""
	}
	s, _ := list[0].(string)
	return s
}

func TestBrowserAdBlockRedirect_Shape(t *testing.T) {
	rules := buildRedirectRules(t, BuildOptions{BrowserAdBlock: true, BrowserAdBlockPort: 8130})
	i := indexOfRedirectRule(rules)
	if i < 0 {
		t.Fatalf("no redirect rule emitted, rules=%v", rules)
	}
	r := rules[i]
	if got := firstString(r["ip_cidr"]); got != BrowserAdBlockProxyHost()+"/32" {
		t.Errorf("ip_cidr = %q, want %q", got, BrowserAdBlockProxyHost()+"/32")
	}
	ports, _ := r["port"].([]any)
	if len(ports) != 1 || ports[0].(float64) != 8130 {
		t.Errorf("port = %v, want [8130]", r["port"])
	}
	if got := firstString(r["network"]); got != "tcp" {
		t.Errorf("network = %v, want [tcp]", r["network"])
	}
	if r["action"] != "route" || r["outbound"] != "direct" {
		t.Errorf("action/outbound = %v/%v, want route/direct", r["action"], r["outbound"])
	}
	if r["override_address"] != "127.0.0.1" {
		t.Errorf("override_address = %v, want 127.0.0.1", r["override_address"])
	}
	if r["override_port"] != float64(8130) {
		t.Errorf("override_port = %v, want 8130", r["override_port"])
	}
}

// A missing port must not emit a rule pointing at port 0: an APK built against
// an older wrapper sends no browserAdBlockPort at all, and a silently dead
// redirect is worse than the documented default.
func TestBrowserAdBlockRedirect_DefaultsPortWhenUnset(t *testing.T) {
	rules := buildRedirectRules(t, BuildOptions{BrowserAdBlock: true})
	i := indexOfRedirectRule(rules)
	if i < 0 {
		t.Fatal("no redirect rule emitted")
	}
	if rules[i]["override_port"] != float64(defaultBrowserAdBlockPort) {
		t.Errorf("override_port = %v, want %d", rules[i]["override_port"], defaultBrowserAdBlockPort)
	}
}

func TestBrowserAdBlockRedirect_AbsentWhenOff(t *testing.T) {
	rules := buildRedirectRules(t, BuildOptions{})
	if i := indexOfRedirectRule(rules); i >= 0 {
		t.Fatalf("redirect rule emitted with BrowserAdBlock off: %v", rules[i])
	}
}

// The whole point of the change: a blocked app must be rejected before its
// traffic is handed to the MITM, and the redirect must win over the into-VPN
// rules — otherwise `domain X -> proxy` would ship the browser's connection to
// the local proxy address off to the remote server.
func TestBrowserAdBlockRedirect_BetweenBlockedAndIntoVpn(t *testing.T) {
	rules := buildRedirectRules(t, BuildOptions{
		SmartMode:      true,
		BrowserAdBlock: true,
		BlockedApps:    "com.blocked",
		IntoVpnApps:    "com.discord",
	})
	block := indexOfPackageRule(rules, "reject")
	redirect := indexOfRedirectRule(rules)
	intoVpn := indexOfPackageRule(rules, "route")
	if block < 0 || redirect < 0 || intoVpn < 0 {
		t.Fatalf("missing rules: block=%d redirect=%d intoVpn=%d", block, redirect, intoVpn)
	}
	if !(block < redirect && redirect < intoVpn) {
		t.Fatalf("want block < redirect < intoVpn, got %d %d %d", block, redirect, intoVpn)
	}
}

// The redirect must also stay ahead of the domain rules that sniff feeds:
// excluded domains route to `direct` by hostname, and the browser's CONNECT to
// the local proxy address sniffs as the *target* host.
func TestBrowserAdBlockRedirect_AheadOfDomainRules(t *testing.T) {
	rules := buildRedirectRules(t, BuildOptions{
		BrowserAdBlock:  true,
		ExcludedDomains: "yandex.ru",
	})
	redirect := indexOfRedirectRule(rules)
	if redirect < 0 {
		t.Fatal("no redirect rule emitted")
	}
	for i, r := range rules {
		sufs, _ := r["domain_suffix"].([]any)
		if len(sufs) > 0 && i < redirect {
			t.Fatalf("domain rule at %d sits ahead of the redirect at %d: %v", i, redirect, r)
		}
	}
}

// Kill-switch panic blackholes app traffic. The redirect carries an ip_cidr and
// routes to `direct`, which is exactly the shape applyKillSwitch preserves for
// the LAN / server-IP bypass — so it needs an explicit carve-out, or the
// browser would keep reaching the MITM and only fail once its upstream hits
// final=block.
func TestBrowserAdBlockRedirect_RejectedInPanic(t *testing.T) {
	rules := buildRedirectRules(t, BuildOptions{
		BrowserAdBlock:  true,
		KillSwitchArmed: true,
		KillSwitchPanic: true,
	})
	i := indexOfRedirectRule(rules)
	if i >= 0 {
		t.Fatalf("redirect survived panic mode with its override intact: %v", rules[i])
	}
	found := false
	for _, r := range rules {
		if ports, ok := r["port"].([]any); ok && len(ports) == 1 && ports[0] == float64(defaultBrowserAdBlockPort) {
			found = true
			if r["action"] != "reject" {
				t.Errorf("redirect rule in panic: action = %v, want reject", r["action"])
			}
			if _, ok := r["outbound"]; ok {
				t.Errorf("redirect rule in panic still carries an outbound: %v", r)
			}
		}
	}
	if !found {
		t.Fatal("redirect rule vanished in panic mode instead of turning into a reject")
	}
}

// The redirect address must stay inside the TUN prefix and must not be the TUN
// address itself: the kernel answers its own address locally, so a CONNECT to
// it would never enter the tunnel and the app identity would be lost again —
// the exact defect this change fixes.
func TestBrowserAdBlockProxyHost_InsideTunPrefix(t *testing.T) {
	prefix, err := netip.ParsePrefix(mobileTunIPv4)
	if err != nil {
		t.Fatalf("parse tun prefix %q: %v", mobileTunIPv4, err)
	}
	host, err := netip.ParseAddr(BrowserAdBlockProxyHost())
	if err != nil {
		t.Fatalf("parse redirect host %q: %v", BrowserAdBlockProxyHost(), err)
	}
	if !prefix.Contains(host) {
		t.Fatalf("redirect host %s is outside the TUN prefix %s", host, prefix)
	}
	if host == prefix.Addr() {
		t.Fatalf("redirect host %s is the TUN address itself", host)
	}
}

// The pinned core decodes options with DisallowUnknownFields and validates
// action options per action, so a misplaced override_* is a dead engine rather
// than an ignored knob. Both live shapes have to survive that: the redirect as
// emitted, and the reject it becomes under kill-switch panic.
func TestBrowserAdBlockRedirect_CoreAcceptsConfig(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts BuildOptions
	}{
		{"redirect", BuildOptions{BrowserAdBlock: true, BrowserAdBlockPort: 8130}},
		{"panic", BuildOptions{BrowserAdBlock: true, KillSwitchArmed: true, KillSwitchPanic: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := BuildSingBoxConfigV2(redirectTestURI, t.TempDir(), encodeOptions(tc.opts))
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			var parsed option.Options
			ctx := include.Context(context.Background())
			if err := singjson.UnmarshalContext(ctx, []byte(out), &parsed); err != nil {
				t.Fatalf("pinned core rejected the config: %v\nconfig: %s", err, out)
			}
		})
	}
}
