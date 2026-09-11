package config

import (
	"testing"
	"time"
)

func TestEffectivePingTypeDefaultsToAuto(t *testing.T) {
	for _, raw := range []string{"", "   ", "nonsense", "TCP"} {
		s := AppSettings{PingType: raw}
		if got := s.EffectivePingType(); got != PingTypeAuto {
			t.Fatalf("PingType %q: got %q, want %q", raw, got, PingTypeAuto)
		}
	}
}

func TestEffectivePingTypeKeepsKnownValues(t *testing.T) {
	for _, want := range []string{PingTypeAuto, PingTypeICMP, PingTypeHTTPGet, PingTypeHTTPHead} {
		s := AppSettings{PingType: want}
		if got := s.EffectivePingType(); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
}

func TestEffectivePingTimeoutClamps(t *testing.T) {
	cases := []struct {
		in   int
		want time.Duration
	}{
		{0, 3 * time.Second},   // не задано → дефолт
		{-5, 3 * time.Second},  // мусор → дефолт
		{1, 1 * time.Second},   // нижняя граница
		{7, 7 * time.Second},   // внутри диапазона
		{10, 10 * time.Second}, // верхняя граница
		{99, 10 * time.Second}, // выше потолка → потолок
	}
	for _, c := range cases {
		s := AppSettings{PingTimeoutSec: c.in}
		if got := s.EffectivePingTimeout(); got != c.want {
			t.Fatalf("PingTimeoutSec %d: got %v, want %v", c.in, got, c.want)
		}
	}
}

func TestEffectivePingTestURLFallsBackToDefault(t *testing.T) {
	for _, raw := range []string{"", "   ", "http://example.com", "ftp://x", "https://", "not a url"} {
		s := AppSettings{PingTestURL: raw}
		if got := s.EffectivePingTestURL(); got != DefaultPingTestURL {
			t.Fatalf("PingTestURL %q: got %q, want default", raw, got)
		}
	}
}

func TestEffectivePingTestURLKeepsValidHTTPS(t *testing.T) {
	s := AppSettings{PingTestURL: "  https://cp.cloudflare.com/generate_204  "}
	if got := s.EffectivePingTestURL(); got != "https://cp.cloudflare.com/generate_204" {
		t.Fatalf("got %q", got)
	}
}

func TestValidatePingTestURLRejectsPlainHTTP(t *testing.T) {
	if err := ValidatePingTestURL("http://www.gstatic.com/generate_204"); err == nil {
		t.Fatal("plain http must be rejected: a local listener can forge its answer")
	}
	if err := ValidatePingTestURL("https://www.gstatic.com/generate_204"); err != nil {
		t.Fatalf("valid https rejected: %v", err)
	}
}
