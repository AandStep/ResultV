// Copyright (C) 2026 ResultProxy
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"resultproxy-wails/internal/config"
)

// ParseProxyURI parses a single protocol URI into a ProxyEntry.
// Supported schemes: vless://, vmess://, ss://, trojan://
func ParseProxyURI(line string) (config.ProxyEntry, error) {
	line = strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(line, "vless://"):
		return parseVLESSURI(line)
	case strings.HasPrefix(line, "vmess://"):
		return parseVMessURI(line)
	case strings.HasPrefix(line, "ss://"):
		return parseShadowsocksURI(line)
	case strings.HasPrefix(line, "trojan://"):
		return parseTrojanURI(line)
	default:
		return config.ProxyEntry{}, fmt.Errorf("unsupported URI scheme: %s", truncate(line, 30))
	}
}

// ParseSubscriptionBody decodes a base64 subscription body into proxy entries.
func ParseSubscriptionBody(body string) ([]config.ProxyEntry, error) {
	decoded, err := base64Decode(body)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}

	lines := strings.Split(decoded, "\n")
	var entries []config.ProxyEntry
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		entry, err := ParseProxyURI(line)
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no valid proxy URIs found in subscription")
	}
	return entries, nil
}

func parseVLESSURI(uri string) (config.ProxyEntry, error) {
	u, err := url.Parse(strings.Replace(uri, "vless://", "http://", 1))
	if err != nil {
		return config.ProxyEntry{}, fmt.Errorf("parsing VLESS URI: %w", err)
	}

	port, _ := strconv.Atoi(u.Port())
	name := "VLESS"
	if u.Fragment != "" {
		name, _ = url.PathUnescape(u.Fragment)
	}

	params := u.Query()
	// Xray-style share links put XHTTP options (padding, headers, …) in JSON under query `extra`.
	// Merge embedded JSON first; explicit query fields below override the same keys.
	extra := map[string]interface{}{}
	mergeVLESSURLEmbeddedExtra(extra, params.Get("extra"))

	extra["uuid"] = u.User.Username()
	extra["network"] = paramOr(params, "type", "tcp")
	extra["security"] = paramOr(params, "security", "none")
	extra["sni"] = params.Get("sni")
	extra["fp"] = params.Get("fp")
	extra["pbk"] = params.Get("pbk")
	extra["sid"] = params.Get("sid")
	extra["flow"] = params.Get("flow")
	extra["path"] = params.Get("path")
	extra["host"] = params.Get("host")
	extra["alpn"] = params.Get("alpn")
	extra["mode"] = params.Get("mode")
	extra["method"] = params.Get("method")

	normalizeVLESSExtraPadding(extra)

	extraJSON, _ := json.Marshal(extra)
	host := u.Hostname()
	return config.ProxyEntry{
		IP:      host,
		Port:    port,
		Type:    "VLESS",
		Name:    name,
		Country: countryFromNameAndHost(name, host),
		Extra:   extraJSON,
	}, nil
}

func parseVMessURI(uri string) (config.ProxyEntry, error) {
	b64 := strings.TrimPrefix(uri, "vmess://")
	decoded, err := base64Decode(b64)
	if err != nil {
		return config.ProxyEntry{}, fmt.Errorf("decoding VMess: %w", err)
	}

	var v map[string]interface{}
	if err := json.Unmarshal([]byte(decoded), &v); err != nil {
		return config.ProxyEntry{}, fmt.Errorf("parsing VMess JSON: %w", err)
	}

	host, _ := v["add"].(string)
	portVal, _ := v["port"]
	port := toInt(portVal)
	name, _ := v["ps"].(string)
	if name == "" {
		name = "VMess"
	}

	extra := map[string]interface{}{
		"uuid":    v["id"],
		"alterId": v["aid"],
		"network": v["net"],
		"path":    v["path"],
		"host":    v["host"],
	}
	if tls, ok := v["tls"].(string); ok && tls == "tls" {
		extra["security"] = "tls"
		extra["tls"] = true
		if sni, ok := v["sni"].(string); ok {
			extra["sni"] = sni
		}
	}

	extraJSON, _ := json.Marshal(extra)
	return config.ProxyEntry{
		IP:      host,
		Port:    port,
		Type:    "VMESS",
		Name:    name,
		Country: countryFromNameAndHost(name, host),
		Extra:   extraJSON,
	}, nil
}

func parseShadowsocksURI(uri string) (config.ProxyEntry, error) {
	remainder := strings.TrimPrefix(uri, "ss://")
	name := "Shadowsocks"
	if idx := strings.Index(remainder, "#"); idx >= 0 {
		name, _ = url.PathUnescape(remainder[idx+1:])
		remainder = remainder[:idx]
	}

	var method, password, host string
	var port int

	if strings.Contains(remainder, "@") {
		parts := strings.SplitN(remainder, "@", 2)
		decoded, err := base64Decode(parts[0])
		if err != nil {
			return config.ProxyEntry{}, fmt.Errorf("decoding SS auth: %w", err)
		}
		authParts := strings.SplitN(decoded, ":", 2)
		if len(authParts) == 2 {
			method = authParts[0]
			password = authParts[1]
		}
		serverPart := strings.SplitN(parts[1], "?", 2)[0]
		hp := strings.SplitN(serverPart, ":", 2)
		host = hp[0]
		if len(hp) > 1 {
			port, _ = strconv.Atoi(hp[1])
		}
	} else {
		decoded, err := base64Decode(remainder)
		if err != nil {
			return config.ProxyEntry{}, fmt.Errorf("decoding SS: %w", err)
		}
		if strings.Contains(decoded, "@") {
			parts := strings.SplitN(decoded, "@", 2)
			authParts := strings.SplitN(parts[0], ":", 2)
			if len(authParts) == 2 {
				method = authParts[0]
				password = authParts[1]
			}
			hp := strings.SplitN(parts[1], ":", 2)
			host = hp[0]
			if len(hp) > 1 {
				port, _ = strconv.Atoi(hp[1])
			}
		}
	}

	if method == "" {
		method = "aes-256-gcm"
	}

	extra := map[string]interface{}{"method": method}
	extraJSON, _ := json.Marshal(extra)

	return config.ProxyEntry{
		IP:       host,
		Port:     port,
		Type:     "SS",
		Name:     name,
		Password: password,
		Country:  countryFromNameAndHost(name, host),
		Extra:    extraJSON,
	}, nil
}

