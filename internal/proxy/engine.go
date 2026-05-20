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
	MITMPort     int // local HTTPS MITM proxy port; 0 disables MITM layer
	KillSwitch   bool
	LocalPort    int
	DNSServers   []string
	TunIPv4      string
	TunIPv6      string
	TunStack     string
	DataDir      string

	// DNSLeakProtection toggles sing-box `strict_route` on the TUN inbound.
	// When true (the default for new installs), sing-box installs Windows
	// Filtering Platform (WFP) rules that drop any outbound packet that
	// would bypass the TUN. This is the only reliable defence against the
	// Smart Multi-Homed Name Resolution leak: Windows otherwise issues
	// DNS queries from every adapter in parallel, and a Russian ISP can
	// transparently hijack the UDP/53 packets that escape via the LAN
	// adapter (returning Rostelecom/MSK-IX addresses instead of the
	// chosen resolver). Has no effect in Proxy mode.
	DNSLeakProtection bool
}

type Engine interface {
	Start(ctx context.Context, cfg EngineConfig) error

	Stop() error

	IsRunning() bool

	GetTrafficStats() (up, down int64)

	// ApplyAppWhitelist swaps the active per-app exclusion list without
	// disconnecting. No-op when not running. Implementations may briefly
	// interrupt traffic while rebuilding the routing config.
	ApplyAppWhitelist(paths []string) error
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

type SBRuleSet struct {
	Type          string           `json:"type,omitempty"`
	Tag           string           `json:"tag"`
	Format        string           `json:"format,omitempty"`
	RemoteOptions SBRemoteRuleSet  `json:"-"`
	LocalOptions  SBLocalRuleSet   `json:"-"`
}

type SBRemoteRuleSet struct {
	URL            string `json:"url,omitempty"`
	DownloadDetour string `json:"download_detour,omitempty"`
	UpdateInterval string `json:"update_interval,omitempty"`
}

type SBLocalRuleSet struct {
	Path string `json:"path,omitempty"`
}

// MarshalJSON flattens remote/local options into sing-box rule_set JSON.
func (r SBRuleSet) MarshalJSON() ([]byte, error) {
	type head struct {
		Type   string `json:"type,omitempty"`
		Tag    string `json:"tag"`
		Format string `json:"format,omitempty"`
	}
	h := head{Type: r.Type, Tag: r.Tag, Format: r.Format}
	switch r.Type {
	case "remote":
		return json.Marshal(struct {
			head
			SBRemoteRuleSet
		}{h, r.RemoteOptions})
	case "local":
		return json.Marshal(struct {
			head
			SBLocalRuleSet
		}{h, r.LocalOptions})
	default:
		return json.Marshal(h)
	}
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
	Domain           []string `json:"domain,omitempty"`
	ProcessPathRegex []string `json:"process_path_regex,omitempty"`
	RuleSet          []string `json:"rule_set,omitempty"`
	Server           string   `json:"server,omitempty"`
	Action           string   `json:"action,omitempty"`
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
	Type                string           `json:"type"`
	Tag                 string           `json:"tag"`
	Server              string           `json:"server,omitempty"`
	ServerPort          int              `json:"server_port,omitempty"`
	Username            string           `json:"username,omitempty"`
	Password            string           `json:"password,omitempty"`
	Method              string           `json:"method,omitempty"`
	Version             string           `json:"version,omitempty"`
	UUID                string           `json:"uuid,omitempty"`
	AlterId             int              `json:"alter_id,omitempty"`
	Flow                string           `json:"flow,omitempty"`
	PacketEncoding      string           `json:"packet_encoding,omitempty"`
	GlobalPadding       bool             `json:"global_padding,omitempty"`
	AuthenticatedLength bool             `json:"authenticated_length,omitempty"`
	Security            string           `json:"security,omitempty"`
	UpMbps              int              `json:"up_mbps,omitempty"`
	DownMbps            int              `json:"down_mbps,omitempty"`
	Obfs                *SBHysteria2Obfs `json:"obfs,omitempty"`

	TLS       *SBOutboundTLS       `json:"tls,omitempty"`
	Transport *SBOutboundTransport `json:"transport,omitempty"`

	DomainStrategy string `json:"domain_strategy,omitempty"`
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
	JC   int `json:"jc,omitempty"`
	JMin int `json:"jmin,omitempty"`
	JMax int `json:"jmax,omitempty"`
	S1   int `json:"s1,omitempty"`
	S2   int `json:"s2,omitempty"`
	S3   int `json:"s3,omitempty"`
	S4   int `json:"s4,omitempty"`
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
	RuleSet     []SBRuleSet   `json:"rule_set,omitempty"`
	Rules       []SBRouteRule `json:"rules,omitempty"`
	Final       string        `json:"final,omitempty"`
	AutoDetect  bool          `json:"auto_detect_interface,omitempty"`
	FindProcess bool          `json:"find_process,omitempty"`
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

func BuildProxyModeConfig(cfg EngineConfig) (SingBoxConfig, error) {
	port := cfg.LocalPort
	if port == 0 {
		port = getFreeLocalPort(14081)
	}

	host, _ := splitHostPort(cfg.ListenAddr, "127.0.0.1", port)

	dd := effectiveDataDir(cfg)
	endpoints, err := buildEndpoints(cfg.Proxy)
	if err != nil {
		return SingBoxConfig{}, err
	}
	sbCfg := SingBoxConfig{
		Log:       &SBLog{Level: "error", Disabled: true},
		DNS:       buildDNS(cfg),
		Endpoints: endpoints,
		Inbounds: []SBInbound{{
			Type:       "mixed",
			Tag:        "mixed-in",
			Listen:     host,
			ListenPort: port,
		}},
		Outbounds:    appendOutbounds(buildOutbounds(cfg.Proxy), cfg),
		Route:        buildRoute(cfg),
		Experimental: buildExperimentalCache(dd),
	}

	return sbCfg, nil
}

func BuildTunnelModeConfig(cfg EngineConfig) (SingBoxConfig, error) {
	tunIPv4 := "172.19.0.1/30"
	if cfg.TunIPv4 != "" {
		tunIPv4 = cfg.TunIPv4
	}
	// IPv6 ULA on the TUN keeps IPv6 traffic riding the tunnel. Without
	// an IPv6 address here, strict_route's WFP filters would silently
	// blackhole IPv6 — leaving the user without IPv6 connectivity while
	// connected.
	tunIPv6 := "fdfe:dcba:9876::1/126"
	if cfg.TunIPv6 != "" {
		tunIPv6 = cfg.TunIPv6
	}
	tunAddresses := []string{tunIPv4, tunIPv6}
	tunStack := effectiveTunStack(cfg.TunStack)
	// strict_route adds WFP filters on Windows that drop outbound packets
	// bypassing the TUN. This is the only reliable way to stop Smart
	// Multi-Homed Name Resolution from leaking DNS queries to the LAN
	// adapter, where Russian ISPs transparently hijack UDP/53 (Rostelecom,
	// MSK-IX). User-controlled via the "DNS leak protection" toggle.
	strictRoute := cfg.DNSLeakProtection

	pt := strings.ToUpper(strings.TrimSpace(cfg.Proxy.Type))

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

	endpoints, err := buildEndpoints(cfg.Proxy)
	if err != nil {
		return SingBoxConfig{}, err
	}
	sbCfg := SingBoxConfig{
		Log:       &SBLog{Level: "error", Disabled: false},
		DNS:       buildDNS(cfg),
		Endpoints: endpoints,
		Inbounds: []SBInbound{{
			Type:                "tun",
			Tag:                 "tun-in",
			Address:             tunAddresses,
			Stack:               tunStack,
			AutoRoute:           true,
			StrictRoute:         strictRoute,
			RouteExcludeAddress: routeExclude,
		}},
		Outbounds:    appendOutbounds(outbounds, cfg),
		Route:        buildRoute(cfg),
		Experimental: buildExperimentalCache(dd),
	}

	return sbCfg, nil
}

func appendOutbounds(base []SBOutbound, cfg EngineConfig) []SBOutbound {
	if cfg.AdBlock && cfg.MITMPort > 0 {
		base = append(base, SBOutbound{
			Type:       "http",
			Tag:        adBlockMITMOutbound,
			Server:     "127.0.0.1",
			ServerPort: cfg.MITMPort,
		})
	}
	return base
}

func effectiveTunStack(stack string) string {
	switch strings.ToLower(strings.TrimSpace(stack)) {
	case "gvisor":
		return "gvisor"
	default:
		return "system"
	}
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

		// Whitelisted apps (split-tunnel direct) must resolve via the local
		// system resolver, NOT through the proxy detour. Otherwise the TCP
		// connection is bypassed but the DNS lookup still rides the tunnel —
		// for SSH/SFTP clients (WinSCP, etc.) this manifests as silent
		// "Failed to establish connection" because the encrypted DNS detour
		// to a public resolver is slower than the SSH handshake timeout.
		if rx := appWhitelistPathRegexes(cfg.AppWhitelist); len(rx) > 0 {
			dns.Rules = append(dns.Rules, SBDNSRule{
				ProcessPathRegex: rx,
				Server:           "local",
			})
		}

		dns.Rules = appendAdBlockDNSRules(cfg, dns.Rules)
		return dns
	}

	// proxy mode: prior versions used direct UDP DNS to 8.8.8.8/1.1.1.1
	// with the comment "DNS leaks are insignificant in proxy mode (apps use
	// system proxy)". That was wrong: plain UDP/53 to public resolvers is
	// readable by the ISP and tags every TLS handshake with the queried
	// domain. The sing-box engine resolves names for the proxy outbound too,
	// so those queries leak even when the rest of the app traffic is
	// tunneled. The fix: encrypt DNS by default via DoT.
	//
	// User-supplied custom DNS keep their explicit type (UDP if the user
	// asked for it — we don't second-guess). When no custom DNS is set,
	// emit TLS DNS entries (port 853) only.
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
			{Type: "tls", Tag: "cloudflare-tls", Server: "1.1.1.1"},
			{Type: "tls", Tag: "google-tls", Server: "8.8.8.8"},
			{Type: "local", Tag: "local"},
		}
	}

	dns := &SBDNS{Servers: servers}
	dns.Rules = appendAdBlockDNSRules(cfg, dns.Rules)
	return dns
}

