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

// seqEngine returns errs[i] on the i-th Start call (nil = success); once the
// sequence is exhausted every further Start succeeds. It records the attempt
// count so a test can assert exactly how many starts the retry logic issued.
type seqEngine struct {
	errs  []error
	calls int
}

func (e *seqEngine) Start(_ context.Context, _ EngineConfig) error {
	i := e.calls
	e.calls++
	if i < len(e.errs) {
		return e.errs[i]
	}
	return nil
}
func (e *seqEngine) Stop() error                        { return nil }
func (e *seqEngine) IsRunning() bool                    { return false }
func (e *seqEngine) GetTrafficStats() (up, down int64)      { return 0, 0 }
func (e *seqEngine) GetProxyTrafficStats() (up, down int64) { return 0, 0 }
func (e *seqEngine) ApplyAppWhitelist([]string) error       { return nil }

// fastTunRetry removes the inter-attempt settle delay so retry tests don't sleep.
func fastTunRetry(t *testing.T) {
	t.Helper()
	prev := tunRetryDelay
	tunRetryDelay = 0
	t.Cleanup(func() { tunRetryDelay = prev })
}

func fakeAdmin(t *testing.T, admin bool) {
	t.Helper()
	prev := isAdminCheck
	isAdminCheck = func() bool { return admin }
	t.Cleanup(func() { isAdminCheck = prev })
}

// A stale Wintun adapter / leftover WFP filters from an unclean prior exit make
// the first TUN start fail with "configure tun interface / access denied". The
// failed start releases those handles, so a single automatic retry must bring
// the tunnel up — this is the core of the "залипание → реконнект" recovery.
func TestStartEngine_RetriesTransientTunErrorThenSucceeds(t *testing.T) {
	fastTunRetry(t)
	fakeAdmin(t, true)

	eng := &seqEngine{errs: []error{
		errors.New("start inbound/tun[tun-in]: configure tun interface: Access is denied."),
	}}
	m := NewManager(logger.New())
	m.engine = eng

	err, _, _, _ := m.startEngine(context.Background(), EngineConfig{Mode: ProxyModeTunnel})
	if err != nil {
		t.Fatalf("expected success after one retry, got %v", err)
	}
	if eng.calls != 2 {
		t.Fatalf("expected exactly 2 start attempts (fail + retry), got %d", eng.calls)
	}
}

// When the user is already elevated, a persistent TUN setup failure is NOT a
// privileges problem — reporting tun_privileges would pop a futile UAC prompt
// (the exact bug). It must be downgraded to a generic engine-start error while
// preserving the real reason.
func TestStartEngine_PersistentTunErrorNotPrivilegesWhenElevated(t *testing.T) {
	fastTunRetry(t)
	fakeAdmin(t, true)

	tunErr := errors.New("configure tun interface: Access is denied.")
	eng := &seqEngine{errs: []error{tunErr, tunErr}}
	m := NewManager(logger.New())
	m.engine = eng

	err, tunnelFailed, reason, errorCode := m.startEngine(context.Background(), EngineConfig{Mode: ProxyModeTunnel})
	if err == nil {
		t.Fatal("expected persistent failure")
	}
	if eng.calls != 2 {
		t.Fatalf("expected 2 attempts before giving up, got %d", eng.calls)
	}
	if !tunnelFailed {
		t.Fatal("expected tunnelFailed=true")
	}
	if errorCode != ConnectErrorEngineStart {
		t.Fatalf("elevated user must not get tun_privileges, got %q", errorCode)
	}
	if !strings.Contains(strings.ToLower(reason), "access is denied") {
		t.Fatalf("reason must carry the real cause, got %q", reason)
	}
}

// Without elevation a TUN privilege error is genuine — keep tun_privileges so
// the UI can offer "restart as administrator".
func TestStartEngine_PersistentTunErrorKeepsPrivilegesWhenNotElevated(t *testing.T) {
	fastTunRetry(t)
	fakeAdmin(t, false)

	tunErr := errors.New("configure tun interface: Access is denied.")
	eng := &seqEngine{errs: []error{tunErr, tunErr}}
	m := NewManager(logger.New())
	m.engine = eng

	_, _, _, errorCode := m.startEngine(context.Background(), EngineConfig{Mode: ProxyModeTunnel})
	if errorCode != ConnectErrorTunPrivileges {
		t.Fatalf("non-elevated user should keep tun_privileges, got %q", errorCode)
	}
}

// Non-TUN failures (DNS, handshake, etc.) are not adapter залипание — retrying
// them only delays the real error. Must fail on the first attempt.
func TestStartEngine_NoRetryForNonTransientError(t *testing.T) {
	fastTunRetry(t)
	fakeAdmin(t, true)

	eng := &seqEngine{errs: []error{errors.New("dns: server misbehaving")}}
	m := NewManager(logger.New())
	m.engine = eng

	err, _, _, _ := m.startEngine(context.Background(), EngineConfig{Mode: ProxyModeTunnel})
	if err == nil {
		t.Fatal("expected failure")
	}
	if eng.calls != 1 {
		t.Fatalf("non-transient error must not retry, got %d calls", eng.calls)
	}
}

// The Wintun retry is tunnel-specific. Proxy mode never creates a TUN, so even a
// TUN-looking error there must not trigger a retry.
func TestStartEngine_NoRetryInProxyMode(t *testing.T) {
	fastTunRetry(t)
	fakeAdmin(t, true)

	eng := &seqEngine{errs: []error{errors.New("configure tun interface: Access is denied.")}}
	m := NewManager(logger.New())
	m.engine = eng

	err, _, _, _ := m.startEngine(context.Background(), EngineConfig{Mode: ProxyModeProxy})
	if err == nil {
		t.Fatal("expected failure")
	}
	if eng.calls != 1 {
		t.Fatalf("proxy mode must not retry TUN errors, got %d calls", eng.calls)
	}
}

// A clean first start must not retry and must report success with no error code.
func TestStartEngine_SuccessFirstTryNoRetry(t *testing.T) {
	fastTunRetry(t)
	fakeAdmin(t, true)

	eng := &seqEngine{}
	m := NewManager(logger.New())
	m.engine = eng

	err, tunnelFailed, _, errorCode := m.startEngine(context.Background(), EngineConfig{Mode: ProxyModeTunnel})
	if err != nil || tunnelFailed || errorCode != "" {
		t.Fatalf("clean start should be pristine, got err=%v tunnelFailed=%v code=%q", err, tunnelFailed, errorCode)
	}
	if eng.calls != 1 {
		t.Fatalf("expected a single start, got %d", eng.calls)
	}
}
