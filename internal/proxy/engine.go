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

	// ResolvedIP is a single server IP learned at connect time (or recovered from
	// the live socket / server-pin cache when the OS resolver is censored). It is
	// the kill-switch's guaranteed-good allow target and the redaction/log anchor.
	// Empty when IP is already a literal address. Not serialized — connect-time
	// cache only. See ResolvedIPs for the full failover set.
	ResolvedIP string `json:"-"`

	// ResolvedIPs is the FULL set of IPs the server domain resolved to at connect
	// time (while the OS resolver still works, before we redirect system DNS).
	// It seeds a static `hosts` DNS record (see buildDNS) so sing-box re-resolves
	// the server domain against these IPs — NOT the fragile redirected OS resolver
	// — and can fail over across a CDN's backends within the live session. Pinning
	// the outbound to a single IP (the pre-fix behaviour) instead nailed the whole
	// session to one backend: when that backend reset, every multiplexed
	// connection died at once and sing-box kept redialling the dead IP. Also feeds
	// route-exclude and the kill-switch allow-set so every backend is covered.
	// Empty when IP is a literal. Not serialized.
	ResolvedIPs []string `json:"-"`
}

type EngineConfig struct {
	Proxy        ProxyConfig
	Mode         ProxyMode
	ListenAddr   string
	RoutingMode  RoutingMode
	Whitelist    []string
	AppWhitelist []string
	// AppForceVPN forces every connection owned by these process names through
	// the proxy outbound, regardless of routing mode rules. This is the
	// reliable equivalent of "cascade by domain": Discord voice and Speedtest
	// talk UDP to bare IPs that no domain/SNI rule can catch, but the owning
	// process is always known to sing-box's find_process. Tunnel mode only.
	AppForceVPN []string
	// BlockedDomains is the censored/blocked block-list (already normalized
	// suffixes from Router.GetBlockedDomains). Consumed only in Smart mode:
	// buildRoute routes these through the proxy while everything else goes
	// direct (Final="direct"). Ignored in Global/Whitelist modes.
	BlockedDomains []string
	// SmartRuleSetPath is the path to the block-list compiled into a binary
	// sing-box rule-set (see CompileSmartRuleSet). When set, buildRoute
	// references it by tag instead of inlining tens of thousands of
	// domain_suffix entries into the config. Empty means "not compiled" and
	// buildRoute falls back to the inline form — the compile step must never be
	// able to block a connect.
	SmartRuleSetPath string
	// BlockedCIDRs is the IP-subnet block-list (Telegram MTProto data-center
	// ranges, from Router.GetBlockedCIDRs). Telegram's native clients dial
	// these IPs directly without a domain/SNI, so domain rules can't catch
	// them — Smart mode adds an ip_cidr → proxy rule. Smart-only.
	BlockedCIDRs []string
	KillSwitch   bool
	LocalPort    int
	DNSServers   []string
	TunIPv4      string
	TunIPv6      string
	TunStack     string
	DataDir      string

	// RoutingLists are user routing subscriptions resolved to local
	// source-format rule_set caches. Applied in ALL modes as explicit rules
	// ahead of the built-in Smart/whitelist/ad-block rules, ordered
	// restrictive-first (block > proxy > direct). See buildRoute.
	RoutingLists []RoutingListSpec

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

	// GetProxyTrafficStats returns cumulative bytes carried specifically by the
	// proxy/endpoint outbound (NOT direct/split-tunnel traffic). The health
	// watchdog uses the per-tick delta to veto a kill-switch engage while the
	// upstream is demonstrably moving real traffic — counting only proxy bytes so
	// direct traffic can never mask a genuinely dead upstream (which must still
	// trip the kill switch).
	GetProxyTrafficStats() (up, down int64)

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
	// Final names the DNS server used when no rule matches. sing-box falls
	// back to the FIRST registered transport when this is empty
	// (dns/transport_manager.go), so Smart mode must name "local" explicitly
	// to keep non-blocked lookups on the system resolver. A non-empty tag with
	// no matching server is a hard start failure ("default DNS server not
	// found"), so only ever set a tag that is registered.
	Final string `json:"final,omitempty"`
}

