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
	"encoding/json"
	"os"
	"testing"
)

func TestNormalizeRoutingListURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/Yuu518/sing-box-rules/blob/rule_set/rule_set_site/adguard.json": "https://raw.githubusercontent.com/Yuu518/sing-box-rules/rule_set/rule_set_site/adguard.json",
		"https://github.com/o/r/blob/main/a/b.txt#L5":                                        "https://raw.githubusercontent.com/o/r/main/a/b.txt",
		"http://github.com/o/r/blob/main/x.lst?plain=1":                                      "https://raw.githubusercontent.com/o/r/main/x.lst",
		// Already-raw and non-github URLs are untouched.
		"https://raw.githubusercontent.com/o/r/main/x.json": "https://raw.githubusercontent.com/o/r/main/x.json",
		"https://example.com/list.txt":                      "https://example.com/list.txt",
		// GitHub non-blob paths (tree/repo root) are untouched.
		"https://github.com/o/r/tree/main": "https://github.com/o/r/tree/main",
		"https://github.com/o/r":           "https://github.com/o/r",
	}
	for in, want := range cases {
		if got := NormalizeRoutingListURL(in); got != want {
			t.Errorf("NormalizeRoutingListURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLooksLikeRoutingListHTML(t *testing.T) {
	html := []string{
		"<!DOCTYPE html>\n<html lang=\"en\">",
		"<html><head></head></html>",
		"  <!doctype HTML>",
		"<?xml version=\"1.0\"?>",
	}
	for _, h := range html {
		if !LooksLikeRoutingListHTML([]byte(h)) {
			t.Errorf("LooksLikeRoutingListHTML(%q) = false, want true", h)
		}
	}
	notHTML := []string{
		"{\"version\":3,\"rules\":[]}",
		"# comment\nexample.com\n",
		"example.com",
		"1.2.3.0/24",
		"",
	}
	for _, n := range notHTML {
		if LooksLikeRoutingListHTML([]byte(n)) {
			t.Errorf("LooksLikeRoutingListHTML(%q) = true, want false", n)
		}
	}
}

func TestParseRoutingListRejectsHTML(t *testing.T) {
	// A GitHub blob HTML page must yield nothing, not scraped junk "domains".
	html := []byte("<!DOCTYPE html>\n<html>\n<head><title>github</title></head>\n<body>class=\"flex-1\"</body>\n</html>")
	got := ParseRoutingListPayload(html)
	if len(got.Domains) != 0 || len(got.CIDRs) != 0 {
		t.Errorf("HTML must parse to empty, got %+v", got)
	}
}

func TestParseRoutingListDropsJunkTokens(t *testing.T) {
	// Non-HTML text with markup-ish junk lines mixed in: only real domains survive.
	raw := []byte("example.com\nclass=\"flex-1\"\ndata-target=\"x\"\n<div>\nsub.example.org\n")
	got := ParseRoutingListPayload(raw)
	if !hasStr(got.Domains, "example.com") || !hasStr(got.Domains, "sub.example.org") {
		t.Errorf("real domains missing: %v", got.Domains)
	}
	for _, d := range got.Domains {
		if d != "example.com" && d != "sub.example.org" {
			t.Errorf("junk token survived as domain: %q", d)
		}
	}
}

func hasStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func TestParseRoutingListPlainText(t *testing.T) {
	raw := []byte("# comment\nexample.com\n||ads.example.org^\n1.2.3.0/24\n8.8.8.8\n\ncdn.example.com\n")
	got := ParseRoutingListPayload(raw)
	if !hasStr(got.Domains, "example.com") {
		t.Errorf("missing example.com: %v", got.Domains)
	}
	if !hasStr(got.Domains, "ads.example.org") {
		t.Errorf("missing ads.example.org: %v", got.Domains)
	}
	// cdn.example.com is covered by example.com suffix -> compressed away.
	if hasStr(got.Domains, "cdn.example.com") {
		t.Errorf("cdn.example.com should be compressed under example.com: %v", got.Domains)
	}
	if !hasStr(got.CIDRs, "1.2.3.0/24") {
		t.Errorf("missing 1.2.3.0/24: %v", got.CIDRs)
	}
	if !hasStr(got.CIDRs, "8.8.8.8/32") {
		t.Errorf("bare IP not widened to /32: %v", got.CIDRs)
	}
}

func TestParseRoutingListSourceJSON(t *testing.T) {
	raw := []byte(`{"version":3,"rules":[{"domain":["exact.test"],"domain_suffix":["sub.test"],"ip_cidr":["10.0.0.0/8","2001:db8::/32"]}]}`)
	got := ParseRoutingListPayload(raw)
	if !hasStr(got.Domains, "exact.test") || !hasStr(got.Domains, "sub.test") {
		t.Errorf("json domains not parsed: %v", got.Domains)
	}
	if !hasStr(got.CIDRs, "10.0.0.0/8") || !hasStr(got.CIDRs, "2001:db8::/32") {
		t.Errorf("json cidrs not parsed: %v", got.CIDRs)
	}
}

func TestParseRoutingListEmpty(t *testing.T) {
	got := ParseRoutingListPayload([]byte("   \n# only a comment\n"))
	if len(got.Domains) != 0 || len(got.CIDRs) != 0 {
		t.Errorf("expected empty, got %+v", got)
	}
}

func TestParseRoutingListMalformedJSONFallsBackToLines(t *testing.T) {
	// Starts with '{' but is not valid JSON -> must fall back to line parsing.
	raw := []byte("{not valid json\nexample.com\n")
	got := ParseRoutingListPayload(raw)
	if !hasStr(got.Domains, "example.com") {
		t.Errorf("malformed JSON should fall back to line parsing, got %+v", got)
	}
}

func TestParseRoutingListPlainTextIPv6CIDR(t *testing.T) {
	raw := []byte("2001:db8::/32\nexample.net\n")
	got := ParseRoutingListPayload(raw)
	if !hasStr(got.CIDRs, "2001:db8::/32") {
		t.Errorf("IPv6 CIDR line not routed to CIDRs: %v", got.CIDRs)
	}
	if !hasStr(got.Domains, "example.net") {
		t.Errorf("domain missing: %v", got.Domains)
	}
}

func TestWriteRoutingListRuleSet(t *testing.T) {
	dir := t.TempDir()
	p := ParsedRoutingList{Domains: []string{"example.com"}, CIDRs: []string{"1.2.3.0/24"}}
	if err := WriteRoutingListRuleSet(dir, "abc", p); err != nil {
		t.Fatalf("write: %v", err)
	}
	path := RoutingListCachePath(dir, "abc")
	blob, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var f srcRuleSetFile
	if err := json.Unmarshal(blob, &f); err != nil {
		t.Fatalf("cache not valid JSON: %v", err)
	}
	if f.Version != routingListRuleSetVersion {
		t.Errorf("version: got %d, want %d", f.Version, routingListRuleSetVersion)
	}
	if len(f.Rules) != 1 || !hasStr(f.Rules[0].DomainSuffix, "example.com") || !hasStr(f.Rules[0].IPCidr, "1.2.3.0/24") {
		t.Errorf("unexpected cache rules: %+v", f.Rules)
	}
}

func TestWriteRoutingListRuleSetEmptyRejected(t *testing.T) {
	dir := t.TempDir()
	if err := WriteRoutingListRuleSet(dir, "empty", ParsedRoutingList{}); err == nil {
		t.Error("expected error writing empty routing list")
	}
	if _, err := os.Stat(RoutingListCachePath(dir, "empty")); err == nil {
		t.Error("empty list must not create a cache file")
	}
}

func TestRoutingListRuleSetTagStable(t *testing.T) {
	if RoutingListRuleSetTag("abc") != RoutingListRuleSetTag("abc") {
		t.Error("tag not stable")
	}
	if RoutingListRuleSetTag("abc") == RoutingListRuleSetTag("def") {
		t.Error("tags must differ per id")
	}
}
