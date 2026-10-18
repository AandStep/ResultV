package mobile

import (
	"encoding/json"
	"testing"

	"resultproxy-wails/internal/proxy"
)

func tunnelDNSAddrs(t *testing.T, cfgJSON string) map[string]bool {
	t.Helper()
	var sb proxy.SingBoxConfig
	if err := json.Unmarshal([]byte(cfgJSON), &sb); err != nil {
		t.Fatal(err)
	}
	addrs := map[string]bool{}
	for _, s := range sb.DNS.Servers {
		if s.Server != "" {
			addrs[s.Server] = true
		}
	}
	return addrs
}

const awgWithDNS = "awg://PRIV@203.0.113.9:51820?public_key=PUB&address=10.0.0.2%2F32&jc=4&dns=10.8.0.1#a"

// DNS из конфига узла сильнее пресета из настроек — так же решает ПК
// (dnsServersFromProxyExtra в app.go).
func TestNodeDNSOverridesSettingsPreset(t *testing.T) {
	cfg, err := BuildSingBoxConfigV2(awgWithDNS, t.TempDir(), encodeOptions(BuildOptions{DNSServers: "9.9.9.9, 149.112.112.112"}))
	if err != nil {
		t.Fatal(err)
	}
	addrs := tunnelDNSAddrs(t, cfg)
	if !addrs["10.8.0.1"] {
		t.Fatalf("DNS узла не попал в конфиг: %v", addrs)
	}
	if addrs["9.9.9.9"] || addrs["149.112.112.112"] {
		t.Fatalf("пресет из настроек должен уступить DNS узла: %v", addrs)
	}
}

func TestSettingsPresetAppliesWhenNodeHasNoDNS(t *testing.T) {
	cfg, err := BuildSingBoxConfigV2(
		"awg://PRIV@203.0.113.9:51820?public_key=PUB&address=10.0.0.2%2F32&jc=4#a",
		t.TempDir(), encodeOptions(BuildOptions{DNSServers: "9.9.9.9"}))
	if err != nil {
		t.Fatal(err)
	}
	if addrs := tunnelDNSAddrs(t, cfg); !addrs["9.9.9.9"] {
		t.Fatalf("без DNS у узла работает пресет: %v", addrs)
	}
}
