package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func saveProxies(t *testing.T, dir string, ids ...string) {
	t.Helper()
	mgr := NewManager(newTestCrypto())
	if err := mgr.Init(dir); err != nil {
		t.Fatal(err)
	}
	cfg := mgr.GetConfig()
	cfg.Proxies = nil
	for _, id := range ids {
		cfg.Proxies = append(cfg.Proxies, ProxyEntry{ID: id, IP: "192.0.2.1", Port: 443, Type: "VLESS"})
	}
	if err := mgr.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
}

func loadProxyIDs(t *testing.T, dir string) (*Manager, []string) {
	t.Helper()
	mgr := NewManager(newTestCrypto())
	if err := mgr.Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	var ids []string
	for _, p := range mgr.GetConfig().Proxies {
		ids = append(ids, p.ID)
	}
	return mgr, ids
}

func unreadableCopies(t *testing.T, dir string) []string {
	t.Helper()
	found, err := filepath.Glob(filepath.Join(dir, "proxy_config.json.unreadable-*"))
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func TestSaveConfigKeepsPreviousFileWhenReplaceFails(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("an open handle blocks replacing a file only on Windows")
	}
	dir := t.TempDir()
	saveProxies(t, dir, "first")
	mgr, _ := loadProxyIDs(t, dir)

	held, err := os.Open(filepath.Join(dir, "proxy_config.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := mgr.GetConfig()
	cfg.Proxies = []ProxyEntry{{ID: "second", IP: "192.0.2.2", Port: 443}}
	saveErr := mgr.SaveConfig(cfg)
	held.Close()

	if saveErr == nil {
		t.Fatal("a save that could not replace the file reported success")
	}
	if _, ids := loadProxyIDs(t, dir); len(ids) != 1 || ids[0] != "first" {
		t.Fatalf("stored profiles after a failed save: %v", ids)
	}
}

func TestSaveConfigNeverExposesPartialFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "proxy_config.json")
	mgr := NewManager(newTestCrypto())
	if err := mgr.Init(dir); err != nil {
		t.Fatal(err)
	}
	cfg := mgr.GetConfig()
	for i := 0; i < 3000; i++ {
		cfg.Proxies = append(cfg.Proxies, ProxyEntry{ID: fmt.Sprint(i), IP: "192.0.2.1", Port: 443, Name: "profile"})
	}
	if err := mgr.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	var broken string
	wg.Add(1)
	go func() {
		defer wg.Done()
		reader := newTestCrypto()
		for {
			select {
			case <-stop:
				return
			default:
			}
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var seen AppConfig
			if err := reader.DecryptInto(string(data), &seen); err != nil {
				broken = fmt.Sprintf("%d bytes on disk: %v", len(data), err)
				return
			}
		}
	}()

	for i := 0; i < 80; i++ {
		_ = mgr.SaveConfig(cfg)
	}
	close(stop)
	wg.Wait()
	if broken != "" {
		t.Fatalf("a reader saw a half-written config: %s", broken)
	}
}

func TestInitRestoresBackupWhenConfigUnreadable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "proxy_config.json")
	saveProxies(t, dir, "kept")
	loadProxyIDs(t, dir)
	if err := os.WriteFile(path, []byte("damaged"), 0o600); err != nil {
		t.Fatal(err)
	}

	mgr, ids := loadProxyIDs(t, dir)

	if len(ids) != 1 || ids[0] != "kept" {
		t.Fatalf("profiles after recovery: %v", ids)
	}
	if mgr.Recovery() != RecoveryRestored {
		t.Fatalf("recovery: %q", mgr.Recovery())
	}
	copies := unreadableCopies(t, dir)
	if len(copies) != 1 {
		t.Fatalf("unreadable copies: %v", copies)
	}
	if data, _ := os.ReadFile(copies[0]); string(data) != "damaged" {
		t.Fatalf("unreadable copy holds %q", data)
	}
	if again, ids := loadProxyIDs(t, dir); len(ids) != 1 || again.Recovery() != RecoveryNone {
		t.Fatalf("next start: profiles %v, recovery %q", ids, again.Recovery())
	}
}

func TestInitStartsCleanWhenConfigAndBackupUnreadable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "proxy_config.json")
	if err := os.WriteFile(path, []byte("damaged"), 0o600); err != nil {
		t.Fatal(err)
	}

	mgr, ids := loadProxyIDs(t, dir)

	if len(ids) != 0 || mgr.Recovery() != RecoveryReset {
		t.Fatalf("profiles %v, recovery %q", ids, mgr.Recovery())
	}
	if mgr.WasCreatedFresh() {
		t.Fatal("an unreadable config was treated as a first run")
	}
	copies := unreadableCopies(t, dir)
	if len(copies) != 1 {
		t.Fatalf("unreadable copies: %v", copies)
	}
	if data, _ := os.ReadFile(copies[0]); string(data) != "damaged" {
		t.Fatalf("unreadable copy holds %q", data)
	}
	cfg := mgr.GetConfig()
	cfg.Proxies = []ProxyEntry{{ID: "new", IP: "192.0.2.3", Port: 443}}
	if err := mgr.SaveConfig(cfg); err != nil {
		t.Fatalf("saving after a reset: %v", err)
	}
	if _, ids := loadProxyIDs(t, dir); len(ids) != 1 || ids[0] != "new" {
		t.Fatalf("profiles saved after a reset: %v", ids)
	}
}

func TestInitKeepsOnlyRecentUnreadableCopies(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "proxy_config.json")
	for day := 1; day <= 7; day++ {
		old := fmt.Sprintf("%s.unreadable-2026010%d-000000.000", path, day)
		if err := os.WriteFile(old, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, []byte("damaged"), 0o600); err != nil {
		t.Fatal(err)
	}

	loadProxyIDs(t, dir)

	copies := unreadableCopies(t, dir)
	if len(copies) != 5 {
		t.Fatalf("kept %d unreadable copies: %v", len(copies), copies)
	}
	newest := false
	for _, c := range copies {
		if data, _ := os.ReadFile(c); string(data) == "damaged" {
			newest = true
		}
	}
	if !newest {
		t.Fatal("the copy made on this start was pruned")
	}
}

func dirSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	snap := map[string]string{}
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		snap[e.Name()] = string(data)
	}
	return snap
}

func sameSnapshot(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for name, data := range a {
		if other, ok := b[name]; !ok || other != data {
			return false
		}
	}
	return true
}

func TestReadStoredSettingsLeavesFilesUntouched(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(newTestCrypto())
	if err := mgr.Init(dir); err != nil {
		t.Fatal(err)
	}
	cfg := mgr.GetConfig()
	cfg.Settings.AlwaysRunAsAdmin = true
	if err := mgr.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	before := dirSnapshot(t, dir)

	settings, ok := ReadStoredSettings(newTestCrypto(), dir)

	if !ok || !settings.AlwaysRunAsAdmin {
		t.Fatalf("stored settings: ok=%v alwaysRunAsAdmin=%v", ok, settings.AlwaysRunAsAdmin)
	}
	if !sameSnapshot(before, dirSnapshot(t, dir)) {
		t.Fatal("reading the stored settings changed files on disk")
	}
}

func TestReadStoredSettingsLeavesUnreadableConfigInPlace(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "proxy_config.json"), []byte("damaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := dirSnapshot(t, dir)

	if _, ok := ReadStoredSettings(newTestCrypto(), dir); ok {
		t.Fatal("an unreadable config yielded settings")
	}
	if !sameSnapshot(before, dirSnapshot(t, dir)) {
		t.Fatal("a failed read changed files on disk")
	}
}
