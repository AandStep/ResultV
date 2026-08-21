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
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"resultproxy-wails/internal/system"
)

func parseExtra(proxy ProxyConfig) map[string]interface{} {
	var extra map[string]interface{}
	if proxy.Extra != nil {
		json.Unmarshal(proxy.Extra, &extra)
	}
	if extra == nil {
		extra = make(map[string]interface{})
	}
	return extra
}

func getStringField(extra map[string]interface{}, key, defaultVal string) string {
	if val, ok := extra[key].(string); ok && val != "" {
		return val
	}
	return defaultVal
}

func getBoolField(extra map[string]interface{}, key string) bool {
	if val, ok := extra[key].(bool); ok {
		return val
	}
	return false
}

func getIntField(extra map[string]interface{}, key string, defaultVal int) int {
	switch v := extra[key].(type) {
	case int:
		return v
	case float64:
		return int(v)
	case int64:
		return int(v)
	}
	return defaultVal
}

// resolvePacketEncoding returns the UDP packet-encoding mode for VLESS/VMess.
// Defaults to "xudp" for full UDP-over-TCP support (matches Xray's xudp). The
// user may override via the "packet_encoding" or "packetEncoding" extra field;
// "none" / empty disables UDP encoding entirely.
func resolvePacketEncoding(extra map[string]interface{}) string {
	v := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		getStringField(extra, "packet_encoding", ""),
		getStringField(extra, "packetEncoding", ""),
	)))
	switch v {
	case "":
		return "xudp"
	case "none", "off", "disable", "disabled":
		return ""
	default:
		return v
	}
}

// vlessEncryptionValidModes / vlessEncryptionValidRTT mirror the two enums
// parseClientEncryption (protocol/vless/outbound.go) requires as segments 2
// and 3 of the handshake string. Any other value there returns an error that
// aborts NewOutbound — i.e. the engine never starts.
var vlessEncryptionValidModes = map[string]bool{"native": true, "xorpub": true, "random": true}
var vlessEncryptionValidRTT = map[string]bool{"0rtt": true, "1rtt": true}

// vlessEncryptionFromExtra passes through a VLESS Encryption handshake string
// only when it has the shape parseClientEncryption (protocol/vless/outbound.go)
// requires: prefix "mlkem768x25519plus", a known xor mode, a known RTT tag,
// and at least one further segment that base64.RawURLEncoding-decodes to a
// 32- or 1184-byte key. The core parses this string while building the
// outbound and aborts the whole engine start on anything malformed — a
// prefix check alone (e.g. a truncated copy-paste) is not enough. Segments
// between the RTT tag and the key are padding and are not otherwise
// validated, matching the core's own tolerance for them.
func vlessEncryptionFromExtra(extra map[string]interface{}) string {
	s := strings.TrimSpace(getStringField(extra, "encryption", ""))
	parts := strings.Split(s, ".")
	if len(parts) < 4 || parts[0] != "mlkem768x25519plus" {
		return ""
	}
	if !vlessEncryptionValidModes[parts[1]] || !vlessEncryptionValidRTT[parts[2]] {
		return ""
	}
	hasKey := false
	for _, seg := range parts[3:] {
		if vlessEncryptionSegmentIsKey(seg) {
			hasKey = true
			break
		}
	}
	if !hasKey {
		return ""
	}
	return s
}

// fingerprintFromExtra returns the uTLS fingerprint, accepting Xray's
// "fp" as well as Clash-style "client-fingerprint" / "clientFingerprint".
func fingerprintFromExtra(extra map[string]interface{}) string {
	return firstNonEmpty(
		getStringField(extra, "fp", ""),
		getStringField(extra, "client-fingerprint", ""),
		getStringField(extra, "clientFingerprint", ""),
		getStringField(extra, "client_fingerprint", ""),
	)
}

// resolvedFingerprint returns the uTLS fingerprint that should actually be
// used for an outbound. Precedence: explicit user value (fp / client-fingerprint
// / ...) → system-derived default (Edge WebView2 version on Windows, Safari on
// macOS/Linux). Pass "none" / "off" / "disable" / "-" in the URI to keep TLS
// without uTLS — useful for debugging when you want the bare Go fingerprint.
// knownUTLSFingerprints is the set sing-box's uTLS layer accepts. An unknown
// value crashes instance creation with "unknown uTLS fingerprint", so anything
// outside this set (a typo or a clipped paste like "ch") is coerced to "chrome".
var knownUTLSFingerprints = map[string]bool{
	"chrome": true, "firefox": true, "edge": true, "safari": true,
	"ios": true, "android": true, "360": true, "qq": true,
	"random": true, "randomized": true,
}

func resolvedFingerprint(extra map[string]interface{}) string {
	raw := strings.TrimSpace(strings.ToLower(fingerprintFromExtra(extra)))
	switch raw {
	case "none", "off", "disable", "disabled", "-":
		return ""
	case "":
		return system.WebViewFingerprint()
	default:
		// Unknown/typo'd fingerprint would abort engine start; fall back to a
		// valid one instead of taking the whole connection down. "randomized*"
		// variants (e.g. randomizedalpn) are accepted by sing-box, so allow the prefix.
		if knownUTLSFingerprints[raw] || strings.HasPrefix(raw, "random") {
			return raw
		}
		return "chrome"
	}
}

// applyTLSExtras applies optional TLS knobs (min_version, max_version,
// cipher_suites) onto an already-built SBOutboundTLS.
func applyTLSExtras(tls *SBOutboundTLS, extra map[string]interface{}) {
	if tls == nil {
		return
	}
	tls.MinVersion = firstNonEmpty(
		getStringField(extra, "min_version", ""),
		getStringField(extra, "minVersion", ""),
		getStringField(extra, "tls-min-version", ""),
	)
	tls.MaxVersion = firstNonEmpty(
		getStringField(extra, "max_version", ""),
		getStringField(extra, "maxVersion", ""),
		getStringField(extra, "tls-max-version", ""),
	)
	if cs := firstNonEmpty(
		getStringField(extra, "cipher_suites", ""),
		getStringField(extra, "cipherSuites", ""),
	); cs != "" {
		var out []string
		for _, p := range strings.FieldsFunc(cs, func(r rune) bool {
			return r == ',' || r == ':' || r == ';' || r == '\n' || r == '|'
		}) {
			if s := strings.TrimSpace(p); s != "" {
				out = append(out, s)
			}
		}
		if len(out) > 0 {
			tls.CipherSuites = out
		}
	}
}

