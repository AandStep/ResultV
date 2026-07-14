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

const testPID = 4242

func TestPickServerIP_SingleEstablishedMatch(t *testing.T) {
	rows := []tcpConnRow{
		{remoteIP: "203.0.113.7", remotePort: 8443, pid: testPID, established: true},
	}
	if got := pickServerIP(rows, testPID, 8443); got != "203.0.113.7" {
		t.Fatalf("expected 203.0.113.7, got %q", got)
	}
}

func TestPickServerIP_PrefersDominantIPOverPortCollision(t *testing.T) {
	// Server on :443 with several XHTTP streams, plus one unrelated connection
	// of ours to a different host on :443. The dominant IP wins.
	rows := []tcpConnRow{
		{remoteIP: "203.0.113.7", remotePort: 443, pid: testPID, established: true},
		{remoteIP: "203.0.113.7", remotePort: 443, pid: testPID, established: true},
		{remoteIP: "203.0.113.7", remotePort: 443, pid: testPID, established: true},
		{remoteIP: "198.51.100.9", remotePort: 443, pid: testPID, established: true},
	}
	if got := pickServerIP(rows, testPID, 443); got != "203.0.113.7" {
		t.Fatalf("expected dominant 203.0.113.7, got %q", got)
	}
}

func TestPickServerIP_IgnoresOtherPIDsPortsAndStates(t *testing.T) {
	rows := []tcpConnRow{
		{remoteIP: "10.0.0.1", remotePort: 8443, pid: testPID + 1, established: true}, // other process
		{remoteIP: "10.0.0.2", remotePort: 9999, pid: testPID, established: true},     // other port
		{remoteIP: "10.0.0.3", remotePort: 8443, pid: testPID, established: false},    // not established
		{remoteIP: "203.0.113.7", remotePort: 8443, pid: testPID, established: true},  // the one
	}
	if got := pickServerIP(rows, testPID, 8443); got != "203.0.113.7" {
		t.Fatalf("expected 203.0.113.7, got %q", got)
	}
}

func TestPickServerIP_AmbiguousTieReturnsEmpty(t *testing.T) {
	rows := []tcpConnRow{
		{remoteIP: "203.0.113.7", remotePort: 443, pid: testPID, established: true},
		{remoteIP: "198.51.100.9", remotePort: 443, pid: testPID, established: true},
	}
	if got := pickServerIP(rows, testPID, 443); got != "" {
		t.Fatalf("expected empty on ambiguous tie, got %q", got)
	}
}

func TestPickServerIP_NoMatchReturnsEmpty(t *testing.T) {
	rows := []tcpConnRow{
		{remoteIP: "10.0.0.2", remotePort: 9999, pid: testPID, established: true},
	}
	if got := pickServerIP(rows, testPID, 8443); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}
