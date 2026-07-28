// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/sagernet/sing-box/common/srs"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/domain"
)

// The Smart blocklist is stored as a compiled binary sing-box rule-set, not as
// inline domain/domain_suffix arrays in the config. Inline cost ~150k entries =
// a 4.6 MB config that has to be marshalled, shipped across JNI three times and
// re-parsed by sing-box on EVERY connect (~2s). The same list as SRS is ~70 KB
// and loads in ~17 ms, so a Smart connect costs the same as a Global one.
//
// This mirrors buildAdBlockRuleSets: `local` rule_sets only — never `remote`.
// sing-box downloads a remote rule_set synchronously on cold start and its
// failure ABORTS engine startup.

const (
	smartRuleSetTag     = "smart"
	smartRuleSetsSubdir = "smart"
	smartSRSFileName    = "smart.srs"
	// minSmartSRSBytes guards against referencing a truncated / half-written
	// SRS. A valid but empty rule-set is ~30 bytes; anything under this is
	// certainly junk. Same rationale as minLocalSRSBytes for ad-block.
	minSmartSRSBytes = 32
)

// smartRuleSetDir is where the compiled Smart SRS lives.
func smartRuleSetDir(dataDir string) string {
	return filepath.Join(dataDir, smartRuleSetsSubdir)
}

// SmartSRSPath is the on-disk path of the compiled Smart rule-set.
func SmartSRSPath(dataDir string) string {
	return filepath.Join(smartRuleSetDir(dataDir), smartSRSFileName)
}

// CompileSmartSRS compiles domains into a binary SRS at path.
//
// The write is atomic (temp + rename): a half-written SRS referenced as a local
// rule_set fails sing-box startup and would break the connection, and a failed
// compile must never clobber a previously good list.
func CompileSmartSRS(domains []string, path string) error {
	exact, suffix := splitSmartDomains(domains)
	if len(exact)+len(suffix) == 0 {
		return fmt.Errorf("smart SRS: empty domain list")
	}
	ruleSet := option.PlainRuleSet{
		Rules: []option.HeadlessRule{{
			Type: C.RuleTypeDefault,
			DefaultOptions: option.DefaultHeadlessRule{
				Domain:       exact,
				DomainSuffix: suffix,
			},
		}},
	}
	var buf bytes.Buffer
	if err := srs.Write(&buf, ruleSet, C.RuleSetVersion3); err != nil {
		return fmt.Errorf("smart SRS: writing: %w", err)
	}
	if err := validateSRS(buf.Bytes()); err != nil {
		return fmt.Errorf("smart SRS: self-validation: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("smart SRS: creating dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("smart SRS: writing temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("smart SRS: renaming: %w", err)
	}
	return nil
}

// localSmartSRSUsable reports whether the SRS at path can be referenced as a
// `local` rule_set. On a failed validation the file is deleted (best effort) so
// the next refresh writes a clean copy instead of the engine refusing to start.
func localSmartSRSUsable(path string) bool {
	st, err := os.Stat(path)
	if err != nil || st.Size() < minSmartSRSBytes {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	if err := validateSRS(data); err != nil {
		_ = os.Remove(path) // self-heal
		return false
	}
	return true
}

// buildSmartRuleSet returns the sing-box rule_set definition for the Smart list,
// or nothing when no usable SRS is cached (caller then falls back to the inline
// SmartBlockedDomains path, which is what desktop still uses).
func buildSmartRuleSet(dataDir string) []SBRouteRuleSet {
	path := SmartSRSPath(dataDir)
	if !localSmartSRSUsable(path) {
		return nil
	}
	return []SBRouteRuleSet{{
		Type:   "local",
		Tag:    smartRuleSetTag,
		Format: "binary",
		Path:   path,
	}}
}

// smartMatcherCache memoises the parsed domain matcher. Reading the SRS costs
// ~17 ms for a 150k-entry list; the per-app membership computation runs on the
// connect path, so we keep the matcher until the file changes underneath us.
var smartMatcherCache struct {
	sync.Mutex
	path    string
	size    int64
	modTime int64
	matcher *domain.Matcher
}

// LoadSmartDomainMatcher returns a matcher for the compiled Smart list.
//
// NOTE: the SRS format stores a COMPILED succinct trie, not the raw domain
// list — srs.Read gives back DomainMatcher with Domain/DomainSuffix empty. That
// is why per-app membership probes the matcher instead of reading domains back.
func LoadSmartDomainMatcher(path string) (*domain.Matcher, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	smartMatcherCache.Lock()
	defer smartMatcherCache.Unlock()
	if smartMatcherCache.matcher != nil &&
		smartMatcherCache.path == path &&
		smartMatcherCache.size == st.Size() &&
		smartMatcherCache.modTime == st.ModTime().UnixNano() {
		return smartMatcherCache.matcher, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	compat, err := srs.Read(bytes.NewReader(data), false)
	if err != nil {
		return nil, fmt.Errorf("smart SRS: reading: %w", err)
	}
	plain, err := compat.Upgrade()
	if err != nil {
		return nil, fmt.Errorf("smart SRS: upgrading: %w", err)
	}
	if len(plain.Rules) == 0 || plain.Rules[0].DefaultOptions.DomainMatcher == nil {
		return nil, fmt.Errorf("smart SRS: no domain matcher in rule-set")
	}
	m := plain.Rules[0].DefaultOptions.DomainMatcher
	smartMatcherCache.path = path
	smartMatcherCache.size = st.Size()
	smartMatcherCache.modTime = st.ModTime().UnixNano()
	smartMatcherCache.matcher = m
	return m, nil
}

// SmartSRSReady reports whether a usable compiled Smart rule-set is cached.
func SmartSRSReady(dataDir string) bool {
	return localSmartSRSUsable(SmartSRSPath(dataDir))
}

// InstallSmartSRS validates data as a sing-box rule-set and installs it as the
// cached Smart list. Used for the APK-bundled seed. Validation happens BEFORE
// anything reaches disk: an invalid local rule_set fails sing-box startup.
func InstallSmartSRS(dataDir string, data []byte) error {
	if len(data) < minSmartSRSBytes {
		return fmt.Errorf("smart SRS seed: too small (%d bytes)", len(data))
	}
	if err := validateSRS(data); err != nil {
		return fmt.Errorf("smart SRS seed: %w", err)
	}
	path := SmartSRSPath(dataDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("smart SRS seed: creating dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("smart SRS seed: writing temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("smart SRS seed: renaming: %w", err)
	}
	return nil
}
