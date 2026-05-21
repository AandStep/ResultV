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


type Logger struct {
	mu       sync.RWMutex
	entries  []LogEntry
	capacity int
	emit     EventEmitter
}


func New() *Logger {
	return &Logger{
		entries:  make([]LogEntry, 0, defaultCapacity),
		capacity: defaultCapacity,
	}
}


func NewWithCapacity(capacity int) *Logger {
	if capacity < 1 {
		capacity = defaultCapacity
	}
	return &Logger{
		entries:  make([]LogEntry, 0, capacity),
		capacity: capacity,
	}
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

	total := len(l.entries)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}

	totalPages := (total + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}

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

	end := start + pageSize
	if end > total {
		end = total
	}

	
	items := make([]LogEntry, end-start)
	copy(items, l.entries[start:end])

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

	result := make([]LogEntry, len(l.entries))
	copy(result, l.entries)
	return result
}


func (l *Logger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = l.entries[:0]
}


func (l *Logger) Count() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.entries)
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
	
	l.entries = append([]LogEntry{entry}, l.entries...)
	if len(l.entries) > l.capacity {
		l.entries = l.entries[:l.capacity]
	}
	emit := l.emit
	l.mu.Unlock()

	
	if emit != nil {
		emit("log", entry)
	}

	
	fmt.Printf("[%s] %s\n", entry.Time, msg)
}
