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
	"testing"
)

// TestMKCP_VLESSURICarriesSeedAndHeader: without seed and headerType the node still
// builds but won't connect — client and server seeds would mismatch. These knobs
// must reach the transport.
func TestMKCP_VLESSURICarriesSeedAndHeader(t *testing.T) {
	entry, err := ParseProxyURI("vless://af815621-b245-4149-89da-dd184cfc4b3d@203.0.113.7:443?type=kcp&security=none&seed=secret-seed&headerType=srtp&mtu=1350#kcp")
	if err != nil {
		t.Fatalf("ParseProxyURI: %v", err)
	}
	out := buildProxyOutbound(ProxyConfig{Type: entry.Type, IP: entry.IP, Port: entry.Port, Extra: entry.Extra})
	if out.Transport == nil || out.Transport.Type != "mkcp" {
		t.Fatalf("transport = %+v", out.Transport)
	}
	if out.Transport.Seed != "secret-seed" || out.Transport.HeaderType != "srtp" || out.Transport.MTU != 1350 {
		t.Fatalf("mkcp knobs lost between URI and outbound: %+v", out.Transport)
	}
}

// TestMKCP_VMessURICarriesSeedAndHeader: in vmess://, the header type lives in "type"
// and the seed lives in "path" — but only when net == kcp.
func TestMKCP_VMessURICarriesSeedAndHeader(t *testing.T) {
	payload := `{"v":"2","ps":"kcp","add":"203.0.113.7","port":"443","id":"af815621-b245-4149-89da-dd184cfc4b3d","aid":"0","net":"kcp","type":"srtp","path":"secret-seed","tls":""}`
	entry, err := ParseProxyURI("vmess://" + base64.StdEncoding.EncodeToString([]byte(payload)))
	if err != nil {
		t.Fatalf("ParseProxyURI: %v", err)
	}
	out := buildProxyOutbound(ProxyConfig{Type: entry.Type, IP: entry.IP, Port: entry.Port, Extra: entry.Extra})
	if out.Transport == nil || out.Transport.Type != "mkcp" {
		t.Fatalf("transport = %+v", out.Transport)
	}
	if out.Transport.Seed != "secret-seed" || out.Transport.HeaderType != "srtp" {
		t.Fatalf("mkcp knobs lost: %+v", out.Transport)
	}
}

// TestMKCP_VMessWebsocketUnaffected is the safety net for this task's main risk: for a
// ws node, "path" stays a path and "type" never turns into a header type.
func TestMKCP_VMessWebsocketUnaffected(t *testing.T) {
	payload := `{"v":"2","ps":"ws","add":"203.0.113.7","port":"443","id":"af815621-b245-4149-89da-dd184cfc4b3d","aid":"0","net":"ws","type":"none","path":"/wspath","host":"cdn.example.com","tls":"tls"}`
	entry, err := ParseProxyURI("vmess://" + base64.StdEncoding.EncodeToString([]byte(payload)))
	if err != nil {
		t.Fatalf("ParseProxyURI: %v", err)
	}
	var extra map[string]interface{}
	if err := json.Unmarshal(entry.Extra, &extra); err != nil {
		t.Fatal(err)
	}
	if extra["path"] != "/wspath" {
		t.Fatalf("ws path mangled: %v", extra["path"])
	}
	if _, ok := extra["seed"]; ok {
		t.Fatalf("seed invented for a ws node: %v", extra)
	}
	if _, ok := extra["headerType"]; ok {
		t.Fatalf("headerType invented for a ws node: %v", extra)
	}
	out := buildProxyOutbound(ProxyConfig{Type: entry.Type, IP: entry.IP, Port: entry.Port, Extra: entry.Extra})
	if out.Transport == nil || out.Transport.Type != "ws" {
		t.Fatalf("ws transport broken: %+v", out.Transport)
	}
}

// TestMKCP_JSONSubscriptionKcpSettings: the Xray subscription format puts the settings
// under streamSettings.kcpSettings, with the header type nested one level deeper in
// header.type.
func TestMKCP_JSONSubscriptionKcpSettings(t *testing.T) {
	body := `[{"outbounds":[{"tag":"kcp-node","protocol":"vless","settings":{"vnext":[{"address":"203.0.113.7","port":443,"users":[{"id":"af815621-b245-4149-89da-dd184cfc4b3d","encryption":"none"}]}]},"streamSettings":{"network":"kcp","security":"none","kcpSettings":{"mtu":1350,"tti":50,"uplinkCapacity":5,"downlinkCapacity":20,"congestion":true,"readBufferSize":2,"writeBufferSize":2,"seed":"secret-seed","header":{"type":"srtp"}}}}],"remarks":"KCP Node"}]`
	entries, err := ParseSubscriptionBody(body)
	if err != nil {
		t.Fatalf("ParseSubscriptionBody: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d", len(entries))
	}
	e := entries[0]
	out := buildProxyOutbound(ProxyConfig{Type: e.Type, IP: e.IP, Port: e.Port, Extra: e.Extra})
	if out.Transport == nil || out.Transport.Type != "mkcp" {
		t.Fatalf("transport = %+v", out.Transport)
	}
	tr := out.Transport
	if tr.Seed != "secret-seed" || tr.HeaderType != "srtp" || tr.MTU != 1350 || tr.TTI != 50 {
		t.Fatalf("kcpSettings lost: %+v", tr)
	}
	if tr.UplinkCapacity != 5 || tr.DownlinkCapacity != 20 || !tr.Congestion {
		t.Fatalf("capacity/congestion lost: %+v", tr)
	}
}

// TestMKCP_JSONSubscriptionKcpSettingsTrojan: the trojan branch of parseJSONOutbound
// repeats the stream-settings parsing independently of vless/vmess (it does not share
// that code), so kcpSettings needs its own parsing there too — a trojan node with
// streamSettings.network "kcp" from a JSON subscription must also carry its seed and
// header type through to the transport.
func TestMKCP_JSONSubscriptionKcpSettingsTrojan(t *testing.T) {
	body := `[{"outbounds":[{"tag":"kcp-trojan","protocol":"trojan","settings":{"servers":[{"address":"203.0.113.7","port":443,"password":"s3cr3t"}]},"streamSettings":{"network":"kcp","security":"none","kcpSettings":{"mtu":1350,"tti":50,"uplinkCapacity":5,"downlinkCapacity":20,"congestion":true,"readBufferSize":2,"writeBufferSize":2,"seed":"secret-seed","header":{"type":"srtp"}}}}],"remarks":"KCP Trojan Node"}]`
	entries, err := ParseSubscriptionBody(body)
	if err != nil {
		t.Fatalf("ParseSubscriptionBody: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d", len(entries))
	}
	e := entries[0]
	if e.Type != "TROJAN" {
		t.Fatalf("type = %s", e.Type)
	}
	out := buildProxyOutbound(ProxyConfig{Type: e.Type, IP: e.IP, Port: e.Port, Extra: e.Extra})
	if out.Transport == nil || out.Transport.Type != "mkcp" {
		t.Fatalf("transport = %+v", out.Transport)
	}
	tr := out.Transport
	if tr.Seed != "secret-seed" || tr.HeaderType != "srtp" || tr.MTU != 1350 {
		t.Fatalf("kcpSettings lost for trojan: %+v", tr)
	}
}
