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
	"time"
)

const (
	defaultCapacity = 500

	TypeInfo    = "info"
	TypeError   = "error"
	TypeSuccess = "success"
	TypeWarning = "warning"
)


type LogEntry struct {
	Timestamp int64  `json:"timestamp"`
	Time      string `json:"time"`
	Msg       string `json:"msg"`
	Type      string `json:"type"`    
	Source    string `json:"source"`  
	Icon      string `json:"icon"`    
	Domain    string `json:"domain"`  
}


type LogPage struct {
	Items      []LogEntry `json:"items"`
	Total      int        `json:"total"`
	Page       int        `json:"page"`
	PageSize   int        `json:"pageSize"`
	TotalPages int        `json:"totalPages"`
}



type EventEmitter func(eventName string, data any)


// emitQueueSize bounds how many pending log events can sit between Logger.add
// and the dedicated emitter goroutine. Sized to absorb bursts (browser
// closing many keep-alive connections to many domains as a tab navigates),
// while still being small enough that backpressure manifests as visible
// dropped events instead of unbounded memory growth.
//
// Production observation: under heavy DPI-mediated browsing, `trackedConn.Close`
// can fire 100+ times in a single burst (one closure per keep-alive socket
// per page navigation). Before async emission, each closure synchronously
// blocked on Wails IPC, stalling sing-box's connection-cleanup goroutines
// and causing browser-perceived lag that vanished when the app closed.
const emitQueueSize = 512

// Logger holds the last `capacity` entries in a fixed-size circular buffer.
// Writes are O(1) with zero allocations beyond the LogEntry itself — the
// previous implementation prepended each entry into a fresh slice, which
// allocated a new backing array on every call and pushed thousands of
// short-lived objects to the GC per minute under load.
//
// head is the index of the most recent entry; count grows up to capacity
// and then stays pinned. Readers walk newest-first starting from head.
//
// Events are emitted ASYNCHRONOUSLY through emitCh. A dedicated emitter
// goroutine drains emitCh and calls the user-supplied emitter. If emitCh
// is full (frontend slow / IPC saturated), entries are silently dropped
// instead of blocking the producer. The ring buffer still records the
// entry, so the frontend can fetch it via GetLogs() on next poll.
type Logger struct {
	mu       sync.RWMutex
	buf      []LogEntry
	head     int
	count    int
	capacity int
	emit     EventEmitter

	emitCh   chan LogEntry
	emitOnce sync.Once
	emitWG   sync.WaitGroup
	stopCh   chan struct{}
	dropped  uint64 // count of events dropped due to backpressure; for diagnostics
}


func New() *Logger {
	return newLogger(defaultCapacity)
}


func NewWithCapacity(capacity int) *Logger {
	if capacity < 1 {
		capacity = defaultCapacity
	}
	return newLogger(capacity)
}

func newLogger(capacity int) *Logger {
	return &Logger{
		buf:      make([]LogEntry, capacity),
		capacity: capacity,
		emitCh:   make(chan LogEntry, emitQueueSize),
		stopCh:   make(chan struct{}),
	}
}

// snapshotNewestFirstLocked returns a fresh slice with entries ordered from
// newest to oldest. Caller must hold at least the read lock.
func (l *Logger) snapshotNewestFirstLocked() []LogEntry {
	out := make([]LogEntry, l.count)
	idx := l.head
	for i := 0; i < l.count; i++ {
		out[i] = l.buf[idx]
		idx--
		if idx < 0 {
			idx = l.capacity - 1
		}
	}
	return out
}



// SetEmitter wires the event-sink and starts the dedicated emitter goroutine
// on first call. Safe to call once; subsequent calls update the emitter
// reference but don't spawn a second goroutine.
func (l *Logger) SetEmitter(emit EventEmitter) {
	l.mu.Lock()
	l.emit = emit
	l.mu.Unlock()
	l.emitOnce.Do(func() {
		l.emitWG.Add(1)
		go l.runEmitter()
	})
}

