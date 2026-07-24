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
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AdguardTeam/golibs/log"
	"github.com/AdguardTeam/urlfilter"
	"github.com/AdguardTeam/urlfilter/rules"
	"golang.org/x/sync/errgroup"

	"resultproxy-wails/internal/filter/ca"
	"resultproxy-wails/internal/filter/mitm"
	filterproxy "resultproxy-wails/internal/filter/mitm/vendoredproxy"
)

// buildScriptletIndex is a seam: tests swap it to exercise the degradation
// path when the scriptlet index builder encounters an error (though
// BuildScriptletIndex itself never errors in production today).
var buildScriptletIndex = mitm.BuildScriptletIndex

// buildCosmeticIndex is a seam mirroring buildScriptletIndex, for the same
// test-degradation reason.
var buildCosmeticIndex = mitm.BuildCosmeticIndex

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
	mitm    *mitm.Server

	// Cached urlfilter engine, reused across MITM restarts (StartMITM is called
	// on every connect). Parsing the filter lists is the expensive part; the key
	// invalidates it when Update() rewrites a list. Own mutex so a rebuild never
	// blocks the meta/status locks.
	engineMu  sync.Mutex
	engine    *urlfilter.Engine
	engineKey string

	// Cached scriptlet index, mirroring engine/engineKey above. Re-scanning
	// ~19MB of lists per connect would regress exactly what the engine cache
	// protects, so the index follows the same pattern.
	scriptletIndexMu  sync.Mutex
	scriptletIndex    *filterproxy.ScriptletIndex
	scriptletIndexKey string

	// Cached cosmetic index, mirroring scriptletIndex above.
	cosmeticIndexMu  sync.Mutex
	cosmeticIndex    *filterproxy.CosmeticIndex
	cosmeticIndexKey string

	networkBlocked  atomic.Uint64
	cosmeticBlocked atomic.Uint64

	// caSeed, when non-empty, makes root CA generation deterministic so a
	// reinstalled app recreates the exact CA already trusted by the system.
	// Guarded by mu. Set once at startup via SetCASeed before any CA access.
	caSeed string
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
		"adguard-annoyances": 7, "adguard-mobile-ads": 8,
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
		Enabled:         m.mitm != nil,
		ListsReady:      len(m.meta.Sources),
		ListsTotal:      len(DefaultSources),
		LastUpdatedUnix: m.meta.UpdatedAt,
		LastError:       m.lastErr,
		NetworkBlocked:  m.networkBlocked.Load(),
		CosmeticBlocked: m.cosmeticBlocked.Load(),
	}
}

