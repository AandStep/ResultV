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
func resolvedFingerprint(extra map[string]interface{}) string {
	raw := strings.TrimSpace(strings.ToLower(fingerprintFromExtra(extra)))
	switch raw {
	case "none", "off", "disable", "disabled", "-":
		return ""
	case "":
		return system.WebViewFingerprint()
	default:
		return raw
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

func buildProxyOutbound(proxy ProxyConfig) SBOutbound {
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
			UpMbps:     intFromExtra(extra, "up_mbps", "upMbps"),
			DownMbps:   intFromExtra(extra, "down_mbps", "downMbps"),
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
		return SBOutbound{
			Type:       "shadowsocks",
			Tag:        "proxy",
			Server:     proxy.IP,
			ServerPort: proxy.Port,
			Password:   proxy.Password,
			Method:     method,
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
			PacketEncoding: resolvePacketEncoding(extra),
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
	network := getStringField(extra, "network", "tcp")
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
		out.Transport = &SBOutboundTransport{
			Type:                "ws",
			Path:                path,
			Host:                host,
			MaxEarlyData:        maxEarly,
			EarlyDataHeaderName: edHeader,
		}
	case "httpupgrade":
		path := getStringField(extra, "path", "/")
		host := getStringField(extra, "host", "")
		out.Transport = &SBOutboundTransport{
			Type: "httpupgrade",
			Path: path,
			Host: host,
		}
	case "grpc":
		serviceName := getStringField(extra, "grpc-service-name", "")
		if serviceName == "" {
			serviceName = getStringField(extra, "serviceName", "")
		}
		if serviceName == "" {
			serviceName = getStringField(extra, "service_name", "")
		}
		idleTimeout := firstNonEmpty(
			getStringField(extra, "idle_timeout", ""),
			getStringField(extra, "idleTimeout", ""),
		)
		pingTimeout := firstNonEmpty(
			getStringField(extra, "ping_timeout", ""),
			getStringField(extra, "pingTimeout", ""),
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
		out.Transport = &SBOutboundTransport{
			Type:   "http",
			Host:   getStringField(extra, "http-host", ""),
			Path:   getStringField(extra, "http-path", "/"),
			Method: getStringField(extra, "http-method", ""),
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
		mode := getStringField(extra, "mode", "auto")
		if mode == "" {
			mode = "auto"
		}
		uplink := stringFromExtraValue(extra["uplink_http_method"])
		if uplink == "" {
			uplink = stringFromExtraValue(extra["method"])
		}
		headers := xhttpHeadersFromExtra(extra)
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
		}
	}
}

func xmuxJSONFromExtra(extra map[string]interface{}) json.RawMessage {
	v, ok := extra["xmux"]
	if !ok || v == nil {
		return nil
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	rename := map[string]string{
		"maxConcurrency":   "max_concurrency",
		"maxConnections":   "max_connections",
		"cMaxReuseTimes":   "c_max_reuse_times",
		"hMaxRequestTimes": "h_max_request_times",
		"hMaxReusableSecs": "h_max_reusable_secs",
		"hKeepAlivePeriod": "h_keep_alive_period",
	}
	out := make(map[string]interface{}, len(m))
	for k, val := range m {
		if nk, ok := rename[k]; ok {
			out[nk] = val
		} else {
			out[k] = val
		}
	}
	raw, err := json.Marshal(out)
	if err != nil || len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return raw
}

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
	switch t := v.(type) {
	case string:
		if strings.TrimSpace(t) == "" {
			return nil
		}
		raw, _ := json.Marshal(t)
		return raw
	case map[string]interface{}:
		raw, _ := json.Marshal(t)
		return raw
	case float64:
		raw, _ := json.Marshal(int64(t))
		return raw
	default:
		raw, _ := json.Marshal(v)
		return raw
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
	b, ok := v.(bool)
	if !ok {
		return nil
	}
	return &b
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

func sanitizeXPaddingBytes(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "0", "0-0", "disable", "disabled", "none", "false":
		return ""
	}
	return s
}

func xhttpHeadersFromExtra(extra map[string]interface{}) map[string]string {
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
