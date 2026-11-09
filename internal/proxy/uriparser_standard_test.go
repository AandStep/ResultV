package proxy

import (
	"strings"
	"testing"
)

func TestParseStandardProxy(t *testing.T) {
	for _, tt := range []struct {
		input, kind, host, user, password string
		port                              int
	}{
		{"socks5://user:p%40ss@192.0.2.5:8000#Office%20proxy", "SOCKS5", "192.0.2.5", "user", "p@ss", 8000},
		{"socks5://user:123:part:word@192.0.2.5:8000", "SOCKS5", "192.0.2.5", "user", "123:part:word", 8000},
		{"SOCKS5H://proxy.example:8000/", "SOCKS5", "proxy.example", "", "", 8000},
		{"socks://192.0.2.5", "SOCKS5", "192.0.2.5", "", "", 1080},
		{"socks4://user@192.0.2.5", "SOCKS4", "192.0.2.5", "user", "", 1080},
		{"socks4a://proxy.example", "SOCKS4A", "proxy.example", "", "", 1080},
		{"http://user:pass@proxy.example:3128", "HTTP", "proxy.example", "user", "pass", 3128},
		{"https://proxy.example", "HTTPS", "proxy.example", "", "", 443},
		{"192.0.2.5", "SOCKS5", "192.0.2.5", "", "", 1080},
		{"proxy.example:8000", "SOCKS5", "proxy.example", "", "", 8000},
		{"[2001:db8::5]:8000", "SOCKS5", "2001:db8::5", "", "", 8000},
		{"2001:db8::5", "SOCKS5", "2001:db8::5", "", "", 1080},
		{"user:pass@192.0.2.5:8000", "SOCKS5", "192.0.2.5", "user", "pass", 8000},
		{"user:pass@proxy.example", "SOCKS5", "proxy.example", "user", "pass", 1080},
		{"192.0.2.5:8000:user:p@ss:word", "SOCKS5", "192.0.2.5", "user", "p@ss:word", 8000},
		{"[2001:db8::5]:8000:user:pass", "SOCKS5", "2001:db8::5", "user", "pass", 8000},
	} {
		t.Run(tt.input, func(t *testing.T) {
			entry, err := ParseProxyURI(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if entry.Type != tt.kind || entry.IP != tt.host || entry.Port != tt.port ||
				entry.Username != tt.user || entry.Password != tt.password {
				t.Fatalf("unexpected proxy entry: %+v", entry)
			}
			if strings.Contains(tt.input, "#") && entry.Name != "Office proxy" {
				t.Fatalf("unexpected name %q", entry.Name)
			}
			out := buildProxyOutbound(proxyConfigFromEntry(entry))
			if out.Server != tt.host || out.ServerPort != tt.port || out.Username != tt.user || out.Password != tt.password {
				t.Fatalf("unexpected outbound: %+v", out)
			}
			if strings.HasPrefix(tt.kind, "SOCKS") {
				if out.Type != "socks" || out.Version != strings.ToLower(strings.TrimPrefix(tt.kind, "SOCKS")) {
					t.Fatalf("unexpected SOCKS outbound: %+v", out)
				}
			} else if out.Type != "http" || (out.TLS != nil && out.TLS.Enabled) != (tt.kind == "HTTPS") {
				t.Fatalf("unexpected HTTP outbound: %+v", out)
			}
		})
	}
}

func TestParseStandardProxyRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{
		"not a proxy", "vless", "user@example.test", "ftp://192.0.2.5:80", "socks5://:1080", "socks5://192.0.2.5:",
		"192.0.2.5:0", "192.0.2.5:65536", "http://example.test/sub", "http://example.test/?token=x",
		"192.0.2.5:8000:user:p@proxy.example:9000",
		"192.0.2.5:8000:user@example.test:p@proxy.example:9000",
		"socks5://user:secret@192.0.2.5:bad",
	} {
		if _, err := ParseProxyURI(input); err == nil || strings.Contains(err.Error(), "secret") {
			t.Errorf("expected a safe parse error for invalid input, got %v", err)
		}
	}
}
