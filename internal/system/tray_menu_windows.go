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
	"strings"

	"github.com/energye/systray"
)

var procGetSystemMetrics = dllUser32.NewProc("GetSystemMetrics")

const (
	smCXIcon   = 11
	smCXSmIcon = 49
)

// energye loads the .ico at SM_CXICON and shrinks it to SM_CXSMICON with
// DrawIconEx, which point-samples. A pixel-doubled image survives that
// shrink unchanged, a properly drawn large one does not.
func buildTrayMenuIcon(name string, mark trayMark) ([]byte, error) {
	small, _, _ := procGetSystemMetrics.Call(smCXSmIcon)
	large, _, _ := procGetSystemMetrics.Call(smCXIcon)
	if small == 0 || large == 0 {
		small, large = 16, 32
	}
	img, err := trayIconImage(name, int(small))
	if err != nil {
		return nil, err
	}
	drawTrayMark(img, mark)
	return encodeICO(pixelUpscale(img, int(large)))
}

// Windows reads "&" in menu text as a mnemonic marker.
func trayMenuText(s string) string {
	return strings.ReplaceAll(s, "&", "&&")
}

// Windows greys a disabled item's bitmap into a solid square, so a header
// line carries no icon at all.
func trayHeaderIcon() []byte {
	return nil
}

func setTrayItemIcon(item *systray.MenuItem, icon []byte) {
	item.SetIcon(icon)
}
