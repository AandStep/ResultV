package config

import (
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Recovery tells how Init dealt with a stored config it could not read.
type Recovery string

const (
	RecoveryNone     Recovery = ""
	RecoveryRestored Recovery = "restored"
	RecoveryReset    Recovery = "reset"
)

const (
	configFileName       = "proxy_config.json"
	backupSuffix         = ".bak"
	unreadableSuffix     = ".unreadable-"
	unreadableCopiesKept = 5
)

func (m *Manager) Recovery() Recovery {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.recovery
}

// recoverUnreadableLocked keeps a copy of the unreadable config and continues
// from the backup taken at the last successful start, or from defaults without
// one. The config file is replaced in one step, so a crash here loses nothing.
func (m *Manager) recoverUnreadableLocked(unreadable []byte) error {
	aside := m.configPath + unreadableSuffix + time.Now().Format("20060102-150405.000")
	if err := writeFileAtomic(aside, unreadable, 0o600); err != nil {
		return err
	}
	pruneUnreadableCopies(m.configPath)

	if backup, err := os.ReadFile(m.configPath + backupSuffix); err == nil {
		var restored AppConfig
		if m.crypto.DecryptInto(string(backup), &restored) == nil {
			if err := writeFileAtomic(m.configPath, backup, 0o600); err != nil {
				return err
			}
			m.cache = ensureDefaults(restored)
			m.recovery = RecoveryRestored
			return nil
		}
	}
	_ = os.Remove(m.configPath)
	m.cache = DefaultConfig()
	m.recovery = RecoveryReset
	return nil
}

func pruneUnreadableCopies(configPath string) {
	copies, err := filepath.Glob(configPath + unreadableSuffix + "*")
	if err != nil || len(copies) <= unreadableCopiesKept {
		return
	}
	sort.Strings(copies)
	for _, old := range copies[:len(copies)-unreadableCopiesKept] {
		_ = os.Remove(old)
	}
}

// ReadStoredSettings reads the saved settings without creating, migrating or
// repairing anything on disk.
func ReadStoredSettings(crypto *CryptoService, userDataPath string) (AppSettings, bool) {
	paths := []string{filepath.Join(userDataPath, configFileName)}
	if legacy := legacyUserDataDir(userDataPath); legacy != "" {
		paths = append(paths, filepath.Join(legacy, configFileName))
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return AppSettings{}, false
		}
		var cfg AppConfig
		if err := crypto.DecryptInto(string(data), &cfg); err != nil {
			return AppSettings{}, false
		}
		return ensureDefaults(cfg).Settings, true
	}
	return AppSettings{}, false
}
