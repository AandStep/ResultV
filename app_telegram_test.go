package main

import (
	"testing"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/proxy"
	"resultproxy-wails/internal/telegramconnect"
)

const testAccessURL = telegramconnect.Origin + telegramconnect.AccessPath + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func TestDropTelegramAccessKeepsTheRealSubscription(t *testing.T) {
	if !telegramconnect.IsAccessSubscription(testAccessURL) {
		t.Fatal("test URL must be a temporary access list")
	}
	a := newTestApp(t, t.TempDir())
	paid := "https://result-service.ru/s/paid123"
	cfg := a.config.GetConfig()
	cfg.Subscriptions = []config.Subscription{
		{ID: "paid", Name: "ResultV", URL: paid},
		{ID: "access", Name: telegramconnect.AccessTitle, URL: testAccessURL},
	}
	cfg.Proxies = []config.ProxyEntry{
		{ID: "p1", IP: "198.51.100.1", Port: 443, Type: "vless", SubscriptionURL: paid},
		{ID: "a1", IP: "203.0.113.7", Port: 2444, Type: "vless", SubscriptionURL: testAccessURL},
	}
	if err := a.config.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	a.dropTelegramAccess()

	cfg = a.config.GetConfig()
	if len(cfg.Subscriptions) != 1 || cfg.Subscriptions[0].ID != "paid" {
		t.Fatalf("subscriptions after cleanup: %+v", cfg.Subscriptions)
	}
	if len(cfg.Proxies) != 1 || cfg.Proxies[0].ID != "p1" {
		t.Fatalf("servers after cleanup: %+v", cfg.Proxies)
	}
}

func TestUsesAccessServerMatchesOnlyAccessEntries(t *testing.T) {
	entries := []config.ProxyEntry{
		{ID: "p1", IP: "198.51.100.1", Port: 443, SubscriptionURL: "https://result-service.ru/s/paid123"},
		{ID: "a1", IP: "203.0.113.7", Port: 2444, SubscriptionURL: testAccessURL},
	}
	urls := map[string]bool{testAccessURL: true}
	if !usesAccessServer(entries, urls, proxy.ProxyConfig{ID: "a1"}) {
		t.Fatal("connection through the access server not detected")
	}
	if !usesAccessServer(entries, urls, proxy.ProxyConfig{IP: "203.0.113.7", Port: 2444}) {
		t.Fatal("connection without an ID not matched by address")
	}
	if usesAccessServer(entries, urls, proxy.ProxyConfig{ID: "p1"}) {
		t.Fatal("paid server mistaken for the access server")
	}
}
