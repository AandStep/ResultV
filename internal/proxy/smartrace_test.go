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
	"net"
	"testing"
	"time"
)

func newPipeConnPair() (net.Conn, net.Conn) {
	return net.Pipe()
}

// liveServer answers as soon as it has been written to. That is what a working
// server does and what a censored one never does.
func liveServer(t *testing.T, reply string) raceDialer {
	t.Helper()
	return func(ctx context.Context) (net.Conn, error) {
		client, server := newPipeConnPair()
		go func() {
			buf := make([]byte, 4096)
			if _, err := server.Read(buf); err != nil {
				server.Close()
				return
			}
			server.Write([]byte(reply))
		}()
		t.Cleanup(func() { client.Close() })
		return client, nil
	}
}

// blackHoleServer accepts the write and says nothing, which is what a RST-less
// DPI drop looks like from the client side.
func blackHoleServer(t *testing.T) raceDialer {
	t.Helper()
	return func(ctx context.Context) (net.Conn, error) {
		client, server := newPipeConnPair()
		go func() {
			buf := make([]byte, 4096)
			server.Read(buf)
			<-ctx.Done()
			server.Close()
		}()
		t.Cleanup(func() { client.Close() })
		return client, nil
	}
}

// muteServer accepts the write and then stays open and silent — unlike
// blackHoleServer it does not hang up when the race's context is cancelled,
// which is what a real mute peer does and what a handed-over connection has
// to survive.
func muteServer(t *testing.T) raceDialer {
	t.Helper()
	return func(ctx context.Context) (net.Conn, error) {
		client, server := newPipeConnPair()
		go func() {
			buf := make([]byte, 4096)
			server.Read(buf)
		}()
		t.Cleanup(func() { server.Close(); client.Close() })
		return client, nil
	}
}

func refusingDialer() raceDialer {
	return func(ctx context.Context) (net.Conn, error) {
		return nil, errors.New("connection refused")
	}
}

func TestRaceTakesDirectWhenDirectAnswers(t *testing.T) {
	res := runSmartRace(context.Background(), []byte("hello"), liveServer(t, "ok"), liveServer(t, "ok"))
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	defer res.Conn.Close()
	if res.ViaProxy {
		t.Fatal("a working direct path lost the race")
	}
	if string(res.Head) != "ok" {
		t.Fatalf("head = %q, want the server's first bytes", res.Head)
	}
}

// The measured case: instagram.com takes a full ten seconds to fail on a
// Russian address because the SYN goes into a black hole. Not waiting that out
// is the whole reason the race exists.
func TestRaceFallsToProxyWhenDirectIsABlackHole(t *testing.T) {
	res := runSmartRace(context.Background(), []byte("hello"), blackHoleServer(t), liveServer(t, "ok"))
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	defer res.Conn.Close()
	if !res.ViaProxy {
		t.Fatal("a silent direct path won the race")
	}
	if string(res.Head) != "ok" {
		t.Fatalf("head = %q, want the proxy server's first bytes", res.Head)
	}
}

// An outright refusal is evidence at once — there is nothing left to wait for.
func TestRaceTakesProxyImmediatelyOnDirectRefusal(t *testing.T) {
	start := time.Now()
	res := runSmartRace(context.Background(), []byte("hello"), refusingDialer(), liveServer(t, "ok"))
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	defer res.Conn.Close()
	if !res.ViaProxy {
		t.Fatal("a refused direct dial still won")
	}
	if elapsed := time.Since(start); elapsed >= smartRaceHeadStart {
		t.Fatalf("a refusal was not acted on immediately: %v", elapsed)
	}
}

// If neither path works the connection has to fail, not hang until something
// times out on its own.
func TestRaceFailsWhenNeitherPathAnswers(t *testing.T) {
	res := runSmartRace(context.Background(), []byte("hello"), refusingDialer(), refusingDialer())
	if res.Err == nil {
		res.Conn.Close()
		t.Fatal("the race reported success with no working path")
	}
}

// The loser must not be left holding a socket: on a busy page that is one
// leaked connection per new name.
func TestRaceClosesTheLoser(t *testing.T) {
	closed := make(chan struct{}, 1)
	loser := func(ctx context.Context) (net.Conn, error) {
		client, server := newPipeConnPair()
		go func() {
			buf := make([]byte, 4096)
			server.Read(buf)
			<-ctx.Done()
			server.Close()
		}()
		return &notifyCloseConn{Conn: client, closed: closed}, nil
	}
	res := runSmartRace(context.Background(), []byte("hello"), loser, liveServer(t, "ok"))
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	defer res.Conn.Close()
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("the losing connection was never closed")
	}
}

