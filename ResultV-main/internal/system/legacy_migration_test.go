package system

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateLegacyFilesOldOnly(t *testing.T) {
	base := t.TempDir()
	newDir := filepath.Join(base, "ResultV")
	legacyDir := filepath.Join(base, "ResultProxy")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "proxy_config.json"), []byte("legacy"), 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	if err := migrateLegacyFiles(newDir, legacyDir, []string{"proxy_config.json", ".machine-fallback-id"}); err != nil {
		t.Fatalf("migrateLegacyFiles: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(newDir, "proxy_config.json"))
	if err != nil {
		t.Fatalf("read migrated config: %v", err)
	}
	if string(data) != "legacy" {
		t.Fatalf("unexpected migrated config: %q", string(data))
	}
	if _, err := os.Stat(filepath.Join(legacyDir, "proxy_config.json")); !os.IsNotExist(err) {
		t.Fatalf("legacy file must be removed, err=%v", err)
	}
}

func TestMigrateLegacyFilesKeepsNewConfig(t *testing.T) {
	base := t.TempDir()
	newDir := filepath.Join(base, "ResultV")
	legacyDir := filepath.Join(base, "ResultProxy")
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatalf("mkdir new: %v", err)
	}
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	if err := os.WriteFile(filepath.Join(newDir, "proxy_config.json"), []byte("new"), 0o600); err != nil {
		t.Fatalf("write new config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "proxy_config.json"), []byte("legacy"), 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "blocked_cache.json"), []byte("cache"), 0o600); err != nil {
		t.Fatalf("write legacy cache: %v", err)
	}

	if err := migrateLegacyFiles(newDir, legacyDir, []string{"proxy_config.json", "blocked_cache.json"}); err != nil {
		t.Fatalf("migrateLegacyFiles: %v", err)
	}

	newConfig, err := os.ReadFile(filepath.Join(newDir, "proxy_config.json"))
	if err != nil {
		t.Fatalf("read new config: %v", err)
	}
	if string(newConfig) != "new" {
		t.Fatalf("new config must win, got: %q", string(newConfig))
	}
	cache, err := os.ReadFile(filepath.Join(newDir, "blocked_cache.json"))
	if err != nil {
		t.Fatalf("read migrated cache: %v", err)
	}
	if string(cache) != "cache" {
		t.Fatalf("unexpected migrated cache: %q", string(cache))
	}
}

func TestMigrateLegacyTreeMovesWhenTargetMissing(t *testing.T) {
	base := t.TempDir()
	newDir := filepath.Join(base, "ResultV", "webview")
	legacyDir := filepath.Join(base, "ResultProxy", "webview")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "data.bin"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}

	if err := migrateLegacyTree(newDir, legacyDir); err != nil {
		t.Fatalf("migrateLegacyTree: %v", err)
	}

	if _, err := os.Stat(filepath.Join(newDir, "data.bin")); err != nil {
		t.Fatalf("migrated tree missing file: %v", err)
	}
}

func TestMigrateLegacyFilesErrorWhenNewPathIsFile(t *testing.T) {
	base := t.TempDir()
	newDir := filepath.Join(base, "ResultV")
	legacyDir := filepath.Join(base, "ResultProxy")
	if err := os.WriteFile(newDir, []byte("x"), 0o600); err != nil {
		t.Fatalf("write new path file: %v", err)
	}
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "proxy_config.json"), []byte("legacy"), 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	if err := migrateLegacyFiles(newDir, legacyDir, []string{"proxy_config.json"}); err == nil {
		t.Fatal("expected error when new path is a file")
	}
}