// normalizeRealityShortID trims whitespace and lowercases the hex portion
// of a REALITY short_id. Xray accepts both upper- and lowercase hex;
// sing-box validates hex strictly so we lowercase to be safe. Empty or
// non-hex input passes through unchanged.
func normalizeRealityShortID(sid string) string {
	s := strings.TrimSpace(sid)
	if s == "" {
		return s
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return s
		}
	}
	return strings.ToLower(s)
}

func extraSecurityExplicitlyNone(extra map[string]interface{}) bool {
	if extra == nil {
		return false
	}
	v, ok := extra["security"]
	if !ok {
		return false
	}
	s, ok := v.(string)
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(s), "none")
}

// ssKnownPlugins is the set of SIP003 plugins the core actually registers
// (transport/sip003), plus the aliases links use for them. An unregistered name
// fails outbound creation, and that aborts the engine start for every node — so
// an unknown plugin is dropped here and the node simply goes unobfuscated.
var ssKnownPlugins = map[string]string{
	"obfs-local":   "obfs-local",
	"simple-obfs":  "obfs-local",
	"obfs":         "obfs-local",
	"v2ray-plugin": "v2ray-plugin",
}

func ssPluginFromExtra(extra map[string]interface{}) (string, string) {
	name := ssKnownPlugins[strings.ToLower(strings.TrimSpace(getStringField(extra, "plugin", "")))]
	if name == "" {
		return "", ""
	}
	return name, firstNonEmpty(
		getStringField(extra, "plugin_opts", ""),
		getStringField(extra, "pluginOpts", ""),
	)
}

// multiplexFromExtra honours an explicit sing-box multiplex profile and nothing
// else. Xray's own "mux" ({enabled, concurrency}) is deliberately NOT mapped
// here: it is a different protocol on the wire from smux/yamux/h2mux, so
// translating it would make a working node speak something its server does not
// understand — worse than leaving it off.
var multiplexProtocols = map[string]bool{"smux": true, "yamux": true, "h2mux": true}

func multiplexFromExtra(extra map[string]interface{}) *SBMultiplex {
	raw, ok := extra["multiplex"].(map[string]interface{})
	if !ok {
		return nil
	}
	enabled, _ := raw["enabled"].(bool)
	if !enabled {
		return nil
	}
	protocol := strings.ToLower(strings.TrimSpace(stringFromExtraValue(raw["protocol"])))
	if !multiplexProtocols[protocol] {
		return nil
	}
	padding, _ := raw["padding"].(bool)
	return &SBMultiplex{
		Enabled:        true,
		Protocol:       protocol,
		MaxConnections: intFromAny(raw["max_connections"]),
		MinStreams:     intFromAny(raw["min_streams"]),
		MaxStreams:     intFromAny(raw["max_streams"]),
		Padding:        padding,
	}
}

func buildProxyOutbound(proxy ProxyConfig) SBOutbound {
	// The outbound keeps the server's original host (domain or literal IP). When
	// it is a domain, sing-box re-resolves it against the static `hosts` DNS
	// record seeded with every connect-time backend IP (see buildDNS +
	// ProxyConfig.ResolvedIPs) — instant, local, and rotating across the CDN's
	// backends — so a single backend reset no longer collapses the whole session.
	// Swapping Server to one pinned IP here was the regression: it nailed the
	// session to that IP and stopped all failover. Re-resolution never touches the
	// redirected OS resolver, so the false-kill-switch fragility stays fixed.
	return buildProxyOutboundRaw(proxy)
}

