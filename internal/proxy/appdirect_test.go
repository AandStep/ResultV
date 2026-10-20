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
	"errors"
	"net/netip"
	"slices"
	"testing"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
)

var (
	nodeRegionAddr = netip.MustParseAddr("203.0.113.10")
	apiAddr        = netip.MustParseAddr("203.0.113.99")
	localAddr      = netip.MustParseAddr("198.51.100.20")
)

// appDirectFixture answers cdn.example.com locally with localAddr, and knows
// that nodeRegionAddr came out of a lookup of that name through the node.
func appDirectFixture() *appDirectOutbound {
	return &appDirectOutbound{
		logger: logger.NOP(),
		lookup: func(_ context.Context, fqdn string) ([]netip.Addr, error) {
			if fqdn == "cdn.example.com" {
				return []netip.Addr{localAddr}, nil
			}
			return nil, errors.New("no such host")
		},
		reverse: func(addr netip.Addr) (string, bool) {
			if addr == nodeRegionAddr {
				return "cdn.example.com", true
			}
			return "", false
		},
	}
}

// The address the app got through the node's resolver is swapped for the one
// the system resolver gives, and kept behind it in case that one is dead.
func TestAppDirect_ResolvesTheLookedUpNameLocally(t *testing.T) {
	md := adapter.InboundContext{Destination: M.SocksaddrFrom(nodeRegionAddr, 443)}
	appDirectFixture().localize(context.Background(), &md, true)
	if !slices.Equal(md.DestinationAddresses, []netip.Addr{localAddr, nodeRegionAddr}) {
		t.Fatalf("addresses %v, want the local answer then the original", md.DestinationAddresses)
	}
}

func TestAppDirect_UDPHasNoFallbackAddress(t *testing.T) {
	md := adapter.InboundContext{Destination: M.SocksaddrFrom(nodeRegionAddr, 443)}
	appDirectFixture().localize(context.Background(), &md, false)
	if !slices.Equal(md.DestinationAddresses, []netip.Addr{localAddr}) {
		t.Fatalf("addresses %v, want the local answer alone", md.DestinationAddresses)
	}
}

func TestAppDirect_ResolvesANameDestination(t *testing.T) {
	md := adapter.InboundContext{Destination: M.ParseSocksaddrHostPort("cdn.example.com", 443)}
	appDirectFixture().localize(context.Background(), &md, true)
	if !slices.Equal(md.DestinationAddresses, []netip.Addr{localAddr}) {
		t.Fatalf("addresses %v", md.DestinationAddresses)
	}
}

func TestAppDirect_LeavesTheConnectionAlone(t *testing.T) {
	cases := []struct {
		name string
		md   adapter.InboundContext
	}{
		// A game server address from a matchmaking API, presented under a
		// shared name: resolving that name would land on another server.
		{"address not from a lookup", adapter.InboundContext{Destination: M.SocksaddrFrom(apiAddr, 443), Domain: "cdn.example.com"}},
		{"local network", adapter.InboundContext{Destination: M.SocksaddrFrom(netip.MustParseAddr("192.168.1.10"), 443)}},
		{"name the system resolver does not know", adapter.InboundContext{Destination: M.ParseSocksaddrHostPort("unknown.example.com", 443)}},
	}
	for _, c := range cases {
		md := c.md
		appDirectFixture().localize(context.Background(), &md, true)
		if md.DestinationAddresses != nil {
			t.Errorf("%s: addresses %v, want none", c.name, md.DestinationAddresses)
		}
	}
}
