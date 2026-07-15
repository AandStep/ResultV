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
	"runtime"
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
	TunIPv6      string
	DataDir      string
	// IsAndroid must be set to true when building a config for Android.
	// We cannot rely on runtime.GOOS because gomobile bind compiles on the
	// host OS (Windows/Linux/macOS), so runtime.GOOS is never "android".
	IsAndroid bool
	// IPv6 enables dual-stack: TUN gets an IPv6 address, DNS resolves AAAA,
	// and IPv6 traffic is routed through the tunnel. Off by default —
	// matches the historical Android behaviour where strategy was forced
	// to ipv4_only and only the v4 TUN address was attached.
	IPv6 bool
	// BypassLAN routes RFC1918 / link-local / multicast traffic to the
	// `direct` outbound instead of through the proxy. Lets users access
	// their printer / NAS / router admin while the VPN is up.
	BypassLAN bool
	// LogLevel overrides sing-box's log.level. Empty → "info" in
	// release-style builds, "debug" only when explicitly requested. The
	// mobile wrapper used to hardcode debug; that was too chatty for ship.
	LogLevel string
	// SmartMode (Antizapret) routes only blocked / listed domains through
	// the proxy; everything else goes direct. The blocked list comes from
	// [SmartBlockedDomains], populated by the caller (typically the
	// blocked_provider HTTP fetcher) — we no longer rely on sing-box's
	// remote rule_set because the upstream geosite-category-blocked.srs
	// release was renamed/removed in 2026.
	SmartMode bool
	// SmartBlockedDomains is the resolved list of domains that should
	// route through the proxy when [SmartMode] is on. Each entry is a
	// suffix-matched domain (e.g. "instagram.com" matches "x.instagram.com").
	// Empty + SmartMode=true means the user enabled Smart but no list was
	// fetched yet — fall back to Global (final=proxy) until the list arrives.
	SmartBlockedDomains []string
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
	ClashAPI  *SBClashAPI  `json:"clash_api,omitempty"`
}

// SBClashAPI flips on sing-box's clash-style traffic manager. We don't
// expose the HTTP controller (no `external_controller`); the in-process
// libbox CommandClient reads the same TrafficManager via its status
// subscription, which is all the Android UI needs.
type SBClashAPI struct {
	ExternalController string `json:"external_controller,omitempty"`
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
	Domain       []string `json:"domain,omitempty"`
	DomainSuffix []string `json:"domain_suffix,omitempty"`
	RuleSet      []string `json:"rule_set,omitempty"`
	Server       string   `json:"server,omitempty"`
	Action       string   `json:"action,omitempty"`
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
	PacketEncoding      string `json:"packet_encoding,omitempty"`
	GlobalPadding       bool   `json:"global_padding,omitempty"`
	AuthenticatedLength bool   `json:"authenticated_length,omitempty"`
	Security            string `json:"security,omitempty"`
	UpMbps     int              `json:"up_mbps,omitempty"`
	DownMbps   int              `json:"down_mbps,omitempty"`
	Obfs       *SBHysteria2Obfs `json:"obfs,omitempty"`

	TLS       *SBOutboundTLS       `json:"tls,omitempty"`
	Transport *SBOutboundTransport `json:"transport,omitempty"`

	DomainStrategy string `json:"domain_strategy,omitempty"`

	// urltest/selector group fields (mobile kill-switch only; omitempty keeps
	// every existing outbound's JSON byte-identical).
	Outbounds []string `json:"outbounds,omitempty"`
	URL       string   `json:"url,omitempty"`
	Interval  string   `json:"interval,omitempty"`
}

type SBHysteria2Obfs struct {
	Type     string `json:"type,omitempty"`
	Password string `json:"password,omitempty"`
}

