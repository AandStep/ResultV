package config

import (
	"errors"
	"net/netip"
	"strings"

	"golang.org/x/net/idna"
)

type Socks5ExitConfig struct {
	Enabled  bool   `json:"enabled,omitempty"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

var socks5DNSName = idna.New(idna.MapForLookup(), idna.VerifyDNSLength(true))

func (cfg *Socks5ExitConfig) IsActive() bool {
	if cfg == nil || !cfg.Enabled {
		return false
	}
	_, err := NormalizeSocks5ExitConfig(*cfg)
	return err == nil
}

func NormalizeSocks5ExitConfig(cfg Socks5ExitConfig) (Socks5ExitConfig, error) {
	host := strings.TrimSpace(cfg.Host)
	address := host
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		address = host[1 : len(host)-1]
	}
	if addr, err := netip.ParseAddr(address); err == nil && addr.Zone() == "" {
		cfg.Host = addr.String()
	} else {
		var err error
		cfg.Host, err = normalizeSocks5DNSName(host)
		if err != nil {
			return Socks5ExitConfig{}, errors.New("invalid SOCKS5 host")
		}
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return Socks5ExitConfig{}, errors.New("SOCKS5 port must be between 1 and 65535")
	}
	if (cfg.Username == "") != (cfg.Password == "") || len(cfg.Username) > 255 || len(cfg.Password) > 255 {
		return Socks5ExitConfig{}, errors.New("SOCKS5 credentials must be supplied as a pair of up to 255 bytes each")
	}

	return cfg, nil
}

func normalizeSocks5DNSName(value string) (string, error) {
	name, err := socks5DNSName.ToASCII(strings.TrimSpace(value))
	name = strings.TrimSuffix(name, ".")
	if err != nil || name == "" {
		return "", errors.New("invalid DNS name")
	}
	return name, nil
}

// SaveSocks5Exit is the only write path for the local profile. Generic
// settings updates can come from a UI snapshot that omits the secret.
func (m *Manager) SaveSocks5Exit(profile Socks5ExitConfig, password *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	current := m.cache.Settings.Socks5Exit
	if current != nil {
		profile.Enabled = current.Enabled
	}
	if password != nil {
		profile.Password = *password
	} else if current != nil && profile.Username != "" {
		profile.Password = current.Password
	}
	normalized, err := NormalizeSocks5ExitConfig(profile)
	if err != nil {
		return err
	}
	if password == nil && current != nil && normalized.Password != "" &&
		(normalized.Host != current.Host || normalized.Port != current.Port) {
		return errors.New("enter the password for the new SOCKS5 address")
	}
	next := m.cache
	next.Settings.Socks5Exit = &normalized
	return m.saveConfigLocked(next)
}

func (m *Manager) SetSocks5ExitEnabled(enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var profile Socks5ExitConfig
	if current := m.cache.Settings.Socks5Exit; current != nil {
		profile = *current
	}
	profile.Enabled = enabled
	return m.saveSocks5ExitLocked(&profile)
}

func (m *Manager) RestoreSocks5Exit(profile *Socks5ExitConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveSocks5ExitLocked(profile)
}

func (m *Manager) ClearSocks5ExitPreviewGuard() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.cache.Settings.LegacySocks5ExitArmed {
		return nil
	}
	next := m.cache
	next.Settings.LegacySocks5ExitArmed = false
	return m.saveConfigLocked(next)
}

func (m *Manager) saveSocks5ExitLocked(profile *Socks5ExitConfig) error {
	next := m.cache
	next.Settings.Socks5Exit = profile
	return m.saveConfigLocked(next)
}