func buildProxyOutboundRaw(proxy ProxyConfig) SBOutbound {
	extra := parseExtra(proxy)

	switch proxy.Type {
	case "HYSTERIA2", "hysteria2":
		out := SBOutbound{
			Type:       "hysteria2",
			Tag:        "proxy",
			Server:     proxy.IP,
			ServerPort: proxy.Port,
			Password: firstNonEmpty(
				getStringField(extra, "password", ""),
				getStringField(extra, "auth", ""),
				getStringField(extra, "auth_str", ""),
				getStringField(extra, "userpass", ""),
				proxy.Password,
			),
			UpMbps:      intFromExtra(extra, "up_mbps", "upMbps"),
			DownMbps:    intFromExtra(extra, "down_mbps", "downMbps"),
			ServerPorts: hysteriaServerPortsFromExtra(extra),
			HopInterval: hysteria2HopIntervalFromExtra(extra),
		}
		if sni := getStringField(extra, "sni", getStringField(extra, "server_name", "")); sni != "" || getBoolField(extra, "insecure") {
			out.TLS = &SBOutboundTLS{
				Enabled:    true,
				ServerName: firstNonEmpty(sni, proxy.IP),
				Insecure:   getBoolField(extra, "insecure"),
			}
			if alpnStr := getStringField(extra, "alpn", ""); alpnStr != "" {
				out.TLS.ALPN = splitALPN(alpnStr)
			}
		} else {
			out.TLS = &SBOutboundTLS{Enabled: true}
			if alpnStr := getStringField(extra, "alpn", ""); alpnStr != "" {
				out.TLS.ALPN = splitALPN(alpnStr)
			}
			out.TLS.ServerName = firstNonEmpty(
				getStringField(extra, "server_name", ""),
				getStringField(extra, "sni", ""),
				proxy.IP,
			)
			out.TLS.Insecure = getBoolField(extra, "insecure")
		}
		if out.TLS != nil && len(out.TLS.ALPN) == 0 {
			out.TLS.ALPN = []string{"h3", "hysteria"}
		}
		if obfsType := getStringField(extra, "obfs_type", getStringField(extra, "obfsType", "")); obfsType != "" {
			out.Obfs = &SBHysteria2Obfs{
				Type:     obfsType,
				Password: getStringField(extra, "obfs_password", getStringField(extra, "obfsPassword", "")),
			}
		}
		return out

	case "SOCKS5", "SOCKS", "socks", "socks5":
		return SBOutbound{
			Type:       "socks",
			Tag:        "proxy",
			Server:     proxy.IP,
			ServerPort: proxy.Port,
			Username:   proxy.Username,
			Password:   proxy.Password,
			Version:    "5",
		}

	case "SS", "shadowsocks", "ss":
		method := getStringField(extra, "method", "aes-256-gcm")
		plugin, pluginOpts := ssPluginFromExtra(extra)
		return SBOutbound{
			Type:          "shadowsocks",
			Tag:           "proxy",
			Server:        proxy.IP,
			ServerPort:    proxy.Port,
			Password:      proxy.Password,
			Method:        method,
			Plugin:        plugin,
			PluginOptions: pluginOpts,
			Multiplex:     multiplexFromExtra(extra),
		}

	case "VMESS", "vmess":

		uuid := getStringField(extra, "uuid", "")
		alterId := getIntField(extra, "alterId", 0)
		// VMess cipher: Xray uses "scy", sing-box uses "security".
		// Default "auto" matches Xray's behavior.
		cipher := firstNonEmpty(
			getStringField(extra, "security_cipher", ""),
			getStringField(extra, "scy", ""),
			getStringField(extra, "encryption", ""),
		)
		// Don't confuse with TLS-layer "security" (none/tls/reality) — that's
		// in extra["security"] but indicates transport security, not cipher.
		out := SBOutbound{
			Type:       "vmess",
			Tag:        "proxy",
			Server:     proxy.IP,
			ServerPort: proxy.Port,
			UUID:       uuid,
			AlterId:    alterId,
			Security:   cipher,
			PacketEncoding:      resolvePacketEncoding(extra),
			GlobalPadding:       getBoolField(extra, "global_padding") || getBoolField(extra, "globalPadding"),
			AuthenticatedLength: getBoolField(extra, "authenticated_length") || getBoolField(extra, "authenticatedLength"),
			Multiplex:           multiplexFromExtra(extra),
		}
		applyTLSAndTransport(&out, extra, proxy.IP)
		return out

	case "VLESS", "vless":
		uuid := getStringField(extra, "uuid", "")
		flow := getStringField(extra, "flow", "")
		out := SBOutbound{
			Type:       "vless",
			Tag:        "proxy",
			Server:     proxy.IP,
			ServerPort: proxy.Port,
			UUID:       uuid,
			Flow:       flow,
			Encryption:     vlessEncryptionFromExtra(extra),
			PacketEncoding: resolvePacketEncoding(extra),
			Multiplex:      multiplexFromExtra(extra),
		}
		applyTLSAndTransport(&out, extra, proxy.IP)
		return out

	case "TROJAN", "trojan":
		out := SBOutbound{
			Type:       "trojan",
			Tag:        "proxy",
			Server:     proxy.IP,
			ServerPort: proxy.Port,
			Password:   proxy.Password,
			Multiplex:  multiplexFromExtra(extra),
		}
		trojanSNI := firstNonEmpty(
			getStringField(extra, "sni", ""),
			getStringField(extra, "serverName", ""),
			getStringField(extra, "servername", ""),
			getStringField(extra, "server_name", ""),
			getStringField(extra, "peer", ""),
			getStringField(extra, "host", ""),
			proxy.IP,
		)

		applyTLSAndTransport(&out, extra, trojanSNI)

		if out.TLS == nil && !extraSecurityExplicitlyNone(extra) {
			out.TLS = &SBOutboundTLS{
				Enabled:    true,
				ServerName: trojanSNI,
				Insecure:   getBoolField(extra, "insecure"),
			}
			if fp := getStringField(extra, "fp", ""); fp != "" {
				out.TLS.UTLS = &SBUTLS{Enabled: true, Fingerprint: fp}
			}
			if alpnStr := getStringField(extra, "alpn", ""); alpnStr != "" {
				out.TLS.ALPN = xhttpPreferH2ALPN(splitALPN(alpnStr), getBoolField(extra, "insecure_h3_override"))
			} else {
				network := getStringField(extra, "network", "tcp")
				out.TLS.ALPN = defaultALPNForNetwork(network)
			}
		}
		return out

	case "NAIVEPROXY", "NAIVE", "naive":
		// sing-box naive outbound (with_naive_outbound) uses Cronet for TLS; TLS
		// in JSON is only enabled + server_name (see sing-box naive NewOutbound).
		sni := firstNonEmpty(
			getStringField(extra, "sni", ""),
			getStringField(extra, "server_name", ""),
			proxy.IP,
		)
		return SBOutbound{
			Type:       "naive",
			Tag:        "proxy",
			Server:     proxy.IP,
			ServerPort: proxy.Port,
			Username:   proxy.Username,
			Password:   proxy.Password,
			TLS: &SBOutboundTLS{
				Enabled:    true,
				ServerName: sni,
			},
		}

	default:
		return SBOutbound{
			Type:       "http",
			Tag:        "proxy",
			Server:     proxy.IP,
			ServerPort: proxy.Port,
			Username:   proxy.Username,
			Password:   proxy.Password,
		}
	}
}

func intFromExtra(extra map[string]interface{}, snake, camel string) int {
	if extra == nil {
		return 0
	}
	if v, ok := extra[snake]; ok {
		return intFromAny(v)
	}
	// Some fields (e.g. "mtu", "tti") have no distinct camelCase spelling — the
	// caller passes "" for camel rather than repeating the snake_case key, and
	// an empty key must not be looked up as if it were a real one.
	if camel == "" {
		return 0
	}
	if v, ok := extra[camel]; ok {
		return intFromAny(v)
	}
	return 0
}

func intFromAny(v interface{}) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return int(i)
		}
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0
		}
		if n, err := json.Number(s).Int64(); err == nil {
			return int(n)
		}
	}
	return 0
}

// hysteriaServerPortsFromExtra converts the URI spelling of Hysteria2 port
// hopping ("mport=10000-20000,30000") into the "start:end" ranges sing-quic
// parses. A range it cannot parse is dropped rather than forwarded: ParsePorts
// returns an error there, and that error aborts the engine start.
func hysteriaServerPortsFromExtra(extra map[string]interface{}) []string {
	var out []string
	for _, part := range hysteriaPortSpecs(extra) {
		lo, hi, ok := parsePortRangeSpec(part)
		if !ok {
			continue
		}
		out = append(out, strconv.Itoa(lo)+":"+strconv.Itoa(hi))
	}
	return out
}

// hysteriaPortSpecs returns the raw, comma-split port-range specs to feed
// parsePortRangeSpec. server_ports is accepted both as a plain string
// ("10000-20000,30000", the URI spelling) and as a []interface{} of
// strings/numbers — the latter is server_ports' natural shape in a
// sing-box-style JSON config, and a producer emitting it that way must not
// have the value silently swallowed by a string-only reader (getStringField
// returns "" for anything that isn't already a string).
func hysteriaPortSpecs(extra map[string]interface{}) []string {
	if raw, ok := extra["server_ports"]; ok {
		switch v := raw.(type) {
		case []interface{}:
			specs := make([]string, 0, len(v))
			for _, item := range v {
				if s := stringFromExtraValue(item); s != "" {
					specs = append(specs, s)
				}
			}
			if len(specs) > 0 {
				return specs
			}
		case string:
			if v != "" {
				return strings.Split(v, ",")
			}
		}
	}
	raw := firstNonEmpty(getStringField(extra, "mport", ""), getStringField(extra, "ports", ""))
	return strings.Split(raw, ",")
}

