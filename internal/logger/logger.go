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
	"fmt"
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


// Logger holds the last `capacity` entries in a fixed-size circular buffer.
// Writes are O(1) with zero allocations beyond the LogEntry itself — the
// previous implementation prepended each entry into a fresh slice, which
// allocated a new backing array on every call and pushed thousands of
// short-lived objects to the GC per minute under load.
//
// head is the index of the most recent entry; count grows up to capacity
// and then stays pinned. Readers walk newest-first starting from head.
type Logger struct {
	mu       sync.RWMutex
	buf      []LogEntry
	head     int
	count    int
	capacity int
	emit     EventEmitter
}


func New() *Logger {
	return &Logger{
		buf:      make([]LogEntry, defaultCapacity),
		capacity: defaultCapacity,
	}
}


func NewWithCapacity(capacity int) *Logger {
	if capacity < 1 {
		capacity = defaultCapacity
	}
	return &Logger{
		buf:      make([]LogEntry, capacity),
		capacity: capacity,
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



func (l *Logger) SetEmitter(emit EventEmitter) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.emit = emit
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
	emit := l.emit
	l.mu.Unlock()

	if emit != nil {
		emit("log", entry)
	}

	fmt.Printf("[%s] %s\n", entry.Time, msg)
}
