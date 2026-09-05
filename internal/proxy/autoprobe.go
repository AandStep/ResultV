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
	"crypto/sha1"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"resultproxy-wails/internal/config"
)

// autoProbeMaxConcurrency caps in-flight probes. Probes are network-bound and
// spend nearly all their time waiting on timeouts, so the pool should cover the
// whole group in one wave: at the previous value of 16 a 48-node subscription
// took three waves and paid the full timeout in each. The cap only exists so a
// several-hundred-node subscription does not open several hundred sockets at
// once.
const autoProbeMaxConcurrency = 64

// autoProbeResolveTimeout bounds one hostname lookup during a sweep. Past this
// the node is probed by name and the dialer's own resolver gets a turn — losing
// a node to a single slow lookup would be worse than an unmeasured one.
const autoProbeResolveTimeout = 3 * time.Second

// autoProbeLookupIPAddr is the raw DNS lookup autoProbeResolveHost wraps.
// Declared as a var (separately from autoProbeResolveHost itself) so tests
// can hand it synthetic, deterministic answers — an AAAA-first list, a
// v6-only list, etc. — and exercise autoProbeResolveHost's real IPv4-picking
// and no-A-record-fails-closed logic below, instead of only ever replacing
// that logic wholesale the way the sweep-level tests replace
// autoProbeResolveHost.
var autoProbeLookupIPAddr = net.DefaultResolver.LookupIPAddr

// autoProbeResolveHost resolves one hostname to a single IPv4 literal.
// Declared as a var so tests can substitute it, matching the pingTCPProbe
// pattern.
//
// IPv4 only, deliberately: every probe below this (pingTCPProbe/PingProxy's
// address building, pingLANProbe/autoProbeDialer's IPv4 bind hint, and
// WireGuard's pingICMPHost forcing "ip4") is IPv4-only by construction, so an
// AAAA-first answer from the OS resolver (RFC 6724 sorts IPv6 first on many
// Windows machines) would hand every probe below a literal it cannot dial.
// This mirrors resolvePinnedServerIP (manager.go) and resolveAllServerIPs'
// "IPv4 only — the tunnel DNS strategy is ipv4_only" rule.
//
// Unlike resolvePinnedServerIP, this does NOT fall back to addrs[0] when
// there is no A record at all: returning ("", false) here makes the caller
// probe by hostname instead, where the dialer's own resolver can pick a
// family that actually matches the bind. Falling back to a v6 literal would
// instead guarantee failure on a stack that cannot dial it. That is the one
// intentional divergence from the neighbouring helper, not an oversight.
var autoProbeResolveHost = func(ctx context.Context, host string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, autoProbeResolveTimeout)
	defer cancel()
	addrs, err := autoProbeLookupIPAddr(ctx, host)
	if err != nil {
		return "", false
	}
	return pickIPv4(addrs)
}

// pickIPv4 returns the first IPv4 literal in a resolver answer, or ("", false)
// when the answer carries none. Split out of autoProbeResolveHost as a pure
// function so the selection rule itself — the part with the actual policy in
// it, documented above — can be tested without substituting a global.
func pickIPv4(addrs []net.IPAddr) (string, bool) {
	for _, a := range addrs {
		if v4 := a.IP.To4(); v4 != nil {
			return v4.String(), true
		}
	}
	return "", false
}

// probeHostKey normalizes an entry's address into the key resolveProbeHosts
// stores under and probeAutoNodesResolved looks up by. Hostnames are
// case-insensitive (RFC 4343), so "CDN1.example.test" and "cdn1.example.test"
// in one subscription must collapse to a single lookup instead of defeating
// resolveProbeHosts' own "each host exactly once" guarantee. AutoNodeKey
// lowercases the host for the same reason.
func probeHostKey(host string) string {
	return strings.ToLower(strings.TrimSpace(host))
}

