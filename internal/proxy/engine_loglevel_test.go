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

import "testing"

// The default has to stay "error": at "info" the core writes a line per
// connection and per DNS answer into the same window the user sends us.
func TestSingBoxLogLevelDefaultsToError(t *testing.T) {
	t.Setenv("RESULTV_SINGBOX_LOG_LEVEL", "")
	if got := singBoxLogLevel(); got != "error" {
		t.Fatalf("singBoxLogLevel() = %q, want error", got)
	}
}

// Only levels the core accepts get through; anything else falls back rather
// than reaching the strict decoder, where an unknown value is not a bad log
// setting but a dead engine for every node.
func TestSingBoxLogLevelAcceptsOnlyKnownLevels(t *testing.T) {
	for env, want := range map[string]string{
		"trace":   "trace",
		"DEBUG":   "debug",
		" info ":  "info",
		"warning": "warn",
		"loud":    "error",
		"42":      "error",
	} {
		t.Setenv("RESULTV_SINGBOX_LOG_LEVEL", env)
		if got := singBoxLogLevel(); got != want {
			t.Fatalf("singBoxLogLevel() with %q = %q, want %q", env, got, want)
		}
	}
}
