package proxy

import (
	"reflect"
	"testing"
)

func TestSmartRegistrableDomain(t *testing.T) {
	cases := map[string]string{
		"com.instagram.android": "instagram.com",
		"ru.ozon.app.android":   "ozon.ru",
		"com.google.android.gm": "google.com",
		"singleword":            "",
		"":                      "",
		"com.":                  "",
	}
	for pkg, want := range cases {
		if got := SmartRegistrableDomain(pkg); got != want {
			t.Errorf("SmartRegistrableDomain(%q) = %q, want %q", pkg, got, want)
		}
	}
}

func TestMatchSmartPackages_ReverseDNSAndAliases(t *testing.T) {
	blocked := map[string]bool{"instagram.com": true, "youtube.com": true, "tiktok.com": true}
	match := func(host string) bool { return blocked[host] }

	got := MatchSmartPackages([]string{
		"com.instagram.android",     // reverse-DNS hit
		"com.google.android.youtube", // alias hit
		"com.zhiliaoapp.musically",   // alias hit
		"ru.sberbank.online",         // must NOT match (bank)
		"com.google.android.gm",      // must NOT match (vendor domain not blocked)
		"singleword",                 // no registrable domain
	}, match)

	want := []string{"com.instagram.android", "com.google.android.youtube", "com.zhiliaoapp.musically"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MatchSmartPackages = %v, want %v", got, want)
	}
}

func TestMatchSmartPackages_UsesCompiledSRS(t *testing.T) {
	dir := t.TempDir()
	if err := CompileSmartSRS([]string{"instagram.com", "youtube.com"}, SmartSRSPath(dir)); err != nil {
		t.Fatal(err)
	}
	m, err := LoadSmartDomainMatcher(SmartSRSPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	got := MatchSmartPackages(
		[]string{"com.instagram.android", "com.google.android.youtube", "ru.sberbank.online"},
		m.Match,
	)
	want := []string{"com.instagram.android", "com.google.android.youtube"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MatchSmartPackages = %v, want %v", got, want)
	}
}
