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

func appDirectWithLookup(answers map[string][]netip.Addr) (*appDirectOutbound, *[]string) {
	var asked []string
	return &appDirectOutbound{
		logger: logger.NOP(),
		lookup: func(_ context.Context, fqdn string) ([]netip.Addr, error) {
			asked = append(asked, fqdn)
			if a, ok := answers[fqdn]; ok {
				return a, nil
			}
			return nil, errors.New("no such host")
		},
	}, &asked
}

var (
	nodeRegionAddr = netip.MustParseAddr("203.0.113.10")
	localAddr      = netip.MustParseAddr("198.51.100.20")
)

// The address the app got through the node's resolver is swapped for the one
// the system resolver gives, and kept behind it in case that one is dead.
func TestAppDirect_ResolvesTheNameLocally(t *testing.T) {
	a, _ := appDirectWithLookup(map[string][]netip.Addr{"cdn.example.com": {localAddr}})
	md := adapter.InboundContext{Destination: M.SocksaddrFrom(nodeRegionAddr, 443), Domain: "cdn.example.com"}
	a.localize(context.Background(), &md, true)
	if !slices.Equal(md.DestinationAddresses, []netip.Addr{localAddr, nodeRegionAddr}) {
		t.Fatalf("addresses %v, want the local answer then the original", md.DestinationAddresses)
	}
}

func TestAppDirect_UDPHasNoFallbackAddress(t *testing.T) {
	a, _ := appDirectWithLookup(map[string][]netip.Addr{"cdn.example.com": {localAddr}})
	md := adapter.InboundContext{Destination: M.SocksaddrFrom(nodeRegionAddr, 443), Domain: "cdn.example.com"}
	a.localize(context.Background(), &md, false)
	if !slices.Equal(md.DestinationAddresses, []netip.Addr{localAddr}) {
		t.Fatalf("addresses %v, want the local answer alone", md.DestinationAddresses)
	}
}

func TestAppDirect_SameAnswerIsNotDuplicated(t *testing.T) {
	a, _ := appDirectWithLookup(map[string][]netip.Addr{"cdn.example.com": {nodeRegionAddr}})
	md := adapter.InboundContext{Destination: M.SocksaddrFrom(nodeRegionAddr, 443), Domain: "cdn.example.com"}
	a.localize(context.Background(), &md, true)
	if !slices.Equal(md.DestinationAddresses, []netip.Addr{nodeRegionAddr}) {
		t.Fatalf("addresses %v", md.DestinationAddresses)
	}
}

// Under FakeIP the destination already is the name.
func TestAppDirect_ResolvesAFakeIPName(t *testing.T) {
	a, _ := appDirectWithLookup(map[string][]netip.Addr{"cdn.example.com": {localAddr}})
	md := adapter.InboundContext{Destination: M.ParseSocksaddrHostPort("cdn.example.com", 443)}
	a.localize(context.Background(), &md, true)
	if !slices.Equal(md.DestinationAddresses, []netip.Addr{localAddr}) {
		t.Fatalf("addresses %v", md.DestinationAddresses)
	}
}

func TestAppDirect_LeavesTheConnectionAlone(t *testing.T) {
	cases := []struct {
		name string
		md   adapter.InboundContext
	}{
		{"no name", adapter.InboundContext{Destination: M.SocksaddrFrom(nodeRegionAddr, 27015)}},
		{"local network", adapter.InboundContext{Destination: M.SocksaddrFrom(netip.MustParseAddr("192.168.1.10"), 443), Domain: "nas.example.com"}},
		{"name the system resolver does not know", adapter.InboundContext{Destination: M.SocksaddrFrom(nodeRegionAddr, 443), Domain: "unknown.example.com"}},
	}
	for _, c := range cases {
		a, _ := appDirectWithLookup(map[string][]netip.Addr{"nas.example.com": {localAddr}})
		md := c.md
		a.localize(context.Background(), &md, true)
		if md.DestinationAddresses != nil {
			t.Errorf("%s: addresses %v, want none", c.name, md.DestinationAddresses)
		}
	}
}
