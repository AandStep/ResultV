// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"encoding/binary"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

// --- fixture builders ----------------------------------------------------
//
// The real databases are hundreds of kilobytes of third-party data; keeping a
// copy in the tree would make these tests depend on someone else's release
// schedule. The wire format is small enough to write by hand, so fixtures are
// built here and every field a reader might trip over is exercised on purpose.

func pbVarint(field int, v uint64) []byte {
	out := binary.AppendUvarint(nil, uint64(field)<<3|wireVarint)
	return binary.AppendUvarint(out, v)
}

func pbBytes(field int, payload []byte) []byte {
	out := binary.AppendUvarint(nil, uint64(field)<<3|wireBytes)
	out = binary.AppendUvarint(out, uint64(len(payload)))
	return append(out, payload...)
}

func pbString(field int, s string) []byte { return pbBytes(field, []byte(s)) }

// geoDomainMsg builds a Domain message. Passing writeType=false omits the type
// field entirely, which is how real files encode Plain.
func geoDomainMsg(kind uint64, value string, writeType bool) []byte {
	var out []byte
	if writeType {
		out = append(out, pbVarint(1, kind)...)
	}
	return append(out, pbString(2, value)...)
}

func geoSiteMsg(name string, domains ...[]byte) []byte {
	out := pbString(1, name)
	for _, d := range domains {
		out = append(out, pbBytes(2, d)...)
	}
	return out
}

func geoSiteListMsg(sites ...[]byte) []byte {
	var out []byte
	for _, s := range sites {
		out = append(out, pbBytes(1, s)...)
	}
	return out
}

func geoCIDRMsg(ip []byte, prefix uint64, writePrefix bool) []byte {
	out := pbBytes(1, ip)
	if writePrefix {
		out = append(out, pbVarint(2, prefix)...)
	}
	return out
}

func geoIPMsg(name string, inverse bool, cidrs ...[]byte) []byte {
	out := pbString(1, name)
	for _, c := range cidrs {
		out = append(out, pbBytes(2, c)...)
	}
	if inverse {
		out = append(out, pbVarint(3, 1)...)
	}
	return out
}

func geoIPListMsg(entries ...[]byte) []byte {
	var out []byte
	for _, e := range entries {
		out = append(out, pbBytes(1, e)...)
	}
	return out
}

// --- geosite -------------------------------------------------------------

func TestParseGeoSiteDatKindsAndCase(t *testing.T) {
	raw := geoSiteListMsg(
		geoSiteMsg("WHITELIST",
			geoDomainMsg(geoDomainDomain, "Example.COM", true), // upper-cased on purpose
			geoDomainMsg(geoDomainFull, "exact.example.org", true),
			geoDomainMsg(geoDomainRegex, ".*ads.*", true),
			geoDomainMsg(geoDomainPlain, "tracker", true),
			geoDomainMsg(0, "implicit-plain", false), // type omitted => Plain
		),
	)

	cats, dropped, err := ParseGeoSiteDat(raw)
	if err != nil {
		t.Fatalf("ParseGeoSiteDat: %v", err)
	}
	// The file stores WHITELIST, rules say geosite:whitelist.
	got, ok := cats["whitelist"]
	if !ok {
		t.Fatalf("category not lower-cased, got keys %v", keysOf(cats))
	}
	want := []GeoDomain{
		{Value: "example.com"},
		{Value: "exact.example.org", Exact: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("domains = %+v, want %+v", got, want)
	}
	// regex + two plain entries cannot be expressed and must be counted.
	if dropped != 3 {
		t.Fatalf("dropped = %d, want 3", dropped)
	}
}

func TestParseGeoSiteDatMergesRepeatedCategory(t *testing.T) {
	raw := geoSiteListMsg(
		geoSiteMsg("RU", geoDomainMsg(geoDomainDomain, "a.ru", true)),
		geoSiteMsg("ru", geoDomainMsg(geoDomainDomain, "b.ru", true)),
	)
	cats, _, err := ParseGeoSiteDat(raw)
	if err != nil {
		t.Fatalf("ParseGeoSiteDat: %v", err)
	}
	if len(cats["ru"]) != 2 {
		t.Fatalf("repeated category replaced instead of merged: %+v", cats["ru"])
	}
}

func TestParseGeoSiteDatRejectsGarbage(t *testing.T) {
	for name, raw := range map[string][]byte{
		"empty":           {},
		"truncated":       {0x0a, 0x40, 0x01}, // length 64, 1 byte left
		"zero field":      {0x02, 0x01, 0x61}, // field number 0
		"not a geo file":  []byte("<!doctype html>"),
		"varint overflow": {0x0a, 0x02, 0x08, 0xff},
	} {
		if _, _, err := ParseGeoSiteDat(raw); err == nil {
			t.Errorf("%s: expected an error, got none", name)
		}
	}
}

func TestParseGeoSiteDatMalformedIsTyped(t *testing.T) {
	_, _, err := ParseGeoSiteDat([]byte{0x0a, 0x40, 0x01})
	if !errors.Is(err, ErrGeoDatMalformed) {
		t.Fatalf("err = %v, want ErrGeoDatMalformed", err)
	}
}

// --- geoip ---------------------------------------------------------------

func TestParseGeoIPDatAddressForms(t *testing.T) {
	v4 := []byte{10, 0, 0, 0}
	v6 := []byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	// 16 bytes carrying a v4-mapped address: ::ffff:8.8.8.8 with prefix 32.
	mapped := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 8, 8, 8, 8}
	host := []byte{1, 2, 3, 4}

	raw := geoIPListMsg(
		geoIPMsg("PRIVATE", false,
			geoCIDRMsg(v4, 8, true),
			geoCIDRMsg(v6, 32, true),
			geoCIDRMsg(mapped, 32, true),
			geoCIDRMsg(host, 0, false), // no prefix => single host
		),
	)

	cats, inverted, err := ParseGeoIPDat(raw)
	if err != nil {
		t.Fatalf("ParseGeoIPDat: %v", err)
	}
	if len(inverted) != 0 {
		t.Fatalf("unexpected inverted categories: %v", inverted)
	}
	want := []string{"10.0.0.0/8", "2001:db8::/32", "8.8.8.8/32", "1.2.3.4/32"}
	if !reflect.DeepEqual(cats["private"], want) {
		t.Fatalf("cidrs = %v, want %v", cats["private"], want)
	}
}

