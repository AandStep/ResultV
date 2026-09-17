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
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"resultproxy-wails/internal/verdict"
)

const (
	smartVerdictStoreFileName = "verdicts.json"
	smartDirectSetFileName    = "verdict-direct.json"
	// smartDirectSetVersion — версия формата rule-set. Ядро принимает 1..5
	// (constant/rule.go); тройка — то, что заведомо понимают и 1.13, и 1.14,
	// а нам от новых версий ничего не нужно: в файле один вид правила.
	smartDirectSetVersion = 3
)

// SmartVerdictStorePath — хешированный стор: все вердикты, включая proxy.
func SmartVerdictStorePath(dataDir string) string {
	return filepath.Join(smartRuleSetDir(dataDir), smartVerdictStoreFileName)
}

// SmartDirectSetPath — плейнтекстовый rule-set: только выученные «ходит
// напрямую». Лежит рядом со Smart-списком, потому что это тот же класс данных
// и та же уборка.
func SmartDirectSetPath(dataDir string) string {
	return filepath.Join(smartRuleSetDir(dataDir), smartDirectSetFileName)
}

// smartDirectSetFile — ровно та форма, которую ядро разбирает как rule-set
// формата source. Ничего лишнего в неё положить нельзя: sing-box отвергает
// незнакомые ключи, поэтому срок жизни имён хранится не здесь, а в сторе.
type smartDirectSetFile struct {
	Version int                  `json:"version"`
	Rules   []smartDirectSetRule `json:"rules"`
}

type smartDirectSetRule struct {
	DomainSuffix []string `json:"domain_suffix"`
}

// EnsureSmartDirectSet создаёт пустой, но валидный файл, если его нет.
//
// Без него ядро не стартует вовсе: NewLocalRuleSet читает путь в конструкторе
// и возвращает ошибку наружу (route/rule/rule_set_local.go). Пустой набор
// правил при этом законен и не матчит ничего.
func EnsureSmartDirectSet(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return RenderSmartDirectSet(path, nil)
}

// RenderSmartDirectSet пишет файл целиком, через временный и rename.
//
// Атомарность здесь не перестраховка: ядро следит за путём через fswatch и
// перечитает файл ровно в тот момент, когда мы его пишем. Половина файла — это
// ошибка разбора в логе и потеря всего выученного до следующего рендера.
func RenderSmartDirectSet(path string, names []string) error {
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)

	file := smartDirectSetFile{Version: smartDirectSetVersion}
	if len(sorted) > 0 {
		file.Rules = []smartDirectSetRule{{DomainSuffix: sorted}}
	} else {
		file.Rules = []smartDirectSetRule{}
	}
	blob, err := json.Marshal(file)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, blob, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadSmartDirectSet достаёт имена из файла. Любая беда — отсутствие, мусор,
// чужая версия — это пустой список, а не ошибка: файл кэш, и начать с нуля
// стоит нескольких проб, тогда как отказ стоил бы всей фичи.
func ReadSmartDirectSet(path string) []string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var file smartDirectSetFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil
	}
	var out []string
	for _, rule := range file.Rules {
		out = append(out, rule.DomainSuffix...)
	}
	return out
}

// directNamesOf — имена, которые ядро может вести напрямую само.
//
// Только вердикт Direct: имя, про которое известно, что оно заблокировано, —
// это то, что человек посещал, и открытым текстом на диск оно не ложится.
// Names() уже отсеивает истёкшее и то, чей плейнтекст этой сессии неизвестен.
func directNamesOf(store *verdict.Store) []string {
	if store == nil {
		return nil
	}
	var out []string
	for name, rec := range store.Names() {
		if rec.Decision == verdict.Direct {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}
