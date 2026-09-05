// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"encoding/base64"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// realPanelPayload is the JSON a live panel publishes, captured verbatim from
// https://my.impio.space/routing/incy/whitelist. Kept exact — the whole reason
// this parser exists is to read what panels already emit, so the test that
// proves it must not read a tidied-up version.
const realPanelPayload = `{"Name":"impVPN Routing Whitelist","GlobalProxy":"true",` +
	`"UseChunkFiles":"true","RemoteDns":"8.8.8.8","DomesticDns":"77.88.8.8",` +
	`"RemoteDNSType":"DoH","RemoteDNSDomain":"https://8.8.8.8/dns-query",` +
	`"RemoteDNSIP":"8.8.8.8","DomesticDNSType":"DoH",` +
	`"DomesticDNSDomain":"https://77.88.8.8/dns-query","DomesticDNSIP":"77.88.8.8",` +
	`"Geoipurl":"https://my.impio.space/downloads/routing/202609010835/geo/geoip.dat",` +
	`"Geositeurl":"https://my.impio.space/downloads/routing/202609010835/geo/geosite.dat",` +
	`"LastUpdated":"1788322632",` +
	`"DnsHosts":{"lkfl2.nalog.ru":"213.24.64.175","lknpd.nalog.ru":"213.24.64.181"},` +
	`"RouteOrder":"block-proxy-direct",` +
	`"DirectSites":["geosite:private","geosite:whitelist"],` +
	`"DirectIp":["geoip:private","geoip:whitelist"],` +
	`"ProxySites":[],"ProxyIp":[],` +
	`"BlockSites":["geosite:win-spy","geosite:torrent","geosite:category-ads"],` +
	`"BlockIp":[],"DomainStrategy":"IPIfNonMatch","FakeDNS":"false"}`

func routingLink(payload string) string {
	return "resultv://routing/onadd/" + base64.RawURLEncoding.EncodeToString([]byte(payload))
}

func TestDecodeRoutingDeepLinkRealPanelPayload(t *testing.T) {
	p, err := DecodeRoutingDeepLink(routingLink(realPanelPayload))
	if err != nil {
		t.Fatalf("DecodeRoutingDeepLink: %v", err)
	}
	if p.Name != "impVPN Routing Whitelist" {
		t.Errorf("Name = %q", p.Name)
	}
	if !reflect.DeepEqual(p.DirectSites, []string{"geosite:private", "geosite:whitelist"}) {
		t.Errorf("DirectSites = %v", p.DirectSites)
	}
	if !reflect.DeepEqual(p.DirectIPs, []string{"geoip:private", "geoip:whitelist"}) {
		t.Errorf("DirectIPs = %v", p.DirectIPs)
	}
	if len(p.BlockSites) != 3 {
		t.Errorf("BlockSites = %v", p.BlockSites)
	}
	if p.RouteOrder != "block-proxy-direct" {
		t.Errorf("RouteOrder = %q", p.RouteOrder)
	}
	if p.DomainStrategy != "IPIfNonMatch" {
		t.Errorf("DomainStrategy = %q", p.DomainStrategy)
	}
	if !strings.HasSuffix(p.GeoSiteURL, "/geosite.dat") || !strings.HasSuffix(p.GeoIPURL, "/geoip.dat") {
		t.Errorf("geo urls = %q / %q", p.GeoIPURL, p.GeoSiteURL)
	}
	// The panel's own stamp, not our import time.
	if p.UpdatedAt != 1788322632 {
		t.Errorf("UpdatedAt = %d, want the panel stamp 1788322632", p.UpdatedAt)
	}
	if p.Source != "deeplink" {
		t.Errorf("Source = %q", p.Source)
	}
	if got := p.RuleCount("direct"); got != 4 {
		t.Errorf("direct count = %d, want 4", got)
	}
	if got := p.RuleCount("block"); got != 3 {
		t.Errorf("block count = %d, want 3", got)
	}
	if got := p.RuleCount("proxy"); got != 0 {
		t.Errorf("proxy count = %d, want 0", got)
	}
}