// hysteria2HopIntervalFromExtra canonicalizes hop_interval into the string
// shape the core's badoption.Duration (option/hysteria2.go) accepts. Hy2 links
// historically spell the value as a bare number of seconds
// ("hop-interval=30"); badoption.Duration's UnmarshalJSON requires a Go
// duration string with a unit and errors "missing unit in duration" on a bare
// number, which aborts the engine start for every node, not just this one.
// A value that is neither a valid duration nor a positive bare number is
// dropped so the core falls back to its own default instead of refusing to
// start.
func hysteria2HopIntervalFromExtra(extra map[string]interface{}) string {
	raw := strings.TrimSpace(firstNonEmpty(
		getStringField(extra, "hop_interval", ""),
		getStringField(extra, "hopInterval", ""),
	))
	if raw == "" {
		return ""
	}
	if _, err := time.ParseDuration(raw); err == nil {
		return raw
	}
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		return strconv.Itoa(n) + "s"
	}
	return ""
}

func parsePortRangeSpec(s string) (int, int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, false
	}
	sep := strings.IndexAny(s, "-:")
	if sep < 0 {
		p, err := strconv.Atoi(s)
		if err != nil || p < 1 || p > 65535 {
			return 0, 0, false
		}
		return p, p, true
	}
	lo, err1 := strconv.Atoi(strings.TrimSpace(s[:sep]))
	hi, err2 := strconv.Atoi(strings.TrimSpace(s[sep+1:]))
	if err1 != nil || err2 != nil || lo < 1 || hi > 65535 || lo > hi {
		return 0, 0, false
	}
	return lo, hi, true
}

func applyTLSAndTransport(out *SBOutbound, extra map[string]interface{}, defaultSNI string) {
	security := getStringField(extra, "security", "none")
	pbk := getStringField(extra, "pbk", getStringField(extra, "publicKey", getStringField(extra, "public_key", "")))

	if pbk != "" && security != "reality" {
		security = "reality" // Auto-detect reality if public key is present
	}

	switch security {
	case "reality":
		sni := getStringField(extra, "sni", defaultSNI)
		fp := resolvedFingerprint(extra)
		if fp == "" {
			// Reality requires a uTLS fingerprint to function — the handshake
			// is itself the masquerade. Fall back to "chrome" if the system
			// default came back empty (explicit "none" override).
			fp = "chrome"
		}
		if pbk == "" {
			pbk = getStringField(extra, "pbk", getStringField(extra, "publicKey", getStringField(extra, "public_key", "")))
		}
		sid := normalizeRealityShortID(getStringField(extra, "sid", ""))
		tlsObj := &SBOutboundTLS{
			Enabled:    true,
			ServerName: sni,
			UTLS:       &SBUTLS{Enabled: true, Fingerprint: fp},
		}
		if pbk != "" {
			tlsObj.Reality = &SBReality{Enabled: true, PublicKey: pbk, ShortID: sid}
		}
		out.TLS = tlsObj
		if alpnStr := getStringField(extra, "alpn", ""); alpnStr != "" {
			out.TLS.ALPN = xhttpPreferH2ALPN(splitALPN(alpnStr), getBoolField(extra, "insecure_h3_override"))
		}
		applyTLSExtras(out.TLS, extra)
	case "tls":
		sni := getStringField(extra, "sni", defaultSNI)
		insecure := getBoolField(extra, "insecure")
		fp := resolvedFingerprint(extra)
		tls := &SBOutboundTLS{
			Enabled:    true,
			ServerName: sni,
			Insecure:   insecure,
		}
		if fp != "" {
			tls.UTLS = &SBUTLS{Enabled: true, Fingerprint: fp}
		}
		if alpnStr := getStringField(extra, "alpn", ""); alpnStr != "" {
			tls.ALPN = xhttpPreferH2ALPN(splitALPN(alpnStr), getBoolField(extra, "insecure_h3_override"))
		}
		out.TLS = tls
		applyTLSExtras(out.TLS, extra)
	default:
		if extraSecurityExplicitlyNone(extra) {
			break
		}
		if getBoolField(extra, "tls") {
			sni := getStringField(extra, "sni", defaultSNI)
			insecure := getBoolField(extra, "insecure")
			out.TLS = &SBOutboundTLS{
				Enabled:    true,
				ServerName: sni,
				Insecure:   insecure,
			}
			if fp := resolvedFingerprint(extra); fp != "" {
				out.TLS.UTLS = &SBUTLS{Enabled: true, Fingerprint: fp}
			}
			if alpnStr := getStringField(extra, "alpn", ""); alpnStr != "" {
				out.TLS.ALPN = xhttpPreferH2ALPN(splitALPN(alpnStr), getBoolField(extra, "insecure_h3_override"))
			}
			applyTLSExtras(out.TLS, extra)
		}
	}

	applyTransportOnly(out, extra)
	if out.TLS != nil && out.Transport != nil && (out.Transport.Type == "xhttp" || out.Transport.Type == "splithttp") {
		// xhttp and splithttp REQUIRE h2 ALPN to function properly, even for Reality.
		if len(out.TLS.ALPN) == 0 {
			out.TLS.ALPN = []string{"h2", "http/1.1"}
		} else {
			out.TLS.ALPN = xhttpPreferH2ALPN(out.TLS.ALPN, false)
		}
	} else if out.TLS != nil && len(out.TLS.ALPN) == 0 {
		isReality := out.TLS.Reality != nil && out.TLS.Reality.Enabled
		if !isReality {
			network := getStringField(extra, "network", "tcp")
			// Для plain TCP (VLESS/VMESS без транспорта) оставляем ALPN пустым —
			// sing-box использует дефолт, что совместимо с любой серверной конфигурацией.
			// Для HTTP-транспортов (grpc, xhttp, ws, h2) ALPN важен для фреймирования.
			if network != "tcp" && network != "" {
				out.TLS.ALPN = defaultALPNForNetwork(network)
			}
		}
	}
}

