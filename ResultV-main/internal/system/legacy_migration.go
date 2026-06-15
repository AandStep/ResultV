package system

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

var migrationFiles = []string{
	"proxy_config.json",
	".machine-fallback-id",
	"blocked_cache.json",
	"sing-box-cache.db",
}

func MigrateLegacyUserData() error {
	newDataPath, legacyDataPath := userDataPaths()
	if err := migrateLegacyFiles(newDataPath, legacyDataPath, migrationFiles); err != nil {
		return err
	}
	newWebviewPath, legacyWebviewPath := webviewDataPaths()
	if err := migrateLegacyTree(newWebviewPath, legacyWebviewPath); err != nil {
		return err
	}
	return nil
}

func migrateLegacyFiles(newDir, legacyDir string, files []string) error {
	if !pathExists(legacyDir) {
		return nil
	}
	if err := os.MkdirAll(newDir, 0o700); err != nil {
		return fmt.Errorf("create new dir: %w", err)
	}
	for _, name := range files {
		legacyPath := filepath.Join(legacyDir, name)
		if !pathExists(legacyPath) {
			continue
		}
		newPath := filepath.Join(newDir, name)
		if pathExists(newPath) {
			continue
		}
		if err := movePath(legacyPath, newPath); err != nil {
			return fmt.Errorf("migrate %s: %w", name, err)
		}
	}
	for _, name := range files {
		_ = os.Remove(filepath.Join(legacyDir, name))
	}
	_ = os.Remove(legacyDir)
	return nil
}

func migrateLegacyTree(newDir, legacyDir string) error {
	if !pathExists(legacyDir) {
		return nil
	}
	if !pathExists(newDir) {
		if err := os.MkdirAll(filepath.Dir(newDir), 0o700); err != nil {
			return fmt.Errorf("create new parent dir: %w", err)
		}
		if err := movePath(legacyDir, newDir); err == nil {
			return nil
		}
	}
	return copyDirMissing(newDir, legacyDir)
}

func copyDirMissing(dst, src string) error {
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if pathExists(target) {
			return nil
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func movePath(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if err := copyDirMissing(dst, src); err != nil {
			return err
		}
		return nil
	}
	return copyFile(src, dst)
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
