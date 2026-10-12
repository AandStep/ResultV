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
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"resultproxy-wails/internal/config"
)

// The X25519MLKEM768 key share pushes a Reality ClientHello past one TCP
// segment, and some networks drop the second segment. Servers on Xray-core
// >= v26.9.8, on the other hand, refuse a ClientHello without it. Which of the
// two a node needs is learned per node and kept for the life of the process.
var realityMLKEM = struct {
	sync.Mutex
	m map[string]bool
}{m: map[string]bool{}}

const realityMLKEMConnectBudget = 8 * time.Second

func realityNodeKey(proxy ProxyConfig) string {
	switch strings.ToLower(strings.TrimSpace(proxy.Type)) {
	case "vless", "vmess", "trojan":
	default:
		return ""
	}
	extra := parseExtra(proxy)
	pbk := getStringField(extra, "pbk", getStringField(extra, "publicKey", getStringField(extra, "public_key", "")))
	if pbk == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d|%s|%s", strings.ToLower(proxy.IP), proxy.Port, pbk, normalizeRealityShortID(getStringField(extra, "sid", "")))
}

func knownRealityMLKEM(key string) (need, known bool) {
	realityMLKEM.Lock()
	defer realityMLKEM.Unlock()
	need, known = realityMLKEM.m[key]
	return need, known
}

func rememberRealityMLKEM(key string, need bool) {
	realityMLKEM.Lock()
	realityMLKEM.m[key] = need
	realityMLKEM.Unlock()
}

// withRealityMLKEM applies the learned choice; a node not learned yet goes
// without the key share.
func withRealityMLKEM(proxy ProxyConfig) ProxyConfig {
	if key := realityNodeKey(proxy); key != "" {
		proxy.RealityMLKEM, _ = knownRealityMLKEM(key)
	}
	return proxy
}

// probeRealityMLKEM runs try with the learned choice, or without the key share
// for a node not learned yet, and on failure with the other choice. Going from
// "without" to "with" needs the server to have answered: a timeout is what the
// dropped second segment looks like, and a lucky pass with the key share would
// then pin the node to the variant that network breaks.
func probeRealityMLKEM(proxy ProxyConfig, try func(ProxyConfig) (int64, bool, string)) (ProxyConfig, int64, bool, string) {
	key := realityNodeKey(proxy)
	if key == "" {
		ms, ok, reason := try(proxy)
		return proxy, ms, ok, reason
	}
	first, _ := knownRealityMLKEM(key)
	proxy.RealityMLKEM = first
	ms, ok, reason := try(proxy)
	if ok {
		rememberRealityMLKEM(key, first)
		return proxy, ms, true, ""
	}
	if !first && reason == "timeout" {
		return proxy, 0, false, reason
	}
	proxy.RealityMLKEM = !first
	if ms, ok, _ := try(proxy); ok {
		rememberRealityMLKEM(key, !first)
		return proxy, ms, true, ""
	}
	proxy.RealityMLKEM = first
	return proxy, 0, false, reason
}

// learnRealityMLKEMForConnect settles the choice for a Reality node before its
// session starts. A node already learned is not probed again.
func (m *Manager) learnRealityMLKEMForConnect(ctx context.Context, proxy ProxyConfig) ProxyConfig {
	key := realityNodeKey(proxy)
	if key == "" {
		return proxy
	}
	if need, known := knownRealityMLKEM(key); known {
		proxy.RealityMLKEM = need
		return proxy
	}
	ctx, cancel := context.WithTimeout(ctx, realityMLKEMConnectBudget)
	defer cancel()
	learned, _, ok, _ := probeRealityMLKEM(proxy, func(p ProxyConfig) (int64, bool, string) {
		return pingThroughNodeProbe(ctx, p, http.MethodGet, config.DefaultPingTestURL, "")
	})
	if ok && m.log != nil {
		if learned.RealityMLKEM {
			m.log.Info("[PROXY] Reality: сервер требует X25519MLKEM768")
		} else {
			m.log.Info("[PROXY] Reality: рукопожатие без X25519MLKEM768")
		}
	}
	return learned
}
