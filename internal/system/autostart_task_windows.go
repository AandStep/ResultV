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
	"encoding/binary"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

const elevatedTaskName = "ResultVAutostartAdmin"

func autostartTaskXML(exePath, userSID string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>%[2]s</UserId>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>%[2]s</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>HighestAvailable</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>false</StartWhenAvailable>
    <Enabled>true</Enabled>
    <Priority>7</Priority>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>%[1]s</Command>
      <Arguments>%[3]s</Arguments>
    </Exec>
  </Actions>
</Task>
`, html.EscapeString(exePath), html.EscapeString(userSID), autostartTrayToken)
}

func currentUserSID() (string, error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return u.User.Sid.String(), nil
}

// writeTaskXML stores the task definition as UTF-16LE with BOM, the encoding
// schtasks /xml expects for the declared header.
func writeTaskXML(exePath string) (string, error) {
	sid, err := currentUserSID()
	if err != nil {
		return "", fmt.Errorf("sid пользователя: %w", err)
	}
	dir := filepath.Join(UserDataDir(), "elevated")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, "autostart-*.xml")
	if err != nil {
		return "", err
	}
	defer f.Close()
	u := append([]uint16{0xFEFF}, utf16.Encode([]rune(autostartTaskXML(exePath, sid)))...)
	if err := binary.Write(f, binary.LittleEndian, u); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

func removeRunValue() {
	_ = DisableAutostart()
}

func EnableElevatedAutostart(exePath string) error {
	path, err := writeTaskXML(exePath)
	if err != nil {
		return err
	}
	defer os.Remove(path)
	if out, err := command("schtasks", "/create", "/tn", elevatedTaskName, "/xml", path, "/f").CombinedOutput(); err != nil {
		return fmt.Errorf("создание задачи автозапуска: %w: %s", err, out)
	}
	removeRunValue()
	return nil
}

func DisableElevatedAutostart() error {
	if !ElevatedAutostartPresent() {
		return nil
	}
	if out, err := command("schtasks", "/delete", "/tn", elevatedTaskName, "/f").CombinedOutput(); err != nil {
		return fmt.Errorf("удаление задачи автозапуска: %w: %s", err, out)
	}
	return nil
}

func ElevatedAutostartPresent() bool {
	return command("schtasks", "/query", "/tn", elevatedTaskName).Run() == nil
}

// ElevatedAutostartCommands is the same create/delete as a command list for
// ExecuteElevatedCommands, for a process without admin rights. cleanup removes
// the temporary XML once the elevated run is over.
func ElevatedAutostartCommands(exePath string, enable bool) ([]string, func(), error) {
	if !enable {
		return []string{fmt.Sprintf(`schtasks /delete /tn %s /f`, elevatedTaskName)}, func() {}, nil
	}
	path, err := writeTaskXML(exePath)
	if err != nil {
		return nil, func() {}, err
	}
	return []string{fmt.Sprintf(`schtasks /create /tn %s /xml "%s" /f`, elevatedTaskName, path)},
		func() { _ = os.Remove(path) }, nil
}