type SBDNSServer struct {
	Type            string `json:"type"`
	Tag             string `json:"tag"`
	Server          string `json:"server,omitempty"`
	ServerPort      int    `json:"server_port,omitempty"`
	Detour          string `json:"detour,omitempty"`
	AddressStrategy string `json:"address_strategy,omitempty"`
	// Predefined seeds a static "hosts" DNS server: domain → fixed IP list.
	// Used to pin the proxy server's own domain to its connect-time IPs so
	// re-resolution never touches the redirected OS resolver (see buildDNS).
	Predefined map[string][]string `json:"predefined,omitempty"`
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
	// UDPTimeout caps the lifetime of NAT slots for UDP flows on the TUN
	// inbound. Default in sing-box is 5 minutes — under heavy DPI environments
	// (RU/CN/IR) where browsers continuously attempt QUIC handshakes that get
	// dropped at UDP/443 by ISP-level filtering, the 5-minute window means
	// every failed handshake holds a NAT slot for the full duration. Pprof
	// captures under such conditions consistently showed lingering
	// udpnat2.natConn waiters that never resolved.
	UDPTimeout string `json:"udp_timeout,omitempty"`
	// EndpointIndependentNat lets multiple destinations share NAT slots for
	// the same (source IP, source port) pair instead of allocating a slot
	// per destination. Under browser QUIC connection storms hitting many
	// CDN IPs from a single ephemeral source port, this reduces total slot
	// count proportionally.
	EndpointIndependentNat bool `json:"endpoint_independent_nat,omitempty"`
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
	I1 string `json:"i1,omitempty"`
	I2 string `json:"i2,omitempty"`
	I3 string `json:"i3,omitempty"`
	I4 string `json:"i4,omitempty"`
	I5 string `json:"i5,omitempty"`
	// J1-J3 and ITime are deliberately absent: the wireguard-go fork behind
	// the engine has never had those device keys in its UAPI (device/uapi.go
	// stops at i5 and its default branch returns "invalid UAPI device key").
	// sing-box-extended used to declare and emit them anyway, which made
	// IpcSet fail outright; it dropped them in v1.13.16-extended-2.6.1.

	// AmneziaWG 3.0, available since sing-box-extended
	// v1.13.16-extended-2.6.1. HeaderProtectionKey is base64 like the other
	// WireGuard keys — upstream decodes it and hex-encodes it for the UAPI.
	// The rest are emitted as strings ("n" or "low-high") so upstream can
	// parse them into *Xbadoption.Range and randomize per use.
	HeaderProtectionKey    string `json:"header_protection_key,omitempty"`
	ContentPaddingAddition string `json:"content_padding_addition,omitempty"`
	RekeyAfterTime         string `json:"rekey_after_time,omitempty"`
	RekeyTimeout           string `json:"rekey_timeout,omitempty"`
	RejectAfterTime        string `json:"reject_after_time,omitempty"`
	KeepaliveTimeout       string `json:"keepalive_timeout,omitempty"`
	MaxHandshakeAttempts   string `json:"max_handshake_attempts,omitempty"`
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

	// xhttp padding obfuscation (sing-box-extended >= 1.13.x-extended-2.x).
	// With x_padding_obfs_mode off the core hardcodes the classic
	// Referer/x_padding pair; with it on, the padding carrier (cookie /
	// header / query / queryInHeader), its key/header name and the filler
	// alphabet come from the node config and must match the server side.
	// Everything is omitempty: a node that says nothing about padding obfs
	// produces exactly the same JSON as before.
	XPaddingObfsMode  *bool  `json:"x_padding_obfs_mode,omitempty"`
	XPaddingKey       string `json:"x_padding_key,omitempty"`
	XPaddingHeader    string `json:"x_padding_header,omitempty"`
	XPaddingPlacement string `json:"x_padding_placement,omitempty"`
	XPaddingMethod    string `json:"x_padding_method,omitempty"`
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
	// Method qualifies Action="reject": "default" answers with ICMP
	// port-unreachable, "drop" black-holes silently. Only "default" produces the
	// fast client-side fallback quicRejectRule relies on.
	Method string `json:"method,omitempty"`
}

