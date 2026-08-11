// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package proxy

import (
	"os"
	"testing"
)

func TestNormalizeRule(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"example.com", "example.com"},
		{"*.ru", "ru"},
		{".ru", "ru"},
		{"ru", "ru"},
		{"*.example.com", "example.com"},
		{"https://example.com/path", "example.com"},
		{"http://example.com/", "example.com"},
		{"  Example.COM  ", "example.com"},
		{"**..test..**", "test"},
		{"", ""},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := normalizeRule(tc.input)
			if got != tc.want {
				t.Errorf("normalizeRule(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestIsWhitelisted_SingleMatch(t *testing.T) {
	r := NewRouter()
	
	result := r.IsWhitelisted("avito.ru", []string{".ru"})
	if !result.IsWhitelisted {
		t.Error("expected avito.ru to be whitelisted with rule .ru")
	}
	if len(result.MatchingRules) != 1 {
		t.Errorf("expected 1 matching rule, got %d", len(result.MatchingRules))
	}
}

func TestIsWhitelisted_DoubleMatch(t *testing.T) {
	r := NewRouter()
	
	result := r.IsWhitelisted("avito.ru", []string{".ru", "avito.ru"})
	if result.IsWhitelisted {
		t.Error("expected avito.ru to NOT be whitelisted with 2 matching rules")
	}
	if len(result.MatchingRules) != 2 {
		t.Errorf("expected 2 matching rules, got %d", len(result.MatchingRules))
	}
}

func TestIsWhitelisted_TripleMatch(t *testing.T) {
	r := NewRouter()
	
	result := r.IsWhitelisted("m.avito.ru", []string{".ru", "avito.ru", "m.avito.ru"})
	if !result.IsWhitelisted {
		t.Error("expected m.avito.ru to be whitelisted with 3 matching rules")
	}
	if len(result.MatchingRules) != 3 {
		t.Errorf("expected 3 matching rules, got %d", len(result.MatchingRules))
	}
}

func TestIsWhitelisted_NoMatch(t *testing.T) {
	r := NewRouter()
	result := r.IsWhitelisted("google.com", []string{"yandex.ru", "mail.ru"})
	if result.IsWhitelisted {
		t.Error("expected google.com to NOT be whitelisted")
	}
}

func TestIsWhitelisted_Empty(t *testing.T) {
	r := NewRouter()
	result := r.IsWhitelisted("google.com", nil)
	if result.IsWhitelisted {
		t.Error("expected false for nil whitelist")
	}

	result = r.IsWhitelisted("", []string{"example.com"})
	if result.IsWhitelisted {
		t.Error("expected false for empty hostname")
	}
}

func TestIsWhitelisted_Deduplication(t *testing.T) {
	r := NewRouter()
	
	result := r.IsWhitelisted("example.com", []string{"example.com", "example.com", "example.com"})
	if !result.IsWhitelisted {
		t.Error("expected whitelisted with deduplicated rules (1 unique match)")
	}
	if len(result.MatchingRules) != 1 {
		t.Errorf("expected 1 matching rule after dedup, got %d", len(result.MatchingRules))
	}
}

func TestIsWhitelisted_SubdomainMatch(t *testing.T) {
	r := NewRouter()
	
	result := r.IsWhitelisted("sub.example.com", []string{"example.com"})
	if !result.IsWhitelisted {
		t.Error("expected sub.example.com to match example.com")
	}
}

func TestShouldProxy_GlobalMode(t *testing.T) {
	r := NewRouter()
	whitelist := []string{"localhost", "127.0.0.1"}

	useProxy, _ := r.ShouldProxy("google.com", ModeGlobal, whitelist)
	if !useProxy {
		t.Error("global mode: non-whitelisted domain should use proxy")
	}

	useProxy, _ = r.ShouldProxy("localhost", ModeGlobal, whitelist)
	if useProxy {
		t.Error("global mode: whitelisted domain should bypass")
	}
}

func TestShouldProxy_SmartMode(t *testing.T) {
	r := NewRouter()
	whitelist := []string{"localhost", "127.0.0.1"}

	
	useProxy, reason := r.ShouldProxy("discord.com", ModeSmart, whitelist)
	if !useProxy {
		t.Errorf("smart mode: blocked domain should use proxy, reason: %s", reason)
	}

	
	useProxy, reason = r.ShouldProxy("google.com", ModeSmart, whitelist)
	if useProxy {
		t.Errorf("smart mode: non-blocked domain should go direct, reason: %s", reason)
	}

	
	useProxy, _ = r.ShouldProxy("localhost", ModeSmart, whitelist)
	if useProxy {
		t.Error("smart mode: whitelisted and non-blocked should bypass")
	}
}

func TestShouldProxy_SmartMode_NestedExceptions(t *testing.T) {
	r := NewRouter()
	
	
	whitelist := []string{".ru", "avito.ru"}

	useProxy, reason := r.ShouldProxy("avito.ru", ModeSmart, whitelist)
	if !useProxy {
		t.Errorf("smart mode: nested exception should proxy, reason: %s", reason)
	}
}

func TestIsBlockedDomain(t *testing.T) {
	r := NewRouter()

	if !r.IsBlockedDomain("discord.com") {
		t.Error("discord.com should be blocked")
	}
	if !r.IsBlockedDomain("cdn.discord.com") {
		t.Error("cdn.discord.com should be blocked (suffix match)")
	}
	if r.IsBlockedDomain("google.com") {
		t.Error("google.com should not be blocked")
	}
	if r.IsBlockedDomain("") {
		t.Error("empty hostname should not be blocked")
	}
}

func TestGetSafeOSWhitelist(t *testing.T) {
	r := NewRouter()

	
	safe := r.GetSafeOSWhitelist([]string{".ru"})
	if len(safe) != 1 || safe[0] != "ru" {
		t.Errorf("expected [ru], got %v", safe)
	}

	
	
	safe = r.GetSafeOSWhitelist([]string{".ru", "avito.ru"})
	if len(safe) != 0 {
		t.Errorf("expected no safe domains with conflicting rules, got %v", safe)
	}
}

func TestGetSafeOSWhitelist_NonConflicting(t *testing.T) {
	r := NewRouter()
	
	safe := r.GetSafeOSWhitelist([]string{"example.com", "test.org"})
	if len(safe) != 2 {
		t.Errorf("expected 2 safe domains, got %v", safe)
	}
}

func TestLoadBlockedLists(t *testing.T) {
	r := NewRouter()
	initial := len(r.GetBlockedDomains())

	
	tmpFile := t.TempDir() + "/test-blocked.txt"
	content := "# comment\nnewblocked.com\nanotherblocked.org\n"
	if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	r.LoadBlockedLists(tmpFile)
	after := len(r.GetBlockedDomains())

	if after != initial+2 {
		t.Errorf("expected %d blocked domains after load, got %d", initial+2, after)
	}
	if !r.IsBlockedDomain("newblocked.com") {
		t.Error("newblocked.com should be blocked after loading")
	}
}

func TestNormalizeCIDRs(t *testing.T) {
	in := []string{
		"149.154.160.0/20",
		"149.154.160.0/20", // duplicate
		"91.108.4.0/22",
		"1.2.3.4",     // bare IPv4 → /32
		"2a0a:f280::/32",
		"::1",         // bare IPv6 → /128
		"  ",          // blank
		"# comment",   // comment
		"not-a-cidr",  // invalid
		"10.0.0.0/33", // invalid prefix
	}
	got := normalizeCIDRs(in)
	want := map[string]bool{
		"149.154.160.0/20": true,
		"91.108.4.0/22":    true,
		"1.2.3.4/32":       true,
		"2a0a:f280::/32":   true,
		"::1/128":          true,
	}
	if len(got) != len(want) {
		t.Fatalf("normalizeCIDRs len = %d (%v), want %d", len(got), got, len(want))
	}
	for _, c := range got {
		if !want[c] {
			t.Errorf("unexpected cidr %q in %v", c, got)
		}
	}
}

func TestDefaultBlockedCIDRs_ValidAndContainsTelegram(t *testing.T) {
	raw := defaultBlockedCIDRs()
	got := normalizeCIDRs(raw)
	if len(got) != len(raw) {
		t.Fatalf("all default CIDRs must be valid: raw=%d normalized=%d (%v)", len(raw), len(got), got)
	}
	var found bool
	for _, c := range got {
		if c == "149.154.160.0/20" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected core Telegram subnet 149.154.160.0/20 in defaults, got %v", got)
	}
}

func TestNewRouter_SeedsBlockedCIDRs(t *testing.T) {
	r := NewRouter()
	if len(r.GetBlockedCIDRs()) == 0 {
		t.Fatal("NewRouter must seed default blocked CIDRs (Telegram)")
	}
}

func TestSetGetBlockedCIDRs(t *testing.T) {
	r := NewRouter()
	r.SetBlockedCIDRs([]string{"91.108.4.0/22", "bad", "5.6.7.8"})
	got := r.GetBlockedCIDRs()
	if len(got) != 2 {
		t.Fatalf("expected 2 valid cidrs (subnet + widened host), got %v", got)
	}
}

func TestSetBlockedDomains(t *testing.T) {
	r := NewRouter()
	r.SetBlockedDomains([]string{"Example.com", "example.com", "*.Test.org", "  "})

	if !r.IsBlockedDomain("api.example.com") {
		t.Error("example.com should be blocked after set")
	}
	if !r.IsBlockedDomain("sub.test.org") {
		t.Error("test.org should be blocked after set")
	}
	got := r.GetBlockedDomains()
	if len(got) != 2 {
		t.Fatalf("expected 2 normalized domains, got %d (%v)", len(got), got)
	}
}

func TestIsBlockedDomainSuffixSemantics(t *testing.T) {
	r := NewRouter()
	r.SetBlockedDomains([]string{"x.com", "discord.com"})

	cases := []struct {
		host string
		want bool
	}{
		{"x.com", true},                // точное совпадение
		{"api.x.com", true},            // сабдомен
		{"cdn.discord.com", true},      // сабдомен
		{"netflix.com", false},         // "x.com" — подстрока, но НЕ суффикс
		{"notdiscord.com", false},      // суффикс-строка без точки — не матч
		{"discord.com.evil.ru", false}, // блок-домен в середине — не матч
		{"", false},
	}
	for _, c := range cases {
		if got := r.IsBlockedDomain(c.host); got != c.want {
			t.Errorf("IsBlockedDomain(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

func TestCustomBlockedDomains(t *testing.T) {
	r := NewRouter()
	r.SetBlockedDomains([]string{"discord.com"})
	r.SetCustomBlockedDomains([]string{"MySite.RU", "discord.com"}) // дубль + регистр

	if !r.IsBlockedDomain("mysite.ru") {
		t.Error("custom domain must be blocked")
	}
	if !r.IsBlockedDomain("cdn.mysite.ru") {
		t.Error("custom domain suffix must be blocked")
	}
	all := r.GetBlockedDomains()
	count := 0
	for _, d := range all {
		if d == "discord.com" {
			count++
		}
		if d == "mysite.ru" {
			count += 10
		}
	}
	if count != 11 { // ровно один discord.com и один mysite.ru
		t.Errorf("union wrong: %v", all)
	}

	r.SetCustomBlockedDomains(nil) // очистка отключает кастом
	if r.IsBlockedDomain("mysite.ru") {
		t.Error("cleared custom domain must not be blocked")
	}
}

// TestNormalizeCIDRs_DropsOverBroadIPv4 is the regression for a real poisoned
// cache: Re:filter's discord_ips.lst shipped 104.16.0.0/12 — all of Cloudflare —
// and Smart mode's ip_cidr rule dragged every Cloudflare-hosted service into the
// tunnel. defaultBlockedCIDRSources documents the invariant; this enforces it.
func TestNormalizeCIDRs_DropsOverBroadIPv4(t *testing.T) {
	got := normalizeCIDRs([]string{
		"104.16.0.0/12",  // all of Cloudflare — must be dropped
		"194.221.0.0/16", // Telegram — the widest legitimate entry
		"5.28.192.0/18",  // Telegram
		"2a0a:f280::/32", // Telegram IPv6 — must survive, v6 is not filtered
	})

	has := make(map[string]bool, len(got))
	for _, c := range got {
		has[c] = true
	}

	if has["104.16.0.0/12"] {
		t.Fatalf("over-broad IPv4 prefix survived: %v", got)
	}
	for _, want := range []string{"194.221.0.0/16", "5.28.192.0/18", "2a0a:f280::/32"} {
		if !has[want] {
			t.Fatalf("legitimate cidr %q was dropped: %v", want, got)
		}
	}
}
