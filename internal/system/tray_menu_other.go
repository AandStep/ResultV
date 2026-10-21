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

//go:build !windows

package system

import (
	"runtime"

	"github.com/energye/systray"
)

// macOS scales menu images to 16pt, so it gets a 2x PNG for Retina.
func buildTrayMenuIcon(name string, mark trayMark) ([]byte, error) {
	size := 16
	if runtime.GOOS == "darwin" {
		size = 32
	}
	img, err := trayIconImage(name, size)
	if err != nil {
		return nil, err
	}
	drawTrayMark(img, mark)
	return encodeTrayPNG(img)
}

func trayMenuText(s string) string {
	return s
}

// Headers get a blank image so they line up with the rows that have one.
func trayHeaderIcon() []byte {
	return trayMenuIcon(trayIconBlank, trayMarkNone)
}

// energye's darwin SetIcon indexes the first byte of the slice.
func setTrayItemIcon(item *systray.MenuItem, icon []byte) {
	if len(icon) > 0 {
		item.SetIcon(icon)
	}
}
