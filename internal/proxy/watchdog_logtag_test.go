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

package proxy

import (
	"strings"
	"testing"

	"resultproxy-wails/internal/logger"
)

// TestWatchdogLogTag pins the prefix split: the health watchdog runs on every
// session, but only an armed kill switch may claim its name in the log.
func TestWatchdogLogTag(t *testing.T) {
	if got := watchdogLogTag(true); got != "[KILL SWITCH]" {
		t.Fatalf("watchdogLogTag(true) = %q, want [KILL SWITCH]", got)
	}
	if got := watchdogLogTag(false); got != "[СЕТЬ]" {
		t.Fatalf("watchdogLogTag(false) = %q, want [СЕТЬ]", got)
	}
}

// TestLogWatchdogTickSeverity: with the kill switch off nothing will be
// blocked, so per-tick probe failures are informational — they must neither
// mention the kill switch nor raise a warning in the user-visible log.
func TestLogWatchdogTickSeverity(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ks      bool
		wantTag string
		wantTyp string
	}{
		{"armed", true, "[KILL SWITCH]", logger.TypeWarning},
		{"disarmed", false, "[СЕТЬ]", logger.TypeInfo},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &Manager{log: logger.New()}
			m.logWatchdogTick(tc.ks, "Проба не прошла (%d/%d): %s", 1, 2, "probe_error")

			items := m.log.GetLogs(1, 10).Items
			if len(items) != 1 {
				t.Fatalf("logged %d entries, want 1", len(items))
			}
			got := items[0]
			if !strings.HasPrefix(got.Msg, tc.wantTag+" ") {
				t.Fatalf("msg = %q, want prefix %q", got.Msg, tc.wantTag)
			}
			if !strings.Contains(got.Msg, "Проба не прошла (1/2): probe_error") {
				t.Fatalf("msg = %q, want formatted body", got.Msg)
			}
			if got.Type != tc.wantTyp {
				t.Fatalf("type = %q, want %q", got.Type, tc.wantTyp)
			}
		})
	}
}
