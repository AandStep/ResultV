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
	"time"
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

	// Мутируем срез первого вызова. RankAutoCandidates строит его свежим при
	// каждом реальном свипе и storeAutoSweepCache копирует его на запись
	// (append(nil, candidates...)), так что first в принципе не разделяет
	// память с тем, что легло в кэш, — эта проверка ловит только regression
	// в самом storeAutoSweepCache, не в чтении. Настоящая read-side проверка
	// — ниже, между second и third.
	origName := first[0].Name
	first[0].Name = "mutated-by-caller"

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
	if second[0].Name != origName {
		t.Fatalf("правка first[0].Name (сделанного до попадания в кэш) просочилась во второй вызов (получили %q, ожидали %q)",
			second[0].Name, origName)
	}

	// Настоящая проверка read-side копии lookupAutoSweepCache: second — это
	// уже РЕЗУЛЬТАТ попадания в кэш (diag.FromCache=true выше), а не свежий
	// свип. Мутируем его и делаем третий вызов, тоже обязанный попасть в тот
	// же кэш (мы всё ещё внутри TTL) — если lookupAutoSweepCache отдаёт
	// entry.candidates напрямую без копии, эта правка переживёт в third,
	// потому что second и third разделяли бы один и тот же backing-массив
	// кэша.
	second[0].Name = "mutated-from-cache-hit"
	third, _ := RankAutoCandidates(context.Background(), nodes, "")
	if atomic.LoadInt32(&probes) != after {
		t.Fatalf("третий вызов внутри TTL тоже не должен опрашивать заново: было %d, стало %d",
			after, atomic.LoadInt32(&probes))
	}
	if third[0].Name != origName {
		t.Fatalf("lookupAutoSweepCache вернул свой собственный массив без копии: правка second[0].Name просочилась в третий вызов (получили %q, ожидали %q)",
			third[0].Name, origName)
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

// TestRankAutoCandidates_TwoGroupsBothHitCache is the regression test for the
// single-slot design this cache started with: a user alternately selecting
// two different AUTO groups produced two different cache keys that evicted
// each other on every call, so the cache never actually hit for that user.
// autoSweepCache is now a map so each group keeps its own slot.
func TestRankAutoCandidates_TwoGroupsBothHitCache(t *testing.T) {
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

	groupA := mkNodes("grp1a", "grp1b")
	groupB := mkNodes("grp2a", "grp2b")

	RankAutoCandidates(context.Background(), groupA, "")
	RankAutoCandidates(context.Background(), groupB, "")
	afterBothWarm := atomic.LoadInt32(&probes)
	if afterBothWarm == 0 {
		t.Fatal("оба первых вызова обязаны реально опросить узлы")
	}

	// Переключаемся обратно на группу A — не должна промахнуться мимо кэша
	// только из-за того, что группа B заняла единственный слот.
	RankAutoCandidates(context.Background(), groupA, "")
	if atomic.LoadInt32(&probes) != afterBothWarm {
		t.Fatalf("группа A обязана попасть в кэш после переключения на B: было %d, стало %d",
			afterBothWarm, atomic.LoadInt32(&probes))
	}

	// И группа B тоже должна остаться в кэше — её слот не должен был быть
	// вытеснен повторным попаданием группы A.
	RankAutoCandidates(context.Background(), groupB, "")
	if atomic.LoadInt32(&probes) != afterBothWarm {
		t.Fatalf("группа B обязана остаться в кэше после переключения на A: было %d, стало %d",
			afterBothWarm, atomic.LoadInt32(&probes))
	}
}

// TestRankAutoCandidates_ConnectFailureBustsCache is the regression test for
// the finding that a real connect failure used to change nodeStats() (via
// RecordConnectOutcome -> NodeStatStore.RecordConnect: ConsecFails,
// LastFailAt) without changing anything the cache key covers, so a node that
// had just failed to connect kept its cached top rank for up to
// autoSweepCacheTTL — silencing exactly the failure-penalty machinery in
// scoreNode this cache should never be allowed to override.
func TestRankAutoCandidates_ConnectFailureBustsCache(t *testing.T) {
	stubProbeResolver(t)
	oldTCP, oldTLS, oldBind, oldPick, oldStore := pingTCPProbe, autoTLSProbe, autoProbeBindsToLAN, pickLANBindIPv4, nodeStats()
	defer func() {
		pingTCPProbe, autoTLSProbe, autoProbeBindsToLAN, pickLANBindIPv4 = oldTCP, oldTLS, oldBind, oldPick
		SetNodeStatStore(oldStore)
		ResetAutoSweepCache()
	}()
	ResetAutoSweepCache()
	autoProbeBindsToLAN = func() bool { return false }
	pickLANBindIPv4 = func() (net.IP, error) { return net.IPv4(192, 168, 1, 5), nil }
	autoTLSProbe = func(string, int, string, []string) (int64, bool, string) { return 20, true, "" }
	// Изолированный стор: изолируемся от любых other тестов, использующих те
	// же имена "cf1"/"cf2", раз этот тест сам пишет в nodeStats() через
	// RecordConnectOutcome.
	SetNodeStatStore(NewNodeStatStore(t.TempDir()))

	var probes int32
	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		atomic.AddInt32(&probes, 1)
		return 30, true, ""
	}

	nodes := mkNodes("cf1", "cf2")
	first, _ := RankAutoCandidates(context.Background(), nodes, "")
	after := atomic.LoadInt32(&probes)
	if after == 0 {
		t.Fatal("первый вызов обязан реально опросить узлы")
	}

	// Второй вызов внутри TTL обязан попасть в кэш — контрольная проверка,
	// что тест действительно застаёт кэш тёплым перед тем, как его сломает
	// неудачный коннект.
	RankAutoCandidates(context.Background(), nodes, "")
	if atomic.LoadInt32(&probes) != after {
		t.Fatalf("контрольный вызов обязан был попасть в кэш: было %d, стало %d", after, atomic.LoadInt32(&probes))
	}

	RecordConnectOutcome(AutoNodeKey(first[0]), false, "error")

	RankAutoCandidates(context.Background(), nodes, "")
	if atomic.LoadInt32(&probes) == after {
		t.Fatal("неудачный коннект обязан сбросить кэш свипа, чтобы штраф из scoreNode реально сработал на следующем вызове")
	}
}