// resolveProbeHosts resolves each distinct hostname in targets exactly once and
// returns lowercased hostname -> literal. Hostnames that fail to resolve are
// absent from the map; callers fall back to the name.
//
// Providers routinely publish four or five ports on one host, so a sweep of 48
// nodes covers about a dozen names. Resolving per probe did that lookup work
// dozens of times over, and — worse — folded a random slice of resolver latency
// into each node's measured RTT, so four ports of one host came back as
// 292/289/293/311ms instead of one comparable figure.
func resolveProbeHosts(ctx context.Context, targets []config.ProxyEntry) map[string]string {
	hosts := make([]string, 0, len(targets))
	seen := make(map[string]struct{}, len(targets))
	for _, t := range targets {
		h := probeHostKey(t.IP)
		if h == "" || net.ParseIP(h) != nil {
			continue
		}
		if _, dup := seen[h]; dup {
			continue
		}
		seen[h] = struct{}{}
		hosts = append(hosts, h)
	}
	if len(hosts) == 0 {
		return nil
	}

	out := make(map[string]string, len(hosts))
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Bounded by the same constant as the probe pool, and for the same reason:
	// a subscription with several hundred distinct hosts would otherwise start
	// several hundred lookups at once, and on Windows every getaddrinfo blocks
	// an OS thread for the duration. In practice nodes outnumber hosts several
	// times over, so this rarely binds — but "rarely" is not "never".
	var next int
	worker := func() {
		defer wg.Done()
		for {
			mu.Lock()
			if next >= len(hosts) {
				mu.Unlock()
				return
			}
			host := hosts[next]
			next++
			mu.Unlock()

			ip, ok := autoProbeResolveHost(ctx, host)
			if !ok {
				continue
			}
			mu.Lock()
			out[host] = ip
			mu.Unlock()
		}
	}

	pool := autoProbeMaxConcurrency
	if pool > len(hosts) {
		pool = len(hosts)
	}
	wg.Add(pool)
	for i := 0; i < pool; i++ {
		go worker()
	}
	wg.Wait()
	return out
}

type AutoProbeDepth int

const (
	// DepthFast probes the transport only: TCP connect, QUIC handshake for
	// Hysteria2, UDP for WireGuard. One sample. Used to sweep the whole group.
	DepthFast AutoProbeDepth = iota
	// DepthFull adds a TLS handshake with the node's real SNI and ALPN and
	// takes three samples. Used on the shortlist only.
	DepthFull
)

type AutoProbeResult struct {
	Key      string
	RTTms    int64
	JitterMs int64
	OK       bool
	Stage    string
	Reason   string
}

// AutoNodeKey identifies a node across subscription refreshes.
//
// ProxyEntry.ID is useless as a persistent key: the backend reassigns it on
// every subscription fetch (app.go). Subscription plus address is stable — but
// it is not unique. CDN-fronted and multi-account panels routinely issue
// several logically distinct nodes on one host:port:type, and the frontend
// already had to work around exactly that collision with namespace-suffixed
// keys (frontend/src/utils/proxyParser.js:657). So the credential and transport
// parameters that actually distinguish those nodes are folded in as well.
//
// Consequence worth stating: when a provider genuinely reconfigures a node its
// key changes and its history restarts. For reliability statistics that is the
// safe direction — better to re-learn a changed node than to credit it with a
// different node's record.
//
// Fields are length-prefixed rather than joined on a bare separator, so a
// separator occurring inside a field cannot make two different tuples collide.
func AutoNodeKey(e config.ProxyEntry) string {
	h := sha1.New()
	write := func(s string) { fmt.Fprintf(h, "%d:%s", len(s), s) }

	// Only host and protocol are case-insensitive. URL paths are case-sensitive
	// per RFC 3986, so the subscription URL is trimmed but never lowercased.
	write(strings.TrimSpace(e.SubscriptionURL))
	write(strings.ToLower(strings.TrimSpace(e.IP)))
	write(strconv.Itoa(e.Port))
	write(strings.ToUpper(strings.TrimSpace(e.Type)))
	write(e.Username)
	write(e.Password)
	write(canonicalizeExtra(e.Extra)) // uuid, sni, path, serviceName — the real identity
	return hex.EncodeToString(h.Sum(nil))
}

// AutoNodeKeyOf derives a node's AutoNodeKey from the ProxyConfig that Connect
// actually received, rather than from the config.ProxyEntry it came from.
//
// The two are the same node, but by the time an AUTO member reaches Connect its
// ID and Name have been overwritten with the group head's (see
// ResolveAutoCandidates) — so anything keyed off the ID at that point is keyed
// off the GROUP, not the member. That is how every UDP-relay verdict ended up
// unusable: startUDPRelayProbe recorded under resolveProxyID, so all 34
// verdicts in a real node_stats.json sat under config ids, none of them
// matching any key the scorer reads, and an AUTO group's members all collapsed
// onto the head's id. AutoNodeKey hashes only fields ProxyConfig also carries,
// so deriving the key here costs nothing and cannot drift from the probe's.
func AutoNodeKeyOf(p ProxyConfig) string {
	return AutoNodeKey(config.ProxyEntry{
		SubscriptionURL: p.SubscriptionURL,
		IP:              p.IP,
		Port:            p.Port,
		Type:            p.Type,
		Username:        p.Username,
		Password:        p.Password,
		Extra:           p.Extra,
	})
}

