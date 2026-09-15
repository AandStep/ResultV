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
	"strings"
	"testing"
)

// The UAPI dump carries the private key and the preshared key. This file is
// opened to be handed to someone, so the filter is a whitelist and the test
// pins that: a new field in the core's dump must not appear here by default.
func TestWGStatsKeepsOnlyWhitelistedFields(t *testing.T) {
	dump := strings.Join([]string{
		"private_key=deadbeef",
		"listen_port=51820",
		"public_key=cafebabe",
		"preshared_key=secret",
		"endpoint=198.51.100.7:3306",
		"last_handshake_time_sec=1757880000",
		"tx_bytes=12345",
		"rx_bytes=67890",
		"persistent_keepalive_interval=25",
		"protocol_version=1",
		"errno=0",
	}, "\n")

	got := filterWGStats(dump)

	for _, want := range []string{
		"endpoint=198.51.100.7:3306",
		"last_handshake_time_sec=1757880000",
		"tx_bytes=12345",
		"rx_bytes=67890",
		"persistent_keepalive_interval=25",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("нет поля %q в %q", want, got)
		}
	}
	for _, leak := range []string{"deadbeef", "cafebabe", "secret", "private_key", "preshared_key"} {
		if strings.Contains(got, leak) {
			t.Errorf("в строку статистики утекло %q: %s", leak, got)
		}
	}
}

func TestWGStatsHandlesEmptyDump(t *testing.T) {
	if got := filterWGStats(""); got != "(пусто)" {
		t.Errorf("пустой дамп должен читаться как пустой, получено %q", got)
	}
	if got := filterWGStats("errno=0\n"); got != "(пусто)" {
		t.Errorf("дамп без интересных полей должен читаться как пустой, получено %q", got)
	}
}

// Without a core log there is nothing to sample into, and without a box context
// there is nothing to sample — neither may start a goroutine or panic.
func TestWGStatsSamplerNoOpWithoutSink(t *testing.T) {
	startWGStatsSampler(context.Background(), context.Background(), nil)
	startWGStatsSampler(context.Background(), nil, &coreLogFile{})
}

// A node that is not WireGuard has no device to read, and that must be an
// ordinary error rather than a panic on a live session.
func TestWGStatsReportsMissingEndpoint(t *testing.T) {
	if _, err := wgStatsLine(context.Background()); err == nil {
		t.Error("контекст без менеджера эндпоинтов должен давать ошибку")
	}
}
