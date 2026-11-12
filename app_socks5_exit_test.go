package main

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/logger"
	"resultproxy-wails/internal/proxy"
)

func TestSocks5ExitSaveKeepsStoredSecret(t *testing.T) {
	dir := t.TempDir()
	newManager := func() *config.Manager {
		m := config.NewManager(config.NewCryptoServiceWithID("socks5-exit-storage-test"))
		if err := m.Init(dir); err != nil {
			t.Fatal(err)
		}
		return m
	}
	a := &App{config: newManager(), log: logger.New()}
	stale, err := a.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	password := "test-exit-password"
	if err := a.SetSocks5ExitEnabled(true); err != nil {
		t.Fatal(err)
	}
	pending, err := a.GetSocks5ExitSettings()
	if err != nil || !pending.Enabled || pending.Host != "" {
		t.Fatal("unable to enable SOCKS5 before filling its settings")
	}
	update := Socks5ExitSettingsUpdate{
		Host: "socks.example.test", Port: 8000, Username: "test-user",
		Password: &password,
	}
	if err := a.SaveSocks5ExitSettings(update); err != nil {
		t.Fatal(err)
	}
	update.Password = nil
	update.Port++
	if err := a.SaveSocks5ExitSettings(update); err == nil {
		t.Fatal("previous password would be reused at a new address")
	}
	update.Password = &password
	if err := a.SaveSocks5ExitSettings(update); err != nil {
		t.Fatal(err)
	}
	stale.Settings.Language = "ru"
	if err := a.SaveConfig(stale); err != nil {
		t.Fatal(err)
	}
	status, err := a.GetSocks5ExitSettings()
	if err != nil {
		t.Fatal(err)
	}
	if status.Port != 8001 || !status.Enabled {
		t.Fatal("saving stale app settings erased the exit profile")
	}
	if status.Password != password {
		t.Fatal("the dedicated settings view cannot restore the masked password")
	}
	cfg, err := a.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Settings.Socks5Exit != nil {
		t.Fatal("general UI config exposes the exit profile")
	}
	update.Port = 0
	if err := a.SaveSocks5ExitSettings(update); err == nil {
		t.Fatal("invalid profile update succeeded")
	}
	cfg = newManager().GetConfig()
	if profile := cfg.Settings.Socks5Exit; profile == nil || !profile.Enabled || profile.Port != 8001 || profile.Password != password {
		t.Fatal("protected settings failed to round-trip")
	}
	stored, err := os.ReadFile(filepath.Join(dir, "proxy_config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored), password) {
		t.Fatal("SOCKS5 password written in plaintext")
	}
	update.Port, update.Username = 8001, ""
	empty := ""
	update.Password = &empty
	if err := a.SaveSocks5ExitSettings(update); err != nil {
		t.Fatal(err)
	}
	status, err = a.GetSocks5ExitSettings()
	if err != nil || status.Password != "" {
		t.Fatalf("clearing stored password failed: %v", err)
	}
	if err := a.SetSocks5ExitEnabled(false); err != nil {
		t.Fatal(err)
	}
	if profile := newManager().GetConfig().Settings.Socks5Exit; profile.Enabled || profile.Host != update.Host {
		t.Fatal("disabling the switch lost the saved proxy address")
	}
}

func TestSocks5ExitSaveRoundTripsLists(t *testing.T) {
	m := config.NewManager(config.NewCryptoServiceWithID("socks5-exit-lists-app-test"))
	if err := m.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	a := &App{config: m}
	if err := a.SetSocks5ExitEnabled(true); err != nil {
		t.Fatal(err)
	}
	if err := a.SaveSocks5ExitSettings(Socks5ExitSettingsUpdate{
		Host: "192.0.2.1", Port: 1080,
		Sites: []string{"Example.COM", "example.com"}, Apps: []string{"tool.exe"},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := a.GetSocks5ExitSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Sites, []string{"example.com"}) || !slices.Equal(got.Apps, []string{"tool.exe"}) {
		t.Fatalf("lists = %q / %q", got.Sites, got.Apps)
	}
}

func TestSocks5ExitLogLine(t *testing.T) {
	for _, tc := range []struct {
		check proxy.Socks5ExitCheck
		line  string
		ok    bool
	}{
		{proxy.Socks5ExitCheck{OK: true, IP: "203.0.113.7", Country: "NL", UDP: true, LatencyMs: 412},
			"[SOCKS5] Выход работает: 203.0.113.7 (NL), UDP: есть, 412 мс", true},
		{proxy.Socks5ExitCheck{OK: true, IP: "203.0.113.7", Country: "NL", LatencyMs: 90},
			"[SOCKS5] Выход работает: 203.0.113.7 (NL), UDP: нет, 90 мс", true},
		{proxy.Socks5ExitCheck{Reason: proxy.Socks5CheckFirstHopDown}, "[SOCKS5] Выход не работает: VPN-узел не отвечает", false},
		{proxy.Socks5ExitCheck{Reason: proxy.Socks5CheckNoResponse}, "[SOCKS5] Выход не работает: SOCKS5-сервер не отвечает", false},
		{proxy.Socks5ExitCheck{Reason: proxy.Socks5CheckBadAuth}, "[SOCKS5] Выход не работает: неверный логин или пароль", false},
		{proxy.Socks5ExitCheck{Reason: proxy.Socks5CheckAuthMethod}, "[SOCKS5] Выход не работает: сервер требует другой способ входа", false},
		{proxy.Socks5ExitCheck{Reason: proxy.Socks5CheckRejected, Code: 5}, "[SOCKS5] Выход не работает: сервер отказал в соединении (код 5)", false},
		{proxy.Socks5ExitCheck{Reason: proxy.Socks5CheckProbeFailed}, "[SOCKS5] Выход не работает: сайт проверки не ответил", false},
	} {
		line, ok := socks5ExitLogLine(tc.check)
		if line != tc.line || ok != tc.ok {
			t.Errorf("got %q/%t, want %q/%t", line, ok, tc.line, tc.ok)
		}
	}
}

func TestSocks5ExitStatusIsPerSession(t *testing.T) {
	stored := Socks5ExitStatusDTO{Socks5ExitCheck: proxy.Socks5ExitCheck{OK: true}, Checked: true}
	if got := statusForSession(1, 1, stored); !got.Checked {
		t.Fatal("status of the current session was dropped")
	}
	if got := statusForSession(1, 2, stored); got.Checked {
		t.Fatal("status of a previous session leaked into the new one")
	}
	if got := statusForSession(0, 0, stored); got.Checked {
		t.Fatal("status shown while disconnected")
	}
}

func TestSocks5ExitClearingAllFieldsPersists(t *testing.T) {
	dir := t.TempDir()
	newApp := func() *App {
		m := config.NewManager(config.NewCryptoServiceWithID("socks5-exit-clear-test"))
		if err := m.Init(dir); err != nil {
			t.Fatal(err)
		}
		return &App{config: m}
	}
	a := newApp()
	if err := a.SetSocks5ExitEnabled(true); err != nil {
		t.Fatal(err)
	}
	password := "test-password"
	if err := a.SaveSocks5ExitSettings(Socks5ExitSettingsUpdate{
		Host: "socks.example.test", Port: 8000, Username: "test-user", Password: &password,
	}); err != nil {
		t.Fatal(err)
	}
	empty := ""
	if err := a.SaveSocks5ExitSettings(Socks5ExitSettingsUpdate{Password: &empty}); err != nil {
		t.Fatalf("clearing all fields: %v", err)
	}
	requireEmpty := func() {
		t.Helper()
		got, err := a.GetSocks5ExitSettings()
		if err != nil || !reflect.DeepEqual(got, Socks5ExitSettingsDTO{Enabled: true}) {
			t.Fatal("clearing must persist an empty profile with the toggle still on")
		}
	}
	requireEmpty()
	for _, enabled := range []bool{false, true} {
		if err := a.SetSocks5ExitEnabled(enabled); err != nil {
			t.Fatal(err)
		}
	}
	a = newApp()
	requireEmpty()
}
