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

import "strings"

// normalizeSingBoxOutbound rewrites a sing-box-shaped outbound into the xray
// shape parseJSONOutbound reads.
//
// The two spell the same facts differently: sing-box keeps the server at the
// outbound root with `tls` and `transport` beside it, while xray nests them
// under `settings` and `streamSettings`. Teaching every protocol branch a
// second spelling would mean two descriptions of each protocol to keep in
// step, so the translation happens once, here, and the branches stay as they
// were.
//
// Untouched:
//   - naive, wireguard and amneziawg, whose branches already read the sing-box
//     root directly (parseJSONWireGuardOutbound accepts both spellings);
//   - anything that already carries `settings`, i.e. is xray-shaped;
//   - anything with no `server` to hang the rewrite on.
//
// The returned map is a copy — the caller's object is left alone.
func normalizeSingBoxOutbound(protocol string, outbound map[string]interface{}) map[string]interface{} {
	switch protocol {
	case "vless", "vmess", "trojan", "shadowsocks", "ss", "hysteria", "hysteria2", "hy2":
	default:
		return outbound
	}
	if _, ok := asMap(outbound["settings"]); ok {
		return outbound
	}
	host := asString(outbound["server"])
	port := asInt(outbound["server_port"])
	if host == "" || port == 0 {
		return outbound
	}

	settings := map[string]interface{}{}
	switch protocol {
	case "vless", "vmess":
		user := map[string]interface{}{}
		if id := asString(outbound["uuid"]); id != "" {
			user["id"] = id
		}
		if flow := asString(outbound["flow"]); flow != "" {
			user["flow"] = flow
		}
		if enc := asString(outbound["encryption"]); enc != "" {
			user["encryption"] = enc
		}
		if aid, ok := outbound["alter_id"]; ok {
			user["alterId"] = aid
		}
		settings["vnext"] = []interface{}{map[string]interface{}{
			"address": host,
			"port":    port,
			"users":   []interface{}{user},
		}}
	case "trojan":
		settings["servers"] = []interface{}{map[string]interface{}{
			"address":  host,
			"port":     port,
			"password": asString(outbound["password"]),
		}}
	case "shadowsocks", "ss":
		settings["servers"] = []interface{}{map[string]interface{}{
			"address":  host,
			"port":     port,
			"password": asString(outbound["password"]),
			"method":   asString(outbound["method"]),
		}}
	default: // hysteria family
		settings["address"] = host
		settings["port"] = port
	}

	out := make(map[string]interface{}, len(outbound)+2)
	for k, v := range outbound {
		out[k] = v
	}
	out["settings"] = settings
	if stream := singBoxStreamSettings(protocol, outbound); len(stream) > 0 {
		out["streamSettings"] = stream
	}
	return out
}

// singBoxStreamSettings builds the xray `streamSettings` a sing-box outbound
// implies from its `transport` and `tls` objects.
//
// Transports beyond ws and grpc contribute only their name: the protocol
// branches read nothing else from them, and inventing settings they never
// look at would be noise.
func singBoxStreamSettings(protocol string, outbound map[string]interface{}) map[string]interface{} {
	stream := map[string]interface{}{}

	if transport, ok := asMap(outbound["transport"]); ok {
		kind := strings.ToLower(asString(transport["type"]))
		if kind != "" {
			stream["network"] = kind
		}
		switch kind {
		case "ws":
			ws := map[string]interface{}{}
			if p := asString(transport["path"]); p != "" {
				ws["path"] = p
			}
			if headers, ok := asMap(transport["headers"]); ok {
				if h := singBoxHeader(headers, "Host"); h != "" {
					ws["headers"] = map[string]interface{}{"Host": h}
				}
			}
			if len(ws) > 0 {
				stream["wsSettings"] = ws
			}
		case "grpc":
			if sn := asString(transport["service_name"]); sn != "" {
				stream["grpcSettings"] = map[string]interface{}{"serviceName": sn}
			}
		}
	}

	if tls, ok := asMap(outbound["tls"]); ok {
		if enabled, _ := tls["enabled"].(bool); enabled {
			sni := asString(tls["server_name"])
			fingerprint := ""
			if utls, ok := asMap(tls["utls"]); ok {
				fingerprint = asString(utls["fingerprint"])
			}

			tlsSettings := map[string]interface{}{}
			if sni != "" {
				tlsSettings["serverName"] = sni
			}
			if fingerprint != "" {
				tlsSettings["fingerprint"] = fingerprint
			}
			if alpn, ok := asSlice(tls["alpn"]); ok && len(alpn) > 0 {
				tlsSettings["alpn"] = alpn
			}
			if insecure, ok := tls["insecure"].(bool); ok {
				tlsSettings["allowInsecure"] = insecure
			}
			stream["tlsSettings"] = tlsSettings

			// REALITY rides inside the same `tls` object in sing-box; xray
			// makes it its own security mode with its own settings block.
			reality, hasReality := asMap(tls["reality"])
			realityOn := false
			if hasReality {
				realityOn, _ = reality["enabled"].(bool)
			}
			if realityOn {
				r := map[string]interface{}{}
				if sni != "" {
					r["serverName"] = sni
				}
				if pbk := asString(reality["public_key"]); pbk != "" {
					r["publicKey"] = pbk
				}
				if sid := asString(reality["short_id"]); sid != "" {
					r["shortId"] = sid
				}
				if fingerprint != "" {
					r["fingerprint"] = fingerprint
				}
				stream["security"] = "reality"
				stream["realitySettings"] = r
			} else {
				stream["security"] = "tls"
			}
		}
	}

	switch protocol {
	case "hysteria", "hysteria2", "hy2":
		// The xray branch reads the auth string out of hysteriaSettings;
		// sing-box calls it `password` and keeps it at the root.
		if pw := asString(outbound["password"]); pw != "" {
			stream["hysteriaSettings"] = map[string]interface{}{"auth": pw}
		}
	}

	return stream
}

// singBoxHeader reads one transport header. sing-box allows a bare string or a
// list of values for each name; xray wants a single string.
func singBoxHeader(headers map[string]interface{}, name string) string {
	for key, value := range headers {
		if !strings.EqualFold(key, name) {
			continue
		}
		switch v := value.(type) {
		case string:
			return v
		case []interface{}:
			if len(v) > 0 {
				return asString(v[0])
			}
		}
	}
	return ""
}
