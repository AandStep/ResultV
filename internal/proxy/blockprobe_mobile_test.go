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

// Имя файла НЕ оканчивается на _android: Go читает такой суффикс как
// ограничение по GOOS и на машине разработчика исключил бы файл целиком —
// тесты молча не запускались бы ни разу, а прогон показывал бы «ok».

import (
	"context"
	"net"
	"testing"
)

// Прямая половина пробы на Android — обычный дозвон по адресу, который ей
// назвали. Тест держит эту границу: если кто-то вернёт сюда ПК-шный разбор
// фейкового адреса или привязку к интерфейсу, проба начнёт отказывать на
// телефоне молча, а причина будет видна только в пустом probe_error.
func TestProbeDirectDial_DialsGivenAddressAsIs(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	accepted := make(chan struct{}, 1)
	go func() {
		c, aErr := ln.Accept()
		if aErr != nil {
			return
		}
		accepted <- struct{}{}
		c.Close()
	}()

	conn, err := probeDirectDial(context.Background(), "tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("probeDirectDial: %v", err)
	}
	defer conn.Close()
	<-accepted
}

// Половина «через узел» без поднятого инбаунда обязана отказаться, а не
// померить прямой путь второй раз: две одинаковые половины дали бы
// classifyProbe вердикт, под которым нет измерения.
func TestProbeFetch_NoInbound_RefusesNodeHalf(t *testing.T) {
	setProbeInboundPort(0)
	out := probeFetch(context.Background(), "https://example.com/", true)
	if out.Err == nil {
		t.Fatalf("ожидался отказ без инбаунда, получено %+v", out)
	}
}
