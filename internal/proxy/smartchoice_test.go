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
	"testing"
	"time"

	"resultproxy-wails/internal/verdict"
)

func choiceStore(t *testing.T) *verdict.Store {
	t.Helper()
	s := verdict.New([]byte("choice-salt"), func() time.Time {
		return time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	})
	s.SetNamespace("test")
	return s
}

func TestDecideSmartFollowsAKnownVerdict(t *testing.T) {
	s := choiceStore(t)
	s.Learn("blocked.example", verdict.Proxy)
	s.Learn("clean.example", verdict.Direct)

	if got := decideSmart(s, "blocked.example", netip.Addr{}, true); got != chooseProxy {
		t.Errorf("blocked name went %v, want proxy", got)
	}
	if got := decideSmart(s, "clean.example", netip.Addr{}, true); got != chooseDirect {
		t.Errorf("clean name went %v, want direct", got)
	}
}

// The race is the only thing that can learn anything, so an unknown name has
// to reach it — that is the whole feature.
func TestDecideSmartRacesAnUnknownName(t *testing.T) {
	s := choiceStore(t)
	if got := decideSmart(s, "never-seen.example", netip.Addr{}, true); got != chooseRace {
		t.Fatalf("unknown name went %v, want race", got)
	}
}

// With the breaker tripped the direct path is presumed broken for reasons that
// have nothing to do with censorship, so nothing may be raced and nothing may
// be learned. Falling back to direct keeps the pre-feature behaviour.
func TestDecideSmartWithoutRaceFallsBackToDirect(t *testing.T) {
	s := choiceStore(t)
	if got := decideSmart(s, "never-seen.example", netip.Addr{}, false); got != chooseDirect {
		t.Fatalf("unknown name went %v with racing off, want direct", got)
	}
}

// A tripped breaker must not throw away what is already known: a name proven
// blocked last week is still blocked while the Wi-Fi is flaky.
func TestDecideSmartKeepsKnownVerdictsWithoutRace(t *testing.T) {
	s := choiceStore(t)
	s.Learn("blocked.example", verdict.Proxy)
	if got := decideSmart(s, "blocked.example", netip.Addr{}, false); got != chooseProxy {
		t.Fatalf("a known verdict was dropped with racing off: %v", got)
	}
}

// Telegram's MTProto and Discord's voice media never present a name.
func TestDecideSmartUsesTheAddressWhenThereIsNoName(t *testing.T) {
	s := choiceStore(t)
	addr := netip.MustParseAddr("149.154.167.51")
	s.LearnIP(addr, verdict.Proxy)
	if got := decideSmart(s, "", addr, true); got != chooseProxy {
		t.Fatalf("bare address went %v, want proxy", got)
	}
}

// A name beats the address it happens to resolve to today: the address is a
// CDN's business and changes under us, the name is what the user asked for.
func TestDecideSmartPrefersTheNameOverTheAddress(t *testing.T) {
	s := choiceStore(t)
	addr := netip.MustParseAddr("203.0.113.9")
	s.Learn("named.example", verdict.Direct)
	s.LearnIP(addr, verdict.Proxy)
	if got := decideSmart(s, "named.example", addr, true); got != chooseDirect {
		t.Fatalf("the address overruled the name: %v", got)
	}
}

func TestDecideSmartWithNoInputAtAll(t *testing.T) {
	s := choiceStore(t)
	if got := decideSmart(s, "", netip.Addr{}, true); got != chooseRace {
		t.Fatalf("got %v, want race", got)
	}
	if got := decideSmart(nil, "x.example", netip.Addr{}, true); got != chooseRace {
		t.Fatalf("a nil store must not panic and must not pretend to know: %v", got)
	}
}
