// Copyright (C) 2026 ResultProxy
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"encoding/json"
	"strings"
)

// parseExtra safely decodes the proxy.Extra JSON blob into a map.
func parseExtra(proxy ProxyConfig) map[string]interface{} {
	var extra map[string]interface{}
	if proxy.Extra != nil {
		json.Unmarshal(proxy.Extra, &extra) //nolint:errcheck — best-effort
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

// getBoolField returns a bool field from an extra map.
// Bug #6 fix: parameter was `key bool` — corrected to `key string`.
func getBoolField(extra map[string]interface{}, key string) bool {
	if val, ok := extra[key].(bool); ok {
		return val
	}
	return false
}

// buildProxyOutbound constructs the sing-box outbound config for the upstream proxy.
// Supports: HTTP, SOCKS5, Shadowsocks, VMess (with TLS+transport), VLess, Trojan.
func buildProxyOutbound(proxy ProxyConfig) SBOutbound {
	extra := parseExtra(proxy)

	switch proxy.Type {
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
		// Bug #3 fix: UUID must be in the `uuid` JSON field, not `username`.
		uuid := getStringField(extra, "uuid", "")
		out := SBOutbound{
			Type:       "vmess",
			Tag:        "proxy",
			Server:     proxy.IP,
			ServerPort: proxy.Port,
			UUID:       uuid,
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
		sni := getStringField(extra, "sni", proxy.IP)
		insecure := getBoolField(extra, "insecure")
		fp := getStringField(extra, "fp", "")
		out.TLS = &SBOutboundTLS{
			Enabled:    true,
			ServerName: sni,
			Insecure:   insecure,
		}
		if fp != "" {
			out.TLS.UTLS = &SBUTLS{Enabled: true, Fingerprint: fp}
		}
		if alpnStr := getStringField(extra, "alpn", ""); alpnStr != "" {
			out.TLS.ALPN = splitALPN(alpnStr)
		}
		applyTransportOnly(&out, extra)
		return out

	default: // "HTTP", "http", "HTTPS", "https"
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

// applyTLSAndTransport applies TLS (including Reality) and transport based on extra fields.
// Used for VMess and VLess.
func applyTLSAndTransport(out *SBOutbound, extra map[string]interface{}, defaultSNI string) {
	security := getStringField(extra, "security", "none")

	switch security {
	case "reality":
		sni := getStringField(extra, "sni", defaultSNI)
		fp := getStringField(extra, "fp", "chrome")
		pbk := getStringField(extra, "pbk", "")
		sid := getStringField(extra, "sid", "")
		out.TLS = &SBOutboundTLS{
			Enabled:    true,
			ServerName: sni,
			UTLS:       &SBUTLS{Enabled: true, Fingerprint: fp},
			Reality:    &SBReality{Enabled: true, PublicKey: pbk, ShortID: sid},
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
			alpnList := splitALPN(alpnStr)
			netw := getStringField(extra, "network", "")
			if netw == "xhttp" || netw == "splithttp" {
				tls.ALPN = xhttpPreferH2ALPN(alpnList)
			} else {
				tls.ALPN = alpnList
			}
		}
		out.TLS = tls
	default:
		if getBoolField(extra, "tls") {
			sni := getStringField(extra, "sni", defaultSNI)
			insecure := getBoolField(extra, "insecure")
			out.TLS = &SBOutboundTLS{
				Enabled:    true,
				ServerName: sni,
				Insecure:   insecure,
			}
		}
	}

	applyTransportOnly(out, extra)
}

// applyTransportOnly applies the transport layer (WS, gRPC, HTTP/2, XHTTP).
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
		out.Transport = &SBOutboundTransport{
			Type:        "grpc",
			ServiceName: getStringField(extra, "grpc-service-name", ""),
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
		if xPadding == "" {
			xPadding = "100-1000"
		}
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

// xmuxJSONFromExtra maps Xray share-link xmux keys to sing-box-extended JSON field names.
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

// xhttpPaddingFromExtra returns sing-box x_padding_bytes from extra (snake_case or Xray camelCase).
func xhttpPaddingFromExtra(extra map[string]interface{}) string {
	if s := stringFromExtraValue(extra["x_padding_bytes"]); s != "" {
		return s
	}
	return stringFromExtraValue(extra["xPaddingBytes"])
}

// xhttpHeadersFromExtra builds transport headers from extra["headers"] (object from VLESS `extra` JSON).
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

// xhttpPreferH2ALPN ensures the first ALPN is not h3. sing-box-extended xhttp uses
// tlsConfig.NextProtos()[0] to choose HTTP/2 (TCP) vs HTTP/3 (UDP); h3 first breaks
// many servers that only serve TLS+h2 on 443.
func xhttpPreferH2ALPN(alpn []string) []string {
	if len(alpn) == 0 {
		return []string{"h2", "http/1.1"}
	}
	if alpn[0] != "h3" {
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

// splitALPN splits a comma-separated ALPN string into a slice.
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
