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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"resultproxy-wails/internal/config"
)



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
	case strings.HasPrefix(line, "hy2://"):
		return parseHysteria2URI(line)
	case strings.HasPrefix(line, "hysteria2://"):
		return parseHysteria2URI(line)
	case strings.HasPrefix(line, "wg://"):
		return parseWireGuardURI(line)
	default:
		return config.ProxyEntry{}, fmt.Errorf("unsupported URI scheme: %s", truncate(line, 30))
	}
}


func ParseSubscriptionBody(body string) ([]config.ProxyEntry, error) {
	body = normalizeSubscriptionBody(body)
	if body == "" {
		return nil, fmt.Errorf("subscription is empty")
	}

	if entries, ok := parseSubscriptionJSON(body); ok {
		if len(entries) == 0 {
			return nil, fmt.Errorf("no valid proxy URIs found in subscription")
		}
		return entries, nil
	}

	if entries := parseSubscriptionLines(body); len(entries) > 0 {
		return entries, nil
	}

	decoded, err := base64Decode(body)
	if err == nil {
		decoded = normalizeSubscriptionBody(decoded)
		if entries, ok := parseSubscriptionJSON(decoded); ok {
			if len(entries) == 0 {
				return nil, fmt.Errorf("no valid proxy URIs found in subscription")
			}
			return entries, nil
		}
		if entries := parseSubscriptionLines(decoded); len(entries) > 0 {
			return entries, nil
		}
	}

	return nil, fmt.Errorf("unsupported subscription format")
}

func normalizeSubscriptionBody(body string) string {
	body = strings.TrimSpace(body)
	body = strings.TrimPrefix(body, "\uFEFF")
	return strings.TrimSpace(body)
}

func parseSubscriptionLines(body string) []config.ProxyEntry {
	lines := strings.Split(body, "\n")
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
	return entries
}

func parseSubscriptionJSON(body string) ([]config.ProxyEntry, bool) {
	if !strings.HasPrefix(body, "{") && !strings.HasPrefix(body, "[") {
		return nil, false
	}
	var root interface{}
	if err := json.Unmarshal([]byte(body), &root); err != nil {
		return nil, false
	}
	var objects []map[string]interface{}
	switch v := root.(type) {
	case []interface{}:
		for _, item := range v {
			if m, ok := asMap(item); ok {
				objects = append(objects, m)
			}
		}
	case map[string]interface{}:
		objects = append(objects, v)
		if arr, ok := asSlice(v["configs"]); ok {
			for _, item := range arr {
				if m, ok := asMap(item); ok {
					objects = append(objects, m)
				}
			}
		}
	}
	if len(objects) == 0 {
		return []config.ProxyEntry{}, true
	}
	entries := make([]config.ProxyEntry, 0, len(objects))
	for _, obj := range objects {
		entry, ok := parseJSONSubscriptionEntry(obj)
		if ok {
			entries = append(entries, entry)
		}
	}
	return entries, true
}

