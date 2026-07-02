package proxy

import (
	"strings"
	"testing"
)

func TestStripMetaCSP_RemovesMetaCSPTag(t *testing.T) {
	body := `<html><head><meta http-equiv="Content-Security-Policy" content="script-src 'self'"><title>t</title></head><body></body></html>`
	got := stripMetaCSP(body)
	if strings.Contains(got, "Content-Security-Policy") {
		t.Fatalf("expected meta CSP tag to be stripped, got: %s", got)
	}
	if !strings.Contains(got, "<title>t</title>") {
		t.Fatalf("expected rest of body to survive stripping, got: %s", got)
	}
}

func TestStripMetaCSP_ReportOnlyVariantAlsoStripped(t *testing.T) {
	body := `<meta http-equiv="Content-Security-Policy-Report-Only" content="default-src 'none'">`
	got := stripMetaCSP(body)
	if strings.Contains(got, "Content-Security-Policy") {
		t.Fatalf("expected report-only meta CSP tag to be stripped, got: %s", got)
	}
}

func TestStripMetaCSP_CaseInsensitive(t *testing.T) {
	body := `<META HTTP-EQUIV="content-security-policy" CONTENT="default-src 'self'">`
	got := stripMetaCSP(body)
	if strings.Contains(strings.ToLower(got), "content-security-policy") {
		t.Fatalf("expected case-insensitive stripping, got: %s", got)
	}
}

func TestStripMetaCSP_NoMetaCSP_BodyUnchanged(t *testing.T) {
	body := `<html><head><title>no csp here</title></head></html>`
	got := stripMetaCSP(body)
	if got != body {
		t.Fatalf("expected body unchanged when no meta CSP present, got: %s", got)
	}
}
