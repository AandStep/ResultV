package mobile

import (
	"encoding/json"
	"testing"
)

// entryFixture is a minimal VLESS entry usable by the config builder.
const entryFixture = `{"name":"t","type":"VLESS","ip":"1.2.3.4","port":443,"uri":"vless://11111111-1111-1111-1111-111111111111@1.2.3.4:443?security=reality&sni=example.com&pbk=abc&fp=chrome&type=tcp#t"}`

func buildForOptions(t *testing.T, opts BuildOptions) SingBoxConfigForTest {
	t.Helper()
	b, _ := json.Marshal(opts)
	cfg, err := BuildSingBoxConfigFromEntryV2(entryFixture, t.TempDir(), string(b))
	if err != nil {
		t.Fatalf("build config: %v", err)
	}
	var parsed SingBoxConfigForTest
	if err := json.Unmarshal([]byte(cfg), &parsed); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	return parsed
}

// SingBoxConfigForTest is a thin view over the built config's inbounds — just
// enough to assert the browser-adblock SOCKS inbound presence/shape.
type SingBoxConfigForTest struct {
	Inbounds []struct {
		Type       string `json:"type"`
		Tag        string `json:"tag"`
		Listen     string `json:"listen"`
		ListenPort int    `json:"listen_port"`
	} `json:"inbounds"`
}

func (c SingBoxConfigForTest) socksInbound() (found bool, listen string, port int) {
	for _, in := range c.Inbounds {
		if in.Tag == "browser-adblock-in" {
			return true, in.Listen, in.ListenPort
		}
	}
	return false, "", 0
}

func TestBuildConfig_BrowserAdBlockOn_AddsLoopbackSocksInbound(t *testing.T) {
	cfg := buildForOptions(t, BuildOptions{BrowserAdBlock: true})
	found, listen, port := cfg.socksInbound()
	if !found {
		t.Fatalf("expected browser-adblock SOCKS inbound, inbounds=%+v", cfg.Inbounds)
	}
	if listen != "127.0.0.1" {
		t.Errorf("SOCKS inbound must listen on loopback, got %q", listen)
	}
	if port != BrowserAdBlockSocksPort {
		t.Errorf("SOCKS inbound port = %d, want %d", port, BrowserAdBlockSocksPort)
	}
}

func TestBuildConfig_BrowserAdBlockOff_NoSocksInbound(t *testing.T) {
	cfg := buildForOptions(t, BuildOptions{})
	if found, _, _ := cfg.socksInbound(); found {
		t.Fatalf("did not expect a SOCKS inbound when BrowserAdBlock is off, inbounds=%+v", cfg.Inbounds)
	}
}
