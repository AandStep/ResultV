// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build no_adblock

// Stub DNS filtering for the `play` distribution.
//
// The store build ships no filtering at all: no ad or tracker rule-sets, no
// curated ad-delivery domains, nothing to download. Google Play reviews what
// the binary contains, so leaving the lists linked but unreachable from the UI
// would not be enough — the tag takes adblock_rules.go, extra_ads.go and
// youtube_ads.go out of the build, and this file stands in for the symbols
// engine.go and the mobile bindings still reference. Same split the no_mitm
// tag uses for internal/filter.
//
// adBlockSupported is what actually disables the feature. Empty lists would
// not: the branches in engine.go would still emit rules, and a sing-box reject
// rule carrying no matchers matches every connection.

package proxy

import (
	"context"
	"errors"
)

const adBlockSupported = false

// Matcher lists referenced by the (compiled-out) engine branches.
var (
	adBlockConnectivityBypassDomains []string
	extraAdDeliveryDomains           []string
	youTubeCoreDomains               []string
	youTubeCoreSuffixes              []string
)

func adDeliveryRejectSuffixes() []string { return nil }

func youTubeDNSBypassSuffixes() []string { return nil }

func availableAdBlockRuleSetTags(string) []string { return nil }

func buildAdBlockRuleSets(string) []SBRouteRuleSet { return nil }

// AdBlockDownloadResult keeps the shape mobile.FetchAdBlockLists marshals, so
// the Kotlin side compiles and runs against either build.
type AdBlockDownloadResult struct {
	Ready int
	Total int
	Err   error
}

// ErrAdBlockUnsupported is what the store build reports instead of silently
// succeeding with zero lists — a caller that somehow asks for filtering here
// deserves an answer it can log, not a no-op.
var ErrAdBlockUnsupported = errors.New("DNS filtering is not part of this build")

// DownloadAdBlockRuleSets reports the feature as absent without touching the
// network or the cache directory.
func DownloadAdBlockRuleSets(context.Context, string) AdBlockDownloadResult {
	return AdBlockDownloadResult{Err: ErrAdBlockUnsupported}
}
