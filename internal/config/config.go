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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var ErrDecryptFailed = errors.New("не удалось расшифровать конфигурацию")

type RoutingRules struct {
	Mode         string   `json:"mode"`
	Whitelist    []string `json:"whitelist"`
	AppWhitelist []string `json:"appWhitelist"`
	// AppForceVPN lists process names whose entire traffic is forced through
	// the tunnel (Smart mode's answer to domainless traffic: Discord voice,
	// Speedtest). Effective only in Tunnel mode — Proxy mode can't see apps
	// that ignore the system proxy.
	AppForceVPN []string `json:"appForceVPN"`
	// CustomBlockedDomains are user-added "route via VPN" domains, unioned
	// with the fetched block-lists in Smart mode.
	CustomBlockedDomains []string `json:"customBlockedDomains"`
	// RoutingLists are user-managed routing subscriptions (URL + action).
	RoutingLists []RoutingList `json:"routingLists"`
}

// RoutingList is a user-managed routing subscription: a remote list of
// domains/CIDRs (plain-text or sing-box source-JSON rule-set) routed by a
// single action. Cached locally as a source-format rule_set and referenced
// by buildRoute ahead of the built-in Smart/whitelist/ad-block rules.
type RoutingList struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	URL           string `json:"url"`
	Action        string `json:"action"` // "proxy" | "direct" | "block"
	Enabled       bool   `json:"enabled"`
	AllowInsecure bool   `json:"allowInsecure,omitempty"`
	UpdatedAt     int64  `json:"updatedAt,omitempty"`
	DomainCount   int    `json:"domainCount,omitempty"`
	CIDRCount     int    `json:"cidrCount,omitempty"`
	LastError     string `json:"lastError,omitempty"`
	// SubscriptionID links a provider-delivered list to its subscription.
	// Empty for user-added lists. Identity within a subscription is the URL.
	SubscriptionID string `json:"subscriptionId,omitempty"`
}

type ProxyEntry struct {
	ID       string `json:"id"`
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Type     string `json:"type"`
	Username string `json:"username"`
	Password string `json:"password"`
	Name     string `json:"name"`
	Country  string `json:"country"`

	URI             string          `json:"uri,omitempty"`
	Extra           json.RawMessage `json:"extra,omitempty"`
	Provider        string          `json:"provider,omitempty"`
	SubscriptionURL string          `json:"subscriptionUrl,omitempty"`
}

type Subscription struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	UpdatedAt string `json:"updatedAt,omitempty"`

	TrafficUpload   int64 `json:"trafficUpload,omitempty"`
	TrafficDownload int64 `json:"trafficDownload,omitempty"`
	TrafficTotal    int64 `json:"trafficTotal,omitempty"`
	ExpireUnix      int64 `json:"expireUnix,omitempty"`

	IconURL string `json:"iconUrl,omitempty"`
	// Source is an optional provenance marker (e.g. "rvsub" for resultv://rvsub/… deeplinks).
	Source string `json:"source,omitempty"`

	// AllowInsecure records that the user explicitly accepted fetching this
	// subscription over plaintext HTTP. We persist the consent so subsequent
	// RefreshSubscription calls don't re-prompt. When true, the fetcher also
	// suppresses the x-hwid header — sending a stable device identifier in
	// plaintext is exactly the leak this flag is opted into.
	AllowInsecure bool `json:"allowInsecure,omitempty"`
	// RemovedRoutingListURLs are provider routing-list URLs the user explicitly
	// deleted; subscription sync must not re-add them. Cleared only by deleting
	// the subscription itself.
	RemovedRoutingListURLs []string `json:"removedRoutingListUrls,omitempty"`
}

