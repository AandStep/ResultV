package verdict

import (
	"net/netip"
	"testing"
	"time"
)

func TestUnverifiedProxyExpiresInHours(t *testing.T) {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	s := testStore(&now)
	s.SetNamespace("home")

	s.LearnUnverified("slow.example")
	rec, ok := s.Lookup("slow.example")
	if !ok || rec.Decision != Proxy {
		t.Fatalf("got %v, ok=%v", rec.Decision, ok)
	}
	now = now.Add(TTLUnverified + time.Minute)
	if _, ok := s.Lookup("slow.example"); ok {
		t.Fatal("an unverified proxy verdict must not outlive TTLUnverified")
	}
}

func TestUnverifiedProxyDoesNotVoteForParent(t *testing.T) {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	s := testStore(&now)
	s.SetNamespace("home")

	s.LearnUnverified("a.cdn.example")
	s.LearnUnverified("b.cdn.example")
	s.LearnUnverified("c.cdn.example")
	if _, ok := s.Lookup("d.cdn.example"); ok {
		t.Fatal("unverified verdicts must not promote")
	}
}

func TestUnverifiedIPProxyExpiresInHours(t *testing.T) {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	s := testStore(&now)
	s.SetNamespace("home")
	addr := netip.MustParseAddr("203.0.113.7")

	s.LearnIPUnverified(addr)
	if rec, ok := s.LookupIP(addr); !ok || rec.Decision != Proxy {
		t.Fatalf("got %v, ok=%v", rec.Decision, ok)
	}
	now = now.Add(TTLUnverified + time.Minute)
	if _, ok := s.LookupIP(addr); ok {
		t.Fatal("expired")
	}
}

func TestPromotionStopsAtPublicSuffix(t *testing.T) {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	s := testStore(&now)
	s.SetNamespace("home")

	for _, h := range []string{"one.co.uk", "two.co.uk", "three.co.uk"} {
		s.Learn(h, Proxy)
	}
	if _, ok := s.Lookup("four.co.uk"); ok {
		t.Fatal("co.uk is a public suffix")
	}
	for _, h := range []string{"d1.cloudfront.net", "d2.cloudfront.net", "d3.cloudfront.net"} {
		s.Learn(h, Proxy)
	}
	if _, ok := s.Lookup("d4.cloudfront.net"); ok {
		t.Fatal("cloudfront.net is a public suffix")
	}
}

func TestPromotionVotesAgeOut(t *testing.T) {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	s := testStore(&now)
	s.SetNamespace("home")

	s.Learn("a.site.example", Proxy)
	s.Learn("b.site.example", Proxy)
	now = now.Add(PromoteWindow + time.Hour)
	s.Learn("c.site.example", Proxy)
	if _, ok := s.Lookup("d.site.example"); ok {
		t.Fatal("votes older than PromoteWindow must not count")
	}
}

func TestForgetDropsOneLearnedName(t *testing.T) {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	s := testStore(&now)
	s.SetNamespace("home")

	s.Learn("a.example.org", Proxy)
	s.Learn("b.example.org", Proxy)
	s.Forget("a.example.org")
	if _, ok := s.Lookup("a.example.org"); ok {
		t.Fatal("forgotten name still answers")
	}
	if _, ok := s.Lookup("b.example.org"); !ok {
		t.Fatal("neighbour was dropped")
	}
}

func TestForgetDropsItsPromotionVote(t *testing.T) {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	s := testStore(&now)
	s.SetNamespace("home")

	s.Learn("a.site.example", Proxy)
	s.Learn("b.site.example", Proxy)
	s.Forget("a.site.example")
	s.Learn("c.site.example", Proxy)
	if _, ok := s.Lookup("d.site.example"); ok {
		t.Fatal("a forgotten child still voted")
	}
}

func TestClearLearnedKeepsSeededRecords(t *testing.T) {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	s := testStore(&now)
	s.SetNamespace("home")

	s.Seed("user.example", Proxy, SourceUser)
	s.Learn("learned.example", Proxy)
	if n := s.ClearLearned(); n != 1 {
		t.Fatalf("cleared %d, want 1", n)
	}
	if _, ok := s.Lookup("learned.example"); ok {
		t.Fatal("learned record survived")
	}
	if _, ok := s.Lookup("user.example"); !ok {
		t.Fatal("user record was dropped")
	}
}

func TestLearnedCountIncludesNamesUnknownThisSession(t *testing.T) {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	s := testStore(&now)
	s.SetNamespace("home")
	s.Learn("a.example.org", Proxy)
	s.Learn("b.example.org", Direct)
	s.Seed("user.example", Proxy, SourceUser)
	if n := s.LearnedCount(Proxy); n != 1 {
		t.Fatalf("got %d, want 1", n)
	}
}
