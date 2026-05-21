//go:build linux

package system

import (
	"strings"
	"testing"
)

func TestBuildNftablesRulesetDefaultDrop(t *testing.T) {
	got := buildNftablesRuleset(nil, nil)
	if !strings.Contains(got, "policy drop;") {
		t.Fatalf("expected default-drop policy, got: %s", got)
	}
	if !strings.Contains(got, "table inet "+nftTableName) {
		t.Fatalf("expected table declaration, got: %s", got)
	}
}

func TestBuildNftablesRulesetAddsIPv4Proxy(t *testing.T) {
	got := buildNftablesRuleset([]string{"1.2.3.4"}, nil)
	if !strings.Contains(got, "ip daddr 1.2.3.4 accept") {
		t.Fatalf("expected ipv4 proxy accept, got: %s", got)
	}
	if strings.Contains(got, "ip6 daddr 1.2.3.4") {
		t.Fatalf("ipv4 should not be emitted as ip6, got: %s", got)
	}
}

func TestBuildNftablesRulesetAddsIPv6Proxy(t *testing.T) {
	got := buildNftablesRuleset([]string{"::1"}, nil)
	if !strings.Contains(got, "ip6 daddr ::1 accept") {
		t.Fatalf("expected ipv6 proxy accept, got: %s", got)
	}
}

func TestBuildNftablesRulesetAllowsLAN(t *testing.T) {
	got := buildNftablesRuleset(nil, []string{"1.1.1.1"})
	for _, want := range []string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"oif lo accept",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in ruleset, got: %s", want, got)
		}
	}
}

// DNS-leak regression: the previous ruleset accepted udp/tcp dport 53 to ANY
// destination. The new ruleset must scope port 53 to the explicit resolver
// list, otherwise queries leak to ISP DNS when the tunnel drops.
func TestBuildNftablesRulesetScopesDNSToResolvers(t *testing.T) {
	got := buildNftablesRuleset([]string{"1.2.3.4"}, []string{"1.1.1.1", "9.9.9.9"})

	// Must not contain the old unrestricted DNS rule.
	for _, forbidden := range []string{
		"\t\tudp dport 53 accept\n",
		"\t\ttcp dport 53 accept\n",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("ruleset must not allow port 53 to any destination, got: %s", got)
		}
	}
	// Must contain per-resolver rules for both protocols.
	for _, want := range []string{
		"ip daddr { 1.1.1.1, 9.9.9.9 } udp dport 53 accept",
		"ip daddr { 1.1.1.1, 9.9.9.9 } tcp dport 53 accept",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in ruleset, got: %s", want, got)
		}
	}
}

func TestBuildNftablesRulesetSeparatesIPv4AndIPv6DNS(t *testing.T) {
	got := buildNftablesRuleset(nil, []string{"1.1.1.1", "2606:4700:4700::1111"})
	if !strings.Contains(got, "ip daddr { 1.1.1.1 } udp dport 53 accept") {
		t.Errorf("expected v4 dns rule, got: %s", got)
	}
	if !strings.Contains(got, "ip6 daddr { 2606:4700:4700::1111 } udp dport 53 accept") {
		t.Errorf("expected v6 dns rule, got: %s", got)
	}
}

func TestBuildNftablesRulesetNoDNSRulesWhenEmpty(t *testing.T) {
	got := buildNftablesRuleset([]string{"1.2.3.4"}, nil)
	if strings.Contains(got, "dport 53") {
		t.Fatalf("empty dnsIPs must produce no port-53 rule, got: %s", got)
	}
}

func TestExtractValidIP(t *testing.T) {
	cases := map[string]string{
		"1.2.3.4:8080":     "1.2.3.4",
		"127.0.0.1:7890":   "127.0.0.1",
		"[::1]:80":         "::1",
		"":                 "",
		"not-an-ip:1":      "",
		"example.com:1234": "",
	}
	for in, want := range cases {
		if got := extractValidIP(in); got != want {
			t.Errorf("extractValidIP(%q) = %q, want %q", in, got, want)
		}
	}
}
