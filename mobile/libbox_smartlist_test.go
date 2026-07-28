package mobile

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"resultproxy-wails/internal/proxy"
)

func writeSeedSRS(t *testing.T, domains []string) []byte {
	t.Helper()
	dir := t.TempDir()
	path := proxy.SmartSRSPath(dir)
	if err := proxy.CompileSmartSRS(domains, path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestInstallSmartSRSSeed_InstallsThenSkips(t *testing.T) {
	dir := t.TempDir()
	seed := writeSeedSRS(t, []string{"instagram.com", "youtube.com"})

	installed, err := InstallSmartSRSSeed(dir, seed)
	if err != nil || !installed {
		t.Fatalf("first install: installed=%v err=%v", installed, err)
	}
	// Second call must be a no-op — a downloaded list must never be
	// overwritten by the (older) bundled seed.
	installed, err = InstallSmartSRSSeed(dir, seed)
	if err != nil || installed {
		t.Fatalf("second install should be a no-op: installed=%v err=%v", installed, err)
	}
}

func TestInstallSmartSRSSeed_RejectsGarbage(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		// Below minSmartSRSBytes (32): exercises the trivial size guard only.
		{"tooSmall", []byte("this is not an SRS file at all")}, // 30 bytes
		// At/above minSmartSRSBytes but still not a valid rule-set: this is the
		// case that actually proves validateSRS runs BEFORE the write. A
		// regression that reordered validation after os.WriteFile would pass
		// the tooSmall case above undetected.
		{"largeButInvalid", bytes.Repeat([]byte("not a real srs file, just padding to clear the size guard "), 3)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if _, err := InstallSmartSRSSeed(dir, tc.data); err == nil {
				t.Fatal("expected an error for a non-SRS payload")
			}
			if _, err := os.Stat(proxy.SmartSRSPath(dir)); !os.IsNotExist(err) {
				t.Fatal("garbage seed must not reach disk")
			}
		})
	}
}

func TestSmartListStatus_ReflectsDisk(t *testing.T) {
	dir := t.TempDir()
	raw, err := SmartListStatus(dir)
	if err != nil {
		t.Fatal(err)
	}
	var st struct {
		SRSReady bool `json:"srsReady"`
	}
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		t.Fatal(err)
	}
	if st.SRSReady {
		t.Fatal("empty dir should report srsReady=false")
	}
	if _, err := InstallSmartSRSSeed(dir, writeSeedSRS(t, []string{"x.com"})); err != nil {
		t.Fatal(err)
	}
	raw, _ = SmartListStatus(dir)
	json.Unmarshal([]byte(raw), &st)
	if !st.SRSReady {
		t.Fatal("after seeding, srsReady must be true")
	}
}