type SBOutboundTLS struct {
	Enabled      bool       `json:"enabled"`
	ServerName   string     `json:"server_name,omitempty"`
	Insecure     bool       `json:"insecure,omitempty"`
	ALPN         []string   `json:"alpn,omitempty"`
	MinVersion   string     `json:"min_version,omitempty"`
	MaxVersion   string     `json:"max_version,omitempty"`
	CipherSuites []string   `json:"cipher_suites,omitempty"`
	UTLS         *SBUTLS    `json:"utls,omitempty"`
	Reality      *SBReality `json:"reality,omitempty"`
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
	// H1-H4 are emitted as strings ("N" or "low-high") so that
	// upstream sing-box-extended (>= v1.13.11-extended-2.0.0) can
	// parse them into *Xbadoption.Range and randomize per packet
	// for AmneziaWG 2.0 H-range support.
	H1    string `json:"h1,omitempty"`
	H2    string `json:"h2,omitempty"`
	H3    string `json:"h3,omitempty"`
	H4    string `json:"h4,omitempty"`
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
	SpiderX   string `json:"spider_x,omitempty"`
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

	MaxEarlyData        int    `json:"max_early_data,omitempty"`
	EarlyDataHeaderName string `json:"early_data_header_name,omitempty"`

	UplinkHTTPMethod     string          `json:"uplink_http_method,omitempty"`
	NoGRPCHeader         *bool           `json:"no_grpc_header,omitempty"`
	IdleTimeout          string          `json:"idle_timeout,omitempty"`
	PingTimeout          string          `json:"ping_timeout,omitempty"`
	PermitWithoutStream  bool            `json:"permit_without_stream,omitempty"`
	Method               string          `json:"method,omitempty"`
	NoSSEHeader          *bool           `json:"no_sse_header,omitempty"`
	ScMaxEachPostBytes   json.RawMessage `json:"sc_max_each_post_bytes,omitempty"`
	ScMinPostsIntervalMs json.RawMessage `json:"sc_min_posts_interval_ms,omitempty"`
	ScStreamUpServerSecs json.RawMessage `json:"sc_stream_up_server_secs,omitempty"`
	Xmux                 json.RawMessage `json:"xmux,omitempty"`
}

type SBRoute struct {
	Rules      []SBRouteRule    `json:"rules,omitempty"`
	RuleSet    []SBRouteRuleSet `json:"rule_set,omitempty"`
	Final      string           `json:"final,omitempty"`
	AutoDetect bool             `json:"auto_detect_interface,omitempty"`
}

