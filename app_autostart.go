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

package main

import (
	"fmt"
	"os"
	"runtime"

	"resultproxy-wails/internal/system"
)

type autostartOps struct {
	windows      bool
	isAdmin      func() bool
	enableRun    func(exe string) error
	disableRun   func() error
	enableTask   func(exe string) error
	disableTask  func() error
	taskPresent  func() bool
	elevatedTask func(exe string, enable bool) error
}

var autostartOS = autostartOps{
	windows:      runtime.GOOS == "windows",
	isAdmin:      system.IsAdmin,
	enableRun:    func(exe string) error { return system.EnableAutostart(exe) },
	disableRun:   system.DisableAutostart,
	enableTask:   system.EnableElevatedAutostart,
	disableTask:  system.DisableElevatedAutostart,
	taskPresent:  system.ElevatedAutostartPresent,
	elevatedTask: runElevatedAutostartTask,
}

func runElevatedAutostartTask(exe string, enable bool) error {
	cmds, cleanup, err := system.ElevatedAutostartCommands(exe, enable)
	if err != nil {
		return err
	}
	defer cleanup()
	return system.ExecuteElevatedCommands(cmds)
}

// syncAutostart brings the OS autostart entries in line with the two settings:
// with "always as admin" on Windows the scheduled task replaces the Run value,
// because a Run entry cannot start elevated without a UAC prompt at logon.
func syncAutostart(exe string, autostart, alwaysAdmin bool) error {
	ops := autostartOS
	setTask := ops.enableTask
	dropTask := ops.disableTask
	if !ops.isAdmin() {
		setTask = func(exe string) error { return ops.elevatedTask(exe, true) }
		dropTask = func() error { return ops.elevatedTask(exe, false) }
	}
	if ops.windows && autostart && alwaysAdmin {
		if err := setTask(exe); err != nil {
			return err
		}
		return ops.disableRun()
	}
	if ops.windows && ops.taskPresent() {
		if err := dropTask(); err != nil {
			return err
		}
	}
	if autostart {
		return ops.enableRun(exe)
	}
	return ops.disableRun()
}

func (a *App) currentSettingsAutostart() (autostart, alwaysAdmin bool) {
	if a.config == nil {
		return false, false
	}
	s := a.config.GetConfig().Settings
	return s.Autostart, s.AlwaysRunAsAdmin
}

func (a *App) SetAlwaysRunAsAdmin(enable bool) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}
	autostart, _ := a.currentSettingsAutostart()
	if err := syncAutostart(exe, autostart, enable); err != nil {
		a.log.Error(fmt.Sprintf("[СИСТЕМА] Не удалось перестроить автозапуск под запуск от администратора: %v", err))
		return err
	}
	if enable {
		a.log.Success("[СИСТЕМА] Запуск от имени администратора включён")
	} else {
		a.log.Info("[СИСТЕМА] Запуск от имени администратора выключен")
	}
	return nil
}

// EnableAlwaysAdminAndRestart is the second button of the "admin rights
// needed" dialog: the setting is saved before the restart, and the elevated
// instance creates the logon task on startup.
func (a *App) EnableAlwaysAdminAndRestart(connectProxyID, mode string) error {
	if a.config != nil {
		cfg := a.config.GetConfig()
		cfg.Settings.AlwaysRunAsAdmin = true
		if err := a.config.SaveConfig(cfg); err != nil {
			return fmt.Errorf("сохранение настройки: %w", err)
		}
	}
	return a.RestartAsAdmin(connectProxyID, mode)
}

func (a *App) syncAutostartOnStartup() {
	if !autostartOS.windows || !autostartOS.isAdmin() {
		return
	}
	autostart, alwaysAdmin := a.currentSettingsAutostart()
	if !autostart || !alwaysAdmin {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	if err := syncAutostart(exe, autostart, alwaysAdmin); err != nil {
		a.log.Warning(fmt.Sprintf("[СИСТЕМА] Не удалось создать задачу автозапуска: %v", err))
	}
}