func TestMatchSmartApps(t *testing.T) {
	dir := t.TempDir()
	if _, err := InstallSmartSRSSeed(dir, writeSeedSRS(t, []string{"instagram.com", "youtube.com"})); err != nil {
		t.Fatal(err)
	}
	got, err := MatchSmartApps(
		"com.instagram.android,com.google.android.youtube,ru.sberbank.online",
		dir,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "com.instagram.android,com.google.android.youtube"
	if got != want {
		t.Fatalf("MatchSmartApps = %q, want %q", got, want)
	}
}

func TestMatchSmartApps_NoSRSReturnsEmpty(t *testing.T) {
	got, err := MatchSmartApps("com.instagram.android", t.TempDir())
	if err != nil {
		t.Fatalf("missing SRS must not be an error, got %v", err)
	}
	if strings.TrimSpace(got) != "" {
		t.Fatalf("expected empty result, got %q", got)
	}
}

// TestMatchSmartApps_TrimsWhitespacePaddedAliasedPackage guards the exact bug
// a missing per-entry trim causes: proxy.MatchSmartPackages deliberately does
// NOT trim (fidelity with the Kotlin original — see Task 3), so CSV hygiene is
// MatchSmartApps's job. DefaultSmartAliases does an exact map lookup on the
// package name; an untrimmed " com.google.android.youtube " misses that
// lookup and falls through to the naive two-label heuristic, which computes
// "google.com" instead of "youtube.com" — a silent false negative on exactly
// the apps the alias table exists to catch.
func TestMatchSmartApps_TrimsWhitespacePaddedAliasedPackage(t *testing.T) {
	dir := t.TempDir()
	if _, err := InstallSmartSRSSeed(dir, writeSeedSRS(t, []string{"youtube.com"})); err != nil {
		t.Fatal(err)
	}
	got, err := MatchSmartApps(" com.google.android.youtube ", dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "com.google.android.youtube" {
		t.Fatalf("MatchSmartApps(%q) = %q, want the trimmed aliased package to match via youtube.com",
			" com.google.android.youtube ", got)
	}
}

// TestShouldCompileSmartSRS pins the decision predicate that guards
// FetchSmartList's SRS compile step (CRITICAL finding: a failed refresh must
// not silently destroy a good rule-set with the 7-domain builtin fallback).
func TestShouldCompileSmartSRS(t *testing.T) {
	cases := []struct {
		name             string
		source           string
		hasDomains       bool
		existingSRSReady bool
		want             bool
	}{
		{"builtin_over_existing_good_srs_is_refused", "builtin", true, true, false},
		{"builtin_installed_when_nothing_on_disk", "builtin", true, false, true},
		{"remote_always_compiles_over_existing", "remote", true, true, true},
		{"remote_compiles_when_nothing_on_disk", "remote", true, false, true},
		{"cache_always_compiles_over_existing", "cache", true, true, true},
		{"no_domains_never_compiles", "remote", false, true, false},
		{"no_domains_never_compiles_builtin", "builtin", false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldCompileSmartSRS(tc.source, tc.hasDomains, tc.existingSRSReady)
			if got != tc.want {
				t.Fatalf("shouldCompileSmartSRS(%q, %v, %v) = %v, want %v",
					tc.source, tc.hasDomains, tc.existingSRSReady, got, tc.want)
			}
		})
	}
}

// TestCompileSmartResult_BuiltinDoesNotOverwriteExistingSRS is the CRITICAL
// regression case: FetchSmartList used to compile whatever
// ResolveBlockedDomains returned, including the tiny "builtin" fallback
// (defaultBlockedDomains(), 7 domains) that ResolveBlockedDomains produces
// when both the network fetch and the on-disk smart-blocked.json cache are
// unavailable — exactly the fresh-install case the bundled seed SRS exists to
// serve. That silently overwrote a good (509 KB seed or previously
// downloaded) rule-set with ~200 bytes. Assert the on-disk bytes are
// byte-for-byte unchanged after a builtin result arrives.
func TestCompileSmartResult_BuiltinDoesNotOverwriteExistingSRS(t *testing.T) {
	dir := t.TempDir()
	path := proxy.SmartSRSPath(dir)
	if err := proxy.CompileSmartSRS([]string{"instagram.com", "youtube.com", "x.com"}, path); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	result := proxy.BlockedDomainsResolveResult{
		Domains: []string{"instagram.com", "facebook.com", "twitter.com", "x.com", "t.me", "discord.com", "netflix.com"},
		Source:  "builtin",
	}
	srsReady := compileSmartResult(&result, dir)
	if !srsReady {
		t.Fatal("existing SRS should still be reported ready even though the builtin result was refused")
	}
	if result.Err != nil {
		t.Fatalf("refusing to compile is not an error condition: %v", result.Err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("a builtin fallback result must not overwrite an existing usable SRS")
	}
}

// TestCompileSmartResult_BuiltinInstalledWhenNoSRSExists covers the cold-start
// side of the same guard: some blocking beats none, so the builtin fallback
// must still be installed when there is no usable SRS on disk at all — this
// mirrors the existing Kotlin seed-install guard's behaviour.
func TestCompileSmartResult_BuiltinInstalledWhenNoSRSExists(t *testing.T) {
	dir := t.TempDir()
	result := proxy.BlockedDomainsResolveResult{
		Domains: []string{"instagram.com", "facebook.com", "twitter.com"},
		Source:  "builtin",
	}
	srsReady := compileSmartResult(&result, dir)
	if !srsReady {
		t.Fatal("builtin result must be compiled when no usable SRS exists yet")
	}
	if !proxy.SmartSRSReady(dir) {
		t.Fatal("expected a usable SRS on disk after compiling the builtin fallback")
	}
}

// TestCompileSmartResult_RemoteAlwaysOverwrites locks in that "remote" (and by
// extension "cache") results are never subject to the builtin guard: they
// always compile and replace whatever was on disk, exactly as before this fix.
func TestCompileSmartResult_RemoteAlwaysOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := proxy.SmartSRSPath(dir)
	if err := proxy.CompileSmartSRS([]string{"instagram.com"}, path); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	result := proxy.BlockedDomainsResolveResult{
		Domains: []string{"instagram.com", "youtube.com", "x.com", "vk.com"},
		Source:  "remote",
	}
	srsReady := compileSmartResult(&result, dir)
	if !srsReady {
		t.Fatal("remote result should compile successfully")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(before, after) {
		t.Fatal("a remote result must overwrite the existing SRS, even though a usable one already existed")
	}
}

// TestMatchSmartApps_CSVHygiene locks in the empty/blank-token handling that
// splitSmartList already does for the domain list — MatchSmartApps must apply
// the same trim-and-drop-empty rule to the packages CSV so no empty package
// name ever reaches proxy.MatchSmartPackages.
func TestMatchSmartApps_CSVHygiene(t *testing.T) {
	dir := t.TempDir()
	if _, err := InstallSmartSRSSeed(dir, writeSeedSRS(t, []string{"instagram.com"})); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		csv  string
		want string
	}{
		{"empty", "", ""},
		{"trailingComma", "com.instagram.android,", "com.instagram.android"},
		{"repeatedCommas", "com.instagram.android,,,ru.sberbank.online", "com.instagram.android"},
		{"leadingComma", ",com.instagram.android", "com.instagram.android"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := MatchSmartApps(tc.csv, dir)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("MatchSmartApps(%q) = %q, want %q", tc.csv, got, tc.want)
			}
		})
	}
}
