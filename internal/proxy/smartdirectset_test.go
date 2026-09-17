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
	stdjson "encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"

	"resultproxy-wails/internal/verdict"
)

// Ядро читает файл в конструкторе rule-set и на ошибке роняет запуск. Пустой
// скелет — валидный файл с нулём правил, и он обязан появиться до старта.
func TestEnsureSmartDirectSet_CreatesValidEmptySkeleton(t *testing.T) {
	path := filepath.Join(t.TempDir(), "smart", "verdict-direct.json")
	if err := EnsureSmartDirectSet(path); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var parsed struct {
		Version int `json:"version"`
		Rules   []struct {
			DomainSuffix []string `json:"domain_suffix"`
		} `json:"rules"`
	}
	if err := stdjson.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("скелет не разбирается: %v (%s)", err, raw)
	}
	if parsed.Version != 3 {
		t.Errorf("version = %d, ожидалось 3", parsed.Version)
	}
	if len(parsed.Rules) != 0 {
		t.Errorf("в скелете должно быть ноль правил, получено %d", len(parsed.Rules))
	}
}

// Существующий файл не затирается: он пережил перезапуск и в нём имена,
// которые ещё не вернулись в стор.
func TestEnsureSmartDirectSet_KeepsExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "verdict-direct.json")
	if err := RenderSmartDirectSet(path, []string{"example.com"}); err != nil {
		t.Fatalf("render: %v", err)
	}
	if err := EnsureSmartDirectSet(path); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if got := ReadSmartDirectSet(path); len(got) != 1 || got[0] != "example.com" {
		t.Fatalf("файл затёрт, прочитано %v", got)
	}
}

func TestRenderAndReadSmartDirectSet_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "verdict-direct.json")
	if err := RenderSmartDirectSet(path, []string{"b.example", "a.example"}); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := ReadSmartDirectSet(path)
	// Порядок фиксирован сортировкой: иначе один и тот же набор имён писал бы
	// разный файл, и fswatch дёргал бы ядро на пустом месте.
	if len(got) != 2 || got[0] != "a.example" || got[1] != "b.example" {
		t.Fatalf("round-trip дал %v", got)
	}
}

func TestReadSmartDirectSet_MissingOrJunkIsEmpty(t *testing.T) {
	dir := t.TempDir()
	if got := ReadSmartDirectSet(filepath.Join(dir, "нет.json")); len(got) != 0 {
		t.Errorf("отсутствующий файл дал %v", got)
	}
	junk := filepath.Join(dir, "junk.json")
	if err := os.WriteFile(junk, []byte("не json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := ReadSmartDirectSet(junk); len(got) != 0 {
		t.Errorf("мусор дал %v", got)
	}
}

// В файл уходят только direct-вердикты: имена заблокированных сайтов остаются
// в хешированном сторе — это и есть вся приватность, которая здесь возможна.
func TestDirectNamesOf_OnlyDirectVerdicts(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC) }
	s := verdict.New([]byte("salt"), now)
	s.Learn("clean.example", verdict.Direct)
	s.Learn("walled.example", verdict.Proxy)

	got := directNamesOf(s)
	if len(got) != 1 || got[0] != "clean.example" {
		t.Fatalf("directNamesOf = %v, ожидалось только clean.example", got)
	}
}

// Файл существует ради одного читателя — ядра. Пять тестов выше проверяют наш
// формат против нас самих; этот проверяет его против настоящего разборщика
// sing-box. Разойдись формат — те тесты останутся зелёными, а телефон
// перестанет подключаться: NewLocalRuleSet читает файл в конструкторе и роняет
// старт ядра целиком.
func TestSmartDirectSet_CoreParserAcceptsWhatWeWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "verdict-direct.json")
	if err := RenderSmartDirectSet(path, []string{"example.com", "b.example"}); err != nil {
		t.Fatalf("render: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	compat, err := singjson.UnmarshalExtended[option.PlainRuleSetCompat](raw)
	if err != nil {
		t.Fatalf("ядро отвергло файл: %v (%s)", err, raw)
	}
	plain, err := compat.Upgrade()
	if err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	if len(plain.Rules) != 1 {
		t.Fatalf("правил %d, ожидалось 1", len(plain.Rules))
	}
	got := plain.Rules[0].DefaultOptions.DomainSuffix
	if len(got) != 2 || got[0] != "b.example" || got[1] != "example.com" {
		t.Fatalf("ядро прочло domain_suffix = %v, ожидалось [b.example example.com]", got)
	}

	// Пустой скелет ядро тоже обязано принять: он пишется до старта, когда
	// выучить ещё нечего.
	empty := filepath.Join(t.TempDir(), "empty.json")
	if err := EnsureSmartDirectSet(empty); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	eraw, err := os.ReadFile(empty)
	if err != nil {
		t.Fatalf("read empty: %v", err)
	}
	emptyCompat, err := singjson.UnmarshalExtended[option.PlainRuleSetCompat](eraw)
	if err != nil {
		t.Fatalf("ядро отвергло пустой скелет: %v (%s)", err, eraw)
	}
	if _, err := emptyCompat.Upgrade(); err != nil {
		t.Fatalf("Upgrade пустого: %v", err)
	}
}
