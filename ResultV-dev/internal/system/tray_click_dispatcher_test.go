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

package system

import (
	"sync"
	"testing"
	"time"
)

func TestTrayClickDispatcher_RebuildDoesNotAccumulateHandlers(t *testing.T) {
	dispatcher := newTrayClickDispatcher()
	defer dispatcher.stop()

	var mu sync.Mutex
	clicks := 0
	dispatcher.start(func(string) {
		mu.Lock()
		clicks++
		mu.Unlock()
	})

	oldChannels := make([]chan struct{}, 0, 64)
	for i := 0; i < 64; i++ {
		ch := make(chan struct{}, 1)
		dispatcher.update([]serverClickBinding{{proxyID: "proxy", ch: ch}})
		oldChannels = append(oldChannels, ch)
	}
	time.Sleep(40 * time.Millisecond)

	for i := 0; i < len(oldChannels)-1; i++ {
		oldChannels[i] <- struct{}{}
	}
	time.Sleep(80 * time.Millisecond)

	mu.Lock()
	oldClicks := clicks
	mu.Unlock()
	if oldClicks != 0 {
		t.Fatalf("expected 0 clicks from stale channels, got %d", oldClicks)
	}

	oldChannels[len(oldChannels)-1] <- struct{}{}
	deadline := time.After(500 * time.Millisecond)
	for {
		mu.Lock()
		got := clicks
		mu.Unlock()
		if got == 1 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("expected exactly one click from active channel, got %d", got)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}
