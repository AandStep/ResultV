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
	"testing"
	"time"
)

func TestHealthStartsHealthy(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	h := newDirectHealth(func() time.Time { return now })
	if !h.healthy() {
		t.Fatal("a fresh engine must assume the network works")
	}
}

// Ten failures on one name is one broken name, and tunnelling the rest of the
// internet over it is the bug this guards against.
func TestOneFailingNameDoesNotTripTheBreaker(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	h := newDirectHealth(func() time.Time { return now })
	for i := 0; i < directHealthDistinctFailures*3; i++ {
		h.record("a.blocked.example", false)
	}
	if !h.healthy() {
		t.Fatal("repeated failures on a single name were read as a broken network")
	}
}

// Failures spread across unrelated domains are not censorship — the link is
// down, and anything learned right now would be a lie with a seven-day TTL.
func TestUnrelatedFailuresTripTheBreaker(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	h := newDirectHealth(func() time.Time { return now })
	for _, n := range []string{"one.example", "two.test", "three.invalid", "four.example.org", "five.example.net"} {
		h.record(n, false)
	}
	if h.healthy() {
		t.Fatal("five unrelated names failing did not trip the breaker")
	}
}

func TestBreakerClosesAfterCooldown(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	h := newDirectHealth(func() time.Time { return now })
	for _, n := range []string{"one.example", "two.test", "three.invalid", "four.example.org", "five.example.net"} {
		h.record(n, false)
	}
	now = now.Add(directHealthCooldown + time.Second)
	if !h.healthy() {
		t.Fatal("the breaker never closed")
	}
}

// Subdomains of one site are one site. Counting them separately would trip the
// breaker on a single CDN having a bad minute.
func TestSubdomainsCountAsOneName(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	h := newDirectHealth(func() time.Time { return now })
	for _, n := range []string{"a.site.example", "b.site.example", "c.site.example", "d.site.example", "e.site.example"} {
		h.record(n, false)
	}
	if !h.healthy() {
		t.Fatal("five subdomains of one site were counted as five broken sites")
	}
}

// Evidence goes stale: failures spread over minutes are not one outage.
func TestFailuresOutsideTheWindowAreForgotten(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	h := newDirectHealth(func() time.Time { return now })
	for _, n := range []string{"one.example", "two.test", "three.invalid", "four.example.org"} {
		h.record(n, false)
		now = now.Add(directHealthWindow/2 + time.Second)
	}
	h.record("five.example.net", false)
	if !h.healthy() {
		t.Fatal("failures spread over minutes were treated as one outage")
	}
}

// One success says the link is alive, which is the strongest possible evidence
// against "the network is down".
func TestSuccessClearsTheEvidence(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	h := newDirectHealth(func() time.Time { return now })
	for _, n := range []string{"one.example", "two.test", "three.invalid", "four.example.org"} {
		h.record(n, false)
	}
	h.record("works.example", true)
	h.record("five.example.net", false)
	if !h.healthy() {
		t.Fatal("a working direct connection did not clear the pile of failures")
	}
}
