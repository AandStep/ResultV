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

package main

// Turning a stored routing profile into rules the engine can run.
//
// A profile lists its rules the way its author wrote them — mostly references
// into two geo databases. So applying one means: fetch those databases, expand
// the references, and write the result as the same source-format rule_set the
// router already consumes for routing lists (see internal/proxy/routinglist.go).
// From there a profile is indistinguishable from a list, which is the point:
// one path through the engine, not two.
//
// Compilation happens when the profile changes — imported, edited, activated —
// and never at connect time. Connecting only stats the cached files.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/proxy"
)

const (
	geoSubdir = "routing/geo"
	// A geo database is third-party data fetched from a URL a deep link chose.
	// The routing-list fetcher already bounds bodies at 8 MiB, which is roomy
	// for the ~0.5 MiB real files and still finite.
	geoKindSite = "geosite"
	geoKindIP   = "geoip"
)

// geoDataDir sits beside the rule-set cache: routingListDataDir() is the data
// root, and proxy.RoutingListsDir puts lists at <root>/routing/lists.
func (a *App) geoDataDir() string {
	return filepath.Join(a.routingListDataDir(), geoSubdir)
}

// geoCachePath keys the cache by the URL, not by the profile: two profiles
// pointing at the same database share one file, and a profile that changes its
// URL fetches afresh instead of silently reusing the old contents.
func (a *App) geoCachePath(kind, url string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(url)))
	name := kind + "-" + hex.EncodeToString(sum[:8]) + ".dat"
	return filepath.Join(a.geoDataDir(), name)
}

// ensureGeoFile returns the cached bytes, fetching them first if the cache is
// cold. force re-fetches even when a cache exists.
func (a *App) ensureGeoFile(kind, url string, force bool) ([]byte, error) {
	if url == "" {
		return nil, nil
	}
	path := a.geoCachePath(kind, url)
	if !force {
		if blob, err := os.ReadFile(path); err == nil && len(blob) > 0 {
			return blob, nil
		}
	}
	blob, err := a.fetchRoutingListPayload(url, false)
	if err != nil {
		return nil, fmt.Errorf("не удалось скачать %s: %w", kind, err)
	}
	// Refuse to cache something that is not a geo database. Without this, a
	// panel's error page would be stored and every later compile would fail
	// against it while looking like a cache hit.
	if err := validateGeoBlob(kind, blob); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(a.geoDataDir(), 0o700); err != nil {
		return nil, err
	}
	if err := writeFileAtomic(path, blob); err != nil {
		return nil, err
	}
	return blob, nil
}

func validateGeoBlob(kind string, blob []byte) error {
	var err error
	switch kind {
	case geoKindSite:
		_, _, err = proxy.ParseGeoSiteDat(blob)
	case geoKindIP:
		_, _, err = proxy.ParseGeoIPDat(blob)
	default:
		return fmt.Errorf("unknown geo database kind %q", kind)
	}
	if err != nil {
		return fmt.Errorf("%s по ссылке не является базой geo: %w", kind, err)
	}
	return nil
}

