package mobile

import (
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
	dir := t.TempDir()
	if _, err := InstallSmartSRSSeed(dir, []byte("this is not an SRS file at all")); err == nil {
		t.Fatal("expected an error for a non-SRS payload")
	}
	if _, err := os.Stat(proxy.SmartSRSPath(dir)); !os.IsNotExist(err) {
		t.Fatal("garbage seed must not reach disk")
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
