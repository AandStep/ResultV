// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

// Compiled rule-sets for routing profiles.
//
// A profile's rules are stored as a compiled binary sing-box rule-set, not as
// the source-format JSON the desktop writes. The reason is the one already
// measured for the Smart list (see smart_ruleset.go): a profile that expands
// "geosite:whitelist" carries tens of thousands of domains, and the core would
// re-parse that JSON on every connect. The same list as SRS is ~70 KB and loads
// in ~17 ms.
//
// The trade: an SRS stores a compiled succinct trie, so the domains cannot be
// read back out of it. Counts shown in the UI therefore come from the stored
// tokens, never from the compiled file.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sagernet/sing-box/common/srs"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/domain"
)

const (
	routingRuleSetsSubdir = "routing"
	// minRoutingSRSBytes guards against referencing a truncated / half-written
	// SRS. A valid but empty rule-set is ~30 bytes; anything under this is
	// certainly junk. Same rationale as minSmartSRSBytes.
	minRoutingSRSBytes = 32
	// maxRoutingProfileIDLen bounds the id used as a file name.
	maxRoutingProfileIDLen = 64
)

// RoutingActions are the three actions a profile can assign, in the order the
// compiler walks them. This is NOT the order rules are emitted in — that is the
// profile's RouteOrder, see NormalizeRoutingOrder.
var RoutingActions = []string{"direct", "proxy", "block"}

// RoutingRuleSetDir is where compiled profile rule-sets live.
func RoutingRuleSetDir(dataDir string) string {
	return filepath.Join(dataDir, routingRuleSetsSubdir)
}

// ValidRoutingProfileID reports whether id is safe to use as a file name.
//
// The id is generated here (NewRoutingProfileID, routing_merge.go), but it
// makes a round trip through a JSON file on the Kotlin side before coming back,
// so it is checked rather than trusted: it lands in a path, and a "../" in it
// would write outside the cache directory.
func ValidRoutingProfileID(id string) bool {
	if id == "" || len(id) > maxRoutingProfileIDLen {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// RoutingProfileRuleSetTag is the sing-box rule_set tag for one action of one
// profile. It is also the cache file's base name, so the two cannot drift.
func RoutingProfileRuleSetTag(profileID, action string) string {
	return "prof-" + profileID + "-" + action
}

// RoutingProfileSRSPath is the cache file for one action of one profile.
func RoutingProfileSRSPath(dataDir, profileID, action string) string {
	return filepath.Join(RoutingRuleSetDir(dataDir),
		RoutingProfileRuleSetTag(profileID, action)+".srs")
}

// CompileRoutingSRS writes the parsed rules as a binary rule-set at path.
//
// An empty list is an error and nothing is written: callers rely on a failed
// compile leaving any previous cache intact, so a profile that briefly fails to
// resolve keeps routing by whatever it last resolved to.
//
// The write is atomic (temp + rename). A half-written SRS referenced as a local
// rule_set fails sing-box startup outright — that breaks the connection, not
// just the profile.
func CompileRoutingSRS(p ParsedRoutingList, path string) error {
	if len(p.Domains) == 0 && len(p.ExactDomains) == 0 && len(p.CIDRs) == 0 {
		return fmt.Errorf("routing SRS: empty rule list")
	}
	base := strings.TrimSuffix(filepath.Base(path), ".srs")
	if base == "" || strings.ContainsAny(base, `/\`) || strings.Contains(base, "..") {
		return fmt.Errorf("routing SRS: unsafe cache path %q", path)
	}
	ruleSet := option.PlainRuleSet{
		Rules: []option.HeadlessRule{{
			Type: C.RuleTypeDefault,
			DefaultOptions: option.DefaultHeadlessRule{
				Domain:       p.ExactDomains,
				DomainSuffix: p.Domains,
				IPCIDR:       p.CIDRs,
			},
		}},
	}
	var buf bytes.Buffer
	if err := srs.Write(&buf, ruleSet, C.RuleSetVersion3); err != nil {
		return fmt.Errorf("routing SRS: writing: %w", err)
	}
	// Validate our own output before it reaches disk: an invalid local rule_set
	// fails the engine's start, and finding that out at connect time gives no
	// clue which profile did it.
	if err := validateSRS(buf.Bytes()); err != nil {
		return fmt.Errorf("routing SRS: self-validation: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("routing SRS: creating dir: %w", err)
	}
	// A unique temp name per call: compiles of different profiles run on
	// independent coroutine scopes with nothing serialising them, and a shared
	// fixed name would let one writer's truncate interleave with another's.
	// Same collision CompileSmartSRS had to fix.
	tmp, err := os.CreateTemp(dir, "routing-*.srs")
	if err != nil {
		return fmt.Errorf("routing SRS: creating temp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("routing SRS: writing temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("routing SRS: closing temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("routing SRS: renaming: %w", err)
	}
	return nil
}

// RoutingProfileSRSReady reports whether one action of one profile has a usable
// compiled rule-set on disk.
func RoutingProfileSRSReady(dataDir, profileID, action string) bool {
	if !ValidRoutingProfileID(profileID) {
		return false
	}
	return localRoutingSRSUsable(RoutingProfileSRSPath(dataDir, profileID, action))
}

// localRoutingSRSUsable reports whether the SRS at path can be referenced as a
// `local` rule_set. On a failed validation the file is deleted (best effort) so
// the next compile writes a clean copy instead of the engine refusing to start.
func localRoutingSRSUsable(path string) bool {
	st, err := os.Stat(path)
	if err != nil || st.Size() < minRoutingSRSBytes {
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

// RemoveRoutingProfileSRS deletes a profile's cached rule-sets. Best effort: a
// file that will not go away is not worth failing a delete over, and the rules
// are emitted only for a profile the caller names, so a leftover routes nothing.
func RemoveRoutingProfileSRS(dataDir, profileID string) {
	if !ValidRoutingProfileID(profileID) {
		return
	}
	for _, action := range RoutingActions {
		_ = os.Remove(RoutingProfileSRSPath(dataDir, profileID, action))
	}
}

// LoadRoutingDomainMatcher reads a compiled rule-set back as a domain matcher.
//
// Only the compiled trie comes back — srs.Read returns DomainMatcher with the
// Domain/DomainSuffix lists empty (see LoadSmartDomainMatcher for the same
// note). So this answers "does this host match", never "what is in the list".
// Used by tests; the engine references the file by path and never reads it.
func LoadRoutingDomainMatcher(path string) (*domain.Matcher, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	compat, err := srs.Read(bytes.NewReader(data), false)
	if err != nil {
		return nil, fmt.Errorf("routing SRS: reading: %w", err)
	}
	plain, err := compat.Upgrade()
	if err != nil {
		return nil, fmt.Errorf("routing SRS: upgrading: %w", err)
	}
	if len(plain.Rules) == 0 || plain.Rules[0].DefaultOptions.DomainMatcher == nil {
		return nil, fmt.Errorf("routing SRS: no domain matcher in rule-set")
	}
	return plain.Rules[0].DefaultOptions.DomainMatcher, nil
}
