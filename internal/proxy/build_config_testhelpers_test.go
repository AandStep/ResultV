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

	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
)

func mustBuildTunnelModeConfig(t *testing.T, cfg EngineConfig) SingBoxConfig {
	t.Helper()
	out, err := BuildTunnelModeConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func mustBuildProxyModeConfig(t *testing.T, cfg EngineConfig) SingBoxConfig {
	t.Helper()
	out, err := BuildProxyModeConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// assertCoreAcceptsConfig hands the built config to the real pinned core exactly
// the way startInstance does. sing-box decodes options with DisallowUnknownFields
// and validates enums while decoding, so a field or value the engine does not
// know is not a silently ignored knob — it is a dead engine for every node.
func assertCoreAcceptsConfig(t *testing.T, cfg SingBoxConfig) {
	t.Helper()
	j, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var opts option.Options
	if err := singjson.UnmarshalContext(include.Context(context.Background()), j, &opts); err != nil {
		t.Fatalf("pinned core rejected the config: %v\nconfig: %s", err, j)
	}
}

// outboundFromExtra builds the "proxy" outbound for a node whose extra map is
// `extra`, so a test only has to spell out the knobs under test.
func outboundFromExtra(t *testing.T, proxyType string, extra map[string]interface{}) SBOutbound {
	t.Helper()
	raw, err := json.Marshal(extra)
	if err != nil {
		t.Fatal(err)
	}
	return buildProxyOutbound(ProxyConfig{Type: proxyType, IP: "203.0.113.7", Port: 443, Password: "p", Extra: raw})
}

// tunnelConfigFromExtra builds the whole tunnel-mode config for such a node, for
// tests that need to hand it to assertCoreAcceptsConfig.
func tunnelConfigFromExtra(t *testing.T, proxyType string, extra map[string]interface{}) SingBoxConfig {
	t.Helper()
	raw, err := json.Marshal(extra)
	if err != nil {
		t.Fatal(err)
	}
	return mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy: ProxyConfig{Type: proxyType, IP: "203.0.113.7", Port: 443, Password: "p", Extra: raw},
		Mode:  ProxyModeTunnel,
	})
}
