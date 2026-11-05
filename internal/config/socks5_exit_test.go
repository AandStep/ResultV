package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeSocks5ExitConfig(t *testing.T) {
	input := Socks5ExitConfig{
		Host: " пример.рф. ", Port: 8000, Username: "test-user", Password: "test-password",
	}
	got, err := NormalizeSocks5ExitConfig(input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Host != "xn--e1afmkfd.xn--p1ai" {
		t.Fatalf("unexpected host: %q", got.Host)
	}
	if got.Username != input.Username || got.Password != input.Password {
		t.Fatal("normalization changed credentials or modified the input slice")
	}

	for _, invalid := range []struct {
		name   string
		change func(*Socks5ExitConfig)
	}{
		{"zero port", func(c *Socks5ExitConfig) { c.Port = 0 }},
		{"out of range port", func(c *Socks5ExitConfig) { c.Port = 65536 }},
		{"URL as host", func(c *Socks5ExitConfig) { c.Host = "socks5://test.invalid" }},
		{"empty host", func(c *Socks5ExitConfig) { c.Host = "" }},
		{"wildcard", func(c *Socks5ExitConfig) { c.Host = "*.example.com" }},
		{"empty DNS label", func(c *Socks5ExitConfig) { c.Host = "a..example.com" }},
		{"partial auth", func(c *Socks5ExitConfig) { c.Password = "" }},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			cfg := input
			invalid.change(&cfg)
			if _, err := NormalizeSocks5ExitConfig(cfg); err == nil {
				t.Fatal("invalid profile was accepted")
			}
		})
	}
}

func TestSocks5ExitConfigLegacyAndExport(t *testing.T) {
	var legacy AppConfig
	if err := json.Unmarshal([]byte(`{"settings":{"mode":"tunnel"}}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Settings.Socks5Exit != nil {
		t.Fatal("legacy config unexpectedly has an exit profile")
	}
	legacy.Settings.Socks5Exit = &Socks5ExitConfig{
		Host: "socks.example.test", Port: 8000, Username: "export-test-user", Password: "export-test-password",
	}
	export, err := ExportConfig(legacy, "local-test-key")
	if err != nil {
		t.Fatal(err)
	}
	imported, err := ImportConfig(export, "local-test-key")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(imported)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"socks.example.test", "export-test-user", "export-test-password"} {
		if strings.Contains(string(raw), value) {
			t.Fatal("device-only SOCKS5 profile was exported")
		}
	}
}
