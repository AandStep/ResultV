package mobile

import (
	"encoding/json"
	"testing"

	"github.com/sagernet/sing-box/experimental/libbox"
	"resultproxy-wails/internal/proxy"
)

func TestAndroidSocks5ExitUsesVPNAsFirstHop(t *testing.T) {
	const uri = "vless://11111111-1111-4111-8111-111111111111@203.0.113.7:443?encryption=none#test"
	raw, err := BuildSingBoxConfigV2(uri, t.TempDir(), `{"bidirectionalRules":true,"excludedDomains":"direct.example.org","socks5Exit":{"enabled":true,"host":"192.0.2.1","port":1080,"sites":["exit.example.org"],"apps":["org.example.app"]}}`)
	if err != nil {
		t.Fatal(err)
	}
	var cfg proxy.SingBoxConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	var direct, exit int
	for i, r := range cfg.Route.Rules {
		if r.Outbound == "direct" && len(r.DomainSuffix) == 1 && r.DomainSuffix[0] == "direct.example.org" {
			direct = i + 1
		}
		if r.Outbound == "socks5-exit" {
			exit = i + 1
		}
	}
	if direct == 0 || exit == 0 || direct > exit || cfg.Route.Final != "proxy" {
		t.Fatalf("need direct rules before selected exit rules and ordinary VPN otherwise: direct=%d exit=%d final=%q", direct, exit, cfg.Route.Final)
	}
	var wire struct{ Outbounds []map[string]any }
	if err := json.Unmarshal([]byte(raw), &wire); err != nil {
		t.Fatal(err)
	}
	for _, out := range wire.Outbounds {
		if out["tag"] == "socks5-exit" && out["detour"] == "proxy" {
			if err := libbox.CheckConfig(raw); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("missing SOCKS5 exit with a VPN detour")
}
