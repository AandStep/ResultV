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
	"testing"
)

func TestGetNetworkTraffic(t *testing.T) {
	stats := GetNetworkTraffic()
	
	
	if stats.Received < 0 || stats.Sent < 0 {
		t.Errorf("unexpected negative traffic: %+v", stats)
	}
	t.Logf("Network stats: received=%d bytes, sent=%d bytes", stats.Received, stats.Sent)
}

func TestIsAdmin(t *testing.T) {
	
	result := IsAdmin()
	t.Logf("IsAdmin: %v", result)
}

func TestIsAutostartEnabled(t *testing.T) {
	
	result := IsAutostartEnabled()
	t.Logf("IsAutostartEnabled: %v", result)
}

func TestElevationRelaunchArgsRoundTrip(t *testing.T) {
	id := "sub 1/ё'\"&;"
	args := append([]string{`C:\Program Files\ResultV\resultv.exe`}, ElevationRelaunchArgs(id, "tunnel")...)
	for _, a := range args[1:] {
		for _, r := range a {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_=", r)) {
				t.Fatalf("argument %q has %q, which a quoting layer would have to escape", a, r)
			}
		}
	}
	gotID, gotMode := ArgsResumeAfterElevation(args)
	if gotID != id || gotMode != "tunnel" {
		t.Fatalf("got (%q, %q), want (%q, tunnel)", gotID, gotMode, id)
	}
}

func TestElevationRelaunchArgsEmpty(t *testing.T) {
	if args := ElevationRelaunchArgs("", ""); len(args) != 0 {
		t.Fatalf("plain restart must carry no args, got %v", args)
	}
	if args := ElevationRelaunchArgs("", "bogus"); len(args) != 0 {
		t.Fatalf("unknown mode must be dropped, got %v", args)
	}
	id, mode := ArgsResumeAfterElevation([]string{"resultv", "--tray", "--mode=bogus", "--connect=%%%"})
	if id != "" || mode != "" {
		t.Fatalf("got (%q, %q), want nothing", id, mode)
	}
}
