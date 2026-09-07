package mobile

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

type blobEntry struct {
	IP    string          `json:"ip"`
	Port  int             `json:"port"`
	Type  string          `json:"type"`
	Name  string          `json:"name"`
	Extra json.RawMessage `json:"extra"`
}

func parseBlob(t *testing.T, text string) []blobEntry {
	t.Helper()
	out, err := ParseProxyBlob(text)
	if err != nil {
		t.Fatalf("ParseProxyBlob: %v", err)
	}
	var entries []blobEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	return entries
}

const blobVLESS = "vless://11111111-1111-1111-1111-111111111111@1.2.3.4:443?security=tls&type=tcp#one"
const blobTrojan = "trojan://secret@5.6.7.8:8443?security=tls#two"

// Pasted share-links, one per line — what the Link pane used to do with a
// per-line loop of its own.
func TestParseProxyBlob_Lines(t *testing.T) {
	entries := parseBlob(t, blobVLESS+"\n"+blobTrojan+"\n")
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(entries), entries)
	}
	if entries[0].Type != "VLESS" || entries[0].IP != "1.2.3.4" {
		t.Errorf("first entry = %+v", entries[0])
	}
	if entries[1].Type != "TROJAN" || entries[1].Port != 8443 {
		t.Errorf("second entry = %+v", entries[1])
	}
}

// Subscription bodies are commonly base64; the same blob pasted by hand has to
// travel the same path.
func TestParseProxyBlob_Base64(t *testing.T) {
	body := base64.StdEncoding.EncodeToString([]byte(blobVLESS + "\n" + blobTrojan))
	if len(parseBlob(t, body)) != 2 {
		t.Fatal("base64 body did not yield both entries")
	}
}

// Xray shape: the server lives in settings.vnext.
func TestParseProxyBlob_XrayOutbound(t *testing.T) {
	const j = `{
	  "protocol": "vless",
	  "tag": "xray-node",
	  "settings": {"vnext": [{"address": "9.9.9.9", "port": 443,
	    "users": [{"id": "22222222-2222-2222-2222-222222222222", "encryption": "none"}]}]},
	  "streamSettings": {"network": "ws", "security": "tls"}
	}`
	entries := parseBlob(t, j)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1: %+v", len(entries), entries)
	}
	if entries[0].IP != "9.9.9.9" || entries[0].Type != "VLESS" {
		t.Errorf("entry = %+v", entries[0])
	}
}

// A whole config rather than a lone outbound: the proxy outbounds are taken,
// the plumbing ones (freedom/blackhole) are not.
func TestParseProxyBlob_ConfigWithOutbounds(t *testing.T) {
	const j = `{
	  "outbounds": [
	    {"protocol": "freedom", "tag": "direct"},
	    {"protocol": "trojan", "tag": "t",
	     "settings": {"servers": [{"address": "4.4.4.4", "port": 8443, "password": "p"}]}},
	    {"protocol": "blackhole", "tag": "block"}
	  ]
	}`
	entries := parseBlob(t, j)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1: %+v", len(entries), entries)
	}
	if entries[0].Type != "TROJAN" || entries[0].IP != "4.4.4.4" {
		t.Errorf("entry = %+v", entries[0])
	}
}

// sing-box shape: no settings wrapper, the server sits at the outbound root.
// The desktop's JS parser never handled this one; the spec asks for it.
func TestParseProxyBlob_SingBoxOutbound(t *testing.T) {
	const j = `{
	  "type": "vless",
	  "tag": "sb-node",
	  "server": "7.7.7.7",
	  "server_port": 8443,
	  "uuid": "33333333-3333-3333-3333-333333333333",
	  "flow": "xtls-rprx-vision",
	  "tls": {"enabled": true, "server_name": "example.com"}
	}`
	entries := parseBlob(t, j)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1: %+v", len(entries), entries)
	}
	e := entries[0]
	if e.Type != "VLESS" || e.IP != "7.7.7.7" || e.Port != 8443 {
		t.Fatalf("entry = %+v", e)
	}
	var extra map[string]any
	if err := json.Unmarshal(e.Extra, &extra); err != nil {
		t.Fatalf("extra %q: %v", e.Extra, err)
	}
	if extra["uuid"] != "33333333-3333-3333-3333-333333333333" {
		t.Errorf("uuid = %v", extra["uuid"])
	}
	if extra["sni"] != "example.com" {
		t.Errorf("sni = %v", extra["sni"])
	}
	if extra["flow"] != "xtls-rprx-vision" {
		t.Errorf("flow = %v", extra["flow"])
	}
}

func TestParseProxyBlob_SingBoxShadowsocksOutbound(t *testing.T) {
	const j = `{"type":"shadowsocks","server":"8.8.4.4","server_port":8388,
	            "method":"aes-256-gcm","password":"pw","tag":"sb-ss"}`
	entries := parseBlob(t, j)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1: %+v", len(entries), entries)
	}
	if entries[0].IP != "8.8.4.4" || entries[0].Port != 8388 {
		t.Errorf("entry = %+v", entries[0])
	}
}

func TestParseProxyBlob_Garbage(t *testing.T) {
	if _, err := ParseProxyBlob("не ссылка и не конфиг"); err == nil {
		t.Fatal("expected an error for unparseable text")
	}
	if _, err := ParseProxyBlob("   "); err == nil {
		t.Fatal("expected an error for blank text")
	}
}
