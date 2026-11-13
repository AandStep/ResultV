package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"resultproxy-wails/internal/config"
)

func TestGetConfigWaitsForStartupLoad(t *testing.T) {
	dir := t.TempDir()
	crypto, err := config.NewCryptoService(dir)
	if err != nil {
		t.Fatal(err)
	}
	stored := config.NewManager(crypto)
	if err := stored.Init(dir); err != nil {
		t.Fatal(err)
	}
	cfg := stored.GetConfig()
	cfg.Proxies = []config.ProxyEntry{{ID: "warp", IP: "192.0.2.1", Port: 2408}}
	cfg.Settings.Mode = "tunnel"
	cfg.Settings.LastSelectedProxyID = "warp"
	if err := stored.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	a := NewApp()
	type configResponse struct {
		cfg config.AppConfig
		err error
	}
	result := make(chan configResponse, 1)
	go func() {
		cfg, err := a.GetConfig()
		result <- configResponse{cfg, err}
	}()

	select {
	case early := <-result:
		t.Fatalf("GetConfig returned before startup load completed: %v", early.err)
	case <-time.After(100 * time.Millisecond):
	}

	if !a.loadStartupConfig(dir) {
		t.Fatal("startup failed to load saved config")
	}
	close(a.startupReady)
	select {
	case loaded := <-result:
		if loaded.err != nil {
			t.Fatal(loaded.err)
		}
		cfg := loaded.cfg
		if len(cfg.Proxies) != 1 || cfg.Proxies[0].ID != "warp" ||
			cfg.Settings.Mode != "tunnel" || cfg.Settings.LastSelectedProxyID != "warp" {
			t.Fatalf("early caller did not receive the saved profile and settings")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("GetConfig did not return after startup load completed")
	}
}

func TestStartupResetsUnreadableConfigAndKeepsCopy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "proxy_config.json")
	foreignKey := config.NewCryptoServiceWithID("unrelated-test-key")
	original, err := foreignKey.Encrypt(config.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	if !a.loadStartupConfig(dir) {
		t.Fatal("an unreadable config left the app without storage")
	}
	close(a.startupReady)

	if notice := a.TakeConfigStartupNotice(); notice != "reset" {
		t.Fatalf("startup notice: %q", notice)
	}
	if notice := a.TakeConfigStartupNotice(); notice != "" {
		t.Fatalf("startup notice repeated: %q", notice)
	}
	copies, _ := filepath.Glob(path + ".unreadable-*")
	if len(copies) != 1 {
		t.Fatalf("unreadable copies: %v", copies)
	}
	if data, _ := os.ReadFile(copies[0]); string(data) != original {
		t.Fatal("the unreadable config was not kept as it was")
	}
	if err := a.SyncProxies([]config.ProxyEntry{{ID: "new", IP: "192.0.2.1", Port: 443}}); err != nil {
		t.Fatalf("saving after a reset: %v", err)
	}
	cfg, err := a.GetConfig()
	if err != nil || len(cfg.Proxies) != 1 {
		t.Fatalf("config after a reset: profiles %d, err %v", len(cfg.Proxies), err)
	}
}

func TestStartupReportsUnavailableStorage(t *testing.T) {
	notADir := filepath.Join(t.TempDir(), "occupied")
	if err := os.WriteFile(notADir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	if a.loadStartupConfig(notADir) {
		t.Fatal("startup accepted storage it cannot use")
	}
	close(a.startupReady)

	if notice := a.TakeConfigStartupNotice(); notice != "unavailable" {
		t.Fatalf("startup notice: %q", notice)
	}
	if _, err := a.GetConfig(); err == nil {
		t.Fatal("GetConfig reported unavailable storage as loaded")
	}
	if err := a.SyncProxies(nil); err == nil {
		t.Fatal("SyncProxies accepted a write without storage")
	}
}

func TestSyncProxiesRejectsWriteBeforeStartupLoad(t *testing.T) {
	a := NewApp()
	result := make(chan error, 1)
	go func() {
		result <- a.SyncProxies(nil)
	}()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("SyncProxies accepted a write before startup load")
		}
	case <-time.After(200 * time.Millisecond):
		close(a.startupReady)
		t.Fatal("SyncProxies queued a write to run after startup load")
	}
}