// quicRejectRule builds the UDP/443 reject that forces a QUIC client back onto
// TCP. Callers pass the same selector as the route-to-proxy rule it shadows, so
// the reject covers exactly the traffic we tunnel and nothing else.
//
// Why this exists: UDP does not survive every proxy outbound. Measured on a
// live tunnel (2026-08-17), Smart-list hosts answered over TCP/TLS in ~200 ms
// while their h3 handshakes timed out at 10 s — reproduced through the local
// SOCKS inbound with the target named by domain, so sniffing is not the
// culprit. Whether UDP works depends on the node, which is why users see it as
// "Discord attachments sometimes open, sometimes don't". A silent black hole
// leaves Chromium (Discord is Electron) retrying QUIC for seconds; an ICMP
// unreachable makes it mark h3 broken and switch to HTTP/2 immediately.
func quicRejectRule(sel SBRouteRule) SBRouteRule {
	sel.Action = "reject"
	sel.Method = "default"
	sel.Network = []string{"udp"}
	sel.Port = []int{443}
	sel.Outbound = ""
	return sel
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
		rx := processPathRegex(n)
		if rx == "" {
			continue
		}
		out = append(out, rx)
	}
	return out
}

// processPathRegex compiles one app-list entry into a sing-box
// process_path_regex.
//
// A bare basename ("wow.exe") anchors on any path separator, so it matches the
// executable wherever the game is installed — the long-standing behaviour.
//
// An entry carrying path components ("Battle.net\Agent\Agent.exe") anchors the
// whole tail instead. Blizzard's updater is named Agent.exe; as a bare basename
// it would also match Docker's, 1C's and every corporate agent on the machine,
// silently routing an unrelated process the wrong way. Separators are accepted
// in either slash direction and matched in either direction, because entries
// are authored by hand and Windows paths arrive both ways.
func processPathRegex(entry string) string {
	parts := strings.FieldsFunc(entry, func(r rune) bool {
		return r == '\\' || r == '/'
	})
	quoted := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		quoted = append(quoted, regexp.QuoteMeta(p))
	}
	if len(quoted) == 0 {
		return ""
	}
	return `(?i)(^|[\\/])` + strings.Join(quoted, `[\\/]`) + `$`
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
		Outbounds:    buildOutbounds(cfg.Proxy),
		Route:        buildRoute(cfg),
		Experimental: buildExperimentalCache(dd),
	}

	return sbCfg, nil
}

