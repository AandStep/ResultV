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
	// Profiles are whole routing rule sets — the shape panels and deep links
	// deliver. Only one is in effect at a time (ActiveProfileID), and only in
	// Global mode: in Smart the client decides routing itself, so a profile
	// there would be fighting the very thing Smart exists to do.
	Profiles []RoutingProfile `json:"routingProfiles"`
	// ActiveProfileID names the profile in effect, "" for none.
	ActiveProfileID string `json:"activeRoutingProfileId"`
}

// RoutingProfile is one complete routing rule set: what goes direct, what goes
// through the proxy and what is blocked, plus where the geo databases those
// rules reference are fetched from.
//
// It is the same object however it arrives — typed in by hand, delivered inside
// a node subscription, or opened from a routing deep link. That is deliberate:
// the three are the same thing from the user's side, and keeping three models
// for them would mean three code paths to keep in step.
type RoutingProfile struct {
	ID   string `json:"id"`
	Name string `json:"name"`

	// Rule tokens exactly as authored — "geosite:private", "domain:example.com",
	// "10.0.0.0/8". Kept raw rather than pre-expanded so the editor shows the
	// author's own text back, and so a geo database update changes what a
	// profile matches without a re-import.
	DirectSites []string `json:"directSites,omitempty"`
	DirectIPs   []string `json:"directIp,omitempty"`
	ProxySites  []string `json:"proxySites,omitempty"`
	ProxyIPs    []string `json:"proxyIp,omitempty"`
	BlockSites  []string `json:"blockSites,omitempty"`
	BlockIPs    []string `json:"blockIp,omitempty"`

	// RouteOrder is the order actions are evaluated in, e.g. "block-proxy-direct".
	RouteOrder string `json:"routeOrder,omitempty"`
	// DomainStrategy mirrors the xray option of the same name (IPIfNonMatch…).
	DomainStrategy string `json:"domainStrategy,omitempty"`

	GeoIPURL   string `json:"geoipUrl,omitempty"`
	GeoSiteURL string `json:"geositeUrl,omitempty"`

	// ListURLs are remote rule lists this profile pulls in, keyed by action
	// ("direct"/"proxy"/"block"). A provider may hand out its routing as links
	// to fetchable lists rather than as inline rules; inlining a 74k-entry list
	// into the config would bloat it past usefulness, so the profile keeps the
	// link and the compile step fetches it.
	ListURLs map[string][]string `json:"listUrls,omitempty"`
	// AllowInsecure carries the subscription's plaintext consent down to those
	// fetches: a provider list inherits the choice the user already made about
	// that provider, and never grants itself one.
	AllowInsecure bool `json:"allowInsecure,omitempty"`

	// Source records where the profile came from: "manual", "deeplink" or
	// "subscription". SubscriptionID is set for the last one.
	Source         string `json:"source,omitempty"`
	SubscriptionID string `json:"subscriptionId,omitempty"`
	// OriginName is the name the publisher gave this profile, kept as it first
	// arrived. Name is the user's to change; this is not, because it is the
	// only handle a re-published payload can be matched against — the JSON
	// carries no id of its own. Matching on Name instead would fork a renamed
	// profile into two the next time its link was opened.
	OriginName string `json:"originName,omitempty"`

	UpdatedAt int64  `json:"updatedAt,omitempty"`
	LastError string `json:"lastError,omitempty"`
}

