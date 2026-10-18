package proxy

import (
	"encoding/json"
	"testing"
)

// DNS из .conf приезжает в ссылке параметром dns и уходит в extra под тем же
// ключом, под которым его кладёт парсер .conf на ПК, — dns_servers.
func TestWireGuardURIKeepsConfigDNS(t *testing.T) {
	for _, uri := range []string{
		"wg://PRIV@203.0.113.9:51820?public_key=PUB&address=10.0.0.2%2F32&dns=10.8.0.1%2C+1.1.1.1#w",
		"awg://PRIV@203.0.113.9:51820?public_key=PUB&address=10.0.0.2%2F32&jc=4&dns=10.8.0.1%2C+1.1.1.1#a",
	} {
		entry, err := ParseProxyURI(uri)
		if err != nil {
			t.Fatalf("%s: %v", uri, err)
		}
		var extra struct {
			DNSServers []string `json:"dns_servers"`
		}
		if err := json.Unmarshal(entry.Extra, &extra); err != nil {
			t.Fatal(err)
		}
		if len(extra.DNSServers) != 2 || extra.DNSServers[0] != "10.8.0.1" || extra.DNSServers[1] != "1.1.1.1" {
			t.Fatalf("%s: dns_servers = %v", entry.Type, extra.DNSServers)
		}
	}
}

func TestWireGuardURIWithoutDNSLeavesNoKey(t *testing.T) {
	entry, err := ParseProxyURI("awg://PRIV@203.0.113.9:51820?public_key=PUB&address=10.0.0.2%2F32&jc=4#a")
	if err != nil {
		t.Fatal(err)
	}
	var extra map[string]any
	_ = json.Unmarshal(entry.Extra, &extra)
	if _, ok := extra["dns_servers"]; ok {
		t.Fatalf("без dns в ссылке ключа быть не должно: %v", extra["dns_servers"])
	}
}