func TestRankAutoCandidates_ReportsPhaseDurations(t *testing.T) {
	oldTCP, oldTLS, oldBind, oldPick := pingTCPProbe, autoTLSProbe, autoProbeBindsToLAN, pickLANBindIPv4
	defer func() {
		pingTCPProbe, autoTLSProbe, autoProbeBindsToLAN, pickLANBindIPv4 = oldTCP, oldTLS, oldBind, oldPick
		ResetAutoSweepCache()
	}()
	ResetAutoSweepCache()
	stubProbeResolver(t) // фикстуры mkNodes содержат имена, а не адреса
	autoProbeBindsToLAN = func() bool { return false }
	pickLANBindIPv4 = func() (net.IP, error) { return net.IPv4(192, 168, 1, 5), nil }

	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		time.Sleep(20 * time.Millisecond)
		return 30, true, ""
	}
	autoTLSProbe = func(string, int, string, []string) (int64, bool, string) {
		time.Sleep(10 * time.Millisecond)
		return 20, true, ""
	}

	_, diag := RankAutoCandidates(context.Background(), mkNodes("a", "b"), "")
	if diag.Phase1Dur <= 0 {
		t.Fatalf("ожидали ненулевую длительность фазы 1, получили %v", diag.Phase1Dur)
	}
	if diag.Phase2Dur <= 0 {
		t.Fatalf("ожидали ненулевую длительность фазы 2, получили %v", diag.Phase2Dur)
	}
	if diag.FromCache {
		t.Fatal("свежий свип не должен помечаться как кэш")
	}
}

// Попадание в кэш ничего не измеряет на ЭТОМ вызове: entry.diag хранит
// длительности исходного свипа, которому может быть до autoSweepCacheTTL
// (90с). Без явного зануления в lookupAutoSweepCache любой будущий читатель,
// не проверивший FromCache первым (новая строка лога, панель диагностики,
// рефакторинг, потерявший ветку), напечатал бы устаревшую длительность как
// будто она измерена только что.
func TestRankAutoCandidates_CacheHitReportsZeroDurations(t *testing.T) {
	oldTCP, oldTLS, oldBind, oldPick := pingTCPProbe, autoTLSProbe, autoProbeBindsToLAN, pickLANBindIPv4
	defer func() {
		pingTCPProbe, autoTLSProbe, autoProbeBindsToLAN, pickLANBindIPv4 = oldTCP, oldTLS, oldBind, oldPick
		ResetAutoSweepCache()
	}()
	ResetAutoSweepCache()
	stubProbeResolver(t)
	autoProbeBindsToLAN = func() bool { return false }
	pickLANBindIPv4 = func() (net.IP, error) { return net.IPv4(192, 168, 1, 5), nil }

	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		time.Sleep(5 * time.Millisecond)
		return 30, true, ""
	}
	autoTLSProbe = func(string, int, string, []string) (int64, bool, string) {
		time.Sleep(5 * time.Millisecond)
		return 20, true, ""
	}

	nodes := mkNodes("zd1", "zd2")
	_, first := RankAutoCandidates(context.Background(), nodes, "")
	if first.Phase1Dur <= 0 || first.Phase2Dur <= 0 {
		t.Fatalf("контрольный первый вызов обязан реально измерить обе фазы, получили %v/%v",
			first.Phase1Dur, first.Phase2Dur)
	}

	_, second := RankAutoCandidates(context.Background(), nodes, "")
	if !second.FromCache {
		t.Fatal("второй вызов внутри TTL обязан попасть в кэш")
	}
	if second.Phase1Dur != 0 {
		t.Fatalf("попадание в кэш не должно нести длительность фазы 1 предыдущего свипа, получили %v", second.Phase1Dur)
	}
	if second.Phase2Dur != 0 {
		t.Fatalf("попадание в кэш не должно нести длительность фазы 2 предыдущего свипа, получили %v", second.Phase2Dur)
	}
}
