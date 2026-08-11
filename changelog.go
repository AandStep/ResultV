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

import (
	_ "embed"
	"encoding/json"
	"strings"
)

// update.json is embedded at build time rather than fetched, the same way
// version.go embeds wails.json. The manifest published on main always
// describes the LATEST release, so a running build could only read its own
// release notes during the window before the next release ships — and never at
// all without network. Baked into the binary, the notes always match the
// version the user is actually running, offline included.
//
//go:embed update.json
var embeddedUpdateJSON []byte

// changelogTypeKey is the JSON key inside a release-note entry that carries the
// entry's kind rather than a translation. Everything else in the object is a
// language code.
const changelogTypeKey = "type"

// changelogLangFallbacks is the ordered fallback chain used when an entry has
// no text for the requested language. Deliberately explicit: ranging over the
// remaining map keys would pick a language at random, since Go randomizes map
// iteration order.
var changelogLangFallbacks = []string{"ru", "en"}

// ChangelogItem is one line of release notes, already resolved to a single
// language. Type is one of "feature", "fix", "improve"; the frontend renders an
// unknown or empty type as a neutral bullet.
type ChangelogItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Changelog is what the "what's new" modal renders.
type Changelog struct {
	Version string          `json:"version"`
	Title   string          `json:"title"`
	Items   []ChangelogItem `json:"items"`
}

// rawChangelogManifest is the slice of update.json this file cares about.
// Both title and note entries are plain string maps so that adding a third
// language to the manifest needs no change here.
type rawChangelogManifest struct {
	ReleaseTitle map[string]string   `json:"releaseTitle"`
	ReleaseNotes []map[string]string `json:"releaseNotes"`
}

// normalizeChangelogLang reduces an i18next tag ("ru-RU", "en-US") to the bare
// language code the manifest is keyed by.
func normalizeChangelogLang(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if i := strings.IndexAny(lang, "-_"); i >= 0 {
		lang = lang[:i]
	}
	return lang
}

// pickChangelogLang returns the first non-empty translation, trying the
// requested language before the fixed fallback chain. Returns "" when the entry
// has no usable text at all, which the caller drops.
func pickChangelogLang(values map[string]string, lang string) string {
	if len(values) == 0 {
		return ""
	}
	for _, key := range append([]string{normalizeChangelogLang(lang)}, changelogLangFallbacks...) {
		if key == "" || key == changelogTypeKey {
			continue
		}
		if text := strings.TrimSpace(values[key]); text != "" {
			return text
		}
	}
	return ""
}

// parseChangelog builds the changelog for one language out of raw manifest
// bytes. Split from GetChangelog so tests can feed it a manifest without
// touching the embedded one.
func parseChangelog(manifest []byte, version, lang string) (*Changelog, error) {
	var raw rawChangelogManifest
	if err := json.Unmarshal(manifest, &raw); err != nil {
		return nil, err
	}

	items := make([]ChangelogItem, 0, len(raw.ReleaseNotes))
	for _, entry := range raw.ReleaseNotes {
		text := pickChangelogLang(entry, lang)
		if text == "" {
			// An entry with no text in any known language would render as an
			// empty bullet. Dropping it is better than showing a blank row.
			continue
		}
		items = append(items, ChangelogItem{
			Type: strings.ToLower(strings.TrimSpace(entry[changelogTypeKey])),
			Text: text,
		})
	}

	return &Changelog{
		// The version shown is the running build's, never the manifest's
		// "version" field — that field is empty in the repo between releases
		// and is stamped by CI only on the copy published to main.
		Version: version,
		Title:   pickChangelogLang(raw.ReleaseTitle, lang),
		Items:   items,
	}, nil
}

// GetChangelog returns the release notes of the running build in the requested
// language. Pure read: it never decides whether the notes should be shown and
// never writes anything. Returns nil when the embedded manifest is unusable or
// carries no notes, so the frontend can simply skip the modal.
func (a *App) GetChangelog(lang string) *Changelog {
	cl, err := parseChangelog(embeddedUpdateJSON, productVersionFromWailsJSON(), lang)
	if err != nil {
		if a != nil && a.log != nil {
			a.log.Warning("Не удалось разобрать список изменений: " + err.Error())
		}
		return nil
	}
	if len(cl.Items) == 0 {
		return nil
	}
	return cl
}

// shouldShowChangelog is the display policy, kept free of *App so it can be
// tested as a table.
//
//	seen == installed          → already shown for this build
//	seen != installed          → updated since we last showed anything
//	seen == "", fresh install   → nothing to catch up on, stay quiet
//	seen == "", existing config → upgraded from a build predating this field,
//	                              so show what changed
func shouldShowChangelog(seen, installed string, freshInstall bool) bool {
	seen = strings.TrimSpace(seen)
	installed = strings.TrimSpace(installed)
	if installed == "" {
		return false
	}
	if seen == "" {
		return !freshInstall
	}
	// Plain inequality rather than a semver comparison: a downgrade should also
	// show notes, and it shows the notes of the build being run, since the
	// manifest is embedded in that very build.
	return seen != installed
}

// ShouldShowChangelog reports whether the "what's new" modal is due this launch.
func (a *App) ShouldShowChangelog() bool {
	if a == nil || a.config == nil {
		return false
	}
	return shouldShowChangelog(
		a.config.GetConfig().Settings.LastChangelogVersion,
		productVersionFromWailsJSON(),
		a.config.WasCreatedFresh(),
	)
}

// AckChangelog records the running version as seen, so the modal does not come
// back until the next update.
func (a *App) AckChangelog() error {
	return a.markChangelogSeen()
}

func (a *App) markChangelogSeen() error {
	if a == nil || a.config == nil {
		return nil
	}
	settings := a.config.GetConfig().Settings
	installed := productVersionFromWailsJSON()
	if settings.LastChangelogVersion == installed {
		return nil
	}
	settings.LastChangelogVersion = installed
	return a.config.UpdateSettings(settings)
}

// seedChangelogVersionOnFreshInstall marks the current version as seen when
// this is the first run of a brand-new install. Without it, the first launch
// stays quiet (WasCreatedFresh is true) but the SECOND launch would find a
// saved config with an empty LastChangelogVersion and read it as an upgrade,
// greeting a new user with notes for changes they were never around for.
func (a *App) seedChangelogVersionOnFreshInstall() {
	if a == nil || a.config == nil || !a.config.WasCreatedFresh() {
		return
	}
	if err := a.markChangelogSeen(); err != nil && a.log != nil {
		a.log.Warning("Не удалось записать версию списка изменений: " + err.Error())
	}
}
