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

package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ProxyMode string

const (
	ProxyModeProxy  ProxyMode = "proxy"
	ProxyModeTunnel ProxyMode = "tunnel"
)

type ProxyConfig struct {
	ID       string `json:"id,omitempty"`
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Type     string `json:"type"`
	Username string `json:"username"`
	Password string `json:"password"`

	URI             string          `json:"uri,omitempty"`
	Extra           json.RawMessage `json:"extra,omitempty"`
	SubscriptionURL string          `json:"subscriptionUrl,omitempty"`
}

type EngineConfig struct {
	Proxy        ProxyConfig
	Mode         ProxyMode
	ListenAddr   string
	RoutingMode  RoutingMode
	Whitelist    []string
	AppWhitelist []string
	AdBlock      bool
	KillSwitch   bool
	LocalPort    int
	DNSServers   []string
	TunIPv4      string
	DataDir      string
}

type Engine interface {
	Start(ctx context.Context, cfg EngineConfig) error

	Stop() error

	IsRunning() bool

	GetTrafficStats() (up, down int64)
}

type SingBoxConfig struct {
	Log          *SBLog          `json:"log,omitempty"`
	DNS          *SBDNS          `json:"dns,omitempty"`
	Endpoints    []SBEndpoint    `json:"endpoints,omitempty"`
	Inbounds     []SBInbound     `json:"inbounds"`
	Outbounds    []SBOutbound    `json:"outbounds"`
	Route        *SBRoute        `json:"route,omitempty"`
	Experimental *SBExperimental `json:"experimental,omitempty"`
}

type SBExperimental struct {
	CacheFile *SBCacheFile `json:"cache_file,omitempty"`
}

type SBCacheFile struct {
	Enabled bool   `json:"enabled,omitempty"`
	Path    string `json:"path,omitempty"`
}

type SBLog struct {
	Level    string `json:"level"`
	Disabled bool   `json:"disabled"`
}

type SBDNS struct {
	Servers  []SBDNSServer `json:"servers"`
	Rules    []SBDNSRule   `json:"rules,omitempty"`
	Strategy string        `json:"strategy,omitempty"`
}

type SBDNSServer struct {
	Type            string `json:"type"`
	Tag             string `json:"tag"`
	Server          string `json:"server,omitempty"`
	ServerPort      int    `json:"server_port,omitempty"`
	Detour          string `json:"detour,omitempty"`
	AddressStrategy string `json:"address_strategy,omitempty"`
}

type SBDNSRule struct {
	Domain []string `json:"domain,omitempty"`
	Server string   `json:"server"`
	Action string   `json:"action,omitempty"`
}

type SBInbound struct {
	Type                string   `json:"type"`
	Tag                 string   `json:"tag"`
	Listen              string   `json:"listen,omitempty"`
	ListenPort          int      `json:"listen_port,omitempty"`
	Address             []string `json:"address,omitempty"`
	Stack               string   `json:"stack,omitempty"`
	AutoRoute           bool     `json:"auto_route,omitempty"`
	StrictRoute         bool     `json:"strict_route,omitempty"`
	RouteExcludeAddress []string `json:"route_exclude_address,omitempty"`
}

type SBOutbound struct {
	Type       string           `json:"type"`
	Tag        string           `json:"tag"`
	Server     string           `json:"server,omitempty"`
	ServerPort int              `json:"server_port,omitempty"`
	Username   string           `json:"username,omitempty"`
	Password   string           `json:"password,omitempty"`
	Method     string           `json:"method,omitempty"`
	Version    string           `json:"version,omitempty"`
	UUID       string           `json:"uuid,omitempty"`
	AlterId    int              `json:"alter_id,omitempty"`
	Flow       string           `json:"flow,omitempty"`
	UpMbps     int              `json:"up_mbps,omitempty"`
	DownMbps   int              `json:"down_mbps,omitempty"`
	Obfs       *SBHysteria2Obfs `json:"obfs,omitempty"`

	TLS       *SBOutboundTLS       `json:"tls,omitempty"`
	Transport *SBOutboundTransport `json:"transport,omitempty"`
	
	DomainStrategy string `json:"domain_strategy,omitempty"`
}

type SBHysteria2Obfs struct {
	Type     string `json:"type,omitempty"`
	Password string `json:"password,omitempty"`
}