func parseJSONSubscriptionEntry(obj map[string]interface{}) (config.ProxyEntry, bool) {
	remarks := asString(obj["remarks"])
	outbounds, ok := asSlice(obj["outbounds"])
	if !ok || len(outbounds) == 0 {
		return config.ProxyEntry{}, false
	}
	firstOutbound, ok := asMap(outbounds[0])
	if !ok {
		return config.ProxyEntry{}, false
	}
	protocol := strings.ToLower(asString(firstOutbound["protocol"]))
	settings, _ := asMap(firstOutbound["settings"])
	stream, _ := asMap(firstOutbound["streamSettings"])
	switch protocol {
	case "vless", "vmess":
		vnext, ok := asSlice(settings["vnext"])
		if !ok || len(vnext) == 0 {
			return config.ProxyEntry{}, false
		}
		node, ok := asMap(vnext[0])
		if !ok {
			return config.ProxyEntry{}, false
		}
		host := asString(node["address"])
		port := asInt(node["port"])
		users, _ := asSlice(node["users"])
		user := map[string]interface{}{}
		if len(users) > 0 {
			user, _ = asMap(users[0])
		}
		name := remarks
		if name == "" {
			name = strings.ToUpper(protocol)
		}
		extra := map[string]interface{}{}
		if id := asString(user["id"]); id != "" {
			extra["uuid"] = id
		}
		if aid, ok := user["alterId"]; ok && protocol == "vmess" {
			extra["alterId"] = aid
		}
		if flow := asString(user["flow"]); flow != "" {
			extra["flow"] = flow
		}
		if enc := asString(user["encryption"]); enc != "" {
			extra["encryption"] = enc
		}
		if network := asString(stream["network"]); network != "" {
			extra["network"] = network
		}
		if security := asString(stream["security"]); security != "" {
			extra["security"] = security
		}
		if grpc, ok := asMap(stream["grpcSettings"]); ok {
			if sn := asString(grpc["serviceName"]); sn != "" {
				extra["grpc-service-name"] = sn
				extra["serviceName"] = sn
			}
			if auth := asString(grpc["authority"]); auth != "" {
				extra["authority"] = auth
			}
		}
		if ws, ok := asMap(stream["wsSettings"]); ok {
			if p := asString(ws["path"]); p != "" {
				extra["path"] = p
			}
			if h, ok := asMap(ws["headers"]); ok {
				if hostHeader := asString(h["Host"]); hostHeader != "" {
					extra["host"] = hostHeader
				}
			}
		}
		if reality, ok := asMap(stream["realitySettings"]); ok {
			if sni := asString(reality["serverName"]); sni != "" {
				extra["sni"] = sni
			}
			if pbk := asString(reality["publicKey"]); pbk != "" {
				extra["pbk"] = pbk
			}
			if sid := asString(reality["shortId"]); sid != "" {
				extra["sid"] = sid
			}
			if fp := asString(reality["fingerprint"]); fp != "" {
				extra["fp"] = fp
			}
		}
		extraJSON, _ := json.Marshal(extra)
		return config.ProxyEntry{
			IP:      host,
			Port:    port,
			Type:    strings.ToUpper(protocol),
			Name:    name,
			Country: countryFromNameAndHost(name, host),
			Extra:   extraJSON,
		}, host != "" && port > 0
	case "trojan":
		servers, ok := asSlice(settings["servers"])
		if !ok || len(servers) == 0 {
			return config.ProxyEntry{}, false
		}
		server, ok := asMap(servers[0])
		if !ok {
			return config.ProxyEntry{}, false
		}
		host := asString(server["address"])
		port := asInt(server["port"])
		password := asString(server["password"])
		name := remarks
		if name == "" {
			name = "TROJAN"
		}
		extra := map[string]interface{}{}
		if network := asString(stream["network"]); network != "" {
			extra["network"] = network
		}
		if security := asString(stream["security"]); security != "" {
			extra["security"] = security
		}
		if tls, ok := asMap(stream["tlsSettings"]); ok {
			if sni := asString(tls["serverName"]); sni != "" {
				extra["sni"] = sni
			}
		}
		extraJSON, _ := json.Marshal(extra)
		return config.ProxyEntry{
			IP:       host,
			Port:     port,
			Type:     "TROJAN",
			Name:     name,
			Password: password,
			Country:  countryFromNameAndHost(name, host),
			Extra:    extraJSON,
		}, host != "" && port > 0
	case "shadowsocks", "ss":
		servers, ok := asSlice(settings["servers"])
		if !ok || len(servers) == 0 {
			return config.ProxyEntry{}, false
		}
		server, ok := asMap(servers[0])
		if !ok {
			return config.ProxyEntry{}, false
		}
		host := asString(server["address"])
		port := asInt(server["port"])
		password := asString(server["password"])
		method := asString(server["method"])
		name := remarks
		if name == "" {
			name = "Shadowsocks"
		}
		extra := map[string]interface{}{}
		if method != "" {
			extra["method"] = method
		}
		extraJSON, _ := json.Marshal(extra)
		return config.ProxyEntry{
			IP:       host,
			Port:     port,
			Type:     "SS",
			Name:     name,
			Password: password,
			Country:  countryFromNameAndHost(name, host),
			Extra:    extraJSON,
		}, host != "" && port > 0
	default:
		return config.ProxyEntry{}, false
	}
}

func asMap(v interface{}) (map[string]interface{}, bool) {
	m, ok := v.(map[string]interface{})
	return m, ok
}

func asSlice(v interface{}) ([]interface{}, bool) {
	s, ok := v.([]interface{})
	return s, ok
}