// systemHasIPv6 reports whether the host has IPv6 wired up at the OS level.
// We treat "any non-loopback, non-tunnel interface has at least one IPv6
// unicast address" as the signal. This catches both adapter-level disabled
// IPv6 and OS-wide DisabledComponents on Windows: with neither in play, every
// box has at least a link-local fe80:: on the LAN adapter, which is enough
// for sing-tun's CreateUnicastIpAddressEntry call to succeed against the TUN.
//
// Conservative-fail: on enumeration error we assume IPv6 is present, so we
// don't silently strip IPv6 from the tunnel on hosts where the check is
// merely flaky (CGO timeout, etc.).
func systemHasIPv6() bool {
	ifaces, err := net.Interfaces()
	if err != nil {
		return true
	}
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		if looksLikeTunnelInterface(ifi.Name) {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.To4() != nil {
				continue
			}
			return true
		}
	}
	return false
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
	//
	// But: on Windows boxes where the IPv6 stack is disabled at the adapter
	// level (or globally via DisabledComponents), sing-tun's attempt to set
	// the IPv6 address on the TUN interface fails with
	// "configure tun interface: set ipv6 address: Element not found",
	// which ClassifyEngineStartError currently maps to "tun_privileges" and
	// the UI surfaces as "нужны права администратора" — sending users on a
	// fruitless quest to elevate. Only attach the IPv6 address when the
	// host actually exposes IPv6 on at least one non-loopback interface,
	// or when the user explicitly set TunIPv6 (override = "I know what I'm
	// doing").
	tunIPv6 := "fdfe:dcba:9876::1/126"
	if cfg.TunIPv6 != "" {
		tunIPv6 = cfg.TunIPv6
	}
	tunAddresses := []string{tunIPv4}
	if cfg.TunIPv6 != "" || systemHasIPv6() {
		tunAddresses = append(tunAddresses, tunIPv6)
	}
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
		// Exclude EVERY backend IP the server resolved to (a CDN domain has
		// several, and sing-box may fail over among them mid-session) so none of
		// the server's own traffic loops back into the TUN. Domains alone yield
		// nothing here (net.ParseIP fails on a hostname) — the pinned IP set is
		// what gives domain-addressed servers their exclude CIDRs.
		for _, host := range serverPinnedIPs(cfg.Proxy) {
			if serverIP := net.ParseIP(host); serverIP != nil {
				cidr := host + "/32"
				if serverIP.To4() == nil {
					cidr = host + "/128"
				}
				routeExclude = append(routeExclude, cidr)
			}
		}
	}

	dd := effectiveDataDir(cfg)
	outbounds := buildOutbounds(cfg.Proxy)

	endpoints, err := buildEndpoints(cfg.Proxy)
	if err != nil {
		return SingBoxConfig{}, err
	}
	// UDPTimeout / EndpointIndependentNat are TUN-inbound NAT knobs aimed at
	// cleaning up dead UDP flows under DPI-driven QUIC retry storms. They
	// must NOT be applied when the active protocol is a WireGuard endpoint:
	// for WG/AWG the TUN inbound feeds packets straight into the endpoint,
	// which maintains its own session state, and forcing the inbound to
	// expire NAT slots after 30s tore down live tunnel traffic (handshake
	// passes, browser works for ~30s, then every UDP flow inside the tunnel
	// collapses). Keep inbound defaults (5min, symmetric) for endpoint protos.
	tun := SBInbound{
		Type:                "tun",
		Tag:                 "tun-in",
		Address:             tunAddresses,
		Stack:               tunStack,
		AutoRoute:           true,
		StrictRoute:         strictRoute,
		RouteExcludeAddress: routeExclude,
	}
	if pt != "WIREGUARD" && pt != "AMNEZIAWG" {
		tun.UDPTimeout = "30s"
		tun.EndpointIndependentNat = true
	}
	// Loopback probe inbound: post-start and watchdog health probes go through
	// this listener instead of the TUN default route. The target hostname
	// travels inside the proxy request and is resolved remotely by sing-box DNS
	// (detour=proxy), so probes keep working when the OS resolver degrades
	// mid-session — applySystemDNSOverride pins physical adapters to public
	// resolvers that are unreachable outside the tunnel, and strict_route drops
	// off-TUN lookups; probes relying on getaddrinfo then time out and falsely
	// trip the kill switch on a healthy server. Bound to 127.0.0.1 only — never
	// LAN-exposed (same surface proxy mode has always had).
	probePort := cfg.LocalPort
	if probePort == 0 {
		probePort = getFreeLocalPort(14081)
	}
	probeIn := SBInbound{
		Type:       "mixed",
		Tag:        "probe-in",
		Listen:     "127.0.0.1",
		ListenPort: probePort,
	}
	sbCfg := SingBoxConfig{
		Log:          &SBLog{Level: "error", Disabled: false},
		DNS:          buildDNS(cfg),
		Endpoints:    endpoints,
		Inbounds:     []SBInbound{tun, probeIn},
		Outbounds:    outbounds,
		Route:        buildRoute(cfg),
		Experimental: buildExperimentalCache(dd),
	}

	return sbCfg, nil
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

