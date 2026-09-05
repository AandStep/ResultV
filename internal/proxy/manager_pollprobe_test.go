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
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// A link that comes up on the second poll must be confirmed within ~one
// interval, not after a fixed multi-second sleep. This is the core of the
// connect speedup.
func TestPollProbe_ConfirmsEarly(t *testing.T) {
	calls := 0
	start := time.Now()
	ok, cancelled, reason := pollProbe(context.Background(), 8*time.Second, 50*time.Millisecond, func() (bool, string) {
		calls++
		return calls >= 2, ""
	})
	elapsed := time.Since(start)

	if !ok || cancelled {
		t.Fatalf("expected ok, got ok=%v cancelled=%v reason=%q", ok, cancelled, reason)
	}
	if calls != 2 {
		t.Fatalf("expected 2 attempts, got %d", calls)
	}
	if elapsed > time.Second {
		t.Fatalf("expected early confirmation (~1 interval), took %v", elapsed)
	}
}

// A dead link fails no sooner than the deadline (preserving worst-case
// robustness) and reports the last failure reason — never claims cancelled.
func TestPollProbe_FailsAtDeadline(t *testing.T) {
	start := time.Now()
	ok, cancelled, reason := pollProbe(context.Background(), 300*time.Millisecond, 50*time.Millisecond, func() (bool, string) {
		return false, "still warming up"
	})
	elapsed := time.Since(start)

	if ok || cancelled {
		t.Fatalf("expected failure, got ok=%v cancelled=%v", ok, cancelled)
	}
	if reason != "still warming up" {
		t.Fatalf("expected last reason propagated, got %q", reason)
	}
	if elapsed < 250*time.Millisecond {
		t.Fatalf("expected to use the full deadline budget, only took %v", elapsed)
	}
}

// A cancelled context aborts promptly and is reported as cancelled (so the
// caller surfaces ErrorCode "cancelled", not a probe failure).
func TestPollProbe_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ok, cancelled, _ := pollProbe(ctx, 8*time.Second, 50*time.Millisecond, func() (bool, string) {
		return false, "unreached"
	})
	if ok || !cancelled {
		t.Fatalf("expected cancelled, got ok=%v cancelled=%v", ok, cancelled)
	}
}

// В режиме туннеля контрольная проба «жив ли сервер» обязана уходить с
// физического адаптера: иначе она меряет локальный стек sing-tun и объявляет
// живым сервер, до которого нет связи, — ровно та ошибка, из-за которой
// неудачный коннект рапортовался как «proxy outbound misconfigured».
func TestRunPostStartProbe_Hysteria2TunnelUsesLANBindLivenessProbe(t *testing.T) {
	oldHTTP := probeHTTPThroughProxyProbe
	oldHY, oldHYLAN := pingHysteria2Probe, pingHysteria2LANProbe
	defer func() {
		probeHTTPThroughProxyProbe = oldHTTP
		pingHysteria2Probe, pingHysteria2LANProbe = oldHY, oldHYLAN
	}()

	probeHTTPThroughProxyProbe = func(string) (bool, string) { return false, "timeout" }
	pingHysteria2Probe = func(_ string, _ int) (int64, bool, string, string) {
		t.Error("в режиме туннеля должна использоваться LAN-bind проба")
		return 0, true, "", ""
	}
	lanCalled := false
	pingHysteria2LANProbe = func(_ string, _ int) (int64, bool, string, string) {
		lanCalled = true
		return 0, false, "timeout", "quic_handshake_lan_bind"
	}

	code, reason := runPostStartProbe(context.Background(), "hysteria2", "1.2.3.4", 443, 1080, ProxyModeTunnel)

	if !lanCalled {
		t.Fatal("LAN-bind проба не вызвана")
	}
	if code != "post_start_probe_failed" {
		t.Fatalf("ожидали post_start_probe_failed, получили %q", code)
	}
	if reason != "timeout" {
		t.Fatalf("ожидали причину от пробы (timeout), получили %q", reason)
	}
}