func asString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func asInt(v interface{}) int {
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float32:
		return int(t)
	case float64:
		return int(t)
	case json.Number:
		i, _ := t.Int64()
		return int(i)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(t))
		return i
	default:
		return 0
	}
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
	if grpcServiceName := firstNonEmpty(
		params.Get("grpc-service-name"),
		params.Get("serviceName"),
		params.Get("service_name"),
		params.Get("grpc_service_name"),
	); grpcServiceName != "" {
		extra["grpc-service-name"] = grpcServiceName
		extra["serviceName"] = grpcServiceName
	}
	if grpcAuthority := firstNonEmpty(
		params.Get("authority"),
		params.Get("grpc-authority"),
		params.Get("grpc_authority"),
	); grpcAuthority != "" {
		extra["authority"] = grpcAuthority
	}

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
	network := firstNonEmpty(
		params.Get("network"),
		params.Get("type"),
		"tcp",
	)
	network = strings.ToLower(strings.TrimSpace(network))
	isGrpcNetwork := network == "grpc"

	var sni string
	if isGrpcNetwork {
		sni = firstNonEmpty(
			params.Get("sni"),
			params.Get("peer"),
			params.Get("serverName"),
			params.Get("servername"),
			params.Get("server_name"),
		)
	} else {
		sni = firstNonEmpty(
			params.Get("sni"),
			params.Get("serverName"),
			params.Get("servername"),
			params.Get("server_name"),
			params.Get("peer"),
		)
	}
	insecure := parseBoolFlexible(firstNonEmpty(
		params.Get("insecure"),
		params.Get("allowInsecure"),
		params.Get("allow_insecure"),
		params.Get("skip-cert-verify"),
		params.Get("skip_cert_verify"),
	))
	extra := map[string]interface{}{
		"sni":      sni,
		"fp":       params.Get("fp"),
		"network":  network,
		"path":     params.Get("path"),
		"host":     params.Get("host"),
		"security": paramOr(params, "security", "tls"),
		"alpn":     params.Get("alpn"),
		"insecure": insecure,
		"pbk":      params.Get("pbk"),
		"sid":      params.Get("sid"),
		"spx":      params.Get("spx"),
		"flow":     params.Get("flow"),
	}
	if grpcServiceName := firstNonEmpty(
		params.Get("grpc-service-name"),
		params.Get("serviceName"),
		params.Get("service_name"),
		params.Get("grpc_service_name"),
	); grpcServiceName != "" {
		extra["grpc-service-name"] = grpcServiceName
		extra["serviceName"] = grpcServiceName
	}
	if grpcAuthority := firstNonEmpty(
		params.Get("authority"),
		params.Get("grpc-authority"),
		params.Get("grpc_authority"),
	); grpcAuthority != "" {
		extra["authority"] = grpcAuthority
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

func parseHysteria2URI(uri string) (config.ProxyEntry, error) {
	normalized := uri
	if strings.HasPrefix(normalized, "hysteria2://") {
		normalized = strings.Replace(normalized, "hysteria2://", "hy2://", 1)
	}
	u, err := url.Parse(strings.Replace(normalized, "hy2://", "http://", 1))
	if err != nil {
		return config.ProxyEntry{}, fmt.Errorf("parsing Hysteria2 URI: %w", err)
	}

	port, _ := strconv.Atoi(u.Port())
	name := "Hysteria2"
	if u.Fragment != "" {
		name, _ = url.PathUnescape(u.Fragment)
	}

	params := u.Query()
	insecure := parseBoolFlexible(params.Get("insecure"))
	extra := map[string]interface{}{
		"password":      u.User.Username(),
		"sni":           params.Get("sni"),
		"alpn":          params.Get("alpn"),
		"insecure":      insecure,
		"obfs_type":     params.Get("obfs"),
		"obfs_password": params.Get("obfs-password"),
	}

	extraJSON, _ := json.Marshal(extra)
	host := u.Hostname()
	return config.ProxyEntry{
		IP:      host,
		Port:    port,
		Type:    "HYSTERIA2",
		Name:    name,
		Country: countryFromNameAndHost(name, host),
		Extra:   extraJSON,
	}, nil
}

func parseWireGuardURI(uri string) (config.ProxyEntry, error) {
	u, err := url.Parse(strings.Replace(uri, "wg://", "http://", 1))
	if err != nil {
		return config.ProxyEntry{}, fmt.Errorf("parsing WireGuard URI: %w", err)
	}
	port, _ := strconv.Atoi(u.Port())
	name := "WireGuard"
	if u.Fragment != "" {
		name, _ = url.PathUnescape(u.Fragment)
	}
	params := u.Query()
	password, _ := u.User.Password()
	privateKey := u.User.Username()
	if privateKey == "" {
		privateKey = params.Get("private_key")
		if privateKey == "" {
			privateKey = params.Get("privateKey")
		}
	}
	extra := map[string]interface{}{
		"private_key": privateKey,
		"public_key":  firstNonEmpty(params.Get("public_key"), params.Get("publicKey")),
		"address":     splitCSV(params.Get("address")),
		"allowed_ips": splitCSV(firstNonEmpty(params.Get("allowed_ips"), params.Get("allowedIps"))),
	}
	if psk := firstNonEmpty(password, params.Get("pre_shared_key"), params.Get("preSharedKey")); psk != "" {
		extra["pre_shared_key"] = psk
	}
	if mtu := strings.TrimSpace(firstNonEmpty(params.Get("mtu"), params.Get("MTU"))); mtu != "" {
		if v, convErr := strconv.Atoi(mtu); convErr == nil {
			extra["mtu"] = v
		}
	}
	extraJSON, _ := json.Marshal(extra)
	host := u.Hostname()
	return config.ProxyEntry{
		IP:      host,
		Port:    port,
		Type:    "WIREGUARD",
		Name:    name,
		Country: countryFromNameAndHost(name, host),
		Extra:   extraJSON,
	}, nil
}




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

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func splitCSV(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		n := strings.TrimSpace(p)
		if n != "" {
			out = append(out, n)
		}
	}
	return out
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

func parseBoolFlexible(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
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
