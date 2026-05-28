// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package filter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sync/errgroup"
)

const (
	rulesetsSubdir = "rulesets"
	minSRSBytes    = 512
)

// RuleSetSource is a sing-box binary rule-set downloaded for network-level ad blocking.
type RuleSetSource struct {
	Tag      string
	FileName string
	URLs     []string
}

// DefaultRuleSetSources mirror internal/proxy/adblock_rules.go (kept here to avoid import cycles).
var DefaultRuleSetSources = []RuleSetSource{
	{
		Tag: "ads", FileName: "ads.srs",
		URLs: []string{
			"https://raw.githubusercontent.com/REIJI007/AdBlock_Rule_For_Sing-box/main/adblock_reject.srs",
		},
	},
	{
		Tag: "ads-ru", FileName: "ads-ru.srs",
		URLs: []string{
			"https://raw.githubusercontent.com/runetfreedom/russia-v2ray-rules-dat/release/sing-box/rule-set-geosite/geosite-category-ads-all.srs",
		},
	},
}

// RuleSetsDir returns the directory where cached .srs files are stored.
func RuleSetsDir(dataDir string) string {
	return filepath.Join(dataDir, "filter", rulesetsSubdir)
}

// RuleSetPaths returns tag -> absolute path for existing local rule-sets.
func RuleSetPaths(dataDir string) map[string]string {
	dir := RuleSetsDir(dataDir)
	out := make(map[string]string, len(DefaultRuleSetSources))
	for _, src := range DefaultRuleSetSources {
		p := filepath.Join(dir, src.FileName)
		if st, err := os.Stat(p); err == nil && st.Size() >= minSRSBytes {
			out[src.Tag] = p
		}
	}
	return out
}

// RuleSetsReady counts how many default rule-sets exist locally.
func RuleSetsReady(dataDir string) (ready, total int) {
	total = len(DefaultRuleSetSources)
	ready = len(RuleSetPaths(dataDir))
	return ready, total
}

// downloadRuleSets fetches all sing-box rule-sets in parallel.
func (m *Manager) downloadRuleSets(ctx context.Context, onProgress func(UpdateProgress)) error {
	dir := RuleSetsDir(m.dataDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	client := newRuleSetHTTPClient()
	if m.meta.RuleSets == nil {
		m.meta.RuleSets = make(map[string]string)
	}

	total := len(DefaultRuleSetSources)
	var (
		mu          sync.Mutex
		errs        []string
		downloaded  int
	)

	g, gctx := errgroup.WithContext(ctx)
	for i, src := range DefaultRuleSetSources {
		i, src := i, src
		g.Go(func() error {
			if onProgress != nil {
				onProgress(UpdateProgress{
					Phase:   "rulesets",
					Current: i + 1,
					Total:   total,
					Item:    src.Tag,
					Message: src.FileName,
				})
			}
			dest := filepath.Join(dir, src.FileName)
			err := downloadSRSFirstOK(gctx, client, src.URLs, dest)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %s", src.Tag, humanizeNetError(err)))
				return nil
			}
			m.meta.RuleSets[src.Tag] = dest
			downloaded++
			return nil
		})
	}
	_ = g.Wait()

	if downloaded == 0 {
		return fmt.Errorf("не удалось загрузить базы блокировки: %s", joinErrors(errs))
	}
	if len(errs) > 0 {
		m.setLastErr(fmt.Errorf("частичное обновление баз (%d/%d): %s",
			downloaded, total, joinErrors(errs)))
	} else {
		m.mu.Lock()
		m.lastErr = ""
		m.mu.Unlock()
	}
	return nil
}

func joinErrors(errs []string) string {
	if len(errs) == 0 {
		return ""
	}
	out := errs[0]
	for i := 1; i < len(errs); i++ {
		out += "; " + errs[i]
	}
	return out
}
