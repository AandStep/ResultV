//go:build windows

package proxy

import (
	"errors"
	"testing"
	"time"
)

func TestDNSPhaseTimings_EmptyFormatsToEmpty(t *testing.T) {
	var tm dnsPhaseTimings
	if got := tm.take(); got != "" {
		t.Fatalf("timings without any recorded step must format to \"\", got %q", got)
	}
}

func TestDNSPhaseTimings_FormatAndReset(t *testing.T) {
	var tm dnsPhaseTimings
	tm.recordList(12*time.Millisecond, false)
	tm.recordSnapshot(3 * time.Millisecond)
	tm.recordSet(700*time.Millisecond, false)
	tm.recordSet(680*time.Millisecond, true)
	tm.recordTun(126*time.Millisecond, false)

	want := "list=12ms(native) snapshot=3ms adapters=2 set=1380ms(ps=1) tun=126ms(native)"
	if got := tm.take(); got != want {
		t.Fatalf("take() = %q, want %q", got, want)
	}
	if got := tm.take(); got != "" {
		t.Fatalf("take() must reset, second call returned %q", got)
	}
}

func TestDNSPhaseTimings_TunOnly(t *testing.T) {
	var tm dnsPhaseTimings
	tm.recordTun(40*time.Millisecond, true)
	want := "list=0ms(native) snapshot=0ms adapters=0 set=0ms(ps=0) tun=40ms(ps)"
	if got := tm.take(); got != want {
		t.Fatalf("take() = %q, want %q", got, want)
	}
}

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
