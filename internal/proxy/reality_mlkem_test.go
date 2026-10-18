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
	"context"
	"encoding/json"
	"testing"
	"time"

	"resultproxy-wails/internal/config"
)

func realityTestNode(t *testing.T, port int) ProxyConfig {
	t.Helper()
	extra, _ := json.Marshal(map[string]string{
		"uuid": "af815621-b245-4149-89da-dd184cfc4b3d", "security": "reality",
		"pbk": "-lCQjfUSO29QFSrFrbIyHPfaSQzjIWRg7lMhaPStUzA", "sid": "f2b30c6723854005",
		"sni": "www.apple.com", "flow": "xtls-rprx-vision", "network": "tcp",
	})
	return ProxyConfig{ID: "r", IP: "203.0.113.7", Port: port, Type: "VLESS", Extra: extra}
}

func resetRealityMLKEM(t *testing.T) {
	t.Helper()
	realityMLKEM.Lock()
	realityMLKEM.m = map[string]bool{}
	realityMLKEM.Unlock()
	t.Cleanup(func() {
		realityMLKEM.Lock()
		realityMLKEM.m = map[string]bool{}
		realityMLKEM.Unlock()
	})
}

type mlkemAttempt struct {
	ok     bool
	reason string
}

func scriptedTries(t *testing.T, script map[bool]mlkemAttempt) (func(ProxyConfig) (int64, bool, string), *[]bool) {
	t.Helper()
	var tried []bool
	return func(p ProxyConfig) (int64, bool, string) {
		tried = append(tried, p.RealityMLKEM)
		a := script[p.RealityMLKEM]
		if a.ok {
			return 10, true, ""
		}
		return 0, false, a.reason
	}, &tried
}

func TestRealityOutboundSendsMLKEMOnlyWhenAsked(t *testing.T) {
	node := realityTestNode(t, 443)
	if out := buildProxyOutbound(node); out.TLS.Reality.SupportX25519MLKEM768 {
		t.Fatal("a node not known to need X25519MLKEM768 must not send it")
	}
	node.RealityMLKEM = true
	if out := buildProxyOutbound(node); !out.TLS.Reality.SupportX25519MLKEM768 {
		t.Fatal("a node that needs X25519MLKEM768 must send it")
	}
}

func TestProbeRealityMLKEMOldServerStaysWithout(t *testing.T) {
	resetRealityMLKEM(t)
	try, tried := scriptedTries(t, map[bool]mlkemAttempt{false: {ok: true}})
	p, _, ok, _ := probeRealityMLKEM(realityTestNode(t, 443), try)
	if !ok || p.RealityMLKEM || len(*tried) != 1 {
		t.Fatalf("ok=%v mlkem=%v tried=%v", ok, p.RealityMLKEM, *tried)
	}
	if need, known := knownRealityMLKEM(realityNodeKey(p)); !known || need {
		t.Fatalf("learned need=%v known=%v", need, known)
	}
}

func TestProbeRealityMLKEMNewServerRejectsWithoutAndIsRemembered(t *testing.T) {
	resetRealityMLKEM(t)
	try, tried := scriptedTries(t, map[bool]mlkemAttempt{false: {reason: "connection_closed"}, true: {ok: true}})
	node := realityTestNode(t, 443)
	p, _, ok, _ := probeRealityMLKEM(node, try)
	if !ok || !p.RealityMLKEM || len(*tried) != 2 {
		t.Fatalf("ok=%v mlkem=%v tried=%v", ok, p.RealityMLKEM, *tried)
	}
	if !withRealityMLKEM(node).RealityMLKEM {
		t.Fatal("the learned choice must apply to the next build of the node")
	}
	try2, tried2 := scriptedTries(t, map[bool]mlkemAttempt{true: {ok: true}})
	if _, _, ok, _ := probeRealityMLKEM(node, try2); !ok || len(*tried2) != 1 || !(*tried2)[0] {
		t.Fatalf("second probe must start with the learned choice: %v", *tried2)
	}
}

