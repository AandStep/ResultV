// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package filter

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AdguardTeam/urlfilter/rules"
	"golang.org/x/sync/errgroup"
)

const metaFileName = "filters-meta.json"

type meta struct {
	UpdatedAt int64             `json:"updatedAt"`
	Sources   map[string]string `json:"sources,omitempty"` // list name -> cached file path
}

// Manager loads filter lists and tracks MITM block counters. The MITM
// server lifecycle itself is added in Task 3 (StartMITM/StopMITM).
type Manager struct {
	mu      sync.RWMutex
	dataDir string
	meta    meta
	lastErr string

	networkBlocked  atomic.Uint64
	cosmeticBlocked atomic.Uint64
}

// Status is exposed to the Android UI via the gomobile bind (Task 4).
type Status struct {
	Enabled         bool   `json:"enabled"`
	ListsReady      int    `json:"listsReady"`
	ListsTotal      int    `json:"listsTotal"`
	LastUpdatedUnix int64  `json:"lastUpdatedUnix"`
	LastError       string `json:"lastError,omitempty"`
	NetworkBlocked  uint64 `json:"networkBlocked"`
	CosmeticBlocked uint64 `json:"cosmeticBlocked"`
}

func NewManager(dataDir string) *Manager {
	m := &Manager{dataDir: dataDir}
	_ = m.loadMeta()
	m.pruneMissingSources()
	return m
}

func (m *Manager) FilterDir() string {
	return filepath.Join(m.dataDir, "filter")
}

// pruneMissingSources runs only during NewManager construction, before any
// goroutine or external caller can observe m, so it needs no lock.
func (m *Manager) pruneMissingSources() {
	if len(m.meta.Sources) == 0 {
		return
	}
	for name, path := range m.meta.Sources {
		if st, err := os.Stat(path); err != nil || st.Size() < minFilterBytes {
			delete(m.meta.Sources, name)
		}
	}
}

// loadMeta runs only during NewManager construction, before any goroutine or
// external caller can observe m, so it needs no lock.
func (m *Manager) loadMeta() error {
	path := filepath.Join(m.FilterDir(), metaFileName)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(b, &m.meta)
}

func (m *Manager) saveMeta() error {
	if err := os.MkdirAll(m.FilterDir(), 0o700); err != nil {
		return err
	}
	m.mu.RLock()
	b, err := json.MarshalIndent(m.meta, "", "  ")
	m.mu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(m.FilterDir(), metaFileName), b, 0o600)
}

func (m *Manager) setLastErr(err error) {
	m.mu.Lock()
	if err == nil {
		m.lastErr = ""
	} else {
		m.lastErr = humanizeNetError(err)
	}
	m.mu.Unlock()
}

// Update downloads DefaultSources (EasyList/AdGuard text filters) into
// FilterDir(), populating meta.Sources for FilterPathsMap to consume. This
// is the wiring that ResultV-dev's Manager never actually had — its
// Update() only ever downloaded sing-box SRS rule-sets (a different,
// already-solved concern — see internal/proxy/adblock_rules.go), leaving
// StartMITM permanently unable to find any cached filter.
//
// If every remote source fails, degrades to the embedded fallback list
// instead of returning an error — a dead CDN must never block the browser
// ad-blocker from turning on with at least a minimal rule set.
func (m *Manager) Update(ctx context.Context, onProgress func(UpdateProgress)) error {
	dir := m.FilterDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	client := newFilterHTTPClient()
	m.mu.Lock()
	if m.meta.Sources == nil {
		m.meta.Sources = make(map[string]string)
	}
	m.mu.Unlock()

	total := len(DefaultSources)
	var (
		errs       []string
		downloaded int
	)

	g, gctx := errgroup.WithContext(ctx)
	for i, src := range DefaultSources {
		i, src := i, src
		g.Go(func() error {
			if onProgress != nil {
				onProgress(UpdateProgress{Phase: "lists", Current: i + 1, Total: total, Item: src.Name})
			}
			dest := filepath.Join(dir, src.Name+".txt")
			err := downloadFirstOK(gctx, client, src.URLs, dest)
			m.mu.Lock()
			defer m.mu.Unlock()
			if err != nil {
				errs = append(errs, src.Name+": "+humanizeNetError(err))
				return nil
			}
			m.meta.Sources[src.Name] = dest
			downloaded++
			return nil
		})
	}
	_ = g.Wait()

	if downloaded == 0 {
		fallbackDest := filepath.Join(dir, "fallback.txt")
		if err := writeEmbeddedFallback(fallbackDest); err != nil {
			m.setLastErr(err)
			return err
		}
		m.mu.Lock()
		m.meta.Sources["fallback"] = fallbackDest
		m.mu.Unlock()
		m.setLastErr(nil)
	} else if len(errs) > 0 {
		m.setLastErr(nil) // partial success is not surfaced as an error
	} else {
		m.setLastErr(nil)
	}

	m.mu.Lock()
	m.meta.UpdatedAt = time.Now().Unix()
	m.mu.Unlock()
	if onProgress != nil {
		onProgress(UpdateProgress{Phase: "done", Total: total, Current: total})
	}
	return m.saveMeta()
}

// FilterPathsMap returns urlfilter list-ID -> cached file path, ready to
// pass into mitm.Config.FilterPaths (Task 3).
func (m *Manager) FilterPathsMap() map[rules.ListID]string {
	idMap := map[string]rules.ListID{
		"adguard-base": 1, "adguard-tracking": 2, "adguard-russian": 3,
		"easylist": 4, "easyprivacy": 5, "fanboy-annoyance": 6,
		"fallback": 99,
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[rules.ListID]string, len(m.meta.Sources))
	for name, path := range m.meta.Sources {
		id := idMap[name]
		if id == 0 {
			id = rules.ListID(len(out) + 100)
		}
		out[id] = path
	}
	return out
}

func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return Status{
		ListsReady:      len(m.meta.Sources),
		ListsTotal:      len(DefaultSources),
		LastUpdatedUnix: m.meta.UpdatedAt,
		LastError:       m.lastErr,
		NetworkBlocked:  m.networkBlocked.Load(),
		CosmeticBlocked: m.cosmeticBlocked.Load(),
	}
}
