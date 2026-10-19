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
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Старый экземпляр при выходе снимает маршруты со всех наших адаптеров и
// восстанавливает DNS из общего снимка. Если новый, запущенный с правами, не
// дождётся его, эта уборка сносит маршруты уже нового туннеля.
func TestWaitForElevationParent_WaitsForParentExit(t *testing.T) {
	cmd := exec.Command("ping", "-n", "3", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan time.Time, 1)
	go func() { _ = cmd.Wait(); exited <- time.Now() }()

	WaitForElevationParent([]string{"ResultV.exe", ElevatedFromFlag + strconv.Itoa(cmd.Process.Pid)})
	returned := time.Now()

	select {
	case at := <-exited:
		if returned.Before(at.Add(-200 * time.Millisecond)) {
			t.Fatalf("вернулись на %v раньше выхода родителя", at.Sub(returned))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("вернулись, а родитель так и не завершился")
	}
}

func TestWaitForElevationParent_NoFlagReturnsAtOnce(t *testing.T) {
	start := time.Now()
	WaitForElevationParent([]string{"ResultV.exe"})
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Fatalf("без флага ждали %v", d)
	}
}

func TestRestartAsAdminArgs_CarryParentPID(t *testing.T) {
	got := restartAsAdminPSCommand(`C:\Program Files\ResultV\ResultV.exe`, []string{"--connect=YWJj"}, 4242)
	if !strings.Contains(got, ElevatedFromFlag+"4242") {
		t.Fatalf("нет %s4242 в команде: %s", ElevatedFromFlag, got)
	}
	if !strings.Contains(got, "--connect=YWJj") {
		t.Fatalf("потерян аргумент возобновления: %s", got)
	}
}