func TestParseGeoIPDatDropsInverseMatch(t *testing.T) {
	raw := geoIPListMsg(
		geoIPMsg("CN", true, geoCIDRMsg([]byte{1, 0, 0, 0}, 8, true)),
		geoIPMsg("PRIVATE", false, geoCIDRMsg([]byte{10, 0, 0, 0}, 8, true)),
	)
	cats, inverted, err := ParseGeoIPDat(raw)
	if err != nil {
		t.Fatalf("ParseGeoIPDat: %v", err)
	}
	if _, ok := cats["cn"]; ok {
		// Importing the listed prefixes would mean the exact opposite of
		// "everything except China".
		t.Fatal("inverse-match category was imported as a normal one")
	}
	if len(inverted) != 1 || inverted[0] != "cn" {
		t.Fatalf("inverted = %v, want [cn]", inverted)
	}
	if len(cats["private"]) != 1 {
		t.Fatal("a normal category next to an inverted one was lost")
	}
}

func TestParseGeoIPDatSkipsBadAddresses(t *testing.T) {
	raw := geoIPListMsg(
		geoIPMsg("X", false,
			geoCIDRMsg([]byte{1, 2, 3}, 8, true),      // 3 bytes: neither v4 nor v6
			geoCIDRMsg([]byte{10, 0, 0, 0}, 99, true), // prefix beyond 32
			geoCIDRMsg([]byte{10, 0, 0, 0}, 8, true),  // the only good one
		),
	)
	cats, _, err := ParseGeoIPDat(raw)
	if err != nil {
		t.Fatalf("ParseGeoIPDat: %v", err)
	}
	if !reflect.DeepEqual(cats["x"], []string{"10.0.0.0/8"}) {
		t.Fatalf("cidrs = %v, want [10.0.0.0/8]", cats["x"])
	}
}

// --- resolver ------------------------------------------------------------

func testDatabases() GeoDatabases {
	return GeoDatabases{
		Sites: map[string][]GeoDomain{
			"whitelist": {{Value: "gosuslugi.ru"}, {Value: "nalog.ru", Exact: true}},
			"ads":       {{Value: "doubleclick.net"}},
		},
		IPs: map[string][]string{
			"private": {"10.0.0.0/8", "192.168.0.0/16"},
		},
		InvertedIPs: map[string]struct{}{"cn": {}},
		SiteDropped: 7,
	}
}

func TestResolveGeoTokensExpandsCategories(t *testing.T) {
	got, report := ResolveGeoTokens(
		[]string{"geosite:whitelist", "geoip:private"}, testDatabases())

	if !reflect.DeepEqual(got.Domains, []string{"gosuslugi.ru"}) {
		t.Fatalf("domains = %v", got.Domains)
	}
	// `full:` must stay exact — as a suffix it would swallow every sub-domain.
	if !reflect.DeepEqual(got.ExactDomains, []string{"nalog.ru"}) {
		t.Fatalf("exact = %v, want [nalog.ru]", got.ExactDomains)
	}
	if !reflect.DeepEqual(got.CIDRs, []string{"10.0.0.0/8", "192.168.0.0/16"}) {
		t.Fatalf("cidrs = %v", got.CIDRs)
	}
	if len(report.Unresolved) != 0 {
		t.Fatalf("unexpected unresolved: %v", report.Unresolved)
	}
	// The count from the database read must survive into the resolve report,
	// otherwise nothing ever tells the user entries were skipped.
	if report.DroppedFromDB != 7 {
		t.Fatalf("DroppedFromDB = %d, want 7", report.DroppedFromDB)
	}
}

