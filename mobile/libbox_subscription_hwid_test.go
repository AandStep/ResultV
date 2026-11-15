package mobile

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchSubscriptionIdentifiesHWIDRefusal(t *testing.T) {
	const placeholder = `[{"remarks":"Notice","outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"0.0.0.0","port":1,"users":[{"id":"00000000-0000-0000-0000-000000000000"}]}]}}]}]`
	for _, tc := range []struct {
		name   string
		header string
		status int
		want   string
	}{
		{"missing HWID in fallback config", "X-Hwid-Not-Supported", 200, "subscription_hwid_required"},
		{"missing HWID in HTTP error", "X-Hwid-Not-Supported", 404, "subscription_hwid_required"},
		{"device limit", "X-Hwid-Max-Devices-Reached", 200, "subscription_hwid_limit_reached"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set(tc.header, "true")
				w.Header().Set("X-Hwid-Limit", "true")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(placeholder))
			}))
			defer server.Close()
			_, err := FetchSubscriptionV3(server.URL+"/private-subscription-path", t.TempDir(), `{"sendHwid":false}`)
			if err == nil || err.Error() != tc.want || strings.Contains(err.Error(), "private-subscription-path") {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}
