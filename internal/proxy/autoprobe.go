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

// autoProbeResolveHost resolves one hostname to a single IPv4/IPv6 literal.
// Declared as a var so tests can substitute it, matching the pingTCPProbe
// pattern.
var autoProbeResolveHost = func(ctx context.Context, host string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, autoProbeResolveTimeout)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(addrs) == 0 {
		return "", false
	}
	return addrs[0].IP.String(), true
}

// resolveProbeHosts resolves each distinct hostname in targets exactly once and
// returns hostname -> literal. Hostnames that fail to resolve are absent from
// the map; callers fall back to the name.
//
// Providers routinely publish four or five ports on one host, so a sweep of 48
// nodes covers about a dozen names. Resolving per probe did that lookup work
// dozens of times over, and — worse — folded a random slice of resolver latency
// into each node's measured RTT, so four ports of one host came back as
// 292/289/293/311ms instead of one comparable figure.
func resolveProbeHosts(ctx context.Context, targets []config.ProxyEntry) map[string]string {
	hosts := make(map[string]struct{}, len(targets))
	for _, t := range targets {
		h := strings.TrimSpace(t.IP)
		if h == "" || net.ParseIP(h) != nil {
			continue
		}
		hosts[h] = struct{}{}
	}
	if len(hosts) == 0 {
		return nil
	}

	out := make(map[string]string, len(hosts))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for h := range hosts {
		wg.Add(1)
		go func(host string) {
			defer wg.Done()
			ip, ok := autoProbeResolveHost(ctx, host)
			if !ok {
				return
			}
			mu.Lock()
			out[host] = ip
			mu.Unlock()
		}(h)
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

// ProbeAutoNodes probes every dialable node in parallel and returns one result
// per probed node, in input order. Non-dialable entries are dropped.
func ProbeAutoNodes(ctx context.Context, nodes []config.ProxyEntry, depth AutoProbeDepth) []AutoProbeResult {
	targets := make([]config.ProxyEntry, 0, len(nodes))
	for _, n := range nodes {
		if isProbeableNode(n) {
			targets = append(targets, n)
		}
	}
	if len(targets) == 0 {
		return nil
	}

	resolved := resolveProbeHosts(ctx, targets)

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
			if ip, ok := resolved[dialHost]; ok {
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

// autoProbeDialer returns a dialer pinned to the physical adapter when one is
// available. Every probe in this file must go out that way: the sweep routinely
// runs while a TUN is up (switching to AUTO without pausing first), and the
// sing-tun system stack completes the local TCP handshake before it even
// attempts the upstream dial. An unpinned probe therefore measures the local
// stack — a few milliseconds — and reports a server we cannot reach as the
// fastest node in the group. pickLANBindIPv4 already excludes tunnel
// interfaces and the engine's own 172.19.0.0/30, so the address it hands back
// is physical by construction.
func autoProbeDialer(timeout time.Duration) *net.Dialer {
	d := &net.Dialer{Timeout: timeout}
	if local, err := pickLANBindIPv4(); err == nil && local != nil {
		d.LocalAddr = &net.TCPAddr{IP: local, Port: 0}
	}
	return d
}

// probeTransport runs the transport-level probe for one node, mirroring the
// protocol dispatch in Manager.Ping — including its choice of LAN-bound probe
// variants, which the auto path previously skipped. See autoProbeDialer for
// why the binding is what makes this measurement mean anything.
func probeTransport(e config.ProxyEntry, dialHost string) (rtt int64, ok bool, stage, reason string) {
	lan := autoProbeBindsToLAN()
	switch strings.ToUpper(strings.TrimSpace(e.Type)) {
	case "HYSTERIA2":
		if lan {
			rtt, ok, reason, stage = pingHysteria2StrictLANProbe(dialHost, e.Port)
		} else {
			rtt, ok, reason, stage = pingHysteria2StrictProbe(dialHost, e.Port)
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
	dialer := autoProbeDialer(4 * time.Second)
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

	samples := make([]int64, 0, autoProbeSamples)
	for i := 0; i < autoProbeSamples; i++ {
		lat, tlsOK, tlsReason := autoTLSProbe(dialHost, e.Port, sni, alpn)
		if !tlsOK {
			// A live TCP handshake with a dead TLS handshake is the DPI
			// signature a bare SYN probe cannot see. Treat it as a failure.
			return AutoProbeResult{Key: res.Key, OK: false, Stage: "tls", Reason: tlsReason}
		}
		samples = append(samples, lat)
	}

	res.RTTms, res.JitterMs = medianAndJitter(samples)
	res.Stage = "tls"
	return res
}
