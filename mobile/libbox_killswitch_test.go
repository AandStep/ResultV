package mobile

import (
	"encoding/json"
	"strings"
	"testing"
)

const ksTestURI = "vless://11111111-1111-1111-1111-111111111111@1.2.3.4:443?security=tls&type=tcp#ks"

func buildKS(t *testing.T, armed, panic bool) map[string]any {
	t.Helper()
	out, err := BuildSingBoxConfigV2(ksTestURI, t.TempDir(), encodeOptions(BuildOptions{
		KillSwitchArmed: armed,
		KillSwitchPanic: panic,
	}))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

func routeFinal(m map[string]any) string {
	r, _ := m["route"].(map[string]any)
	f, _ := r["final"].(string)
	return f
}

func hasUrltestGroup(m map[string]any) bool {
	obs, _ := m["outbounds"].([]any)
	for _, o := range obs {
		ob, _ := o.(map[string]any)
		if ob["type"] == "urltest" && ob["tag"] == "ks-test" {
			return true
		}
	}
	return false
}

func TestKillSwitchArmed_AddsGroupAndFinal(t *testing.T) {
	m := buildKS(t, true, false)
	if !hasUrltestGroup(m) {
		t.Fatal("armed: expected ks-test urltest group")
	}
	if got := routeFinal(m); got != "ks-test" {
		t.Fatalf("armed: route.final = %q, want ks-test", got)
	}
}

func TestKillSwitchPanic_FinalBlock(t *testing.T) {
	m := buildKS(t, true, true)
	if !hasUrltestGroup(m) {
		t.Fatal("panic: expected ks-test urltest group to remain for probing")
	}
	if got := routeFinal(m); got != "block" {
		t.Fatalf("panic: route.final = %q, want block", got)
	}
}

func TestKillSwitchOff_NoGroup(t *testing.T) {
	m := buildKS(t, false, false)
	if hasUrltestGroup(m) {
		t.Fatal("off: ks-test group must be absent")
	}
	if got := routeFinal(m); got != "proxy" {
		t.Fatalf("off: route.final = %q, want proxy", got)
	}
}

// TestKillSwitchArmed_SmartModeKeepsDirectFinal is a regression test for a
// defect where applyKillSwitch unconditionally overwrote route.final to
// "ks-test" for the armed-but-not-engaged state. In Smart mode buildRoute
// sets final="direct" (only listed domains hit the proxy, everything else
// goes direct); clobbering that to "ks-test" (which resolves to the proxy)
// forced ALL fall-through traffic through the proxy whenever the kill
// switch was armed, defeating split tunneling. The fix only takes over
// route.final when it was the global "proxy" default.
func TestKillSwitchArmed_SmartModeKeepsDirectFinal(t *testing.T) {
	out, err := BuildSingBoxConfigV2(ksTestURI, t.TempDir(), encodeOptions(BuildOptions{
		KillSwitchArmed:         true,
		SmartMode:               true,
		SmartBlockedDomainsList: "example.com",
	}))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !hasUrltestGroup(m) {
		t.Fatal("armed+smart: expected ks-test urltest group")
	}
	if got := routeFinal(m); got != "direct" {
		t.Fatalf("armed+smart: route.final = %q, want direct (split tunnel must be preserved)", got)
	}
}

// lanBypassCIDRsForTest mirrors the ranges returned by the unexported
// proxy.lanBypassCIDRs() (internal/proxy/engine.go). It is unexported and
// lives in a different package, so the mobile package test can't call it
// directly; these literals are copied from reading that function so the
// assertion below is tied to the actual LAN bypass ranges rather than a
// generic "any ip_cidr rule survived" check.
var lanBypassCIDRsForTest = []string{
	"10.0.0.0/8",
	"172.16.0.0/15",
	"172.18.0.0/16",
	"172.20.0.0/14",
	"172.24.0.0/13",
	"192.168.0.0/16",
	"169.254.0.0/16",
	"224.0.0.0/4",
	"255.255.255.255/32",
	"fc00::/7",
	"fe80::/10",
	"ff00::/8",
}

// TestKillSwitchPanic_PreservesLANBypassBlocksExcluded is a regression test
// for a defect where the panic-mode loop in applyKillSwitch rewrote EVERY
// rule with Outbound "proxy"/"direct" to reject, including the bypass-LAN
// rule (IPCidr: lanBypassCIDRs(), Outbound: "direct") and the
// proxy-server-IP bypass rule. The old comment claimed "LAN bypass is
// preserved by buildRoute's own LAN handling" — false, LAN bypass is a
// plain route rule with no special-casing elsewhere. The spec requires LAN
// to stay reachable in panic (matching the desktop firewall's RFC1918
// allowance) and the proxy endpoint to stay reachable for recovery
// detection, while domain-based excluded-domain rules must still be
// rejected.
func TestKillSwitchPanic_PreservesLANBypassBlocksExcluded(t *testing.T) {
	out, err := BuildSingBoxConfigV2(ksTestURI, t.TempDir(), encodeOptions(BuildOptions{
		KillSwitchArmed: true,
		KillSwitchPanic: true,
		BypassLAN:       true,
		ExcludedDomains: "excluded.example",
	}))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	route, _ := m["route"].(map[string]any)
	rules, _ := route["rules"].([]any)
	if len(rules) == 0 {
		t.Fatal("panic: expected route.rules to be non-empty")
	}

	lanCIDRSet := make(map[string]struct{}, len(lanBypassCIDRsForTest))
	for _, c := range lanBypassCIDRsForTest {
		lanCIDRSet[c] = struct{}{}
	}

	foundLANBypass := false
	foundExcludedReject := false
	for _, ri := range rules {
		r, _ := ri.(map[string]any)
		outbound, _ := r["outbound"].(string)
		action, _ := r["action"].(string)

		if ipCidrRaw, ok := r["ip_cidr"].([]any); ok && outbound == "direct" {
			for _, c := range ipCidrRaw {
				if cs, ok := c.(string); ok {
					if _, isLAN := lanCIDRSet[cs]; isLAN {
						foundLANBypass = true
					}
				}
			}
		}

		matchesExcluded := false
		if domSuffix, ok := r["domain_suffix"].([]any); ok {
			for _, d := range domSuffix {
				if ds, ok := d.(string); ok && strings.Contains(ds, "excluded.example") {
					matchesExcluded = true
				}
			}
		}
		if dom, ok := r["domain"].([]any); ok {
			for _, d := range dom {
				if ds, ok := d.(string); ok && strings.Contains(ds, "excluded.example") {
					matchesExcluded = true
				}
			}
		}
		if matchesExcluded {
			if action == "reject" && outbound == "" {
				foundExcludedReject = true
			}
		}
	}

	if !foundLANBypass {
		t.Fatal("panic: expected a still-direct ip_cidr rule containing a lanBypassCIDRs() range (LAN bypass must survive panic mode)")
	}
	if !foundExcludedReject {
		t.Fatal("panic: expected the excluded-domain rule to be rewritten to action=reject with no outbound")
	}
}
