package config

import (
	"errors"
	"net/netip"
	"strings"

	"golang.org/x/net/idna"
)

type Socks5ExitConfig struct {
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Username string   `json:"username,omitempty"`
	Password string   `json:"password,omitempty"`
	Domains  []string `json:"domains"`
}

var socks5DNSName = idna.New(idna.MapForLookup(), idna.VerifyDNSLength(true))

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

	seen := make(map[string]bool, len(cfg.Domains))
	domains := make([]string, 0, len(cfg.Domains))
	for _, value := range cfg.Domains {
		if strings.TrimSpace(value) == "" {
			continue
		}
		name, err := normalizeSocks5DNSName(value)
		if err != nil {
			return Socks5ExitConfig{}, errors.New("invalid SOCKS5 target domain")
		}
		if _, err := netip.ParseAddr(name); err == nil {
			return Socks5ExitConfig{}, errors.New("SOCKS5 targets must be domain names")
		}
		if cfg.Host == name || strings.HasSuffix(cfg.Host, "."+name) {
			return Socks5ExitConfig{}, errors.New("SOCKS5 host matches a target domain")
		}
		if !seen[name] {
			domains = append(domains, name)
			seen[name] = true
		}
	}
	if len(domains) == 0 {
		return Socks5ExitConfig{}, errors.New("add at least one SOCKS5 target domain")
	}
	cfg.Domains = domains
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

	if password != nil {
		profile.Password = *password
	} else if current := m.cache.Settings.Socks5Exit; current != nil {
		profile.Password = current.Password
	}
	normalized, err := NormalizeSocks5ExitConfig(profile)
	if err != nil {
		return err
	}
	next := m.cache
	next.Settings.Socks5Exit = &normalized
	return m.saveConfigLocked(next)
}

// SetSocks5ExitArmed records whether auto-connect must stop after an unclean
// restart. It does not request SOCKS5 on a fresh user-initiated session.
func (m *Manager) SetSocks5ExitArmed(armed bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cache.Settings.Socks5ExitArmed == armed {
		return nil
	}
	next := m.cache
	next.Settings.Socks5ExitArmed = armed
	return m.saveConfigLocked(next)
}