// canonicalizeExtra normalizes Extra's JSON encoding before it is hashed.
//
// Extra reaches the backend through two different encoders that disagree on
// escaping: Go's encoding/json HTML-escapes &, <, > by default, while the
// frontend's JSON.stringify (used on every settings round-trip — see
// updateSetting, fired as early as the first connect via
// lastSelectedProxyId) does not. Hashing the raw bytes would therefore give
// the SAME logical node two different keys depending on which encoder wrote
// it last — e.g. a ws/xhttp path containing "?ed=2048&v=1" — breaking
// hysteresis and restarting its history.
//
// Unmarshal-then-remarshal fixes this: both encodings decode to the
// identical Go value, and re-encoding through encoding/json is deterministic
// (sorted map keys, fixed escaping) regardless of which form was fed in, so
// the two forms collapse to the same canonical bytes.
func canonicalizeExtra(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		// Malformed JSON: fall back to the raw bytes. A parse failure should
		// still yield a stable (if imperfect) key, not a panic or every
		// malformed node silently collapsing onto the empty-Extra key.
		return string(raw)
	}
	canon, err := json.Marshal(v)
	if err != nil {
		return string(raw)
	}
	return string(canon)
}

// isProbeableNode reports whether an entry has something to dial. SECTION rows
// are subscription labels whose IP/Port were blanked by normalizeSectionEntry;
// probing one dials ":0".
func isProbeableNode(e config.ProxyEntry) bool {
	if strings.EqualFold(strings.TrimSpace(e.Type), "SECTION") {
		return false
	}
	return strings.TrimSpace(e.IP) != "" && e.Port > 0
}

// CountProbeableNodes reports how many of these entries a sweep would actually
// dial. Exported because the caller's log line ("опрошено N узлов") is the one
// place where the difference matters: a group carrying SECTION labels or
// address-less rows would otherwise report the raw member count as the number
// of nodes probed, which is exactly the kind of inflated figure that line was
// added to rule out.
func CountProbeableNodes(nodes []config.ProxyEntry) int {
	n := 0
	for _, e := range nodes {
		if isProbeableNode(e) {
			n++
		}
	}
	return n
}

// ProbeAutoNodes probes every dialable node in parallel and returns one result
// per probed node, in input order. Non-dialable entries are dropped.
func ProbeAutoNodes(ctx context.Context, nodes []config.ProxyEntry, depth AutoProbeDepth) []AutoProbeResult {
	return probeAutoNodesResolved(ctx, nodes, depth, nil)
}

// probeAutoNodesResolved is ProbeAutoNodes with the hostname->literal map
// supplied by the caller. resolved may be nil, in which case this resolves the
// targets itself.
//
// The seam exists for RankAutoCandidates, which probes twice (the wide phase 1
// and the shortlist phase 2). Resolving inside each call did the lookups twice
// and — worse — let a round-robin DNS answer hand phase 2 a different backend
// than phase 1 measured, so the median TLS figure could belong to a machine
// other than the one DepthFast ranked.
func probeAutoNodesResolved(ctx context.Context, nodes []config.ProxyEntry, depth AutoProbeDepth, resolved map[string]string) []AutoProbeResult {
	targets := make([]config.ProxyEntry, 0, len(nodes))
	for _, n := range nodes {
		if isProbeableNode(n) {
			targets = append(targets, n)
		}
	}
	if len(targets) == 0 {
		return nil
	}

	if resolved == nil {
		resolved = resolveProbeHosts(ctx, targets)
	}

	out := make([]AutoProbeResult, len(targets))
	var next int
	var mu sync.Mutex
	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for {
			mu.Lock()
			if next >= len(targets) {
				mu.Unlock()
				return
			}
			i := next
			next++
			mu.Unlock()

			if ctx.Err() != nil {
				return
			}
			dialHost := strings.TrimSpace(targets[i].IP)
			if ip, ok := resolved[probeHostKey(dialHost)]; ok {
				dialHost = ip
			}
			out[i] = probeOne(targets[i], depth, dialHost)
		}
	}

	pool := autoProbeMaxConcurrency
	if pool > len(targets) {
		pool = len(targets)
	}
	wg.Add(pool)
	for i := 0; i < pool; i++ {
		go worker()
	}
	wg.Wait()

	return out
}

