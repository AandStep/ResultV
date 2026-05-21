//go:build darwin

package system

import (
	"strings"
	"testing"
)

func TestBuildPFRulesAlwaysBlocksByDefault(t *testing.T) {
	got := buildPFRules(nil, nil)
	if !strings.Contains(got, "block out all") {
		t.Fatalf("expected default block: %s", got)
	}
	if strings.Contains(got, "pass out from any to \n") {
		t.Fatalf("empty proxy IP should not produce empty pass rule: %s", got)
	}
}

func TestBuildPFRulesAddsProxyPass(t *testing.T) {
	got := buildPFRules([]string{"1.2.3.4"}, nil)
	if !strings.Contains(got, "pass out from any to 1.2.3.4") {
		t.Fatalf("expected proxy pass rule, got: %s", got)
	}
}

func TestBuildPFRulesAllowsLAN(t *testing.T) {
	got := buildPFRules(nil, []string{"1.1.1.1"})
	for _, want := range []string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"port 53",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in ruleset, got: %s", want, got)
		}
	}
}

// DNS-leak regression: the previous ruleset used "to any port 53". The new
// ruleset must scope DNS to specific resolver IPs.
func TestBuildPFRulesScopesDNSToResolvers(t *testing.T) {
	got := buildPFRules([]string{"1.2.3.4"}, []string{"1.1.1.1", "9.9.9.9"})
	if strings.Contains(got, "to any port 53") {
		t.Fatalf("pf rules must not allow port 53 to any destination, got: %s", got)
	}
	for _, want := range []string{
		"pass out proto udp from any to 1.1.1.1 port 53",
		"pass out proto tcp from any to 1.1.1.1 port 53",
		"pass out proto udp from any to 9.9.9.9 port 53",
		"pass out proto tcp from any to 9.9.9.9 port 53",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in ruleset, got: %s", want, got)
		}
	}
}

func TestBuildPFRulesNoDNSRuleWhenEmpty(t *testing.T) {
	got := buildPFRules([]string{"1.2.3.4"}, nil)
	if strings.Contains(got, "port 53") {
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
