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
	"net"
	"testing"
	"time"

	"resultproxy-wails/internal/verdict"
)

func ok200() probeOutcome {
	return probeOutcome{Status: 200, Bytes: 4096, Elapsed: 300 * time.Millisecond}
}

// Measured 2026-09-10: instagram.com answers a Russian address with nothing at
// all — dial tcp 157.240.205.174:443: i/o timeout after ten seconds.
func TestBlackholeDirectMeansBlocked(t *testing.T) {
	direct := probeOutcome{Err: errors.New("dial tcp 157.240.205.174:443: i/o timeout"), Elapsed: 10 * time.Second}
	if got := classifyProbe(direct, ok200()); got != verdict.Proxy {
		t.Fatalf("got %v, want proxy", got)
	}
}

// Measured 2026-09-10: claude.ai answers a Russian address with 302 to
// claude.com/app-unavailable-in-region over a perfectly healthy TCP connection.
func TestRegionRedirectMeansBlocked(t *testing.T) {
	direct := probeOutcome{Status: 302, Location: "https://claude.com/app-unavailable-in-region", Elapsed: 407 * time.Millisecond}
	if got := classifyProbe(direct, ok200()); got != verdict.Proxy {
		t.Fatalf("got %v, want proxy", got)
	}
}

func TestForbiddenAndUnavailableForLegalReasonsMeanBlocked(t *testing.T) {
	for _, status := range []int{403, 451} {
		direct := probeOutcome{Status: status, Elapsed: 200 * time.Millisecond}
		if got := classifyProbe(direct, ok200()); got != verdict.Proxy {
			t.Fatalf("status %d: got %v, want proxy", status, got)
		}
	}
}

func TestHealthyDirectMeansDirect(t *testing.T) {
	if got := classifyProbe(ok200(), ok200()); got != verdict.Direct {
		t.Fatalf("got %v, want direct", got)
	}
}

// If the node is dead too, the probe learned nothing about censorship — it
// learned that the probe is broken. Writing "blocked" here is how a flaky
// minute turns into a week of wrong routing.
func TestBothSidesFailingTeachesNothing(t *testing.T) {
	dead := probeOutcome{Err: errors.New("i/o timeout")}
	if got := classifyProbe(dead, dead); got != verdict.Unknown {
		t.Fatalf("got %v, want unknown", got)
	}
}

// A redirect to the same site is ordinary web plumbing, not a region wall.
func TestOrdinaryRedirectIsNotABlock(t *testing.T) {
	direct := probeOutcome{Status: 301, Location: "https://www.example.com/", Elapsed: 120 * time.Millisecond}
	if got := classifyProbe(direct, ok200()); got != verdict.Direct {
		t.Fatalf("got %v, want direct", got)
	}
}

// The node failing while direct works says the node is having a bad minute.
// It is not evidence about the direct path, so it must not be recorded.
func TestNodeFailureAloneTeachesNothing(t *testing.T) {
	viaNode := probeOutcome{Err: errors.New("connection reset by peer")}
	if got := classifyProbe(ok200(), viaNode); got != verdict.Unknown {
		t.Fatalf("got %v, want unknown", got)
	}
}

func TestProbeDirectDial_DialsGivenAddressAsIs(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	accepted := make(chan struct{}, 1)
	go func() {
		c, aErr := ln.Accept()
		if aErr != nil {
			return
		}
		accepted <- struct{}{}
		c.Close()
	}()

	conn, err := probeDirectDial(context.Background(), "tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("probeDirectDial: %v", err)
	}
	defer conn.Close()
	<-accepted
}

func TestProbeFetch_NoInbound_RefusesNodeHalf(t *testing.T) {
	setProbeInboundPort(0)
	out := probeFetch(context.Background(), "https://example.com/", true)
	if out.Err == nil {
		t.Fatalf("expected error without inbound, got %+v", out)
	}
}
