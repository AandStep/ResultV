// Copyright (C) 2026 ResultProxy

package proxy

import (
	"testing"
)

func TestXHTTPPreferH2ALPN(t *testing.T) {
	got := xhttpPreferH2ALPN([]string{"h3", "h2", "http/1.1"})
	if len(got) < 3 || got[0] != "h2" || got[1] != "h3" {
		t.Fatalf("got %v", got)
	}
	if xhttpPreferH2ALPN([]string{"h2", "h3"})[0] != "h2" {
		t.Fatal("h2 first should stay")
	}
	empty := xhttpPreferH2ALPN(nil)
	if len(empty) < 1 || empty[0] != "h2" {
		t.Fatalf("default: %v", empty)
	}
}
