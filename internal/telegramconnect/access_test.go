package telegramconnect

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeRemnawave struct {
	mu      sync.Mutex
	created []map[string]any
	deleted []string
	users   []map[string]any
	fail    bool
}

func (f *fakeRemnawave) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer access-token" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/users":
		if f.fail {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.created = append(f.created, body)
		n := len(f.created)
		_ = json.NewEncoder(w).Encode(map[string]any{"response": map[string]any{
			"id": 1000 + n, "vlessUuid": fmt.Sprintf("00000000-0000-4000-8000-%012d", n),
		}})
	case r.Method == http.MethodGet && r.URL.Path == "/api/users":
		_ = json.NewEncoder(w).Encode(map[string]any{"response": map[string]any{"users": f.users, "total": len(f.users)}})
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/users/"):
		f.deleted = append(f.deleted, strings.TrimPrefix(r.URL.Path, "/api/users/"))
		w.WriteHeader(http.StatusOK)
	default:
		http.NotFound(w, r)
	}
}

func newTestAccess(t *testing.T, panel *fakeRemnawave) *AccessIssuer {
	t.Helper()
	server := httptest.NewServer(panel)
	t.Cleanup(server.Close)
	access := NewAccessIssuer(AccessConfig{
		RemnawaveURL: server.URL, Token: "access-token",
		Squad:   "11111111-2222-4333-8444-555555555555",
		Address: "203.0.113.7", Port: 2444, ServerName: "www.apple.com",
		PublicKey: "test-public-key", ShortID: "abcd",
	})
	if access == nil {
		t.Fatal("complete access configuration was rejected")
	}
	return access
}

func TestAccessIssuerIsDisabledWithoutFullConfig(t *testing.T) {
	if NewAccessIssuer(AccessConfig{RemnawaveURL: "http://x", Token: "t"}) != nil {
		t.Fatal("partial configuration must disable temporary access")
	}
}

func TestAccessLeaseIsReusedAndScopedToSquad(t *testing.T) {
	panel := &fakeRemnawave{}
	access := newTestAccess(t, panel)
	first, err := access.issue(context.Background(), "k")
	if err != nil {
		t.Fatal(err)
	}
	second, _ := access.issue(context.Background(), "k")
	if first != second || len(panel.created) != 1 {
		t.Fatalf("the same key must reuse its lease, created %d users", len(panel.created))
	}
	user := panel.created[0]
	squads, _ := user["activeInternalSquads"].([]any)
	if user["tag"] != accessTag || !strings.HasPrefix(user["username"].(string), accessUserPrefix) ||
		len(squads) != 1 || squads[0] != "11111111-2222-4333-8444-555555555555" {
		t.Fatalf("temporary user is not confined to the access squad: %v", user)
	}
	expire, _ := time.Parse(time.RFC3339, user["expireAt"].(string))
	if d := time.Until(expire); d > AccessLifetime || d < AccessLifetime-time.Minute {
		t.Fatalf("lease lifetime = %v", d)
	}
	link := access.link(first)
	for _, part := range []string{"vless://" + first.uuid + "@203.0.113.7:2444", "security=reality", "sni=www.apple.com", "pbk=test-public-key", "sid=abcd", "flow=xtls-rprx-vision"} {
		if !strings.Contains(link, part) {
			t.Fatalf("link %q lacks %q", link, part)
		}
	}
}

func TestAccessIssuanceIsRateLimited(t *testing.T) {
	panel := &fakeRemnawave{}
	access := newTestAccess(t, panel)
	for i := 0; i < accessPerMinute; i++ {
		if _, err := access.issue(context.Background(), fmt.Sprint(i)); err != nil {
			t.Fatalf("lease %d: %v", i, err)
		}
	}
	if _, err := access.issue(context.Background(), "over"); err == nil {
		t.Fatal("issuance beyond the per-minute limit must fail")
	}
}

func TestSweepDeletesOnlyExpiredTemporaryUsers(t *testing.T) {
	past := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	panel := &fakeRemnawave{users: []map[string]any{
		{"id": 1, "username": "tg_123", "tag": nil, "expireAt": past},
		{"id": 2, "username": "rv_tg_live", "tag": accessTag, "expireAt": future},
		{"id": 3, "username": "someone", "tag": accessTag, "expireAt": past},
		{"id": 4, "username": "rv_tg_old", "tag": "OTHER", "expireAt": past},
		{"id": 5, "username": "rv_tg_done", "tag": accessTag, "expireAt": past},
	}}
	access := newTestAccess(t, panel)
	if err := access.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Join(panel.deleted, ",") != "5" {
		t.Fatalf("deleted %v, want only the expired temporary user", panel.deleted)
	}
}

