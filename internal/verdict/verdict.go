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

// Package verdict decides where a destination should go — straight out, or
// through the tunnel — and remembers the answer.
//
// It holds no sockets and knows nothing about sing-box on purpose: everything
// the adaptive Smart mode calls "smart" lives here, so it can be tested as a
// table of inputs and expected answers instead of against a live network.
package verdict

import (
	"net"
	"net/netip"
	"strings"
	"time"
)

// Decision is what the routing layer should do with a destination.
type Decision uint8

const (
	Unknown Decision = iota
	Direct
	Proxy
)

func (d Decision) String() string {
	switch d {
	case Direct:
		return "direct"
	case Proxy:
		return "proxy"
	default:
		return "unknown"
	}
}

// Source ranks where a record came from. Higher wins.
//
// The order is the whole safety model: what the user typed is never overruled
// by a guess, the hand-proven floor is never overruled by the data plane, and
// a fetched list — the thing this whole feature exists to stop trusting — sits
// at the bottom where anything measured can replace it.
type Source uint8

const (
	SourceList Source = iota
	SourceLearned
	SourceFloor
	SourceUser
)

// Record is one stored verdict.
type Record struct {
	Decision Decision  `json:"d"`
	Source   Source    `json:"s"`
	Observed time.Time `json:"o"`
	// ExpiresAt zero means never: user rules, floor entries and list entries
	// are replaced, never aged out.
	ExpiresAt time.Time `json:"e,omitempty"`
}

// TTLs are asymmetric on purpose. A stale "blocked" costs some wasted node
// traffic; a stale "clean" costs a broken site. Different price, different
// lifetime.
const (
	TTLProxy  = 7 * 24 * time.Hour
	TTLDirect = 24 * time.Hour
)

// NormalizeHost lowercases a host and strips the things that are the same
// destination wearing a different coat: a port, brackets around an IPv6
// literal, the root dot, and the leading dot list formats use for suffixes.
func NormalizeHost(raw string) string {
	h := strings.TrimSpace(raw)
	if h == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	h = strings.Trim(h, "[]")
	h = strings.Trim(h, ".")
	return strings.ToLower(h)
}

// Suffixes returns the host and every parent worth holding a verdict on, most
// specific first. A bare TLD is never included: a record on "com" would route
// the entire internet on the strength of three unlucky measurements.
func Suffixes(host string) []string {
	if host == "" {
		return nil
	}
	out := []string{host}
	for parent := ParentDomain(host); parent != ""; parent = ParentDomain(parent) {
		out = append(out, parent)
	}
	return out
}

// ParentDomain drops the leftmost label, returning "" when what remains has no
// dot left — i.e. when the parent would be a bare TLD.
func ParentDomain(host string) string {
	i := strings.IndexByte(host, '.')
	if i < 0 {
		return ""
	}
	parent := host[i+1:]
	if !strings.Contains(parent, ".") {
		return ""
	}
	return parent
}

// IPKey renders one address as a store key. Bare-IP protocols (Telegram's
// MTProto, Discord's voice media) never present a name, so they are keyed by
// address instead.
func IPKey(addr netip.Addr) string {
	if !addr.IsValid() {
		return ""
	}
	return addr.Unmap().String()
}

// IPParent is the aggregate an address is promoted into once enough of its
// neighbours agree: /24 for IPv4, /48 for IPv6.
func IPParent(addr netip.Addr) string {
	if !addr.IsValid() {
		return ""
	}
	a := addr.Unmap()
	bits := 24
	if a.Is6() {
		bits = 48
	}
	prefix, err := a.Prefix(bits)
	if err != nil {
		return ""
	}
	return prefix.String()
}
