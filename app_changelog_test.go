package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"resultproxy-wails/internal/config"
)

// The manifest that actually ships must always be renderable. A typo that makes
// releaseNotes unparsable, or an entry missing a translation, would otherwise
// surface as an empty or half-English modal on release day — this catches it on
// the pull request instead.
func TestEmbeddedUpdateManifestIsRenderable(t *testing.T) {
	for _, lang := range []string{"ru", "en"} {
		cl, err := parseChangelog(embeddedUpdateJSON, "9.9.9", lang)
		if err != nil {
			t.Fatalf("update.json does not parse for %q: %v", lang, err)
		}
		if cl.Title == "" {
			t.Errorf("update.json has no releaseTitle for %q", lang)
		}
		if len(cl.Items) == 0 {
			t.Fatalf("update.json has no releaseNotes entries for %q", lang)
		}
		for i, item := range cl.Items {
			if item.Text == "" {
				t.Errorf("release note %d has no text for %q", i, lang)
			}
		}
	}

	// Every entry must carry text in BOTH languages, not merely fall back to
	// the other one. parseChangelog is deliberately forgiving at runtime, so
	// only an explicit check keeps a half-translated manifest out of a tag.
	var raw rawChangelogManifest
	if err := json.Unmarshal(embeddedUpdateJSON, &raw); err != nil {
		t.Fatalf("decode update.json: %v", err)
	}
	for i, entry := range raw.ReleaseNotes {
		for _, lang := range []string{"ru", "en"} {
			if entry[lang] == "" {
				t.Errorf("release note %d is missing the %q translation", i, lang)
			}
		}
	}
	for _, lang := range []string{"ru", "en"} {
		if raw.ReleaseTitle[lang] == "" {
			t.Errorf("releaseTitle is missing the %q translation", lang)
		}
	}
}

func TestParseChangelogResolvesLanguageAndType(t *testing.T) {
	manifest := []byte(`{
	  "releaseTitle": {"ru": "Заголовок", "en": "Title"},
	  "releaseNotes": [
	    {"type": "fix", "ru": "Починили", "en": "Fixed"},
	    {"type": "FEATURE", "ru": "Добавили", "en": "Added"},
	    {"ru": "Без типа", "en": "Untyped"}
	  ]
	}`)

	cl, err := parseChangelog(manifest, "3.2.8", "en-US")
	if err != nil {
		t.Fatalf("parseChangelog: %v", err)
	}
	if cl.Version != "3.2.8" {
		t.Errorf("version = %q, want the running build's version 3.2.8", cl.Version)
	}
	if cl.Title != "Title" {
		t.Errorf("title = %q, want %q — en-US must normalize to en", cl.Title, "Title")
	}
	want := []ChangelogItem{
		{Type: "fix", Text: "Fixed"},
		{Type: "feature", Text: "Added"},
		{Type: "", Text: "Untyped"},
	}
	if len(cl.Items) != len(want) {
		t.Fatalf("got %d items, want %d", len(cl.Items), len(want))
	}
	for i, w := range want {
		if cl.Items[i] != w {
			t.Errorf("item %d = %+v, want %+v", i, cl.Items[i], w)
		}
	}
}

func TestParseChangelogFallsBackAndDropsEmptyEntries(t *testing.T) {
	manifest := []byte(`{
	  "releaseTitle": {"ru": "Только по-русски"},
	  "releaseNotes": [
	    {"type": "fix", "ru": "Только русский"},
	    {"type": "fix"},
	    {"type": "fix", "ru": "   ", "en": "English only"}
	  ]
	}`)

	// German is not in the manifest, so everything falls back down the chain.
	cl, err := parseChangelog(manifest, "1.0.0", "de")
	if err != nil {
		t.Fatalf("parseChangelog: %v", err)
	}
	if cl.Title != "Только по-русски" {
		t.Errorf("title = %q, want the ru fallback", cl.Title)
	}
	if len(cl.Items) != 2 {
		t.Fatalf("got %d items, want 2 — the translation-less entry must be dropped", len(cl.Items))
	}
	if cl.Items[0].Text != "Только русский" {
		t.Errorf("item 0 = %q, want the ru fallback", cl.Items[0].Text)
	}
	// A whitespace-only ru value must not win over a real en one.
	if cl.Items[1].Text != "English only" {
		t.Errorf("item 1 = %q, want %q", cl.Items[1].Text, "English only")
	}
}

