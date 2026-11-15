package proxy

import (
	"path/filepath"
	"testing"
	"time"

	"resultproxy-wails/internal/verdict"
)

func learnedManager(t *testing.T) *Manager {
	t.Helper()
	store := verdict.New([]byte("learned-salt"), nil)
	store.SetNamespace("test")
	return &Manager{verdicts: store, verdictPath: filepath.Join(t.TempDir(), "v.json")}
}

func TestLearnedVerdictsListsOnlyTunnelledNames(t *testing.T) {
	m := learnedManager(t)
	m.verdicts.Learn("blocked.example", verdict.Proxy)
	m.verdicts.LearnUnverified("guess.example")
	m.verdicts.Learn("fine.example", verdict.Direct)
	m.verdicts.Seed("user.example", verdict.Proxy, verdict.SourceUser)

	got := m.LearnedVerdicts()
	if len(got.Items) != 2 || got.Hidden != 0 {
		t.Fatalf("got %+v", got)
	}
	if got.Items[0].Host != "blocked.example" || got.Items[0].Unverified {
		t.Fatalf("first: %+v", got.Items[0])
	}
	if got.Items[1].Host != "guess.example" || !got.Items[1].Unverified {
		t.Fatalf("second: %+v", got.Items[1])
	}
	if got.Items[0].ExpiresAt <= time.Now().UnixMilli() {
		t.Fatalf("expiry not carried: %+v", got.Items[0])
	}
}

func TestLearnedVerdictsCountsNamesFromAnEarlierSession(t *testing.T) {
	m := learnedManager(t)
	m.verdicts.Learn("old.example", verdict.Proxy)
	if err := m.verdicts.Save(m.verdictPath); err != nil {
		t.Fatal(err)
	}
	reloaded, err := verdict.Load(m.verdictPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	reloaded.SetNamespace("test")
	m.verdicts = reloaded

	got := m.LearnedVerdicts()
	if len(got.Items) != 0 || got.Hidden != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestForgetLearnedVerdictPersists(t *testing.T) {
	m := learnedManager(t)
	m.verdicts.Learn("a.example.org", verdict.Proxy)
	m.verdicts.Learn("b.example.org", verdict.Proxy)
	m.ForgetLearnedVerdict("a.example.org")

	reloaded, _ := verdict.Load(m.verdictPath, nil)
	reloaded.SetNamespace("test")
	if _, ok := reloaded.Lookup("a.example.org"); ok {
		t.Fatal("forgotten name is still on disk")
	}
	if _, ok := reloaded.Lookup("b.example.org"); !ok {
		t.Fatal("neighbour was lost")
	}
}

func TestClearLearnedVerdictsEmptiesTheList(t *testing.T) {
	m := learnedManager(t)
	m.verdicts.Learn("a.example.org", verdict.Proxy)
	m.verdicts.Learn("b.example.org", verdict.Direct)
	m.ClearLearnedVerdicts()
	if got := m.LearnedVerdicts(); len(got.Items) != 0 || got.Hidden != 0 {
		t.Fatalf("got %+v", got)
	}
	if _, ok := m.verdicts.Lookup("b.example.org"); ok {
		t.Fatal("direct verdicts must go too")
	}
}
