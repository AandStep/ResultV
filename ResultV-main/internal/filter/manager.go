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
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AdguardTeam/urlfilter/rules"

	"resultproxy-wails/internal/filter/ca"
	"resultproxy-wails/internal/filter/mitm"
)

const (
	metaFileName     = "filters-meta.json"
	defaultUpdateGap = 24 * time.Hour
	maxListBytes     = 32 * 1024 * 1024
)

type meta struct {
	UpdatedAt int64             `json:"updatedAt"`
	Sources   map[string]string `json:"sources,omitempty"`   // MITM text lists (optional)
	RuleSets  map[string]string `json:"ruleSets,omitempty"` // tag -> .srs path
}

// Manager loads filter lists, builds urlfilter engine, and runs the MITM proxy.
type Manager struct {
	mu      sync.RWMutex
	dataDir string
	meta    meta
	mitm    *mitm.Server

	networkBlocked  atomic.Uint64
	cosmeticBlocked atomic.Uint64
	lastErr         string
	enabled         bool

	updateMu         sync.Mutex
	updateInProgress bool
	updateProgress   UpdateProgress
}

func NewManager(dataDir string) *Manager {
	return &Manager{dataDir: dataDir}
}

func (m *Manager) DataDir() string { return m.dataDir }

func (m *Manager) FilterDir() string {
	return filepath.Join(m.dataDir, "filter")
}

func (m *Manager) LoadCache() error {
	if err := m.loadMeta(); err != nil {
		return err
	}
	m.pruneMissingSources()
	return nil
}

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
	b, err := json.MarshalIndent(m.meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(m.FilterDir(), metaFileName), b, 0o600)
}

// NeedsUpdate reports whether rule-sets should be refreshed.
func (m *Manager) NeedsUpdate() bool {
	ready, total := RuleSetsReady(m.dataDir)
	if ready < total {
		return true
	}
	if m.meta.UpdatedAt == 0 {
		return true
	}
	return time.Since(time.Unix(m.meta.UpdatedAt, 0)) > defaultUpdateGap
}

func (m *Manager) beginUpdate(onProgress func(UpdateProgress)) {
	m.updateMu.Lock()
	m.updateInProgress = true
	m.updateProgress = UpdateProgress{Phase: "rulesets", Current: 0, Total: len(DefaultRuleSetSources)}
	m.updateMu.Unlock()
	if onProgress != nil {
		onProgress(m.updateProgress)
	}
}

func (m *Manager) setUpdateProgress(p UpdateProgress, onProgress func(UpdateProgress)) {
	m.updateMu.Lock()
	m.updateProgress = p
	m.updateMu.Unlock()
	if onProgress != nil {
		onProgress(p)
	}
}

func (m *Manager) endUpdate() {
	m.updateMu.Lock()
	m.updateInProgress = false
	m.updateProgress = UpdateProgress{}
	m.updateMu.Unlock()
}

func (m *Manager) updateProgressSnapshot() UpdateProgress {
	m.updateMu.Lock()
	defer m.updateMu.Unlock()
	return m.updateProgress
}

func (m *Manager) isUpdateInProgress() bool {
	m.updateMu.Lock()
	defer m.updateMu.Unlock()
	return m.updateInProgress
}

// Update downloads sing-box rule-sets used for network ad blocking (fast, parallel).
// Large MITM text lists are not downloaded here — they are only needed for HTTPS page filtering.
func (m *Manager) Update(ctx context.Context, onProgress func(UpdateProgress)) error {
	m.beginUpdate(onProgress)
	defer m.endUpdate()

	if err := os.MkdirAll(m.FilterDir(), 0o700); err != nil {
		return err
	}

	if err := m.downloadRuleSets(ctx, onProgress); err != nil {
		m.setLastErr(err)
		if onProgress != nil {
			onProgress(UpdateProgress{Phase: "error", Message: humanizeNetError(err)})
		}
		return err
	}

	m.meta.UpdatedAt = time.Now().Unix()
	if onProgress != nil {
		onProgress(UpdateProgress{Phase: "done", Total: len(DefaultRuleSetSources), Current: len(DefaultRuleSetSources)})
	}
	return m.saveMeta()
}

func (m *Manager) filterPathsMap() map[rules.ListID]string {
	idMap := map[string]rules.ListID{
		"adguard-base": 1, "adguard-tracking": 2, "adguard-russian": 3,
		"easylist": 4, "easyprivacy": 5, "fanboy-annoyance": 6,
	}
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

func (m *Manager) setLastErr(err error) {
	m.mu.Lock()
	m.lastErr = humanizeNetError(err)
	m.mu.Unlock()
}

// StartMITM runs the local HTTPS filtering proxy; upstreamPort is sing-box mixed inbound.
func (m *Manager) StartMITM(listenPort, upstreamPort int) error {
	paths := m.filterPathsMap()
	if len(paths) == 0 {
		return fmt.Errorf("no cached filters; update filters first")
	}
	root, err := ca.EnsureRoot(m.FilterDir())
	if err != nil {
		return err
	}
	srv, err := mitm.NewServer(mitm.Config{
		ListenPort:   listenPort,
		UpstreamPort: upstreamPort,
		RootCert:     root.Certificate,
		RootKey:      root.PrivateKey,
		FilterPaths:  paths,
		OnBlocked: func(cosmetic bool) {
			if cosmetic {
				m.cosmeticBlocked.Add(1)
			} else {
				m.networkBlocked.Add(1)
			}
		},
	})
	if err != nil {
		return err
	}
	if err := srv.Start(); err != nil {
		return err
	}
	m.mu.Lock()
	m.mitm = srv
	m.enabled = true
	m.mu.Unlock()
	return nil
}

func (m *Manager) StopMITM() {
	m.mu.Lock()
	if m.mitm != nil {
		m.mitm.Close()
		m.mitm = nil
	}
	m.enabled = false
	m.mu.Unlock()
}

func (m *Manager) IsMITMRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mitm != nil
}

func (m *Manager) Status(adBlockEnabled, connected, engineAdBlock bool) Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ready, total := RuleSetsReady(m.dataDir)
	prog := m.updateProgressSnapshot()
	st := Status{
		Enabled:            adBlockEnabled,
		FilterCount:        len(m.meta.Sources),
		RuleSetsReady:      ready,
		RuleSetsTotal:      total,
		LastUpdatedUnix:    m.meta.UpdatedAt,
		LastError:          m.lastErr,
		CAInstalled:        ca.IsInstalled(),
		NetworkBlocked:     m.networkBlocked.Load(),
		CosmeticBlocked:    m.cosmeticBlocked.Load(),
		UpdateInProgress:   m.isUpdateInProgress(),
		UpdatePhase:        prog.Phase,
		UpdateCurrent:      prog.Current,
		UpdateTotal:        prog.Total,
		UpdateItem:         prog.Item,
		NetworkBlockActive: connected && engineAdBlock,
		NeedsReconnect:     connected && adBlockEnabled != engineAdBlock,
	}
	return st
}

// InstallCA installs the generated root certificate into the system trust store.
func (m *Manager) InstallCA() error {
	root, err := ca.EnsureRoot(m.FilterDir())
	if err != nil {
		return err
	}
	return ca.Install(root.CertificatePath)
}

func (m *Manager) UninstallCA() error {
	return ca.Uninstall()
}

// CAInstalled reports whether the root CA is in the system trust store.
func CAInstalled() bool {
	return ca.IsInstalled()
}

func (m *Manager) CARootPath() (string, error) {
	root, err := ca.EnsureRoot(m.FilterDir())
	if err != nil {
		return "", err
	}
	return root.CertificatePath, nil
}