// autoProbeBindsToLAN reports whether probes can be pinned to a physical
// adapter. Declared as a var purely so the two branches stay reachable from
// tests; production always goes through pickLANBindIPv4.
var autoProbeBindsToLAN = func() bool {
	ip, err := pickLANBindIPv4()
	return err == nil && ip != nil
}

// isLoopbackProbeHost reports whether a probe target lives on this machine's
// loopback, in which case the LAN binding below must be skipped: Windows
// rejects a connect to 127.0.0.1 from a non-loopback source address outright
// ("connectex: The requested address is not valid in its context"), so binding
// would turn a reachable local node into an unreachable one. Skipping costs
// nothing — the binding exists to escape the TUN's default route, and loopback
// never travels through the TUN.
//
// Both spellings are handled because a host reaches this function either as
// the entry's literal address or as whatever resolveProbeHosts resolved it to.
func isLoopbackProbeHost(host string) bool {
	host = strings.TrimSpace(host)
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// autoProbeDialer returns a dialer pinned to the physical adapter when one is
// available and the target is not on loopback. Every probe in this file must go
// out that way: the sweep routinely runs while a TUN is up (switching to AUTO
// without pausing first), and the sing-tun system stack completes the local TCP
// handshake before it even attempts the upstream dial. An unpinned probe
// therefore measures the local stack — a few milliseconds — and reports a
// server we cannot reach as the fastest node in the group. pickLANBindIPv4
// already excludes tunnel interfaces and the engine's own 172.19.0.0/30, so the
// address it hands back is physical by construction.
func autoProbeDialer(timeout time.Duration, host string) *net.Dialer {
	d := &net.Dialer{Timeout: timeout}
	if isLoopbackProbeHost(host) {
		return d
	}
	if local, err := pickLANBindIPv4(); err == nil && local != nil {
		d.LocalAddr = &net.TCPAddr{IP: local, Port: 0}
	}
	return d
}

// probeTransport runs the transport-level probe for one node, mirroring the
// protocol dispatch in Manager.Ping — and, like it, picking the LAN-bound
// variant of each probe, which the auto path previously skipped. See
// autoProbeDialer for why the binding is what makes this measurement mean
// anything.
//
// Only the dispatch is mirrored, not the condition that selects the bound
// variant: Manager.Ping binds when the tunnel is actually up
// (connected && mode == ProxyModeTunnel), while a sweep has no session to ask
// about and binds whenever a physical adapter is available at all. Binding
// with no tunnel up costs nothing — the packet leaves the same adapter either
// way — so the sweep uses the weaker, always-available condition on purpose.
func probeTransport(e config.ProxyEntry, dialHost string) (rtt int64, ok bool, stage, reason string) {
	// See isLoopbackProbeHost: binding a loopback target to the physical
	// adapter makes the connect fail outright, so a local node would rank as
	// dead purely because of how we measured it.
	lan := autoProbeBindsToLAN() && !isLoopbackProbeHost(dialHost)
	switch strings.ToUpper(strings.TrimSpace(e.Type)) {
	case "HYSTERIA2":
		sni := autoProbeHysteria2SNI(e)
		if lan {
			rtt, ok, reason, stage = pingHysteria2StrictLANProbe(dialHost, e.Port, sni)
		} else {
			rtt, ok, reason, stage = pingHysteria2StrictProbe(dialHost, e.Port, sni)
		}
		return rtt, ok, stage, reason
	case "WIREGUARD", "AMNEZIAWG":
		if lan {
			rtt, ok, reason = pingWireGuardLANProbe(dialHost, e.Port)
		} else {
			rtt, ok, reason = pingWireGuardProbe(dialHost, e.Port)
		}
		return rtt, ok, "udp", reason
	default:
		if lan {
			rtt, ok, reason = pingLANProbe(dialHost, e.Port)
		} else {
			rtt, ok, reason = pingTCPProbe(dialHost, e.Port)
		}
		return rtt, ok, "tcp", reason
	}
}

// autoProbeSamples is the number of TLS handshakes averaged in DepthFull.
// One sample is noisy — a single slow handshake could be a GC pause on the
// node, not the path. Three gives a median that shrugs off one outlier plus
// a jitter figure (max-min) that a single sample cannot produce at all.
const autoProbeSamples = 3

// autoTLSProbe performs a plain TLS handshake against host:port with the given
// SNI and ALPN. Declared as a var so tests can substitute it, matching the
// pingTCPProbe pattern in manager.go.
//
// For Reality nodes this deliberately does NOT perform Reality authentication:
// an unauthenticated ClientHello is forwarded by the server to its camouflage
// site and completes as an ordinary handshake. That is exactly the signal we
// want — the path is alive and the SNI is not being cut. The probe sends no
// payload, so certificate identity is irrelevant and is not verified: under
// Reality the certificate belongs to the camouflage site, and plenty of nodes
// present a certificate that does not match the dialed IP.
var autoTLSProbe = func(host string, port int, sni string, alpn []string) (int64, bool, string) {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	dialer := autoProbeDialer(4*time.Second, host)
	start := time.Now()
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{
		ServerName:         sni,
		NextProtos:         alpn,
		InsecureSkipVerify: true, // probe only; see comment above
		MinVersion:         tls.VersionTLS12,
	})
	elapsed := time.Since(start)
	if err != nil {
		return 0, false, pingReasonFromError(err)
	}
	_ = conn.Close()
	return elapsed.Milliseconds(), true, ""
}

