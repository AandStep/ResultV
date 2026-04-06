// Copyright (C) 2026 ResultProxy
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ProxyMode determines the operating mode.
type ProxyMode string

const (
	ProxyModeProxy  ProxyMode = "proxy"  // System proxy via mixed inbound
	ProxyModeTunnel ProxyMode = "tunnel" // TUN interface (all traffic)
)

// ProxyConfig describes the upstream proxy server to connect through.
type ProxyConfig struct {
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Type     string `json:"type"`     // "http", "socks5", "ss", "vmess", "vless", "trojan", etc.
	Username string `json:"username"`
	Password string `json:"password"`
	// Extended protocol fields.
	URI   string          `json:"uri,omitempty"`
	Extra json.RawMessage `json:"extra,omitempty"`
}

// EngineConfig holds all parameters for the proxy engine.
type EngineConfig struct {
	Proxy        ProxyConfig
	Mode         ProxyMode
	ListenAddr   string // e.g. "127.0.0.1:14081"
	RoutingMode  RoutingMode
	Whitelist    []string
	AppWhitelist []string
	AdBlock      bool
	KillSwitch   bool
}

// Engine is the interface for the proxy core.
// Implementations: SingBoxEngine (production), MockEngine (testing).
type Engine interface {
	// Start launches the proxy engine with the given configuration.
	Start(ctx context.Context, cfg EngineConfig) error
	// Stop gracefully shuts down the engine.
	Stop() error
	// IsRunning returns whether the engine is currently active.
	IsRunning() bool
	// GetTrafficStats returns current upload/download bytes.
	GetTrafficStats() (up, down int64)
}

// --- sing-box JSON config builder ---
// We build the config as a JSON-serializable struct that matches
// sing-box 1.12+ config format, then pass it programmatically.

// SingBoxConfig represents the top-level sing-box configuration (1.12+).
type SingBoxConfig struct {
	Log       *SBLog        `json:"log,omitempty"`
	DNS       *SBDNS        `json:"dns,omitempty"`
	Inbounds  []SBInbound   `json:"inbounds"`
	Outbounds []SBOutbound  `json:"outbounds"`
	Route     *SBRoute      `json:"route,omitempty"`
}

type SBLog struct {
	Level    string `json:"level"`
	Disabled bool   `json:"disabled"`
}

type SBDNS struct {
	Servers []SBDNSServer `json:"servers"`
	Rules   []SBDNSRule   `json:"rules,omitempty"`
}

type SBDNSServer struct {
	Type       string `json:"type"`
	Tag        string `json:"tag"`
	Server     string `json:"server,omitempty"`
	ServerPort int    `json:"server_port,omitempty"`
}

type SBDNSRule struct {
	Domain []string `json:"domain,omitempty"`
	Server string   `json:"server"`
	Action string   `json:"action,omitempty"` // 1.11+: "block", "reject"
}

type SBInbound struct {
	Type       string   `json:"type"`
	Tag        string   `json:"tag"`
	Listen     string   `json:"listen,omitempty"`
	ListenPort int      `json:"listen_port,omitempty"`
	Address    []string `json:"address,omitempty"` // TUN: CIDR addresses (1.12+)
	Stack      string   `json:"stack,omitempty"`   // TUN: "gvisor"
	AutoRoute  bool     `json:"auto_route,omitempty"`
	StrictRoute bool    `json:"strict_route,omitempty"`
}

type SBOutbound struct {
	Type       string `json:"type"`
	Tag        string `json:"tag"`
	Server     string `json:"server,omitempty"`
	ServerPort int    `json:"server_port,omitempty"`
	Username   string `json:"username,omitempty"` // HTTP/SOCKS5
	Password   string `json:"password,omitempty"`
	Method     string `json:"method,omitempty"`   // shadowsocks
	Version    string `json:"version,omitempty"`  // socks
	UUID       string `json:"uuid,omitempty"`     // VMess/VLess/Trojan
	Flow       string `json:"flow,omitempty"`     // VLESS: xtls-rprx-vision
	// TLS + транспорт для VMess/VLess/Trojan
	TLS       *SBOutboundTLS       `json:"tls,omitempty"`
	Transport *SBOutboundTransport `json:"transport,omitempty"`
}

// SBOutboundTLS описывает TLS-настройки для outbound.
type SBOutboundTLS struct {
	Enabled    bool       `json:"enabled"`
	ServerName string     `json:"server_name,omitempty"`
	Insecure   bool       `json:"insecure,omitempty"`
	ALPN       []string   `json:"alpn,omitempty"`
	UTLS       *SBUTLS    `json:"utls,omitempty"`
	Reality    *SBReality `json:"reality,omitempty"`
}

