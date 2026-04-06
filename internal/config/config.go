// Copyright (C) 2026 ResultProxy
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// RoutingRules defines proxy routing behavior.
type RoutingRules struct {
	Mode         string   `json:"mode"`         // "global", "whitelist", "smart"
	Whitelist    []string `json:"whitelist"`     // domain whitelist
	AppWhitelist []string `json:"appWhitelist"`  // process name whitelist
}

// ProxyEntry represents a single saved proxy server.
type ProxyEntry struct {
	ID       string `json:"id"`
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Type     string `json:"type"`     // "http", "socks5", "ss", "vmess", etc.
	Username string `json:"username"`
	Password string `json:"password"`
	Name     string `json:"name"`
	Country  string `json:"country"`
	// Extended fields for sing-box protocols.
	URI             string          `json:"uri,omitempty"`
	Extra           json.RawMessage `json:"extra,omitempty"`
	Provider        string          `json:"provider,omitempty"`
	SubscriptionURL string          `json:"subscriptionUrl,omitempty"`
}

// Subscription represents a saved subscription source.
type Subscription struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	UpdatedAt string `json:"updatedAt,omitempty"`
	// TrafficUpload/TrafficDownload from Subscription-Userinfo (bytes used).
	TrafficUpload   int64 `json:"trafficUpload,omitempty"`
	TrafficDownload int64 `json:"trafficDownload,omitempty"`
	TrafficTotal    int64 `json:"trafficTotal,omitempty"` // 0 = unlimited
	ExpireUnix      int64 `json:"expireUnix,omitempty"`     // 0 = unknown / none
	// IconURL optional: from subscription response headers or derived favicon URL.
	IconURL string `json:"iconUrl,omitempty"`
}

// AppSettings stores application-wide settings.
type AppSettings struct {
	Autostart  bool   `json:"autostart"`
	KillSwitch bool   `json:"killswitch"`
	AdBlock    bool   `json:"adblock"`
	Mode       string `json:"mode"`       // "proxy" or "tunnel"
	Language   string `json:"language"`   // "en", "ru"
	Theme      string `json:"theme"`      // "dark", "light", "system"
}

// AppConfig is the root configuration structure.
type AppConfig struct {
	RoutingRules  RoutingRules   `json:"routingRules"`
	Proxies       []ProxyEntry   `json:"proxies"`
	Settings      AppSettings    `json:"settings"`
	Subscriptions []Subscription `json:"subscriptions,omitempty"`
}

// DefaultConfig returns a fresh config with sensible defaults.
func DefaultConfig() AppConfig {
	return AppConfig{
		RoutingRules: RoutingRules{
			Mode:         "global",
			Whitelist:    []string{"localhost", "127.0.0.1"},
			AppWhitelist: []string{},
		},
		Proxies: []ProxyEntry{},
		Settings: AppSettings{
			Mode:     "proxy",
			Language: "ru",
			Theme:    "dark",
		},
	}
}

// Manager handles loading and saving the encrypted config file.
type Manager struct {
	mu         sync.RWMutex
	configPath string
	crypto     *CryptoService
	cache      AppConfig
	loaded     bool
}

// NewManager creates an uninitialized config manager.
func NewManager(crypto *CryptoService) *Manager {
	return &Manager{
		crypto: crypto,
		cache:  DefaultConfig(),
	}
}

// Init sets the config file path and loads the config.
// userDataPath is typically the Wails user data directory.
func (m *Manager) Init(userDataPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.configPath = filepath.Join(userDataPath, "proxy_config.json")

	if err := m.loadLocked(); err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	return nil
}

// GetConfig returns a copy of the current config.
func (m *Manager) GetConfig() AppConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cache
}

// SaveConfig saves the provided config to disk (encrypted).
func (m *Manager) SaveConfig(cfg AppConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.configPath == "" {
		return fmt.Errorf("config manager not initialized")
	}

	// Ensure defaults for missing fields.
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

// UpdateRoutingRules updates just the routing rules section.
func (m *Manager) UpdateRoutingRules(rules RoutingRules) error {
	m.mu.Lock()
	cfg := m.cache
	m.mu.Unlock()

	cfg.RoutingRules = rules
	return m.SaveConfig(cfg)
}

// UpdateSettings updates just the settings section.
func (m *Manager) UpdateSettings(settings AppSettings) error {
	m.mu.Lock()
	cfg := m.cache
	m.mu.Unlock()

	cfg.Settings = settings
	return m.SaveConfig(cfg)
}

// loadLocked loads config from disk. Must be called with m.mu held.
func (m *Manager) loadLocked() error {
	if m.configPath == "" {
		return nil
	}

	data, err := os.ReadFile(m.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No config file — use defaults.
			m.cache = DefaultConfig()
			m.loaded = true
			return nil
		}
		return fmt.Errorf("reading config file: %w", err)
	}

	var cfg AppConfig
	if err := m.crypto.DecryptInto(string(data), &cfg); err != nil {
		// Decryption failed — could be corrupted or from different machine.
		// Fall back to defaults.
		m.cache = DefaultConfig()
		m.loaded = true
		return nil
	}

	m.cache = ensureDefaults(cfg)
	m.loaded = true
	return nil
}

// ensureDefaults fills in missing fields with sensible defaults.
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
