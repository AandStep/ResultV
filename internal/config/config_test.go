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

package config

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestEffectiveDNSLeakProtection_LegacyConfigDefaultsOn verifies the upgrade
// path: configs written before the DNSLeakProtection field existed have nil
// here, and we must treat that as ON. Otherwise existing users would
// silently fall back to a leaky state on first run after upgrade.
func TestEffectiveDNSLeakProtection_LegacyConfigDefaultsOn(t *testing.T) {
	// JSON without the dnsLeakProtection field — mimics on-disk config from
	// a release prior to this feature.
	raw := []byte(`{"autostart":false,"killswitch":false,"adblock":false,"mode":"proxy","language":"ru","theme":"dark"}`)
	var s AppSettings
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.DNSLeakProtection != nil {
		t.Fatalf("expected nil pointer for missing field, got %v", *s.DNSLeakProtection)
	}
	if !s.EffectiveDNSLeakProtection() {
		t.Fatal("legacy config without dnsLeakProtection must default to true (on)")
	}
}

func TestEffectiveDNSLeakProtection_ExplicitlyDisabled(t *testing.T) {
	off := false
	s := AppSettings{DNSLeakProtection: &off}
	if s.EffectiveDNSLeakProtection() {
		t.Fatal("expected false when user has explicitly disabled DNS leak protection")
	}
}

func TestEffectiveDNSLeakProtection_ExplicitlyEnabled(t *testing.T) {
	on := true
	s := AppSettings{DNSLeakProtection: &on}
	if !s.EffectiveDNSLeakProtection() {
		t.Fatal("expected true when user has explicitly enabled DNS leak protection")
	}
}

func TestDefaultConfig_DNSLeakProtectionOn(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Settings.DNSLeakProtection == nil {
		t.Fatal("DefaultConfig should set DNSLeakProtection explicitly so fresh installs persist the choice")
	}
	if !*cfg.Settings.DNSLeakProtection {
		t.Fatal("DefaultConfig should enable DNS leak protection by default")
	}
}

func TestDefaultConfig_SubscriptionSettingsPreserveCurrentBehavior(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Settings.EffectiveSubscriptionAutoUpdate() {
		t.Fatal("subscription auto update should default to on")
	}
	if got := cfg.Settings.EffectiveSubscriptionUpdateIntervalHours(); got != 6 {
		t.Fatalf("subscription update interval: want 6, got %d", got)
	}
	if !cfg.Settings.EffectiveSubscriptionSendHWID() {
		t.Fatal("subscription HWID sending should default to on")
	}
	if got := cfg.Settings.EffectiveTunStack(); got != "system" {
		t.Fatalf("TUN stack default: want system, got %q", got)
	}
}

func TestAppSettings_LegacySubscriptionSettingsDefaults(t *testing.T) {
	raw := []byte(`{"autostart":false,"killswitch":false,"adblock":false,"mode":"proxy","language":"ru","theme":"dark"}`)
	var s AppSettings
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !s.EffectiveSubscriptionAutoUpdate() {
		t.Fatal("legacy config without subscriptionAutoUpdate must default to true")
	}
	if got := s.EffectiveSubscriptionUpdateIntervalHours(); got != 6 {
		t.Fatalf("legacy interval default: want 6, got %d", got)
	}
	if !s.EffectiveSubscriptionSendHWID() {
		t.Fatal("legacy config without subscriptionSendHWID must default to true")
	}
	if got := s.EffectiveTunStack(); got != "system" {
		t.Fatalf("legacy TUN stack default: want system, got %q", got)
	}
}

func TestManagerLoadSaveRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	cs := newTestCrypto()
	mgr := NewManager(cs)

	if err := mgr.Init(tmpDir); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	cfg := mgr.GetConfig()
	if cfg.RoutingRules.Mode != "smart" {
		t.Errorf("expected default mode 'smart', got %q", cfg.RoutingRules.Mode)
	}
	if cfg.Settings.Mode != "proxy" {
		t.Errorf("expected default settings.mode 'proxy', got %q", cfg.Settings.Mode)
	}

	cfg.RoutingRules.Mode = "smart"
	cfg.Proxies = []ProxyEntry{
		{ID: "1", IP: "1.2.3.4", Port: 8080, Type: "http", Name: "Test"},
	}
	if err := mgr.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	configPath := filepath.Join(tmpDir, "proxy_config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("config file was not created")
	}

	mgr2 := NewManager(cs)
	if err := mgr2.Init(tmpDir); err != nil {
		t.Fatalf("Init (reload) failed: %v", err)
	}

	cfg2 := mgr2.GetConfig()
	if cfg2.RoutingRules.Mode != "smart" {
		t.Errorf("expected mode 'smart' after reload, got %q", cfg2.RoutingRules.Mode)
	}
	if len(cfg2.Proxies) != 1 {
		t.Fatalf("expected 1 proxy after reload, got %d", len(cfg2.Proxies))
	}
	if cfg2.Proxies[0].IP != "1.2.3.4" {
		t.Errorf("expected proxy IP '1.2.3.4', got %q", cfg2.Proxies[0].IP)
	}
}

func TestManagerMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	cs := newTestCrypto()
	mgr := NewManager(cs)

	if err := mgr.Init(tmpDir); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	cfg := mgr.GetConfig()
	if cfg.RoutingRules.Mode != "smart" {
		t.Errorf("expected default mode when file missing, got %q", cfg.RoutingRules.Mode)
	}
}

func TestManagerCorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "proxy_config.json")
	os.WriteFile(configPath, []byte("not json at all {{{{"), 0o600)

	cs := newTestCrypto()
	mgr := NewManager(cs)

	if err := mgr.Init(tmpDir); err == nil || !errors.Is(err, ErrDecryptFailed) {
		t.Fatalf("Init with corrupted file should return ErrDecryptFailed: %v", err)
	}

	cfg := mgr.GetConfig()
	if cfg.RoutingRules.Mode != "smart" {
		t.Errorf("expected default mode for corrupted file, got %q", cfg.RoutingRules.Mode)
	}
}

func TestManagerUpdateRoutingRules(t *testing.T) {
	tmpDir := t.TempDir()
	cs := newTestCrypto()
	mgr := NewManager(cs)
	mgr.Init(tmpDir)

	err := mgr.UpdateRoutingRules(RoutingRules{
		Mode:         "whitelist",
		Whitelist:    []string{"example.com"},
		AppWhitelist: []string{"chrome.exe"},
	})
	if err != nil {
		t.Fatalf("UpdateRoutingRules failed: %v", err)
	}

	cfg := mgr.GetConfig()
	if cfg.RoutingRules.Mode != "whitelist" {
		t.Errorf("expected mode 'whitelist', got %q", cfg.RoutingRules.Mode)
	}
	if len(cfg.RoutingRules.Whitelist) != 1 || cfg.RoutingRules.Whitelist[0] != "example.com" {
		t.Errorf("unexpected whitelist: %v", cfg.RoutingRules.Whitelist)
	}
}

func TestEnsureDefaults(t *testing.T) {
	empty := AppConfig{}
	result := ensureDefaults(empty)

	if result.RoutingRules.Mode != "smart" {
		t.Errorf("Mode: got %q, want 'smart'", result.RoutingRules.Mode)
	}
	if result.Proxies == nil {
		t.Error("Proxies should not be nil after ensureDefaults")
	}
	if result.Settings.Theme != "dark" {
		t.Errorf("Theme: got %q, want 'dark'", result.Settings.Theme)
	}
}

func TestAppSettingsHasLastSelectedProxyIDField(t *testing.T) {
	settingsType := reflect.TypeOf(AppSettings{})
	field, ok := settingsType.FieldByName("LastSelectedProxyID")
	if !ok {
		t.Fatal("AppSettings must contain LastSelectedProxyID field")
	}

	if got := field.Tag.Get("json"); got != "lastSelectedProxyId,omitempty" {
		t.Fatalf("json tag mismatch: got %q, want %q", got, "lastSelectedProxyId,omitempty")
	}
}

func TestManagerSaveLoadLastSelectedProxyID(t *testing.T) {
	settingsType := reflect.TypeOf(AppSettings{})
	field, ok := settingsType.FieldByName("LastSelectedProxyID")
	if !ok {
		t.Fatal("AppSettings must contain LastSelectedProxyID field")
	}
	if field.Type.Kind() != reflect.String {
		t.Fatalf("LastSelectedProxyID must be string, got %s", field.Type.Kind())
	}

	tmpDir := t.TempDir()
	cs := newTestCrypto()
	mgr := NewManager(cs)
	if err := mgr.Init(tmpDir); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	cfg := mgr.GetConfig()
	settingsValue := reflect.ValueOf(&cfg.Settings).Elem()
	settingsValue.FieldByName("LastSelectedProxyID").SetString("proxy-42")

	if err := mgr.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	mgr2 := NewManager(cs)
	if err := mgr2.Init(tmpDir); err != nil {
		t.Fatalf("Init (reload) failed: %v", err)
	}
	cfg2 := mgr2.GetConfig()
	got := reflect.ValueOf(cfg2.Settings).FieldByName("LastSelectedProxyID").String()
	if got != "proxy-42" {
		t.Fatalf("lastSelectedProxyId was not preserved: got %q", got)
	}
}

