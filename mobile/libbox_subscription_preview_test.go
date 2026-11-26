package mobile

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"resultproxy-wails/internal/config"
)

func TestSubscriptionPreviewShowsSectionOnlyResponse(t *testing.T) {
	const notices = `[{"remarks":"Account expired","outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"0.0.0.0","port":1,"users":[{"id":"00000000-0000-0000-0000-000000000000"}]}]}}]},{"remarks":"Contact support","outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"::","port":1,"users":[{"id":"00000000-0000-0000-0000-000000000000"}]}]}}]}]`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(notices))
	}))
	t.Cleanup(server.Close)
	dataDir := t.TempDir()
	raw, err := FetchSubscriptionV3(server.URL, dataDir, `{"sendHwid":false,"preview":true}`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct{ Entries []config.ProxyEntry }
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	want := []string{"Account expired", "Contact support"}
	if len(result.Entries) != len(want) {
		t.Fatalf("got %d preview rows, want %d", len(result.Entries), len(want))
	}
	for i, entry := range result.Entries {
		if entry.Type != "SECTION" || entry.Name != want[i] {
			t.Fatalf("row %d = %s %q, want SECTION %q", i, entry.Type, entry.Name, want[i])
		}
	}
	if _, err := FetchSubscriptionV3(server.URL, dataDir, `{"sendHwid":false}`); err == nil {
		t.Fatal("a response without servers must not replace an existing subscription")
	}
}
