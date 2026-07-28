// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import "strings"

// Smart per-app membership: which installed apps are "associated with a blocked
// resource" and should therefore ride the VPN in Smart mode.
//
// Ported from the Kotlin SmartAppMatcher so the domain list never has to cross
// JNI — the caller sends ~200 package names and gets back the matched subset.
//
// Precision over recall, by design. The membership excludes every unmatched app
// from the tunnel, so a FALSE POSITIVE is actively harmful: a VPN-hostile app
// (bank, gov, payments) wrongly pulled in would break, which is the very thing
// Smart mode exists to prevent. Misses are cheap — the user adds them via the
// manual "в VPN" list.

// DefaultSmartAliases maps packages whose reverse-DNS registrable domain does
// NOT equal their blocked service domain onto a domain that is in the blocklist.
var DefaultSmartAliases = map[string]string{
	"com.google.android.youtube":            "youtube.com",
	"com.google.android.youtube.tv":         "youtube.com",
	"com.google.android.apps.youtube.music": "youtube.com",
	"com.zhiliaoapp.musically":              "tiktok.com",
	"com.zhiliaoapp.musically.go":           "tiktok.com",
	"com.ss.android.ugc.trill":              "tiktok.com",
	"org.thunderdog.challegram":             "t.me",
}

// SmartRegistrableDomain is the registrable domain implied by a reverse-DNS
// package name: `com.instagram.android` → `instagram.com`. Returns "" for
// packages with fewer than two non-empty labels.
//
// Deliberately naive (first two labels = TLD.SLD): it does not consult a public
// suffix list, so `com.co.uk.app` yields `co.uk`. That only risks a rare false
// positive if such a suffix were itself blocked, which the RU list does not
// contain — acceptable for a membership hint the user can override.
func SmartRegistrableDomain(pkg string) string {
	labels := strings.Split(strings.ToLower(strings.TrimSpace(pkg)), ".")
	if len(labels) < 2 {
		return ""
	}
	tld, sld := labels[0], labels[1]
	if tld == "" || sld == "" {
		return ""
	}
	return sld + "." + tld
}

// MatchSmartPackages returns the packages considered blocked-associated, in
// input order. match reports whether a host is in the blocklist — pass
// (*domain.Matcher).Match from LoadSmartDomainMatcher.
func MatchSmartPackages(packages []string, match func(string) bool) []string {
	if match == nil {
		return nil
	}
	out := make([]string, 0, 16)
	seen := make(map[string]struct{}, len(packages))
	for _, pkg := range packages {
		pkg = strings.TrimSpace(pkg)
		if pkg == "" {
			continue
		}
		if _, dup := seen[pkg]; dup {
			continue
		}
		if alias, ok := DefaultSmartAliases[pkg]; ok && match(alias) {
			seen[pkg] = struct{}{}
			out = append(out, pkg)
			continue
		}
		if reg := SmartRegistrableDomain(pkg); reg != "" && match(reg) {
			seen[pkg] = struct{}{}
			out = append(out, pkg)
		}
	}
	return out
}
