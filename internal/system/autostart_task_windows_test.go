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
	"strings"
	"testing"
)

func TestAutostartTaskXML(t *testing.T) {
	x := autostartTaskXML(`C:\Program Files\R&D\ResultV.exe`, "S-1-5-21-1")
	for _, want := range []string{
		"<RunLevel>HighestAvailable</RunLevel>",
		"<LogonType>InteractiveToken</LogonType>",
		"<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>",
		"<DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>",
		"<StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>",
		"<MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>",
		"<UserId>S-1-5-21-1</UserId>",
		"<Arguments>--autostart</Arguments>",
		`<Command>C:\Program Files\R&amp;D\ResultV.exe</Command>`,
	} {
		if !strings.Contains(x, want) {
			t.Errorf("нет %q в XML задачи", want)
		}
	}
}
