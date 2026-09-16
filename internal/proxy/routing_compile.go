// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

// Turning a stored routing profile into rules the engine can run.
//
// A profile lists its rules the way its author wrote them — mostly references
// into two geo databases. So applying one means: fetch those databases, expand
// the references, and compile the result into the binary rule-sets the router
// consumes (routing_ruleset.go).
//
// Compilation happens when the profile changes — imported, edited, activated,
// refreshed — and NEVER at connect time. Connecting only stats the cached files.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"resultproxy-wails/internal/config"
)

const (
	geoSubdir   = "geo"
	geoKindSite = "geosite"
	geoKindIP   = "geoip"
)

// RoutingCompileReport is what a compile has to say for itself: how many rules
// each action ended up with, and every token it could not express.
//
// Unresolved is a map rather than a count because a profile that imports half
// its rules must be able to say WHICH half and why — otherwise the user sees
// traffic take the wrong route with no explanation anywhere.
type RoutingCompileReport struct {
	Counts     map[string]int    `json:"counts"`
	Unresolved map[string]string `json:"unresolved"`
}

// geoDataDir sits beside the rule-set cache.
func geoDataDir(dataDir string) string {
	return filepath.Join(RoutingRuleSetDir(dataDir), geoSubdir)
}

// GeoCachePath keys the cache by the URL, not by the profile: two profiles
// pointing at the same database share one file, and a profile that changes its
// URL fetches afresh instead of silently reusing the old contents.
func GeoCachePath(dataDir, kind, url string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(url)))
	return filepath.Join(geoDataDir(dataDir), kind+"-"+hex.EncodeToString(sum[:8])+".dat")
}

// ProfileNeedsGeo reports which databases the profile actually references, so a
// profile of plain domains never downloads half a megabyte it will not read.
func ProfileNeedsGeo(p config.RoutingProfile) (site, ip bool) {
	for _, action := range RoutingActions {
		for _, token := range RoutingProfileTokens(p, action) {
			low := strings.ToLower(strings.TrimSpace(token))
			if strings.HasPrefix(low, "geosite:") {
				site = true
			}
			if strings.HasPrefix(low, "geoip:") {
				ip = true
			}
		}
	}
	return site && p.GeoSiteURL != "", ip && p.GeoIPURL != ""
}

func validateGeoBlob(kind string, blob []byte) error {
	var err error
	switch kind {
	case geoKindSite:
		_, _, err = ParseGeoSiteDat(blob)
	case geoKindIP:
		_, _, err = ParseGeoIPDat(blob)
	default:
		return fmt.Errorf("unknown geo database kind %q", kind)
	}
	if err != nil {
		return fmt.Errorf("по ссылке не база %s: %w", kind, err)
	}
	return nil
}

// ensureGeoFile returns the cached bytes, fetching them first if the cache is
// cold. force re-fetches even when a cache exists.
func ensureGeoFile(ctx context.Context, dataDir, kind, url string, allowInsecure, force bool) ([]byte, error) {
	if url == "" {
		return nil, nil
	}
	path := GeoCachePath(dataDir, kind, url)
	if !force {
		if blob, err := os.ReadFile(path); err == nil && len(blob) > 0 {
			return blob, nil
		}
	}
	blob, err := FetchRoutingPayload(ctx, url, allowInsecure)
	if err != nil {
		return nil, fmt.Errorf("не удалось скачать %s: %w", kind, err)
	}
	// Refuse to cache something that is not a geo database. Without this, a
	// panel's error page would be stored and every later compile would fail
	// against it while looking like a cache hit — the kind of failure that
	// survives a retry and reads as "the profile is broken".
	if err := validateGeoBlob(kind, blob); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(geoDataDir(dataDir), 0o700); err != nil {
		return nil, err
	}
	if err := writeGeoFileAtomic(path, blob); err != nil {
		return nil, err
	}
	return blob, nil
}

