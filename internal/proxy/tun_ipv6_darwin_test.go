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

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

func TestDarwinTunnelRejectsDisabledIPv6BeforeTCPHandshake(t *testing.T) {
	stubHostSupportsIPv6(t, true)
	stubRoutableIPv6(t, false)
	for _, tc := range []struct {
		name        string
		enableIPv6  bool
		destination string
		reject      bool
	}{
		{"IPv6 disabled", false, "[2001:db8::123]:443", true},
		{"IPv6 enabled", true, "[2001:db8::123]:443", false},
		{"IPv4", false, "198.51.100.10:443", false},
		{"private IPv6", false, "[fd00::123]:443", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := ipv6ToggleConfig(tc.enableIPv6)
			cfg.DataDir = t.TempDir()
			raw, err := json.Marshal(mustBuildTunnelModeConfig(t, cfg))
			if err != nil {
				t.Fatal(err)
			}
			ctx := extendedBoxContext(context.Background())
			var opts option.Options
			if err := singjson.UnmarshalContext(ctx, raw, &opts); err != nil {
				t.Fatal(err)
			}
			instance, err := box.New(box.Options{Context: ctx, Options: opts})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = instance.Close() })
			result := instance.Router().PreMatch(adapter.InboundContext{
				Inbound:     "tun-in",
				InboundType: "tun",
				Network:     N.NetworkTCP,
				Source:      M.ParseSocksaddr("[fdfe:dcba:9876::1]:54231"),
				Destination: M.ParseSocksaddr(tc.destination),
			}, nil)
			if rejected := result.Action == adapter.PreMatchReject; rejected != tc.reject {
				t.Fatalf("pre-match action = %v, want reject = %t", result.Action, tc.reject)
			}
		})
	}
}