// autoProbeWantsTLS mirrors applyTLSAndTransport's TLS-enable condition
// (outbound.go:428-500), the engine's own rule for whether a node's outbound
// gets a TLS record at all: security must be "tls" or "reality", a Reality
// public key ("pbk") must be present, or the legacy tls:true flag must be
// set — and that legacy flag is itself vetoed by an explicit
// security:"none" (extraSecurityExplicitlyNone, outbound.go). This must not
// grow its own approximation of "looks like it wants TLS": parseVLESSURI
// defaults security to "none" and parseVMessURI writes no security key at
// all unless tls=="tls" (uriparser.go), so anything looser than the engine's
// own condition fires a ClientHello at a node that never speaks TLS — which
// is exactly the bug this function exists to fix (probe times out, node gets
// deleted from AUTO selection and penalised as if it had actually failed).
func autoProbeWantsTLS(extra map[string]interface{}) bool {
	security := getStringField(extra, "security", "none")
	pbk := getStringField(extra, "pbk", "")
	if pbk != "" && !strings.EqualFold(security, "reality") {
		security = "reality" // mirrors applyTLSAndTransport's own auto-detect
	}
	switch strings.ToLower(security) {
	case "reality", "tls":
		return true
	}
	if extraSecurityExplicitlyNone(extra) {
		return false
	}
	return getBoolField(extra, "tls")
}

// autoProbeTLSParams derives the SNI and ALPN to present. Protocols that never
// wrap in TLS report wantTLS=false so the stage is skipped for them.
func autoProbeTLSParams(e config.ProxyEntry) (sni string, alpn []string, wantTLS bool) {
	protoType := strings.ToUpper(strings.TrimSpace(e.Type))
	switch protoType {
	case "SS", "SHADOWSOCKS", "WIREGUARD", "AMNEZIAWG", "HYSTERIA2":
		return "", nil, false
	}

	extra := map[string]interface{}{}
	if len(e.Extra) > 0 {
		_ = json.Unmarshal(e.Extra, &extra)
	}

	wantTLS = autoProbeWantsTLS(extra)
	// TROJAN's outbound builder (outbound.go's buildProxyOutbound, TROJAN
	// case) forces TLS on by default even with no security field at all —
	// unlike VLESS/VMess, Trojan is not a meaningful protocol without TLS —
	// and only an explicit security=none turns it off. Mirror that
	// protocol-specific override here too, or every Trojan node with a bare
	// Extra (the common case) would silently skip the stage that is its
	// whole point.
	if !wantTLS && protoType == "TROJAN" && !extraSecurityExplicitlyNone(extra) {
		wantTLS = true
	}
	if !wantTLS {
		return "", nil, false
	}

	sni = getStringField(extra, "sni", "")
	if sni == "" {
		sni = getStringField(extra, "serverName", "")
	}
	if sni == "" {
		sni = strings.TrimSpace(e.IP)
	}

	if raw := getStringField(extra, "alpn", ""); raw != "" {
		for _, p := range strings.Split(raw, ",") {
			if p = strings.TrimSpace(p); p != "" {
				alpn = append(alpn, p)
			}
		}
	}

	// A bare IP as ServerName is fine to hand to crypto/tls as-is: per the
	// stdlib doc on tls.Config.ServerName, an IP-shaped value is simply left
	// out of the ClientHello's SNI extension while the handshake proceeds
	// normally. So there is nothing to special-case here — we still get the
	// "TLS actually completes behind this live socket" signal, just without
	// an SNI to test for domain-based blocking on nodes that have no
	// CDN/domain front.
	return sni, alpn, true
}

