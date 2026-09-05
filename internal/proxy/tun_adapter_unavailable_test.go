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
	"context"
	"errors"
	"strings"
	"testing"

	"resultproxy-wails/internal/logger"
)

// The two texts Windows produces when it creates the Wintun device node but
// never binds a driver to it, so the adapter never becomes usable.
//
//   - tunFreshCreateErr: no leftover node. wintun.CreateAdapter builds one, waits
//     for the device object, gives up — sing-tun returns the bare errno
//     (tun_windows.go:43 "return nil, err"), so nothing labels it.
//   - tunPhantomErr: the previous attempt's half-built node is still in the
//     device tree as CM_PROB_PHANTOM. CreateAdapter now reports ErrExist and
//     OpenAdapter cannot open a non-present device, so sing-tun returns
//     E.Errors(...) — whose Error() is "(" + join(" | ") + ")".
const (
	tunFreshCreateErr = "starting sing-box: start inbound/tun[tun-in]: configure tun interface: The system cannot find the file specified."
	tunPhantomErr     = "starting sing-box: start inbound/tun[tun-in]: configure tun interface: (create adapter: The system cannot find the file specified. | open existing adapter: Element not found.)"
)

func TestIsTunAdapterUnavailableError(t *testing.T) {
	for _, msg := range []string{tunFreshCreateErr, tunPhantomErr} {
		if !isTunAdapterUnavailableError(errors.New(msg)) {
			t.Fatalf("must recognise the Wintun-never-started failure: %q", msg)
		}
	}
	// Everything else that also carries "configure tun interface" belongs to a
	// different remedy and must not be swallowed by this one: the IPv6 case is
	// fixed by rebuilding the config, and a genuinely contended adapter is fixed
	// by removing the device.
	for _, msg := range []string{
		"configure tun interface: set ipv6 address: Element not found.",
		"configure tun interface: set ipv4 address: Element not found.",
		"configure tun interface: Access is denied.",
		"configure tun interface: set ipv4 options: Element not found.",
	} {
		if isTunAdapterUnavailableError(errors.New(msg)) {
			t.Fatalf("must not claim the adapter-unavailable class for %q", msg)
		}
	}
	if isTunAdapterUnavailableError(nil) {
		t.Fatal("nil is not an adapter-unavailable error")
	}
}

func TestClassifyEngineStartError_TunAdapterUnavailable(t *testing.T) {
	for _, msg := range []string{tunFreshCreateErr, tunPhantomErr} {
		tunnelFailed, reason, code := ClassifyEngineStartError(ProxyModeTunnel, errors.New(msg))
		if !tunnelFailed {
			t.Fatalf("tunnelFailed must be true for %q", msg)
		}
		if code != ConnectErrorTunAdapter {
			t.Fatalf("expected %q, got %q for %q", ConnectErrorTunAdapter, code, msg)
		}
		// The whole point of the fix: the user must not be handed Windows' own
		// "The system cannot find the file specified." as an explanation.
		if strings.Contains(reason, "cannot find the file specified") || strings.Contains(reason, "Element not found") {
			t.Fatalf("reason must explain the failure, not echo the errno: %q", reason)
		}
		if !strings.Contains(reason, "Wintun") {
			t.Fatalf("reason must name the adapter that failed, got %q", reason)
		}
	}
	// Proxy mode never builds a TUN, so the class cannot apply there.
	if _, _, code := ClassifyEngineStartError(ProxyModeProxy, errors.New(tunFreshCreateErr)); code == ConnectErrorTunAdapter {
		t.Fatal("adapter-unavailable is tunnel-only")
	}
}

