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
	"testing"
	"time"
)

// A link that comes up on the second poll must be confirmed within ~one
// interval, not after a fixed multi-second sleep. This is the core of the
// connect speedup.
func TestPollProbe_ConfirmsEarly(t *testing.T) {
	calls := 0
	start := time.Now()
	ok, cancelled, reason := pollProbe(context.Background(), 8*time.Second, 50*time.Millisecond, func() (bool, string) {
		calls++
		return calls >= 2, ""
	})
	elapsed := time.Since(start)

	if !ok || cancelled {
		t.Fatalf("expected ok, got ok=%v cancelled=%v reason=%q", ok, cancelled, reason)
	}
	if calls != 2 {
		t.Fatalf("expected 2 attempts, got %d", calls)
	}
	if elapsed > time.Second {
		t.Fatalf("expected early confirmation (~1 interval), took %v", elapsed)
	}
}

// A dead link fails no sooner than the deadline (preserving worst-case
// robustness) and reports the last failure reason — never claims cancelled.
func TestPollProbe_FailsAtDeadline(t *testing.T) {
	start := time.Now()
	ok, cancelled, reason := pollProbe(context.Background(), 300*time.Millisecond, 50*time.Millisecond, func() (bool, string) {
		return false, "still warming up"
	})
	elapsed := time.Since(start)

	if ok || cancelled {
		t.Fatalf("expected failure, got ok=%v cancelled=%v", ok, cancelled)
	}
	if reason != "still warming up" {
		t.Fatalf("expected last reason propagated, got %q", reason)
	}
	if elapsed < 250*time.Millisecond {
		t.Fatalf("expected to use the full deadline budget, only took %v", elapsed)
	}
}

// A cancelled context aborts promptly and is reported as cancelled (so the
// caller surfaces ErrorCode "cancelled", not a probe failure).
func TestPollProbe_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ok, cancelled, _ := pollProbe(ctx, 8*time.Second, 50*time.Millisecond, func() (bool, string) {
		return false, "unreached"
	})
	if ok || !cancelled {
		t.Fatalf("expected cancelled, got ok=%v cancelled=%v", ok, cancelled)
	}
}