type SBOutboundTLS struct {
	Enabled    bool       `json:"enabled"`
	ServerName string     `json:"server_name,omitempty"`
	Insecure   bool       `json:"insecure,omitempty"`
	ALPN       []string   `json:"alpn,omitempty"`
	UTLS       *SBUTLS    `json:"utls,omitempty"`
	Reality    *SBReality `json:"reality,omitempty"`
}

type SBEndpoint struct {
	Type          string              `json:"type"`
	Tag           string              `json:"tag"`
	Detour        string              `json:"detour,omitempty"`
	System        bool                `json:"system,omitempty"`
	Name          string              `json:"name,omitempty"`
	MTU           int                 `json:"mtu,omitempty"`
	Address       []string            `json:"address,omitempty"`
	PrivateKey    string              `json:"private_key,omitempty"`
	ListenPort    int                 `json:"listen_port,omitempty"`
	Peers         []SBWireGuardPeer   `json:"peers,omitempty"`
	UDPTimeout    string              `json:"udp_timeout,omitempty"`
	Workers       int                 `json:"workers,omitempty"`
	DisablePauses bool                `json:"disable_pauses,omitempty"`
	Amnezia       *SBWireGuardAmnezia `json:"amnezia,omitempty"`
}

type SBWireGuardPeer struct {
	Address                     string   `json:"address,omitempty"`
	Port                        int      `json:"port,omitempty"`
	PublicKey                   string   `json:"public_key,omitempty"`
	PreSharedKey                string   `json:"pre_shared_key,omitempty"`
	AllowedIPs                  []string `json:"allowed_ips,omitempty"`
	PersistentKeepaliveInterval int      `json:"persistent_keepalive_interval,omitempty"`
	Reserved                    []int    `json:"reserved,omitempty"`
}

type SBWireGuardAmnezia struct {
	JC    int    `json:"jc,omitempty"`
	JMin  int    `json:"jmin,omitempty"`
	JMax  int    `json:"jmax,omitempty"`
	S1    int    `json:"s1,omitempty"`
	S2    int    `json:"s2,omitempty"`
	S3    int    `json:"s3,omitempty"`
	S4    int    `json:"s4,omitempty"`
	H1    uint32 `json:"h1,omitempty"`
	H2    uint32 `json:"h2,omitempty"`
	H3    uint32 `json:"h3,omitempty"`
	H4    uint32 `json:"h4,omitempty"`
	I1    string `json:"i1,omitempty"`
	I2    string `json:"i2,omitempty"`
	I3    string `json:"i3,omitempty"`
	I4    string `json:"i4,omitempty"`
	I5    string `json:"i5,omitempty"`
	J1    string `json:"j1,omitempty"`
	J2    string `json:"j2,omitempty"`
	J3    string `json:"j3,omitempty"`
	ITime int64  `json:"itime,omitempty"`
}

type SBUTLS struct {
	Enabled     bool   `json:"enabled"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

type SBReality struct {
	Enabled   bool   `json:"enabled"`
	PublicKey string `json:"public_key"`
	ShortID   string `json:"short_id,omitempty"`
}

type SBOutboundTransport struct {
	Type          string            `json:"type"`
	Path          string            `json:"path,omitempty"`
	Host          string            `json:"host,omitempty"`
	ServiceName   string            `json:"service_name,omitempty"`
	Authority     string            `json:"authority,omitempty"`
	Mode          string            `json:"mode,omitempty"`
	XPaddingBytes string            `json:"x_padding_bytes,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`

	UplinkHTTPMethod     string          `json:"uplink_http_method,omitempty"`
	NoGRPCHeader         *bool           `json:"no_grpc_header,omitempty"`
	NoSSEHeader          *bool           `json:"no_sse_header,omitempty"`
	ScMaxEachPostBytes   json.RawMessage `json:"sc_max_each_post_bytes,omitempty"`
	ScMinPostsIntervalMs json.RawMessage `json:"sc_min_posts_interval_ms,omitempty"`
	ScStreamUpServerSecs json.RawMessage `json:"sc_stream_up_server_secs,omitempty"`
	Xmux                 json.RawMessage `json:"xmux,omitempty"`
}

type SBRoute struct {
	Rules      []SBRouteRule `json:"rules,omitempty"`
	Final      string        `json:"final,omitempty"`
	AutoDetect bool          `json:"auto_detect_interface,omitempty"`
}