// serverPinnedIPs returns every literal IP associated with the proxy server,
// deduped and order-stable: the IP field when it is already a literal, then the
// full connect-time resolved set (ResolvedIPs), then the single learned/cached
// ResolvedIP. Used for the static hosts DNS record, route-exclude, and the
// kill-switch allow-set so a CDN server's every backend is covered.
func serverPinnedIPs(proxy ProxyConfig) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || net.ParseIP(s) == nil {
			return
		}
		if _, dup := seen[s]; dup {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	add(proxy.IP) // no-op unless IP is already a literal
	for _, ip := range proxy.ResolvedIPs {
		add(ip)
	}
	add(proxy.ResolvedIP)
	return out
}

// serverEndpointUnresolvable reports whether a TUN connect should be aborted up
// front: the server is addressed by a domain but no IP could be pinned at connect
// time, so sing-box would have to dial it through the censored OS `local` resolver
// (the custom DNS servers route detour=proxy, which isn't up yet during connect).
// That path either loops the server's own packets back into the TUN (EOF flood) or
// resolves to a poisoned/CDN-fronted IP (x509-github). A literal-IP server needs no
// resolution, and proxy mode never builds the TUN route-exclude, so neither is gated.
func serverEndpointUnresolvable(proxy ProxyConfig, mode ProxyMode) bool {
	if mode != ProxyModeTunnel {
		return false
	}
	if proxy.IP == "" || net.ParseIP(proxy.IP) != nil {
		return false
	}
	return len(serverPinnedIPs(proxy)) == 0
}

// outboundTLSDiagnostic reports the TLS state of the BUILT proxy outbound:
// "reality" (Reality active), "tls" (plain TLS, no Reality), or "none" (no TLS
// layer, e.g. Shadowsocks). A vless+reality server reporting "tls" is the
// signature of a stripped Reality block — the cause of the x509-github failures
// — so logging this at connect surfaces the bug from a reporter's log alone.
func outboundTLSDiagnostic(proxy ProxyConfig) string {
	out := buildProxyOutbound(proxy)
	if out.TLS == nil || !out.TLS.Enabled {
		return "none"
	}
	if out.TLS.Reality != nil && out.TLS.Reality.Enabled {
		return "reality"
	}
	return "tls"
}

// smartRuleSetActive reports whether buildRoute registers the compiled
// Smart-mode block-list rule-set. buildDNS references the very same tag, and a
// DNS rule pointing at an unregistered rule_set fails the start — so both sides
// ask this one function instead of repeating the condition and drifting apart.
func smartRuleSetActive(cfg EngineConfig) bool {
	return cfg.RoutingMode == ModeSmart &&
		len(cfg.BlockedDomains) > 0 &&
		cfg.SmartRuleSetPath != ""
}

// firstDetourServerTag returns the tag of the first DNS server routed through
// the given detour. That server is already the de-facto default today: with no
// dns.final, sing-box uses the first registered transport and reaches the rest
// only through rules. Pointing Smart mode's tunnel rules at it therefore
// preserves current behaviour for blocked domains exactly.
func firstDetourServerTag(servers []SBDNSServer, detour string) string {
	for _, s := range servers {
		if s.Detour == detour && s.Tag != "" {
			return s.Tag
		}
	}
	return ""
}

