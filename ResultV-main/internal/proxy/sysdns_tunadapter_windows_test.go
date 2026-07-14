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

package proxy

import "testing"

// The loopback adapter always carries 127.0.0.1, so the lookup must find it
// on any machine — that's the same mechanism OverrideTunnelAdapter uses to
// locate the sing-tun adapter by its interface address.
func TestFindInterfaceIndexByIP_Loopback(t *testing.T) {
	idx, err := findInterfaceIndexByIP("127.0.0.1")
	if err != nil {
		t.Fatalf("findInterfaceIndexByIP(127.0.0.1): %v", err)
	}
	if idx <= 0 {
		t.Fatalf("expected a positive interface index, got %d", idx)
	}
}

func TestFindInterfaceIndexByIP_NotFound(t *testing.T) {
	if _, err := findInterfaceIndexByIP("192.0.2.123"); err == nil {
		t.Fatal("expected error for an address no adapter carries")
	}
}

func TestFindInterfaceIndexByIP_BadInput(t *testing.T) {
	if _, err := findInterfaceIndexByIP("not-an-ip"); err == nil {
		t.Fatal("expected error for malformed ip")
	}
}
