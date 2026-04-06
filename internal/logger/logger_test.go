package logger

import (
	"sync"
	"testing"
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
	l.Info("4") // should evict "1"

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

	// Page 1 of 10.
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

	// Page 3 of 10 — should have 5 items.
	page3 := l.GetLogs(3, 10)
	if len(page3.Items) != 5 {
		t.Errorf("page 3: expected 5 items, got %d", len(page3.Items))
	}

	// Page beyond range.
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

	l.SetEmitter(func(eventName string, data any) {
		if eventName != "log" {
			t.Errorf("unexpected event: %q", eventName)
		}
		mu.Lock()
		received = append(received, data.(LogEntry))
		mu.Unlock()
	})

	l.Info("event test")

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 emitted event, got %d", len(received))
	}
	if received[0].Msg != "event test" {
		t.Errorf("emitted msg: got %q, want 'event test'", received[0].Msg)
	}
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

	// Concurrent writes.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Info("concurrent")
		}()
	}

	// Concurrent reads.
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