func newTestGateway(t *testing.T, access *AccessIssuer, upstream http.HandlerFunc) *httptest.Server {
	t.Helper()
	panel := httptest.NewServer(upstream)
	t.Cleanup(panel.Close)
	gateway, err := NewSubscriptionGateway(panel.URL, access)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(gateway)
	t.Cleanup(server.Close)
	return server
}

func subscriptionReply(expire int64, contentType, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Profile-Title", "base64:UmVzdWx0Vg==")
		w.Header().Set("Routing", "happ://routing/onadd/abc")
		w.Header().Set("Subscription-Userinfo", fmt.Sprintf("upload=0; download=0; total=0; expire=%d", expire))
		_, _ = io.WriteString(w, body)
	}
}

func fetch(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("User-Agent", "ResultV/3.6.1/Windows/11")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

const expiredJSON = `[{"remarks":"⌛ Подписка закончилась","dns":{"servers":["1.1.1.1"]},"outbounds":[{"tag":"proxy","protocol":"vless","settings":{"vnext":[{"address":"0.0.0.0","port":1}]}},{"tag":"direct","protocol":"freedom"}]},{"remarks":"https://t.me/result_vpn_bot","outbounds":[]}]`

func TestGatewayKeepsActiveSubscriptionIntact(t *testing.T) {
	panel := &fakeRemnawave{}
	body := `[{"remarks":"paid"}]`
	var seen *http.Request
	server := newTestGateway(t, newTestAccess(t, panel), func(w http.ResponseWriter, r *http.Request) {
		seen = r.Clone(context.Background())
		subscriptionReply(time.Now().Add(time.Hour).Unix(), "application/json", body)(w, r)
	})
	resp, got := fetch(t, server.URL+"/api/sub/AbCdEf123/json?x=1")
	if got != body || resp.Header.Get("Routing") == "" || resp.Header.Get("Profile-Title") == "" {
		t.Fatalf("active subscription changed: %q %v", got, resp.Header)
	}
	if seen.URL.Path != "/api/sub/AbCdEf123/json" || seen.URL.RawQuery != "x=1" || seen.UserAgent() != "ResultV/3.6.1/Windows/11" {
		t.Fatalf("upstream saw %s?%s UA %q", seen.URL.Path, seen.URL.RawQuery, seen.UserAgent())
	}
	if seen.Header.Get("X-Forwarded-Proto") != "https" || seen.Header.Get("X-Forwarded-For") == "" {
		t.Fatalf("Remnawave drops requests without forwarding headers: %v", seen.Header)
	}
	if len(panel.created) != 0 {
		t.Fatal("an active subscription must not issue temporary access")
	}
}

func TestGatewayAddsAccessToExpiredJSON(t *testing.T) {
	panel := &fakeRemnawave{}
	server := newTestGateway(t, newTestAccess(t, panel), subscriptionReply(time.Now().Add(-time.Hour).Unix(), "application/json", expiredJSON))
	resp, got := fetch(t, server.URL+"/api/sub/AbCdEf123")
	var configs []map[string]any
	if err := json.Unmarshal([]byte(got), &configs); err != nil || len(configs) != 3 {
		t.Fatalf("want the two notices plus access, got %d: %s", len(configs), got)
	}
	if configs[0]["remarks"] != "⌛ Подписка закончилась" || configs[1]["remarks"] != "https://t.me/result_vpn_bot" {
		t.Fatal("notices must stay first and unchanged")
	}
	entry := configs[2]
	if entry["remarks"] != AccessName || entry["dns"] == nil {
		t.Fatalf("access entry lost the template: %v", entry)
	}
	raw, _ := json.Marshal(entry["outbounds"])
	if !strings.Contains(string(raw), `"address":"203.0.113.7"`) || !strings.Contains(string(raw), `"publicKey":"test-public-key"`) ||
		strings.Contains(string(raw), "0.0.0.0") {
		t.Fatalf("access outbound is wrong: %s", raw)
	}
	if resp.Header.Get("Content-Length") != fmt.Sprint(len(got)) || resp.Header.Get("Routing") == "" {
		t.Fatal("headers were not carried over")
	}
	fetch(t, server.URL+"/api/sub/AbCdEf123")
	if len(panel.created) != 1 {
		t.Fatalf("refreshes during one lease created %d users", len(panel.created))
	}
}

func TestGatewayAddsAccessToExpiredBase64(t *testing.T) {
	panel := &fakeRemnawave{}
	list := "vless://00000000-0000-0000-0000-000000000000@0.0.0.0:1?security=none#expired\n"
	server := newTestGateway(t, newTestAccess(t, panel), subscriptionReply(time.Now().Add(-time.Hour).Unix(), "text/plain; charset=utf-8", base64.StdEncoding.EncodeToString([]byte(list))))
	_, got := fetch(t, server.URL+"/api/sub/AbCdEf123")
	decoded, err := base64.StdEncoding.DecodeString(got)
	lines := strings.Fields(string(decoded))
	if err != nil || len(lines) != 2 || !strings.HasPrefix(lines[1], "vless://") || !strings.Contains(lines[1], "203.0.113.7:2444") {
		t.Fatalf("base64 list = %q (%v)", decoded, err)
	}
}

func TestGatewayReturnsExpiredAsIsWhenAccessFails(t *testing.T) {
	panel := &fakeRemnawave{fail: true}
	server := newTestGateway(t, newTestAccess(t, panel), subscriptionReply(time.Now().Add(-time.Hour).Unix(), "application/json", expiredJSON))
	_, got := fetch(t, server.URL+"/api/sub/AbCdEf123")
	if got != expiredJSON {
		t.Fatalf("failed issuance must not alter the subscription: %s", got)
	}
}

func TestGatewayWithoutAccessIsTransparent(t *testing.T) {
	server := newTestGateway(t, nil, subscriptionReply(time.Now().Add(-time.Hour).Unix(), "application/json", expiredJSON))
	if _, got := fetch(t, server.URL+"/api/sub/AbCdEf123"); got != expiredJSON {
		t.Fatalf("disabled access changed the subscription: %s", got)
	}
	if resp, _ := fetch(t, server.URL+"/api/users"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("gateway must serve only subscription paths, got %d", resp.StatusCode)
	}
}

var accessLinkInPage = regexp.MustCompile(`const accessURL = "([^"]+)"`)

func TestLoginPageIssuesAccessOnlyForItsSession(t *testing.T) {
	panel := &fakeRemnawave{}
	access := newTestAccess(t, panel)
	server := httptest.NewServer(NewHandler(Origin, 123, nil, nil, access))
	defer server.Close()

	if resp, _ := fetch(t, server.URL+AccessPath+newCode()); resp.StatusCode != http.StatusGone {
		t.Fatalf("unknown access code returned %d", resp.StatusCode)
	}
	_, page := fetch(t, server.URL+Prefix+"/login?port=40000&challenge="+newCode()+"&lang=ru")
	match := accessLinkInPage.FindStringSubmatch(page)
	if match == nil {
		t.Fatal("login page does not offer temporary access")
	}
	link := strings.ReplaceAll(match[1], `\/`, "/")
	if !IsAccessSubscription(link) {
		t.Fatalf("offered link %q is not recognised as temporary access", link)
	}
	resp, body := fetch(t, server.URL+strings.TrimPrefix(link, Origin))
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(body, "vless://") ||
		!strings.Contains(resp.Header.Get("Subscription-Userinfo"), "expire=") {
		t.Fatalf("access list = %d %q", resp.StatusCode, body)
	}
	if font, _ := fetch(t, server.URL+Prefix+"/assets/benzin-bold.woff2"); font.Header.Get("Content-Type") != "font/woff2" {
		t.Fatal("brand font is not served")
	}
}

func TestLoginPageWithoutAccessHidesOffer(t *testing.T) {
	server := httptest.NewServer(NewHandler(Origin, 123, nil, nil))
	defer server.Close()
	_, page := fetch(t, server.URL+Prefix+"/login?port=40000&challenge="+newCode()+"&lang=en")
	if strings.Contains(page, `id="access"`) {
		t.Fatal("temporary access offered while disabled")
	}
}

func TestIsAccessSubscription(t *testing.T) {
	code := newCode()
	for raw, want := range map[string]bool{
		Origin + AccessPath + code:                 true,
		Origin + AccessPath + code + "?x=1":        false,
		"https://evil.example" + AccessPath + code: false,
		Origin + "/s/" + code:                      false,
		Origin + AccessPath + "short":              false,
	} {
		if got := IsAccessSubscription(raw); got != want {
			t.Errorf("IsAccessSubscription(%q) = %v", raw, got)
		}
	}
}