// StartMITM runs the local HTTPS filtering proxy on 127.0.0.1:listenPort.
// Fails immediately if no filter list has been downloaded yet — Android's
// caller (Task 4/5) must call Update() at least once before this.
//
// upstreamDial, when non-nil, is used for every connection the proxy opens to
// an origin server. On Android the caller passes a SOCKS5 dialer aimed at the
// engine's loopback inbound so filtered browser traffic re-enters the tunnel
// (the app process is excluded from its own VPN). Nil = direct dial (desktop).
func (m *Manager) StartMITM(listenPort int, upstreamDial func(network, addr string) (net.Conn, error)) error {
	paths, err := m.mitmFilterPaths()
	if err != nil {
		return err
	}
	// Stop any already-running proxy before starting the new one so a
	// repeat call (e.g. Task 7's watchdog restarting on the fixed port
	// 8130) rebinds cleanly instead of leaking the previous server's
	// listening socket and goroutines. StopMITM self-locks m.mu and safely
	// no-ops when nothing is running, so it must be called outside any m.mu
	// critical section here.
	m.StopMITM()
	// Use the same deterministic seed as CARootPath so a fresh install that
	// reaches StartMITM before the cert wizard still materializes the
	// reproducible CA (empty seed here would write a random CA to disk, which
	// every later EnsureRoot then reloads — defeating reinstall reproduction).
	m.mu.RLock()
	seed := m.caSeed
	m.mu.RUnlock()
	root, err := ca.EnsureRoot(m.FilterDir(), seed)
	if err != nil {
		return err
	}
	// Reuse the cached engine when the filter files are unchanged so a repeat
	// StartMITM (every connect) skips the multi-second list parse.
	engine, err := m.cachedEngine(paths)
	if err != nil {
		return err
	}
	// Reuse the cached scriptlet index for the same reason — see
	// cachedScriptletIndex. Unlike the engine (load-bearing), a scriptlet index
	// build failure degrades gracefully: we log it and continue with a nil index
	// (scriptlets disabled). The proxy is still functional and serves traffic
	// without the best-effort scriptlet injection enhancement.
	scriptletIndex, err := m.cachedScriptletIndex(paths)
	if err != nil {
		log.Error("failed to build scriptlet index, scriptlets disabled: %v", err)
		scriptletIndex = nil
	}
	// Same graceful-degradation contract as the scriptlet index: a build
	// failure disables the subdomain/ExtCSS supplement but keeps the proxy up.
	cosmeticIndex, err := m.cachedCosmeticIndex(paths)
	if err != nil {
		log.Error("failed to build cosmetic index, subdomain/ExtCSS cosmetics disabled: %v", err)
		cosmeticIndex = nil
	}
	srv, err := mitm.NewServer(mitm.Config{
		ListenPort:     listenPort,
		RootCert:       root.Certificate,
		RootKey:        root.PrivateKey,
		FilterPaths:    paths,
		Engine:         engine,
		ScriptletIndex: scriptletIndex,
		CosmeticIndex:  cosmeticIndex,
		UpstreamDial:   upstreamDial,
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
	m.mu.Unlock()
	return nil
}

// cachedEngine returns a urlfilter engine for the given filter files, reusing
// the last-built one when the file set is unchanged. Parsing the lists is the
// expensive part of starting the MITM proxy; caching it keeps the post-connect
// attach fast so the "browser briefly unfiltered" window stays short.
func (m *Manager) cachedEngine(paths map[rules.ListID]string) (*urlfilter.Engine, error) {
	key := engineCacheKey(paths)
	m.engineMu.Lock()
	defer m.engineMu.Unlock()
	if m.engine != nil && m.engineKey == key {
		return m.engine, nil
	}
	eng, err := mitm.BuildEngine(paths)
	if err != nil {
		return nil, err
	}
	m.engine = eng
	m.engineKey = key
	return eng, nil
}

// cachedScriptletIndex returns a ScriptletIndex for the given filter files,
// reusing the last-built one when the file set is unchanged. Mirrors
// cachedEngine: re-scanning ~19MB of lists per connect would regress exactly
// what that cache protects, so the index follows the same cache-key and
// invalidation approach.
func (m *Manager) cachedScriptletIndex(paths map[rules.ListID]string) (*filterproxy.ScriptletIndex, error) {
	key := engineCacheKey(paths)
	m.scriptletIndexMu.Lock()
	defer m.scriptletIndexMu.Unlock()
	if m.scriptletIndex != nil && m.scriptletIndexKey == key {
		return m.scriptletIndex, nil
	}
	ix, err := buildScriptletIndex(paths)
	if err != nil {
		return nil, err
	}
	m.scriptletIndex = ix
	m.scriptletIndexKey = key
	return ix, nil
}

// cachedCosmeticIndex returns a CosmeticIndex for the given filter files,
// reusing the last-built one when the file set is unchanged (mirrors
// cachedScriptletIndex).
func (m *Manager) cachedCosmeticIndex(paths map[rules.ListID]string) (*filterproxy.CosmeticIndex, error) {
	key := engineCacheKey(paths)
	m.cosmeticIndexMu.Lock()
	defer m.cosmeticIndexMu.Unlock()
	if m.cosmeticIndex != nil && m.cosmeticIndexKey == key {
		return m.cosmeticIndex, nil
	}
	ix, err := buildCosmeticIndex(paths)
	if err != nil {
		return nil, err
	}
	m.cosmeticIndex = ix
	m.cosmeticIndexKey = key
	return ix, nil
}

// engineCacheKey fingerprints the filter file set (id + path + size + mtime) so
// the cached engine is invalidated whenever Update() rewrites a list.
func engineCacheKey(paths map[rules.ListID]string) string {
	ids := make([]int, 0, len(paths))
	for id := range paths {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	var b strings.Builder
	for _, id := range ids {
		p := paths[rules.ListID(id)]
		fmt.Fprintf(&b, "%d:%s:", id, p)
		if st, err := os.Stat(p); err == nil {
			fmt.Fprintf(&b, "%d:%d", st.Size(), st.ModTime().UnixNano())
		}
		b.WriteByte('|')
	}
	return b.String()
}

func (m *Manager) StopMITM() {
	m.mu.Lock()
	if m.mitm != nil {
		m.mitm.Close()
		m.mitm = nil
	}
	m.mu.Unlock()
}

func (m *Manager) IsMITMRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mitm != nil
}

// SetCASeed records the stable device seed used for deterministic CA
// generation. Call before the first CARootPath()/StartMITM() so the CA is
// created deterministically rather than randomly.
func (m *Manager) SetCASeed(seed string) {
	m.mu.Lock()
	m.caSeed = seed
	m.mu.Unlock()
}

// CARootPath returns the path to the (PEM-encoded) root CA certificate so
// the Android side can read it and hand it to KeyChain.createInstallIntent.
func (m *Manager) CARootPath() (string, error) {
	m.mu.RLock()
	seed := m.caSeed
	m.mu.RUnlock()
	root, err := ca.EnsureRoot(m.FilterDir(), seed)
	if err != nil {
		return "", err
	}
	return root.CertificatePath, nil
}