// The counts the profile list shows must be the ones the mock shows for this
// very profile: "• 4 direct • 3 block".
func TestRoutingProfileCountsMatchTheDesign(t *testing.T) {
	p, err := ParseRoutingProfileJSON([]byte(realPanelPayload))
	if err != nil {
		t.Fatalf("ParseRoutingProfileJSON: %v", err)
	}
	if p.RuleCount("direct") != 4 || p.RuleCount("block") != 3 {
		t.Fatalf("counts = %d direct / %d block, want 4 / 3",
			p.RuleCount("direct"), p.RuleCount("block"))
	}
}

func TestRoutingDeepLinkAcceptedSpellings(t *testing.T) {
	body := base64.RawURLEncoding.EncodeToString([]byte(realPanelPayload))
	padded := base64.URLEncoding.EncodeToString([]byte(realPanelPayload))
	std := base64.StdEncoding.EncodeToString([]byte(realPanelPayload))

	for name, link := range map[string]string{
		"onadd":            "resultv://routing/onadd/" + body,
		"add":              "resultv://routing/add/" + body,
		"bare routing":     "resultv://routing/" + body,
		"opaque scheme":    "resultv:routing/onadd/" + body,
		"padded base64url": "resultv://routing/onadd/" + padded,
		"standard base64":  "resultv://routing/onadd/" + std,
		"trailing slash":   "resultv://routing/onadd/" + body + "/",
		"upper-case path":  "resultv://ROUTING/ONADD/" + body,
	} {
		t.Run(name, func(t *testing.T) {
			if !IsRoutingDeepLink(link) {
				t.Fatalf("not recognised as a routing link")
			}
			if kind := DeepLinkKind(link); kind != DeepLinkKindRouting {
				t.Fatalf("kind = %q, want routing", kind)
			}
			p, err := DecodeRoutingDeepLink(link)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if p.Name != "impVPN Routing Whitelist" {
				t.Fatalf("Name = %q", p.Name)
			}
		})
	}
}

func TestSubscriptionLinksAreNotRouting(t *testing.T) {
	// A subscription link must keep going down the old path: these are already
	// in the wild and carry nothing that says which kind they are.
	for _, link := range []string{
		"resultv://import/AAAA",
		"resultv://rvsub/AAAA",
		"resultv://crypt4/AAAA",
		"resultv://AAAA",
		"resultv://plain/https://example.com/sub",
	} {
		if IsRoutingDeepLink(link) {
			t.Errorf("%s: classified as routing", link)
		}
		if kind := DeepLinkKind(link); kind != DeepLinkKindSubscription {
			t.Errorf("%s: kind = %q", link, kind)
		}
		if _, err := DecodeRoutingDeepLink(link); !errors.Is(err, ErrNotRoutingDeepLink) {
			t.Errorf("%s: err = %v, want ErrNotRoutingDeepLink", link, err)
		}
	}
}

func TestRoutingProfileRejectsUnusablePayloads(t *testing.T) {
	cases := map[string]string{
		"not json":  `not json at all`,
		"no rules":  `{"Name":"Empty"}`,
		"only meta": `{"Name":"X","RouteOrder":"block-proxy-direct","DirectSites":[]}`,
	}
	for name, payload := range cases {
		if _, err := ParseRoutingProfileJSON([]byte(payload)); err == nil {
			t.Errorf("%s: expected an error, got none", name)
		}
	}
}

func TestRoutingProfileDropsDangerousGeoURLs(t *testing.T) {
	// The geo URLs are fetched later. A link is attacker-reachable, so a
	// non-http scheme must never reach the fetcher.
	payload := `{"Name":"X","DirectSites":["example.com"],` +
		`"Geoipurl":"file:///etc/passwd","Geositeurl":"javascript:alert(1)"}`
	p, err := ParseRoutingProfileJSON([]byte(payload))
	if err != nil {
		t.Fatalf("ParseRoutingProfileJSON: %v", err)
	}
	if p.GeoIPURL != "" || p.GeoSiteURL != "" {
		t.Fatalf("geo urls kept: %q / %q", p.GeoIPURL, p.GeoSiteURL)
	}
}

