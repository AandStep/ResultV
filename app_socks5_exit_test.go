package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/logger"
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
