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
	"sync/atomic"
	"time"

	"resultproxy-wails/internal/verdict"
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
	// EnableIPv6 is the user-facing "Сеть → IPv6" toggle, default off. It moves
	// BOTH halves at once: the IPv6 address on the TUN and buildDNS's strategy.
	// Attaching the address while DNS stayed ipv4_only would make the setting a
	// lie — no domain would resolve to AAAA, so the TUN's IPv6 would carry
	// nothing but literal-IPv6 traffic.
	EnableIPv6 bool
	// TunIPv6 optionally overrides the default ULA. It says WHICH address, never
	// WHETHER — that is EnableIPv6's job.
	TunIPv6  string
	TunStack string
	// TunDisableIPv6 strips every IPv6 address from the TUN inbound and forces
	// strict_route on. It is a RETRY-ONLY switch, never user-facing: startEngine
	// flips it after sing-tun reports "set ipv6 address: <err>", i.e. the freshly
	// created Wintun adapter refused an IPv6 address that was explicitly opted
	// into via TunIPv6.
	//
	// Without it that failure is a permanent wedge: the message matches
	// isTransientTunError, so the retry fires, tears down a device that was never
	// the problem, and re-Starts the IDENTICAL config — which fails identically.
	//
	// It overrides the explicit TunIPv6 opt-in: a tunnel that will not start is
	// worse than a tunnel without IPv6.
	TunDisableIPv6 bool
	DataDir        string

	// RoutingLists are user routing subscriptions resolved to local
	// source-format rule_set caches. Applied in ALL modes as explicit rules
	// ahead of the built-in Smart/whitelist/ad-block rules, ordered
	// restrictive-first (block > proxy > direct). See buildRoute.
	RoutingLists []RoutingListSpec
	// RoutingOrder is the order the list actions are emitted in. Empty means
	// DefaultRoutingOrder. An active routing profile may ask for another one.
	RoutingOrder []string

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

	// AdaptiveSmart turns on the experimental verdict engine. Its first
	// visible half is FakeIP: with it on, every connection carries the real
	// domain before any rule is matched (fork route/route.go:422), which is
	// what lets UDP and HTTP/3 be classified at all.
	AdaptiveSmart bool
	// AdaptiveSmartBlockBrowserDoH rejects well-known browser DoH endpoints so
	// the browser falls back to the system resolver FakeIP can see.
	AdaptiveSmartBlockBrowserDoH bool
	// SelfExecutablePath is this application's own binary. Its lookups must
	// never be answered with a fake address — see buildDNS.
	SelfExecutablePath string
	// Verdicts is the store the smart outbound asks. Nil means "decide as if
	// nothing is known", which is what every config built by a test does.
	Verdicts *verdict.Store
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
	Type          string          `json:"type,omitempty"`
	Tag           string          `json:"tag"`
	Format        string          `json:"format,omitempty"`
	RemoteOptions SBRemoteRuleSet `json:"-"`
	LocalOptions  SBLocalRuleSet  `json:"-"`
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
	// StoreFakeIP persists the fake address to domain mapping. Mandatory
	// whenever a fakeip server is emitted: without it an in-place reload drops
	// the mapping while clients still hold the addresses, and the fork turns
	// each such connection into a fatal "missing fakeip record"
	// (route/route.go:426).
	StoreFakeIP bool `json:"store_fakeip,omitempty"`
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
	// Optimistic trades a little staleness for a resolver that never blocks on
	// an expired entry — the same bargain the Smart lists already make at
	// startup. Set by newSBDNS, never by hand.
	Optimistic *SBDNSOptimistic `json:"optimistic,omitempty"`
}

// SBDNSOptimistic configures sing-box 1.14's optimistic DNS cache: an expired
// entry is answered immediately while a refresh runs in the background. The
// core rejects it alongside disable_cache or disable_expire, neither of which
// this client emits.
type SBDNSOptimistic struct {
	Enabled bool   `json:"enabled,omitempty"`
	Timeout string `json:"timeout,omitempty"`
}

// newSBDNS builds the DNS block for a real session, so the options every mode
// must share cannot be forgotten by one of buildDNS's exits. The ping engine
// spells out its own tiny DNS block instead and is right not to come here: it
// serves one static hosts record for the node's own name and lives for the
// length of a single measurement, so there is nothing for a cache to be
// optimistic about.
//
// The optimistic window is stated rather than defaulted: the core would serve a
// stale answer for three days, which outlives any network change the user makes
// — six hours still covers a laptop that slept overnight while a move between
// Wi-Fi and mobile refreshes well inside it.
func newSBDNS(servers []SBDNSServer) *SBDNS {
	return &SBDNS{
		Servers:    servers,
		Optimistic: &SBDNSOptimistic{Enabled: true, Timeout: "6h"},
	}
}

type SBDNSServer struct {
	Type       string `json:"type"`
	Tag        string `json:"tag"`
	Server     string `json:"server,omitempty"`
	ServerPort int    `json:"server_port,omitempty"`
	Detour     string `json:"detour,omitempty"`
	// DomainResolver bootstraps a resolver that is itself addressed by a
	// hostname. Without a detour to hide behind, sing-box 1.14 refuses to build
	// the dialer for such a server at all — "missing domain resolver for domain
	// server address" — so this is not a warning but the difference between an
	// engine that starts and one that does not. See buildDNS.
	DomainResolver string `json:"domain_resolver,omitempty"`
	// Predefined seeds a static "hosts" DNS server: domain → fixed IP list.
	// Used to pin the proxy server's own domain to its connect-time IPs so
	// re-resolution never touches the redirected OS resolver (see buildDNS).
	Predefined map[string][]string `json:"predefined,omitempty"`
	// Inet4Range/Inet6Range configure a fakeip server's pools. Only a server
	// of type "fakeip" reads them.
	Inet4Range string `json:"inet4_range,omitempty"`
	Inet6Range string `json:"inet6_range,omitempty"`

	// Servers/Strategy/Timeout configure a server of type "fallback": it holds
	// no address of its own, only the tags of servers to try, in order, each
	// bounded by Timeout. See tunnelDNSResolver.
	Servers  []string `json:"servers,omitempty"`
	Strategy string   `json:"strategy,omitempty"`
	Timeout  string   `json:"timeout,omitempty"`

	// throughDetour marks a fallback wrapper whose members all route through
	// this detour. Never serialised — the core reaches members by tag — it
	// exists so firstDetourServerTag can name the wrapper rather than one of
	// its legs.
	throughDetour string `json:"-"`
}

type SBDNSRule struct {
	Domain           []string `json:"domain,omitempty"`
	DomainSuffix     []string `json:"domain_suffix,omitempty"`
	ProcessPathRegex []string `json:"process_path_regex,omitempty"`
	RuleSet          []string `json:"rule_set,omitempty"`
	// QueryType is no longer a filter that only applies to what the inbound
	// asks. On sing-box 1.14 an internal resolve — the dial for a
	// domain-addressed server — reaches dns/router.go lookupWithRules, which
	// builds real A and AAAA questions, so these rules match there too. Two
	// consequences before adding another one: the lookup fans A and AAAA out as
	// two INDEPENDENT rule walks, so a rule scoped to one type splits a single
	// dial's resolution across two servers; and the only reason the fakeip
	// catch-all below is harmless is that an internal lookup passes
	// allowFakeIP=false and the core skips a fakeip transport outright. That
	// protection comes from the server's type, not from this field.
	// TestQueryTypeIsOnlyEverUsedForTheFakeIPRule holds the boundary.
	QueryType []string `json:"query_type,omitempty"`
	Server    string   `json:"server,omitempty"`
	Action    string   `json:"action,omitempty"`
}