type AppSettings struct {
	Autostart                       bool     `json:"autostart"`
	KillSwitch                      bool     `json:"killswitch"`
	Mode                            string   `json:"mode"`
	Language                        string   `json:"language"`
	Theme                           string   `json:"theme"`
	LastSelectedProxyID             string   `json:"lastSelectedProxyId,omitempty"`
	LocalPort                       int      `json:"localPort,omitempty"`
	ListenLAN                       bool     `json:"listenLan,omitempty"`
	DNSServers                      []string `json:"dnsServers,omitempty"`
	TunIPv4                         string   `json:"tunIpv4,omitempty"`
	TunStack                        string   `json:"tunStack,omitempty"`
	Favorites                       []string `json:"favorites,omitempty"`
	SubscriptionAutoUpdate          *bool    `json:"subscriptionAutoUpdate,omitempty"`
	SubscriptionUpdateIntervalHours int      `json:"subscriptionUpdateIntervalHours,omitempty"`
	SubscriptionSendHWID            *bool    `json:"subscriptionSendHWID,omitempty"`
	SubscriptionUserAgent           string   `json:"subscriptionUserAgent,omitempty"`

	// DNSLeakProtection toggles sing-box `strict_route` on the TUN inbound.
	// Pointer so legacy configs (where the field is absent) default to ON
	// via EffectiveDNSLeakProtection — anything else would silently
	// downgrade existing installs to a leaky state on upgrade.
	DNSLeakProtection *bool `json:"dnsLeakProtection,omitempty"`

	// EnableIPv6 turns IPv6 on for the tunnel: the TUN gets an IPv6 address and
	// the resolver stops being pinned to ipv4_only. Off by default and a plain
	// bool (not a pointer like DNSLeakProtection) precisely because the default
	// is false — the zero value of a config written before this field existed is
	// already the wanted answer, so no Effective* helper is needed.
	//
	// Off is the safe default here, not a timid one: IPv6 paths are frequently
	// broken or unproxied in the networks this app targets, and an IPv6 address
	// the adapter refuses does not degrade the tunnel — it kills the whole TUN
	// inbound ("set ipv6 address: ...").
	EnableIPv6 bool `json:"enableIPv6,omitempty"`

	// RoutingListUpdateHours is the app-wide auto-update interval for
	// user routing lists. 0/absent → 24h via EffectiveRoutingListUpdateHours.
	RoutingListUpdateHours int `json:"routingListUpdateHours,omitempty"`

	// LastChangelogVersion is the product version whose release notes the user
	// has already seen. Empty in a config written before this field existed —
	// which is exactly how an upgraded install is told apart from a brand-new
	// one, since a brand-new install is seeded on first run (see
	// Manager.WasCreatedFresh).
	LastChangelogVersion string `json:"lastChangelogVersion,omitempty"`
}

// EffectiveDNSLeakProtection returns true unless the user has explicitly
// disabled DNS leak protection. Existing configs from before this setting
// existed have nil here; we treat that as "on" so the upgrade path is safe.
func (s AppSettings) EffectiveDNSLeakProtection() bool {
	if s.DNSLeakProtection == nil {
		return true
	}
	return *s.DNSLeakProtection
}

func (s AppSettings) EffectiveTunStack() string {
	switch s.TunStack {
	case "gvisor":
		return "gvisor"
	case "system":
		return "system"
	default:
		return "system"
	}
}

func (s AppSettings) EffectiveSubscriptionAutoUpdate() bool {
	if s.SubscriptionAutoUpdate == nil {
		return true
	}
	return *s.SubscriptionAutoUpdate
}

func (s AppSettings) EffectiveSubscriptionUpdateIntervalHours() int {
	if s.SubscriptionUpdateIntervalHours < 1 {
		return 6
	}
	return s.SubscriptionUpdateIntervalHours
}

func (s AppSettings) EffectiveRoutingListUpdateHours() int {
	if s.RoutingListUpdateHours < 1 {
		return 24
	}
	return s.RoutingListUpdateHours
}

func (s AppSettings) EffectiveSubscriptionSendHWID() bool {
	if s.SubscriptionSendHWID == nil {
		return true
	}
	return *s.SubscriptionSendHWID
}

type AppConfig struct {
	RoutingRules  RoutingRules   `json:"routingRules"`
	Proxies       []ProxyEntry   `json:"proxies"`
	Settings      AppSettings    `json:"settings"`
	Subscriptions []Subscription `json:"subscriptions,omitempty"`
}