func TestProbeRealityMLKEMTimeoutWithoutDoesNotSwitch(t *testing.T) {
	resetRealityMLKEM(t)
	try, tried := scriptedTries(t, map[bool]mlkemAttempt{false: {reason: "timeout"}, true: {ok: true}})
	p, _, ok, reason := probeRealityMLKEM(realityTestNode(t, 443), try)
	if ok || p.RealityMLKEM || len(*tried) != 1 || reason != "timeout" {
		t.Fatalf("ok=%v mlkem=%v tried=%v reason=%q", ok, p.RealityMLKEM, *tried, reason)
	}
	if _, known := knownRealityMLKEM(realityNodeKey(p)); known {
		t.Fatal("nothing proven, nothing to remember")
	}
}

func TestProbeRealityMLKEMLearnedWithFailsFallsBackWithout(t *testing.T) {
	resetRealityMLKEM(t)
	node := realityTestNode(t, 443)
	rememberRealityMLKEM(realityNodeKey(node), true)
	try, tried := scriptedTries(t, map[bool]mlkemAttempt{true: {reason: "timeout"}, false: {ok: true}})
	p, _, ok, _ := probeRealityMLKEM(node, try)
	if !ok || p.RealityMLKEM || len(*tried) != 2 {
		t.Fatalf("ok=%v mlkem=%v tried=%v", ok, p.RealityMLKEM, *tried)
	}
}

func TestProbeRealityMLKEMIgnoresNonRealityNodes(t *testing.T) {
	resetRealityMLKEM(t)
	extra, _ := json.Marshal(map[string]string{"uuid": "af815621-b245-4149-89da-dd184cfc4b3d", "security": "tls", "sni": "example.com"})
	node := ProxyConfig{IP: "203.0.113.7", Port: 443, Type: "VLESS", Extra: extra}
	if realityNodeKey(node) != "" {
		t.Fatal("a TLS node has no Reality choice to learn")
	}
	try, tried := scriptedTries(t, map[bool]mlkemAttempt{false: {reason: "connection_closed"}})
	if _, _, ok, _ := probeRealityMLKEM(node, try); ok || len(*tried) != 1 {
		t.Fatalf("tried=%v", *tried)
	}
}

func TestPingRealityNodeLearnsMLKEM(t *testing.T) {
	resetRealityMLKEM(t)
	old := pingThroughNodeProbe
	t.Cleanup(func() { pingThroughNodeProbe = old })
	var tried []bool
	pingThroughNodeProbe = func(_ context.Context, p ProxyConfig, _, _ string) (int64, bool, string) {
		tried = append(tried, p.RealityMLKEM)
		if p.RealityMLKEM {
			return 42, true, ""
		}
		return 0, false, "connection_closed"
	}
	node := realityTestNode(t, 443)
	entry, _ := json.Marshal(config.ProxyEntry{Name: "r", IP: node.IP, Port: node.Port, Type: node.Type, Extra: node.Extra})
	ms, ok, _, _ := PingNode(string(entry), PingOptions{
		Type: config.PingTypeHTTPGet, TestURL: "https://example.com/x", Timeout: 3 * time.Second,
	}, false)
	if !ok || ms != 42 || len(tried) != 2 {
		t.Fatalf("ok=%v ms=%d tried=%v", ok, ms, tried)
	}
	if !withRealityMLKEM(node).RealityMLKEM {
		t.Fatal("ping must teach the connect path")
	}
}

func TestLearnRealityMLKEMForConnectProbesOnlyUnknownNodes(t *testing.T) {
	resetRealityMLKEM(t)
	old := pingThroughNodeProbe
	t.Cleanup(func() { pingThroughNodeProbe = old })
	calls := 0
	pingThroughNodeProbe = func(_ context.Context, p ProxyConfig, _, _ string) (int64, bool, string) {
		calls++
		if p.RealityMLKEM {
			return 42, true, ""
		}
		return 0, false, "connection_closed"
	}
	node := realityTestNode(t, 443)
	if got := LearnRealityMLKEMForConnect(context.Background(), node); !got.RealityMLKEM || calls != 2 {
		t.Fatalf("mlkem=%v calls=%d", got.RealityMLKEM, calls)
	}
	if got := LearnRealityMLKEMForConnect(context.Background(), node); !got.RealityMLKEM || calls != 2 {
		t.Fatalf("a learned node must not be probed again: mlkem=%v calls=%d", got.RealityMLKEM, calls)
	}
}
