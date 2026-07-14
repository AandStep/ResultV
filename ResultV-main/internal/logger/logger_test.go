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

package logger

import (
	"sync"
	"testing"
	"time"
)

func TestLogAndCount(t *testing.T) {
	l := New()
	l.Info("test message 1")
	l.Error("test message 2")
	l.Warning("test message 3")

	if l.Count() != 3 {
		t.Errorf("expected 3 log entries, got %d", l.Count())
	}
}

func TestLogNewestFirst(t *testing.T) {
	l := New()
	l.Info("first")
	l.Info("second")
	l.Info("third")

	entries := l.GetAll()
	if entries[0].Msg != "third" {
		t.Errorf("newest entry should be first, got %q", entries[0].Msg)
	}
	if entries[2].Msg != "first" {
		t.Errorf("oldest entry should be last, got %q", entries[2].Msg)
	}
}

func TestLogCapacity(t *testing.T) {
	l := NewWithCapacity(3)
	l.Info("1")
	l.Info("2")
	l.Info("3")
	l.Info("4") 

	if l.Count() != 3 {
		t.Errorf("expected 3 entries (capacity limit), got %d", l.Count())
	}

	entries := l.GetAll()
	if entries[2].Msg != "2" {
		t.Errorf("oldest entry should be '2' after eviction, got %q", entries[2].Msg)
	}
}

func TestGetLogsPagination(t *testing.T) {
	l := New()
	for i := 0; i < 25; i++ {
		l.Info("msg")
	}

	
	page := l.GetLogs(1, 10)
	if len(page.Items) != 10 {
		t.Errorf("page 1: expected 10 items, got %d", len(page.Items))
	}
	if page.Total != 25 {
		t.Errorf("total: expected 25, got %d", page.Total)
	}
	if page.TotalPages != 3 {
		t.Errorf("totalPages: expected 3, got %d", page.TotalPages)
	}

	
	page3 := l.GetLogs(3, 10)
	if len(page3.Items) != 5 {
		t.Errorf("page 3: expected 5 items, got %d", len(page3.Items))
	}

	
	pageBeyond := l.GetLogs(10, 10)
	if len(pageBeyond.Items) != 0 {
		t.Errorf("page beyond range should be empty, got %d items", len(pageBeyond.Items))
	}
}

func TestLogWithSource(t *testing.T) {
	l := New()
	l.LogWithSource("connected via chrome", TypeInfo, "chrome.exe", "/icons/chrome.ico", "google.com")

	entries := l.GetAll()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Source != "chrome.exe" {
		t.Errorf("Source: got %q, want 'chrome.exe'", entries[0].Source)
	}
	if entries[0].Domain != "google.com" {
		t.Errorf("Domain: got %q, want 'google.com'", entries[0].Domain)
	}
}

func TestEventEmitter(t *testing.T) {
	l := New()

	var received []LogEntry
	var mu sync.Mutex
	done := make(chan struct{}, 1)

	l.SetEmitter(func(eventName string, data any) {
		if eventName != "log" {
			t.Errorf("unexpected event: %q", eventName)
		}
		mu.Lock()
		received = append(received, data.(LogEntry))
		mu.Unlock()
		select {
		case done <- struct{}{}:
		default:
		}
	})

	l.Info("event test")

	// Emission is async — wait for the dedicated emitter goroutine.
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for async emitter")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 emitted event, got %d", len(received))
	}
	if received[0].Msg != "event test" {
		t.Errorf("emitted msg: got %q, want 'event test'", received[0].Msg)
	}
}

// TestEventEmitterNonBlockingUnderBackpressure verifies that the producer
// (Logger.add via Info/Error/etc.) NEVER blocks when the emit queue is full,
// even if the emitter goroutine is wedged. Critical: connection-close paths
// in sing-box hit this code; if it blocks, packet flow stalls and the
// user's browser lags. This is the exact bug we fixed.
func TestEventEmitterNonBlockingUnderBackpressure(t *testing.T) {
	l := New()

	// Block the emitter immediately by making the user-supplied emit
	// function sleep forever. After the first event drains from emitCh,
	// the emitter goroutine is stuck inside this function — all subsequent
	// events must queue up in emitCh, and once it's full they must be
	// dropped, NOT block the producer.
	wedge := make(chan struct{})
	l.SetEmitter(func(_ string, _ any) {
		<-wedge
	})

	// Fire well more than emitQueueSize entries. If add() blocks, this
	// test will hang and the test framework will time it out.
	produced := emitQueueSize * 3
	doneProducing := make(chan struct{})
	go func() {
		for i := 0; i < produced; i++ {
			l.Info("stress")
		}
		close(doneProducing)
	}()

	select {
	case <-doneProducing:
	case <-time.After(2 * time.Second):
		t.Fatal("producer blocked under backpressure — async emit fix regressed")
	}

	if l.DroppedCount() == 0 {
		t.Error("expected drops under backpressure, got 0")
	}

	close(wedge)
}

func TestClear(t *testing.T) {
	l := New()
	l.Info("a")
	l.Info("b")
	l.Clear()

	if l.Count() != 0 {
		t.Errorf("expected 0 entries after clear, got %d", l.Count())
	}
}

func TestConcurrentAccess(t *testing.T) {
	l := New()
	var wg sync.WaitGroup

	
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Info("concurrent")
		}()
	}

	
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.GetLogs(1, 10)
		}()
	}

	wg.Wait()

	if l.Count() != 100 {
		t.Errorf("expected 100 entries after concurrent writes, got %d", l.Count())
	}
}

func TestLogTypes(t *testing.T) {
	l := New()
	l.Info("info msg")
	l.Error("error msg")
	l.Success("success msg")
	l.Warning("warning msg")

	entries := l.GetAll()
	expected := []string{TypeWarning, TypeSuccess, TypeError, TypeInfo}
	for i, e := range entries {
		if e.Type != expected[i] {
			t.Errorf("entry %d: type = %q, want %q", i, e.Type, expected[i])
		}
	}
}
