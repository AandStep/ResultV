// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"strings"
	"testing"

	sblog "github.com/sagernet/sing-box/log"

	"resultproxy-wails/internal/logger"
)

// TestEngineLogDropsSelfInflictedProbeNoise pins the two ERROR classes that
// flooded the user-visible log while nothing was actually wrong.
//
// probe-in is OUR loopback inbound: only the kill-switch watchdog
// (probeProxyHealth, manager.go) ever dials it, with a 4s client timeout and
// DisableKeepAlives. When a probe overruns that timeout the Go client hangs up
// mid-response and sing-box fails the write back — WSAECONNABORTED, reported as
// "aborted by the software in your host machine". The probe's own verdict is
// already logged by the watchdog as "Проба не прошла" (tagged [KILL SWITCH] or
// [СЕТЬ] depending on the setting, see watchdogLogTag), so the
// engine-side wreckage carries no information the user can act on.
//
// The download/upload-closed RSTs are the peer's doing: Google and friends
// reset idle TCP sessions rather than closing them, which Windows surfaces as
// "forcibly closed by the remote host". The transfer already happened; the
// reset is bookkeeping.
func TestEngineLogDropsSelfInflictedProbeNoise(t *testing.T) {
	noise := []string{
		"[89962416 6.16s] inbound/mixed[probe-in]: process connection from 127.0.0.1:45492: " +
			"write tcp 127.0.0.1:14081->127.0.0.1:45492: wsasend: An established connection was " +
			"aborted by the software in your host machine.",
		"[2453506640 7.3s] inbound/mixed[probe-in]: process connection from 127.0.0.1:45486: " +
			"(http2: response body closed | write tcp 127.0.0.1:14081->127.0.0.1:45486: wsasend: " +
			"An established connection was aborted by the software in your host machine.)",
		"[2759695818 26.92s] connection: connection download closed: raw-read tcp " +
			"172.20.10.2:4341->108.177.14.94:443: An existing connection was forcibly closed by the remote host.",
		"[978093202 29.11s] connection: connection upload closed: raw-read tcp " +
			"172.20.10.2:14178->3.164.230.47:443: An existing connection was forcibly closed by the remote host.",
		"[425729837 23.73s] connection: connection download closed: read tcp " +
			"172.20.10.2:45461->209.85.233.94:443: connection reset by peer",
	}

	for _, msg := range noise {
		log := logger.New()
		w := newSingBoxLogWriter(log, ProxyConfig{})
		w.WriteMessage(sblog.LevelError, msg)
		if entries := log.GetAll(); len(entries) != 0 {
			t.Errorf("шум должен быть отброшен, но записалось %q\nисходное: %s", entries[0].Msg, msg)
		}
	}
}

// TestEngineLogKeepsActionableErrors is the other half of the filter: the noise
// rules must not swallow a real failure. A dial that never completes and a
// handshake rejection say something the user can act on, and both live in the
// same ERROR stream as the noise above.
func TestEngineLogKeepsActionableErrors(t *testing.T) {
	actionable := []string{
		"outbound/vless[proxy]: dial tcp 203.0.113.7:443: i/o timeout",
		"inbound/tun[tun-in]: process packet: permission denied",
		"[123 1.0s] connection: connection download closed: unexpected EOF",
	}

	for _, msg := range actionable {
		log := logger.New()
		w := newSingBoxLogWriter(log, ProxyConfig{})
		w.WriteMessage(sblog.LevelError, msg)
		entries := log.GetAll()
		if len(entries) != 1 {
			t.Errorf("значимая ошибка потерялась: %s", msg)
			continue
		}
		if !strings.Contains(entries[0].Msg, "[SING-BOX] ") {
			t.Errorf("ожидали префикс [SING-BOX], получили %q", entries[0].Msg)
		}
	}
}
