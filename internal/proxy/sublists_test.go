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
	"encoding/base64"
	"strings"
	"testing"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestExtractRoutingListsFromHeader(t *testing.T) {
	payload := `[{"name":"РКН","url":"https://example.com/rkn.lst","action":"proxy"},
	             {"name":"Ads","url":"https://github.com/o/r/blob/main/ads.json","action":"block"}]`
	got := ExtractSubscriptionRoutingLists(b64(payload), "")
	if len(got) != 2 {
		t.Fatalf("want 2 lists, got %d: %+v", len(got), got)
	}
	if got[0].Name != "РКН" || got[0].Action != "proxy" {
		t.Errorf("entry 0 wrong: %+v", got[0])
	}
	// GitHub blob URL is normalized to raw.
	if got[1].URL != "https://raw.githubusercontent.com/o/r/main/ads.json" {
		t.Errorf("blob URL not normalized: %q", got[1].URL)
	}
}

func TestExtractRoutingListsHeaderNoPadding(t *testing.T) {
	payload := `[{"url":"https://example.com/a.lst","action":"direct"}]`
	raw := base64.RawStdEncoding.EncodeToString([]byte(payload))
	got := ExtractSubscriptionRoutingLists(raw, "")
	if len(got) != 1 || got[0].Action != "direct" {
		t.Fatalf("raw-base64 header not parsed: %+v", got)
	}
	// Name fallback = URL host.
	if got[0].Name != "example.com" {
		t.Errorf("name fallback: got %q, want example.com", got[0].Name)
	}
}

func TestExtractRoutingListsFromJSONBody(t *testing.T) {
	body := `{"routingLists":[{"name":"X","url":"https://example.com/x.lst","action":"block"}],"other":1}`
	got := ExtractSubscriptionRoutingLists("", body)
	if len(got) != 1 || got[0].Action != "block" {
		t.Fatalf("json body field not parsed: %+v", got)
	}
}

func TestExtractRoutingListsHeaderWinsOverBody(t *testing.T) {
	hdr := b64(`[{"url":"https://header.test/h.lst","action":"proxy"}]`)
	body := `{"routingLists":[{"url":"https://body.test/b.lst","action":"block"}]}`
	got := ExtractSubscriptionRoutingLists(hdr, body)
	if len(got) != 1 || !strings.Contains(got[0].URL, "header.test") {
		t.Fatalf("header must win: %+v", got)
	}
}

func TestExtractRoutingListsValidation(t *testing.T) {
	payload := `[
	  {"url":"https://ok.test/a.lst","action":"proxy"},
	  {"url":"https://bad.test/b.lst","action":"tunnel"},
	  {"url":"ftp://bad.test/c.lst","action":"proxy"},
	  {"url":"","action":"proxy"},
	  {"name":"` + strings.Repeat("я", 100) + `","url":"https://ok.test/d.lst","action":"block"}
	]`
	got := ExtractSubscriptionRoutingLists(b64(payload), "")
	if len(got) != 2 {
		t.Fatalf("want 2 valid entries, got %d: %+v", len(got), got)
	}
	if len([]rune(got[1].Name)) != 64 {
		t.Errorf("name not capped at 64 runes: %d", len([]rune(got[1].Name)))
	}
}