type SBInbound struct {
	Type string `json:"type"`
	Tag  string `json:"tag"`
	// InterfaceName pins the Windows TUN adapter name. Left empty, sing-box
	// falls back to "tun0" (protocol/tun/inbound.go: CalculateInterfaceName)
	// and sing-tun derives the Wintun GUID from that name — so we would share a
	// devnode with every other sing-box client on the machine. See
	// tunInterfaceName for the two constraints on the value.
	InterfaceName       string   `json:"interface_name,omitempty"`
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
	// UDPMapping and UDPFiltering replace endpoint_independent_nat, which
	// sing-box 1.14 kept in the schema but stopped reading — a silently
	// ignored knob is worse than a removed one, because the config still
	// parses and only the behaviour changes. Both take "endpoint_independent",
	// "address_dependent" or "address_and_port_dependent".
	//
	// Endpoint-independent lets multiple destinations share NAT slots for the
	// same (source IP, source port) pair instead of allocating a slot per
	// destination: under browser QUIC storms hitting many CDN IPs from one
	// ephemeral port that cuts the slot count proportionally. It is also the
	// core's new default, which is the opposite of what 1.13 did when the
	// field was absent — so both branches in buildTun say it out loud rather
	// than inherit anything.
	UDPMapping   string `json:"udp_mapping,omitempty"`
	UDPFiltering string `json:"udp_filtering,omitempty"`
	// UDPNATMax caps how many UDP NAT slots the TUN inbound holds at once.
	// UDPTimeout alone only bounds how long a dead flow lingers; under the DPI
	// retry storms this client runs in — browsers reopening QUIC handshakes
	// that get dropped at UDP/443 — the table still grows faster than it
	// drains, and pprof captures showed udpnat2.natConn waiters piling up. Not
	// set for WireGuard endpoints: they keep their own session state, and
	// starving the inbound's table is the same class of mistake that once
	// collapsed live tunnel traffic.
	UDPNATMax uint32 `json:"udp_nat_max,omitempty"`
	// DNSMode says how the TUN interface handles DNS: "disabled", "native"
	// (set the platform's per-interface DNS, which on Windows means the
	// adapter's own DNS servers) or "hijack" (native plus intercepting DNS
	// traffic). sing-box 1.14 defaults to hijack, which is what this client has
	// always relied on — written down here so a future default cannot move it
	// silently, the way endpoint_independent_nat did. DNSAddress is left unset
	// on purpose: the core then derives the hijack address from the TUN address,
	// which is the behaviour that existed before the option did.
	DNSMode string `json:"dns_mode,omitempty"`
}

type SBOutbound struct {
	Type       string `json:"type"`
	Tag        string `json:"tag"`
	Server     string `json:"server,omitempty"`
	ServerPort int    `json:"server_port,omitempty"`
	// DomainResolver names the DNS server that resolves Server when it is a
	// domain rather than a literal IP. See serverDomainResolverTag for which
	// tag belongs here and why the field exists at all; left empty for a
	// literal address, where the core builds no resolve dialer in the first
	// place.
	DomainResolver string `json:"domain_resolver,omitempty"`
	Username       string `json:"username,omitempty"`
	Password       string `json:"password,omitempty"`
	Method         string `json:"method,omitempty"`
	// Plugin/PluginOptions carry SIP003 (obfs-local, v2ray-plugin). A node whose
	// server runs a plugin does not work without them.
	Plugin        string `json:"plugin,omitempty"`
	PluginOptions string `json:"plugin_opts,omitempty"`
	Version       string `json:"version,omitempty"`
	UUID          string `json:"uuid,omitempty"`
	AlterId       int    `json:"alter_id,omitempty"`
	Flow          string `json:"flow,omitempty"`
	// Encryption carries VLESS Encryption (the post-quantum handshake string
	// "mlkem768x25519plus.<mode>.<rtt>[.<padding>].<key>"). Without it a node
	// that runs the feature never completes the handshake the server expects.
	Encryption          string `json:"encryption,omitempty"`
	PacketEncoding      string `json:"packet_encoding,omitempty"`
	GlobalPadding       bool   `json:"global_padding,omitempty"`
	AuthenticatedLength bool   `json:"authenticated_length,omitempty"`
	Security            string `json:"security,omitempty"`
	// Inet4BindAddress pins this outbound's own dialing to one local IPv4.
	//
	// Only the ping probe engine sets it, and only while a tunnel session is
	// up: without it the probe's traffic enters the TUN like everything else
	// and we would be measuring the tunnel through the tunnel. This is the
	// same correction pingLANProbe/autoProbeDialer already apply to the direct
	// probes, moved to where sing-box does the dialing.
	Inet4BindAddress string `json:"inet4_bind_address,omitempty"`
	UpMbps           int    `json:"up_mbps,omitempty"`
	DownMbps         int    `json:"down_mbps,omitempty"`
	// ServerPorts/HopInterval drive Hysteria2 port hopping. sing-quic parses only
	// "start:end" ranges, so the URI's "10000-20000" spelling is converted before
	// it gets here — a range it cannot parse aborts outbound creation.
	ServerPorts []string         `json:"server_ports,omitempty"`
	HopInterval string           `json:"hop_interval,omitempty"`
	Obfs        *SBHysteria2Obfs `json:"obfs,omitempty"`
	Multiplex   *SBMultiplex     `json:"multiplex,omitempty"`

	TLS       *SBOutboundTLS       `json:"tls,omitempty"`
	Transport *SBOutboundTransport `json:"transport,omitempty"`

	DomainStrategy string `json:"domain_strategy,omitempty"`

	// Outbounds names the members of a group outbound. Only a group reads
	// it — for us that is the "smart" type, whose two members are the plain
	// direct and proxy outbounds it chooses between.
	Outbounds []string `json:"outbounds,omitempty"`
}

type SBHysteria2Obfs struct {
	Type     string `json:"type,omitempty"`
	Password string `json:"password,omitempty"`
}

type SBMultiplex struct {
	Enabled        bool   `json:"enabled,omitempty"`
	Protocol       string `json:"protocol,omitempty"`
	MaxConnections int    `json:"max_connections,omitempty"`
	MinStreams     int    `json:"min_streams,omitempty"`
	MaxStreams     int    `json:"max_streams,omitempty"`
	Padding        bool   `json:"padding,omitempty"`
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
	// HandshakeTimeout is set for hysteria2 only, see hysteria2HandshakeTimeout.
	HandshakeTimeout string `json:"handshake_timeout,omitempty"`
}

type SBEndpoint struct {
	Type   string `json:"type"`
	Tag    string `json:"tag"`
	Detour string `json:"detour,omitempty"`
	// DomainResolver resolves a peer addressed by a domain. The endpoint dials
	// through Detour="direct", so without this the peer's address is resolved
	// by the direct outbound's own resolve dialer — the deprecated
	// walk-the-rules path. Naming the server here reaches the same answer
	// without depending on it. See serverDomainResolverTag.
	DomainResolver string              `json:"domain_resolver,omitempty"`
	System         bool                `json:"system,omitempty"`
	Name           string              `json:"name,omitempty"`
	MTU            int                 `json:"mtu,omitempty"`
	Address        []string            `json:"address,omitempty"`
	PrivateKey     string              `json:"private_key,omitempty"`
	ListenPort     int                 `json:"listen_port,omitempty"`
	Peers          []SBWireGuardPeer   `json:"peers,omitempty"`
	UDPTimeout     string              `json:"udp_timeout,omitempty"`
	Workers        int                 `json:"workers,omitempty"`
	DisablePauses  bool                `json:"disable_pauses,omitempty"`
	Amnezia        *SBWireGuardAmnezia `json:"amnezia,omitempty"`
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
	H1 string `json:"h1,omitempty"`
	H2 string `json:"h2,omitempty"`
	H3 string `json:"h3,omitempty"`
	H4 string `json:"h4,omitempty"`
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

	// Xray clients never strip the X25519MLKEM768 key share, and REALITY
	// servers since Xray-core v26.9.8 reject a ClientHello without it.
	SupportX25519MLKEM768 bool `json:"support_x25519mlkem768,omitempty"`
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

	// The rest of the xhttp obfuscation profile (sing-box-extended). These decide
	// where the session id, the sequence number and the uplink payload ride, so a
	// server running an obfs profile only matches when the client mirrors it.
	// session/seq placement: path|cookie|header|query. uplink_data_placement:
	// auto|body always, cookie|header only in packet-up mode — the core validates
	// all of them while decoding the config.
	SessionPlacement     string          `json:"session_placement,omitempty"`
	SessionKey           string          `json:"session_key,omitempty"`
	SeqPlacement         string          `json:"seq_placement,omitempty"`
	SeqKey               string          `json:"seq_key,omitempty"`
	UplinkDataPlacement  string          `json:"uplink_data_placement,omitempty"`
	UplinkDataKey        string          `json:"uplink_data_key,omitempty"`
	SessionIDTable       string          `json:"session_id_table,omitempty"`
	SessionIDLength      json.RawMessage `json:"session_id_length,omitempty"`
	UplinkChunkSize      json.RawMessage `json:"uplink_chunk_size,omitempty"`
	CongestionController string          `json:"congestion_controller,omitempty"`
	CWND                 int             `json:"cwnd,omitempty"`
	ScMaxBufferedPosts   int64           `json:"sc_max_buffered_posts,omitempty"`

	// mKCP (type "mkcp" in sing-box, "kcp" in Xray links). UDP-based, no TLS
	// framing involved — an unmapped network used to fall through to raw TCP,
	// which cannot talk to a mKCP server at all.
	MTU              int    `json:"mtu,omitempty"`
	TTI              int    `json:"tti,omitempty"`
	UplinkCapacity   int    `json:"uplink_capacity,omitempty"`
	DownlinkCapacity int    `json:"downlink_capacity,omitempty"`
	Congestion       bool   `json:"congestion,omitempty"`
	ReadBufferSize   int    `json:"read_buffer_size,omitempty"`
	WriteBufferSize  int    `json:"write_buffer_size,omitempty"`
	HeaderType       string `json:"header_type,omitempty"`
	Seed             string `json:"seed,omitempty"`
}