// SBRouteRuleSet describes a remote (or local) sing-box rule_set. Used by
// Smart-mode routing to pull e.g. the Antizapret / Russia-blocked list and
// match only those domains for proxy routing.
type SBRouteRuleSet struct {
	Type           string `json:"type"`
	Tag            string `json:"tag"`
	Format         string `json:"format,omitempty"`
	URL            string `json:"url,omitempty"`
	Path           string `json:"path,omitempty"`
	DownloadDetour string `json:"download_detour,omitempty"`
	UpdateInterval string `json:"update_interval,omitempty"`
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
	RuleSet          []string `json:"rule_set,omitempty"`
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
		// Even without a writable cache, enable clash_api so the libbox
		// status subscription has a TrafficManager to read.
		return &SBExperimental{
			ClashAPI: &SBClashAPI{},
		}
	}
	return &SBExperimental{
		CacheFile: &SBCacheFile{
			Enabled: true,
			Path:    filepath.Join(dataDir, "sing-box-cache.db"),
		},
		// clash_api with no external_controller spins up the in-process
		// TrafficManager (uplink/downlink/connection counts) without
		// opening any TCP port. The mobile UI consumes it via the libbox
		// CommandClient status subscription; desktop will keep ignoring
		// it because it tracks bytes through its own trafficTracker on
		// the sing-box adapter callbacks.
		ClashAPI: &SBClashAPI{},
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
	// Dual-stack TUN when IPv6 is requested. Pick a fixed ULA prefix in
	// fc00::/7 (RFC 4193) — small /126 is enough for the point-to-point
	// link to gvisor, and stays out of any real on-link IPv6 the device
	// might be using.
	if cfg.IPv6 {
		tunIPv6 := "fdfe:dcba:9876::1/126"
		if cfg.TunIPv6 != "" {
			tunIPv6 = cfg.TunIPv6
		}
		tunAddresses = append(tunAddresses, tunIPv6)
	}
	tunStack := "gvisor"
	strictRoute := true

	pt := strings.ToUpper(strings.TrimSpace(cfg.Proxy.Type))

	if pt == "WIREGUARD" || pt == "AMNEZIAWG" {
		tunStack = "system"
		strictRoute = false
	}

	if pt == "HYSTERIA2" || pt == "TROJAN" || pt == "SS" || pt == "SHADOWSOCKS" {
		strictRoute = false
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

	// Default sing-box log level. `error` keeps logcat quiet on ship;
	// callers (mobile wrapper) can override via cfg.LogLevel for debug
	// builds where they want the REALITY/XHTTP transport noise.
	logLevel := strings.ToLower(strings.TrimSpace(cfg.LogLevel))
	if logLevel == "" {
		logLevel = "error"
	}

	config := SingBoxConfig{
		Log:       &SBLog{Level: logLevel, Disabled: false},
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

		// IPv4-only by default — Android's per-app UID matching is happier
		// with deterministic v4 resolutions, and the old behaviour locked
		// us out of v6 anyway. The toggle is opt-in; when on, sing-box
		// answers both A and AAAA and lets the kernel pick.
		if cfg.IPv6 {
			dns.Strategy = "prefer_ipv4"
		} else {
			dns.Strategy = "ipv4_only"
		}

		if detour != "" && cfg.Proxy.IP != "" && net.ParseIP(cfg.Proxy.IP) == nil {
			dns.Rules = append(dns.Rules, SBDNSRule{
				Domain: []string{cfg.Proxy.IP},
				Server: "local",
			})
		}

		// Ad-block: refuse DNS for ad/tracker domains so the app never even
		// opens the connection. References the rule_set tags defined on the
		// route (buildRoute). Non-fatal if a list fails to load.
		// Android connectivity / Private-DNS validation hosts are exempt —
		// blocking them makes the OS mark the VPN as "no internet".
		// YouTube / video CDN / ad-delivery hosts must resolve even when ad lists
		// flag them — route rules pick proxy vs direct.
		if cfg.AdBlock {
			if tag := firstUpstreamDNSTag(servers); tag != "" {
				dns.Rules = append(dns.Rules, SBDNSRule{
					Domain: adBlockConnectivityBypassDomains,
					Server: tag,
				})
				// YouTube / video CDN hosts often appear in ad SRS lists.
				// Always let them resolve — route rules pick proxy vs direct.
				dns.Rules = append(dns.Rules, SBDNSRule{
					Domain:       youTubeCoreDomains,
					DomainSuffix: youTubeDNSBypassSuffixes(),
					Server:       tag,
				})
			}
			dns.Rules = append(dns.Rules, SBDNSRule{
				Domain:       extraAdDeliveryDomains,
				DomainSuffix: adDeliveryRejectSuffixes(),
				Action:       "reject",
			})
			dns.Rules = append(dns.Rules, SBDNSRule{
				RuleSet: adBlockRuleSetTags(),
				Action:  "reject",
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

// firstUpstreamDNSTag returns the tag of the first non-local resolver in a
// tunnel-mode DNS server list. Used to route ad-block bypass domains through
// a real upstream before the reject rule fires.
func firstUpstreamDNSTag(servers []SBDNSServer) string {
	for _, s := range servers {
		if s.Type != "local" && s.Tag != "" {
			return s.Tag
		}
	}
	return ""
}

// lanBypassCIDRs returns RFC1918/link-local/multicast ranges for the BypassLAN
// route rule, with 172.19.0.0/16 carved out of 172.16.0.0/12. The engine TUN
// interface uses 172.19.0.1/30 and libbox hands Android 172.19.0.2 as the
// in-tunnel DNS address — if that /16 were treated as LAN bypass, DNS hijack
// breaks and the VPN shows zero traffic.
func lanBypassCIDRs() []string {
	return []string{
		"10.0.0.0/8",
		// 172.16.0.0/12 minus 172.19.0.0/16
		"172.16.0.0/15",
		"172.18.0.0/16",
		"172.20.0.0/14",
		"172.24.0.0/13",
		"192.168.0.0/16",
		"169.254.0.0/16", // link-local v4
		"224.0.0.0/4",    // multicast v4
		"255.255.255.255/32",
		"fc00::/7",  // ULA
		"fe80::/10", // link-local v6
		"ff00::/8",  // multicast v6
	}
}

// lanBypassContainsIP reports whether ip falls into any lanBypassCIDRs range.
func lanBypassContainsIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, cidr := range lanBypassCIDRs() {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// splitSmartDomains turns a normalised blocked-list (entries from
// blocked_provider.normalizeDomains: lowercase, no scheme, no wildcards)
// into sing-box `domain` (exact) + `domain_suffix` (subdomain match)
// buckets.
//
// Heuristic: treat every base domain as a `domain_suffix` so
// "instagram.com" also catches "x.instagram.com" — that's what users
// expect from an Antizapret-style filter. The cost of the broader match
// is paid by sing-box's hash-trie, which scales to 10k+ entries cheaply.
// Pure-IP entries (rare but present in some Russia lists) go into
// `domain` so they're matched verbatim.
func splitSmartDomains(domains []string) (exact []string, suffix []string) {
	seen := make(map[string]struct{}, len(domains))
	for _, raw := range domains {
		d := strings.ToLower(strings.TrimSpace(raw))
		d = strings.TrimPrefix(d, ".")
		if d == "" {
			continue
		}
		if _, dup := seen[d]; dup {
			continue
		}
		seen[d] = struct{}{}
		if net.ParseIP(d) != nil {
			exact = append(exact, d)
			continue
		}
		// Bare (no leading dot) on purpose: sing-box's matcher treats a
		// dotless suffix as "the domain itself OR any subdomain", with a
		// whole-label boundary ("instagram.com" matches "x.instagram.com"
		// but not "fakeinstagram.com" — see sing/common/domain rootLabel).
		// A dotted suffix (".instagram.com") would match ONLY subdomains,
		// silently sending the bare SNI ("x.com", "youtube.com") to
		// final=direct — dead behind RKN blocks.
		suffix = append(suffix, d)
	}
	return exact, suffix
}

func buildRoute(cfg EngineConfig) *SBRoute {
	final := "proxy"
	// Smart mode flips the default: only listed domains hit the proxy,
	// the rest goes direct. When Smart is on but the blocked list is still
	// empty (first connect / fetch pending), keep Global behaviour so the
	// tunnel isn't a no-op.
	if cfg.SmartMode && len(cfg.SmartBlockedDomains) > 0 {
		final = "direct"
	}
	route := &SBRoute{
		Final:      final,
		AutoDetect: !cfg.IsAndroid && runtime.GOOS != "android",
	}

	// Ad-block rule-sets (binary SRS): cached-local if present, else remote
	// (sing-box downloads them via the direct outbound). Referenced by tag from
	// the reject rules below and in buildDNS.
	if cfg.AdBlock {
		route.RuleSet = append(route.RuleSet, buildAdBlockRuleSets(effectiveDataDir(cfg))...)
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

	// Bypass-LAN: send RFC1918 / link-local / multicast / broadcast / ULA
	// traffic straight to `direct` so the printer / NAS / router admin /
	// mDNS still reachable while the VPN is up. Runs BEFORE sniff because
	// it's a pure IP/CIDR rule — no Host/SNI knowledge required.
	// The engine TUN prefix (172.19.0.0/16) is carved out — see lanBypassCIDRs.
	if cfg.BypassLAN {
		rules = append(rules, SBRouteRule{
			Action:   "route",
			IPCidr:   lanBypassCIDRs(),
			Outbound: "direct",
		})
	}

	rules = append(rules, SBRouteRule{
		Action: "sniff",
	})

	rules = append(rules, SBRouteRule{
		Protocol: []string{"dns"},
		Action:   "hijack-dns",
	})

	// Neutralize DNS-over-TLS (Android "Private DNS"). DoT sniffs as "tls",
	// not "dns", so it skips hijack-dns above; in Smart mode final=direct it
	// then routes to the direct outbound and STALLS — the device probes the
	// advertised in-tunnel DNS IP :853 (no DoT listener → 5s i/o timeout) and
	// public resolvers :853 (~20s). Rejecting port 853 makes DoT fail fast so
	// Android falls back to plaintext DNS on port 53, which IS hijacked. Port
	// matching needs no sniff, so this fires immediately.
	if cfg.Mode == ProxyModeTunnel {
		rules = append(rules, SBRouteRule{
			Port:   []int{853},
			Action: "reject",
		})
	}

	// Ad-block: reject connections to ad/tracker domains. Backstop for the DNS
	// reject in buildDNS — catches hardcoded IPs / DoH that skip our resolver.
	// MUST come after sniff so the rule_set domain matcher sees the host.
	// Android captive-portal / Private-DNS hosts bypass the reject list so the
	// OS doesn't flag the VPN as offline (see adBlockConnectivityBypassDomains).
	if cfg.AdBlock {
		// Video CDN is often in ad SRS lists; pin to proxy before reject.
		rules = append(rules, SBRouteRule{
			Domain:       youTubeCoreDomains,
			DomainSuffix: youTubeCoreSuffixes,
			Outbound:     "proxy",
			Action:       "route",
		})
		rules = append(rules, SBRouteRule{
			Domain:   adBlockConnectivityBypassDomains,
			Outbound: "direct",
			Action:   "route",
		})
		rules = append(rules, SBRouteRule{
			Domain:       extraAdDeliveryDomains,
			DomainSuffix: adDeliveryRejectSuffixes(),
			Action:       "reject",
		})
		rules = append(rules, SBRouteRule{
			RuleSet: adBlockRuleSetTags(),
			Action:  "reject",
		})
	}

	// Smart mode: domains in SmartBlockedDomains go through the proxy.
	// MUST come after the sniff rule above so the domain matcher sees a
	// populated host. Everything else falls through to Final=direct.
	//
	// We split entries with a leading dot (or a 2-label form that
	// implicitly matches subdomains) into `domain_suffix` and keep exact
	// FQDNs in `domain` — sing-box's matcher is hash-based and handles
	// 10k+ entries cheaply, but the split keeps the rule honest and
	// debuggable.
	if cfg.SmartMode && len(cfg.SmartBlockedDomains) > 0 {
		exactDomains, domainSuffixes := splitSmartDomains(cfg.SmartBlockedDomains)
		if len(exactDomains)+len(domainSuffixes) > 0 {
			rules = append(rules, SBRouteRule{
				Action:       "route",
				Domain:       exactDomains,
				DomainSuffix: domainSuffixes,
				Outbound:     "proxy",
			})
		}
	}

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

var pingTCPProbe = PingProxy

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
	latency, ok, r := quicHandshakeProbe(ip, port)
	if ok {
		return latency, true, "", "quic_handshake"
	}

	tcpLat, tcpOK, tcpR := pingTCPProbe(ip, port)
	if tcpOK {
		return tcpLat, true, "", "tcp_fallback"
	}
	if r == "" {
		r = tcpR
	}
	return 0, false, r, "quic_handshake"
}

func PingProxyUDP(ip string, port int) (latencyMs int64, reachable bool, reason string) {
	addr := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("udp", addr, 3*time.Second)
	if err != nil {
		return 0, false, pingReasonFromError(err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(1 * time.Second))
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

// PingWireGuard probes a WireGuard / AmneziaWG endpoint for latency. Those
// transports are UDP-only and silently drop any packet that isn't a valid
// handshake, so neither a TCP connect nor a raw UDP byte yields an RTT (the UDP
// probe can only confirm the host didn't actively refuse). An ICMP echo to the
// host measures the real network round-trip independent of the VPN transport;
// only when ICMP is blocked do we fall back to the UDP liveness probe (which
// returns -1ms → shown as "—").
func PingWireGuard(ip string, port int) (latencyMs int64, reachable bool, reason string) {
	if ms, ok := pingICMPHost(ip, ""); ok {
		return ms, true, ""
	}
	return PingProxyUDP(ip, port)
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

