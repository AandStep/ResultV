package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func storedSalt(t *testing.T, dir string) (string, []byte, []byte) {
	t.Helper()
	salt, err := loadOrCreateInstallSalt(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, installSaltFile)
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, salt, onDisk
}

func TestInstallSaltSurvivesFailedRead(t *testing.T) {
	path, _, onDisk := storedSalt(t, t.TempDir())
	busy := func(string) ([]byte, error) { return nil, errors.New("file is busy") }

	if _, err := installSalt(path, busy); err == nil {
		t.Fatal("a salt that could not be read was silently replaced")
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(after, onDisk) {
		t.Fatal("a failed read rewrote the stored salt")
	}
}

func TestInstallSaltRetriesBriefReadFailure(t *testing.T) {
	path, salt, _ := storedSalt(t, t.TempDir())
	failures := 2
	flaky := func(p string) ([]byte, error) {
		if failures > 0 {
			failures--
			return nil, errors.New("file is busy")
		}
		return os.ReadFile(p)
	}

	got, err := installSalt(path, flaky)

	if err != nil || !bytes.Equal(got, salt) {
		t.Fatalf("salt after a brief read failure: equal=%v err=%v", bytes.Equal(got, salt), err)
	}
}