// SBRoute has no DefaultDomainResolver field, and that is the decision rather
// than an oversight. sing-box 1.14 deprecated resolving a domain-addressed
// server with no resolver named, but naming one here would send every internal
// resolve straight to that transport and past dns.rules entirely — and the rule
// walk is what keeps a blocked domain resolving through the tunnel instead of
// through the censored local resolver. The node's own dial fields are named
// instead (serverDomainResolverTag); TestRouteNeverNamesADefaultDomainResolver
// holds this shut and carries the full reasoning.
type SBRoute struct {
	RuleSet     []SBRuleSet   `json:"rule_set,omitempty"`
	Rules       []SBRouteRule `json:"rules,omitempty"`
	Final       string        `json:"final,omitempty"`
	AutoDetect  bool          `json:"auto_detect_interface,omitempty"`
	FindProcess bool          `json:"find_process,omitempty"`
}

type SBRouteRule struct {
	// Inbound scopes a rule to specific listeners by tag. Used to keep probe
	// traffic ("probe-in", loopback-only in tunnel mode) on a path of its own
	// without touching what the user's apps do with the same destination.
	Inbound          []string `json:"inbound,omitempty"`
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

// probeInboundTag names the loopback-only inbound the app's own health probes
// use in tunnel mode. Route rules key off it to give probe traffic a path the
// user's traffic does not inherit.
const probeInboundTag = "probe-in"

// probeInboundPortValue is the loopback port of the "probe-in" inbound for the
// engine currently configured. The block prober needs it to send its
// through-the-node half somewhere, and the port is chosen while the config is
// built.
//
// Zero means there is no such inbound — proxy mode, or nothing started yet — and
// the prober then refuses to run rather than quietly measuring the direct path
// twice and calling the result a comparison.
var probeInboundPortValue atomic.Int64

func setProbeInboundPort(port int) { probeInboundPortValue.Store(int64(port)) }

func probeInboundPort() int { return int(probeInboundPortValue.Load()) }

// updateInboundTag names the loopback inbound the in-app updater downloads
// through. Everything arriving on it goes to the node regardless of mode,
// whereas the app's own traffic is otherwise kept direct.
const updateInboundTag = "update-in"

// updateInboundPortValue is the port of the "update-in" inbound of the engine
// that is actually running; zero when none is.
var updateInboundPortValue atomic.Int64

// UpdateInboundPort returns the loopback port of the running engine's
// "update-in" inbound, or 0 when no engine is running.
func UpdateInboundPort() int { return int(updateInboundPortValue.Load()) }

func updateInbound() SBInbound {
	return SBInbound{
		Type:       "mixed",
		Tag:        updateInboundTag,
		Listen:     "127.0.0.1",
		ListenPort: getFreeLocalPort(0),
	}
}

func inboundPort(sb SingBoxConfig, tag string) int {
	for _, in := range sb.Inbounds {
		if in.Tag == tag {
			return in.ListenPort
		}
	}
	return 0
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
// smartTunneledApps are the processes Smart mode always sends through the
// tunnel, whatever the domain/CIDR block-lists say. Membership is earned by
// one property: the app carries traffic that no domain or ip_cidr rule can
// classify, so leaving it to the block-lists means leaving it censored.
//
// Discord (measured 2026-08-26 over 64 voice sessions from Discord's own
// renderer log): the client opens its voice gateway to *.discord.media:443 —
// a domain, already covered by the block-list — and is then handed a bare
// media IP:port to send UDP to. Two backends are in rotation:
//
//	104.29.x.x:19294-19335   (Cloudflare)     8 direct / 19 tunnelled / 1 dead
//	 35.217.x.x:50003-50008  (Google Cloud)   0 direct / 19 tunnelled / 17 dead
//
// The GCP backend on the classic 50000+ voice ports never once completed a
// UDP handshake over the direct path — RU DPI kills it outright — while the
// same server connects every time through the tunnel. Neither backend can be
// added to blockedCIDRs: both are shared provider space (PTR
// googleusercontent.com, AS15169; Cloudflare 104.29.0.0/16), and Discord
// rotates the addresses per call, so any static list is stale by design.
// find_process is the only classifier that holds.
var smartTunneledApps = []string{
	"Discord.exe",
	"DiscordPTB.exe",
	"DiscordCanary.exe",
	"DiscordDevelopment.exe",
}

// smartTunneledAppRegexes compiles smartTunneledApps into the process-path
// regexes buildRoute emits. Split out so route building and the find_process
// decision can never disagree about whether the built-in is active.
func smartTunneledAppRegexes(cfg EngineConfig) []string {
	if cfg.Mode != ProxyModeTunnel || cfg.RoutingMode != ModeSmart {
		return nil
	}
	return appWhitelistPathRegexes(smartTunneledApps)
}

func quicRejectRule(sel SBRouteRule) SBRouteRule {
	sel.Action = "reject"
	sel.Method = "default"
	sel.Network = []string{"udp"}
	sel.Port = []int{443}
	sel.Outbound = ""
	return sel
}

// browserDoHDomains are the DoH endpoints browsers ship as built-in providers.
//
// Membership has one criterion, the same one blockedDomainFloor uses: the host
// exists to serve DoH, so a domain_suffix rule on it pulls in nothing else.
// That is why cloudflare-dns.com is here as a whole domain — every
// mozilla./chrome./family./security. prefix under it is a resolver — while
// Quad9 and AdGuard are listed host by host, because their registrable domains
// also carry the company's website, and this rule rejects rather than reroutes:
// swallowing quad9.net would take the site off the network.
//
// Not covered, and it cannot be: a browser pointed at a custom DoH template,
// especially one written as a literal IP. Blocking resolver IPs was considered
// and rejected — the application's own DoH fallback (doh.go) reaches the same
// addresses, and a rule that cannot tell the two apart would cut the ground
// out from under the resolver of last resort.
func browserDoHDomains() []string {
	return []string{
		// Chrome, Edge and Firefox all ship Google's endpoint.
		"dns.google",
		"dns.google.com",
		// The whole domain is the resolver product.
		"cloudflare-dns.com",
		"one.one.one.one",
		// Quad9 by host: quad9.net is also their website.
		"dns.quad9.net",
		"dns9.quad9.net",
		"dns10.quad9.net",
		"dns11.quad9.net",
		"dns.opendns.com",
		"doh.opendns.com",
		"doh.familyshield.opendns.com",
		"dns.nextdns.io",
		"doh.xfinity.com",
		"dns.adguard-dns.com",
		"unfiltered.adguard-dns.com",
		"family.adguard-dns.com",
		"doh.cleanbrowsing.org",
		"dns.controld.com",
		"freedns.controld.com",
	}
}

func effectiveDataDir(cfg EngineConfig) string {
	if cfg.DataDir != "" {
		return cfg.DataDir
	}
	return resultProxyDataDir()
}

// singBoxCacheDBName is the core's own cache file. Named once because the
// FakeIP mapping has to land in this very file and not beside it.
const singBoxCacheDBName = "sing-box-cache.db"

// singBoxLogLevel is "error" unless RESULTV_SINGBOX_LOG_LEVEL says otherwise.
//
// The default is not a preference, it is a necessity: at "info" the core logs
// a line per connection and per DNS answer, and the log window is also what
// the user sends us. But several questions can only be answered by the core's
// own voice — which service a hanging Close is stuck on (box.Close traces each
// one with its elapsed time at "trace"), and whether an answer came out of the
// optimistic cache rather than the network ("optimistic <domain>" at "debug").
// Leaving a documented way in beats rebuilding a special binary each time.
func singBoxLogLevel() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("RESULTV_SINGBOX_LOG_LEVEL"))) {
	case "trace":
		return "trace"
	case "debug":
		return "debug"
	case "info":
		return "info"
	case "warn", "warning":
		return "warn"
	default:
		return "error"
	}
}

