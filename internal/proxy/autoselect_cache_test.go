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
	"net"
	"sync/atomic"
	"testing"
)

func TestRankAutoCandidates_SecondCallWithinTTLReusesSweep(t *testing.T) {
	stubProbeResolver(t)
	oldTCP, oldTLS, oldBind, oldPick := pingTCPProbe, autoTLSProbe, autoProbeBindsToLAN, pickLANBindIPv4
	defer func() {
		pingTCPProbe, autoTLSProbe, autoProbeBindsToLAN, pickLANBindIPv4 = oldTCP, oldTLS, oldBind, oldPick
		ResetAutoSweepCache()
	}()
	ResetAutoSweepCache()
	autoProbeBindsToLAN = func() bool { return false }
	pickLANBindIPv4 = func() (net.IP, error) { return net.IPv4(192, 168, 1, 5), nil }
	autoTLSProbe = func(string, int, string, []string) (int64, bool, string) { return 20, true, "" }

	var probes int32
	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		atomic.AddInt32(&probes, 1)
		return 30, true, ""
	}

	nodes := mkNodes("a", "b", "c")
	first, _ := RankAutoCandidates(context.Background(), nodes, "")
	after := atomic.LoadInt32(&probes)
	if after == 0 {
		t.Fatal("первый вызов обязан реально опросить узлы")
	}

	second, diag := RankAutoCandidates(context.Background(), nodes, "")
	if atomic.LoadInt32(&probes) != after {
		t.Fatalf("второй вызов внутри TTL не должен опрашивать заново: было %d, стало %d",
			after, atomic.LoadInt32(&probes))
	}
	if !diag.FromCache {
		t.Fatal("ожидали diag.FromCache=true на попадании в кэш")
	}
	if len(first) != len(second) {
		t.Fatalf("кэш вернул другой набор: %d против %d", len(first), len(second))
	}
	for i := range first {
		if first[i].IP != second[i].IP {
			t.Fatalf("порядок кандидатов из кэша разошёлся на позиции %d", i)
		}
	}
}

// Другой набор узлов — другой ключ, кэш обязан промахнуться.
func TestRankAutoCandidates_DifferentMembersMissCache(t *testing.T) {
	stubProbeResolver(t)
	oldTCP, oldTLS, oldBind, oldPick := pingTCPProbe, autoTLSProbe, autoProbeBindsToLAN, pickLANBindIPv4
	defer func() {
		pingTCPProbe, autoTLSProbe, autoProbeBindsToLAN, pickLANBindIPv4 = oldTCP, oldTLS, oldBind, oldPick
		ResetAutoSweepCache()
	}()
	ResetAutoSweepCache()
	autoProbeBindsToLAN = func() bool { return false }
	pickLANBindIPv4 = func() (net.IP, error) { return net.IPv4(192, 168, 1, 5), nil }
	autoTLSProbe = func(string, int, string, []string) (int64, bool, string) { return 20, true, "" }

	var probes int32
	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		atomic.AddInt32(&probes, 1)
		return 30, true, ""
	}

	RankAutoCandidates(context.Background(), mkNodes("a", "b"), "")
	after := atomic.LoadInt32(&probes)
	RankAutoCandidates(context.Background(), mkNodes("a", "b", "c"), "")
	if atomic.LoadInt32(&probes) == after {
		t.Fatal("смена состава группы обязана промахнуться мимо кэша")
	}
}

// Смена сети меняет bind-адрес, а значит и ключ: держать замеры со старого
// линка нельзя.
func TestRankAutoCandidates_BindAddressChangeMissesCache(t *testing.T) {
	stubProbeResolver(t)
	oldTCP, oldTLS, oldBind, oldPick := pingTCPProbe, autoTLSProbe, autoProbeBindsToLAN, pickLANBindIPv4
	defer func() {
		pingTCPProbe, autoTLSProbe, autoProbeBindsToLAN, pickLANBindIPv4 = oldTCP, oldTLS, oldBind, oldPick
		ResetAutoSweepCache()
	}()
	ResetAutoSweepCache()
	autoProbeBindsToLAN = func() bool { return false }
	autoTLSProbe = func(string, int, string, []string) (int64, bool, string) { return 20, true, "" }

	var probes int32
	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		atomic.AddInt32(&probes, 1)
		return 30, true, ""
	}

	nodes := mkNodes("a", "b")
	pickLANBindIPv4 = func() (net.IP, error) { return net.IPv4(192, 168, 1, 5), nil }
	RankAutoCandidates(context.Background(), nodes, "")
	after := atomic.LoadInt32(&probes)

	pickLANBindIPv4 = func() (net.IP, error) { return net.IPv4(10, 8, 0, 3), nil }
	RankAutoCandidates(context.Background(), nodes, "")
	if atomic.LoadInt32(&probes) == after {
		t.Fatal("смена bind-адреса обязана промахнуться мимо кэша")
	}
}

// Пустой результат кэшировать нельзя: «сейчас никто не отвечает» — состояние,
// которое обязано перепроверяться на следующем клике, а не залипать на 90 с.
func TestRankAutoCandidates_EmptyResultIsNotCached(t *testing.T) {
	stubProbeResolver(t)
	oldTCP, oldBind, oldPick, oldStore := pingTCPProbe, autoProbeBindsToLAN, pickLANBindIPv4, nodeStats()
	defer func() {
		pingTCPProbe, autoProbeBindsToLAN, pickLANBindIPv4 = oldTCP, oldBind, oldPick
		SetNodeStatStore(oldStore)
		ResetAutoSweepCache()
	}()
	ResetAutoSweepCache()
	autoProbeBindsToLAN = func() bool { return false }
	pickLANBindIPv4 = func() (net.IP, error) { return net.IPv4(192, 168, 1, 5), nil }
	// Изолированный стор: этот тест намеренно проваливает пробы для узлов "a"
	// и "b", а RecordProbe пишет LastFailAt даже при отказе — с общим глобальным
	// стором этот штраф пережил бы тест и испортил бы порядок в
	// TestRankAutoCandidates_OrdersByRTTAndCapsAtFive, который переиспользует
	// те же имена "a"/"b" среди своих узлов.
	SetNodeStatStore(NewNodeStatStore(t.TempDir()))

	var probes int32
	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		atomic.AddInt32(&probes, 1)
		return 0, false, "timeout"
	}

	nodes := mkNodes("a", "b")
	RankAutoCandidates(context.Background(), nodes, "")
	after := atomic.LoadInt32(&probes)
	RankAutoCandidates(context.Background(), nodes, "")
	if atomic.LoadInt32(&probes) == after {
		t.Fatal("пустой результат не должен кэшироваться")
	}
}
