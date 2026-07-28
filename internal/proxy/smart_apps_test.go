package proxy

import (
	"reflect"
	"testing"
)

func TestSmartRegistrableDomain(t *testing.T) {
	cases := map[string]string{
		"com.instagram.android":   "instagram.com",
		"ru.ozon.app.android":     "ozon.ru",
		"com.google.android.gm":   "google.com",
		"singleword":              "",
		"":                        "",
		"com.":                    "",
		".com":                    "", // empty first label
		"COM.Instagram.Android":   "instagram.com", // case normalization
		"  com.google.android  ": "google.com", // whitespace-padded (trimmed in SmartRegistrableDomain)
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

func TestMatchSmartPackages_DoesNotMatch_FalsePositiveRegressions(t *testing.T) {
	// Real registrable domains that ARE blocked in the RU list.
	blocked := map[string]bool{
		"instagram.com": true, "youtube.com": true, "tiktok.com": true,
		"x.com": true, "twitter.com": true, "facebook.com": true,
		"telegram.org": true, "t.me": true,
	}
	match := func(host string) bool { return blocked[host] }

	// Regression for the on-device over-matching: none of these vendors'
	// registrable domains are blocked, so none may be auto-tunnelled.
	apps := []string{
		"ru.ozon.app.android",            // ozon.ru (not blocked)
		"com.idamob.tinkoff.android",     // idamob.com (not blocked)
		"ru.yandex.mail",                 // yandex.ru (not blocked)
		"ru.yandex.yandexnavi",           // yandex.ru (not blocked)
		"ru.mts.mymts",                   // mts.ru (not blocked)
		"com.google.android.apps.photos", // google.com (not blocked)
		"com.google.android.apps.docs",   // google.com (not blocked)
		"com.wildberries.ru.app",         // wildberries.ru (not blocked)
	}
	got := MatchSmartPackages(apps, match)
	if len(got) != 0 {
		t.Fatalf("MatchSmartPackages (false-positive regression) = %v, want []", got)
	}
}

func TestMatchSmartPackages_AliasRespectsBlocklist(t *testing.T) {
	// If the region's list does not block tiktok.com, the aliased app must
	// not be forced into the tunnel. The alias check is merely a hint; the
	// domain must actually be blocked.
	blocked := map[string]bool{"instagram.com": true} // tiktok.com absent
	match := func(host string) bool { return blocked[host] }

	got := MatchSmartPackages([]string{"com.zhiliaoapp.musically"}, match)
	if len(got) != 0 {
		t.Fatalf("MatchSmartPackages (alias-but-not-blocked) = %v, want []", got)
	}
}

func TestMatchSmartPackages_DeduplicatesInput(t *testing.T) {
	// Duplicate packages in input should appear only once in output,
	// in the order of first appearance.
	blocked := map[string]bool{"instagram.com": true}
	match := func(host string) bool { return blocked[host] }

	got := MatchSmartPackages([]string{
		"com.instagram.android",
		"com.instagram.android", // duplicate
		"com.instagram.android", // another duplicate
	}, match)
	want := []string{"com.instagram.android"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MatchSmartPackages (dedup) = %v, want %v", got, want)
	}
}

func TestMatchSmartPackages_PreservesInputOrder(t *testing.T) {
	// Output order must match input order, not some other order.
	blocked := map[string]bool{
		"instagram.com": true, "twitter.com": true, "facebook.com": true,
	}
	match := func(host string) bool { return blocked[host] }

	got := MatchSmartPackages([]string{
		"com.facebook.katana",
		"com.instagram.android",
		"com.twitter.android",
	}, match)
	want := []string{"com.facebook.katana", "com.instagram.android", "com.twitter.android"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MatchSmartPackages (order) = %v, want %v", got, want)
	}
}
