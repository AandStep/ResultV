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
	"testing"
)

// На только что созданном TUN-адаптере read-back не успевает увидеть значение
// с первой попытки, и код уходил в PowerShell — 3.5 с против 26 мс нативно.
// Пара коротких повторов снимает откат, не ослабляя гарантию
// «применено и подтверждено».
func TestSetAdapterDNS_RetriesVerifyBeforeFallingBackToPowerShell(t *testing.T) {
	oldSet, oldVerify, oldPS := setAdapterDNSNativeFn, verifyAdapterDNSFn, setAdapterDNSPowerShellFn
	defer func() {
		setAdapterDNSNativeFn, verifyAdapterDNSFn, setAdapterDNSPowerShellFn = oldSet, oldVerify, oldPS
	}()

	setAdapterDNSNativeFn = func(int, []string) error { return nil }
	verifyCalls := 0
	verifyAdapterDNSFn = func(int, []string) bool {
		verifyCalls++
		return verifyCalls >= 2 // первая попытка не видит значение, вторая видит
	}
	setAdapterDNSPowerShellFn = func(int, []string) error {
		t.Error("откат на PowerShell не нужен: нативная проверка подтвердилась на повторе")
		return nil
	}

	usedPS, err := setAdapterDNS(12, []string{"172.19.0.2"})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if usedPS {
		t.Fatal("ожидали нативный путь без отката")
	}
	if verifyCalls != 2 {
		t.Fatalf("ожидали 2 попытки проверки, было %d", verifyCalls)
	}
}

// Если значение так и не подтвердилось — откат обязан сработать: это
// leak-protection, подтверждённое состояние важнее скорости.
func TestSetAdapterDNS_FallsBackToPowerShellWhenVerifyNeverConfirms(t *testing.T) {
	oldSet, oldVerify, oldPS := setAdapterDNSNativeFn, verifyAdapterDNSFn, setAdapterDNSPowerShellFn
	defer func() {
		setAdapterDNSNativeFn, verifyAdapterDNSFn, setAdapterDNSPowerShellFn = oldSet, oldVerify, oldPS
	}()

	setAdapterDNSNativeFn = func(int, []string) error { return nil }
	verifyCalls := 0
	verifyAdapterDNSFn = func(int, []string) bool { verifyCalls++; return false }
	psCalled := false
	setAdapterDNSPowerShellFn = func(int, []string) error { psCalled = true; return nil }

	usedPS, err := setAdapterDNS(12, []string{"172.19.0.2"})
	if err != nil || !usedPS || !psCalled {
		t.Fatalf("ожидали откат на PowerShell: usedPS=%v psCalled=%v err=%v", usedPS, psCalled, err)
	}
	if verifyCalls != adapterDNSVerifyAttempts {
		t.Fatalf("ожидали %d попыток проверки, было %d", adapterDNSVerifyAttempts, verifyCalls)
	}
}

// Ошибка самого нативного применения — повторять проверку незачем.
func TestSetAdapterDNS_SkipsVerifyRetriesWhenNativeApplyFails(t *testing.T) {
	oldSet, oldVerify, oldPS := setAdapterDNSNativeFn, verifyAdapterDNSFn, setAdapterDNSPowerShellFn
	defer func() {
		setAdapterDNSNativeFn, verifyAdapterDNSFn, setAdapterDNSPowerShellFn = oldSet, oldVerify, oldPS
	}()

	setAdapterDNSNativeFn = func(int, []string) error { return errors.New("status 87") }
	verifyAdapterDNSFn = func(int, []string) bool {
		t.Error("проверять нечего: нативное применение не удалось")
		return false
	}
	setAdapterDNSPowerShellFn = func(int, []string) error { return nil }

	if usedPS, err := setAdapterDNS(12, []string{"172.19.0.2"}); !usedPS || err != nil {
		t.Fatalf("ожидали откат на PowerShell без ошибки: usedPS=%v err=%v", usedPS, err)
	}
}
