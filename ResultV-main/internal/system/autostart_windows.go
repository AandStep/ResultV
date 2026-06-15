// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

//go:build windows

package system

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	taskName             = "ResultVAutostart"
	legacyTaskName       = "ResultProxyAutostart"
	runRegistryPath      = `Software\Microsoft\Windows\CurrentVersion\Run`
	runRegistryValue     = "ResultV"
	legacyRunRegistryKey = "ResultProxy"
	autostartTrayToken   = "--autostart"
)

func buildAutostartRunCommand(exePath string, extraArgs ...string) string {
	p := strings.ReplaceAll(exePath, "/", "\\")
	var b strings.Builder
	b.WriteString(`"`)
	b.WriteString(p)
	b.WriteString(`"`)
	b.WriteString(" ")
	b.WriteString(autostartTrayToken)
	for _, a := range extraArgs {
		b.WriteString(" ")
		b.WriteString(a)
	}
	return b.String()
}

func deleteLegacyScheduledTask() {
	for _, name := range []string{taskName, legacyTaskName} {
		cmd := command("schtasks", "/delete", "/tn", name, "/f")
		_ = cmd.Run()
	}
}

func runAutostartValuePresent() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, runRegistryPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	_, _, err = key.GetStringValue(runRegistryValue)
	if err == nil {
		return true
	}
	_, _, err = key.GetStringValue(legacyRunRegistryKey)
	return err == nil
}

func legacyScheduledTaskPresent() bool {
	for _, name := range []string{taskName, legacyTaskName} {
		cmd := command("schtasks", "/query", "/tn", name)
		if cmd.Run() == nil {
			return true
		}
	}
	return false
}

func EnableAutostart(exePath string, args ...string) error {
	deleteLegacyScheduledTask()

	key, err := registry.OpenKey(registry.CURRENT_USER, runRegistryPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("открытие ключа Run: %w", err)
	}
	defer key.Close()

	cmdLine := buildAutostartRunCommand(exePath, args...)
	if err := key.SetStringValue(runRegistryValue, cmdLine); err != nil {
		return fmt.Errorf("запись автозапуска: %w", err)
	}
	_ = key.DeleteValue(legacyRunRegistryKey)
	return nil
}

func DisableAutostart() error {
	deleteLegacyScheduledTask()

	key, err := registry.OpenKey(registry.CURRENT_USER, runRegistryPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("открытие ключа Run: %w", err)
	}
	defer key.Close()

	for _, valueName := range []string{runRegistryValue, legacyRunRegistryKey} {
		if err := key.DeleteValue(valueName); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return fmt.Errorf("удаление автозапуска: %w", err)
		}
	}
	return nil
}

func IsAutostartEnabled() bool {
	if runAutostartValuePresent() {
		return true
	}
	return legacyScheduledTaskPresent()
}

func SetRunAsAdminFlag(exePath string, enable bool) error {
	regPath := `Software\Microsoft\Windows NT\CurrentVersion\AppCompatFlags\Layers`

	key, err := registry.OpenKey(registry.CURRENT_USER, regPath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {

		key, _, err = registry.CreateKey(registry.CURRENT_USER, regPath, registry.SET_VALUE)
		if err != nil {
			return fmt.Errorf("opening AppCompatFlags registry key: %w", err)
		}
	}
	defer key.Close()

	if enable {
		return key.SetStringValue(exePath, "~ RUNASADMIN")
	}

	_ = key.DeleteValue(exePath)
	return nil
}