type SBRouteRule struct {
	Protocol         []string `json:"protocol,omitempty"`
	Network          []string `json:"network,omitempty"`
	Port             []int    `json:"port,omitempty"`
	Domain           []string `json:"domain,omitempty"`
	DomainSuffix     []string `json:"domain_suffix,omitempty"`
	IPCidr           []string `json:"ip_cidr,omitempty"`
	ProcessName      []string `json:"process_name,omitempty"`
	ProcessPathRegex []string `json:"process_path_regex,omitempty"`
	Outbound         string   `json:"outbound,omitempty"`
	Action           string   `json:"action,omitempty"`
}

func effectiveDataDir(cfg EngineConfig) string {
	if cfg.DataDir != "" {
		return cfg.DataDir
	}
	return resultProxyDataDir()
}

func buildExperimentalCache(dataDir string) *SBExperimental {
	if dataDir == "" {
		return nil
	}
	return &SBExperimental{
		CacheFile: &SBCacheFile{
			Enabled: true,
			Path:    filepath.Join(dataDir, "sing-box-cache.db"),
		},
	}
}

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

func BuildProxyModeConfig(cfg EngineConfig) SingBoxConfig {
	port := cfg.LocalPort
	if port == 0 {
		port = getFreeLocalPort(14081)
	}

	host, _ := splitHostPort(cfg.ListenAddr, "127.0.0.1", port)

	dd := effectiveDataDir(cfg)
	config := SingBoxConfig{
		Log:       &SBLog{Level: "error", Disabled: true},
		DNS:       buildDNS(cfg),
		Endpoints: buildEndpoints(cfg.Proxy),
		Inbounds: []SBInbound{{
			Type:       "mixed",
			Tag:        "mixed-in",
			Listen:     host,
			ListenPort: port,
		}},
		Outbounds:    buildOutbounds(cfg.Proxy),
		Route:        buildRoute(cfg),
		Experimental: buildExperimentalCache(dd),
	}

	return config
}

func BuildTunnelModeConfig(cfg EngineConfig) SingBoxConfig {
	tunIPv4 := "172.19.0.1/30"
	if cfg.TunIPv4 != "" {
		tunIPv4 = cfg.TunIPv4
	}
	tunAddresses := []string{tunIPv4}
	tunStack := "gvisor"
	strictRoute := true

	pt := strings.ToUpper(strings.TrimSpace(cfg.Proxy.Type))

	if pt == "WIREGUARD" || pt == "AMNEZIAWG" {
		tunStack = "system"
		strictRoute = false
	}

	if pt == "HYSTERIA2" || pt == "TROJAN" {
		strictRoute = false
	}

	if pt == "WIREGUARD" || pt == "AMNEZIAWG" {
		tunAddresses = []string{tunIPv4}
	}

	var routeExclude []string
	if pt != "WIREGUARD" && pt != "AMNEZIAWG" {
		if serverIP := net.ParseIP(cfg.Proxy.IP); serverIP != nil {
			cidr := cfg.Proxy.IP + "/32"
			if serverIP.To4() == nil {
				cidr = cfg.Proxy.IP + "/128"
			}
			routeExclude = append(routeExclude, cidr)
		}
	}

	dd := effectiveDataDir(cfg)
	outbounds := buildOutbounds(cfg.Proxy)

	config := SingBoxConfig{
		Log:       &SBLog{Level: "error", Disabled: false},
		DNS:       buildDNS(cfg),
		Endpoints: buildEndpoints(cfg.Proxy),
		Inbounds: []SBInbound{{
			Type:                "tun",
			Tag:                 "tun-in",
			Address:             tunAddresses,
			Stack:               tunStack,
			AutoRoute:           true,
			StrictRoute:         strictRoute,
			RouteExcludeAddress: routeExclude,
		}},
		Outbounds:    outbounds,
		Route:        buildRoute(cfg),
		Experimental: buildExperimentalCache(dd),
	}

	return config
}

func buildOutbounds(proxy ProxyConfig) []SBOutbound {
	pt := strings.ToUpper(strings.TrimSpace(proxy.Type))
	if pt == "WIREGUARD" || pt == "AMNEZIAWG" {
		return []SBOutbound{
			{Type: "direct", Tag: "direct"},
			{Type: "block", Tag: "block"},
		}
	}
	outbounds := []SBOutbound{
		{Type: "direct", Tag: "direct"},
		{Type: "block", Tag: "block"},
		buildProxyOutbound(proxy),
	}
	return outbounds
}