func parseTrojanURI(uri string) (config.ProxyEntry, error) {
	u, err := url.Parse(strings.Replace(uri, "trojan://", "http://", 1))
	if err != nil {
		return config.ProxyEntry{}, fmt.Errorf("parsing Trojan URI: %w", err)
	}

	port, _ := strconv.Atoi(u.Port())
	password := u.User.Username()
	name := "Trojan"
	if u.Fragment != "" {
		name, _ = url.PathUnescape(u.Fragment)
	}

	params := u.Query()
	extra := map[string]interface{}{
		"sni":      params.Get("sni"),
		"fp":       params.Get("fp"),
		"network":  paramOr(params, "type", "tcp"),
		"path":     params.Get("path"),
		"host":     params.Get("host"),
		"security": paramOr(params, "security", "tls"),
	}

	extraJSON, _ := json.Marshal(extra)
	thost := u.Hostname()
	return config.ProxyEntry{
		IP:       thost,
		Port:     port,
		Type:     "TROJAN",
		Name:     name,
		Password: password,
		Country:  countryFromNameAndHost(name, thost),
		Extra:    extraJSON,
	}, nil
}

// --- helpers ---

// mergeVLESSURLEmbeddedExtra parses the `extra` query value (JSON object) and merges into dst.
func mergeVLESSURLEmbeddedExtra(dst map[string]interface{}, raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	var inner map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &inner); err != nil {
		dec, err2 := url.QueryUnescape(raw)
		if err2 != nil || dec == raw {
			return
		}
		if err := json.Unmarshal([]byte(dec), &inner); err != nil {
			return
		}
	}
	if inner == nil {
		return
	}
	for k, v := range inner {
		dst[k] = v
	}
	normalizeVLESSExtraPadding(dst)
}

// normalizeVLESSExtraPadding copies camelCase xPaddingBytes (Xray link JSON) to x_padding_bytes for outbound.
func normalizeVLESSExtraPadding(extra map[string]interface{}) {
	if stringFromExtraValue(extra["x_padding_bytes"]) != "" {
		return
	}
	if s := stringFromExtraValue(extra["xPaddingBytes"]); s != "" {
		extra["x_padding_bytes"] = s
	}
}

func stringFromExtraValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	default:
		return fmt.Sprint(v)
	}
}

func base64Decode(s string) (string, error) {
	s = strings.TrimSpace(s)
	for _, enc := range [](*base64.Encoding){base64.StdEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.RawURLEncoding} {
		if decoded, err := enc.DecodeString(s); err == nil {
			return string(decoded), nil
		}
	}
	return "", fmt.Errorf("not valid base64")
}

func paramOr(params url.Values, key, fallback string) string {
	v := params.Get(key)
	if v == "" {
		return fallback
	}
	return v
}

func toInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(n)
		return i
	case int:
		return n
	default:
		return 0
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

const riMin, riMax = '\U0001F1E6', '\U0001F1FF'

func countryFromLeadingFlagEmoji(displayName string) string {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return ""
	}
	r, w := utf8.DecodeRuneInString(displayName)
	if r == utf8.RuneError || w == 0 {
		return ""
	}
	r2, w2 := utf8.DecodeRuneInString(displayName[w:])
	if r2 == utf8.RuneError || w2 == 0 {
		return ""
	}
	if r >= riMin && r <= riMax && r2 >= riMin && r2 <= riMax {
		c1 := byte(r - riMin + 'a')
		c2 := byte(r2 - riMin + 'a')
		if c1 >= 'a' && c1 <= 'z' && c2 >= 'a' && c2 <= 'z' {
			return string([]byte{c1, c2})
		}
	}
	return ""
}

// countryHintFromHostname uses the first DNS label (e.g. fi.bvpn.cc → fi, de2.example → de).
func countryHintFromHostname(hostname string) string {
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	if hostname == "" || strings.HasPrefix(hostname, "[") {
		return ""
	}
	dot := strings.IndexByte(hostname, '.')
	if dot <= 0 {
		return ""
	}
	first := hostname[:dot]
	if len(first) == 2 && first[0] >= 'a' && first[0] <= 'z' && first[1] >= 'a' && first[1] <= 'z' {
		return first
	}
	if len(first) >= 3 && first[0] >= 'a' && first[0] <= 'z' && first[1] >= 'a' && first[1] <= 'z' {
		suffix := first[2:]
		allNum := true
		for i := 0; i < len(suffix); i++ {
			if suffix[i] < '0' || suffix[i] > '9' {
				allNum = false
				break
			}
		}
		if allNum {
			return first[:2]
		}
	}
	return ""
}

func countryFromNameAndHost(displayName, hostname string) string {
	if c := countryFromLeadingFlagEmoji(displayName); c != "" {
		return c
	}
	return countryHintFromHostname(hostname)
}