// SBUTLS configures the uTLS client fingerprint.
type SBUTLS struct {
	Enabled     bool   `json:"enabled"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// SBReality configures VLESS Reality TLS parameters.
type SBReality struct {
	Enabled   bool   `json:"enabled"`
	PublicKey string `json:"public_key"`
	ShortID   string `json:"short_id,omitempty"`
}

// SBOutboundTransport описывает транспортный слой (WebSocket, gRPC, HTTP/2, XHTTP).
type SBOutboundTransport struct {
	Type        string            `json:"type"`
	Path        string            `json:"path,omitempty"`         // WebSocket / XHTTP path
	Host        string            `json:"host,omitempty"`         // WebSocket Host header
	ServiceName string            `json:"service_name,omitempty"` // gRPC service name
	Mode        string            `json:"mode,omitempty"`         // XHTTP mode (auto, packet-up, stream-up, stream-one)
	XPaddingBytes string          `json:"x_padding_bytes,omitempty"` // XHTTP padding range string, e.g. "100-1000" (sing-box Range)
	Headers     map[string]string `json:"headers,omitempty"`      // extra headers (not Host — sing-box forbids)
	// XHTTP (sing-box-extended V2RayXHTTPBaseOptions JSON names; link "method" maps here)
	UplinkHTTPMethod string          `json:"uplink_http_method,omitempty"`
	NoGRPCHeader     *bool           `json:"no_grpc_header,omitempty"`
	NoSSEHeader      *bool           `json:"no_sse_header,omitempty"`
	ScMaxEachPostBytes   json.RawMessage `json:"sc_max_each_post_bytes,omitempty"`
	ScMinPostsIntervalMs json.RawMessage `json:"sc_min_posts_interval_ms,omitempty"`
	ScStreamUpServerSecs json.RawMessage `json:"sc_stream_up_server_secs,omitempty"`
	Xmux             json.RawMessage `json:"xmux,omitempty"`
}

type SBRoute struct {
	Rules        []SBRouteRule `json:"rules,omitempty"`
	Final        string        `json:"final,omitempty"`
	AutoDetect   bool          `json:"auto_detect_interface,omitempty"`
}

type SBRouteRule struct {
	Protocol         []string `json:"protocol,omitempty"`
	Domain           []string `json:"domain,omitempty"`
	DomainSuffix     []string `json:"domain_suffix,omitempty"`
	ProcessName      []string `json:"process_name,omitempty"`
	ProcessPathRegex []string `json:"process_path_regex,omitempty"`
	Outbound         string   `json:"outbound,omitempty"`
	Action           string   `json:"action,omitempty"` // 1.11+: "block", "hijack-dns"
}

// appWhitelistPathRegexes builds case-insensitive regexes that match the executable
// basename at the end of a full process path. sing-box process_name matching is
// case-sensitive on filepath.Base(); the UI lowercases names, which breaks on Windows.
func appWhitelistPathRegexes(names []string) []string {
	seen := make(map[string]struct{}, len(names))
	var out []string
	for _, w := range names {
		n := strings.TrimSpace(w)
		if n == "" {
			continue
		}
		key := strings.ToLower(n)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		esc := regexp.QuoteMeta(n)
		out = append(out, `(?i)(^|[\\/])`+esc+`$`)
	}
	return out
}

// BuildProxyModeConfig creates a sing-box config for system proxy mode.
// Mixed inbound (HTTP+SOCKS5) on the listen address.
func BuildProxyModeConfig(cfg EngineConfig) SingBoxConfig {
	host, port := splitHostPort(cfg.ListenAddr, "127.0.0.1", 14081)

	config := SingBoxConfig{
		Log: &SBLog{Level: "error", Disabled: true},
		DNS: buildDNS(cfg),
		Inbounds: []SBInbound{{
			Type:       "mixed",
			Tag:        "mixed-in",
			Listen:     host,
			ListenPort: port,
		}},
		Outbounds: buildOutbounds(cfg.Proxy),
		Route:     buildRoute(cfg),
	}

	return config
}

// BuildTunnelModeConfig creates a sing-box config for TUN mode.
// TUN interface captures all system traffic.
func BuildTunnelModeConfig(cfg EngineConfig) SingBoxConfig {
	config := SingBoxConfig{
		Log: &SBLog{Level: "error", Disabled: true},
		DNS: buildDNS(cfg),
		Inbounds: []SBInbound{{
			Type:        "tun",
			Tag:         "tun-in",
			Address:     []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"},
			Stack:       "gvisor",
			AutoRoute:   true,
			StrictRoute: true,
		}},
		Outbounds: buildOutbounds(cfg.Proxy),
		Route:     buildRoute(cfg),
	}

	return config
}

func buildOutbounds(proxy ProxyConfig) []SBOutbound {
	outbounds := []SBOutbound{
		{Type: "direct", Tag: "direct"},
		{Type: "block", Tag: "block"}, // Для AdBlock и блокировки DNS
		buildProxyOutbound(proxy),
	}
	return outbounds
}



func buildDNS(cfg EngineConfig) *SBDNS {
	if cfg.Mode == ProxyModeTunnel {
		return &SBDNS{
			Servers: []SBDNSServer{
				{Type: "udp", Tag: "udp", Server: "8.8.8.8"},
				{Type: "tls", Tag: "google", Server: "8.8.8.8"},
				{Type: "tls", Tag: "cloudflare", Server: "1.1.1.1"},
				{Type: "local", Tag: "local"},
			},
		}
	}

	return &SBDNS{
		Servers: []SBDNSServer{
			{Type: "udp", Tag: "google", Server: "8.8.8.8"},
			{Type: "udp", Tag: "cloudflare", Server: "1.1.1.1"},
			{Type: "local", Tag: "local"},
		},
	}
}

func buildRoute(cfg EngineConfig) *SBRoute {
	route := &SBRoute{
		Final:      "proxy", // default: all traffic through proxy
		AutoDetect: true,
	}

	var rules []SBRouteRule

	// Sniff rule action replaces legacy inbound sniff/sniff_override_destination (1.11+).
	rules = append(rules, SBRouteRule{
		Action: "sniff",
	})

	// DNS hijacking rule (1.11+ action-based).
	rules = append(rules, SBRouteRule{
		Protocol: []string{"dns"},
		Action:   "hijack-dns",
	})

	// App whitelist: bypass by process path regex (case-insensitive basename match).
	if rx := appWhitelistPathRegexes(cfg.AppWhitelist); len(rx) > 0 {
		rules = append(rules, SBRouteRule{
			Action:           "route",
			ProcessPathRegex: rx,
			Outbound:         "direct",
		})
	}

	// Domain whitelist with nested exceptions support.
	// Example: [".ru", "2ip.ru"] -> ru=direct, 2ip.ru=proxy (override).
	if len(cfg.Whitelist) > 0 {
		seen := make(map[string]struct{}, len(cfg.Whitelist))
		var normalized []string
		for _, w := range cfg.Whitelist {
			n := normalizeRule(w)
			if n == "" {
				continue
			}
			if _, ok := seen[n]; ok {
				continue
			}
			seen[n] = struct{}{}
			normalized = append(normalized, n)
		}

		if len(normalized) > 0 {
			ordered := append([]string(nil), normalized...)
			sort.SliceStable(ordered, func(i, j int) bool {
				di := strings.Count(ordered[i], ".")
				dj := strings.Count(ordered[j], ".")
				if di != dj {
					return di > dj
				}
				if len(ordered[i]) != len(ordered[j]) {
					return len(ordered[i]) > len(ordered[j])
				}
				return ordered[i] < ordered[j]
			})

			isWhitelisted := func(host string, all []string) bool {
				matchCount := 0
				for _, rule := range all {
					if host == rule || strings.HasSuffix(host, "."+rule) {
						matchCount++
					}
				}
				return matchCount > 0 && matchCount%2 == 1
			}

			for _, suffix := range ordered {
				outbound := "proxy"
				if isWhitelisted(suffix, normalized) {
					outbound = "direct"
				}
				rules = append(rules, SBRouteRule{
					Action:       "route",
					DomainSuffix: []string{suffix},
					Outbound:     outbound,
				})
			}
		}
	}

	route.Rules = rules
	return route
}

func splitHostPort(addr, defaultHost string, defaultPort int) (string, int) {
	if addr == "" {
		return defaultHost, defaultPort
	}
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return defaultHost, defaultPort
	}
	port := defaultPort
	if n, err := net.LookupPort("tcp", portStr); err == nil {
		port = n
	}
	return host, port
}

// PingProxy tests TCP connectivity to a proxy server.
func PingProxy(ip string, port int) (latencyMs int64, reachable bool) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, port), 5*time.Second)
	if err != nil {
		return 0, false
	}
	conn.Close()
	return time.Since(start).Milliseconds(), true
}