func buildExperimentalCache(dataDir string) *SBExperimental {
	if dataDir == "" {
		return nil
	}
	return &SBExperimental{
		CacheFile: &SBCacheFile{
			Enabled: true,
			Path:    filepath.Join(dataDir, singBoxCacheDBName),
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
	// DNS first: in proxy mode the node's own resolver tag is whichever
	// transport the core would have reached for by itself, so the tag cannot be
	// named before the server list exists.
	dns := buildDNS(cfg)
	nodeResolver := serverDomainResolverTag(cfg.Proxy, ProxyModeProxy, dns)
	endpoints, err := buildEndpoints(cfg.Proxy, nodeResolver)
	if err != nil {
		return SingBoxConfig{}, err
	}
	sbCfg := SingBoxConfig{
		Log:       &SBLog{Level: "error", Disabled: true},
		DNS:       dns,
		Endpoints: endpoints,
		Inbounds: []SBInbound{{
			Type:       "mixed",
			Tag:        "mixed-in",
			Listen:     host,
			ListenPort: port,
		}, updateInbound()},
		Outbounds:    buildOutbounds(cfg.Proxy, nodeResolver, ""),
		Route:        buildRoute(cfg),
		Experimental: buildExperimentalCache(dd),
	}

	return sbCfg, nil
}

// hostSupportsIPv6 reports whether the host has an IPv6 stack at all — the
// question "will the adapter accept an IPv6 address", NOT "could IPv6 leak".
// A link-local fe80:: counts and is in fact the normal signal: with neither
// adapter-level IPv6 nor OS-wide DisabledComponents in play, every box has one,
// and that is enough for sing-tun's CreateUnicastIpAddressEntry to succeed.
//
// This is the preventive half of the no-IPv6 guard: without it, a user ticking
// the IPv6 box on a machine where IPv6 is switched off gets
// "set ipv6 address: ..." — which does not degrade the tunnel, it kills the
// whole inbound.
//
// Conservative-fail: on enumeration error assume yes and let the reactive half
// (TunDisableIPv6, see startEngine) catch it, rather than silently withholding
// IPv6 from a user who asked for it because a probe was flaky.
func hostSupportsIPv6() bool {
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
			if ip != nil && ip.To4() == nil && ip.To16() != nil {
				return true
			}
		}
	}
	return false
}

// tunCarriesIPv6 is the single predicate behind both halves of the toggle, so
// the TUN address and the DNS strategy can never disagree.
func tunCarriesIPv6(cfg EngineConfig) bool {
	return cfg.EnableIPv6 && !cfg.TunDisableIPv6 && hostSupportsIPv6Fn()
}

// hasRoutableIPv6 reports whether the host holds an IPv6 address that can
// actually reach the internet — the only kind that can leak.
//
// It replaced systemHasIPv6, which answered a different question ("can this box
// take an IPv6 address at all") and counted link-local fe80:: — present on
// practically every Windows machine. That made it useless as a leak signal:
// keyed on it, we would force the WFP filters on essentially everyone.
//
// Conservative-fail: on enumeration error assume yes, so a flaky probe makes us
// over-block rather than leak.
func hasRoutableIPv6() bool {
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
			if isLeakableIPv6(ip) {
				return true
			}
		}
	}
	return false
}

// isLeakableIPv6 reports whether ip is an IPv6 address that could carry traffic
// out to the internet past the tunnel. Link-local (fe80::/10) and ULA (fc00::/7)
// cannot, so they are not leaks — note Go's IsGlobalUnicast returns TRUE for ULA,
// which is why the ULA check is explicit rather than implied.
func isLeakableIPv6(ip net.IP) bool {
	if ip == nil || ip.To4() != nil || ip.To16() == nil {
		return false
	}
	if !ip.IsGlobalUnicast() {
		return false
	}
	if v6 := ip.To16(); v6[0]&0xfe == 0xfc {
		return false
	}
	return true
}

