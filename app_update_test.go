package main

import (
	"errors"
	"testing"
)

type stubKillSwitch struct {
	disableCalls int
	disableErr   error
}

func (s *stubKillSwitch) Enable(string, []string) error { return nil }
func (s *stubKillSwitch) Disable() error {
	s.disableCalls++
	return s.disableErr
}
func (s *stubKillSwitch) IsEnabled() bool { return false }

func TestPrepareForUpdateInstall_DisablesKillSwitchEvenWithoutProxy(t *testing.T) {
	app := NewApp()
	ks := &stubKillSwitch{}
	app.killSwitch = ks

	if err := app.prepareForUpdateInstall(); err != nil {
		t.Fatalf("prepareForUpdateInstall returned error: %v", err)
	}
	if ks.disableCalls != 1 {
		t.Fatalf("expected kill switch disable to be called once, got %d", ks.disableCalls)
	}
}

func TestPrepareForUpdateInstall_ReturnsDisableError(t *testing.T) {
	app := NewApp()
	ks := &stubKillSwitch{disableErr: errors.New("boom")}
	app.killSwitch = ks

	err := app.prepareForUpdateInstall()
	if err == nil {
		t.Fatal("expected error from kill switch disable")
	}
}
