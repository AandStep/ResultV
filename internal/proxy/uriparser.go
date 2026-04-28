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

// getQueryParamCI looks up a URL query parameter case-insensitively.
// It tries the exact key first, then Title case, then UPPER CASE.
// This is needed for AWG URIs where providers use Jc/JC/jc interchangeably.
func getQueryParamCI(params url.Values, key string) string {
	if v := params.Get(key); v != "" {
		return v
	}
	// Title case: jc → Jc, jmin → Jmin
	if len(key) > 0 {
		titled := strings.ToUpper(key[:1]) + key[1:]
		if v := params.Get(titled); v != "" {
			return v
		}
	}
	// ALL CAPS: jc → JC
	if v := params.Get(strings.ToUpper(key)); v != "" {
		return v
	}
	// Brute-force: iterate all keys for case-insensitive match
	lowerKey := strings.ToLower(key)
	for k, vals := range params {
		if strings.ToLower(k) == lowerKey && len(vals) > 0 && vals[0] != "" {
			return vals[0]
		}
	}
	return ""
}



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
	case strings.HasPrefix(line, "awg://"):
		return parseAmneziaWGURI(line)
	default:
		return config.ProxyEntry{}, fmt.Errorf("unsupported URI scheme: %s", truncate(line, 30))
	}
}


func ParseSubscriptionBody(body string) ([]config.ProxyEntry, error) {
	decrypted, err := tryDecryptSubscription(body)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt subscription: %w", err)
	}
	body = normalizeSubscriptionBody(decrypted)
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
	var entries []config.ProxyEntry
	for _, obj := range objects {
		parsed := parseJSONSubscriptionEntry(obj)
		if len(parsed) > 0 {
			entries = append(entries, parsed...)
		}
	}
	return entries, true
}

func parseJSONSubscriptionEntry(obj map[string]interface{}) []config.ProxyEntry {
	var entries []config.ProxyEntry
	
	// Иногда obj сам является outbound'ом
	protocol := asString(obj["protocol"])
	if protocol != "" {
		if entry, ok := parseJSONOutbound(obj, asString(obj["tag"])); ok {
			entries = append(entries, entry)
		}
	}

	// Иногда obj содержит массив outbounds
	outbounds, ok := asSlice(obj["outbounds"])
	if ok {
		for _, ob := range outbounds {
			if obMap, ok := asMap(ob); ok {
				remarks := asString(obj["remarks"]) // Top-level remarks
				if remarks == "" {
					remarks = asString(obMap["tag"]) // Или используем tag как имя
				}
				if entry, ok := parseJSONOutbound(obMap, remarks); ok {
					entries = append(entries, entry)
				}
			}
		}
	}

	return entries
}

