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
	"testing"

	"resultproxy-wails/internal/logger"
	"resultproxy-wails/internal/system"
)

const tunBusyMessage = "starting sing-box: start inbound/tun[tun-in]: configure tun interface: Cannot create a file when that file already exists."

func stubReleaseTunHolders(t *testing.T, rep system.OtherInstancesReport) *int {
	t.Helper()
	calls := new(int)
	prev := releaseTunHoldersFn
	releaseTunHoldersFn = func() system.OtherInstancesReport { *calls++; return rep }
	t.Cleanup(func() { releaseTunHoldersFn = prev })
	return calls
}

func TestIsTunAdapterBusyError(t *testing.T) {
	cases := map[string]bool{
		tunBusyMessage: true,
		"configure tun interface: set ipv4 address: Cannot create a file when that file already exists.": false,
		"configure tun interface: The system cannot find the file specified.":                              false,
		"dns: server misbehaving": false,
	}
	for msg, want := range cases {
		if got := isTunAdapterBusyError(errors.New(msg)); got != want {
			t.Errorf("%q: got %v want %v", msg, got, want)
		}
	}
}

// Another live process still owns the adapter. Removing the device does not
// free it (measured: pnputil reports success and the next create fails the same
// way until the owner exits), so the retry has to deal with the owner instead.
func TestStartEngine_BusyAdapterReleasesHoldersInsteadOfRemovingDevice(t *testing.T) {
	fastTunRetry(t)
	fakeAdmin(t, true)
	removed := stubRemoveStaleTunAdapter(t)
	released := stubReleaseTunHolders(t, system.OtherInstancesReport{Found: []uint32{42}, Killed: []uint32{42}})

	eng := &seqEngine{errs: []error{errors.New(tunBusyMessage)}}
	m := NewManager(logger.New())
	m.engine = eng

	err, _, _, _ := m.startEngine(context.Background(), EngineConfig{Mode: ProxyModeTunnel})
	if err != nil {
		t.Fatalf("expected success after the holder was released, got %v", err)
	}
	if eng.calls != 2 || *released != 1 {
		t.Fatalf("starts=%d releases=%d, want 2 and 1", eng.calls, *released)
	}
	if *removed != 0 {
		t.Fatalf("the device must not be removed for a busy adapter, removals=%d", *removed)
	}
}

func TestStartEngine_PersistentlyBusyAdapterReportsItsOwnCode(t *testing.T) {
	fastTunRetry(t)
	fakeAdmin(t, true)
	removed := stubRemoveStaleTunAdapter(t)
	stubReleaseTunHolders(t, system.OtherInstancesReport{Found: []uint32{42}, Stuck: []uint32{42}})

	busy := errors.New(tunBusyMessage)
	eng := &seqEngine{errs: []error{busy, busy, busy, busy}}
	m := NewManager(logger.New())
	m.engine = eng

	err, tunnelFailed, _, code := m.startEngine(context.Background(), EngineConfig{Mode: ProxyModeTunnel})
	if err == nil || !tunnelFailed {
		t.Fatalf("expected a tunnel failure, got err=%v tunnelFailed=%v", err, tunnelFailed)
	}
	if code != ConnectErrorTunBusy {
		t.Fatalf("code=%q, want %q", code, ConnectErrorTunBusy)
	}
	if eng.calls != 2 || *removed != 0 {
		t.Fatalf("starts=%d removals=%d, want 2 and 0", eng.calls, *removed)
	}
}

func TestRecoverSystemLeftoversWaitsForPreviousInstances(t *testing.T) {
	fakeAdmin(t, true)
	released := stubReleaseTunHolders(t, system.OtherInstancesReport{})
	m := NewManager(logger.New())

	m.RecoverSystemLeftovers()
	if *released != 1 {
		t.Fatalf("previous instances must be awaited before the leftover scan, calls=%d", *released)
	}
}