func appendAdBlockDNSRules(cfg EngineConfig, rules []SBDNSRule) []SBDNSRule {
	if !cfg.AdBlock {
		return rules
	}
	tags := adBlockRuleSetTags()
	return append(rules, SBDNSRule{
		RuleSet: tags,
		Action:  "reject",
	})
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
	findProcess := len(cfg.AppWhitelist) > 0
	if cfg.AdBlock && cfg.MITMPort > 0 {
		findProcess = true
	}
	route := &SBRoute{
		Final:       "proxy",
		AutoDetect:  true,
		FindProcess: findProcess,
	}
	if cfg.AdBlock {
		route.RuleSet = buildAdBlockRuleSets(effectiveDataDir(cfg))
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

	rules = appendAdBlockRouteRules(cfg, rules)

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

func appendAdBlockRouteRules(cfg EngineConfig, rules []SBRouteRule) []SBRouteRule {
	if !cfg.AdBlock {
		return rules
	}
	tags := adBlockRuleSetTags()
	rules = append(rules, SBRouteRule{
		RuleSet: tags,
		Action:  "reject",
	})

	if cfg.MITMPort <= 0 {
		return rules
	}

	browserRX := browserProcessPathRegexes()
	if len(browserRX) == 0 {
		return rules
	}

	// Pinning-sensitive domains bypass MITM (direct).
	for _, d := range adBlockPinningBypassDomains {
		rules = append(rules, SBRouteRule{
			Action:   "route",
			Domain:   []string{d},
			Outbound: "direct",
		})
	}

	// Force TCP TLS through local MITM proxy for browsers.
	rules = append(rules, SBRouteRule{
		Action:           "route",
		ProcessPathRegex: browserRX,
		Network:          []string{"tcp"},
		Port:             []int{443},
		Outbound:         adBlockMITMOutbound,
	})

	// Block QUIC/HTTP3 so browsers fall back to TCP (MITM cannot inspect QUIC).
	rules = append(rules, SBRouteRule{
		Action:           "reject",
		ProcessPathRegex: browserRX,
		Network:          []string{"udp"},
		Port:             []int{443},
	})

	return rules
}

// OverlappingProbeDomains returns user-whitelist entries that match (exactly
// or as a parent suffix) one of the tunnelProbeDomains. These are forced
// through the proxy outbound by buildRoute regardless of the user's
// "direct" intent — necessary so the post-start health probe truly
// transits the tunnel (otherwise a broken SS/VLESS/VMESS would mask as
// healthy). Callers (Manager.Connect) use this to warn the user that
// e.g. their ".gstatic.com" rule won't apply to "connectivitycheck.gstatic.com"
// during the first few seconds of a session.
func OverlappingProbeDomains(userWhitelist []string) []string {
	if len(userWhitelist) == 0 || len(tunnelProbeDomains) == 0 {
		return nil
	}
	var hits []string
	seen := make(map[string]struct{}, len(userWhitelist))
	for _, raw := range userWhitelist {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		needle := strings.ToLower(strings.TrimPrefix(entry, "."))
		for _, probe := range tunnelProbeDomains {
			p := strings.ToLower(probe)
			if p == needle || strings.HasSuffix(p, "."+needle) {
				if _, dup := seen[entry]; !dup {
					seen[entry] = struct{}{}
					hits = append(hits, entry)
				}
				break
			}
		}
	}
	return hits
}

// GetFreeLocalPort returns defaultPort if available, otherwise a random free port.
func GetFreeLocalPort(defaultPort int) int {
	return getFreeLocalPort(defaultPort)
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
