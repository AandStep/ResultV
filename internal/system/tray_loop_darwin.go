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

package system

import (
	"embed"

	"github.com/energye/systray"
)

//go:embed trayicons/menu_*.png
var trayMenuArt embed.FS

func startTray(onReady, onExit func()) {
	start, _ := systray.RunWithExternalLoop(onReady, onExit)
	start()
}

func (t *Tray) configureTrayMenu() {
	for _, entry := range []struct {
		item *systray.MenuItem
		icon string
	}{
		{t.miConnect, "power"},
		{t.miDisconnect, "power"},
		{t.miServers, "list"},
		{t.miShowWindow, "home"},
		{t.miQuit, "close"},
	} {
		if icon, err := trayMenuArt.ReadFile("trayicons/menu_" + entry.icon + ".png"); err == nil {
			entry.item.SetTemplateIcon(icon, nil)
		}
	}
	systray.CreateMenu()
}

// AppKit draws the checkmark in the menu's state column in both appearances.
func applyTrayServerMark(item *systray.MenuItem, mark trayMark) trayMark {
	checked := mark != trayMarkNone
	if checked != item.Checked() {
		if checked {
			item.Check()
		} else {
			item.Uncheck()
		}
	}
	return trayMarkNone
}
