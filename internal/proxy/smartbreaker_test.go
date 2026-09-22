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
	"net/netip"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
)

// The breaker exists so a broken link cannot write seven-day verdicts. It must
// not also stop the hedging that is the only thing saving the user while a set
// of destinations is blackholed — that is what made the engine turn itself off
// on its own successes (2026-09-22).

func TestProxyWinIsNotEvidenceAboutTheLink(t *testing.T) {
	report, _ := raceLinkEvidence(raceResult{ViaProxy: true})
	if report {
		t.Fatal("победа узла записалась как отказ прямого пути: один заблокированный адрес — не сломанный канал")
	}
}

func TestDirectWinIsEvidenceTheLinkWorks(t *testing.T) {
	report, ok := raceLinkEvidence(raceResult{})
	if !report || !ok {
		t.Fatalf("победа direct должна подтверждать канал, получено report=%v ok=%v", report, ok)
	}
}

func TestRaceLostOnBothLegsIsEvidenceAgainstTheLink(t *testing.T) {
	report, ok := raceLinkEvidence(raceResult{Err: errors.New("оба пути молчат")})
	if !report || ok {
		t.Fatalf("провал обеих ног должен быть уликой против канала, получено report=%v ok=%v", report, ok)
	}
}

// A page pulling five blacklisted CDNs used to trip the breaker through
// raceConnection; with proxy wins no longer filed, it stays closed.
func TestRescuedSitesDoNotTripTheBreaker(t *testing.T) {
	now := time.Now()
	h := newDirectHealth(func() time.Time { return now })
	for _, host := range []string{"cdnjs.cloudflare.com", "unpkg.com", "vjs.zencdn.net", "i.imgur.com", "raw.githubusercontent.com", "registry.npmjs.org"} {
		if report, ok := raceLinkEvidence(raceResult{ViaProxy: true}); report {
			h.record(host, ok)
		}
	}
	if !h.healthy() {
		t.Fatal("предохранитель взвёлся от сайтов, которые гонка спасла")
	}
}

func TestUnknownDestinationIsRacedWhileTheBreakerIsOpen(t *testing.T) {
	store := choiceStore(t)
	if got := decideSmart(store, "example.com", netip.Addr{}); got != chooseRace {
		t.Fatalf("неизвестное направление должно идти в гонку независимо от предохранителя, получено %v", got)
	}
}

func TestOpenBreakerStillStopsLearning(t *testing.T) {
	now := time.Now()
	h := newDirectHealth(func() time.Time { return now })
	for _, host := range []string{"a.com", "b.com", "c.com", "d.com", "e.com"} {
		h.record(host, false)
	}
	if h.healthy() {
		t.Fatal("пять отказов подряд обязаны взвести предохранитель")
	}

	store := choiceStore(t)
	s := &smartOutbound{store: store, health: h}
	s.learn(adapter.InboundContext{Domain: "example.com"}, true)
	if _, ok := store.Lookup("example.com"); ok {
		t.Fatal("вердикт записан при взведённом предохранителе — именно этого он должен не допускать")
	}
}
