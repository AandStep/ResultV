package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// End-to-end: a subscription with two balancer configs must produce two AUTO
// heads, each owning only its own members, with the rest of the servers
// visible as ordinary entries.
func TestFetchSubscriptionBuildsOneAutoHeadPerBalancerSection(t *testing.T) {
	node := func(tag, host string, port int) string {
		return `{"tag":"` + tag + `","protocol":"vless","settings":{"vnext":[{"address":"` + host +
			`","port":` + strconv.Itoa(port) +
			`,"users":[{"id":"af815621-b245-4149-89da-dd184cfc4b3d","encryption":"none"}]}]},` +
			`"streamSettings":{"network":"tcp","security":"none"}}`
	}
	svc := `{"tag":"direct","protocol":"freedom"},{"tag":"block","protocol":"blackhole"},{"tag":"dns-out","protocol":"dns"}`
	bal := func(remarks, sel string, nodes ...string) string {
		return `{"remarks":"` + remarks + `","routing":{"balancers":[{"tag":"bal-auto","selector":["` + sel +
			`"],"fallbackTag":"` + sel + `"}]},"outbounds":[` + strings.Join(nodes, ",") + `,` + svc + `]}`
	}
	plain := func(remarks, host string, port int) string {
		return `{"remarks":"` + remarks + `","routing":{"rules":[]},"outbounds":[` +
			node("proxy", host, port) + `,` + svc + `]}`
	}

	body := "[" + strings.Join([]string{
		bal("🚀 impVPN Auto", "basic-proxy",
			node("basic-proxy", "n1.example", 443), node("basic-proxy-2", "n2.example", 443)),
		bal("⚡ Авто | ✅ Когда не глушат интернет", "basic-proxy",
			node("basic-proxy", "n1.example", 443), node("basic-proxy-2", "n2.example", 443)),
		plain("🇺🇸 США | VLESS TCP | №1", "n1.example", 443),
		plain("🇬🇪 Грузия | VLESS TCP | №2", "n2.example", 443),
	}, ",") + "]"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	app := NewApp()
	res, err := app.fetchSubscriptionFromURL(ts.URL, true)
	entries := res.Entries
	if err != nil {
		t.Fatalf("fetchSubscriptionFromURL: %v", err)
	}

	var heads []string
	memberIDs := map[string][]string{}
	for _, e := range entries {
		if e.Type != "AUTO" {
			continue
		}
		heads = append(heads, e.Name)
		var extra struct {
			Members []string `json:"members"`
		}
		if err := json.Unmarshal(e.Extra, &extra); err != nil {
			t.Fatalf("members of %q: %v", e.Name, err)
		}
		memberIDs[e.ID] = extra.Members
	}

	if len(heads) != 2 {
		t.Fatalf("got %d AUTO heads (%v), want 2", len(heads), heads)
	}
	if heads[0] != "🚀 impVPN Auto" || heads[1] != "⚡ Авто | ✅ Когда не глушат интернет" {
		t.Errorf("head names = %v", heads)
	}
	if len(memberIDs) != 2 {
		t.Fatalf("AUTO heads share an ID: %v", memberIDs)
	}

	// No member may be claimed by two heads.
	owner := map[string]string{}
	for headID, ids := range memberIDs {
		if len(ids) != 2 {
			t.Errorf("head %s owns %d members, want 2", headID, len(ids))
		}
		for _, id := range ids {
			if prev, dup := owner[id]; dup {
				t.Errorf("member %s owned by both %s and %s", id, prev, headID)
			}
			owner[id] = headID
		}
	}

	// Heads first, then hidden members, then the individual servers.
	if entries[0].Type != "AUTO" || entries[1].Type != "AUTO" {
		t.Errorf("AUTO heads must lead the slice, got %s/%s", entries[0].Type, entries[1].Type)
	}
	if got := visibleSubscriptionCount(entries); got != 4 {
		t.Errorf("visible = %d, want 4 (2 AUTO + 2 servers)", got)
	}
}
