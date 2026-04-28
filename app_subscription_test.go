package main

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchSubscriptionFromURLSendsHWIDAndParsesEntries(t *testing.T) {
	oldProvider := stableHWIDProvider
	stableHWIDProvider = func(_ string) (string, error) {
		return "unit-hwid-123", nil
	}
	defer func() {
		stableHWIDProvider = oldProvider
	}()

	var seenHWID string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seenHWID == "" && strings.TrimSpace(r.Header.Get("x-hwid")) != "" {
			seenHWID = strings.TrimSpace(r.Header.Get("x-hwid"))
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("vless://af815621-b245-4149-89da-dd184cfc4b3d@example.com:443?type=tcp&security=none#Node"))
	}))
	defer ts.Close()

	app := NewApp()
	entries, _, _, _, _, _, _, err := app.fetchSubscriptionFromURL(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seenHWID != "unit-hwid-123" {
		t.Fatalf("x-hwid header mismatch: %q", seenHWID)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Type != "VLESS" {
		t.Fatalf("expected VLESS, got %s", entries[0].Type)
	}
}

func TestFetchSubscriptionFromURLEmptyBodyReturnsHWIDDiagnostic(t *testing.T) {
	oldProvider := stableHWIDProvider
	stableHWIDProvider = func(_ string) (string, error) {
		return "unit-hwid-limit", nil
	}
	defer func() {
		stableHWIDProvider = oldProvider
	}()

	announce := "Лимит устройств для подписки"
	title := "V2RayTun [test]"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Hwid-Limit", "true")
		w.Header().Set("Announce", "base64:"+base64.StdEncoding.EncodeToString([]byte(announce)))
		w.Header().Set("Profile-Title", "base64:"+base64.StdEncoding.EncodeToString([]byte(title)))
		w.Header().Set("Support-Url", "https://example.com/support")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	app := NewApp()
	_, _, _, _, _, _, _, err := app.fetchSubscriptionFromURL(ts.URL)
	if err == nil {
		t.Fatal("expected error")
	}
	got := err.Error()
	if !strings.Contains(got, "достигнут лимит устройств") {
		t.Fatalf("unexpected error text: %s", got)
	}
	if !strings.Contains(got, announce) {
		t.Fatalf("announce text not found: %s", got)
	}
	if !strings.Contains(got, title) {
		t.Fatalf("profile title not found: %s", got)
	}
	if !strings.Contains(got, "https://example.com/support") {
		t.Fatalf("support url not found: %s", got)
	}
}

func TestFetchSubscriptionFromURLProfileTitleOverridesProvider(t *testing.T) {
	title := "v2RayTun VPN"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Profile-Title", "base64:"+base64.StdEncoding.EncodeToString([]byte(title)))
		_, _ = w.Write([]byte("vless://af815621-b245-4149-89da-dd184cfc4b3d@example.com:443?type=tcp&security=none#Node"))
	}))
	defer ts.Close()

	app := NewApp()
	entries, _, _, _, _, _, gotTitle, err := app.fetchSubscriptionFromURL(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotTitle != title {
		t.Fatalf("profile title: want %q got %q", title, gotTitle)
	}
	if len(entries) != 1 || entries[0].Provider != title {
		t.Fatalf("provider: want %q got %q", title, entries[0].Provider)
	}
}

func TestPickIconFromSubscriptionHTMLAppleTouchAssetsPath(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".png") {
			gotPath = r.URL.Path
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte{137, 80, 78, 71, 13, 10, 26, 10})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	html := `<head><link rel="apple-touch-icon" sizes="180x180" href="/assets/apple-touch-icon-180x180.png"></head>`
	client := &http.Client{}
	icon := pickIconFromSubscriptionHTML(client, ts.URL, html)
	if icon == "" || !strings.HasPrefix(icon, "data:image/png;base64,") {
		t.Fatalf("expected inlined png, got %q", icon)
	}
	if gotPath != "/assets/apple-touch-icon-180x180.png" {
		t.Fatalf("fetch path: want /assets/... got %q", gotPath)
	}
}