// The adapter never coming up is not a rights problem — tunnel mode is
// admin-gated up front. Reporting tun_privileges would send a non-elevated user
// through a restart-as-admin loop that cannot help.
func TestStartEngine_AdapterUnavailableIsNeverPrivileges(t *testing.T) {
	for _, admin := range []bool{true, false} {
		fastTunRetry(t)
		fakeAdmin(t, admin)
		stubRemoveStaleTunAdapter(t)

		tunErr := errors.New(tunFreshCreateErr)
		eng := &seqEngine{errs: []error{tunErr, tunErr, tunErr}}
		m := NewManager(logger.New())
		m.engine = eng

		_, _, _, code := m.startEngine(context.Background(), EngineConfig{Mode: ProxyModeTunnel})
		if code != ConnectErrorTunAdapter {
			t.Fatalf("admin=%v: expected %q, got %q", admin, ConnectErrorTunAdapter, code)
		}
	}
}

// extractErrorReason cut on the LAST ": ", which on a sing multiError left the
// dangling ")" of the group and threw away the first cause entirely — the half
// that says what CreateAdapter actually returned.
func TestExtractErrorReason_KeepsBothCausesOfAMultiError(t *testing.T) {
	got := extractErrorReason(tunPhantomErr)
	if strings.HasSuffix(got, ")") {
		t.Fatalf("must not leave the multiError dangling paren: %q", got)
	}
	if !strings.Contains(got, "create adapter") || !strings.Contains(got, "open existing adapter") {
		t.Fatalf("both causes must survive, got %q", got)
	}
	// The ordinary single-cause path is unchanged.
	if got := extractErrorReason("configure tun interface: Access is denied."); got != "Access is denied." {
		t.Fatalf("single-cause extraction regressed: %q", got)
	}
	// A message that merely ends in ")" is not a multiError — leave it alone.
	if got := extractErrorReason("dial failed: no route to host (attempt 2)"); got != "no route to host (attempt 2)" {
		t.Fatalf("non-multiError tail must be preserved, got %q", got)
	}
}

// pnputil returns as soon as the removal is QUEUED; the device node lives on for
// a while afterwards. Retrying before it is gone is what produced the bare
// ERROR_FILE_NOT_FOUND on the second attempt. The retry must wait for the node
// to actually leave the device tree.
func TestStartEngine_WaitsForDevNodeToLeaveBeforeRetrying(t *testing.T) {
	fastTunRetry(t)
	fakeAdmin(t, true)
	stubRemoveStaleTunAdapter(t)

	// Node reported present for the first two polls, then gone.
	polls := 0
	prev := tunDevNodeGoneFn
	tunDevNodeGoneFn = func() bool { polls++; return polls > 2 }
	t.Cleanup(func() { tunDevNodeGoneFn = prev })

	eng := &seqEngine{errs: []error{errors.New("start inbound/tun[tun-in]: configure tun interface: Access is denied.")}}
	m := NewManager(logger.New())
	m.engine = eng

	if err, _, _, _ := m.startEngine(context.Background(), EngineConfig{Mode: ProxyModeTunnel}); err != nil {
		t.Fatalf("expected success after the retry, got %v", err)
	}
	if polls < 3 {
		t.Fatalf("the retry must poll until the device node is gone, polled %d time(s)", polls)
	}
	if eng.calls != 2 {
		t.Fatalf("expected fail + retry, got %d starts", eng.calls)
	}
}

// A node that never disappears must not wedge the connect forever — the wait is
// bounded and the retry proceeds anyway.
func TestStartEngine_DevNodeWaitIsBounded(t *testing.T) {
	fastTunRetry(t)
	fakeAdmin(t, true)
	stubRemoveStaleTunAdapter(t)

	prev := tunDevNodeGoneFn
	tunDevNodeGoneFn = func() bool { return false }
	t.Cleanup(func() { tunDevNodeGoneFn = prev })

	eng := &seqEngine{errs: []error{errors.New("start inbound/tun[tun-in]: configure tun interface: Access is denied.")}}
	m := NewManager(logger.New())
	m.engine = eng

	if err, _, _, _ := m.startEngine(context.Background(), EngineConfig{Mode: ProxyModeTunnel}); err != nil {
		t.Fatalf("the bounded wait must still let the retry run, got %v", err)
	}
	if eng.calls != 2 {
		t.Fatalf("expected fail + retry, got %d starts", eng.calls)
	}
}