func TestExtractRoutingListsLimit(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; i < 15; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"url":"https://example.test/l` + string(rune('a'+i)) + `.lst","action":"proxy"}`)
	}
	sb.WriteString("]")
	got := ExtractSubscriptionRoutingLists(b64(sb.String()), "")
	if len(got) != MaxSubscriptionRoutingLists {
		t.Fatalf("limit: want %d, got %d", MaxSubscriptionRoutingLists, len(got))
	}
}

func TestExtractRoutingListsInvalidHeaderFallsBackToBody(t *testing.T) {
	hdr := b64(`[{"url":"https://bad.test/x.lst","action":"tunnel"}]`) // parses, 0 valid
	body := `{"routingLists":[{"url":"https://body.test/b.lst","action":"block"}]}`
	got := ExtractSubscriptionRoutingLists(hdr, body)
	if len(got) != 1 || !strings.Contains(got[0].URL, "body.test") {
		t.Fatalf("all-invalid header must fall back to body: %+v", got)
	}
}

func TestExtractRoutingListsGarbage(t *testing.T) {
	for _, hdr := range []string{"", "not-base64!!!", b64("not json"), b64("{}")} {
		if got := ExtractSubscriptionRoutingLists(hdr, "plain text body"); len(got) != 0 {
			t.Errorf("garbage %q must yield nothing, got %+v", hdr, got)
		}
	}
}

func TestExtractEmbeddedRoutingLists(t *testing.T) {
	body := `[{"routing":{"rules":[
	  {"type":"field","protocol":["bittorrent"],"outboundTag":"block"},
	  {"type":"field","domain":["domain:telegram.org","t.me"],"outboundTag":"proxy"},
	  {"type":"field","domain":["2gis.ru","nalog.ru","geosite:category-ads","regexp:.*\\.ru"],"outboundTag":"direct"},
	  {"type":"field","ip":["geoip:private","1.2.3.0/24"],"outboundTag":"direct"},
	  {"type":"field","network":"tcp,udp","outboundTag":"proxy"}
	]}}]`
	got := ExtractEmbeddedRoutingLists(body)

	// block action: bittorrent rule has no domain/ip -> no block list emitted.
	if _, ok := got["block"]; ok {
		t.Errorf("block must not be emitted (protocol-only rule): %+v", got["block"])
	}
	// proxy: telegram.org, t.me (catch-all network rule contributes nothing).
	p := got["proxy"]
	if !hasStr(p.Domains, "telegram.org") || !hasStr(p.Domains, "t.me") || len(p.CIDRs) != 0 {
		t.Errorf("proxy wrong: %+v", p)
	}
	// direct: 2gis.ru, nalog.ru (geosite/regexp dropped) + 1.2.3.0/24 (geoip:private dropped).
	d := got["direct"]
	if !hasStr(d.Domains, "2gis.ru") || !hasStr(d.Domains, "nalog.ru") {
		t.Errorf("direct domains wrong: %+v", d.Domains)
	}
	for _, bad := range d.Domains {
		if strings.Contains(bad, "geosite") || strings.Contains(bad, "regexp") || strings.Contains(bad, "*") {
			t.Errorf("unsupported domain form survived: %q", bad)
		}
	}
	if !hasStr(d.CIDRs, "1.2.3.0/24") || len(d.CIDRs) != 1 {
		t.Errorf("direct cidrs wrong (geoip:private must drop): %+v", d.CIDRs)
	}
}

func TestExtractEmbeddedRoutingListsSingleObjectAndGarbage(t *testing.T) {
	obj := `{"routing":{"rules":[{"type":"field","domain":["example.com"],"outboundTag":"direct"}]}}`
	if got := ExtractEmbeddedRoutingLists(obj); len(got) != 1 || !hasStr(got["direct"].Domains, "example.com") {
		t.Errorf("single-object config not parsed: %+v", got)
	}
	for _, b := range []string{"", "not json", "[]", `[{"log":{}}]`, `{"routing":{}}`} {
		if got := ExtractEmbeddedRoutingLists(b); len(got) != 0 {
			t.Errorf("garbage/no-routing %q must yield nothing: %+v", b, got)
		}
	}
}

func TestExtractEmbeddedRoutingListsUsesFirstConfigWithRules(t *testing.T) {
	// Array where the first config has no routing; second does.
	body := `[{"log":{"loglevel":"warning"}},{"routing":{"rules":[{"type":"field","domain":["a.test"],"outboundTag":"block"}]}}]`
	got := ExtractEmbeddedRoutingLists(body)
	if len(got) != 1 || !hasStr(got["block"].Domains, "a.test") {
		t.Errorf("must use first config that HAS rules: %+v", got)
	}
}
