// Copyright (C) 2026 ResultProxy
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

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

// LogEntry is the structured log record sent to the frontend.
type LogEntry struct {
	Timestamp int64  `json:"timestamp"`
	Time      string `json:"time"`
	Msg       string `json:"msg"`
	Type      string `json:"type"`    // info, error, success, warning
	Source    string `json:"source"`  // "chrome.exe", "telegram.exe", "youtube.com"
	Icon      string `json:"icon"`    // URL or path for icon
	Domain    string `json:"domain"`  // target domain for search
}

// LogPage is the paginated response for the frontend.
type LogPage struct {
	Items      []LogEntry `json:"items"`
	Total      int        `json:"total"`
	Page       int        `json:"page"`
	PageSize   int        `json:"pageSize"`
	TotalPages int        `json:"totalPages"`
}

// EventEmitter is a callback for pushing log events to the frontend.
// In production, this wraps runtime.EventsEmit.
type EventEmitter func(eventName string, data any)

// Logger is a thread-safe ring-buffer logger with push notifications.
type Logger struct {
	mu       sync.RWMutex
	entries  []LogEntry
	capacity int
	emit     EventEmitter
}

// New creates a Logger with default capacity (500).
func New() *Logger {
	return &Logger{
		entries:  make([]LogEntry, 0, defaultCapacity),
		capacity: defaultCapacity,
	}
}

// NewWithCapacity creates a Logger with a custom capacity.
func NewWithCapacity(capacity int) *Logger {
	if capacity < 1 {
		capacity = defaultCapacity
	}
	return &Logger{
		entries:  make([]LogEntry, 0, capacity),
		capacity: capacity,
	}
}

// SetEmitter sets the event emitter for push notifications.
// Should be called after Wails startup when runtime is available.
func (l *Logger) SetEmitter(emit EventEmitter) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.emit = emit
}

// Log adds a log entry with default "info" type.
func (l *Logger) Log(msg string) {
	l.add(msg, TypeInfo, "", "", "")
}

// Info adds an info log entry.
func (l *Logger) Info(msg string) {
	l.add(msg, TypeInfo, "", "", "")
}

// Error adds an error log entry.
func (l *Logger) Error(msg string) {
	l.add(msg, TypeError, "", "", "")
}

// Success adds a success log entry.
func (l *Logger) Success(msg string) {
	l.add(msg, TypeSuccess, "", "", "")
}

// Warning adds a warning log entry.
func (l *Logger) Warning(msg string) {
	l.add(msg, TypeWarning, "", "", "")
}

// LogWithSource adds a log entry with source and domain context.
func (l *Logger) LogWithSource(msg, logType, source, icon, domain string) {
	l.add(msg, logType, source, icon, domain)
}

// GetLogs returns a paginated slice of logs (newest first).
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

	// Copy to avoid data races on the underlying slice.
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

// GetAll returns all log entries (newest first).
func (l *Logger) GetAll() []LogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]LogEntry, len(l.entries))
	copy(result, l.entries)
	return result
}

// Clear removes all log entries.
func (l *Logger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = l.entries[:0]
}

// Count returns the number of log entries.
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
	// Prepend (newest first) — matches Node.js behavior.
	l.entries = append([]LogEntry{entry}, l.entries...)
	if len(l.entries) > l.capacity {
		l.entries = l.entries[:l.capacity]
	}
	emit := l.emit
	l.mu.Unlock()

	// Push event outside the lock to avoid deadlocks.
	if emit != nil {
		emit("log", entry)
	}

	// Also print to stdout for debugging.
	fmt.Printf("[%s] %s\n", entry.Time, msg)
}
