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
	"strings"
	"testing"
)

type autostartFake struct {
	calls       []string
	taskPresent bool
	admin       bool
}

func stubAutostart(t *testing.T, windows, admin, taskPresent bool) *autostartFake {
	t.Helper()
	f := &autostartFake{admin: admin, taskPresent: taskPresent}
	prev := autostartOS
	autostartOS = autostartOps{
		windows:     windows,
		isAdmin:     func() bool { return f.admin },
		enableRun:   func(string) error { f.calls = append(f.calls, "run+"); return nil },
		disableRun:  func() error { f.calls = append(f.calls, "run-"); return nil },
		enableTask:  func(string) error { f.calls = append(f.calls, "task+"); return nil },
		disableTask: func() error { f.calls = append(f.calls, "task-"); return nil },
		taskPresent: func() bool { return f.taskPresent },
		elevatedTask: func(_ string, enable bool) error {
			if enable {
				f.calls = append(f.calls, "uac:task+")
			} else {
				f.calls = append(f.calls, "uac:task-")
			}
			return nil
		},
	}
	t.Cleanup(func() { autostartOS = prev })
	return f
}

func TestSyncAutostart(t *testing.T) {
	cases := []struct {
		name                        string
		windows, admin, taskPresent bool
		autostart, alwaysAdmin      bool
		want                        string
	}{
		{"обе вкл, с правами", true, true, false, true, true, "task+ run-"},
		{"обе вкл, без прав", true, false, false, true, true, "uac:task+ run-"},
		{"только автозапуск, задачи нет", true, true, false, true, false, "run+"},
		{"только автозапуск, задача есть, без прав", true, false, true, true, false, "uac:task- run+"},
		{"только автозапуск, задача есть, с правами", true, true, true, true, false, "task- run+"},
		{"всё выкл, задача есть", true, true, true, false, true, "task- run-"},
		{"всё выкл", true, false, false, false, false, "run-"},
		{"не Windows: админ-флаг игнорируется", false, false, false, true, true, "run+"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := stubAutostart(t, tc.windows, tc.admin, tc.taskPresent)
			if err := syncAutostart(`C:\x\ResultV.exe`, tc.autostart, tc.alwaysAdmin); err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(f.calls, " "); got != tc.want {
				t.Fatalf("вызовы %q, ждали %q", got, tc.want)
			}
		})
	}
}
