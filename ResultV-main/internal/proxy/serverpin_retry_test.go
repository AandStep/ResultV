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

func TestShouldRetryWithoutPin(t *testing.T) {
	cases := []struct {
		name      string
		usedCache bool
		errorCode string
		want      bool
	}{
		{"stale pin, probe failed", true, "post_start_probe_failed", true},
		{"probe failed but no cached pin", false, "post_start_probe_failed", false},
		{"cached pin but cancelled", true, "cancelled", false},
		{"cached pin but engine start error", true, "engine_start", false},
		{"cached pin, success (empty code)", true, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := shouldRetryWithoutPin(c.usedCache, c.errorCode); got != c.want {
				t.Fatalf("shouldRetryWithoutPin(%v, %q) = %v, want %v", c.usedCache, c.errorCode, got, c.want)
			}
		})
	}
}