func buildDNS(cfg EngineConfig) *SBDNS {
	if cfg.Mode == ProxyModeTunnel {

		pt := strings.ToUpper(strings.TrimSpace(cfg.Proxy.Type))
		isEndpoint := pt == "WIREGUARD" || pt == "AMNEZIAWG"

		detour := "proxy"
		if isEndpoint {
			detour = ""
		}

		servers := []SBDNSServer{}
		if len(cfg.DNSServers) > 0 {
			for i, raw := range cfg.DNSServers {
				server, port := splitDNSServer(raw)
				if server == "" {
					continue
				}
				srvType := "udp"
				if detour != "" {
					srvType = "tcp"
				}
				servers = append(servers, SBDNSServer{
					Type:       srvType,
					Tag:        fmt.Sprintf("custom-%d", i+1),
					Server:     server,
					ServerPort: port,
					Detour:     detour,
				})
			}
			servers = append(servers, SBDNSServer{Type: "local", Tag: "local"})
		} else {
			if detour != "" {
				servers = []SBDNSServer{
					{Type: "tcp", Tag: "google-tcp", Server: "8.8.8.8", Detour: detour},
					{Type: "tcp", Tag: "cloudflare-tcp", Server: "1.1.1.1", Detour: detour},
					{Type: "tls", Tag: "google-tls", Server: "8.8.8.8", Detour: detour},
					{Type: "tls", Tag: "cloudflare-tls", Server: "1.1.1.1", Detour: detour},
					{Type: "local", Tag: "local"},
				}
			} else {
				servers = []SBDNSServer{
					{Type: "udp", Tag: "udp", Server: "8.8.8.8", Detour: detour},
					{Type: "tls", Tag: "google", Server: "8.8.8.8", Detour: detour},
					{Type: "tls", Tag: "cloudflare", Server: "1.1.1.1", Detour: detour},
					{Type: "local", Tag: "local"},
				}
			}
		}

		dns := &SBDNS{
			Servers: servers,
		}

		dns.Strategy = "ipv4_only"

		if detour != "" && cfg.Proxy.IP != "" && net.ParseIP(cfg.Proxy.IP) == nil {
			dns.Rules = append(dns.Rules, SBDNSRule{
				Domain: []string{cfg.Proxy.IP},
				Server: "local",
			})
		}

		return dns
	}

	// proxy mode: прямой UDP DNS без detour.
	// Роутинг DNS через proxy-outbound создаёт circular dependency:
	// DNS нужен для резолва трафика → DNS идёт через proxy → proxy нужно соединение → ...
	// В proxy-режиме DNS-leaks несущественны (приложения используют системный прокси).
	servers := []SBDNSServer{}
	if len(cfg.DNSServers) > 0 {
		for i, raw := range cfg.DNSServers {
			server, port := splitDNSServer(raw)
			if server == "" {
				continue
			}
			servers = append(servers, SBDNSServer{
				Type:       "udp",
				Tag:        fmt.Sprintf("custom-%d", i+1),
				Server:     server,
				ServerPort: port,
			})
		}
		servers = append(servers, SBDNSServer{Type: "local", Tag: "local"})
	} else {
		servers = []SBDNSServer{
			{Type: "udp", Tag: "google", Server: "8.8.8.8"},
			{Type: "udp", Tag: "cloudflare", Server: "1.1.1.1"},
			{Type: "local", Tag: "local"},
		}
	}

	return &SBDNS{Servers: servers}
}

func splitDNSServer(raw string) (string, int) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", 0
	}
	if host, portStr, err := net.SplitHostPort(s); err == nil {
		if n, err := strconv.Atoi(portStr); err == nil && n > 0 {
			return host, n
		}
		return host, 0
	}
	if strings.Count(s, ":") == 1 {
		parts := strings.SplitN(s, ":", 2)
		if len(parts) == 2 {
			host := strings.TrimSpace(parts[0])
			if host != "" {
				if n, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && n > 0 {
					return host, n
				}
			}
		}
	}
	return s, 0
}