type notifyCloseConn struct {
	net.Conn
	closed chan struct{}
}

func (c *notifyCloseConn) Close() error {
	select {
	case c.closed <- struct{}{}:
	default:
	}
	return c.Conn.Close()
}

// Решение 2026-09-22, развилка 1. По потолку гонки соединение до узла уже
// установлено и в него уже записаны байты клиента — сервер просто ещё не
// заговорил. Раньше его закрывали и отдавали клиенту отказ; про direct при
// этом уже точно известно, что он мёртв, то есть выбрасывался единственный
// оставшийся живой кандидат.
func TestSilentNodeConnectionIsHandedOverInsteadOfDropped(t *testing.T) {
	res := runSmartRace(context.Background(), []byte("hello"), refusingDialer(), blackHoleServer(t))
	if res.Err != nil {
		t.Fatalf("гонка отдала отказ вместо живого соединения узла: %v", res.Err)
	}
	if res.Conn == nil {
		t.Fatal("соединения нет")
	}
	defer res.Conn.Close()
	if !res.ViaProxy {
		t.Fatal("отдан не тот путь: direct отказал, живым оставался только узел")
	}
	if !res.Unproven {
		t.Fatal("соединение отдано как доказанное, хотя сервер по нему не сказал ни байта")
	}
}

// Дедлайн чтения, которым попытка ловит молчание, обязан быть снят перед
// передачей клиенту: иначе первое же его чтение мгновенно упадёт по таймауту.
func TestHandedOverConnectionHasNoLeftoverDeadline(t *testing.T) {
	res := runSmartRace(context.Background(), []byte("hello"), refusingDialer(), muteServer(t))
	if res.Err != nil || res.Conn == nil {
		t.Fatalf("ожидалось живое соединение: err=%v", res.Err)
	}
	defer res.Conn.Close()
	_ = res.Conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	started := time.Now()
	buf := make([]byte, 16)
	_, _ = res.Conn.Read(buf)
	if elapsed := time.Since(started); elapsed < 200*time.Millisecond {
		t.Fatalf("чтение упало мгновенно (%v) — на соединении остался старый дедлайн", elapsed)
	}
}

// Доказанный ответ всегда лучше молчания: молчащий кандидат не имеет права
// выиграть у пути, по которому сервер заговорил.
func TestAnsweredPathBeatsASilentOne(t *testing.T) {
	res := runSmartRace(context.Background(), []byte("hello"), blackHoleServer(t), liveServer(t, "ok"))
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	defer res.Conn.Close()
	if res.Unproven {
		t.Fatal("победа отдана молчащему пути при живом ответе на другом")
	}
	if !res.ViaProxy {
		t.Fatal("ответил узел, а победа записана direct")
	}
}

// Развилка 2. Потолок с молчащей ногой — улика двусмысленная: direct мог
// отказать из-за цензуры конкретного адреса, а узел промолчать из-за
// медленного сервера. Копить такое в счётчик поломки канала значит взводить
// предохранитель на том, ради чего он не писался.
func TestDeadlineWithAnUnfinishedPathIsNotEvidenceAgainstTheLink(t *testing.T) {
	report, _ := raceLinkEvidence(raceResult{Err: errors.New("EOF"), Unfinished: 1})
	if report {
		t.Fatal("молчание засчитано как доказательство, что сломан канал")
	}
}

// Отданное без ответа соединение ничего не доказывает, а вердикт живёт семь
// дней — записывать по нему нельзя.
func TestHandedOverConnectionTeachesNothing(t *testing.T) {
	if raceTeaches(raceResult{Unproven: true}) {
		t.Fatal("по недоказанному соединению записывается вердикт")
	}
	if !raceTeaches(raceResult{}) {
		t.Fatal("обычная победа перестала учить")
	}
}

// Передавать клиенту молчащий DIRECT нельзя: молчание прямого пути — это и
// есть подпись чёрной дыры, и вкладка, висящая своим таймаутом минуту, хуже
// честного отказа за секунду. Отдаём только узел.
func TestMuteDirectIsNotHandedOver(t *testing.T) {
	res := runSmartRace(context.Background(), []byte("hello"), muteServer(t), refusingDialer())
	if res.Err == nil {
		res.Conn.Close()
		t.Fatal("клиенту отдали молчащий прямой путь вместо отказа")
	}
}