func TestManagerLoadLegacyConfigWithoutLastSelectedProxyID(t *testing.T) {
	type legacyAppSettings struct {
		Autostart  bool   `json:"autostart"`
		KillSwitch bool   `json:"killswitch"`
		AdBlock    bool   `json:"adblock"`
		Mode       string `json:"mode"`
		Language   string `json:"language"`
		Theme      string `json:"theme"`
	}
	type legacyAppConfig struct {
		RoutingRules  RoutingRules      `json:"routingRules"`
		Proxies       []ProxyEntry      `json:"proxies"`
		Settings      legacyAppSettings `json:"settings"`
		Subscriptions []Subscription    `json:"subscriptions,omitempty"`
	}

	tmpDir := t.TempDir()
	cs := newTestCrypto()
	mgr := NewManager(cs)

	legacyCfg := legacyAppConfig{
		RoutingRules: RoutingRules{
			Mode:         "global",
			Whitelist:    []string{"localhost", "127.0.0.1"},
			AppWhitelist: []string{},
		},
		Proxies: []ProxyEntry{
			{ID: "1", IP: "10.0.0.2", Port: 8080, Type: "http", Name: "Legacy"},
		},
		Settings: legacyAppSettings{
			Mode:     "proxy",
			Language: "ru",
			Theme:    "dark",
		},
	}

	encrypted, err := cs.Encrypt(legacyCfg)
	if err != nil {
		t.Fatalf("encrypt legacy config failed: %v", err)
	}

	configPath := filepath.Join(tmpDir, "proxy_config.json")
	if err := os.WriteFile(configPath, []byte(encrypted), 0o600); err != nil {
		t.Fatalf("write legacy config failed: %v", err)
	}

	if err := mgr.Init(tmpDir); err != nil {
		t.Fatalf("Init with legacy config should succeed: %v", err)
	}

	cfg := mgr.GetConfig()
	if cfg.Settings.LastSelectedProxyID != "" {
		t.Fatalf("expected empty lastSelectedProxyId for legacy config, got %q", cfg.Settings.LastSelectedProxyID)
	}
	if len(cfg.Proxies) != 1 || cfg.Proxies[0].ID != "1" {
		t.Fatalf("legacy proxies were not loaded correctly: %+v", cfg.Proxies)
	}
}

func TestManagerInitMigratesLegacyConfig(t *testing.T) {
	base := t.TempDir()
	newDir := filepath.Join(base, "ResultV")
	legacyDir := filepath.Join(base, "ResultProxy")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatalf("mkdir legacy dir: %v", err)
	}

	cs := newTestCrypto()
	legacyCfg := DefaultConfig()
	legacyCfg.Proxies = []ProxyEntry{
		{ID: "legacy-1", IP: "8.8.8.8", Port: 443, Type: "http", Name: "Legacy"},
	}
	enc, err := cs.Encrypt(legacyCfg)
	if err != nil {
		t.Fatalf("encrypt legacy config: %v", err)
	}
	legacyPath := filepath.Join(legacyDir, "proxy_config.json")
	if err := os.WriteFile(legacyPath, []byte(enc), 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	mgr := NewManager(cs)
	if err := mgr.Init(newDir); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	cfg := mgr.GetConfig()
	if len(cfg.Proxies) != 1 || cfg.Proxies[0].ID != "legacy-1" {
		t.Fatalf("legacy config was not migrated: %+v", cfg.Proxies)
	}
	if _, err := os.Stat(filepath.Join(newDir, "proxy_config.json")); err != nil {
		t.Fatalf("new config file missing after migration: %v", err)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy config file should be moved, err=%v", err)
	}
}