// RuleCount totals the tokens of one action, for the "• 4 direct • 3 block"
// line in the profile list. Remote lists count as one entry each: how many
// rules hide behind a link is unknown until it is fetched.
func (p RoutingProfile) RuleCount(action string) int {
	n := len(p.ListURLs[action])
	switch action {
	case "direct":
		return n + len(p.DirectSites) + len(p.DirectIPs)
	case "proxy":
		return n + len(p.ProxySites) + len(p.ProxyIPs)
	case "block":
		return n + len(p.BlockSites) + len(p.BlockIPs)
	}
	return 0
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

	// AutoGroup names the provider-declared auto-routing pool this entry
	// belongs to. It is set only by the subscription parser, from a config's
	// `remarks` when that config declares an xray `routing.balancers` — i.e.
	// the provider itself said "these outbounds are one auto group".
	//
	// Grouping used to be reverse-engineered from display names, which broke
	// the moment impio renamed its group to Cyrillic "⚡ Авто | …" and split it
	// into per-tier sections: every member of the unrecognised section showed
	// up as a separate server card. The provider's own declaration does not
	// move when they rename things.
	//
	// Empty for manual entries, for line-based subscriptions, and for plain
	// per-server configs. omitempty keeps pre-existing configs readable with
	// no migration.
	AutoGroup string `json:"autoGroup,omitempty"`
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

	// SupportURL is the provider's own support address, taken from the
	// subscription answer (a Support-Url / Profile-Web-Page-Url header, or any
	// header whose name mentions support). Providers that send nothing leave
	// it empty, and the UI then has no support link to show.
	SupportURL string `json:"supportUrl,omitempty"`
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

	// NameOverridden records that the user renamed this subscription by hand.
	// A refresh normally adopts the profile title the provider sends; once the
	// user has picked a name, that title must not overwrite it again.
	NameOverridden bool `json:"nameOverridden,omitempty"`

	// ShowOnHome controls whether this subscription's servers appear in the
	// server list on the home screen. Nil means "shown": a subscription added
	// before this flag existed must not silently disappear.
	ShowOnHome *bool `json:"showOnHome,omitempty"`

	// UpdateIntervalMinutes overrides the global subscription auto-refresh
	// interval for this subscription alone. Nil means "follow the global
	// setting", 0 means "never refresh on a timer".
	UpdateIntervalMinutes *int `json:"updateIntervalMinutes,omitempty"`
}

// EffectiveShowOnHome reports whether this subscription's servers belong in
// the home-screen list. See ShowOnHome for why the absent value means yes.
func (s Subscription) EffectiveShowOnHome() bool {
	if s.ShowOnHome == nil {
		return true
	}
	return *s.ShowOnHome
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

	// AutoNodeRecheck lets an AUTO group re-evaluate its pick while the session
	// is running and move to a better node when the one in use has clearly
	// degraded. Pointer with an "on unless explicitly disabled" default, like
	// DNSLeakProtection: the whole point of an auto group is that the choice is
	// not the user's problem, and a group that picks once and then rides a
	// dying node for hours is not doing its job.
	//
	// It is a setting at all because the switch is not free — reconnecting
	// tears down every open connection — so someone who would rather keep a
	// mediocre-but-stable session needs a way to say so.
	AutoNodeRecheck *bool `json:"autoNodeRecheck,omitempty"`

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

// EffectiveAutoNodeRecheck returns true unless the user has explicitly turned
// the mid-session AUTO recheck off. Configs written before the field existed
// have nil here and get the feature.
func (s AppSettings) EffectiveAutoNodeRecheck() bool {
	if s.AutoNodeRecheck == nil {
		return true
	}
	return *s.AutoNodeRecheck
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
	RoutingRules RoutingRules `json:"routingRules"`
	Proxies      []ProxyEntry `json:"proxies"`
	Settings     AppSettings  `json:"settings"`
	// Без `omitempty`: пустой список должен доезжать до фронта именно пустым
	// списком. С `omitempty` последняя удалённая подписка выпадала из ответа
	// целиком, фронт видел «поля нет» и оставлял в состоянии прежний список —
	// на странице серверов оставался заголовок только что удалённой подписки.
	Subscriptions []Subscription `json:"subscriptions"`
}

func DefaultConfig() AppConfig {
	dnsLeakOn := true
	subscriptionAutoUpdate := true
	subscriptionSendHWID := true
	return AppConfig{
		RoutingRules: RoutingRules{
			Mode:                 "smart",
			Whitelist:            []string{"localhost", "127.0.0.1"},
			AppWhitelist:         []string{},
			AppForceVPN:          []string{},
			CustomBlockedDomains: []string{},
			RoutingLists:         []RoutingList{},
			Profiles:             []RoutingProfile{},
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
		/* Умный режим — стандартный: конфиг без режима означает «пользователь
		   не выбирал», а не «выбрал глобальный». Явно записанный "global"
		   этой веткой не затрагивается и остаётся как был. */
		cfg.RoutingRules.Mode = "smart"
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
	if cfg.RoutingRules.Profiles == nil {
		cfg.RoutingRules.Profiles = []RoutingProfile{}
	}
	if cfg.RoutingRules.RoutingLists == nil {
		cfg.RoutingRules.RoutingLists = []RoutingList{}
	}
	if cfg.Proxies == nil {
		cfg.Proxies = []ProxyEntry{}
	}
	if cfg.Subscriptions == nil {
		cfg.Subscriptions = []Subscription{}
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