func buildRoute(cfg EngineConfig) *SBRoute {
	route := &SBRoute{
		Final:      "proxy",
		AutoDetect: true,
	}

	var rules []SBRouteRule

	if cfg.Mode == ProxyModeTunnel {
		if serverIP := net.ParseIP(cfg.Proxy.IP); serverIP != nil {
			cidr := cfg.Proxy.IP + "/32"
			if serverIP.To4() == nil {
				cidr = cfg.Proxy.IP + "/128"
			}
			rules = append(rules, SBRouteRule{
				Action:   "route",
				IPCidr:   []string{cidr},
				Outbound: "direct",
			})
		} else if cfg.Proxy.IP != "" {

			rules = append(rules, SBRouteRule{
				Action:   "route",
				Domain:   []string{cfg.Proxy.IP},
				Outbound: "direct",
			})
		}
	}

	rules = append(rules, SBRouteRule{
		Action: "sniff",
	})

	rules = append(rules, SBRouteRule{
		Protocol: []string{"dns"},
		Action:   "hijack-dns",
	})

	isEndpointProtocol := strings.EqualFold(strings.TrimSpace(cfg.Proxy.Type), "wireguard") ||
		strings.EqualFold(strings.TrimSpace(cfg.Proxy.Type), "amneziawg")
	if cfg.Mode == ProxyModeTunnel && !isEndpointProtocol {
		// Probe domains must go through the proxy outbound, even when issued
		// from the app's own process. Without this, the self-direct rule below
		// would route the post-start HTTP probe out via direct, masking a broken
		// SS/VLESS/VMESS tunnel as healthy.
		if len(tunnelProbeDomains) > 0 {
			rules = append(rules, SBRouteRule{
				Action:   "route",
				Domain:   append([]string(nil), tunnelProbeDomains...),
				Outbound: "proxy",
			})
		}
		if exe, err := os.Executable(); err == nil {
			if base := filepath.Base(exe); base != "" && base != "." {
				rx := `(?i)(^|[\\/])` + regexp.QuoteMeta(base) + `$`
				rules = append(rules, SBRouteRule{
					Action:           "route",
					ProcessPathRegex: []string{rx},
					Outbound:         "direct",
				})
			}
		}
	}

	if rx := appWhitelistPathRegexes(cfg.AppWhitelist); len(rx) > 0 {
		rules = append(rules, SBRouteRule{
			Action:           "route",
			ProcessPathRegex: rx,
			Outbound:         "direct",
		})
	}

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

func getFreeLocalPort(defaultPort int) int {
	if defaultPort > 0 {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", defaultPort))
		if err == nil {
			ln.Close()
			return defaultPort
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 14081
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
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

func PingProxy(ip string, port int) (latencyMs int64, reachable bool, reason string) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, port), 5*time.Second)
	elapsed := time.Since(start)
	if err != nil {
		return 0, false, pingReasonFromError(err)
	}
	conn.Close()
	return elapsed.Milliseconds(), true, ""
}

func PingHysteria2QUIC(ip string, port int) (latencyMs int64, reachable bool, reason, checkType string) {

	latency, ok, r := PingProxyUDP(ip, port)
	if ok {
		return latency, true, "", "udp"
	}

	tcpLat, tcpOK, tcpR := PingProxy(ip, port)
	if tcpOK {
		return tcpLat, true, "", "tcp_fallback"
	}
	if r == "" {
		r = tcpR
	}
	return 0, false, r, "udp"
}

func PingProxyUDP(ip string, port int) (latencyMs int64, reachable bool, reason string) {
	addr := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("udp", addr, 3*time.Second)
	if err != nil {
		return 0, false, pingReasonFromError(err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	start := time.Now()
	_, _ = conn.Write([]byte{0x00})
	buf := make([]byte, 1)
	_, readErr := conn.Read(buf)
	elapsed := time.Since(start)
	if readErr != nil {
		if ne, ok := readErr.(net.Error); ok && ne.Timeout() {

			return -1, true, ""
		}
		msg := strings.ToLower(readErr.Error())
		if strings.Contains(msg, "refused") {
			return 0, false, "connection_refused"
		}

		return -1, true, ""
	}

	return elapsed.Milliseconds(), true, ""
}

func pingReasonFromError(err error) string {
	if err == nil {
		return ""
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return "timeout"
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		msg := strings.ToLower(opErr.Err.Error())
		switch {
		case strings.Contains(msg, "refused"):
			return "connection_refused"
		case strings.Contains(msg, "unreachable"):
			return "network_unreachable"
		case strings.Contains(msg, "no route"):
			return "no_route_to_host"
		case strings.Contains(msg, "i/o timeout"):
			return "timeout"
		}
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "refused"):
		return "connection_refused"
	case strings.Contains(msg, "unreachable"):
		return "network_unreachable"
	case strings.Contains(msg, "no route"):
		return "no_route_to_host"
	case strings.Contains(msg, "timeout"):
		return "timeout"
	case strings.Contains(msg, "forcibly closed"):
		return "connection_closed"
	}
	return "probe_error"
}
