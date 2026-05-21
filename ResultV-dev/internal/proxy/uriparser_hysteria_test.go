package proxy

import (
	"testing"
)

func TestParseSubscriptionBodyJSONAllOutboundsAndHysteria(t *testing.T) {
	jsonBody := `{
  "outbounds": [
    {
      "tag": "basic-proxy-1",
      "protocol": "vless",
      "settings": {
        "vnext": [{"address": "example.com", "port": 443, "users": [{"id": "uuid"}]}]
      }
    },
    {
      "tag": "basic-proxy-2",
      "protocol": "hysteria",
      "settings": {
        "address": "hy2.example.com",
        "port": 3443,
        "version": 2
      },
      "streamSettings": {
        "network": "hysteria",
        "hysteriaSettings": {
          "version": 2,
          "auth": "pass"
        },
        "security": "tls",
        "tlsSettings": {
          "serverName": "hy2.example.com",
          "fingerprint": "chrome",
          "alpn": ["h3"]
        }
      }
    }
  ]
}`

	entries, err := ParseSubscriptionBody(jsonBody)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].Type != "VLESS" {
		t.Errorf("expected VLESS, got %s", entries[0].Type)
	}

	if entries[1].Type != "HYSTERIA2" {
		t.Errorf("expected HYSTERIA2, got %s", entries[1].Type)
	}
	if entries[1].IP != "hy2.example.com" {
		t.Errorf("expected IP hy2.example.com, got %s", entries[1].IP)
	}
	if entries[1].Port != 3443 {
		t.Errorf("expected port 3443, got %d", entries[1].Port)
	}
	if entries[1].Password != "pass" {
		t.Errorf("expected password 'pass', got '%s'", entries[1].Password)
	}
}