func parseJSONOutbound(outbound map[string]interface{}, name string) (config.ProxyEntry, bool) {
	protocol := strings.ToLower(asString(outbound["protocol"]))
	if protocol == "freedom" || protocol == "blackhole" || protocol == "dns" {
		return config.ProxyEntry{}, false
	}

	settings, _ := asMap(outbound["settings"])
	stream, _ := asMap(outbound["streamSettings"])

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
		// Parse tlsSettings for VLESS/VMESS (alpn, sni, fp, insecure)
		if tls, ok := asMap(stream["tlsSettings"]); ok {
			if sni := asString(tls["serverName"]); sni != "" {
				extra["sni"] = sni
			}
			if fp := asString(tls["fingerprint"]); fp != "" {
				extra["fp"] = fp
			}
			if alpn, ok := asSlice(tls["alpn"]); ok && len(alpn) > 0 {
				var parts []string
				for _, a := range alpn {
					if s := asString(a); s != "" {
						parts = append(parts, s)
					}
				}
				if len(parts) > 0 {
					extra["alpn"] = strings.Join(parts, ",")
				}
			} else if alpnStr := asString(tls["alpn"]); alpnStr != "" {
				extra["alpn"] = alpnStr
			}
			if insecure := tls["allowInsecure"]; insecure != nil {
				if b, ok := insecure.(bool); ok {
					extra["insecure"] = b
				}
			}
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
		if xhttp, ok := asMap(stream["xhttpSettings"]); ok {
			if p := asString(xhttp["path"]); p != "" {
				extra["path"] = p
			}
			switch h := xhttp["host"].(type) {
			case string:
				if h != "" {
					extra["host"] = h
				}
			case []interface{}:
				if len(h) > 0 {
					if s := asString(h[0]); s != "" {
						extra["host"] = s
					}
				}
			}
			if mode := asString(xhttp["mode"]); mode != "" {
				extra["mode"] = mode
			}
			if method := asString(xhttp["method"]); method != "" {
				extra["method"] = method
			}
			if innerExtra, ok := asMap(xhttp["extra"]); ok {
				for k, v := range innerExtra {
					extra[k] = v
				}
				normalizeVLESSExtraPadding(extra)
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
		if xhttp, ok := asMap(stream["xhttpSettings"]); ok {
			if p := asString(xhttp["path"]); p != "" {
				extra["path"] = p
			}
			switch h := xhttp["host"].(type) {
			case string:
				if h != "" {
					extra["host"] = h
				}
			case []interface{}:
				if len(h) > 0 {
					if s := asString(h[0]); s != "" {
						extra["host"] = s
					}
				}
			}
			if mode := asString(xhttp["mode"]); mode != "" {
				extra["mode"] = mode
			}
			if method := asString(xhttp["method"]); method != "" {
				extra["method"] = method
			}
			if innerExtra, ok := asMap(xhttp["extra"]); ok {
				for k, v := range innerExtra {
					extra[k] = v
				}
				normalizeVLESSExtraPadding(extra)
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
	case "hysteria", "hysteria2", "hy2":
		host := asString(settings["address"])
		port := asInt(settings["port"])
		
		var auth string
		if hySettings, ok := asMap(stream["hysteriaSettings"]); ok {
			auth = asString(hySettings["auth"])
		}
		
		if name == "" {
			name = "HYSTERIA2"
		}
		
		extra := map[string]interface{}{}
		extra["password"] = auth
		
		if tls, ok := asMap(stream["tlsSettings"]); ok {
			if sni := asString(tls["serverName"]); sni != "" {
				extra["sni"] = sni
			}
			if fp := asString(tls["fingerprint"]); fp != "" {
				extra["fp"] = fp
			}
			if alpn, ok := asSlice(tls["alpn"]); ok && len(alpn) > 0 {
				extra["alpn"] = asString(alpn[0])
			}
		}
		
		extraJSON, _ := json.Marshal(extra)
		return config.ProxyEntry{
			IP:       host,
			Port:     port,
			Type:     "HYSTERIA2",
			Name:     name,
			Password: auth,
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
		"mode":     params.Get("mode"),
		"method":   params.Get("method"),
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




func parseAmneziaWGURI(uri string) (config.ProxyEntry, error) {
	u, err := url.Parse(strings.Replace(uri, "awg://", "http://", 1))
	if err != nil {
		return config.ProxyEntry{}, fmt.Errorf("parsing AmneziaWG URI: %w", err)
	}
	port, _ := strconv.Atoi(u.Port())
	name := "AmneziaWG"
	if u.Fragment != "" {
		name, _ = url.PathUnescape(u.Fragment)
	}
	params := u.Query()
	password, _ := u.User.Password()
	username := u.User.Username()

	privateKey := params.Get("private_key")
	if privateKey == "" {
		privateKey = params.Get("privateKey")
	}
	publicKey := firstNonEmpty(params.Get("public_key"), params.Get("publicKey"))

	// If one is missing, fallback to username
	if privateKey == "" {
		privateKey = username
	} else if publicKey == "" {
		publicKey = username
	}

	extra := map[string]interface{}{
		"private_key": privateKey,
		"public_key":  publicKey,
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

	// Amnezia-specific obfuscation parameters.
	// Use case-insensitive lookup because AmneziaVPN clients emit
	// capitalized keys (Jc, Jmin, S1, H1, …) while other generators
	// use lowercase (jc, jmin, s1, h1, …).
	amnezia := map[string]interface{}{}
	amneziaIntKeys := []string{"jc", "jmin", "jmax", "s1", "s2", "s3", "s4", "h1", "h2", "h3", "h4", "itime"}
	for _, k := range amneziaIntKeys {
		if v := strings.TrimSpace(getQueryParamCI(params, k)); v != "" {
			if n, convErr := strconv.ParseInt(v, 10, 64); convErr == nil {
				amnezia[k] = n
			}
		}
	}
	amneziaStringKeys := []string{"i1", "i2", "i3", "i4", "i5", "j1", "j2", "j3"}
	for _, k := range amneziaStringKeys {
		if v := strings.TrimSpace(getQueryParamCI(params, k)); v != "" {
			amnezia[k] = v
		}
	}
	if len(amnezia) > 0 {
		extra["amnezia"] = amnezia
	}

	extraJSON, _ := json.Marshal(extra)
	host := u.Hostname()
	return config.ProxyEntry{
		IP:      host,
		Port:    port,
		Type:    "AMNEZIAWG",
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

// StripLeadingFlagEmoji splits "🇷🇺 impVPN Auto" into ("🇷🇺", "impVPN Auto").
// Returns ("", s) when the string does not start with a regional-indicator pair.
func StripLeadingFlagEmoji(s string) (emoji, rest string) {
	s = strings.TrimSpace(s)
	r1, w1 := utf8.DecodeRuneInString(s)
	if r1 == utf8.RuneError || w1 == 0 || r1 < riMin || r1 > riMax {
		return "", s
	}
	r2, w2 := utf8.DecodeRuneInString(s[w1:])
	if r2 == utf8.RuneError || w2 == 0 || r2 < riMin || r2 > riMax {
		return "", s
	}
	return s[:w1+w2], strings.TrimSpace(s[w1+w2:])
}

// nameBaseParts strips the leading flag emoji, then returns the trimmed remainder.
// It also removes any explicit separator suffix (" | …", " – …", " — …", " - …").
func nameBaseParts(raw string) string {
	_, base := StripLeadingFlagEmoji(raw)
	for _, sep := range []string{" | ", " – ", " — ", " - "} {
		if idx := strings.Index(base, sep); idx > 0 {
			return strings.TrimSpace(base[:idx])
		}
	}
	return base
}

// lcpRunes returns the longest common UTF-8 prefix of a and b.
func lcpRunes(a, b string) string {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	// Walk back to a valid UTF-8 boundary.
	for i > 0 && !utf8.RuneStart(a[i-1]) {
		i--
	}
	return a[:i]
}

// trimAutoName removes trailing spaces and common separator characters so that
// e.g. "impVPN Auto " → "impVPN Auto".
func trimAutoName(s string) string {
	return strings.TrimRight(s, " \t|-–—")
}

// ExtractAutoGroupName returns the shared base name and true when every entry
// shares the same base name after stripping a leading flag emoji and any
// " | suffix" variant. Returns ("", false) when names differ or are empty.
//
// Two detection strategies are tried in order:
//  1. Exact match after separator-suffix stripping  ("impVPN Auto | VLESS" → "impVPN Auto").
//  2. Longest-common-prefix across all raw base names ("impVPN Auto VLESS" and
//     "impVPN Auto HYSTERIA2" → "impVPN Auto"). The LCP must be ≥ 3 runes.
func ExtractAutoGroupName(entries []config.ProxyEntry) (string, bool) {
	if len(entries) == 0 {
		return "", false
	}

	bases := make([]string, len(entries))
	for i, e := range entries {
		bases[i] = nameBaseParts(e.Name)
	}

	// Strategy 1: exact match after separator stripping.
	first := bases[0]
	if first != "" {
		allSame := true
		for _, b := range bases[1:] {
			if b != first {
				allSame = false
				break
			}
		}
		if allSame {
			return first, true
		}
	}

	// Strategy 2: longest common prefix of raw (flag-stripped) base names.
	rawBases := make([]string, len(entries))
	for i, e := range entries {
		_, rawBases[i] = StripLeadingFlagEmoji(e.Name)
	}
	lcp := rawBases[0]
	for _, b := range rawBases[1:] {
		lcp = lcpRunes(lcp, b)
		if lcp == "" {
			break
		}
	}
	lcp = trimAutoName(lcp)
	if utf8.RuneCountInString(lcp) >= 3 {
		return lcp, true
	}

	return "", false
}

// AllSameBaseName returns true when every entry shares the same non-emoji name part.
func AllSameBaseName(entries []config.ProxyEntry) bool {
	_, ok := ExtractAutoGroupName(entries)
	return ok
}

// FilterInvalidSubscriptionEntries removes sentinel/routing-only entries
// with no real host (IP == "0.0.0.0").
func FilterInvalidSubscriptionEntries(entries []config.ProxyEntry) []config.ProxyEntry {
	out := make([]config.ProxyEntry, 0, len(entries))
	for _, e := range entries {
		if e.IP == "0.0.0.0" {
			continue
		}
		out = append(out, e)
	}
	return out
}

// SplitAutoEntries separates entries whose base name (after stripping a
// leading flag emoji) contains the word "auto" (case-insensitive) from the
// rest. This handles providers that send a mix of "Auto" and individual
// server entries in the same subscription response.
//
// Returns:
//   - autoEntries: entries that belong to the auto group
//   - autoName: the shared display name for the auto group (e.g. "🚀 impVPN Auto")
//   - individualEntries: entries that are not part of the auto group
//   - ok: true when at least 2 auto entries were found with the same base name
func SplitAutoEntries(entries []config.ProxyEntry) (autoEntries []config.ProxyEntry, autoName string, individualEntries []config.ProxyEntry, ok bool) {
	if len(entries) == 0 {
		return nil, "", nil, false
	}

	for _, e := range entries {
		_, base := StripLeadingFlagEmoji(e.Name)
		if containsWordAuto(base) {
			autoEntries = append(autoEntries, e)
		} else {
			individualEntries = append(individualEntries, e)
		}
	}

	if len(autoEntries) < 2 {
		// Not enough auto entries — treat everything as individual.
		return nil, "", entries, false
	}

	autoName, ok = ExtractAutoGroupName(autoEntries)
	if !ok {
		// Auto entries don't share a common name — fall back.
		return nil, "", entries, false
	}

	return autoEntries, autoName, individualEntries, true
}

// containsWordAuto checks whether s contains the word "auto" as a
// case-insensitive whole word (not part of "autostart" etc.).
func containsWordAuto(s string) bool {
	low := strings.ToLower(s)
	idx := strings.Index(low, "auto")
	if idx < 0 {
		return false
	}
	// Check that "auto" is at a word boundary (end of string or followed by
	// a non-letter character).
	end := idx + 4
	if end < len(low) {
		next := low[end]
		if next >= 'a' && next <= 'z' {
			return false
		}
	}
	return true
}