func applyTransportOnly(out *SBOutbound, extra map[string]interface{}) {
	// Case-insensitive: the mKCP gate in parseVMessURI is case-insensitive
	// ("net":"KCP" already stores seed/headerType into extra), but this switch
	// used to compare against lowercase literals only, so an upper/mixed-case
	// network here built no transport at all even though the knobs were there.
	network := strings.ToLower(strings.TrimSpace(getStringField(extra, "network", "tcp")))
	switch network {
	case "ws", "websocket":
		path := getStringField(extra, "ws-path", "")
		if path == "" {
			path = getStringField(extra, "path", "/")
		}
		host := getStringField(extra, "ws-host", "")
		if host == "" {
			host = getStringField(extra, "host", "")
		}
		// Early data: Xray-style ed=<bytes> in URI, also early_data_header_name.
		// If the path contains "?ed=N", split it; sing-box wants the bytes count
		// in max_early_data and the header name separately.
		maxEarly := getIntField(extra, "max_early_data", 0)
		if maxEarly == 0 {
			maxEarly = getIntField(extra, "maxEarlyData", 0)
		}
		edHeader := firstNonEmpty(
			getStringField(extra, "early_data_header_name", ""),
			getStringField(extra, "earlyDataHeaderName", ""),
			getStringField(extra, "edHeader", ""),
		)
		if maxEarly == 0 {
			if i := strings.Index(path, "?ed="); i >= 0 {
				rest := path[i+4:]
				end := len(rest)
				if amp := strings.IndexAny(rest, "&"); amp >= 0 {
					end = amp
				}
				if n, err := strconv.Atoi(rest[:end]); err == nil && n > 0 {
					maxEarly = n
					path = path[:i]
					if edHeader == "" {
						edHeader = "Sec-WebSocket-Protocol"
					}
				}
			}
		}
		if maxEarly > 0 && edHeader == "" {
			edHeader = "Sec-WebSocket-Protocol"
		}
		// sing-box's websocket transport has no "host" field: V2RayWebsocketOptions
		// is Path/Headers/MaxEarlyData/EarlyDataHeaderName only. Emitting one made
		// the config unparsable ("unknown field host") and that kills the whole
		// instance, not just this outbound. The core lifts headers["Host"] into the
		// request URL (v2raywebsocket/client.go), which is where the host belongs.
		headers := transportHeadersFromExtra(extra)
		if host != "" {
			if headers == nil {
				headers = make(map[string]string, 1)
			}
			headers["Host"] = host
		}
		out.Transport = &SBOutboundTransport{
			Type:                "ws",
			Path:                path,
			Headers:             headers,
			MaxEarlyData:        maxEarly,
			EarlyDataHeaderName: edHeader,
		}
	case "httpupgrade":
		path := getStringField(extra, "path", "/")
		host := getStringField(extra, "host", "")
		out.Transport = &SBOutboundTransport{
			Type:    "httpupgrade",
			Path:    path,
			Host:    host,
			Headers: transportHeadersFromExtra(extra),
		}
	case "grpc":
		serviceName := getStringField(extra, "grpc-service-name", "")
		if serviceName == "" {
			serviceName = getStringField(extra, "serviceName", "")
		}
		if serviceName == "" {
			serviceName = getStringField(extra, "service_name", "")
		}
		// Translate Xray's serviceName semantics into an explicit gRPC path.
		// sing-box misreads an old-school name that carries an inner slash
		// ("host-name/hash", the scheme several providers use), which breaks
		// every connection on such a node. Only correct with the full gRPC
		// transport — see xrayGRPCServiceName.
		if grpcFullTransport {
			serviceName = xrayGRPCServiceName(serviceName)
		}
		// Default idle/ping timeouts when the config omits them. sing-box's gRPC
		// client only arms HTTP/2 keepalive PINGs when idle_timeout > 0 (see
		// v2raygrpc/client.go WithKeepaliveParams); with both unset a half-open
		// session — the peer/ISP silently dropped the underlying TCP but sent no
		// RST/FIN — is never detected and the transport wedges forever, taking the
		// whole tunnel down until a manual reconnect. 15s idle / 15s ping ack is the
		// long-standing v2ray/xray-recommended floor: cheap on the wire, well under
		// any server "too_many_pings" GOAWAY threshold. User-supplied values win.
		idleTimeout := firstNonEmpty(
			getStringField(extra, "idle_timeout", ""),
			getStringField(extra, "idleTimeout", ""),
			"15s",
		)
		pingTimeout := firstNonEmpty(
			getStringField(extra, "ping_timeout", ""),
			getStringField(extra, "pingTimeout", ""),
			"15s",
		)
		permit := getBoolField(extra, "permit_without_stream") || getBoolField(extra, "permitWithoutStream")
		out.Transport = &SBOutboundTransport{
			Type:                "grpc",
			ServiceName:         serviceName,
			IdleTimeout:         idleTimeout,
			PingTimeout:         pingTimeout,
			PermitWithoutStream: permit,
		}
	case "http", "h2":
		// "http-host"/"http-path"/"http-method" are not keys any parser in this
		// project ever writes into extra — the real fields links and JSON
		// subscriptions carry are "host"/"path"/"method". Falling back to those
		// keeps an h2 node's host (and path/method) from silently going missing.
		out.Transport = &SBOutboundTransport{
			Type:    "http",
			Host:    firstNonEmpty(getStringField(extra, "http-host", ""), getStringField(extra, "host", "")),
			Path:    firstNonEmpty(getStringField(extra, "http-path", ""), getStringField(extra, "path", "/")),
			Method:  firstNonEmpty(getStringField(extra, "http-method", ""), getStringField(extra, "method", "")),
			Headers: transportHeadersFromExtra(extra),
		}
	case "kcp", "mkcp":
		// All six numeric mKCP knobs are uint32 in the core (option/v2ray_transport.go
		// V2RayKCPOptions): a negative value fails to decode ("cannot unmarshal number
		// -5 into ... uint32"), which aborts the engine start for every node, not just
		// this one. positiveIntFromExtra drops anything not strictly positive so the
		// core's own default applies instead.
		out.Transport = &SBOutboundTransport{
			Type:             "mkcp",
			Seed:             getStringField(extra, "seed", ""),
			HeaderType:       mkcpHeaderTypeFromExtra(extra),
			MTU:              positiveIntFromExtra(extra, "mtu", ""),
			TTI:              positiveIntFromExtra(extra, "tti", ""),
			UplinkCapacity:   positiveIntFromExtra(extra, "uplink_capacity", "uplinkCapacity"),
			DownlinkCapacity: positiveIntFromExtra(extra, "downlink_capacity", "downlinkCapacity"),
			Congestion:       getBoolField(extra, "congestion"),
			ReadBufferSize:   positiveIntFromExtra(extra, "read_buffer_size", "readBufferSize"),
			WriteBufferSize:  positiveIntFromExtra(extra, "write_buffer_size", "writeBufferSize"),
		}
	case "tcp":
		// Xray's "tcp" + headerType=http obfuscation maps onto sing-box's
		// "http" transport. Without HTTP headers we leave Transport nil so
		// sing-box uses raw TCP — matching the historical default.
		header, _ := extra["header"].(map[string]interface{})
		ht := ""
		if header != nil {
			ht = strings.ToLower(stringFromExtraValue(header["type"]))
		}
		if ht == "http" {
			path := "/"
			host := ""
			method := ""
			if req, ok := header["request"].(map[string]interface{}); ok {
				method = stringFromExtraValue(req["method"])
				if pl, ok := req["path"].([]interface{}); ok && len(pl) > 0 {
					path = stringFromExtraValue(pl[0])
				} else if ps := stringFromExtraValue(req["path"]); ps != "" {
					path = ps
				}
				if hdrs, ok := req["headers"].(map[string]interface{}); ok {
					switch hv := hdrs["Host"].(type) {
					case []interface{}:
						if len(hv) > 0 {
							host = stringFromExtraValue(hv[0])
						}
					case string:
						host = strings.TrimSpace(hv)
					}
				}
			}
			out.Transport = &SBOutboundTransport{
				Type:   "http",
				Path:   path,
				Host:   host,
				Method: method,
			}
		}
	case "xhttp", "splithttp":
		host := getStringField(extra, "host", "")
		xPadding := xhttpPaddingFromExtra(extra)
		mode := xhttpModeFromExtra(extra)
		uplink := xhttpUplinkHTTPMethodFromExtra(extra, mode)
		headers := transportHeadersFromExtra(extra)
		xmuxRaw := xmuxJSONFromExtra(extra)
		out.Transport = &SBOutboundTransport{
			Type:                 "xhttp",
			Path:                 getStringField(extra, "path", "/"),
			Host:                 host,
			Mode:                 mode,
			UplinkHTTPMethod:     uplink,
			XPaddingBytes:        xPadding,
			Headers:              headers,
			NoGRPCHeader:         boolPtrFromExtra(extra, "noGRPCHeader", "no_grpc_header"),
			NoSSEHeader:          boolPtrFromExtra(extra, "noSSEHeader", "no_sse_header"),
			ScMaxEachPostBytes:   rangeRawFromExtra(extra, "scMaxEachPostBytes", "sc_max_each_post_bytes"),
			ScMinPostsIntervalMs: rangeRawFromExtra(extra, "scMinPostsIntervalMs", "sc_min_posts_interval_ms"),
			ScStreamUpServerSecs: rangeRawFromExtra(extra, "scStreamUpServerSecs", "sc_stream_up_server_secs"),
			Xmux:                 xmuxRaw,
			XPaddingObfsMode:     boolPtrFromExtra(extra, "xPaddingObfsMode", "x_padding_obfs_mode"),
			XPaddingKey: firstNonEmpty(
				getStringField(extra, "x_padding_key", ""),
				getStringField(extra, "xPaddingKey", ""),
			),
			XPaddingHeader: firstNonEmpty(
				getStringField(extra, "x_padding_header", ""),
				getStringField(extra, "xPaddingHeader", ""),
			),
			XPaddingPlacement: xhttpPaddingPlacementFromExtra(extra),
			XPaddingMethod:    xhttpPaddingMethodFromExtra(extra),

			SessionPlacement:     xhttpSessionPlacementFromExtra(extra, "session_placement", "sessionPlacement"),
			SessionKey:           firstNonEmpty(getStringField(extra, "session_key", ""), getStringField(extra, "sessionKey", "")),
			SeqPlacement:         xhttpSessionPlacementFromExtra(extra, "seq_placement", "seqPlacement"),
			SeqKey:               firstNonEmpty(getStringField(extra, "seq_key", ""), getStringField(extra, "seqKey", "")),
			UplinkDataPlacement:  xhttpUplinkDataPlacementFromExtra(extra, mode),
			UplinkDataKey:        firstNonEmpty(getStringField(extra, "uplink_data_key", ""), getStringField(extra, "uplinkDataKey", "")),
			SessionIDTable:       firstNonEmpty(getStringField(extra, "session_id_table", ""), getStringField(extra, "sessionIdTable", "")),
			SessionIDLength:      rangeRawFromExtra(extra, "sessionIdLength", "session_id_length"),
			UplinkChunkSize:      rangeRawFromExtra(extra, "uplinkChunkSize", "uplink_chunk_size"),
			CongestionController: xhttpCongestionFromExtra(extra),
			CWND:                 positiveIntFromExtra(extra, "cwnd", "cwnd"),
			ScMaxBufferedPosts:   int64(positiveIntFromExtra(extra, "sc_max_buffered_posts", "scMaxBufferedPosts")),
		}
	}
}