// writeGeoFileAtomic writes through a temp file and a rename, so a failed or
// interrupted write never leaves a half-file behind for the next read to trust.
func writeGeoFileAtomic(path string, blob []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(blob); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// loadGeoDatabases fetches and parses whatever databases the profile references.
// A profile with no geo tokens needs neither file and gets empty databases —
// its plain domains and CIDRs still resolve.
func loadGeoDatabases(ctx context.Context, p config.RoutingProfile, dataDir string, force bool) (GeoDatabases, error) {
	var db GeoDatabases
	needSite, needIP := ProfileNeedsGeo(p)

	if needSite {
		blob, err := ensureGeoFile(ctx, dataDir, geoKindSite, p.GeoSiteURL, p.AllowInsecure, force)
		if err != nil {
			return db, err
		}
		if len(blob) > 0 {
			sites, dropped, perr := ParseGeoSiteDat(blob)
			if perr != nil {
				return db, perr
			}
			db.Sites = sites
			db.SiteDropped = dropped
		}
	}
	if needIP {
		blob, err := ensureGeoFile(ctx, dataDir, geoKindIP, p.GeoIPURL, p.AllowInsecure, force)
		if err != nil {
			return db, err
		}
		if len(blob) > 0 {
			ips, inverted, perr := ParseGeoIPDat(blob)
			if perr != nil {
				return db, perr
			}
			db.IPs = ips
			db.InvertedIPs = make(map[string]struct{}, len(inverted))
			for _, n := range inverted {
				db.InvertedIPs[n] = struct{}{}
			}
		}
	}
	return db, nil
}

// fetchProfileList downloads one linked rule list and parses it. Same guard and
// bounds as everything else here — a profile's link is no more trusted for
// coming from a provider.
func fetchProfileList(ctx context.Context, listURL string, allowInsecure bool) (ParsedRoutingList, error) {
	body, err := FetchRoutingPayload(ctx, listURL, allowInsecure)
	if err != nil {
		return ParsedRoutingList{}, err
	}
	if LooksLikeRoutingListHTML(body) {
		return ParsedRoutingList{}, fmt.Errorf(
			"ссылка вернула веб-страницу, а не список — для GitHub нужна ссылка Raw")
	}
	parsed := ParseRoutingListPayload(body)
	if len(parsed.Domains) == 0 && len(parsed.CIDRs) == 0 && len(parsed.ExactDomains) == 0 {
		return ParsedRoutingList{}, fmt.Errorf("в списке не найдено доменов или подсетей")
	}
	return parsed, nil
}

// CompileRoutingProfile expands a profile's rules into cached rule-sets and
// reports what it could not express.
func CompileRoutingProfile(
	ctx context.Context,
	p config.RoutingProfile,
	dataDir string,
	refreshGeo bool,
) (RoutingCompileReport, error) {
	rep := RoutingCompileReport{
		Counts:     map[string]int{},
		Unresolved: map[string]string{},
	}
	if !ValidRoutingProfileID(p.ID) {
		return rep, fmt.Errorf("routing profile: invalid id %q", p.ID)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	db, err := loadGeoDatabases(ctx, p, dataDir, refreshGeo)
	if err != nil {
		return rep, err
	}

	wrote := 0
	for _, action := range RoutingActions {
		path := RoutingProfileSRSPath(dataDir, p.ID, action)
		tokens := RoutingProfileTokens(p, action)
		if len(tokens) == 0 && len(p.ListURLs[action]) == 0 {
			// An action the profile does not use must leave no stale file from
			// a previous version of it behind, or it would keep routing.
			_ = os.Remove(path)
			rep.Counts[action] = 0
			continue
		}
		parsed, report := ResolveGeoTokens(tokens, db)
		for token, reason := range report.Unresolved {
			rep.Unresolved[token] = reason
		}
		// Rules the provider only linked to are fetched now and merged in. A
		// failed fetch is reported, not fatal: the rest of the profile still
		// routes, and saying nothing would leave traffic quietly unrouted.
		for _, listURL := range p.ListURLs[action] {
			extra, ferr := fetchProfileList(ctx, listURL, p.AllowInsecure)
			if ferr != nil {
				rep.Unresolved[listURL] = ferr.Error()
				continue
			}
			parsed.Domains = append(parsed.Domains, extra.Domains...)
			parsed.ExactDomains = append(parsed.ExactDomains, extra.ExactDomains...)
			parsed.CIDRs = append(parsed.CIDRs, extra.CIDRs...)
		}
		total := len(parsed.Domains) + len(parsed.ExactDomains) + len(parsed.CIDRs)
		rep.Counts[action] = total
		if total == 0 {
			_ = os.Remove(path)
			continue
		}
		if err := CompileRoutingSRS(parsed, path); err != nil {
			return rep, err
		}
		wrote++
	}

	if wrote == 0 {
		return rep, fmt.Errorf(
			"ни одно правило профиля не удалось применить — проверьте ссылки на geo-базы")
	}
	return rep, nil
}
