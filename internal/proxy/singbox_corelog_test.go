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
	"os"
	"path/filepath"
	"strings"
	"testing"

	sblog "github.com/sagernet/sing-box/log"

	"resultproxy-wails/internal/logger"
)

// newTestCoreLog opens a sink on a path the test owns, bypassing
// newCoreLogFile's data-dir lookup.
func newTestCoreLog(t *testing.T) (*coreLogFile, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "core.log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("не удалось открыть файл стока: %v", err)
	}
	c := &coreLogFile{file: file}
	t.Cleanup(c.Close)
	return c, path
}

// The whole reason the sink exists: RESULTV_SINGBOX_LOG_LEVEL raises the level
// the core logs at, but WriteMessage drops everything below Warn, so a raised
// level used to change nothing an investigator could read. Debug and trace are
// where the core answers the questions worth asking — whether the WireGuard
// handshake is being retried, whether the peer endpoint resolves, whether the
// bind is being rebuilt underneath a live session.
func TestCoreLogCapturesWhatTheVisibleLogDrops(t *testing.T) {
	core, path := newTestCoreLog(t)
	log := logger.New()
	w := newSingBoxLogWriter(log, ProxyConfig{}, core)

	w.WriteMessage(sblog.LevelDebug, "peer(abc…) - sending handshake initiation")
	w.WriteMessage(sblog.LevelTrace, "pre-match: forward tcp connection via outbound/wireguard[proxy]")

	if entries := log.GetAll(); len(entries) != 0 {
		t.Fatalf("отладочные строки не должны попадать в видимый лог, попало: %q", entries[0].Msg)
	}

	core.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("файл стока не читается: %v", err)
	}
	body := string(data)
	for _, want := range []string{"sending handshake initiation", "pre-match: forward", "[debug]", "[trace]"} {
		if !strings.Contains(body, want) {
			t.Errorf("в файле нет %q:\n%s", want, body)
		}
	}
}

// Noise the visible log filters out is still evidence in the file: the filters
// exist to keep the user's log readable, not to decide what an investigation
// gets to see.
func TestCoreLogKeepsFilteredNoise(t *testing.T) {
	core, path := newTestCoreLog(t)
	log := logger.New()
	w := newSingBoxLogWriter(log, ProxyConfig{}, core)

	w.WriteMessage(sblog.LevelError, "[89962416 6.16s] inbound/mixed[probe-in]: process connection from 127.0.0.1:45492: wsasend")

	if entries := log.GetAll(); len(entries) != 0 {
		t.Fatalf("шум probe-in должен оставаться отброшенным в видимом логе, попало: %q", entries[0].Msg)
	}
	core.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("файл стока не читается: %v", err)
	}
	if !strings.Contains(string(data), "probe-in") {
		t.Errorf("отфильтрованная строка не сохранилась в файле:\n%s", data)
	}
}

// A private key must never reach the file. sing-box-extended reports a failed
// WireGuard setup by dumping the whole ipcConf into the error text, and this
// file is opened precisely to be sent to someone.
func TestCoreLogRedactsSecrets(t *testing.T) {
	core, path := newTestCoreLog(t)
	w := newSingBoxLogWriter(logger.New(), ProxyConfig{}, core)

	w.WriteMessage(sblog.LevelError, "endpoint/wireguard[proxy]: ipc set: private_key=deadbeefdeadbeef public_key=cafe")

	core.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("файл стока не читается: %v", err)
	}
	body := string(data)
	if strings.Contains(body, "deadbeefdeadbeef") {
		t.Errorf("приватный ключ утёк в файл:\n%s", body)
	}
	// public_key is not a secret and is what makes the dump diagnosable.
	if !strings.Contains(body, "public_key=cafe") {
		t.Errorf("публичный ключ должен остаться:\n%s", body)
	}
}

// A nil sink is the normal case — the level was not raised — and every call
// site relies on it being usable without a branch.
func TestCoreLogNilIsUsable(t *testing.T) {
	var core *coreLogFile
	core.write(sblog.LevelDebug, "не должно паниковать")
	core.writeLine("и это тоже")
	if got := core.Path(); got != "" {
		t.Errorf("Path() у nil-стока должен быть пустым, получено %q", got)
	}
	core.Close()

	if got := newCoreLogFile("error"); got != nil {
		t.Error("уровень error не должен открывать файл")
	}
	if got := newCoreLogFile(""); got != nil {
		t.Error("пустой уровень не должен открывать файл")
	}
}