func DefaultConfig() AppConfig {
	dnsLeakOn := true
	subscriptionAutoUpdate := true
	subscriptionSendHWID := true
	return AppConfig{
		RoutingRules: RoutingRules{
			Mode:                 "global",
			Whitelist:            []string{"localhost", "127.0.0.1"},
			AppWhitelist:         []string{},
			AppForceVPN:          []string{},
			CustomBlockedDomains: []string{},
			RoutingLists:         []RoutingList{},
		},
		Proxies: []ProxyEntry{},
		Settings: AppSettings{
			Mode:                            "proxy",
			Language:                        "ru",
			Theme:                           "dark",
			TunStack:                        "system",
			SubscriptionAutoUpdate:          &subscriptionAutoUpdate,
			SubscriptionUpdateIntervalHours: 6,
			SubscriptionSendHWID:            &subscriptionSendHWID,
			DNSLeakProtection:               &dnsLeakOn,
		},
	}
}

type Manager struct {
	mu         sync.RWMutex
	configPath string
	crypto     *CryptoService
	cache      AppConfig
	loaded     bool

	// createdFresh records that Init found no config file at all, i.e. this is
	// the first run of a brand-new install rather than an upgrade over an
	// existing one. A config that exists but fails to decrypt does NOT count as
	// fresh — the user had the app before, we just cannot read their settings.
	createdFresh bool
}

func NewManager(crypto *CryptoService) *Manager {
	return &Manager{
		crypto: crypto,
		cache:  DefaultConfig(),
	}
}

func (m *Manager) Init(userDataPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.configPath = filepath.Join(userDataPath, "proxy_config.json")
	legacyConfigPath := filepath.Join(legacyUserDataDir(userDataPath), "proxy_config.json")
	if err := migrateLegacyConfigFile(m.configPath, legacyConfigPath); err != nil {
		return fmt.Errorf("migrating legacy config: %w", err)
	}
	if err := promoteLegacyConfigIfNeeded(m.configPath, legacyConfigPath, m.crypto); err != nil {
		return fmt.Errorf("promoting legacy config: %w", err)
	}

	if err := m.loadLocked(); err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Migration handshake with CryptoService: if loadLocked succeeded via
	// the legacy key, the crypto layer flagged needsReencrypt. We re-save
	// the now-decrypted cache under the new per-install-salt key, then
	// clear the flag so subsequent (already-migrated) loads don't trigger
	// another rewrite. SaveConfig takes its own lock — drop ours first.
	needsMigration := m.crypto != nil && m.crypto.NeedsReencrypt()
	if needsMigration {
		current := m.cache
		m.mu.Unlock()
		err := m.SaveConfig(current)
		m.mu.Lock()
		if err != nil {
			return fmt.Errorf("re-encrypting config after key migration: %w", err)
		}
		m.crypto.ClearReencryptFlag()
	}
	return nil
}

func migrateLegacyConfigFile(newConfigPath, legacyConfigPath string) error {
	if _, err := os.Stat(newConfigPath); err == nil {
		return nil
	}
	if _, err := os.Stat(legacyConfigPath); err != nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(newConfigPath), 0o700); err != nil {
		return err
	}
	if err := os.Rename(legacyConfigPath, newConfigPath); err == nil {
		return nil
	}
	data, err := os.ReadFile(legacyConfigPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(newConfigPath, data, 0o600); err != nil {
		return err
	}
	_ = os.Remove(legacyConfigPath)
	return nil
}

func promoteLegacyConfigIfNeeded(newConfigPath, legacyConfigPath string, crypto *CryptoService) error {
	if _, err := os.Stat(newConfigPath); err != nil {
		return nil
	}
	if _, err := os.Stat(legacyConfigPath); err != nil {
		return nil
	}

	newData, err := os.ReadFile(newConfigPath)
	if err != nil {
		return err
	}
	legacyData, err := os.ReadFile(legacyConfigPath)
	if err != nil {
		return err
	}

	var newCfg AppConfig
	if err := crypto.DecryptInto(string(newData), &newCfg); err != nil {
		return nil
	}
	var legacyCfg AppConfig
	if err := crypto.DecryptInto(string(legacyData), &legacyCfg); err != nil {
		return nil
	}

	newScore := len(newCfg.Proxies) + len(newCfg.Subscriptions)
	legacyScore := len(legacyCfg.Proxies) + len(legacyCfg.Subscriptions)
	if newScore > 0 || legacyScore == 0 {
		return nil
	}
	if err := os.WriteFile(newConfigPath, legacyData, 0o600); err != nil {
		return err
	}
	_ = os.Remove(legacyConfigPath)
	return nil
}