func TestManagerInitPrefersNewConfigOverLegacy(t *testing.T) {
	base := t.TempDir()
	newDir := filepath.Join(base, "ResultV")
	legacyDir := filepath.Join(base, "ResultProxy")
	if err := os.MkdirAll(newDir, 0o700); err != nil {
		t.Fatalf("mkdir new dir: %v", err)
	}
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatalf("mkdir legacy dir: %v", err)
	}

	cs := newTestCrypto()
	newCfg := DefaultConfig()
	newCfg.Proxies = []ProxyEntry{{ID: "new", IP: "1.1.1.1", Port: 443, Type: "http"}}
	legacyCfg := DefaultConfig()
	legacyCfg.Proxies = []ProxyEntry{{ID: "legacy", IP: "8.8.8.8", Port: 443, Type: "http"}}

	newEnc, err := cs.Encrypt(newCfg)
	if err != nil {
		t.Fatalf("encrypt new config: %v", err)
	}
	legacyEnc, err := cs.Encrypt(legacyCfg)
	if err != nil {
		t.Fatalf("encrypt legacy config: %v", err)
	}

	newPath := filepath.Join(newDir, "proxy_config.json")
	legacyPath := filepath.Join(legacyDir, "proxy_config.json")
	if err := os.WriteFile(newPath, []byte(newEnc), 0o600); err != nil {
		t.Fatalf("write new config: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte(legacyEnc), 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	mgr := NewManager(cs)
	if err := mgr.Init(newDir); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	cfg := mgr.GetConfig()
	if len(cfg.Proxies) != 1 || cfg.Proxies[0].ID != "new" {
		t.Fatalf("new config must have priority, got: %+v", cfg.Proxies)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy config should remain untouched when new exists: %v", err)
	}
}

// End-to-end migration: a config written by an old build (using only the
// legacy sha256 key) must be re-encrypted under the new PBKDF2-derived key
// after the first Init() of a new build. Verified by checking that:
//   1. The file decrypts and content is intact.
//   2. The on-disk bytes change (re-write happened).
//   3. After migration the CryptoService no longer reports needsReencrypt.
//   4. The migrated file decrypts with the NEW key alone (legacy not needed).
func TestManagerInitMigratesLegacyEncryptionToNewKey(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "proxy_config.json")

	// Step 1: write a config encrypted with the legacy-only key (key == legacyKey).
	legacyKey := sha256.Sum256([]byte("e2e-migration-machine" + keySalt))
	legacyCS := &CryptoService{key: legacyKey, legacyKey: legacyKey}
	legacyCfg := DefaultConfig()
	legacyCfg.Proxies = []ProxyEntry{{ID: "m1", IP: "9.9.9.9", Port: 443, Type: "http", Password: "secret"}}
	enc, err := legacyCS.Encrypt(legacyCfg)
	if err != nil {
		t.Fatalf("legacy encrypt: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(enc), 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}
	beforeBytes, _ := os.ReadFile(configPath)

	// Step 2: new build — fresh primary key, same legacyKey.
	migrationCS := &CryptoService{
		key:       sha256.Sum256([]byte("post-migration-salt")),
		legacyKey: legacyKey,
	}
	mgr := NewManager(migrationCS)
	if err := mgr.Init(tmpDir); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Step 3: confirm content preserved.
	cfg := mgr.GetConfig()
	if len(cfg.Proxies) != 1 || cfg.Proxies[0].Password != "secret" {
		t.Fatalf("config content lost during migration: %+v", cfg.Proxies)
	}

	// Step 4: file bytes must have changed (Manager re-saved under new key).
	afterBytes, _ := os.ReadFile(configPath)
	if string(beforeBytes) == string(afterBytes) {
		t.Fatal("expected config file to be rewritten after migration, bytes unchanged")
	}

	// Step 5: needsReencrypt cleared.
	if migrationCS.NeedsReencrypt() {
		t.Fatal("needsReencrypt should be cleared after Manager re-saved")
	}

	// Step 6: a fresh service WITHOUT the legacy key must now decrypt — the
	// file is encrypted under the new primary key.
	verifyCS := &CryptoService{
		key:       migrationCS.key,
		legacyKey: sha256.Sum256([]byte("totally unrelated")),
	}
	verifyMgr := NewManager(verifyCS)
	if err := verifyMgr.Init(tmpDir); err != nil {
		t.Fatalf("post-migration load without legacy key failed: %v", err)
	}
	if verifyCS.NeedsReencrypt() {
		t.Fatal("post-migration load should not require another migration")
	}
}

func TestManagerInitPromotesLegacyWhenNewIsEmpty(t *testing.T) {
	base := t.TempDir()
	newDir := filepath.Join(base, "ResultV")
	legacyDir := filepath.Join(base, "ResultProxy")
	if err := os.MkdirAll(newDir, 0o700); err != nil {
		t.Fatalf("mkdir new dir: %v", err)
	}
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatalf("mkdir legacy dir: %v", err)
	}

	cs := newTestCrypto()
	newCfg := DefaultConfig()
	legacyCfg := DefaultConfig()
	legacyCfg.Proxies = []ProxyEntry{{ID: "legacy", IP: "8.8.4.4", Port: 443, Type: "http"}}

	newEnc, err := cs.Encrypt(newCfg)
	if err != nil {
		t.Fatalf("encrypt new config: %v", err)
	}
	legacyEnc, err := cs.Encrypt(legacyCfg)
	if err != nil {
		t.Fatalf("encrypt legacy config: %v", err)
	}

	newPath := filepath.Join(newDir, "proxy_config.json")
	legacyPath := filepath.Join(legacyDir, "proxy_config.json")
	if err := os.WriteFile(newPath, []byte(newEnc), 0o600); err != nil {
		t.Fatalf("write new config: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte(legacyEnc), 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	mgr := NewManager(cs)
	if err := mgr.Init(newDir); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	cfg := mgr.GetConfig()
	if len(cfg.Proxies) != 1 || cfg.Proxies[0].ID != "legacy" {
		t.Fatalf("legacy config should replace empty new config, got: %+v", cfg.Proxies)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy config should be removed after promotion, err=%v", err)
	}
}

func TestRoutingRulesNewFieldsDefaults(t *testing.T) {
	// Конфиг старого формата (3.2.x) без новых полей: после ensureDefaults
	// оба списка не nil, чтобы JSON для фронта отдавал [], а не null.
	raw := []byte(`{"routingRules":{"mode":"smart","whitelist":["localhost"],"appWhitelist":[]}}`)
	var cfg AppConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	cfg = ensureDefaults(cfg)
	if cfg.RoutingRules.AppForceVPN == nil {
		t.Error("AppForceVPN must default to empty slice, got nil")
	}
	if cfg.RoutingRules.CustomBlockedDomains == nil {
		t.Error("CustomBlockedDomains must default to empty slice, got nil")
	}
	def := DefaultConfig()
	if def.RoutingRules.AppForceVPN == nil || def.RoutingRules.CustomBlockedDomains == nil {
		t.Error("DefaultConfig must initialise new routing fields")
	}
	if def.RoutingRules.RoutingLists == nil {
		t.Error("DefaultConfig must initialise RoutingLists to empty slice, got nil")
	}
}

func TestRoutingListsDefaults(t *testing.T) {
	cfg := ensureDefaults(AppConfig{})
	if cfg.RoutingRules.RoutingLists == nil {
		t.Error("RoutingLists must default to empty slice, got nil")
	}
	if got := cfg.Settings.EffectiveRoutingListUpdateHours(); got != 24 {
		t.Errorf("EffectiveRoutingListUpdateHours default: got %d, want 24", got)
	}
}

func TestRoutingListRoundTrip(t *testing.T) {
	rl := RoutingList{
		ID: "abc", Name: "My list", URL: "https://example.com/list.txt",
		Action: "proxy", Enabled: true, DomainCount: 3, CIDRCount: 1,
	}
	rr := RoutingRules{Mode: "smart", RoutingLists: []RoutingList{rl}}
	blob, err := json.Marshal(rr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back RoutingRules
	if err := json.Unmarshal(blob, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(back.RoutingLists) != 1 || back.RoutingLists[0].Action != "proxy" || back.RoutingLists[0].ID != "abc" {
		t.Fatalf("round-trip mismatch: %+v", back.RoutingLists)
	}
}

func TestRoutingListSubscriptionProvenance(t *testing.T) {
	rl := RoutingList{ID: "x", URL: "https://a.test/l.txt", Action: "proxy", SubscriptionID: "sub1"}
	blob, err := json.Marshal(rl)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(blob), `"subscriptionId":"sub1"`) {
		t.Errorf("subscriptionId tag missing: %s", blob)
	}
	sub := Subscription{ID: "sub1", RemovedRoutingListURLs: []string{"https://a.test/gone.txt"}}
	blob2, err := json.Marshal(sub)
	if err != nil {
		t.Fatalf("marshal sub: %v", err)
	}
	var back Subscription
	if err := json.Unmarshal(blob2, &back); err != nil {
		t.Fatalf("unmarshal sub: %v", err)
	}
	if len(back.RemovedRoutingListURLs) != 1 || back.RemovedRoutingListURLs[0] != "https://a.test/gone.txt" {
		t.Errorf("tombstones round-trip failed: %+v", back.RemovedRoutingListURLs)
	}
}
