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
	"testing"
	"time"

	"resultproxy-wails/internal/logger"
)

// Measured 14.09.2026: a WireGuard session's Close took about two minutes to
// return, and until it did, the still-open instance held the bbolt lock on
// sing-box-cache.db — cache-file is the LAST thing box.Close shuts down, long
// after the endpoint it is waiting on. Every connect attempt in that window
// spent ten seconds inside bbolt.Open (ten retries, one second each) and then
// failed with "initialize cache-file: timeout", which names neither the cause
// nor the cure. Three attempts in a row failed that way before the lock
// cleared on its own.
func TestAwaitPendingCloseWaitsForTheLockToClear(t *testing.T) {
	pending := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		close(pending)
	}()
	if err := awaitPendingClose(context.Background(), pending, time.Now(), time.Second, logger.New()); err != nil {
		t.Fatalf("awaitPendingClose: %v", err)
	}
}

// Nothing pending is the normal case and must cost nothing.
func TestAwaitPendingCloseReturnsImmediatelyWhenNothingPending(t *testing.T) {
	started := time.Now()
	if err := awaitPendingClose(context.Background(), nil, time.Time{}, time.Minute, logger.New()); err != nil {
		t.Fatalf("awaitPendingClose: %v", err)
	}
	if waited := time.Since(started); waited > 100*time.Millisecond {
		t.Fatalf("waited %s with no pending close", waited)
	}
}

// A cancelled connect must not sit in the wait: Disconnect cancels the connect
// context before it reaches the engine, and that is what lets the disconnect
// button work while a previous session is still unwinding.
func TestAwaitPendingCloseHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := awaitPendingClose(ctx, make(chan struct{}), time.Now(), time.Minute, logger.New())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("awaitPendingClose = %v, want context.Canceled", err)
	}
}

// When the wait runs out, the error has to say what is happening — the whole
// point of the change is that the user stops seeing "initialize cache-file:
// timeout" for a previous session that has not let go yet.
func TestAwaitPendingCloseExplainsTheTimeout(t *testing.T) {
	err := awaitPendingClose(context.Background(), make(chan struct{}), time.Now(), 20*time.Millisecond, logger.New())
	if err == nil {
		t.Fatal("awaitPendingClose returned nil, want a timeout error")
	}
	if !errors.Is(err, errPreviousSessionClosing) {
		t.Fatalf("awaitPendingClose = %v, want errPreviousSessionClosing", err)
	}
}