func TestResolveGeoTokensReportsWhatItCannotDo(t *testing.T) {
	tokens := []string{
		"geosite:nosuch",
		"geosite:google@ads",
		"geosite:!ru",
		"geoip:nosuch",
		"geoip:cn",
		"ext:custom.dat:foo",
		"regexp:.*ad.*",
		"keyword:tracker",
		"   ",
		"###",
	}
	got, report := ResolveGeoTokens(tokens, testDatabases())

	if len(got.Domains) != 0 || len(got.CIDRs) != 0 || len(got.ExactDomains) != 0 {
		t.Fatalf("nothing should have resolved, got %+v", got)
	}
	// Blank and comment lines are skipped, not reported as failures.
	if len(report.Unresolved) != 8 {
		t.Fatalf("unresolved = %d (%v), want 8", len(report.Unresolved), report.Tokens())
	}
	if r := report.Unresolved["geoip:cn"]; !strings.Contains(r, "inverse") {
		t.Fatalf("geoip:cn reason = %q, want it to mention the inverse match", r)
	}
	if r := report.Unresolved["geosite:google@ads"]; !strings.Contains(r, "@ads") {
		t.Fatalf("attribute reason = %q, want it to name the attribute", r)
	}
}

func TestResolveGeoTokensPlainForms(t *testing.T) {
	got, report := ResolveGeoTokens([]string{
		"domain:example.com",
		"full:exact.example.net",
		"8.8.8.8",
		"172.16.0.0/12",
		"Bare.Example.ORG",
	}, testDatabases())

	if !report.Empty() && len(report.Unresolved) != 0 {
		t.Fatalf("unresolved: %v", report.Unresolved)
	}
	wantDomains := []string{"bare.example.org", "example.com"}
	sortedEqual(t, "domains", got.Domains, wantDomains)
	sortedEqual(t, "exact", got.ExactDomains, []string{"exact.example.net"})
	sortedEqual(t, "cidrs", got.CIDRs, []string{"172.16.0.0/12", "8.8.8.8/32"})
}

func TestResolveGeoTokensDropsExactCoveredBySuffix(t *testing.T) {
	// A suffix rule already matches the host itself and everything under it,
	// so keeping the exact entry would only grow the compiled rule-set.
	got, _ := ResolveGeoTokens([]string{
		"domain:example.com",
		"full:example.com",
		"full:api.example.com",
		"full:kept.example.net",
	}, testDatabases())

	sortedEqual(t, "exact", got.ExactDomains, []string{"kept.example.net"})
}

func TestResolveGeoTokensWithoutDatabases(t *testing.T) {
	// Nothing loaded: every geo reference must be reported, never treated as
	// an empty category that quietly resolves to no rules.
	_, report := ResolveGeoTokens(
		[]string{"geosite:whitelist", "geoip:private"}, GeoDatabases{})
	if len(report.Unresolved) != 2 {
		t.Fatalf("unresolved = %v, want both tokens", report.Tokens())
	}
	for token, reason := range report.Unresolved {
		if !strings.Contains(reason, "not loaded") {
			t.Errorf("%s: reason = %q, want it to say the database is missing", token, reason)
		}
	}
}

// --- rule-set output -----------------------------------------------------

func TestWriteRoutingListRuleSetKeepsExactDomainsApart(t *testing.T) {
	dir := t.TempDir()
	p := ParsedRoutingList{
		Domains:      []string{"example.com"},
		ExactDomains: []string{"exact.example.net"},
		CIDRs:        []string{"10.0.0.0/8"},
	}
	if err := WriteRoutingListRuleSet(dir, "test", p); err != nil {
		t.Fatalf("WriteRoutingListRuleSet: %v", err)
	}
	blob := readGeoTestFile(t, RoutingListCachePath(dir, "test"))
	// sing-box reads "domain" as an exact match and "domain_suffix" as a
	// sub-domain match; the exact host must not land in the suffix list.
	if !strings.Contains(blob, `"domain":["exact.example.net"]`) {
		t.Fatalf("exact domain missing from the rule-set: %s", blob)
	}
	if strings.Contains(blob, `"domain_suffix":["example.com","exact.example.net"]`) {
		t.Fatalf("exact domain folded into suffixes: %s", blob)
	}
}

func TestWriteRoutingListRuleSetAcceptsExactOnlyList(t *testing.T) {
	dir := t.TempDir()
	p := ParsedRoutingList{ExactDomains: []string{"only.example.com"}}
	if err := WriteRoutingListRuleSet(dir, "exactonly", p); err != nil {
		t.Fatalf("a list of exact domains is not empty: %v", err)
	}
}

// --- helpers -------------------------------------------------------------

func readGeoTestFile(t *testing.T, path string) string {
	t.Helper()
	blob, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(blob)
}

func keysOf(m map[string][]GeoDomain) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func sortedEqual(t *testing.T, what string, got, want []string) {
	t.Helper()
	g := append([]string(nil), got...)
	w := append([]string(nil), want...)
	sortStringsForTest(g)
	sortStringsForTest(w)
	if !reflect.DeepEqual(g, w) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
}

func sortStringsForTest(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
