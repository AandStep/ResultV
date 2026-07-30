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

package proxy

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sagernet/sing-box/common/srs"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
)

const (
	// smartRuleSetTag is the route rule_set tag the Smart-mode block-list rule
	// references.
	smartRuleSetTag = "smart-blocked"
	// smartRuleSetSubdir keeps compiled lists next to the other routing state.
	smartRuleSetSubdir = "routing"
	smartRuleSetPrefix = "smart-"
	smartRuleSetExt    = ".srs"
	// smartRuleSetSchema is folded into the cache key: bump it whenever the rule
	// we compile changes shape, so every cached file is invalidated.
	smartRuleSetSchema = "v1"
	// srsBinaryVersion is the sing-box rule-set binary format we emit — the same
	// version the ad-block rule-sets ship in.
	srsBinaryVersion = 3
)

// CompileSmartRuleSet compiles the Smart-mode block-list into a binary sing-box
// rule-set and returns its path. Inlining the list as domain_suffix costs
// ~160 ms per connect on a 78k-entry list (1.4 MB of JSON to marshal, parse and
// index); a pre-compiled rule-set loads in a fraction of that.
//
// The file name carries a hash of the list, so an unchanged list is a pure stat
// and a changed list lands on a new path. Errors are the caller's cue to fall
// back to inline rules — a broken cache must never block a connect.
func CompileSmartRuleSet(dataDir string, domains []string) (string, error) {
	if dataDir == "" {
		return "", errors.New("smart rule-set: empty data dir")
	}
	if len(domains) == 0 {
		return "", errors.New("smart rule-set: empty domain list")
	}

	path := smartRuleSetPath(dataDir, domains)
	// A non-empty file is enough to trust the cache: we publish only via
	// os.Rename below, so a truncated rule-set can never become visible under
	// this name.
	if st, err := os.Stat(path); err == nil && st.Size() > 0 {
		return path, nil
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("smart rule-set: create dir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, smartRuleSetPrefix+"*.tmp")
	if err != nil {
		return "", fmt.Errorf("smart rule-set: temp file: %w", err)
	}
	tmpName := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	ruleSet := option.PlainRuleSet{
		Rules: []option.HeadlessRule{{
			Type: "default",
			DefaultOptions: option.DefaultHeadlessRule{
				DomainSuffix: badoption.Listable[string](domains),
			},
		}},
	}
	if err := srs.Write(tmp, ruleSet, srsBinaryVersion); err != nil {
		return "", fmt.Errorf("smart rule-set: write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return "", fmt.Errorf("smart rule-set: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("smart rule-set: close: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return "", fmt.Errorf("smart rule-set: rename: %w", err)
	}
	renamed = true

	pruneStaleSmartRuleSets(dir, filepath.Base(path))
	return path, nil
}

// smartRuleSetPath is the cache path for this exact domain list.
func smartRuleSetPath(dataDir string, domains []string) string {
	return filepath.Join(dataDir, smartRuleSetSubdir, smartRuleSetPrefix+smartRuleSetKey(domains)+smartRuleSetExt)
}

func smartRuleSetKey(domains []string) string {
	h := sha256.New()
	h.Write([]byte(smartRuleSetSchema))
	h.Write([]byte{'\n'})
	for _, d := range domains {
		h.Write([]byte(d))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// pruneStaleSmartRuleSets removes rule-sets compiled from older list revisions.
// Best-effort: a leftover file only wastes disk, so failures are ignored.
func pruneStaleSmartRuleSets(dir, keep string) {
	matches, err := filepath.Glob(filepath.Join(dir, smartRuleSetPrefix+"*"+smartRuleSetExt))
	if err != nil {
		return
	}
	for _, m := range matches {
		if filepath.Base(m) == keep {
			continue
		}
		_ = os.Remove(m)
	}
}