// xmuxConservativeDefaults caps connection lifetime and reuse for the v2rayxhttp
// transport in sing-box. Without these, the transport keeps every physical
// HTTP/2 sub-connection alive for the entire session and never recycles streams,
// so per-stream write buffers (http2.clientStream.writeRequestBody) accumulate
// under sustained short-lived-connection load (typical browser pattern: many
// tabs hitting many domains over hours). Pprof on a real session showed
// ~22 MB stuck in those buffers after two hours of browsing, with ~90 live
// v2rayxhttp goroutines and visible browser-side latency that disappeared
// instantly when the app was closed.
//
// The values below are conservative — they force a TLS handshake roughly
// every 5 minutes or every 100 requests on a given sub-conn, whichever
// comes first. The handshake cost is invisible against typical browser
// load. max_concurrency=32 caps the number of multiplexed streams per
// sub-connection (XTLS docs recommend 16-32 for stability).
//
// User-supplied xmux fields ALWAYS override these defaults. The defaults
// only fill in keys the user did not specify.
var xmuxConservativeDefaults = map[string]interface{}{
	"max_concurrency":     32,
	"c_max_reuse_times":   100,
	"h_max_reusable_secs": 300,
}

// xmuxKnownKeys is the exact field set of V2RayXHTTPXmuxOptions. A node's xmux
// object is user input: anything outside this set would reach the core as an
// unknown field, and the config decoder runs with DisallowUnknownFields — the
// engine would refuse to start for every node, not just this one.
var xmuxKnownKeys = map[string]bool{
	"max_concurrency":     true,
	"max_connections":     true,
	"c_max_reuse_times":   true,
	"h_max_request_times": true,
	"h_max_reusable_secs": true,
	"h_keep_alive_period": true,
}

