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
	"errors"
	"net"
	"strings"
	"testing"
)

// TestProbeFailureReasonKeepsUnknownDetail: the catch-all bucket used to throw
// the error text away, so a watchdog log full of "probe_error" said nothing
// about why a healthy server kept failing its probe. Recognised classes stay
// as bare codes — the veto logic and the log both key off them.
func TestProbeFailureReasonKeepsUnknownDetail(t *testing.T) {
	got := probeFailureReason(errors.New("EOF"))
	if !strings.HasPrefix(got, "probe_error: ") {
		t.Fatalf("reason = %q, want probe_error: prefix", got)
	}
	if !strings.Contains(got, "EOF") {
		t.Fatalf("reason = %q, want the original error text", got)
	}
}

func TestProbeFailureReasonKnownClassesUnchanged(t *testing.T) {
	if got := probeFailureReason(errors.New("connection refused")); got != "connection_refused" {
		t.Fatalf("reason = %q, want connection_refused", got)
	}
	if got := probeFailureReason(errors.New("i/o timeout")); got != "timeout" {
		t.Fatalf("reason = %q, want timeout", got)
	}
}

// A local-resolver failure must keep its local_dns: prefix — isLocalDNSProbeFailure
// is what stops the watchdog counting it as a server-dead strike.
func TestProbeFailureReasonLocalDNSStillDetected(t *testing.T) {
	err := &net.DNSError{Err: "no such host", Name: "example.org", IsNotFound: true}
	got := probeFailureReason(err)
	if !isLocalDNSProbeFailure(got) {
		t.Fatalf("reason = %q, want local_dns: prefix", got)
	}
}