// autoProbeHysteria2SNI derives the ClientHello ServerName the QUIC probe
// should present for a HYSTERIA2 node. HYSTERIA2 is excluded from
// autoProbeTLSParams (it never runs the DepthFull TLS stage — QUIC's own
// handshake in probeTransport is its only TLS-bearing check), so it needs its
// own small mirror of the field-lookup rule instead of sharing that helper.
//
// This mirrors buildProxyOutboundRaw's HYSTERIA2 case (outbound.go): "sni",
// then "server_name" (snake_case — this protocol's Extra convention differs
// from autoProbeTLSParams' "serverName" for the other protocols), falling
// back to the entry's own address. The fallback deliberately ends at e.IP,
// not at the sweep's resolved dial literal: an IP-shaped ServerName makes
// crypto/tls omit the SNI extension entirely (tls.Config.ServerName's own
// doc), and probing with no SNI over a hostname's presence there is a lie
// about what the real outbound sends.
func autoProbeHysteria2SNI(e config.ProxyEntry) string {
	extra := map[string]interface{}{}
	if len(e.Extra) > 0 {
		_ = json.Unmarshal(e.Extra, &extra)
	}
	sni := getStringField(extra, "sni", getStringField(extra, "server_name", ""))
	if sni == "" {
		sni = strings.TrimSpace(e.IP)
	}
	return sni
}

// medianAndJitter reduces repeated-sample latencies to one figure each. The
// median (not mean) resists a single stalled sample skewing the result, and
// jitter as max-min surfaces path instability that a single sample can never
// reveal, independent of the median's own smoothing.
func medianAndJitter(v []int64) (median, jitter int64) {
	if len(v) == 0 {
		return 0, 0
	}
	s := append([]int64(nil), v...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[len(s)/2], s[len(s)-1] - s[0]
}

// probeOne probes a single node. dialHost is what the probe actually connects
// to — the sweep's pre-resolved literal when the entry's address is a hostname,
// the address itself otherwise. e stays the source of truth for identity (the
// key hashes e.IP) and for TLS parameters (the SNI must remain the hostname, or
// the TLS stage stops seeing the SNI-based blocking it exists to detect).
func probeOne(e config.ProxyEntry, depth AutoProbeDepth, dialHost string) AutoProbeResult {
	res := AutoProbeResult{Key: AutoNodeKey(e)}

	rtt, ok, stage, reason := probeTransport(e, dialHost)
	res.RTTms, res.OK, res.Stage, res.Reason = rtt, ok, stage, reason
	if !ok || depth == DepthFast {
		return res
	}

	sni, alpn, wantTLS := autoProbeTLSParams(e)
	if !wantTLS {
		return res
	}

	// A live TCP handshake with a dead TLS handshake is the DPI signature a
	// bare SYN probe cannot see, so a failing TLS stage does fail the node —
	// but only when the MAJORITY of samples fail, not the first one.
	//
	// Failing on the first loss was measurably wrong: these three handshakes
	// go out back-to-back at one node, and losing one of them on a loaded
	// uplink is ordinary. On 2026-09-05 that cost the user the entire group —
	// phase 1 found 23 of 30 members alive, phase 2 killed all five
	// shortlisted leaders on a single dropped handshake each, and AUTO
	// reported "ни один из 30 узлов не доступен". Taking a median of three
	// samples only makes sense if one outlier cannot decide the verdict.
	//
	// The loop stops as soon as the majority is out of reach (two losses), so
	// a genuinely blocked node still costs at most two timeouts, not three.
	samples := make([]int64, 0, autoProbeSamples)
	var lost int
	for i := 0; i < autoProbeSamples; i++ {
		lat, tlsOK, tlsReason := autoTLSProbe(dialHost, e.Port, sni, alpn)
		if !tlsOK {
			lost++
			if lost > autoProbeSamples/2 {
				return AutoProbeResult{Key: res.Key, OK: false, Stage: "tls", Reason: tlsReason}
			}
			continue
		}
		samples = append(samples, lat)
	}

	res.RTTms, res.JitterMs = medianAndJitter(samples)
	res.Stage = "tls"
	return res
}