func legacyUserDataDir(userDataPath string) string {
	if filepath.Base(userDataPath) != "ResultV" {
		return ""
	}
	return filepath.Join(filepath.Dir(userDataPath), "ResultProxy")
}

func (m *Manager) GetConfig() AppConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cache
}

// WasCreatedFresh reports whether Init started from no config file at all.
// Used to decide whether an empty LastChangelogVersion means "brand-new
// install, nothing to catch up on" or "upgraded from a build that predates the
// field, so show them what changed".
func (m *Manager) WasCreatedFresh() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.createdFresh
}

func (m *Manager) SaveConfig(cfg AppConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.configPath == "" {
		return fmt.Errorf("config manager not initialized")
	}

	cfg = ensureDefaults(cfg)

	encrypted, err := m.crypto.Encrypt(cfg)
	if err != nil {
		return fmt.Errorf("encrypting config: %w", err)
	}

	dir := filepath.Dir(m.configPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	if err := os.WriteFile(m.configPath, []byte(encrypted), 0o600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	m.cache = cfg
	m.loaded = true
	return nil
}

func (m *Manager) UpdateRoutingRules(rules RoutingRules) error {
	m.mu.Lock()
	cfg := m.cache
	m.mu.Unlock()

	cfg.RoutingRules = rules
	return m.SaveConfig(cfg)
}

func (m *Manager) UpdateSettings(settings AppSettings) error {
	m.mu.Lock()
	cfg := m.cache
	m.mu.Unlock()

	cfg.Settings = settings
	return m.SaveConfig(cfg)
}

func (m *Manager) loadLocked() error {
	if m.configPath == "" {
		return nil
	}

	data, err := os.ReadFile(m.configPath)
	if err != nil {
		if os.IsNotExist(err) {

			m.cache = DefaultConfig()
			m.loaded = true
			m.createdFresh = true
			return nil
		}
		return fmt.Errorf("reading config file: %w", err)
	}

	var cfg AppConfig
	if err := m.crypto.DecryptInto(string(data), &cfg); err != nil {
		m.cache = DefaultConfig()
		m.loaded = true
		return fmt.Errorf("%w: %v", ErrDecryptFailed, err)
	}

	m.cache = ensureDefaults(cfg)
	m.loaded = true
	return nil
}

func ensureDefaults(cfg AppConfig) AppConfig {
	if cfg.RoutingRules.Mode == "" {
		cfg.RoutingRules.Mode = "global"
	}
	if cfg.RoutingRules.Whitelist == nil {
		cfg.RoutingRules.Whitelist = []string{"localhost", "127.0.0.1"}
	}
	if cfg.RoutingRules.AppWhitelist == nil {
		cfg.RoutingRules.AppWhitelist = []string{}
	}
	if cfg.RoutingRules.AppForceVPN == nil {
		cfg.RoutingRules.AppForceVPN = []string{}
	}
	if cfg.RoutingRules.CustomBlockedDomains == nil {
		cfg.RoutingRules.CustomBlockedDomains = []string{}
	}
	if cfg.RoutingRules.RoutingLists == nil {
		cfg.RoutingRules.RoutingLists = []RoutingList{}
	}
	if cfg.Proxies == nil {
		cfg.Proxies = []ProxyEntry{}
	}
	if cfg.Settings.Mode == "" {
		cfg.Settings.Mode = "proxy"
	}
	if cfg.Settings.Language == "" {
		cfg.Settings.Language = "ru"
	}
	if cfg.Settings.Theme == "" {
		cfg.Settings.Theme = "dark"
	}
	return cfg
}
