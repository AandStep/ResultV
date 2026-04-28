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
	"strings"
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
			Password:   getStringField(extra, "password", proxy.Password),
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
		out := SBOutbound{
			Type:       "vmess",
			Tag:        "proxy",
			Server:     proxy.IP,
			ServerPort: proxy.Port,
			UUID:       uuid,
			AlterId:    alterId,
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
		fp := getStringField(extra, "fp", "chrome")
		if pbk == "" {
			pbk = getStringField(extra, "pbk", getStringField(extra, "publicKey", getStringField(extra, "public_key", "")))
		}
		sid := getStringField(extra, "sid", "")
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
	case "tls":
		sni := getStringField(extra, "sni", defaultSNI)
		insecure := getBoolField(extra, "insecure")
		fp := getStringField(extra, "fp", "")
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
			if fp := getStringField(extra, "fp", ""); fp != "" {
				out.TLS.UTLS = &SBUTLS{Enabled: true, Fingerprint: fp}
			}
			if alpnStr := getStringField(extra, "alpn", ""); alpnStr != "" {
				out.TLS.ALPN = xhttpPreferH2ALPN(splitALPN(alpnStr), getBoolField(extra, "insecure_h3_override"))
			}
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
		out.Transport = &SBOutboundTransport{
			Type: "ws",
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
		out.Transport = &SBOutboundTransport{
			Type:        "grpc",
			ServiceName: serviceName,
			// authority is not supported by sing-box gRPC transport; SNI goes into tls.server_name
		}
	case "http", "h2":
		out.Transport = &SBOutboundTransport{
			Type: "http",
			Host: getStringField(extra, "http-host", ""),
			Path: getStringField(extra, "http-path", "/"),
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
	for _, s := range strings.Split(alpn, ",") {
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