// runEmitter drains emitCh, calling the user-supplied emitter for each
// entry. Runs until stopCh is closed. The emitter runs in its OWN goroutine
// so that a slow Wails IPC channel cannot block sing-box's connection-handling
// goroutines.
func (l *Logger) runEmitter() {
	defer l.emitWG.Done()
	for {
		select {
		case <-l.stopCh:
			return
		case entry, ok := <-l.emitCh:
			if !ok {
				return
			}
			l.mu.RLock()
			emit := l.emit
			l.mu.RUnlock()
			if emit != nil {
				emit("log", entry)
			}
		}
	}
}

// Close stops the emitter goroutine. Idempotent. Used on app shutdown.
func (l *Logger) Close() {
	select {
	case <-l.stopCh:
		// already closed
	default:
		close(l.stopCh)
	}
	l.emitWG.Wait()
}

// DroppedCount returns the number of log events that were dropped due to
// emit-queue backpressure. Useful for diagnostics.
func (l *Logger) DroppedCount() uint64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.dropped
}


func (l *Logger) Log(msg string) {
	l.add(msg, TypeInfo, "", "", "")
}


func (l *Logger) Info(msg string) {
	l.add(msg, TypeInfo, "", "", "")
}


func (l *Logger) Error(msg string) {
	l.add(msg, TypeError, "", "", "")
}


func (l *Logger) Success(msg string) {
	l.add(msg, TypeSuccess, "", "", "")
}


func (l *Logger) Warning(msg string) {
	l.add(msg, TypeWarning, "", "", "")
}


func (l *Logger) LogWithSource(msg, logType, source, icon, domain string) {
	l.add(msg, logType, source, icon, domain)
}


func (l *Logger) GetLogs(page, pageSize int) LogPage {
	l.mu.RLock()
	defer l.mu.RUnlock()

	total := l.count
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}

	totalPages := max((total+pageSize-1)/pageSize, 1)

	start := (page - 1) * pageSize
	if start >= total {
		return LogPage{
			Items:      []LogEntry{},
			Total:      total,
			Page:       page,
			PageSize:   pageSize,
			TotalPages: totalPages,
		}
	}

	end := min(start+pageSize, total)

	// Walk the ring newest-first, skipping the first `start` entries.
	items := make([]LogEntry, end-start)
	idx := l.head - start
	for idx < 0 {
		idx += l.capacity
	}
	for i := range items {
		items[i] = l.buf[idx]
		idx--
		if idx < 0 {
			idx = l.capacity - 1
		}
	}

	return LogPage{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}


func (l *Logger) GetAll() []LogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.snapshotNewestFirstLocked()
}


func (l *Logger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.head = 0
	l.count = 0
	// Zero entries so retained strings can be GC'd. Cheap: defaultCapacity is 500.
	for i := range l.buf {
		l.buf[i] = LogEntry{}
	}
}


func (l *Logger) Count() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.count
}

func (l *Logger) add(msg, logType, source, icon, domain string) {
	now := time.Now()
	entry := LogEntry{
		Timestamp: now.UnixMilli(),
		Time:      now.Format("15:04:05"),
		Msg:       msg,
		Type:      logType,
		Source:    source,
		Icon:      icon,
		Domain:    domain,
	}

	l.mu.Lock()
	if l.count == 0 {
		l.head = 0
	} else {
		l.head = (l.head + 1) % l.capacity
	}
	l.buf[l.head] = entry
	if l.count < l.capacity {
		l.count++
	}
	hasEmitter := l.emit != nil
	l.mu.Unlock()

	if !hasEmitter {
		return
	}

	// Non-blocking send. If the emitter goroutine is keeping up — entry
	// gets emitted to Wails. If the queue is full (frontend or IPC channel
	// can't keep up), DROP the event silently. The producer NEVER blocks.
	//
	// Rationale: dropping a log entry is recoverable (the entry stays in the
	// ring buffer; the frontend polls GetLogs() on demand). Blocking the
	// producer is NOT recoverable — the producer goroutine may be in the
	// middle of closing a TCP connection inside sing-box's data path; if
	// it blocks, packet processing stalls and the user's browser lags.
	select {
	case l.emitCh <- entry:
	default:
		l.mu.Lock()
		l.dropped++
		l.mu.Unlock()
	}
}