func TestParseChangelogRejectsBrokenManifest(t *testing.T) {
	if _, err := parseChangelog([]byte(`{"releaseNotes": "not an array"}`), "1.0.0", "ru"); err == nil {
		t.Fatal("expected an error for a manifest whose releaseNotes is not an array")
	}
}

func TestShouldShowChangelog(t *testing.T) {
	tests := []struct {
		name         string
		seen         string
		installed    string
		freshInstall bool
		want         bool
	}{
		{"already seen this build", "3.2.8", "3.2.8", false, false},
		{"updated since last shown", "3.2.7", "3.2.8", false, true},
		{"downgraded", "3.3.0", "3.2.8", false, true},
		{"fresh install stays quiet", "", "3.2.8", true, false},
		{"upgrade from a build predating the field", "", "3.2.8", false, true},
		{"unknown running version", "3.2.7", "", false, false},
		{"whitespace counts as unseen", "   ", "3.2.8", false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldShowChangelog(tc.seen, tc.installed, tc.freshInstall)
			if got != tc.want {
				t.Errorf("shouldShowChangelog(%q, %q, fresh=%v) = %v, want %v",
					tc.seen, tc.installed, tc.freshInstall, got, tc.want)
			}
		})
	}
}

func TestAckChangelogPersistsRunningVersion(t *testing.T) {
	app := NewApp()
	mgr := config.NewManager(config.NewCryptoServiceWithID("changelog-ack-test"))
	if err := mgr.Init(t.TempDir()); err != nil {
		t.Fatalf("config init: %v", err)
	}
	app.config = mgr

	if err := app.AckChangelog(); err != nil {
		t.Fatalf("AckChangelog: %v", err)
	}

	want := productVersionFromWailsJSON()
	if got := mgr.GetConfig().Settings.LastChangelogVersion; got != want {
		t.Errorf("LastChangelogVersion = %q, want %q", got, want)
	}
	if app.ShouldShowChangelog() {
		t.Error("the modal must not be due again after being acknowledged")
	}
}

// A brand-new install is seeded on its first launch. Without that, the config
// saved on launch one would carry an empty LastChangelogVersion into launch
// two, where WasCreatedFresh is false — and the new user would be shown notes
// for a release they never lived through.
func TestSeedChangelogVersionOnFreshInstall(t *testing.T) {
	dir := t.TempDir()

	app := NewApp()
	mgr := config.NewManager(config.NewCryptoServiceWithID("changelog-seed-test"))
	if err := mgr.Init(dir); err != nil {
		t.Fatalf("config init: %v", err)
	}
	app.config = mgr

	if !mgr.WasCreatedFresh() {
		t.Fatal("a config dir with no config file must report a fresh install")
	}
	if app.ShouldShowChangelog() {
		t.Error("a fresh install must not be greeted with release notes")
	}

	app.seedChangelogVersionOnFreshInstall()

	// Second launch: same directory, config file now exists.
	secondApp := NewApp()
	secondMgr := config.NewManager(config.NewCryptoServiceWithID("changelog-seed-test"))
	if err := secondMgr.Init(dir); err != nil {
		t.Fatalf("second config init: %v", err)
	}
	secondApp.config = secondMgr

	if secondMgr.WasCreatedFresh() {
		t.Fatal("a config dir with an existing config file must not report a fresh install")
	}
	if secondApp.ShouldShowChangelog() {
		t.Error("the second launch of a fresh install must stay quiet too")
	}
}

// An install upgraded from a build that predates LastChangelogVersion has a
// config file on disk with the field absent. That is the debut case: those
// users must see the modal.
func TestUpgradedInstallSeesChangelog(t *testing.T) {
	dir := t.TempDir()
	crypto := config.NewCryptoServiceWithID("changelog-upgrade-test")

	// Write a config the way an older build would have left it.
	seed := config.NewManager(crypto)
	if err := seed.Init(dir); err != nil {
		t.Fatalf("config init: %v", err)
	}
	if err := seed.SaveConfig(config.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "proxy_config.json")); err != nil {
		t.Fatalf("expected a config file on disk: %v", err)
	}

	app := NewApp()
	mgr := config.NewManager(crypto)
	if err := mgr.Init(dir); err != nil {
		t.Fatalf("reload config: %v", err)
	}
	app.config = mgr

	if mgr.GetConfig().Settings.LastChangelogVersion != "" {
		t.Fatal("precondition: the field must be absent in a pre-upgrade config")
	}
	app.seedChangelogVersionOnFreshInstall() // must be a no-op here
	if !app.ShouldShowChangelog() {
		t.Error("an upgraded install must see the release notes")
	}
}