func BuildTunnelModeConfig(cfg EngineConfig) (SingBoxConfig, error) {
	tunIPv4 := "172.19.0.1/30"
	if cfg.TunIPv4 != "" {
		tunIPv4 = cfg.TunIPv4
	}
	// IPv6 on the TUN is off unless the user turns it on (EnableIPv6). It used to
	// be attached automatically whenever the host had any IPv6 at all, while
	// buildDNS pinned strategy=ipv4_only — so no domain ever resolved to AAAA and
	// the address carried nothing but literal-IPv6 traffic. That bought close to
	// nothing and owned a whole class of hard failures: when Windows refuses the
	// address, sing-tun fails the ENTIRE inbound with "set ipv6 address: ..." and
	// the tunnel does not come up.
	//
	// tunCarriesIPv6 folds in both guards: hostSupportsIPv6 (the user ticked the
	// box on a machine with no IPv6 stack — behave as if unticked) and
	// TunDisableIPv6 (the adapter already refused the address on the previous
	// attempt, so honouring the opt-in would rebuild the config that just failed).
	tunIPv6 := "fdfe:dcba:9876::1/126"
	if cfg.TunIPv6 != "" {
		tunIPv6 = cfg.TunIPv6
	}
	tunAddresses := []string{tunIPv4}
	if tunCarriesIPv6(cfg) {
		tunAddresses = append(tunAddresses, tunIPv6)
	}
	tunStack := effectiveTunStack(cfg.TunStack)
	// strict_route adds WFP filters on Windows that drop outbound packets
	// bypassing the TUN. This is the only reliable way to stop Smart
	// Multi-Homed Name Resolution from leaking DNS queries to the LAN
	// adapter, where Russian ISPs transparently hijack UDP/53 (Rostelecom,
	// MSK-IX). User-controlled via the "DNS leak protection" toggle.
	//
	// Forced on when the TUN carries no IPv6 while the host holds a globally
	// routable one. Without the WFP filters that traffic is not blackholed, it is
	// routed out the physical adapter. buildDNS's ipv4_only keeps domain traffic
	// off IPv6, but any app running its own DoH resolver — every modern browser —
	// gets AAAA independently, so the leak is real rather than theoretical.
	//
	// Deliberately keyed on hasRoutableIPv6 and not on "IPv6 is absent from the
	// TUN": a host with only fe80::/fd00:: has no IPv6 that can reach the
	// internet, and forcing the filters there would silently re-enable a feature
	// the user turned off, for no gain.
	ipv6Unrouted := len(tunAddresses) == 1 && hasRoutableIPv6Fn()
	strictRoute := cfg.DNSLeakProtection || ipv6Unrouted

	pt := strings.ToUpper(strings.TrimSpace(cfg.Proxy.Type))

	// Exclude EVERY backend IP the server resolved to (a CDN domain has several,
	// and sing-box may fail over among them mid-session) so none of the server's
	// own traffic loops back into the TUN. Domains alone yield nothing here
	// (net.ParseIP fails on a hostname) — the pinned IP set is what gives
	// domain-addressed servers their exclude CIDRs.
	//
	// WireGuard and AmneziaWG used to be left out of this, and that is what took
	// the tunnel down on engine 1.14. Without the exclusion the node's own UDP
	// enters the TUN and is let back out by the routing rule matching the
	// server's address — the core log says it outright, "inbound packet
	// connection to <server>:3306" — so every byte the tunnel carries crosses
	// the inbound twice: once as payload, once as the encrypted packet carrying
	// it. On 1.13 that only wasted work. sing-tun 0.9 rebuilt both NAT tables
	// and put a flow dispatcher in front of every packet, and the same loop now
	// strangles the session.
	//
	// Measured on the same node, same engine, back to back (tunrepro bench,
	// 15.09.2026): without the exclusion, three 25 MB downloads all failed,
	// every small request timed out, tcp_established never left zero and
	// rx_bytes crawled from 1516 to 4452 in eighty seconds. With it, the same
	// three downloads ran at 170, 101 and 142 Mbit/s, small requests answered in
	// 74-224 ms, and retransmits and resets stayed at zero.
	//
	// The exclusion has a cost, and it is the reason this is pinned to resolved
	// addresses rather than done by name: a node whose CDN moves it to a backend
	// outside the pinned set would have its handshake routed into the tunnel it
	// is trying to establish. That risk is identical for every other protocol
	// here, which has carried this exclusion for months.
	var routeExclude []string
	for _, host := range serverPinnedIPs(cfg.Proxy) {
		if serverIP := net.ParseIP(host); serverIP != nil {
			cidr := host + "/32"
			if serverIP.To4() == nil {
				cidr = host + "/128"
			}
			routeExclude = append(routeExclude, cidr)
		}
	}

	dd := effectiveDataDir(cfg)
	// In tunnel mode the tag does not depend on the built server list — it
	// mirrors the DNS rule buildDNS emits for this same domain — so it can be
	// named before the DNS block exists.
	nodeResolver := serverDomainResolverTag(cfg.Proxy, ProxyModeTunnel, nil)
	outbounds := buildOutbounds(cfg.Proxy, nodeResolver, directDomainResolverTag(cfg))
	if adaptiveSmartActive(cfg) {
		outbounds = append(outbounds, SBOutbound{
			Type:      smartOutboundTag,
			Tag:       smartOutboundTag,
			Outbounds: []string{"direct", "proxy"},
		})
	}

	endpoints, err := buildEndpoints(cfg.Proxy, nodeResolver)
	if err != nil {
		return SingBoxConfig{}, err
	}
	// UDPTimeout / UDPMapping / UDPFiltering are TUN-inbound NAT knobs aimed at
	// cleaning up dead UDP flows under DPI-driven QUIC retry storms. The
	// timeout must NOT be applied when the active protocol is a WireGuard
	// endpoint: for WG/AWG the TUN inbound feeds packets straight into the
	// endpoint, which maintains its own session state, and forcing the inbound
	// to expire NAT slots after 30s tore down live tunnel traffic (handshake
	// passes, browser works for ~30s, then every UDP flow inside the tunnel
	// collapses). The timeout stays at the inbound default there, but the NAT
	// behaviour can no longer be left unsaid: sing-box 1.13 defaulted to
	// symmetric and 1.14 defaults to endpoint-independent, so silence would
	// now mean the opposite of what this branch intends.
	tun := SBInbound{
		Type:                "tun",
		Tag:                 "tun-in",
		InterfaceName:       tunInterfaceName,
		Address:             tunAddresses,
		Stack:               tunStack,
		AutoRoute:           true,
		StrictRoute:         strictRoute,
		RouteExcludeAddress: routeExclude,
		DNSMode:             "hijack",
	}
	if pt != "WIREGUARD" && pt != "AMNEZIAWG" {
		tun.UDPTimeout = "30s"
		tun.UDPMapping = "endpoint_independent"
		tun.UDPFiltering = "endpoint_independent"
		// 8192 is a starting value, not a measured one: it is far above what
		// ordinary browsing holds open, so the cap only bites during a storm.
		// If real traffic ever starts hitting it — UDP failing while TCP is
		// fine — raise it and write down what forced the change.
		tun.UDPNATMax = 8192
	} else {
		// Same NAT behaviour this branch had before 1.14, now stated explicitly
		// because the core default moved out from under it.
		tun.UDPMapping = "address_and_port_dependent"
		tun.UDPFiltering = "address_and_port_dependent"
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
	setProbeInboundPort(probePort)
	probeIn := SBInbound{
		Type:       "mixed",
		Tag:        probeInboundTag,
		Listen:     "127.0.0.1",
		ListenPort: probePort,
	}
	sbCfg := SingBoxConfig{
		Log:          &SBLog{Level: singBoxLogLevel(), Disabled: false},
		DNS:          buildDNS(cfg),
		Endpoints:    endpoints,
		Inbounds:     []SBInbound{tun, probeIn, updateInbound()},
		Outbounds:    outbounds,
		Route:        buildRoute(cfg),
		Experimental: buildExperimentalCache(dd),
	}
	// A fakeip server without a persistent mapping is a time bomb: see the
	// comment on SBCacheFile.StoreFakeIP. buildExperimentalCache already emits
	// that same file for every other reason the core caches things, so this
	// only has to make sure it exists and carries the flag — a second cache
	// file would split the mapping away from the rest of the core's state.
	if adaptiveSmartActive(cfg) {
		if sbCfg.Experimental == nil {
			sbCfg.Experimental = &SBExperimental{}
		}
		if sbCfg.Experimental.CacheFile == nil {
			sbCfg.Experimental.CacheFile = &SBCacheFile{}
		}
		sbCfg.Experimental.CacheFile.Enabled = true
		sbCfg.Experimental.CacheFile.StoreFakeIP = true
		if sbCfg.Experimental.CacheFile.Path == "" {
			sbCfg.Experimental.CacheFile.Path = filepath.Join(dd, singBoxCacheDBName)
		}
	}

	return sbCfg, nil
}

// effectiveTunStack resolves which TUN stack to run, letting RESULTV_TUN_STACK
// override the stored setting.
//
// The override is here because the stack is the one half of the tunnel the app
// offers no way to change, and 14.09.2026 put it under suspicion: with the
// engine on 1.14 the same AmneziaWG node moves 109 Mbit/s in proxy mode with
// zero retransmits, and collapses within a minute of real throughput in tunnel
// mode. The endpoint, the WireGuard device and its own gVisor stack are the
// same objects in both, so what differs is the TUN inbound — and sing-tun 0.9
// rewrote the system stack, putting a flow dispatcher in front of every packet
// and replacing both NAT tables. Comparing system against gvisor needs a switch
// the user can flip without editing an encrypted config.
func effectiveTunStack(stack string) string {
	if override := strings.ToLower(strings.TrimSpace(os.Getenv("RESULTV_TUN_STACK"))); override != "" {
		switch override {
		case "gvisor", "system":
			return override
		}
	}
	switch strings.ToLower(strings.TrimSpace(stack)) {
	case "gvisor":
		return "gvisor"
	default:
		return "system"
	}
}

// buildOutbounds assembles the outbound list. domainResolver is the tag from
// serverDomainResolverTag and lands on the node's own outbound;
// directResolver is directDomainResolverTag's answer for "direct" and is empty
// unless FakeIP is on. See directDomainResolverTag for why that one exists.
func buildOutbounds(proxy ProxyConfig, domainResolver, directResolver string) []SBOutbound {
	directOut := SBOutbound{Type: "direct", Tag: "direct", DomainResolver: directResolver}
	pt := strings.ToUpper(strings.TrimSpace(proxy.Type))
	if pt == "WIREGUARD" || pt == "AMNEZIAWG" {
		return []SBOutbound{
			directOut,
			{Type: "block", Tag: "block"},
		}
	}
	proxyOut := buildProxyOutbound(proxy)
	proxyOut.DomainResolver = domainResolver
	outbounds := []SBOutbound{
		directOut,
		{Type: "block", Tag: "block"},
		proxyOut,
	}
	return outbounds
}

// directDomainResolverTag names the server that resolves a domain the direct
// outbound is asked to dial.
//
// It is needed only under FakeIP, and there it is not a preference but the
// difference between a working direct path and none at all. FakeIP replaces the
// destination address with the NAME before any rule runs, so every direct dial
// becomes a dial-by-name; with no resolver on the outbound the core resolves it
// by walking the DNS rules, where the fakeip catch-all claims it, and a fakeip
// transport cannot answer an internal lookup. Measured on a live engine
// (2026-09-22): ten seconds of silence, then "lookup example.com: context
// deadline exceeded", for every destination that was not on the block-list.
//
// "local" and not the tunnel resolver: this is the direct path, and Smart
// already resolves direct traffic through the system resolver for the GeoDNS
// reason dns.Final carries. buildDNS emits the tag in every tunnel-mode branch.
func directDomainResolverTag(cfg EngineConfig) string {
	if !adaptiveSmartActive(cfg) {
		return ""
	}
	return "local"
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

// serverDomainResolverTag names the DNS server that must answer for the node's
// own address, for the `domain_resolver` dial field on the outbound or endpoint
// that dials it. Empty when the node is a literal IP: there is nothing to
// resolve and the core builds no resolve dialer at all.
//
// Why the field exists. sing-box 1.14 deprecated dialing a domain-addressed
// server with no resolver named, and scheduled the fallback for removal. On
// this fork the fallback survives and is in fact the semantics the rest of the
// config depends on — it walks dns.rules, which is how a blocked domain still
// leaves through the tunnel — so `route.default_domain_resolver` stays unset on
// purpose (see SBRoute). The node is the one dial where the right answer is
// known ahead of any rule, so it is the one that gets spelled out.
//
// Which tag. In tunnel mode the answer mirrors the DNS rule buildDNS already
// emits for the same domain: the static hosts record when connect time pinned
// the server's IPs, the system resolver when it did not. It must never be a
// resolver that rides the tunnel — that tunnel is what this dial is opening.
//
// Proxy mode has no TUN, so nothing redirects the system resolver and no hosts
// record is built. Naming "local" there would move the node's own domain off
// the encrypted resolver it uses today and hand it to the ISP in plaintext, so
// the tag is whatever the core would have chosen by itself: the first
// registered transport. Turning the field on then changes nothing but the
// deprecation.
func serverDomainResolverTag(proxy ProxyConfig, mode ProxyMode, dns *SBDNS) string {
	if proxy.IP == "" || net.ParseIP(proxy.IP) != nil {
		return ""
	}
	if mode == ProxyModeTunnel {
		if len(serverPinnedIPs(proxy)) > 0 {
			return serverPinDNSTag
		}
		return "local"
	}
	if dns != nil && len(dns.Servers) > 0 {
		return dns.Servers[0].Tag
	}
	return ""
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

// osConnectivityProbeDomains are the hostnames the operating system uses to
// decide whether this machine has internet at all. They are matched as
// suffixes because each family has several members (ipv6., www., dns.) and
// Windows has changed which one it asks for between releases.
var osConnectivityProbeDomains = []string{
	"msftconnecttest.com",
	"msftncsi.com",
}

const (
	fakeIPTag        = "fakeip"
	fakeIPInet4Range = "198.18.0.0/15"
	fakeIPInet6Range = "fc00::/18"
)

// serverPinDNSTag names the static `hosts` DNS server seeded with the node's
// connect-time IPs. Both the DNS rule that points at it and the `domain_resolver`
// dial field that names it spell the tag through this constant, so the two can
// never drift apart — a dial field naming an unregistered server is a dead
// engine, not a warning.
const serverPinDNSTag = "server-pin"

// isFakeIPAddr reports whether an address came out of the fake pool rather than
// out of the internet.
//
// This has to be checked at the point of USE, not prevented at the point of
// answer: on Windows a name is resolved by the DNS Client service, not by the
// process that asked, so every lookup reaches the engine wearing svchost's
// name and the process_path_regex exemption in buildDNS cannot see ours. The
// application therefore gets fake addresses like everyone else, and the only
// place that can tell is the code about to dial one.
//
// A fake address is harmless while the connection goes through the TUN — the
// router turns it back into the name before matching a rule. It is fatal the
// moment we deliberately bypass the TUN, which is exactly what the LAN-bound
// probes do: 198.18.x.x means nothing on the physical adapter, and the dial
// sits there until it times out.
//
// The ranges are safe to reject unconditionally: 198.18.0.0/15 is RFC 2544
// benchmarking space and fc00::/18 is ULA — no reachable server lives in
// either, whatever produced the answer.
func isFakeIPAddr(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, cidr := range []string{fakeIPInet4Range, fakeIPInet6Range} {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// realIPv4s keeps the IPv4 addresses in a resolver answer that a socket can
// actually reach, dropping fakes and duplicates.
//
// Every place the application resolves a name for ITS OWN use has to go through
// this: the pings and the AUTO sweep dial bound to the physical adapter, and
// the server pin feeds route_exclude_address, so a fake address there does not
// degrade anything gracefully — it points the tunnel at itself. An empty result
// is the signal for the caller to fall back to DoH, which every one of them
// already knows how to do.
func realIPv4s(addrs []net.IPAddr) []string {
	seen := make(map[string]struct{}, len(addrs))
	var out []string
	for _, a := range addrs {
		v4 := a.IP.To4()
		if v4 == nil || isFakeIPAddr(v4) {
			continue
		}
		s := v4.String()
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// adaptiveSmartActive reports whether the experimental verdict engine is on for
// this config. One predicate for the whole feature, because every part of it
// stands or falls together: the fake pool exists to carry a name to the smart
// outbound, and the smart outbound exists to compare two paths.
//
// Proxy mode is excluded: it has no TUN, so nothing would route the fake range
// anywhere.
//
// WireGuard and AmneziaWG are excluded — but NOT for the reason this comment
// used to give. It claimed a group naming "proxy" would point at a tag the core
// cannot resolve, because buildOutbounds emits only direct+block for endpoint
// protocols. That is wrong and was wrong on 1.13 too: OutboundManager.Outbound
// falls back to endpoint.Get (adapter/outbound/manager.go), so smartOutbound.Start
// would find the WireGuard endpoint under that tag like any other member.
//
// What is true is that nobody has ever run the fake pool and the verdict engine
// against an endpoint: the smart outbound would be racing a FlowOutbound, whose
// packets can bypass the connection path entirely, and none of that has been
// measured. The exclusion stays until it is — as an untested path, not an
// impossible one.
//
// Separately: FakeIP used to be emitted for these nodes anyway, which bought
// every cost of the fake pool (launchers seeing 198.18.x.x, names with no A
// record turning into dead connections) and none of the benefit, since with no
// second member there is nobody to ask what was learned.
func adaptiveSmartActive(cfg EngineConfig) bool {
	if !cfg.AdaptiveSmart || cfg.Mode != ProxyModeTunnel || cfg.RoutingMode != ModeSmart {
		return false
	}
	pt := strings.ToUpper(strings.TrimSpace(cfg.Proxy.Type))
	return pt != "WIREGUARD" && pt != "AMNEZIAWG"
}

// firstDetourServerTag returns the tag of the first DNS server routed through
// the given detour: the tunnel resolver that Smart mode's rules point at and
// that every other tunnel mode uses as dns.final.
func firstDetourServerTag(servers []SBDNSServer, detour string) string {
	// A fallback wrapper is what rules must point at: its legs carry the
	// detour, and naming a leg directly would give up the other one. Wrappers
	// are emitted after their legs (the core resolves members by tag at
	// construction, so the legs have to exist first), which is why this scans
	// for the wrapper before falling back to a plain server.
	for _, s := range servers {
		if s.throughDetour == detour && s.Tag != "" {
			return s.Tag
		}
	}
	for _, s := range servers {
		if s.Detour == detour && s.Tag != "" {
			return s.Tag
		}
	}
	return ""
}

// tunnelDNSResolver emits the servers that carry one resolver through the
// tunnel: DoH first, plain DNS-over-TCP second, and a "fallback" wrapper that
// rules point at.
//
// It used to be a single DNS-over-TCP server, and on engine 1.14 that stopped
// answering. sing-box 1.14 put a query multiplexer
// (dns/transport/multiplexer.go) in front of the tcp, tls and udp transports:
// once a background probe decides the resolver supports reuse, every query
// moves onto one shared, long-lived connection whose liveness check is
// `conn != nil`. Through a proxy outbound that connection wedges.
//
// Measured on the user's live tunnel (15.09.2026, core log at debug): 294 of
// 295 lookups routed to the node never came back — no answer, no error, not
// one "lookup failed", so they hung until the engine was stopped and the
// browser never learned a single YouTube address. Ordinary TCP through the
// same node was healthy the whole time: the connectivity probe answered in
// 461 ms and 129 connections completed normally. On a stand the failure shows
// as "write request: EOF" returned in 0-1 ms, query after query.
//
// The https transport is the only remote transport the multiplexer does not
// touch, which is why DoH leads. The tcp leg stays rather than being deleted:
// a resolver that speaks no DoH would otherwise have no path at all. The
// wrapper bounds each leg with a timeout, so even a wedged leg now costs one
// timeout instead of hanging forever the way the bare transport did.
//
// port applies to the tcp leg only. DoH is HTTPS and has to reach 443, so a
// resolver pinned to a non-standard DNS port keeps that port on the tcp leg
// while DoH tries the standard one and, failing that, hands over.
func tunnelDNSResolver(tag, server string, port int, detour string) []SBDNSServer {
	return []SBDNSServer{
		{Type: "https", Tag: tag + "-doh", Server: server, Detour: detour},
		{Type: "tcp", Tag: tag + "-tcp", Server: server, ServerPort: port, Detour: detour},
		{
			Type:          "fallback",
			Tag:           tag,
			Servers:       []string{tag + "-doh", tag + "-tcp"},
			Strategy:      "sequential",
			Timeout:       "5s",
			throughDetour: detour,
		},
	}
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

		// Every resolver below is emitted by tunnelDNSResolver as a DoH leg, a
		// DNS-over-TCP leg and the fallback wrapper rules point at. Read its
		// comment before changing the shape: a bare tcp server is what stopped
		// answering on engine 1.14.
		servers := []SBDNSServer{}
		if len(cfg.DNSServers) > 0 {
			for i, raw := range cfg.DNSServers {
				server, port := splitDNSServer(raw)
				if server == "" {
					continue
				}
				servers = append(servers, tunnelDNSResolver(fmt.Sprintf("custom-%d", i+1), server, port, detour)...)
			}
			servers = append(servers, SBDNSServer{Type: "local", Tag: "local"})
		} else {
			servers = append(servers, tunnelDNSResolver("google", "8.8.8.8", 0, detour)...)
			servers = append(servers, tunnelDNSResolver("cloudflare", "1.1.1.1", 0, detour)...)
			servers = append(servers, SBDNSServer{Type: "local", Tag: "local"})
		}

		dns := newSBDNS(servers)

		// Same predicate as the TUN address, so the two halves can never disagree:
		// AAAA answers with no IPv6 path to use them would be worse than no AAAA.
		// prefer_ipv4 rather than prefer_ipv6 even when enabled — IPv4 stays the
		// first choice and IPv6 is the fallback, the cautious reading of "IPv6 on".
		dns.Strategy = "ipv4_only"
		if tunCarriesIPv6(cfg) {
			dns.Strategy = "prefer_ipv4"
		}

		// The application's own lookups must never be answered from the fake
		// pool: the prober, the updater and the subscription fetch would each
		// receive a perfectly successful answer of 198.18.x.x and fail in
		// silence. The project has already paid for the quieter version of this
		// bug once, when the app's own resolver was killed by its own DNS
		// override.
		//
		// READ THIS BEFORE RELYING ON IT: on Windows this rule almost never
		// fires, and it is NOT what protects the app. Names are resolved by the
		// DNS Client service inside svchost, not by the process that asked, so
		// the engine matches svchost against this regex and misses. Measured on
		// a live tunnel: resolvePingHost, going through net.DefaultResolver from
		// our own process, still got example.com = 198.18.0.224 with this rule
		// in place, and the ping bound to the physical adapter then spent the
		// full five seconds timing out against it.
		//
		// What actually protects the app is the check at the point of use —
		// isFakeIPAddr and realIPv4s, applied in resolvePingHost,
		// probeDirectDial, resolveSelfServerIPs and pickIPv4. The rule is kept
		// because it is free and does fire for a lookup that reaches sing-box
		// from our process directly, bypassing getaddrinfo; it must never be
		// counted as the defence. Emitted FIRST because DNS rules are ordered
		// and everything below would otherwise claim what it does catch.
		if adaptiveSmartActive(cfg) && cfg.SelfExecutablePath != "" {
			if rx := appWhitelistPathRegexes([]string{cfg.SelfExecutablePath}); len(rx) > 0 {
				dns.Rules = append(dns.Rules, SBDNSRule{
					ProcessPathRegex: rx,
					Server:           "local",
				})
			}
		}

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
					Tag:        serverPinDNSTag,
					Predefined: map[string][]string{cfg.Proxy.IP: pinned},
				})
				dns.Rules = append(dns.Rules, SBDNSRule{
					Domain: []string{cfg.Proxy.IP},
					Server: serverPinDNSTag,
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
		//
		// Same Windows defect as the self-exemption above, and it predates the
		// adaptive work: getaddrinfo hands the query to the DNS Client service,
		// so the engine sees svchost and this regex does not match. The rule
		// therefore does not do the job it was written for — an excluded app's
		// lookups are still resolved by whatever the rules below decide, not by
		// this one. It is kept rather than deleted because it is free and it is
		// correct wherever a process resolves names itself instead of calling
		// getaddrinfo; it must not be read as a guarantee.
		//
		// What the excluded app's lookup actually hits, once this rule misses:
		// in Smart mode without the adaptive engine, dns.Final = "local" below,
		// so it lands on the system resolver and the original symptom is gone
		// by accident. With the adaptive engine on, the fakeip catch-all is the
		// last rule and claims every A/AAAA, so the app is handed a fake
		// address — which still works, because the route-level process rule
		// (buildRoute matches processes on the connection, where Windows does
		// preserve the owner) sends the connection direct and the direct
		// outbound resolves the real name. Global mode keeps the original
		// symptom in full. A real fix needs a selector the Windows resolver
		// preserves, and the rule language has none today.
		if rx := appWhitelistPathRegexes(cfg.AppWhitelist); len(rx) > 0 {
			dns.Rules = append(dns.Rules, SBDNSRule{
				ProcessPathRegex: rx,
				Server:           "local",
			})
		}

		// Excluded domains leave direct, so they resolve via the system resolver.
		if cfg.RoutingMode != ModeSmart {
			if tunnelTag := firstDetourServerTag(dns.Servers, detour); tunnelTag != "" {
				for _, w := range whitelistSuffixes(cfg.Whitelist) {
					server := tunnelTag
					if w.direct {
						server = "local"
					}
					dns.Rules = append(dns.Rules, SBDNSRule{
						DomainSuffix: []string{w.suffix},
						Server:       server,
					})
				}
			}
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

		// The OS connectivity probes have to keep getting the truth. A probe
		// exists to answer "is there a working path", and a fake address makes
		// it answer yes every time.
		//
		// One of them makes the cost visible: ipv6.msftconnecttest.com has AAAA
		// records and no A record at all. FakeIP answers every A query anyway,
		// so Windows dialled 198.18.x.x, the router turned that back into the
		// name, and the direct outbound resolved it for real under ipv4_only —
		// "lookup ipv6.msftconnecttest.com: empty result" for a probe that had
		// simply returned nothing before. Sent to the system resolver, it goes
		// back to returning nothing, and no connection is opened over a name
		// that was never going to resolve.
		if adaptiveSmartActive(cfg) {
			dns.Rules = append(dns.Rules, SBDNSRule{
				DomainSuffix: append([]string(nil), osConnectivityProbeDomains...),
				Server:       "local",
			})
		}

		// FakeIP goes last: every exemption above has already claimed what it
		// needs, and what is left is the traffic whose name we actually want to
		// carry into the router. Scoped to A/AAAA because that is all a fake
		// answer can stand in for. dns.Final stays a real server — the fork
		// rejects a fakeip default outright (dns/transport_manager.go:217).
		//
		// This rule is also reached by the engine's OWN lookups on 1.14, where
		// query_type no longer confines a rule to the inbound's questions (see
		// SBDNSRule.QueryType). It does no harm there only because such a lookup
		// carries allowFakeIP=false and resolveDNSRoute skips the transport,
		// falling through to dns.Final — so a dial for a domain-addressed server
		// never receives 198.18.x.x. Change the server type here and that
		// protection is gone.
		if adaptiveSmartActive(cfg) {
			server := SBDNSServer{
				Type:       "fakeip",
				Tag:        fakeIPTag,
				Inet4Range: fakeIPInet4Range,
			}
			if tunCarriesIPv6(cfg) {
				server.Inet6Range = fakeIPInet6Range
			}
			dns.Servers = append(dns.Servers, server)
			dns.Rules = append(dns.Rules, SBDNSRule{
				QueryType: []string{"A", "AAAA"},
				Server:    fakeIPTag,
			})
			if dns.Final == "" {
				dns.Final = "local"
			}
		}

		// Left empty, the core would default to the first registered server,
		// which is a bare DoH leg, and a resolver without DoH would never reach
		// its TCP leg.
		if dns.Final == "" {
			dns.Final = firstDetourServerTag(dns.Servers, detour)
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
			entry := SBDNSServer{
				Type:       "udp",
				Tag:        fmt.Sprintf("custom-%d", i+1),
				Server:     server,
				ServerPort: port,
			}
			// A resolver named by hostname has to be resolved by something
			// before it can answer anything, and nothing here validates that
			// the user typed an IP. In tunnel mode the detour covers it — the
			// node resolves the name remotely — but in proxy mode there is no
			// detour, and sing-box 1.14 refuses to build the dialer at all
			// rather than guess: "missing domain resolver for domain server
			// address". That is a dead engine for every node, from one
			// hostname in a settings field. The system resolver is the only
			// answer that cannot be circular.
			if net.ParseIP(server) == nil {
				entry.DomainResolver = "local"
			}
			servers = append(servers, entry)
		}
		servers = append(servers, SBDNSServer{Type: "local", Tag: "local"})
	} else {
		servers = []SBDNSServer{
			{Type: "tls", Tag: "cloudflare-tls", Server: "1.1.1.1"},
			{Type: "tls", Tag: "google-tls", Server: "8.8.8.8"},
			{Type: "local", Tag: "local"},
		}
	}

	return newSBDNS(servers)
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
	builtinAppRegexes := smartTunneledAppRegexes(cfg)
	findProcess := len(cfg.AppWhitelist) > 0 ||
		(cfg.Mode == ProxyModeTunnel && len(cfg.AppForceVPN) > 0) ||
		len(builtinAppRegexes) > 0
	// Smart mode inverts the default: everything goes direct and only the
	// censored block-list is tunneled (see the blocked-domain rule below).
	// Global/Whitelist keep proxy as the catch-all.
	final := "proxy"
	if cfg.RoutingMode == ModeSmart {
		// Smart mode inverts the default: everything not on the block-list goes
		// direct. With the adaptive engine on, that default stops being a blind
		// "direct" and becomes "ask what we know about this one" — every explicit
		// rule above still fires first and still wins, so what reaches final is
		// exactly the traffic no list had an opinion about.
		final = "direct"
		if adaptiveSmartActive(cfg) {
			final = smartOutboundTag
		}
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

	// Ahead of the user's lists and the self-direct rule: the updater must reach
	// GitHub through the node even when Smart would send github.com direct.
	rules = append(rules, SBRouteRule{
		Action:   "route",
		Inbound:  []string{updateInboundTag},
		Outbound: "proxy",
	})

	// User routing lists win over the built-in Smart/whitelist/ad-block rules:
	// inserted here, after the DNS/server infra rules but before every built-in.
	rules = appendRoutingListRouteRules(cfg.RoutingLists, rules, cfg.RoutingOrder)

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
		// The UDP relay probe (ProbeUDPRelay) must measure the node, not the
		// local uplink: in Smart mode Final=direct would otherwise send its
		// STUN packets straight out and report every node as UDP-capable.
		// Scoped to the probe inbound by tag so the same STUN hosts keep
		// whatever routing the user's own WebRTC traffic would have had.
		if len(udpRelayProbeDomains) > 0 {
			rules = append(rules, SBRouteRule{
				Action:   "route",
				Inbound:  []string{probeInboundTag},
				Network:  []string{"udp"},
				Domain:   append([]string(nil), udpRelayProbeDomains...),
				Outbound: "proxy",
			})
		}
		// The throughput probe has the same exposure as the UDP one: in Smart
		// mode Final=direct would send its download out of the local uplink
		// and record the user's own broadband as the node's speed. Scoped to
		// the probe inbound so a user visiting the same host gets whatever
		// routing they would have had.
		if len(throughputProbeDomains) > 0 {
			rules = append(rules, SBRouteRule{
				Action:   "route",
				Inbound:  []string{probeInboundTag},
				Domain:   append([]string(nil), throughputProbeDomains...),
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

	// Built-in Smart coverage for apps the block-lists structurally cannot
	// reach (see smartTunneledApps). Deliberately placed AFTER the
	// app-whitelist direct rule: a user who puts Discord in
	// "Приложения-исключения" means it, and their exclusion keeps winning.
	// Placed BEFORE the blocked-domain/CIDR rules so the process decision is
	// taken once for the whole app instead of being split between a tunnelled
	// signalling half and a direct media half — that split is the bug.
	if len(builtinAppRegexes) > 0 {
		// Same reasoning as the force-VPN list: sending an app wholesale
		// through the tunnel must not hand back the QUIC hang. Safe for voice
		// — Discord media runs on 19294-19335 and 50003-50008, never 443.
		rules = append(rules, quicRejectRule(SBRouteRule{ProcessPathRegex: builtinAppRegexes}))
		rules = append(rules, SBRouteRule{
			Action:           "route",
			ProcessPathRegex: builtinAppRegexes,
			Outbound:         "proxy",
		})
	}

	// Browser DoH, under its own sub-toggle. A browser with Secure DNS on never
	// asks the system resolver, so FakeIP never sees the name and the
	// connection arrives at the router as a bare address. The race still works
	// on it, but what it learns is filed under an address — and a CDN rotates
	// addresses, so the knowledge is weaker, ages faster and does not
	// generalise to the domain. Rejecting the endpoint makes the browser fall
	// back to the system resolver, where FakeIP can name it.
	//
	// Placed here deliberately: AFTER the user's own routing lists, app
	// exclusions and force-VPN rules, so anything the user said explicitly
	// still wins; BEFORE the block-list, because a DoH endpoint that happens to
	// be on the list would otherwise be routed to the node and keep working,
	// which is exactly the outcome this rule exists to prevent.
	//
	// Only with the adaptive engine on: without FakeIP the rule costs the user
	// their DoH and buys nothing.
	if adaptiveSmartActive(cfg) && cfg.AdaptiveSmartBlockBrowserDoH {
		rules = append(rules, SBRouteRule{
			Action:       "reject",
			Method:       "default",
			DomainSuffix: browserDoHDomains(),
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

	for _, w := range whitelistSuffixes(cfg.Whitelist) {
		outbound := "proxy"
		if w.direct {
			outbound = "direct"
		}
		rules = append(rules, SBRouteRule{
			Action:       "route",
			DomainSuffix: []string{w.suffix},
			Outbound:     outbound,
		})
	}

	// Smart-mode QUIC backstop. Everything above classifies UDP/443 by the
	// sniffed SNI; this catches what the sniffer could not name, which in Smart
	// mode would otherwise fall through to Final="direct" and leave the tunnel.
	//
	// That is not hypothetical. Captured on the user's Ethernet adapter
	// (2026-09-02) while Smart was connected: HTTP/3 flows to claude.ai,
	// a-api.anthropic.com, a-cdn.anthropic.com, www.anthropic.com,
	// assets.claude.ai, a.claude.ai and claude.com all left direct, from the
	// real address, while the same hosts over TCP went through the node. All
	// seven are in the block-list, so the rules above should have caught them.
	// Replaying the captured client Initials through sing-box's own sniffer
	// shows why they did not:
	//
	//	packet 1  DCID 528f6c7b…  -> "need more data: length check 2 failed"
	//	packet 2  DCID 01063525…  -> "cipher: message authentication failed"
	//
	// Chrome's ClientHello spans more than one Initial datagram. The first is
	// forwarded while the sniffer waits for the rest; the server answers it and
	// hands the client its own connection ID, so the client's next Initial
	// carries the SERVER's DCID. sing-box re-derives the Initial keys from each
	// packet's own DCID, but RFC 9000 keys the whole Initial space off the
	// ORIGINAL one — so the AEAD check fails. That is a hard error rather than
	// ErrNeedMoreData, so the sniff loop gives up and the connection is routed
	// with no domain at all.
	//
	// Rejecting here rather than routing to "proxy" for the same reason the
	// per-domain rule above rejects: UDP through the node is unreliable, and a
	// rejected QUIC attempt makes the client fall back to TCP at once, where
	// the domain rules do work. Placed last on purpose — every earlier rule,
	// including the user's own app exclusions and whitelist, still wins, so
	// this only ever fires on traffic that had no classification to lose. The
	// cost is HTTP/3 for direct destinations, which drop to TCP; Global mode
	// already loses h3 the same way (Final="proxy" sends it into the node's
	// dead UDP path), so this only brings Smart in line.
	//
	// The blanket form is a consequence of not knowing the name: the sniffer
	// cannot pull SNI out of Chrome's multi-packet QUIC ClientHello, so an
	// unnamed HTTP/3 request could not be classified and had to be knocked back
	// onto TCP. FakeIP names the connection before any rule runs, so with the
	// adaptive engine on the smart outbound decides UDP the same way it decides
	// TCP and only refuses what it genuinely does not know yet. The targeted
	// rejects above stay in both cases: those are about UDP being unreliable
	// through the node, which FakeIP does not change.
	if cfg.RoutingMode == ModeSmart && !adaptiveSmartActive(cfg) {
		rules = append(rules, quicRejectRule(SBRouteRule{}))
	}

	route.Rules = rules
	return route
}

type whitelistSuffix struct {
	suffix string
	direct bool
}

// whitelistSuffixes orders the exclusion list deepest-first and marks each
// suffix direct on an odd number of matches, so a nested entry ("avito.ru"
// under ".ru") flips its parent back to the tunnel.
func whitelistSuffixes(whitelist []string) []whitelistSuffix {
	seen := make(map[string]struct{}, len(whitelist))
	var normalized []string
	for _, w := range whitelist {
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
	if len(normalized) == 0 {
		return nil
	}

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

	out := make([]whitelistSuffix, 0, len(ordered))
	for _, suffix := range ordered {
		matchCount := 0
		for _, rule := range normalized {
			if suffix == rule || strings.HasSuffix(suffix, "."+rule) {
				matchCount++
			}
		}
		out = append(out, whitelistSuffix{suffix: suffix, direct: matchCount%2 == 1})
	}
	return out
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

func PingProxyUDP(ip string, port int, wait time.Duration) (latencyMs int64, reachable bool, reason string) {
	addr := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("udp", addr, 3*time.Second)
	if err != nil {
		return 0, false, pingReasonFromError(err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(wait))
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
// returns -1ms → shown as "—"). Both steps together stay inside budget.
func PingWireGuard(ip string, port int, budget time.Duration) (latencyMs int64, reachable bool, reason string) {
	icmpWait, udpWait := wireGuardProbeBudgets(budget)
	if ms, ok := pingICMPProbe(ip, "", icmpWait); ok {
		return ms, true, ""
	}
	return PingProxyUDP(ip, port, udpWait)
}

// wireGuardProbeDefaultBudget is for callers without a deadline of their own:
// it splits into the 2 s ICMP + 1 s UDP these probes always used.
const wireGuardProbeDefaultBudget = 3*time.Second + pingBudgetMargin

// wireGuardProbeBudgets splits one budget between the ICMP echo and the UDP
// fallback that runs after it.
func wireGuardProbeBudgets(total time.Duration) (icmp, udp time.Duration) {
	udp = total / 3
	if udp > time.Second {
		udp = time.Second
	}
	icmp = total - udp - pingBudgetMargin
	if icmp < 0 {
		icmp = 0
	}
	return icmp, udp
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