func xmuxJSONFromExtra(extra map[string]interface{}) json.RawMessage {
	rename := map[string]string{
		"maxConcurrency":   "max_concurrency",
		"maxConnections":   "max_connections",
		"cMaxReuseTimes":   "c_max_reuse_times",
		"hMaxRequestTimes": "h_max_request_times",
		"hMaxReusableSecs": "h_max_reusable_secs",
		"hKeepAlivePeriod": "h_keep_alive_period",
	}

	user := make(map[string]interface{}, 8)
	if v, ok := extra["xmux"]; ok && v != nil {
		if m, ok := v.(map[string]interface{}); ok {
			for k, val := range m {
				if nk, ok := rename[k]; ok {
					k = nk
				}
				if !xmuxKnownKeys[k] {
					continue
				}
				// The known-keys filter above only protects against unknown
				// FIELDS; the VALUES still reach the core's decoder untouched.
				// max_concurrency etc. are badoption.Range[int] and reject a
				// bool or non-numeric string ("cannot unmarshal ..."), while
				// h_keep_alive_period is a plain int64. Either failure aborts
				// the whole engine, so an invalid value is dropped along with
				// its key — our conservative default (if any) stays in place.
				if k == "h_keep_alive_period" {
					if !isPositiveNumberValue(val) {
						continue
					}
				} else if !isPositiveRangeValue(val) {
					continue
				}
				user[k] = val
			}
		}
	}

	out := make(map[string]interface{}, len(xmuxConservativeDefaults)+len(user))
	for k, val := range xmuxConservativeDefaults {
		out[k] = val
	}
	for k, val := range user {
		out[k] = val
	}

	// The core rejects a config that carries both knobs ("max_connections cannot
	// be specified together with max_concurrency") and rejection means no engine
	// at all. A node asking for max_connections wins — over our default, and over
	// a max_concurrency the same node contradicts itself with.
	if _, ok := out["max_connections"]; ok {
		delete(out, "max_concurrency")
	}

	raw, err := json.Marshal(out)
	if err != nil || len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return raw
}

// isPositiveRangeString reports whether s is either a bare positive integer or
// an "a-b" range where both bounds are positive integers and a <= b — the two
// shapes badoption.Range[T] (common/json/badoption/range.go) actually parses.
// Anything else fails to decode ("invalid range" or a strconv error), and that
// aborts the engine start for every node.
func isPositiveRangeString(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n > 0
	}
	idx := strings.Index(s, "-")
	if idx <= 0 || idx == len(s)-1 {
		return false
	}
	lo, errLo := strconv.ParseInt(s[:idx], 10, 64)
	hi, errHi := strconv.ParseInt(s[idx+1:], 10, 64)
	if errLo != nil || errHi != nil {
		return false
	}
	return lo > 0 && hi > 0 && lo <= hi
}

// isPositiveRangeValue is isPositiveRangeString for a value straight out of an
// extra map — a JSON number decodes to float64, so that shape is checked
// separately instead of stringifying it first.
func isPositiveRangeValue(v interface{}) bool {
	switch t := v.(type) {
	case float64:
		return t > 0 && t == float64(int64(t))
	case string:
		return isPositiveRangeString(t)
	default:
		return false
	}
}

// isPositiveNumberValue reports whether v is a JSON number (float64, as
// decoded from an extra map) that is a strictly positive integer. Used for
// xmux's h_keep_alive_period, which is a plain int64 in the core rather than
// a badoption.Range.
func isPositiveNumberValue(v interface{}) bool {
	n, ok := v.(float64)
	return ok && n > 0 && n == float64(int64(n))
}

// rangeRawFromExtra accepts only the two shapes isPositiveRangeValue validates
// — a bare positive number or a positive "a-b" range — and drops anything
// else so the core's own default applies instead of a decode error taking
// down the whole engine.
func rangeRawFromExtra(extra map[string]interface{}, camel, snake string) json.RawMessage {
	var v interface{}
	switch {
	case extra[snake] != nil:
		v = extra[snake]
	case extra[camel] != nil:
		v = extra[camel]
	default:
		return nil
	}
	if !isPositiveRangeValue(v) {
		return nil
	}
	switch t := v.(type) {
	case string:
		raw, _ := json.Marshal(strings.TrimSpace(t))
		return raw
	case float64:
		raw, _ := json.Marshal(int64(t))
		return raw
	default:
		return nil
	}
}

func boolPtrFromExtra(extra map[string]interface{}, camel, snake string) *bool {
	var v interface{}
	if x, ok := extra[snake]; ok {
		v = x
	} else if x, ok := extra[camel]; ok {
		v = x
	} else {
		return nil
	}
	// Providers quote everything when they embed the xhttp knobs as JSON inside
	// a URI ("xPaddingObfsMode":"true"), so a bare assertion on bool would drop
	// the flag silently — the very failure this helper exists to prevent.
	switch t := v.(type) {
	case bool:
		return &t
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "1", "true", "yes", "on":
			b := true
			return &b
		case "0", "false", "no", "off":
			b := false
			return &b
		}
	case float64:
		b := t != 0
		return &b
	}
	return nil
}

func xhttpPaddingFromExtra(extra map[string]interface{}) string {
	s := stringFromExtraValue(extra["x_padding_bytes"])
	if s == "" {
		s = stringFromExtraValue(extra["xPaddingBytes"])
	}
	s = sanitizeXPaddingBytes(s)
	if s == "" {
		// sing-box requires x_padding_bytes for xhttp; absence is treated as "disabled" and rejected
		return "100-1000"
	}
	return s
}

// xhttpPaddingPlacements / xhttpPaddingMethods canonicalize the two enum knobs
// of the padding-obfuscation profile. The core validates both while PARSING the
// config (checkV2RayXHTTPBaseOptions), and an unsupported value aborts the whole
// instance — every other node included — not just this outbound. So anything we
// do not recognize is dropped, and the core applies its own default
// (queryInHeader / repeat-x) instead of failing the start.
var xhttpPaddingPlacements = map[string]string{
	"cookie":          "cookie",
	"header":          "header",
	"query":           "query",
	"queryinheader":   "queryInHeader",
	"query_in_header": "queryInHeader",
	"query-in-header": "queryInHeader",
}

var xhttpPaddingMethods = map[string]string{
	"repeat-x": "repeat-x",
	"repeat_x": "repeat-x",
	"repeatx":  "repeat-x",
	"tokenish": "tokenish",
}

func xhttpPaddingPlacementFromExtra(extra map[string]interface{}) string {
	raw := firstNonEmpty(
		getStringField(extra, "x_padding_placement", ""),
		getStringField(extra, "xPaddingPlacement", ""),
	)
	return xhttpPaddingPlacements[strings.ToLower(strings.TrimSpace(raw))]
}

func xhttpPaddingMethodFromExtra(extra map[string]interface{}) string {
	raw := firstNonEmpty(
		getStringField(extra, "x_padding_method", ""),
		getStringField(extra, "xPaddingMethod", ""),
	)
	return xhttpPaddingMethods[strings.ToLower(strings.TrimSpace(raw))]
}

// xhttpSessionPlacements is the enum the core accepts for session_placement and
// seq_placement. Anything else aborts config decoding — and with it the engine.
var xhttpSessionPlacements = map[string]string{
	"path":   "path",
	"cookie": "cookie",
	"header": "header",
	"query":  "query",
}

func xhttpSessionPlacementFromExtra(extra map[string]interface{}, snake, camel string) string {
	raw := firstNonEmpty(getStringField(extra, snake, ""), getStringField(extra, camel, ""))
	return xhttpSessionPlacements[strings.ToLower(strings.TrimSpace(raw))]
}

