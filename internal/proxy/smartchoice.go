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

// smartChoice is what the relay does with one connection.
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
// An unknown destination is always raced. The breaker (spec §6.5) used to send
// it direct instead; that handed every unknown name straight to a blackholed
// path at the exact moment hedging was worth most. It now gates learning only —
// see SmartRelay.learn.
func decideSmart(store smartLookup, host string, addr netip.Addr) smartChoice {
	rec, ok := lookupSmart(store, host, addr)
	if !ok {
		return chooseRace
	}
	return choiceFor(rec.Decision)
}

// lookupSmart is decideSmart's first half on its own: the record, and whether
// there was one at all.
//
// Two callers need more than the choice. UDP needs to tell "known to work
// directly" from "nothing is known", which collapse into the same choice. The
// refresh trigger needs the record's age. Both used to ask the store a second
// time for it; one lookup answers all three questions.
func lookupSmart(store smartLookup, host string, addr netip.Addr) (verdict.Record, bool) {
	if store == nil {
		return verdict.Record{}, false
	}
	if key := verdict.NormalizeHost(host); key != "" {
		if rec, ok := store.Lookup(key); ok {
			return rec, true
		}
	}
	if addr.IsValid() {
		if rec, ok := store.LookupIP(addr); ok {
			return rec, true
		}
	}
	return verdict.Record{}, false
}

// choiceFrom is choiceFor for a caller that already has the record, so it does
// not have to ask the store again just to turn it into a choice.
func choiceFrom(rec verdict.Record, known bool) smartChoice {
	if !known {
		return chooseRace
	}
	return choiceFor(rec.Decision)
}

func choiceFor(d verdict.Decision) smartChoice {
	switch d {
	case verdict.Proxy:
		return chooseProxy
	case verdict.Direct:
		return chooseDirect
	default:
		return chooseRace
	}
}