// writeFileAtomic writes through a temp file and a rename, so a failed or
// interrupted write never leaves a half-file behind for the next read to trust.
func writeFileAtomic(path string, blob []byte) error {
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
	if err := tmp.Chmod(0o600); err != nil {
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
func (a *App) loadGeoDatabases(p config.RoutingProfile, force bool) (proxy.GeoDatabases, error) {
	var db proxy.GeoDatabases
	needSite, needIP := profileNeedsGeo(p)

	if needSite {
		blob, err := a.ensureGeoFile(geoKindSite, p.GeoSiteURL, force)
		if err != nil {
			return db, err
		}
		if len(blob) > 0 {
			sites, dropped, perr := proxy.ParseGeoSiteDat(blob)
			if perr != nil {
				return db, perr
			}
			db.Sites = sites
			db.SiteDropped = dropped
		}
	}
	if needIP {
		blob, err := a.ensureGeoFile(geoKindIP, p.GeoIPURL, force)
		if err != nil {
			return db, err
		}
		if len(blob) > 0 {
			ips, inverted, perr := proxy.ParseGeoIPDat(blob)
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

// profileNeedsGeo reports which databases the profile actually references, so a
// profile of plain domains never downloads half a megabyte it will not read.
func profileNeedsGeo(p config.RoutingProfile) (site, ip bool) {
	for _, action := range []string{"direct", "proxy", "block"} {
		for _, token := range proxy.RoutingProfileTokens(p, action) {
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

// routingProfileRuleSetID is the cache id of one action of one profile. It
// shares the routing-list namespace on purpose — the engine treats both the
// same, so they may as well live in one directory with one cleanup path.
func routingProfileRuleSetID(profileID, action string) string {
	return "prof-" + profileID + "-" + action
}

// CompileRoutingProfile expands a profile's rules into cached rule-sets and
// reports what it could not express. Returns per-action counts for the UI.
func (a *App) CompileRoutingProfile(id string, refreshGeo bool) (map[string]any, error) {
	if a == nil || a.config == nil {
		return nil, fmt.Errorf("config manager not initialized")
	}
	var profile config.RoutingProfile
	found := false
	for _, p := range a.config.GetConfig().RoutingRules.Profiles {
		if p.ID == id {
			profile = p
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("профиль не найден")
	}
	return a.compileRoutingProfile(profile, refreshGeo)
}

func (a *App) compileRoutingProfile(p config.RoutingProfile, refreshGeo bool) (map[string]any, error) {
	db, err := a.loadGeoDatabases(p, refreshGeo)
	if err != nil {
		return nil, err
	}
	dir := a.routingListDataDir()
	counts := map[string]any{}
	unresolved := map[string]string{}
	wrote := 0

	for _, action := range []string{"direct", "proxy", "block"} {
		id := routingProfileRuleSetID(p.ID, action)
		path := proxy.RoutingListCachePath(dir, id)
		tokens := proxy.RoutingProfileTokens(p, action)
		if len(tokens) == 0 {
			// An action the profile does not use must leave no stale file from
			// a previous version of it behind.
			_ = removeFileIfExists(path)
			counts[action] = 0
			continue
		}
		parsed, report := proxy.ResolveGeoTokens(tokens, db)
		for token, reason := range report.Unresolved {
			unresolved[token] = reason
		}
		total := len(parsed.Domains) + len(parsed.ExactDomains) + len(parsed.CIDRs)
		counts[action] = total
		if total == 0 {
			_ = removeFileIfExists(path)
			continue
		}
		if err := proxy.WriteRoutingListRuleSet(dir, id, parsed); err != nil {
			return nil, err
		}
		wrote++
	}

	if wrote == 0 {
		return nil, fmt.Errorf("ни одно правило профиля не удалось применить — проверьте ссылки на geo-базы")
	}
	a.log.Info(fmt.Sprintf("[МАРШРУТИЗАЦИЯ] профиль %q собран: direct %v, proxy %v, block %v; не принято правил: %d",
		p.Name, counts["direct"], counts["proxy"], counts["block"], len(unresolved)))

	return map[string]any{
		"counts":     counts,
		"unresolved": unresolved,
	}, nil
}

// removeRoutingProfileRuleSets deletes a profile's cached rule-sets.
func (a *App) removeRoutingProfileRuleSets(profileID string) {
	dir := a.routingListDataDir()
	for _, action := range []string{"direct", "proxy", "block"} {
		_ = removeFileIfExists(proxy.RoutingListCachePath(dir, routingProfileRuleSetID(profileID, action)))
	}
}

// buildRoutingProfileSpecs returns the rule-set specs of the active profile.
//
// Empty in Smart mode, and that is the whole rule: in Smart the client works
// out routing itself, so a profile there would be pulling against it. The UI
// only offers profiles in Global; this makes it true of the engine as well,
// rather than trusting the UI to be the enforcement point.
func (a *App) buildRoutingProfileSpecs() []proxy.RoutingListSpec {
	if a == nil || a.config == nil {
		return nil
	}
	rr := a.config.GetConfig().RoutingRules
	if rr.Mode == "smart" {
		return nil
	}
	profile, ok := a.ActiveRoutingProfile()
	if !ok {
		return nil
	}
	dir := a.routingListDataDir()
	var out []proxy.RoutingListSpec
	for _, action := range []string{"direct", "proxy", "block"} {
		id := routingProfileRuleSetID(profile.ID, action)
		path := proxy.RoutingListCachePath(dir, id)
		if _, err := os.Stat(path); err != nil {
			// Not compiled yet, or compiled to nothing — omit rather than hand
			// the engine a dangling path.
			continue
		}
		out = append(out, proxy.RoutingListSpec{
			Tag:    proxy.RoutingListRuleSetTag(id),
			Path:   path,
			Action: action,
		})
	}
	return out
}

// activeRoutingOrder is the action order the engine should use: the active
// profile's, when there is one and it asked for a particular order.
func (a *App) activeRoutingOrder() []string {
	if a == nil || a.config == nil {
		return nil
	}
	if a.config.GetConfig().RoutingRules.Mode == "smart" {
		return nil
	}
	profile, ok := a.ActiveRoutingProfile()
	if !ok || profile.RouteOrder == "" {
		return nil
	}
	return proxy.NormalizeRoutingOrder(profile.RouteOrder)
}
