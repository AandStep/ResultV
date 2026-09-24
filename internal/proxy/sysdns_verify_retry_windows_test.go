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

import (
	"errors"
	"reflect"
	"testing"
)

// На только что созданном TUN-адаптере read-back не успевает увидеть значение
// с первой попытки, и код уходил в PowerShell — 3.5 с против 26 мс нативно.
// Пара коротких повторов снимает откат, не ослабляя гарантию
// «применено и подтверждено».
func TestSetAdapterDNS_RetriesVerifyBeforeFallingBack(t *testing.T) {
	stubAdapterDNSSetters(t)
	setAdapterDNSNativeFn = func(int, []string) error { return nil }
	verifyCalls := 0
	verifyAdapterDNSFn = func(int, []string) bool {
		verifyCalls++
		return verifyCalls >= 2 // первая попытка не видит значение, вторая видит
	}

	path, err := setAdapterDNS(12, []string{"172.19.0.2"})
	if err != nil || path != dnsPathNative {
		t.Fatalf("ожидали нативный путь без отката: path=%v err=%v", path, err)
	}
	if verifyCalls != 2 {
		t.Fatalf("ожидали 2 попытки проверки, было %d", verifyCalls)
	}
}

// Если значение так и не подтвердилось ни нативно, ни через netsh — остаётся
// PowerShell: это leak-protection, подтверждённое состояние важнее скорости.
func TestSetAdapterDNS_FallsBackToPowerShellWhenVerifyNeverConfirms(t *testing.T) {
	calls := stubAdapterDNSSetters(t)
	setAdapterDNSNativeFn = func(int, []string) error { return nil }
	verifyCalls := 0
	verifyAdapterDNSFn = func(int, []string) bool { verifyCalls++; return false }

	path, err := setAdapterDNS(12, []string{"172.19.0.2"})
	if err != nil || path != dnsPathPowerShell || !calls.netsh || !calls.ps {
		t.Fatalf("ожидали netsh, затем PowerShell: path=%v calls=%+v err=%v", path, *calls, err)
	}
	if verifyCalls != 2*adapterDNSVerifyAttempts {
		t.Fatalf("ожидали %d попыток проверки, было %d", 2*adapterDNSVerifyAttempts, verifyCalls)
	}
}

// Нативный вызов недоступен (Windows 10 до 2004) — как у официального
// клиента AmneziaWG, следующим идёт netsh, PowerShell не нужен.
func TestSetAdapterDNS_NetshWhenNativeFails(t *testing.T) {
	calls := stubAdapterDNSSetters(t)
	setAdapterDNSNativeFn = func(int, []string) error { return errors.New("proc not found") }
	verifyAdapterDNSFn = func(int, []string) bool { return calls.netsh }

	path, err := setAdapterDNS(12, []string{"172.19.0.2"})
	if err != nil || path != dnsPathNetsh || calls.ps {
		t.Fatalf("ожидали netsh без PowerShell: path=%v calls=%+v err=%v", path, *calls, err)
	}
}

func TestSetAdapterDNS_PowerShellWhenNetshFails(t *testing.T) {
	calls := stubAdapterDNSSetters(t)
	setAdapterDNSNativeFn = func(int, []string) error { return errors.New("status 87") }
	setAdapterDNSNetshFn = func(int, []string) error { calls.netsh = true; return errors.New("exit status 1") }
	verifyAdapterDNSFn = func(int, []string) bool {
		t.Error("проверять нечего: ни один способ применения не удался")
		return false
	}

	path, err := setAdapterDNS(12, []string{"172.19.0.2"})
	if err != nil || path != dnsPathPowerShell || !calls.netsh || !calls.ps {
		t.Fatalf("ожидали откат на PowerShell: path=%v calls=%+v err=%v", path, *calls, err)
	}
}

func TestResetAdapterDNS_NetshWhenNativeFails(t *testing.T) {
	calls := stubAdapterDNSSetters(t)
	resetAdapterDNSNativeDispatchFn = func(int) error { return errors.New("proc not found") }

	path, err := resetAdapterDNS(12)
	if err != nil || path != dnsPathNetsh || calls.ps {
		t.Fatalf("ожидали netsh без PowerShell: path=%v calls=%+v err=%v", path, *calls, err)
	}
}

func TestNetshSetDNSCommands(t *testing.T) {
	got, err := netshSetDNSCommands(7, []string{"1.1.1.1", "2606:4700::1111", "8.8.8.8"})
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"interface", "ipv4", "set", "dnsservers", "name=7", "source=static", "address=1.1.1.1", "validate=no"},
		{"interface", "ipv4", "add", "dnsservers", "name=7", "address=8.8.8.8", "index=2", "validate=no"},
		{"interface", "ipv6", "set", "dnsservers", "name=7", "source=static", "address=2606:4700::1111", "validate=no"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if _, err := netshSetDNSCommands(7, []string{"1.1.1.1 & calc"}); err == nil {
		t.Fatal("ожидали отказ на небезопасном токене")
	}
}

type adapterDNSCalls struct{ netsh, ps bool }

func stubAdapterDNSSetters(t *testing.T) *adapterDNSCalls {
	t.Helper()
	oldSet, oldVerify, oldNetsh, oldPS := setAdapterDNSNativeFn, verifyAdapterDNSFn, setAdapterDNSNetshFn, setAdapterDNSPowerShellFn
	oldReset, oldResetNetsh, oldResetPS := resetAdapterDNSNativeDispatchFn, resetAdapterDNSNetshFn, resetAdapterDNSPowerShellFn
	t.Cleanup(func() {
		setAdapterDNSNativeFn, verifyAdapterDNSFn, setAdapterDNSNetshFn, setAdapterDNSPowerShellFn = oldSet, oldVerify, oldNetsh, oldPS
		resetAdapterDNSNativeDispatchFn, resetAdapterDNSNetshFn, resetAdapterDNSPowerShellFn = oldReset, oldResetNetsh, oldResetPS
	})
	calls := &adapterDNSCalls{}
	setAdapterDNSNetshFn = func(int, []string) error { calls.netsh = true; return nil }
	setAdapterDNSPowerShellFn = func(int, []string) error { calls.ps = true; return nil }
	resetAdapterDNSNetshFn = func(int) error { calls.netsh = true; return nil }
	resetAdapterDNSPowerShellFn = func(int) error { calls.ps = true; return nil }
	return calls
}
