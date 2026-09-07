package proxy

import (
	"encoding/json"
	"testing"
)

func parseOutboundJSON(t *testing.T, raw string) (map[string]interface{}, map[string]interface{}) {
	t.Helper()
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	entries, err := ParseSubscriptionBody(raw)
	if err != nil {
		t.Fatalf("ParseSubscriptionBody: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1: %+v", len(entries), entries)
	}
	extra := map[string]interface{}{}
	if len(entries[0].Extra) > 0 {
		if err := json.Unmarshal(entries[0].Extra, &extra); err != nil {
			t.Fatalf("extra %q: %v", entries[0].Extra, err)
		}
	}
	entryMap := map[string]interface{}{
		"ip":   entries[0].IP,
		"port": entries[0].Port,
		"type": entries[0].Type,
		"name": entries[0].Name,
	}
	return entryMap, extra
}

// REALITY lives inside sing-box's `tls` object; xray makes it its own security
// mode. The translation has to land in the fields the VLESS branch reads.
func TestSingBoxOutbound_VLESSRealityOverWS(t *testing.T) {
	entry, extra := parseOutboundJSON(t, `{
	  "type": "vless",
	  "tag": "node",
	  "server": "1.1.1.1",
	  "server_port": 443,
	  "uuid": "11111111-1111-1111-1111-111111111111",
	  "flow": "xtls-rprx-vision",
	  "tls": {
	    "enabled": true,
	    "server_name": "example.com",
	    "utls": {"enabled": true, "fingerprint": "chrome"},
	    "reality": {"enabled": true, "public_key": "PBK", "short_id": "ab12"}
	  },
	  "transport": {"type": "ws", "path": "/ws", "headers": {"Host": "cdn.example.com"}}
	}`)

	if entry["ip"] != "1.1.1.1" || entry["port"] != 443 || entry["type"] != "VLESS" {
		t.Fatalf("entry = %+v", entry)
	}
	if entry["name"] != "node" {
		t.Errorf("name = %v, want the outbound tag", entry["name"])
	}
	for key, want := range map[string]string{
		"uuid":     "11111111-1111-1111-1111-111111111111",
		"flow":     "xtls-rprx-vision",
		"sni":      "example.com",
		"fp":       "chrome",
		"pbk":      "PBK",
		"sid":      "ab12",
		"network":  "ws",
		"security": "reality",
		"path":     "/ws",
		"host":     "cdn.example.com",
	} {
		if extra[key] != want {
			t.Errorf("extra[%q] = %v, want %q", key, extra[key], want)
		}
	}
}

// Plain TLS must not be reported as reality just because a tls object exists.
func TestSingBoxOutbound_TrojanPlainTLS(t *testing.T) {
	entry, extra := parseOutboundJSON(t, `{
	  "type": "trojan", "server": "2.2.2.2", "server_port": 8443, "password": "pw",
	  "tls": {"enabled": true, "server_name": "t.example.com", "insecure": true,
	          "alpn": ["h2", "http/1.1"]},
	  "transport": {"type": "grpc", "service_name": "GunService"}
	}`)

	if entry["type"] != "TROJAN" || entry["port"] != 8443 {
		t.Fatalf("entry = %+v", entry)
	}
	if extra["security"] != "tls" {
		t.Errorf("security = %v, want tls", extra["security"])
	}
	if extra["network"] != "grpc" {
		t.Errorf("network = %v, want grpc", extra["network"])
	}
	if extra["sni"] != "t.example.com" {
		t.Errorf("sni = %v", extra["sni"])
	}
}

// The rewrite must not touch an outbound that is already xray-shaped, even
// when it happens to carry a `server` key too.
func TestSingBoxOutbound_XrayShapeUntouched(t *testing.T) {
	raw := map[string]interface{}{
		"protocol": "vless",
		"server":   "decoy.example.com",
		"settings": map[string]interface{}{
			"vnext": []interface{}{map[string]interface{}{"address": "3.3.3.3", "port": 443}},
		},
	}
	out := normalizeSingBoxOutbound("vless", raw)
	if _, added := out["streamSettings"]; added {
		t.Error("xray-shaped outbound gained streamSettings")
	}
	settings, _ := asMap(out["settings"])
	if _, ok := asSlice(settings["vnext"]); !ok {
		t.Error("original settings were replaced")
	}
}

// naive and wireguard branches read the sing-box root themselves; rewriting
// them would hide the fields they expect.
func TestSingBoxOutbound_RootReadingProtocolsSkipped(t *testing.T) {
	for _, protocol := range []string{"naive", "wireguard", "amneziawg"} {
		raw := map[string]interface{}{
			"type": protocol, "server": "4.4.4.4", "server_port": 443,
		}
		out := normalizeSingBoxOutbound(protocol, raw)
		if _, added := out["settings"]; added {
			t.Errorf("%s outbound was rewritten", protocol)
		}
	}
}

// Without a server there is nothing to rewrite; the outbound must come back
// unchanged rather than gaining an empty settings block that the branches
// would then read as "present but broken".
func TestSingBoxOutbound_NoServerLeftAlone(t *testing.T) {
	raw := map[string]interface{}{"type": "vless", "uuid": "x"}
	out := normalizeSingBoxOutbound("vless", raw)
	if _, added := out["settings"]; added {
		t.Error("serverless outbound gained settings")
	}
}