// xhttpModes is the enum V2RayXHTTPOptions.UnmarshalJSON accepts for "mode"
// (transport/v2rayhttp in the core). Anything else fails decoding
// ("unsupported mode: ..."), which aborts the engine start — so an unknown
// value falls back to "auto", the branch's own long-standing default, instead
// of reaching the core.
var xhttpModes = map[string]bool{
	"auto": true, "packet-up": true, "stream-up": true, "stream-one": true,
}

func xhttpModeFromExtra(extra map[string]interface{}) string {
	raw := strings.ToLower(strings.TrimSpace(getStringField(extra, "mode", "")))
	if xhttpModes[raw] {
		return raw
	}
	return "auto"
}

// xhttpUplinkHTTPMethodFromExtra canonicalizes uplink_http_method. The core
// upper-cases it and rejects "GET" outside packet-up mode
// (checkV2RayXHTTPBaseOptions), which aborts the engine start; an empty value
// makes the core default to POST. Only POST and (packet-up-only) GET are ones
// this project has ever seen used, so anything else is dropped rather than
// forwarded unverified.
func xhttpUplinkHTTPMethodFromExtra(extra map[string]interface{}, mode string) string {
	raw := strings.ToUpper(strings.TrimSpace(firstNonEmpty(
		stringFromExtraValue(extra["uplink_http_method"]),
		stringFromExtraValue(extra["method"]),
	)))
	switch raw {
	case "POST":
		return "POST"
	case "GET":
		if mode == "packet-up" {
			return "GET"
		}
		return ""
	default:
		return ""
	}
}

// xhttpUplinkDataPlacementFromExtra is mode-aware on purpose: the core allows
// cookie/header only in packet-up mode and returns an error otherwise, which
// means the engine never starts.
func xhttpUplinkDataPlacementFromExtra(extra map[string]interface{}, mode string) string {
	raw := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		getStringField(extra, "uplink_data_placement", ""),
		getStringField(extra, "uplinkDataPlacement", ""),
	)))
	switch raw {
	case "auto", "body":
		return raw
	case "cookie", "header":
		if mode == "packet-up" {
			return raw
		}
		return ""
	default:
		return ""
	}
}

// xhttpCongestionControllers mirrors common/congestion.NewCongestionControl: an
// unknown name fails client creation, i.e. the engine start.
var xhttpCongestionControllers = map[string]bool{
	"bbr": true, "bbr_standard": true, "bbr2": true,
	"bbr2_variant": true, "cubic": true, "reno": true,
}

func xhttpCongestionFromExtra(extra map[string]interface{}) string {
	raw := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		getStringField(extra, "congestion_controller", ""),
		getStringField(extra, "congestionController", ""),
	)))
	if !xhttpCongestionControllers[raw] {
		return ""
	}
	return raw
}

// positiveIntFromExtra returns 0 for anything not strictly positive, so a bogus
// value falls back to the core's own default instead of being forwarded.
func positiveIntFromExtra(extra map[string]interface{}, snake, camel string) int {
	n := intFromExtra(extra, snake, camel)
	if n <= 0 {
		return 0
	}
	return n
}

func sanitizeXPaddingBytes(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "0", "0-0", "disable", "disabled", "none", "false":
		return ""
	}
	return s
}

// transportHeadersFromExtra returns the node's custom request headers for the
// HTTP-flavoured transports (ws, httpupgrade, http, xhttp). "host" is stripped:
// sing-box rejects it inside headers for xhttp, and the transports that do have
// a top-level host field take it from there instead.
func transportHeadersFromExtra(extra map[string]interface{}) map[string]string {
	v, ok := extra["headers"]
	if !ok || v == nil {
		return nil
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	out := make(map[string]string)
	for k, val := range m {
		if strings.ToLower(k) == "host" {
			continue // sing-box rejects "host" inside headers; it goes to the top-level Host field
		}
		s := stringFromExtraValue(val)
		if s != "" {
			out[k] = s
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// mkcpHeaderTypes mirrors v2raykcp.NewPacketHeader. An unknown obfuscation
// header does not fail transport creation — NewPacketHeader silently returns
// nil for it — but forwarding it anyway would still desync the wire framing
// from a server that expects a real header, so it is dropped and the core
// falls back to "none".
var mkcpHeaderTypes = map[string]string{
	"none": "none", "srtp": "srtp", "utp": "utp",
	"wechat-video": "wechat-video", "wechatvideo": "wechat-video",
	"dtls": "dtls", "wireguard": "wireguard",
}

func mkcpHeaderTypeFromExtra(extra map[string]interface{}) string {
	raw := firstNonEmpty(
		getStringField(extra, "header_type", ""),
		getStringField(extra, "headerType", ""),
	)
	return mkcpHeaderTypes[strings.ToLower(strings.TrimSpace(raw))]
}

func xhttpPreferH2ALPN(alpn []string, skipHack bool) []string {
	if len(alpn) == 0 {
		return []string{"h2", "http/1.1"}
	}
	if skipHack || alpn[0] != "h3" {
		return alpn
	}
	var rest []string
	hasH2 := false
	for _, p := range alpn {
		if p == "h2" {
			hasH2 = true
			continue
		}
		rest = append(rest, p)
	}
	if hasH2 {
		return append([]string{"h2"}, rest...)
	}
	return append([]string{"h2"}, alpn...)
}

func splitALPN(alpn string) []string {
	var result []string
	for _, s := range strings.FieldsFunc(alpn, func(r rune) bool {
		return r == ',' || r == '\n' || r == '|' || r == ';'
	}) {
		s = strings.TrimSpace(s)
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}

// defaultALPNForNetwork возвращает список ALPN по умолчанию на основе network-транспорта.
// Для plain TCP (Trojan, VLESS/VMESS без transport) используется только "http/1.1",
// так как h2 ALPN в Trojan-контексте может привести к тому что сервер согласует HTTP/2,
// а sing-box продолжит слать Trojan binary framing — это вызывает ERR_SSL_PROTOCOL_ERROR.
// Для транспортов которые реально используют HTTP/2 (ws, grpc, xhttp, h2) — добавляем h2.
func defaultALPNForNetwork(network string) []string {
	switch strings.ToLower(strings.TrimSpace(network)) {
	case "grpc", "xhttp", "splithttp", "http", "h2":
		return []string{"h2", "http/1.1"}
	case "ws", "websocket":
		// WebSocket работает поверх HTTP/1.1, но многие серверы принимают и h2 upgrade
		return []string{"h2", "http/1.1"}
	default:
		// tcp и прочие: только http/1.1
		return []string{"http/1.1"}
	}
}