// В режиме прокси туннеля нет, привязка не нужна — остаётся обычная проба.
func TestRunPostStartProbe_Hysteria2ProxyModeUsesPlainLivenessProbe(t *testing.T) {
	oldHTTP := probeHTTPThroughProxyProbe
	oldHY, oldHYLAN := pingHysteria2Probe, pingHysteria2LANProbe
	defer func() {
		probeHTTPThroughProxyProbe = oldHTTP
		pingHysteria2Probe, pingHysteria2LANProbe = oldHY, oldHYLAN
	}()

	probeHTTPThroughProxyProbe = func(string) (bool, string) { return false, "timeout" }
	pingHysteria2LANProbe = func(_ string, _ int) (int64, bool, string, string) {
		t.Error("в режиме прокси LAN-bind проба не нужна")
		return 0, false, "", ""
	}
	plainCalled := false
	pingHysteria2Probe = func(_ string, _ int) (int64, bool, string, string) {
		plainCalled = true
		return 0, false, "timeout", "quic_handshake"
	}

	if code, _ := runPostStartProbe(context.Background(), "hysteria2", "1.2.3.4", 443, 1080, ProxyModeProxy); code != "post_start_probe_failed" {
		t.Fatalf("ожидали post_start_probe_failed, получили %q", code)
	}
	if !plainCalled {
		t.Fatal("обычная проба не вызвана")
	}
}

// Один медленный адрес не имеет права задерживать вердикт: при
// последовательном обходе одна попытка стоила до 9 с и вылетала за
// восьмисекундный бюджет, из-за чего pollProbe не делал ни одного
// повтора.
func TestProbeHTTPThroughProxy_RacesTargetsAndReturnsOnFirstSuccess(t *testing.T) {
	var slowHits, fastHits int32

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&slowHits, 1)
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer slow.Close()
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&fastHits, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer fast.Close()

	oldTargets := probeTargets
	defer func() { probeTargets = oldTargets }()
	probeTargets = func() []probeTarget {
		return []probeTarget{
			{url: slow.URL, wantStatus: http.StatusNoContent},
			{url: slow.URL, wantStatus: http.StatusNoContent},
			{url: fast.URL, wantStatus: http.StatusNoContent},
		}
	}

	start := time.Now()
	ok, reason := probeHTTPThroughProxy("")
	elapsed := time.Since(start)

	if !ok {
		t.Fatalf("ожидали успех, получили reason=%q", reason)
	}
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("вердикт занял %v — адреса опрашиваются последовательно", elapsed)
	}
	if atomic.LoadInt32(&fastHits) == 0 {
		t.Fatal("быстрый адрес не опрошен")
	}
}

// Когда не отвечает никто, должна вернуться причина, а не пустая строка.
func TestProbeHTTPThroughProxy_ReturnsReasonWhenAllTargetsFail(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadURL := dead.URL
	dead.Close() // порт закрыт — соединение будет отвергнуто

	oldTargets := probeTargets
	defer func() { probeTargets = oldTargets }()
	probeTargets = func() []probeTarget {
		return []probeTarget{
			{url: deadURL, wantStatus: http.StatusNoContent},
			{url: deadURL, wantStatus: http.StatusNoContent},
		}
	}

	ok, reason := probeHTTPThroughProxy("")
	if ok {
		t.Fatal("ожидали неудачу")
	}
	if reason == "" {
		t.Fatal("ожидали непустую причину")
	}
}

// 502 от локального инбаунда — не доказательство рабочего туннеля.
//
// Регрессия по логам 2026-09-05. Пробы ходят по обычному http, а sing на
// таком запросе не пробрасывает соединение, а сам отвечает клиенту: любая
// ошибка аутбаунда превращается в «502 Bad Gateway», написанный нашим же
// локальным инбаундом (sing/protocol/http/handshake.go, handleHTTPConnection).
// Правило «любой HTTP-ответ через прокси означает, что туннель работает»
// принимало этот ответ за успех — оттого проба и укладывалась в 2 мс на
// полностью мёртвом аутбаунде, подключение объявлялось успешным, а сторож
// здоровья потом молчал всю сессию.
func TestProbeHTTPThroughProxy_RejectsGatewayErrorFromLocalInbound(t *testing.T) {
	badGateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer badGateway.Close()

	oldTargets := probeTargets
	defer func() { probeTargets = oldTargets }()
	probeTargets = func() []probeTarget {
		return []probeTarget{
			{url: badGateway.URL, wantStatus: http.StatusNoContent},
			{url: badGateway.URL, wantStatus: http.StatusNoContent},
		}
	}

	ok, reason := probeHTTPThroughProxy("")
	if ok {
		t.Fatal("502 не доказывает, что туннель несёт трафик — его пишет наш собственный инбаунд")
	}
	if reason == "" {
		t.Fatal("ожидали непустую причину отказа")
	}
}