func buildDNS(cfg EngineConfig) *SBDNS {
	if cfg.Mode == ProxyModeTunnel {
		// All DNS servers route through the proxy/endpoint outbound (tag
		// "proxy" — same tag for SS/VLESS outbounds and for the WG/AWG
		// endpoint, see buildEndpoints). Earlier code set detour="" for
		// WG/AWG endpoints, relying on the peer's own DNS, but sing-box
		// then sent UDP/53 to 8.8.8.8 via the direct outbound. With
		// DNSLeakProtection (= strict_route) on, sing-tun's WFP filters
		// dropped those direct packets — DNS for the post-start HTTP probe
		// never resolved, the probe timed out, and Connect hung at
		// "Подключение..." until the daemon RPC ctx expired (~70s) and
		// reported "cancelled". Pinning detour to "proxy" sends DNS through
		// the tunnel for all protocols, eliminating the WFP race.
		detour := "proxy"

		servers := []SBDNSServer{}
		if len(cfg.DNSServers) > 0 {
			for i, raw := range cfg.DNSServers {
				server, port := splitDNSServer(raw)
				if server == "" {
					continue
				}
				servers = append(servers, SBDNSServer{
					Type:       "tcp",
					Tag:        fmt.Sprintf("custom-%d", i+1),
					Server:     server,
					ServerPort: port,
					Detour:     detour,
				})
			}
			servers = append(servers, SBDNSServer{Type: "local", Tag: "local"})
		} else {
			servers = []SBDNSServer{
				{Type: "tcp", Tag: "google-tcp", Server: "8.8.8.8", Detour: detour},
				{Type: "tcp", Tag: "cloudflare-tcp", Server: "1.1.1.1", Detour: detour},
				{Type: "tls", Tag: "google-tls", Server: "8.8.8.8", Detour: detour},
				{Type: "tls", Tag: "cloudflare-tls", Server: "1.1.1.1", Detour: detour},
				{Type: "local", Tag: "local"},
			}
		}

		dns := &SBDNS{
			Servers: servers,
		}

		dns.Strategy = "ipv4_only"

		// Resolve the server's own hostname. When we pinned its IPs at connect
		// time (CDN/multi-IP domain), serve them from a static `hosts` record so
		// sing-box re-resolves the domain instantly and locally — rotating across
		// every backend on a session reset — instead of hitting the redirected OS
		// `local` resolver, which times out mid-session (the false-kill-switch
		// fragility) and, when pinned to a single IP, took the whole session down
		// with one dead backend. Fall back to `local` only when nothing resolved.
		if cfg.Proxy.IP != "" && net.ParseIP(cfg.Proxy.IP) == nil {
			if pinned := serverPinnedIPs(cfg.Proxy); len(pinned) > 0 {
				dns.Servers = append(dns.Servers, SBDNSServer{
					Type:       "hosts",
					Tag:        "server-pin",
					Predefined: map[string][]string{cfg.Proxy.IP: pinned},
				})
				dns.Rules = append(dns.Rules, SBDNSRule{
					Domain: []string{cfg.Proxy.IP},
					Server: "server-pin",
				})
			} else {
				dns.Rules = append(dns.Rules, SBDNSRule{
					Domain: []string{cfg.Proxy.IP},
					Server: "local",
				})
			}
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

		// Smart mode: make DNS mirror the traffic split. buildRoute sets
		// Final="direct" here, so everything outside the block-list leaves from
		// the user's real address — yet every lookup still exited through the
		// tunnel. GeoDNS services (Battle.net/WoW, Akamai, game CDNs) answered
		// for the exit node's region while the game connected directly: that
		// mismatch is what produced the high ping, the launcher's "VPN
		// detected" and the mid-session drops.
		//
		// Blocked domains keep resolving through the tunnel — a local answer
		// for a censored domain is a poisoned answer. Force-VPN apps do too:
		// their whole reason for being on that list is that the local answer is
		// unusable.
		if smartRuleSetActive(cfg) {
			if tunnelTag := firstDetourServerTag(dns.Servers, detour); tunnelTag != "" {
				if rx := appWhitelistPathRegexes(cfg.AppForceVPN); len(rx) > 0 {
					dns.Rules = append(dns.Rules, SBDNSRule{
						ProcessPathRegex: rx,
						Server:           tunnelTag,
					})
				}
				dns.Rules = append(dns.Rules, SBDNSRule{
					RuleSet: []string{smartRuleSetTag},
					Server:  tunnelTag,
				})
				// Everything else lands on the system resolver, which is what
				// restores correct GeoDNS answers. The "local" server is
				// appended unconditionally in both branches above, so this tag
				// always resolves.
				dns.Final = "local"
			}
		}

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
	return dns
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
	findProcess := len(cfg.AppWhitelist) > 0 ||
		(cfg.Mode == ProxyModeTunnel && len(cfg.AppForceVPN) > 0)
	// Smart mode inverts the default: everything goes direct and only the
	// censored block-list is tunneled (see the blocked-domain rule below).
	// Global/Whitelist keep proxy as the catch-all.
	final := "proxy"
	if cfg.RoutingMode == ModeSmart {
		final = "direct"
	}
	route := &SBRoute{
		Final:       final,
		AutoDetect:  true,
		FindProcess: findProcess,
	}
	route.RuleSet = append(route.RuleSet, buildRoutingListRuleSets(cfg.RoutingLists)...)

	var rules []SBRouteRule

	if cfg.Mode == ProxyModeTunnel {
		// Every literal IP the server is known by (the IP field when literal, plus
		// the connect-time resolved set for a domain server) → direct, by ip_cidr.
		// This keeps the outbound's own dial to the server off the TUN at L3. A
		// domain server used to get ONLY the fragile domain → direct rule below,
		// which doesn't catch the outbound's dial-by-resolved-IP — so the server
		// connection could loop back into the TUN (EOF flood, github-issue 2026-06).
		var serverCIDRs []string
		for _, ip := range serverPinnedIPs(cfg.Proxy) {
			cidr := ip + "/32"
			if parsed := net.ParseIP(ip); parsed != nil && parsed.To4() == nil {
				cidr = ip + "/128"
			}
			serverCIDRs = append(serverCIDRs, cidr)
		}
		if len(serverCIDRs) > 0 {
			rules = append(rules, SBRouteRule{
				Action:   "route",
				IPCidr:   serverCIDRs,
				Outbound: "direct",
			})
		}
		// Domain-addressed server keeps a domain → direct fallback in addition to
		// the ip_cidr rules above: the pin set can be empty if connect-time
		// resolution failed (the connect path now fails fast in that case, but the
		// fallback also helps sniff-tagged connections match the right outbound).
		if cfg.Proxy.IP != "" && net.ParseIP(cfg.Proxy.IP) == nil {
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

	// User routing lists win over the built-in Smart/whitelist/ad-block rules:
	// inserted here, after the DNS/server infra rules but before every built-in.
	rules = appendRoutingListRouteRules(cfg.RoutingLists, rules)


	if cfg.Mode == ProxyModeTunnel {
		// Probe domains must go through the proxy/endpoint outbound, even when
		// issued from the app's own process. Without this, the self-direct rule
		// below would route the post-start HTTP probe out via direct, masking a
		// broken tunnel as healthy. The endpoint tag for WG/AWG is also "proxy"
		// (see buildEndpoints), so the same rule routes probes through the
		// WireGuard/AmneziaWG endpoint as well.
		if len(tunnelProbeDomains) > 0 {
			rules = append(rules, SBRouteRule{
				Action:   "route",
				Domain:   append([]string(nil), tunnelProbeDomains...),
				Outbound: "proxy",
			})
		}
		// Self-direct: keep our own process's non-probe traffic (updater,
		// telemetry, internal HTTP) out of the tunnel. Without this, sing-box's
		// auto_route pulls every socket of the host process into the TUN, and
		// for WG/AWG the post-start HTTP probe to gstatic/msftconnecttest/
		// cloudflare races against Windows' multi-homed DNS — the lookups can
		// escape via the LAN adapter and get dropped by strict_route's WFP
		// rules, so the probe times out even though the tunnel is healthy.
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

	// Force-VPN apps: the whole process family goes through the tunnel. Placed
	// before the app-whitelist direct rule so an explicit "via VPN" beats an
	// accidental overlap with the exclusion list (the UI prevents adding one
	// app to both). Tunnel-only: in proxy mode apps that ignore the system
	// proxy never reach us, so the rule would be dead weight.
	if cfg.Mode == ProxyModeTunnel {
		if rx := appWhitelistPathRegexes(cfg.AppForceVPN); len(rx) > 0 {
			// Sending an app through the tunnel wholesale is the workaround
			// users reach for when a service half-works; it must not hand them
			// back the QUIC hang the Smart rule below cures. Smart-only on
			// purpose: in Global mode every app already rides the proxy, so
			// singling out the force-VPN list would fix QUIC for those apps and
			// leave it broken for the rest — an inconsistency worth deciding on
			// its own rather than inheriting from here.
			if cfg.RoutingMode == ModeSmart {
				rules = append(rules, quicRejectRule(SBRouteRule{ProcessPathRegex: rx}))
			}
			rules = append(rules, SBRouteRule{
				Action:           "route",
				ProcessPathRegex: rx,
				Outbound:         "proxy",
			})
		}
	}

	if rx := appWhitelistPathRegexes(cfg.AppWhitelist); len(rx) > 0 {
		rules = append(rules, SBRouteRule{
			Action:           "route",
			ProcessPathRegex: rx,
			Outbound:         "direct",
		})
	}

	// Smart mode: tunnel the censored block-list, leave everything else direct
	// (Final="direct"). Placed BEFORE the whitelist block so a blocked domain
	// that also sits under a whitelisted suffix still tunnels — matching
	// Router.ShouldProxy, where a blocked resource wins over an odd (single)
	// whitelist match. This ordering is what makes the Smart-mode UI work:
	// there the domain list edits config.RoutingRules.CustomBlockedDomains
	// ("route via VPN", see RulesView.jsx), which Router.GetBlockedDomains
	// unions into cfg.BlockedDomains — while cfg.Whitelist still carries the
	// exclusions the user set in Global mode. Emitting the whitelist first
	// would let a stale Global exclusion override an explicit Smart-mode
	// "send this via VPN". The block-list domains are already normalized
	// suffixes. App-whitelist (process) direct rules above keep priority, so
	// an excluded app's traffic stays direct even for blocked domains.
	//
	// A pre-compiled binary rule-set is preferred when the caller supplied one:
	// inlining ~78k suffixes costs ~160 ms of config marshal/parse/index per
	// connect. Same rule, same position — only the matcher's storage differs.
	if cfg.RoutingMode == ModeSmart && len(cfg.BlockedDomains) > 0 {
		if smartRuleSetActive(cfg) {
			route.RuleSet = append(route.RuleSet, SBRuleSet{
				Type:         "local",
				Tag:          smartRuleSetTag,
				Format:       "binary",
				LocalOptions: SBLocalRuleSet{Path: cfg.SmartRuleSetPath},
			})
			rules = append(rules, quicRejectRule(SBRouteRule{RuleSet: []string{smartRuleSetTag}}))
			rules = append(rules, SBRouteRule{
				Action:   "route",
				RuleSet:  []string{smartRuleSetTag},
				Outbound: "proxy",
			})
		} else {
			rules = append(rules, quicRejectRule(SBRouteRule{
				DomainSuffix: append([]string(nil), cfg.BlockedDomains...),
			}))
			rules = append(rules, SBRouteRule{
				Action:       "route",
				DomainSuffix: append([]string(nil), cfg.BlockedDomains...),
				Outbound:     "proxy",
			})
		}
	}

	// Smart mode: tunnel IP-only blocked ranges (Telegram MTProto data centers).
	// These have no domain/SNI, so the domain-suffix rule above can't match
	// them; an ip_cidr rule on the destination address is the only way to pull
	// the native Telegram client through the proxy. The server-IP bypass added
	// at the top of buildRoute still wins, so the tunnel's own endpoint stays
	// direct even if it ever shared a range.
	if cfg.RoutingMode == ModeSmart && len(cfg.BlockedCIDRs) > 0 {
		rules = append(rules, SBRouteRule{
			Action:   "route",
			IPCidr:   append([]string(nil), cfg.BlockedCIDRs...),
			Outbound: "proxy",
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

	latency, ok, r := quicHandshakeProbe(ip, port, "")
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
