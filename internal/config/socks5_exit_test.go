package config

import (
	"encoding/json"
	"slices"
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

func TestNormalizeSocks5ExitLists(t *testing.T) {
	got, err := NormalizeSocks5ExitConfig(Socks5ExitConfig{
		Host: "192.0.2.1", Port: 1080,
		Sites: []string{" Example.COM. ", "example.com", "", "10.0.0.0/8"},
		Apps:  []string{"Discord.exe", "discord.exe", " "},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Sites, []string{"example.com", "10.0.0.0/8"}) {
		t.Fatalf("sites = %q", got.Sites)
	}
	if !slices.Equal(got.Apps, []string{"Discord.exe"}) {
		t.Fatalf("apps = %q", got.Apps)
	}
}

func TestSaveSocks5ExitKeepsListsWithoutAddress(t *testing.T) {
	m := NewManager(NewCryptoServiceWithID("socks5-exit-lists-test"))
	if err := m.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := m.SetSocks5ExitEnabled(true); err != nil {
		t.Fatal(err)
	}
	if err := m.SaveSocks5Exit(Socks5ExitConfig{Sites: []string{"Example.com"}}, nil); err != nil {
		t.Fatal(err)
	}
	p := m.GetConfig().Settings.Socks5Exit
	if p == nil || !p.Enabled || p.Host != "" || !slices.Equal(p.Sites, []string{"example.com"}) {
		t.Fatalf("stored profile = %+v", p)
	}
	if p.IsActive() {
		t.Fatal("a profile without an address must not be active")
	}
}

func TestSameSocks5ExitLists(t *testing.T) {
	a := &Socks5ExitConfig{Sites: []string{"example.com"}}
	for _, c := range []struct {
		name string
		x, y *Socks5ExitConfig
		want bool
	}{
		{"nil and empty", nil, &Socks5ExitConfig{}, true},
		{"same sites", a, &Socks5ExitConfig{Sites: []string{"example.com"}}, true},
		{"apps differ", &Socks5ExitConfig{Apps: []string{"tool.exe"}}, &Socks5ExitConfig{}, false},
		{"sites differ", a, &Socks5ExitConfig{Sites: []string{"example.org"}}, false},
	} {
		if got := SameSocks5ExitLists(c.x, c.y); got != c.want {
			t.Errorf("%s: got %t", c.name, got)
		}
	}
	if (&Socks5ExitConfig{}).HasLists() || (*Socks5ExitConfig)(nil).HasLists() || !a.HasLists() {
		t.Fatal("HasLists")
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
