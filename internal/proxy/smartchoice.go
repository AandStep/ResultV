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
	"net/netip"

	"resultproxy-wails/internal/verdict"
)

// smartChoice is what the smart outbound does with one connection.
type smartChoice uint8

const (
	// chooseRace means nothing is known about this destination, so both paths
	// are tried and the outcome is written down. It is the only choice that
	// teaches the store anything.
	chooseRace smartChoice = iota
	chooseDirect
	chooseProxy
)

func (c smartChoice) String() string {
	switch c {
	case chooseDirect:
		return "direct"
	case chooseProxy:
		return "proxy"
	default:
		return "race"
	}
}

// smartLookup is the part of verdict.Store the decision needs. Narrow on
// purpose: the tests hand it a real store, and nothing in this file can reach
// for anything wider.
type smartLookup interface {
	Lookup(host string) (verdict.Record, bool)
	LookupIP(addr netip.Addr) (verdict.Record, bool)
}

// decideSmart answers where one connection should go.
//
// The name wins over the address whenever there is one: the address is
// whichever CDN edge answered today, the name is what the user actually asked
// for, and a verdict filed under the name survives the rotation.
//
// raceAllowed is the breaker (spec §6.5). When it is off, an unknown
// destination goes direct — exactly what the client did before this feature —
// rather than being tunnelled on a guess made while the network is broken.
func decideSmart(store smartLookup, host string, addr netip.Addr, raceAllowed bool) smartChoice {
	if store == nil {
		return unknownChoice(raceAllowed)
	}
	if key := verdict.NormalizeHost(host); key != "" {
		if rec, ok := store.Lookup(key); ok {
			return choiceFor(rec.Decision, raceAllowed)
		}
	}
	if addr.IsValid() {
		if rec, ok := store.LookupIP(addr); ok {
			return choiceFor(rec.Decision, raceAllowed)
		}
	}
	return unknownChoice(raceAllowed)
}

func choiceFor(d verdict.Decision, raceAllowed bool) smartChoice {
	switch d {
	case verdict.Proxy:
		return chooseProxy
	case verdict.Direct:
		return chooseDirect
	default:
		return unknownChoice(raceAllowed)
	}
}

func unknownChoice(raceAllowed bool) smartChoice {
	if raceAllowed {
		return chooseRace
	}
	return chooseDirect
}