func TestRoutingProfileDropsUnknownRouteOrder(t *testing.T) {
	// The order decides which rule wins when several match; guessing at an
	// unrecognised value would route traffic on an invented policy.
	payload := `{"Name":"X","DirectSites":["example.com"],"RouteOrder":"whatever"}`
	p, err := ParseRoutingProfileJSON([]byte(payload))
	if err != nil {
		t.Fatalf("ParseRoutingProfileJSON: %v", err)
	}
	if p.RouteOrder != "" {
		t.Fatalf("RouteOrder = %q, want it dropped", p.RouteOrder)
	}
}

func TestRoutingProfileDedupesAndTrimsTokens(t *testing.T) {
	payload := `{"Name":"X","DirectSites":["  example.com  ","EXAMPLE.com","", "other.com"]}`
	p, err := ParseRoutingProfileJSON([]byte(payload))
	if err != nil {
		t.Fatalf("ParseRoutingProfileJSON: %v", err)
	}
	if !reflect.DeepEqual(p.DirectSites, []string{"example.com", "other.com"}) {
		t.Fatalf("DirectSites = %v", p.DirectSites)
	}
}

func TestRoutingProfileNameFallback(t *testing.T) {
	p, err := ParseRoutingProfileJSON([]byte(`{"DirectSites":["example.com"]}`))
	if err != nil {
		t.Fatalf("ParseRoutingProfileJSON: %v", err)
	}
	if p.Name == "" {
		t.Fatal("a nameless profile must still get a label to show in the list")
	}
}

func TestRoutingProfileRejectsOversizedPayload(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"Name":"Huge","DirectSites":[`)
	for i := 0; i < MaxRoutingProfileTokens+10; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		// Distinct hosts, or the dedupe would shrink the list below the cap.
		b.WriteString(`"h`)
		b.WriteString(strings.Repeat("0", 0))
		b.WriteString(itoaTest(i))
		b.WriteString(`.example.com"`)
	}
	b.WriteString(`]}`)
	if _, err := ParseRoutingProfileJSON([]byte(b.String())); err == nil {
		t.Fatal("expected an oversized profile to be refused")
	}
}

func TestRoutingProfileTokensJoinsSitesAndIPs(t *testing.T) {
	p, err := ParseRoutingProfileJSON([]byte(realPanelPayload))
	if err != nil {
		t.Fatalf("ParseRoutingProfileJSON: %v", err)
	}
	got := RoutingProfileTokens(p, "direct")
	want := []string{"geosite:private", "geosite:whitelist", "geoip:private", "geoip:whitelist"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens = %v, want %v", got, want)
	}
	if len(RoutingProfileTokens(p, "nonsense")) != 0 {
		t.Fatal("unknown action must yield no tokens")
	}
}

// The two halves must meet: what the link carries has to resolve against the
// geo databases into actual rules.
func TestRoutingProfileResolvesAgainstGeoDatabases(t *testing.T) {
	p, err := ParseRoutingProfileJSON([]byte(realPanelPayload))
	if err != nil {
		t.Fatalf("ParseRoutingProfileJSON: %v", err)
	}
	db := GeoDatabases{
		Sites: map[string][]GeoDomain{
			"private":   {{Value: "localhost"}},
			"whitelist": {{Value: "gosuslugi.ru"}},
		},
		IPs: map[string][]string{
			"private":   {"10.0.0.0/8"},
			"whitelist": {"213.24.64.0/24"},
		},
	}
	got, report := ResolveGeoTokens(RoutingProfileTokens(p, "direct"), db)
	if len(report.Unresolved) != 0 {
		t.Fatalf("unresolved: %v", report.Unresolved)
	}
	if len(got.Domains) != 2 || len(got.CIDRs) != 2 {
		t.Fatalf("resolved to %d domains / %d cidrs, want 2 / 2", len(got.Domains), len(got.CIDRs))
	}
}

func itoaTest(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
