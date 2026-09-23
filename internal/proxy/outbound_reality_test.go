package proxy

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
)

// REALITY servers built after 2026-09-08 (Xray-core >= v26.9.8) drop a
// ClientHello without an X25519MLKEM768 key share, and the core strips it
// unless told otherwise.
func TestRealityKeepsX25519MLKEM768KeyShare(t *testing.T) {
	entry, err := ParseProxyURI("vless://af815621-b245-4149-89da-dd184cfc4b3d@example.com:443?type=tcp&security=reality&sni=google.com&fp=chrome&pbk=-lCQjfUSO29QFSrFrbIyHPfaSQzjIWRg7lMhaPStUzA&sid=f2b30c6723854005#r")
	if err != nil {
		t.Fatal(err)
	}
	out := buildProxyOutbound(ProxyConfig{IP: entry.IP, Port: entry.Port, Type: entry.Type, Username: entry.Username, Password: entry.Password, Extra: entry.Extra})
	out.Tag = "proxy"

	raw, err := json.Marshal(map[string]any{"outbounds": []SBOutbound{out}})
	if err != nil {
		t.Fatal(err)
	}
	var options option.Options
	if err := singjson.UnmarshalContext(extendedBoxContext(context.Background()), raw, &options); err != nil {
		t.Fatalf("core rejected outbound: %v", err)
	}
	vless, ok := options.Outbounds[0].Options.(*option.VLESSOutboundOptions)
	if !ok || vless.TLS == nil || vless.TLS.Reality == nil {
		t.Fatalf("reality not parsed: %s", raw)
	}
	if !vless.TLS.Reality.SupportX25519MLKEM768 {
		t.Fatalf("support_x25519mlkem768 not set: %s", raw)
	}
}
