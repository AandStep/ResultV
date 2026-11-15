package config

import (
	"errors"
	"net/netip"
	"slices"
	"strings"

	"golang.org/x/net/idna"
)

type Socks5ExitConfig struct {
	Enabled  bool     `json:"enabled,omitempty"`
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Username string   `json:"username,omitempty"`
	Password string   `json:"password,omitempty"`
	Sites    []string `json:"sites,omitempty"`
	Apps     []string `json:"apps,omitempty"`
}

var socks5DNSName = idna.New(idna.MapForLookup(), idna.VerifyDNSLength(true))

func normalizeSocks5Sites(sites []string) []string {
	var out []string
	for _, site := range sites {
		site = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(site)), ".")
		if site != "" && !slices.Contains(out, site) {
			out = append(out, site)
		}
	}
	return out
}

func normalizeSocks5Apps(apps []string) []string {
	var out []string
	for _, app := range apps {
		app = strings.TrimSpace(app)
		if app != "" && !slices.ContainsFunc(out, func(seen string) bool { return strings.EqualFold(seen, app) }) {
			out = append(out, app)
		}
	}
	return out
}

func NormalizeSocks5ExitConfig(cfg Socks5ExitConfig) (Socks5ExitConfig, error) {
	cfg.Sites, cfg.Apps = normalizeSocks5Sites(cfg.Sites), normalizeSocks5Apps(cfg.Apps)
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
