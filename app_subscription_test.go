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
	entries, _, _, _, _, _, err := app.fetchSubscriptionFromURL(ts.URL)
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
	_, _, _, _, _, _, err := app.fetchSubscriptionFromURL(ts.URL)
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
