# Блок 5, задача 2 — AmneziaWG 3.1, счётчики WG и два переключателя движка

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Android получает AmneziaWG 3.1 целиком — ключи разбираются из всех трёх источников, доезжают до пробы пинга и до живого устройства в туннеле, — плюс счётчики WG-устройства и gVisor-стека в журнале и два переключателя движка (MTU WireGuard, стек TUN) в настройках.

**Architecture:** Разбор, валидация и проба — чистый Go, правится поверх android-поведения (неподдерживаемые ключи сохраняются, а не выбрасываются). Применение к живому устройству отличается от ПК принципиально: на `android` `internal/proxy/singbox.go` никем не вызывается, ядро поднимает `libbox.CommandServer`, которым владеет Kotlin. Поэтому Go-функция принимает `adapter.EndpointManager`, а новый биндинг `mobile.ApplyAWG31(server)` достаёт его из сервера (`server.Instance().Box().Endpoint()`); Kotlin зовёт биндинг сразу после `startOrReloadService`. Узел, к которому применяются ключи, `mobile` запоминает в момент сборки конфига — там единственная воронка для всех путей (подписка, ручной профиль, AUTO-подмена, любой reload). Два переключателя движка на ПК живут переменными окружения; на Android они мертвы (Go копирует environ при загрузке `.so`, а `Os.setenv` из Kotlin случается позже), поэтому едут полями `BuildOptions` и рядами в настройках.

**Tech Stack:** Go 1.26.4, `sing-box-extended v1.14.0-extended-2.7.1`, `wireguard-go v0.0.5-extended-1.6.1`, gomobile (форк sagernet, v0.1.12), Kotlin/Compose в `android/`.

**Spec:** `docs/superpowers/specs/2026-09-17-android-core-1.14-sync-design.md`, раздел 5 (блок 2). Три развилки, которых в спеке не было, решены человеком 2026-09-17: мост через `CommandServer`, счётчики с адаптацией под Android, оба переключателя полями `BuildOptions`. Задача 11 записывает их в спек.

## Global Constraints

- Ветка `android` в `C:\ResultV`. Точка отката — `822adff` (последний коммит перед задачей).
- Теги сборки:
  - `TAGS_FULL="mobile,with_gvisor,with_utls,with_clash_api,with_quic,with_wireguard,with_grpc"` — содержимое `scripts/android-build-tags.txt`, дословно.
  - `TAGS_PLAY="$TAGS_FULL,no_mitm,no_adblock"`.
- **Каждый** `go build` / `go test` гоняется в обеих конфигурациях. Конфигурация, которую не прогнали, считается красной.
- Без тега `mobile` пакеты `mobile` и часть `internal/proxy` не собираются, и LSP будет врать про «undefined». Это шум, а не ошибка.
- AAR **никогда** не пересобирается gradle-ом: только `DIST=full ./scripts/build-android-aar.sh` и `DIST=play ./scripts/build-android-aar.sh`. `gradlew assemble*` переупаковывает уже лежащий `.so`, и правки Go молча не доедут до телефона.
- `scripts/build-android-aar.sh` через пайп отдаёт код 0 при упавшей сборке (известный хвост блока 1). Проверять не код возврата пайплайна, а наличие и время изменения выходного `.aar`.
- **Перед сборкой AAR, которая поедет на телефон, экспортировать ключ подписок.**
  Скрипт сам `.env` не читает и без ключа собирает молча-рабочий, но неполноценный
  бинарь: `resultv://` и RVSUB1-подписки в нём не расшифровываются, то есть реальные
  профили человека не импортируются. Проверяется строкой в логе сборки:
  `🔐 Embedding subscription decryption key` — а не `⚠️ SUBSCRIPTION_ENCRYPT_KEY not set`.
  ```bash
  set -a; . /c/ResultV/.env; set +a
  ```
- **Копированием файлов с ПК нельзя.** В `uriparser.go`, `config_validation.go`, `endpoints.go` живёт решение блока 1: неподдерживаемые AWG-ключи (`j1`-`j3`, `itime`) сохраняются в `extra.amnezia` и объявляются через `UnsupportedAWGKnobs`, а версия с `dev` их выбрасывает. Переносятся только изменения 3.1, поверх android-поведения.
- Ключи 3.1 **не попадают в JSON-конфиг ядра** ни при каких условиях: ядро декодирует с `DisallowUnknownFields`, и неизвестное поле под `amnezia` роняет старт целиком, а не игнорируется.
- Ничего из этой задачи не гасится флейворами: ни ad-block, ни MITM здесь не участвуют, код одинаков в `full` и `play`.
- Никаких попутных улучшений: каждая изменённая строка должна прослеживаться до задачи плана.
- Сообщения коммитов на русском, в формате ветки: `тип(область): описание`, тело — почему, а не что. Последняя строка:
  `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>`
- Ни один коммит не пушится в `origin` до приёмки задачи 11.

## Карта файлов

Создаются:

| Файл | За что отвечает |
|---|---|
| `internal/proxy/awg31.go` | модель двух выключателей 3.1, разбор из `extra`, рендер UAPI-строк, путь до живого устройства через неэкспортированные поля ядра |
| `internal/proxy/awg31_test.go` | разбор всех написаний значения + пломба на путь до устройства |
| `internal/proxy/uriparser_awg31_test.go` | ключи доезжают из ссылки и из подписки и не попадают в конфиг ядра |
| `internal/proxy/wgdiag.go` | счётчики WG-устройства (белый список полей UAPI) и gVisor-стека внутри эндпоинта |
| `internal/proxy/wgdiag_test.go` | белый список не пропускает ключи; пустой дамп; отсутствующий эндпоинт — ошибка, а не паника |
| `internal/proxy/endpoints_mtu_test.go` | границы переопределения MTU |
| `internal/proxy/engine_tunstack_test.go` | выбор стека TUN и его дефолт |
| `mobile/libbox_awg31.go` | биндинги `ApplyAWG31` / `WGDiagLine` и память о последнем собранном узле |
| `mobile/libbox_awg31_test.go` | память о собранном узле обновляется на каждой сборке конфига |
| `android/app/src/main/java/com/resultv/android/vpn/WgDiagSampler.kt` | опрос счётчиков раз в 5 с, пока включён подробный журнал |

Изменяются:

| Файл | Что в нём меняется |
|---|---|
| `internal/proxy/uriparser.go` | два места: `parseJSONWireGuardOutbound` и `parseAmneziaWGURI` — ключи 3.1 кладутся в `extra.amnezia` |
| `internal/proxy/config_validation.go` | пересечение H1-H4 с учётом дефолтов + нечитаемое значение выключателя 3.1 |
| `internal/proxy/config_validation_awg3_test.go` | пять новых тестов на обе проверки |
| `internal/proxy/ping_wg_handshake.go` | проба шлёт 3.1 в свой UAPI |
| `internal/proxy/ping_wg_awg3_uapi_test.go` | проба шлёт/не шлёт 3.1 |
| `internal/proxy/endpoints.go` | `wireguardEndpointTag`, переопределение MTU |
| `internal/proxy/engine.go` | `effectiveTunStack`, поля `WGMTU` / `TunStack` в `EngineConfig` |
| `mobile/libbox.go` | поля `wgMtu` / `tunStack` в `BuildOptions`, запоминание узла в `buildSingBoxConfigFromEntry` |
| `android/.../vpn/BoxModule.kt` | вызов `applyAWG31` после старта и после reload; запуск/остановка сэмплера |
| `android/.../vpn/WireGuardConfParser.kt` | `.conf` с ключами 3.1 не теряет их при импорте |
| `android/.../vpn/SettingsRepository.kt` | три новых поля состояния и их сеттеры |
| `android/.../vpn/BuildOptions.kt` | два новых поля в `optionsJson` |
| `android/.../ui/screens/SettingsScreen.kt` | блок «Диагностика» в шторке «Сеть» |
| `android/app/src/main/res/values{,-ru}/strings.xml` | строки журнала и настроек |
| `docs/superpowers/specs/2026-09-17-android-core-1.14-sync-design.md` | запись о закрытии блока 2 |
| `docs/android-pc-sync-and-play-spec.md` | строка статуса блока 5 |

---

### Task 1: Зелёная база

Прежде чем что-то менять, надо знать, что было зелёным. Иначе первый же красный тест не отличить от красного до начала работы.

**Files:**
- Изменяется: ничего, если база зелёная.

**Interfaces:**
- Consumes: —
- Produces: зафиксированный список падений «до», на который ссылаются все следующие задачи.

**Замер 2026-09-17 (до начала работы):** Go — 9 пакетов `ok`, ноль падений в обеих
конфигурациях. Kotlin — full 135 тестов, play 133, ноль падений и ошибок.
(Цифры 84/82 из записи блока 1 считались иначе; здесь — сумма по
`app/build/test-results/test*DebugUnitTest/*.xml`.) Красного до начала работы нет,
поэтому любое падение дальше — регрессия этой задачи.

- [ ] **Step 1: Зафиксировать теги в переменных оболочки**

```bash
cd /c/ResultV
TAGS_FULL="$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)"
TAGS_PLAY="${TAGS_FULL},no_mitm,no_adblock"
echo "full: ${TAGS_FULL}"
echo "play: ${TAGS_PLAY}"
```

Ожидается: `full: mobile,with_gvisor,with_utls,with_clash_api,with_quic,with_wireguard,with_grpc`

- [ ] **Step 2: Собрать обе конфигурации**

```bash
go build -tags="${TAGS_FULL}" ./... && echo "FULL BUILD OK"
go build -tags="${TAGS_PLAY}" ./... && echo "PLAY BUILD OK"
```

Ожидается: обе строки OK.

- [ ] **Step 3: Прогнать Go-тесты в обеих конфигурациях**

```bash
go test -tags="${TAGS_FULL}" ./... 2>&1 | tail -30
go test -tags="${TAGS_PLAY}" ./... 2>&1 | tail -30
```

Ожидается: `ok` по всем пакетам. Если что-то падает — **не чинить наугад**: записать точный список сюда, в тело задачи 1, и дальше сверяться с ним.

- [ ] **Step 4: Прогнать Kotlin-тесты обеих сборок**

```bash
cd /c/ResultV/android && ./gradlew :app:testFullDebugUnitTest :app:testPlayDebugUnitTest 2>&1 | tail -15
```

Ожидается: BUILD SUCCESSFUL. Записать число тестов (на конец блока 1 было full 84 / play 82) — на него ссылается приёмка.

- [ ] **Step 5: Коммита нет**

Задача ничего не меняет. Если Step 3 или 4 потребовали правки — коммитить её отдельно и написать в теле коммита, что это было красным до начала работы.

---

### Task 2: Ключи 3.1 разбираются из ссылки и подписки и валидируются

Два выключателя AmneziaWG 3.1 — `random_trailers` (случайный хвост у рукопожатия) и `disable_cookies` (не отвечать cookie-ответами). Ядро о них не знает вовсе: в `option.WireGuardAmnezia` таких полей нет ни в 1.13, ни в 1.14 (проверено в `option/wireguard.go:37-61` закреплённой версии), а форк wireguard-go принимает их в UAPI (`device/uapi.go:539,547`). В этой задаче они только разбираются и валидируются; до пробы и до устройства их донесут задачи 3-5.

Почему нечитаемое значение — ошибка, а не «выключено»: `random_trailers` симметричен. Приёмная сторона принимает пакет большего размера только когда выключатель включён, так что узел с хвостами клиенту без флага не поднимется никогда — рукопожатия отбрасываются по размеру, и в логе об этом ни строки.

**Files:**
- Создаётся: `internal/proxy/awg31.go` (в этой задаче — только модель и разбор; путь до устройства добавит задача 5)
- Создаётся: `internal/proxy/awg31_test.go`
- Создаётся: `internal/proxy/uriparser_awg31_test.go`
- Изменяется: `internal/proxy/uriparser.go` (после цикла `awg3Keys` в `parseJSONWireGuardOutbound`, ~строка 836; и после такого же цикла в `parseAmneziaWGURI`, ~строка 1678)
- Изменяется: `internal/proxy/config_validation.go` (в `validateAmneziaOptions`, после цикла проверки переносов строк, ~строка 86)
- Изменяется: `internal/proxy/config_validation_awg3_test.go` (дописать в конец)

**Interfaces:**
- Consumes: `normalizeAWGKey(string) string`, `amneziaMapFromExtra(map[string]interface{}) map[string]interface{}`, `parseExtra(ProxyConfig) map[string]interface{}`, `amneziaHeaderString(interface{}) string`, `validateAWGRange(string) error`, `asString(interface{}) string` — всё уже есть в `internal/proxy`.
- Produces: `awg31Keys []string`; тип `awg31Knobs{RandomTrailers, DisableCookies *bool}` с методами `empty() bool`, `ipcLines() string`, `describe() string`; `awg31FromExtra(map[string]interface{}) awg31Knobs`; `awgBoolFromAny(interface{}) *bool`; `awg31KnobsFor(ProxyConfig) awg31Knobs`; тип `awg31Error` с методом `Error() string`.

- [ ] **Step 1: Написать падающие тесты модели**

Создать `internal/proxy/awg31_test.go`:

```go
// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"encoding/json"
	"strings"
	"testing"
)

// AmneziaWG writes these as on/off in its config files, JSON subscriptions send
// booleans, and some providers send 1/0. All three mean the same switch.
func TestAWG31ParsesEverySpelling(t *testing.T) {
	cases := []struct {
		raw  string
		want awg31Knobs
	}{
		{`{"random_trailers":"on","disable_cookies":"off"}`, awg31Knobs{RandomTrailers: awgBoolPtr(true), DisableCookies: awgBoolPtr(false)}},
		{`{"RandomTrailers":true,"DisableCookies":true}`, awg31Knobs{RandomTrailers: awgBoolPtr(true), DisableCookies: awgBoolPtr(true)}},
		{`{"random-trailers":1}`, awg31Knobs{RandomTrailers: awgBoolPtr(true)}},
		{`{"random_trailers":"enabled"}`, awg31Knobs{RandomTrailers: awgBoolPtr(true)}},
		{`{"jc":4,"s1":16}`, awg31Knobs{}},
	}
	for _, tc := range cases {
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(tc.raw), &m); err != nil {
			t.Fatalf("%s: %v", tc.raw, err)
		}
		got := awg31FromExtra(m)
		if !sameAWGKnob(got.RandomTrailers, tc.want.RandomTrailers) ||
			!sameAWGKnob(got.DisableCookies, tc.want.DisableCookies) {
			t.Errorf("%s: получено {rt:%s cookies:%s}, ожидалось {rt:%s cookies:%s}",
				tc.raw, awgKnobString(got.RandomTrailers), awgKnobString(got.DisableCookies),
				awgKnobString(tc.want.RandomTrailers), awgKnobString(tc.want.DisableCookies))
		}
	}
}

// An unreadable value must stay unstated. Turning it into "off" would quietly
// pick the other protocol: with random trailers mismatched, every handshake the
// peer sends is dropped for being the wrong size.
func TestAWG31IgnoresUnreadableValues(t *testing.T) {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(`{"random_trailers":"maybe","disable_cookies":"да"}`), &m); err != nil {
		t.Fatal(err)
	}
	if got := awg31FromExtra(m); !got.empty() {
		t.Errorf("нечитаемые значения не должны становиться выключателями: %+v", got)
	}
}

// The UAPI parses with strconv.ParseBool, which does not know on/off — so what
// reaches the device must be true/false, one key per line.
func TestAWG31IpcLinesAreUAPIShaped(t *testing.T) {
	knobs := awg31Knobs{RandomTrailers: awgBoolPtr(true), DisableCookies: awgBoolPtr(false)}
	got := knobs.ipcLines()
	for _, want := range []string{"random_trailers=true\n", "disable_cookies=false\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("нет строки %q в %q", want, got)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		if strings.Count(line, "=") != 1 {
			t.Errorf("строка UAPI должна быть ровно key=value, получено %q", line)
		}
	}
}

// Only WireGuard-shaped nodes carry these switches; reading them off a VLESS
// node would apply them to an endpoint that does not exist.
func TestAWG31OnlyForWireGuardNodes(t *testing.T) {
	extra := json.RawMessage(`{"amnezia":{"random_trailers":"on"}}`)
	if got := awg31KnobsFor(ProxyConfig{Type: "vless", Extra: extra}); !got.empty() {
		t.Errorf("VLESS-узел не должен отдавать параметры AWG: %+v", got)
	}
	got := awg31KnobsFor(ProxyConfig{Type: "amneziawg", Extra: extra})
	if got.RandomTrailers == nil || !*got.RandomTrailers {
		t.Errorf("AWG-узел должен отдать random_trailers=on, получено %+v", got)
	}
}

// The description goes into a log that exists in two languages, so it is
// written in the UAPI spelling: a knob name is the same word in both.
func TestAWG31DescribeUsesUAPINames(t *testing.T) {
	knobs := awg31Knobs{RandomTrailers: awgBoolPtr(true), DisableCookies: awgBoolPtr(false)}
	if got := knobs.describe(); got != "random_trailers=on, disable_cookies=off" {
		t.Errorf("описание = %q", got)
	}
}

func awgBoolPtr(v bool) *bool { return &v }

func sameAWGKnob(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func awgKnobString(v *bool) string {
	if v == nil {
		return "—"
	}
	if *v {
		return "on"
	}
	return "off"
}
```

- [ ] **Step 2: Убедиться, что тесты падают**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run TestAWG31 2>&1 | tail -20
```

Ожидается: `undefined: awg31FromExtra` и соседние — пакет не компилируется.

- [ ] **Step 3: Написать модель**

Создать `internal/proxy/awg31.go`:

```go
// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"strconv"
	"strings"
)

// AmneziaWG 3.1 adds two device-wide switches on top of 3.0:
//
//   - random_trailers — appends a random-length tail of random bytes to the
//     handshake initiation, the response and the cookie reply. It is symmetric:
//     the receiver only accepts an oversized packet when the switch is on, so a
//     peer that sends tails to a client without it has its handshakes dropped
//     for being the wrong size (wireguard-go device/receive.go). "Same value on
//     both sides" is not advice, it is the protocol.
//   - disable_cookies — stops the device from answering with cookie replies,
//     the message a loaded server uses to make an initiator prove its address.
//     Under probing, silence is a weaker fingerprint than a reply.
//
// The engine's WireGuard device (wireguard-go fork) implements both in its UAPI
// (device/uapi.go). sing-box-extended does not: option.WireGuardAmnezia has no
// field for either, and the ipcConf it builds in transport/wireguard stops at
// the 3.0 knobs — so through the config file these two are unreachable, in 1.13
// and 1.14 alike. They reach the tunnel by a second IpcSet on the running
// device (ApplyAWG31) and the ping probe by its own UAPI string
// (writeAmneziaUAPI).
//
// Deliberately NOT emitted into the sing-box JSON: the core parses its config
// strictly, and an unknown key under "amnezia" fails the whole start rather
// than being ignored.
const (
	awg31RandomTrailersKey = "random_trailers"
	awg31DisableCookiesKey = "disable_cookies"
)

// awg31Keys is the pair in the order the URI parsers carry them across.
var awg31Keys = []string{awg31RandomTrailersKey, awg31DisableCookiesKey}

// awg31Knobs carries the two switches. Nil means "not stated by the config",
// which is not the same as false: a stated "off" is worth sending, because the
// device default can move and a config that says off must keep meaning off.
type awg31Knobs struct {
	RandomTrailers *bool
	DisableCookies *bool
}

func (k awg31Knobs) empty() bool {
	return k.RandomTrailers == nil && k.DisableCookies == nil
}

// ipcLines renders the knobs as UAPI lines. The device's IpcSet takes a partial
// configuration: it seeds itself from the live device and merges, and peers are
// only touched by a "public_key" line — which is why sending just these two is
// safe on a running session (wireguard-go device/uapi.go, IpcSetOperation).
func (k awg31Knobs) ipcLines() string {
	var b strings.Builder
	if k.RandomTrailers != nil {
		b.WriteString(awg31RandomTrailersKey + "=" + strconv.FormatBool(*k.RandomTrailers) + "\n")
	}
	if k.DisableCookies != nil {
		b.WriteString(awg31DisableCookiesKey + "=" + strconv.FormatBool(*k.DisableCookies) + "\n")
	}
	return b.String()
}

// describe renders the applied switches for the log. The UAPI spelling, not a
// translated phrase: this string is handed to Kotlin and printed into a log
// that exists in two languages, where a knob name is the same word in both.
func (k awg31Knobs) describe() string {
	var parts []string
	if k.RandomTrailers != nil {
		parts = append(parts, awg31RandomTrailersKey+"="+awgOnOff(*k.RandomTrailers))
	}
	if k.DisableCookies != nil {
		parts = append(parts, awg31DisableCookiesKey+"="+awgOnOff(*k.DisableCookies))
	}
	return strings.Join(parts, ", ")
}

func awgOnOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

// awg31FromExtra reads the switches out of a node's amnezia block.
//
// Spellings are folded the same way the 3.0 knobs are (normalizeAWGKey), so
// "RandomTrailers" from a .conf-style provider and "random_trailers" from a
// JSON subscription are one key. Values follow the AmneziaWG config file, which
// writes on/off — a spelling Go's ParseBool rejects, and the UAPI is ParseBool.
func awg31FromExtra(m map[string]interface{}) awg31Knobs {
	var knobs awg31Knobs
	if len(m) == 0 {
		return knobs
	}
	for rawKey, rawVal := range m {
		switch normalizeAWGKey(rawKey) {
		case normalizeAWGKey(awg31RandomTrailersKey):
			knobs.RandomTrailers = awgBoolFromAny(rawVal)
		case normalizeAWGKey(awg31DisableCookiesKey):
			knobs.DisableCookies = awgBoolFromAny(rawVal)
		}
	}
	return knobs
}

// awgBoolFromAny accepts every spelling seen in the wild for these switches,
// and returns nil for anything it cannot read — an unreadable value must not
// silently become "off", which is a different protocol.
//
// int64 is in the list for a local reason: parseAmneziaWGURI stores numbers
// from a link with strconv.ParseInt, so a value that arrived through a query
// parameter is int64 where the same value from JSON is float64.
func awgBoolFromAny(v interface{}) *bool {
	switch t := v.(type) {
	case nil:
		return nil
	case bool:
		return &t
	case float64:
		b := t != 0
		return &b
	case int:
		b := t != 0
		return &b
	case int64:
		b := t != 0
		return &b
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "on", "true", "1", "yes", "enabled", "enable":
			b := true
			return &b
		case "off", "false", "0", "no", "disabled", "disable":
			b := false
			return &b
		}
	}
	return nil
}

// awg31KnobsFor extracts the switches for a node, or an empty set when the node
// is not WireGuard-shaped or says nothing about them.
func awg31KnobsFor(proxy ProxyConfig) awg31Knobs {
	pt := strings.ToUpper(strings.TrimSpace(proxy.Type))
	if pt != "WIREGUARD" && pt != "AMNEZIAWG" {
		return awg31Knobs{}
	}
	return awg31FromExtra(amneziaMapFromExtra(parseExtra(proxy)))
}

// awg31Error is the error type for every step on the way to the device. It is
// declared here because ApplyAWG31 and the counters in wgdiag.go both raise it.
type awg31Error struct{ what string }

func (e *awg31Error) Error() string { return e.what }
```

- [ ] **Step 4: Тесты модели проходят**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run TestAWG31 -v 2>&1 | tail -20
```

Ожидается: пять `--- PASS`.

- [ ] **Step 5: Написать падающие тесты разбора ссылок**

Создать `internal/proxy/uriparser_awg31_test.go`:

```go
// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

// The 3.1 switches have to survive the whole way: link → extra → knobs. Losing
// them anywhere in between is silent, and with random_trailers mismatched the
// tunnel never comes up at all.
func TestAWG31SurvivesURIRoundTrip(t *testing.T) {
	q := url.Values{}
	q.Set("private_key", "aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=")
	q.Set("public_key", "WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=")
	q.Set("address", "10.0.0.2/32")
	q.Set("allowed_ips", "0.0.0.0/0")
	q.Set("RandomTrailers", "on")
	q.Set("DisableCookies", "off")

	entry, err := ParseProxyURI("awg://1.2.3.4:51820?" + q.Encode() + "#awg31")
	if err != nil {
		t.Fatal(err)
	}
	knobs := awg31KnobsFor(ProxyConfig{Type: entry.Type, Extra: entry.Extra})
	if knobs.RandomTrailers == nil || !*knobs.RandomTrailers {
		t.Errorf("random_trailers потерялся по дороге: %s", entry.Extra)
	}
	if knobs.DisableCookies == nil || *knobs.DisableCookies {
		t.Errorf("disable_cookies потерялся или перевернулся: %s", entry.Extra)
	}
}

// Ту же дорогу проходит запись из JSON-подписки, где amnezia приезжает
// объектом, а не параметрами запроса. Форма — sing-box-outbound в корне,
// какую принимает parseJSONWireGuardOutbound (проверено на закреплённом
// парсере: сегодня из такого блока доезжают jc и s1, а ключи 3.1 теряются).
func TestAWG31SurvivesJSONOutbound(t *testing.T) {
	const j = `{
	  "type": "amneziawg",
	  "tag": "awg-node",
	  "server": "1.2.3.4",
	  "server_port": 51820,
	  "private_key": "aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=",
	  "public_key": "WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=",
	  "address": ["10.0.0.2/32"],
	  "allowed_ips": ["0.0.0.0/0"],
	  "amnezia": {"jc": 4, "s1": 15, "random_trailers": "on", "DisableCookies": "off"}
	}`
	entries, err := ParseSubscriptionBody(j)
	if err != nil {
		t.Fatalf("подписка не разобралась: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ожидалась одна запись, получено %d", len(entries))
	}
	knobs := awg31KnobsFor(ProxyConfig{Type: entries[0].Type, Extra: entries[0].Extra})
	if knobs.RandomTrailers == nil || !*knobs.RandomTrailers {
		t.Errorf("random_trailers не доехал в extra: %s", entries[0].Extra)
	}
	if knobs.DisableCookies == nil || *knobs.DisableCookies {
		t.Errorf("disable_cookies не доехал или перевернулся: %s", entries[0].Extra)
	}
}

// AWG 3.1 knobs must not leak into the sing-box config: the core parses it
// strictly and an unknown key under "amnezia" fails the entire start. They
// travel to the device by a second IpcSet instead (see ApplyAWG31).
func TestAWG31NeverReachesEngineConfig(t *testing.T) {
	q := url.Values{}
	q.Set("private_key", "aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=")
	q.Set("public_key", "WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=")
	q.Set("address", "10.0.0.2/32")
	q.Set("allowed_ips", "0.0.0.0/0")
	q.Set("RandomTrailers", "on")
	q.Set("DisableCookies", "on")

	entry, err := ParseProxyURI("awg://1.2.3.4:51820?" + q.Encode() + "#awg31")
	if err != nil {
		t.Fatal(err)
	}
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy: ProxyConfig{IP: entry.IP, Port: entry.Port, Type: entry.Type, Extra: entry.Extra},
		Mode:  ProxyModeTunnel,
	})
	rendered, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range awg31Keys {
		if strings.Contains(string(rendered), key) {
			t.Errorf("%s попал в конфиг ядра — ядро отвергнет неизвестный ключ целиком:\n%s", key, rendered)
		}
	}
	// Не «ключей нет», а «ядро принимает»: пустая проверка прошла бы и на
	// сломанном конфиге.
	assertCoreAcceptsConfig(t, cfg)
}
```

- [ ] **Step 6: Убедиться, что оба «survives» падают, а «never reaches» проходит**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run 'TestAWG31Survives' -v 2>&1 | tail -20
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run 'TestAWG31NeverReaches' -v 2>&1 | tail -5
```

Ожидается: два FAIL («потерялся по дороге») и один PASS. Третий проходит и до правки — он пломба, его дело остаться зелёным после Step 7.

- [ ] **Step 7: Донести ключи до extra**

В `internal/proxy/uriparser.go`, в `parseJSONWireGuardOutbound`, сразу после цикла `for _, k := range awg3Keys { … }` (он заканчивается перед `if len(amOut) > 0 {`):

```go
			// AmneziaWG 3.1 switches. Carried as the raw string so one reader
			// (awgBoolFromAny) decides what on/off/true/1 mean, instead of each
			// parser inventing its own.
			for _, k := range awg31Keys {
				for rawKey, rawVal := range am {
					if normalizeAWGKey(rawKey) != normalizeAWGKey(k) {
						continue
					}
					if v := asString(rawVal); v != "" {
						amOut[k] = v
					}
					break
				}
			}
```

В том же файле, в `parseAmneziaWGURI`, сразу после такого же цикла по `awg3Keys` (перед `if len(amnezia) > 0 {`):

```go
	// AmneziaWG 3.1 switches — тем же сопоставлением по нормализованному ключу:
	// провайдеры пишут их и RandomTrailers, и random_trailers.
	for _, k := range awg31Keys {
		for rawKey, vals := range params {
			if len(vals) == 0 || normalizeAWGKey(rawKey) != normalizeAWGKey(k) {
				continue
			}
			if v := strings.TrimSpace(vals[0]); v != "" {
				amnezia[k] = v
			}
			break
		}
	}
```

- [ ] **Step 8: Тесты разбора проходят**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run 'TestAWG31' -v 2>&1 | tail -25
```

Ожидается: все восемь PASS, включая `TestAWG31NeverReachesEngineConfig`.

- [ ] **Step 9: Написать падающие тесты валидации**

Дописать в конец `internal/proxy/config_validation_awg3_test.go`:

```go

// The engine refuses overlapping packet-type headers inside IpcSet, and that
// refusal costs the whole connect and arrives with the entire ipcConf attached.
// Catching it here names the pair instead.
func TestAWGOverlappingHeadersRejected(t *testing.T) {
	err := validateAmneziaOptions(awgExtra(map[string]interface{}{"h1": "5", "h2": "3-7"}))
	if err == nil {
		t.Fatal("пересекающиеся H1/H2 должны быть отклонены")
	}
	if !strings.Contains(err.Error(), "overlap") {
		t.Errorf("сообщение должно называть пересечение, получено: %v", err)
	}
}

// Unset slots keep their protocol defaults (1,2,3,4), so a config that sets
// only H1 = 3 collides with the H3 the engine will use.
func TestAWGHeaderCollidesWithDefault(t *testing.T) {
	if err := validateAmneziaOptions(awgExtra(map[string]interface{}{"h1": "3"})); err == nil {
		t.Error("H1=3 сталкивается с дефолтным H3=3 — ядро это отвергнет")
	}
}

// Defaults on their own must stay valid: an amnezia block without H-values is
// the common case.
func TestAWGDefaultHeadersAccepted(t *testing.T) {
	if err := validateAmneziaOptions(awgExtra(map[string]interface{}{"jc": 4})); err != nil {
		t.Errorf("конфиг без H-значений должен проходить, получено: %v", err)
	}
}

// A 3.1 switch that cannot be read must be refused rather than dropped: with
// random_trailers mismatched the tunnel never comes up and the log says
// nothing.
func TestAWG31UnreadableSwitchRejected(t *testing.T) {
	err := validateAmneziaOptions(awgExtra(map[string]interface{}{"random_trailers": "maybe"}))
	if err == nil {
		t.Fatal("нечитаемое значение random_trailers должно быть отклонено")
	}
	if !strings.Contains(err.Error(), "random_trailers") {
		t.Errorf("сообщение должно называть параметр, получено: %v", err)
	}
}

func TestAWG31ReadableSwitchesAccepted(t *testing.T) {
	for _, block := range []map[string]interface{}{
		{"random_trailers": "on", "disable_cookies": "off"},
		{"RandomTrailers": true},
	} {
		if err := validateAmneziaOptions(awgExtra(block)); err != nil {
			t.Errorf("%v должен проходить валидацию, получено: %v", block, err)
		}
	}
}
```

- [ ] **Step 10: Убедиться, что падают нужные три**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run 'TestAWG(Overlapping|HeaderCollides|DefaultHeaders)|TestAWG31(Unreadable|Readable)' -v 2>&1 | tail -20
```

Ожидается: FAIL у `TestAWGOverlappingHeadersRejected`, `TestAWGHeaderCollidesWithDefault`, `TestAWG31UnreadableSwitchRejected`; PASS у двух остальных — они пломбы на «не сломать разрешённое».

- [ ] **Step 11: Добавить обе проверки**

В `internal/proxy/config_validation.go`, в `validateAmneziaOptions`, сразу после цикла, проверяющего переносы строк (`for _, slot := range slots { … }`):

```go
	if err := validateAWGHeaderRanges(m); err != nil {
		return err
	}
	if err := validateAWG31Switches(m); err != nil {
		return err
	}
```

И в конец файла:

```go
// awgDefaultHeaders are the packet-type values WireGuard uses when H1-H4 say
// nothing: initiation 1, response 2, cookie 3, transport 4.
var awgDefaultHeaders = [4]uint64{1, 2, 3, 4}

// validateAWGHeaderRanges rejects H1-H4 that overlap.
//
// The engine checks this itself, inside IpcSet (wireguard-go device/uapi.go,
// mergeWithDevice → "headers must not overlap"), and a failure there aborts the
// whole connect with the entire ipcConf as its message. The same verdict here
// names the two headers that collide.
//
// Unset slots are checked at their defaults rather than skipped: a config that
// sets only H1 = 3 collides with the default H3, and the engine would see the
// collision even though the config never mentions H3.
func validateAWGHeaderRanges(m map[string]interface{}) error {
	type headerRange struct {
		name      string
		low, high uint64
	}
	ranges := make([]headerRange, 0, 4)
	for i, name := range []string{"h1", "h2", "h3", "h4"} {
		raw := amneziaHeaderString(m[name])
		if raw == "" {
			ranges = append(ranges, headerRange{name: name, low: awgDefaultHeaders[i], high: awgDefaultHeaders[i]})
			continue
		}
		if err := validateAWGRange(raw); err != nil {
			return fmt.Errorf("amneziawg %s: %w", name, err)
		}
		lowRaw, highRaw, isRange := strings.Cut(raw, "-")
		low, err := strconv.ParseUint(strings.TrimSpace(lowRaw), 10, 32)
		if err != nil {
			return fmt.Errorf("amneziawg %s: invalid value %q", name, raw)
		}
		high := low
		if isRange {
			high, err = strconv.ParseUint(strings.TrimSpace(highRaw), 10, 32)
			if err != nil {
				return fmt.Errorf("amneziawg %s: invalid value %q", name, raw)
			}
		}
		ranges = append(ranges, headerRange{name: name, low: low, high: high})
	}
	for i := 0; i < len(ranges); i++ {
		for j := i + 1; j < len(ranges); j++ {
			if ranges[i].low <= ranges[j].high && ranges[j].low <= ranges[i].high {
				return fmt.Errorf("amneziawg %s and %s overlap: the engine rejects overlapping packet-type headers",
					ranges[i].name, ranges[j].name)
			}
		}
	}
	return nil
}

// validateAWG31Switches rejects a stated-but-unreadable random_trailers or
// disable_cookies.
//
// Silence would be worse than a refusal here. random_trailers has to match the
// peer — with it off against a peer that has it on, every handshake the peer
// sends is dropped for being the wrong size, and the session simply never comes
// up, with nothing in the log to say why. A config that means to set it must
// either be understood or rejected out loud.
func validateAWG31Switches(m map[string]interface{}) error {
	for _, name := range awg31Keys {
		for rawKey, rawVal := range m {
			if normalizeAWGKey(rawKey) != normalizeAWGKey(name) {
				continue
			}
			if rawVal == nil {
				continue
			}
			if awgBoolFromAny(rawVal) == nil {
				return fmt.Errorf("amneziawg %s: unreadable value %v, expected on/off", name, rawVal)
			}
		}
	}
	return nil
}
```

- [ ] **Step 12: Прогнать весь пакет в обеих конфигурациях**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ 2>&1 | tail -5
go test -tags="${TAGS_PLAY}" ./internal/proxy/ 2>&1 | tail -5
```

Ожидается: `ok` обе. Особое внимание к `awg3_e2e_test.go`, `awg2_e2e_test.go` и `awg_realworld_test.go`: если какой-то из них задавал пересекающиеся H-значения, он теперь падает — это находка, а не регрессия. Поправить данные теста и записать факт в теле задачи.

**Что нашлось на исполнении (2026-09-17).** Упал ровно один тест —
`TestAmneziaValidationIgnoresNonAmneziaConfigs`: его «AWG 2.0 профиль» задавал
`h1: "1-5"`, то есть диапазон, накрывающий дефолтные H2=2, H3=3, H4=4. Прогон
через настоящее устройство форка (`newProbeDevice` + `IpcSet`) подтвердил, что
это не придирка проверки: `h1=1-5` → `IPC error -22: failed to merge with
device: headers must not overlap`, `h1=3` → та же ошибка, `h1=5` принимается.
То есть тест утверждал приёмку конфига, который движок не поднимет. Данные
заменены на `h1: "10-20"` с комментарием и ссылкой на замер; смысл теста («AWG
2.0 профиль не должен получить новых способов упасть») сохранён.

- [ ] **Step 13: Коммит**

```bash
cd /c/ResultV
git add internal/proxy/awg31.go internal/proxy/awg31_test.go \
        internal/proxy/uriparser_awg31_test.go internal/proxy/uriparser.go \
        internal/proxy/config_validation.go internal/proxy/config_validation_awg3_test.go
git commit -F- <<'MSG'
feat(proxy): AmneziaWG 3.1 — ключи разбираются и проверяются

random_trailers симметричен: приёмная сторона принимает пакет большего
размера только когда выключатель включён, так что узел с хвостами клиенту
без флага не поднимется никогда — рукопожатия отбрасываются по размеру, и в
логе об этом ни строки. Поэтому нечитаемое значение отклоняется, а не
превращается в «выключено»: молчание тут стоит дороже отказа.

В конфиг ядра ключи не попадают намеренно — неизвестное поле под amnezia
роняет старт целиком, а не игнорируется. До устройства их донесёт отдельный
IpcSet, до пробы пинга — её собственная UAPI-строка.

Заодно закрыта проверка, которую ядро делает само и на которой валится весь
connect с ipcConf в тексте ошибки: пересечение H1-H4 с учётом дефолтов 1-4
для незаданных.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
MSG
```

---

### Task 3: Импорт `.conf` не теряет ключи 3.1

Конфиги AmneziaWG пользователи чаще всего вставляют файлом `.conf`, а не ссылкой. Разбирает его Kotlin (`WireGuardConfParser`), собирая из него `awg://`-ссылку, — это android-only путь, на ПК такого парсера нет вовсе. Без этой задачи `.conf` с `RandomTrailers = on` даёт профиль без ключа, и задача 2 для самого частого источника бесполезна.

**Files:**
- Изменяется: `android/app/src/main/java/com/resultv/android/vpn/WireGuardConfParser.kt` (блок `awg3`, ~строка 78; список `isAmnezia`, ~строка 88)
- Изменяется: `android/app/src/test/java/com/resultv/android/vpn/WireGuardConfParserAwg3Test.kt`

**Interfaces:**
- Consumes: `WireGuardConfParser.toUri(...)` — существующая функция, сигнатура не меняется.
- Produces: ссылка `awg://…&random_trailers=on&disable_cookies=off`, которую разбирает `parseAmneziaWGURI` из задачи 2.

- [ ] **Step 1: Написать падающий тест**

Дописать в `android/app/src/test/java/com/resultv/android/vpn/WireGuardConfParserAwg3Test.kt`. Сигнатура — как у соседних тестов файла: `WireGuardConfParser.toUri(conf)` с одним аргументом, результат nullable.

```kotlin
    private val awg31Conf = """
        [Interface]
        PrivateKey = aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=
        Address = 10.8.1.2/24
        Jc = 8
        RandomTrailers = on
        DisableCookies = off

        [Peer]
        PublicKey = WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=
        Endpoint = 203.0.113.7:51820
        AllowedIPs = 0.0.0.0/0
    """.trimIndent()

    /**
     * random_trailers симметричен: узел с хвостами клиенту без флага не
     * поднимется вовсе. Потерять ключ при импорте — значит получить профиль,
     * который «почему-то не подключается», без единой строки в журнале.
     */
    @Test fun awg31SwitchesSurviveIntoTheUri() {
        val uri = WireGuardConfParser.toUri(awg31Conf)
        assertNotNull(uri)
        uri!!

        assertTrue("нет random_trailers в $uri", uri.contains("random_trailers=on"))
        assertTrue("нет disable_cookies в $uri", uri.contains("disable_cookies=off"))
    }

    /**
     * Конфиг с одними лишь ключами 3.1 — это всё ещё AmneziaWG: схема должна
     * стать awg://, иначе они уедут в ветку обычного WireGuard и потеряются.
     */
    @Test fun awg31SwitchesAloneMakeItAmnezia() {
        val conf = awg31Conf.replace("Jc = 8\n", "")
        val uri = WireGuardConfParser.toUri(conf)
        assertNotNull(uri)
        assertTrue("схема не awg://: $uri", uri!!.startsWith("awg://"))
    }

- [ ] **Step 2: Убедиться, что тесты падают**

```bash
cd /c/ResultV/android && ./gradlew :app:testFullDebugUnitTest --tests '*WireGuardConfParserAwg3Test*' 2>&1 | tail -20
```

Ожидается: два падения по `assertTrue`.

- [ ] **Step 3: Донести ключи**

В `WireGuardConfParser.kt`, в блок `awg3` (там, где `.conf`-имена уже приведены к нижнему регистру парсером секций), добавить две пары:

```kotlin
        // AmneziaWG 3.1: два выключателя, которых нет в конфиге ядра, — они
        // доезжают до устройства отдельным IpcSet. Значение передаётся как
        // есть (on/off/true/1): что оно значит, решает один читатель в Go
        // (awgBoolFromAny), а не каждый парсер по-своему.
        val awg31 = listOf(
            "random_trailers" to iface["randomtrailers"].orEmpty(),
            "disable_cookies" to iface["disablecookies"].orEmpty(),
        )
```

В вычислении `isAmnezia` добавить `+ awg31.map { it.second }`, а в блок `if (isAmnezia) { … }` — `pairs.addAll(awg31)` следом за `pairs.addAll(awg3)`.

- [ ] **Step 4: Тесты проходят**

```bash
cd /c/ResultV/android && ./gradlew :app:testFullDebugUnitTest --tests '*WireGuardConfParserAwg3Test*' 2>&1 | tail -10
```

Ожидается: BUILD SUCCESSFUL.

- [ ] **Step 5: Коммит**

```bash
cd /c/ResultV
git add android/app/src/main/java/com/resultv/android/vpn/WireGuardConfParser.kt \
        android/app/src/test/java/com/resultv/android/vpn/WireGuardConfParserAwg3Test.kt
git commit -F- <<'MSG'
feat(android): импорт .conf несёт ключи AmneziaWG 3.1

Конфиг AmneziaWG приходит к пользователю файлом чаще, чем ссылкой, и
разбирает его Kotlin — на ПК такого парсера нет вовсе. Без этих двух пар
разбор 3.1 в Go работал бы для всех источников, кроме самого частого.

Значение передаётся строкой как есть: что значит on/off/true/1, решает один
читатель в Go, иначе у каждого парсера заводится своё мнение.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
MSG
```

---

### Task 4: Проба пинга шлёт ключи 3.1

`ping_wg_handshake.go` строит UAPI-строку сам, минуя option-структуру ядра, — это единственный путь, который умеет говорить полным набором AmneziaWG. Для 3.1 это не украшение: `random_trailers` симметричен, поэтому проба без флага против узла с флагом получает отброшенные по размеру рукопожатия и читается как «сервер недоступен».

**Files:**
- Изменяется: `internal/proxy/ping_wg_handshake.go` (`writeAmneziaUAPI`, после цикла по `awg3Keys`, ~строка 297)
- Изменяется: `internal/proxy/ping_wg_awg3_uapi_test.go` (дописать в конец)

**Interfaces:**
- Consumes: `awg31Keys`, `awgBoolFromAny` (задача 2), `amneziaScalar(any) string`.
- Produces: строки `random_trailers=true` / `disable_cookies=false` в UAPI пробы.

- [ ] **Step 1: Написать падающие тесты**

Дописать в конец `internal/proxy/ping_wg_awg3_uapi_test.go`:

```go

// random_trailers is symmetric: a probe without the flag against a peer that
// has it on gets every handshake dropped for being the wrong size, and the
// node reads as unreachable. So the probe has to speak 3.1 too, and in the
// spelling the UAPI parses — strconv.ParseBool, which does not know on/off.
func TestWriteAmneziaUAPIEmitsAWG31Switches(t *testing.T) {
	var b strings.Builder
	writeAmneziaUAPI(&b, map[string]any{
		"amnezia": map[string]any{
			"jc":              8,
			"random_trailers": "on",
			"DisableCookies":  "off",
		},
	})
	got := b.String()

	for _, want := range []string{"random_trailers=true\n", "disable_cookies=false\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// Unstated is not "off": the device default can move, and sending a switch the
// config never mentioned would make the probe measure a different protocol
// from the one the tunnel will run.
func TestWriteAmneziaUAPIOmitsUnsetAWG31Switches(t *testing.T) {
	var b strings.Builder
	writeAmneziaUAPI(&b, map[string]any{"amnezia": map[string]any{"jc": 8}})
	got := b.String()

	for _, unwanted := range []string{"random_trailers", "disable_cookies"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("не заданный %s не должен попадать в UAPI:\n%s", unwanted, got)
		}
	}
}

// An unreadable value is not a switch. The probe must behave like the config
// path, which refuses it outright — here there is nobody to refuse to, so the
// switch simply stays unstated rather than becoming a guess.
func TestWriteAmneziaUAPISkipsUnreadableAWG31(t *testing.T) {
	var b strings.Builder
	writeAmneziaUAPI(&b, map[string]any{"amnezia": map[string]any{"random_trailers": "maybe"}})
	if got := b.String(); strings.Contains(got, "random_trailers") {
		t.Errorf("нечитаемое значение не должно доезжать до устройства:\n%s", got)
	}
}
```

- [ ] **Step 2: Убедиться, что первый и третий падают**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run 'TestWriteAmneziaUAPI.*AWG31' -v 2>&1 | tail -20
```

Ожидается: FAIL у `TestWriteAmneziaUAPIEmitsAWG31Switches`, PASS у двух остальных (пока ключей нет, они тривиально зелёные; после Step 3 они начинают что-то значить).

- [ ] **Step 3: Дописать 3.1 в UAPI пробы**

В `internal/proxy/ping_wg_handshake.go`, в `writeAmneziaUAPI`, сразу после цикла по `awg3Keys`:

```go
	// ── AmneziaWG 3.1 ──
	// Симметричный random_trailers делает это обязательным, а не приятным:
	// проба без флага против узла с флагом получает рукопожатия, отброшенные
	// по размеру, и узел читается как недоступный. Значение проходит через
	// того же читателя, что и путь конфига (awgBoolFromAny), и печатается в
	// том виде, который понимает UAPI: strconv.ParseBool не знает on/off.
	for _, k := range awg31Keys {
		for rawKey, rawVal := range amRaw {
			if normalizeAWGKey(rawKey) != normalizeAWGKey(k) {
				continue
			}
			if v := awgBoolFromAny(rawVal); v != nil {
				fmt.Fprintf(b, "%s=%t\n", k, *v)
			}
			break
		}
	}
```

- [ ] **Step 4: Тесты проходят**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run 'TestWriteAmneziaUAPI' -v 2>&1 | tail -20
go test -tags="${TAGS_PLAY}" ./internal/proxy/ -run 'TestWriteAmneziaUAPI' 2>&1 | tail -3
```

Ожидается: все PASS в обеих конфигурациях.

- [ ] **Step 5: Коммит**

```bash
cd /c/ResultV
git add internal/proxy/ping_wg_handshake.go internal/proxy/ping_wg_awg3_uapi_test.go
git commit -F- <<'MSG'
feat(proxy): проба рукопожатия WG говорит на AmneziaWG 3.1

Проба строит UAPI-строку сама, минуя option-структуру ядра, и это
единственный путь, который умеет полный набор AmneziaWG. Для 3.1 это не
украшение: random_trailers симметричен, и проба без флага против узла с
флагом получает рукопожатия, отброшенные по размеру, — узел читается как
недоступный, хотя туннель к нему встанет.

Нечитаемое значение остаётся незаданным, а не превращается в «выключено»:
проба обязана мерить тот же протокол, который потом поднимет туннель.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
MSG
```

---

### Task 5: Ключи доезжают до живого устройства (Go-сторона)

Путь к устройству идёт через неэкспортированные поля ядра, потому что другого шва нет: UAPI-строку ядро собирает у себя внутри и наружу не отдаёт ни устройство, ни хук. Каждый шаг проверяется по имени и типу, срыв сообщается и деградирует до поведения 3.0 — падать на подключении из-за переименованного поля недопустимо.

Отличие от ПК: там `applyAWG31` берёт менеджер эндпоинтов из контекста ядра (`service.FromContext`), потому что контекст строит сам `SingBoxEngine`. На `android` этого контекста нет ни у кого в Go — ядро поднимает `libbox`, — поэтому функция принимает `adapter.EndpointManager` параметром, а добывает его биндинг из задачи 6.

**Files:**
- Изменяется: `internal/proxy/awg31.go` (дописать путь до устройства)
- Изменяется: `internal/proxy/awg31_test.go` (дописать пломбу и проверки отказов)

**Interfaces:**
- Consumes: `awg31KnobsFor`, `awg31Knobs.ipcLines/describe/empty`, `awg31Error` (задача 2); `adapter.EndpointManager` и `*wgprotocol.Endpoint` из ядра.
- Produces:
  - `const wireguardEndpointTag = "proxy"` — **объявляется в `endpoints.go`** (задача 9 переводит на неё литерал в `buildEndpoints`; здесь она уже нужна, поэтому объявляется тут же, в `endpoints.go`, а не в `awg31.go`);
  - `ApplyAWG31(manager adapter.EndpointManager, proxyCfg ProxyConfig) (string, error)` — описание применённого или `""`, когда узел ничего не заявил;
  - `wgEndpointFrom(manager adapter.EndpointManager) (*wgprotocol.Endpoint, error)`;
  - `awg31DeviceInterface(*wgprotocol.Endpoint) (any, error)`, `awg31Device(*wgprotocol.Endpoint) (ipcSetter, error)`;
  - `unexportedField(reflect.Value, string) (reflect.Value, error)`;
  - интерфейс `ipcSetter{ IpcSet(string) error }`.

- [ ] **Step 1: Написать падающие тесты**

Дописать в конец `internal/proxy/awg31_test.go` (и добавить в импорты `reflect` и `wgprotocol "github.com/sagernet/sing-box/protocol/wireguard"`):

```go

// The path ApplyAWG31 walks is made of unexported fields, so it cannot be
// checked by the compiler. This test is the guard: it fails on the next engine
// bump that renames or retypes any step, which is the moment to re-check the
// route — not months later, in a user's log, as "3.1 not applied".
func TestAWG31DeviceFieldsStillExist(t *testing.T) {
	protocolEndpoint := reflect.TypeOf(wgprotocol.Endpoint{})
	field, found := protocolEndpoint.FieldByName("endpoint")
	if !found {
		t.Fatal("protocol/wireguard.Endpoint потерял поле endpoint — путь к устройству надо искать заново")
	}
	transportEndpoint := field.Type
	if transportEndpoint.Kind() != reflect.Pointer {
		t.Fatalf("ожидался указатель на transport-эндпоинт, получено %s", transportEndpoint)
	}
	deviceField, found := transportEndpoint.Elem().FieldByName("device")
	if !found {
		t.Fatal("transport/wireguard.Endpoint потерял поле device")
	}
	if !deviceField.Type.Implements(reflect.TypeOf((*ipcSetter)(nil)).Elem()) {
		t.Fatalf("устройство %s больше не принимает IpcSet", deviceField.Type)
	}
}

// Nothing stated means nothing sent: a node that says nothing about 3.1 must
// not have its device touched at all — and must not be an error either.
func TestAWG31EmptyKnobsDoNothing(t *testing.T) {
	applied, err := ApplyAWG31(nil, ProxyConfig{Type: "amneziawg", Extra: []byte(`{"amnezia":{"jc":4}}`)})
	if err != nil {
		t.Errorf("пустой набор не должен быть ошибкой: %v", err)
	}
	if applied != "" {
		t.Errorf("применять было нечего, получено описание %q", applied)
	}
}

// Every failure on the way to the device is reported, never panicked: a session
// with 3.0 behaviour is worth more than a crash on connect.
func TestAWG31ReportsMissingManager(t *testing.T) {
	_, err := ApplyAWG31(nil, ProxyConfig{Type: "amneziawg", Extra: []byte(`{"amnezia":{"random_trailers":"on"}}`)})
	if err == nil {
		t.Error("отсутствие менеджера эндпоинтов должно быть ошибкой")
	}
}

// A zero-value endpoint has no device yet — the state a domain-addressed peer
// is in until the post-start stage. It must read as "not ready", not as a
// broken engine.
func TestAWG31DeviceNotReady(t *testing.T) {
	_, err := awg31Device(&wgprotocol.Endpoint{})
	if err == nil {
		t.Fatal("ожидалась ошибка для эндпоинта без устройства")
	}
	if !strings.Contains(err.Error(), "ещё не создано") {
		t.Errorf("ошибка должна называть состояние, получено: %v", err)
	}
}
```

- [ ] **Step 2: Убедиться, что тесты падают**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run 'TestAWG31(Device|Empty|Reports)' 2>&1 | tail -10
```

Ожидается: `undefined: ApplyAWG31`, `undefined: ipcSetter`, `undefined: awg31Device`.

- [ ] **Step 3: Объявить тег эндпоинта**

В `internal/proxy/endpoints.go`, рядом с `awg3Keys` (перед ним):

```go
// wireguardEndpointTag is the tag a WireGuard/AmneziaWG node is given in the
// engine config. Named because ApplyAWG31 and the WG counters look the
// endpoint up by it after start — the two must never drift apart.
const wireguardEndpointTag = "proxy"
```

и заменить в `buildEndpoints` строку `Tag: "proxy",` на `Tag: wireguardEndpointTag,`.

- [ ] **Step 4: Дописать путь до устройства**

В `internal/proxy/awg31.go` добавить в импорты `fmt`, `reflect`, `unsafe`, `"github.com/sagernet/sing-box/adapter"`, `wgprotocol "github.com/sagernet/sing-box/protocol/wireguard"` и дописать в конец файла:

```go
// ipcSetter is the one method needed off the core's WireGuard device. Declared
// here rather than imported so this file does not pull wireguard-go in for a
// single signature.
type ipcSetter interface {
	IpcSet(string) error
}

// ApplyAWG31 pushes a node's 3.1 switches into the running device.
//
// Why this is reached through unexported fields: the core builds the UAPI
// string itself inside transport/wireguard.Endpoint.Start and exposes neither
// the device nor a hook, so the only seam left is the object graph —
// protocol/wireguard.Endpoint.endpoint → transport/wireguard.Endpoint.device.
// Every step is verified by name and type, and a mismatch is reported instead
// of panicking: an engine bump that renames a field must degrade to "3.1 not
// applied" with a line in the log, never to a crash on connect. The guard that
// catches such a bump early is TestAWG31DeviceFieldsStillExist.
//
// The manager comes in as a parameter rather than out of a context because on
// Android nobody in Go owns the box: libbox starts it, and the binding hands us
// its endpoint manager (mobile/libbox_awg31.go).
//
// Returns the description of what was applied, empty when the node states
// nothing at all.
func ApplyAWG31(manager adapter.EndpointManager, proxyCfg ProxyConfig) (string, error) {
	knobs := awg31KnobsFor(proxyCfg)
	if knobs.empty() {
		return "", nil
	}
	wgEndpoint, err := wgEndpointFrom(manager)
	if err != nil {
		return "", err
	}
	device, err := awg31Device(wgEndpoint)
	if err != nil {
		return "", err
	}
	if err := device.IpcSet(knobs.ipcLines()); err != nil {
		return "", &awg31Error{"ядро отклонило параметры: " + err.Error()}
	}
	return knobs.describe(), nil
}

// wgEndpointFrom resolves the WireGuard endpoint the session is running on.
func wgEndpointFrom(manager adapter.EndpointManager) (*wgprotocol.Endpoint, error) {
	if manager == nil {
		return nil, &awg31Error{"ядро не отдало менеджер эндпоинтов"}
	}
	ep, loaded := manager.Get(wireguardEndpointTag)
	if !loaded {
		return nil, &awg31Error{"эндпоинт " + wireguardEndpointTag + " не найден"}
	}
	wgEndpoint, ok := ep.(*wgprotocol.Endpoint)
	if !ok {
		return nil, &awg31Error{"узел не WireGuard"}
	}
	return wgEndpoint, nil
}

// awg31Device walks the endpoint down to the wireguard-go device.
func awg31Device(wgEndpoint *wgprotocol.Endpoint) (ipcSetter, error) {
	deviceValue, err := awg31DeviceInterface(wgEndpoint)
	if err != nil {
		return nil, err
	}
	device, ok := deviceValue.(ipcSetter)
	if !ok {
		return nil, &awg31Error{fmt.Sprintf("устройство не принимает UAPI (%T)", deviceValue)}
	}
	return device, nil
}

// awg31DeviceInterface exposes the device as an untyped value so both the 3.1
// writer and the stats reader can ask it for their own interface.
func awg31DeviceInterface(wgEndpoint *wgprotocol.Endpoint) (any, error) {
	transportEndpoint, err := unexportedField(reflect.ValueOf(wgEndpoint), "endpoint")
	if err != nil {
		return nil, &awg31Error{"protocol/wireguard.Endpoint: " + err.Error()}
	}
	if transportEndpoint.Kind() == reflect.Pointer && transportEndpoint.IsNil() {
		return nil, &awg31Error{"устройство WireGuard ещё не создано"}
	}
	deviceValue, err := unexportedField(transportEndpoint, "device")
	if err != nil {
		return nil, &awg31Error{"transport/wireguard.Endpoint: " + err.Error()}
	}
	if !deviceValue.IsValid() || deviceValue.IsZero() {
		return nil, &awg31Error{"устройство WireGuard ещё не создано"}
	}
	return deviceValue.Interface(), nil
}

// unexportedField reads an unexported struct field by name.
//
// reflect refuses to hand over unexported values through Interface(), so the
// field is re-created at its own address — the standard escape hatch, and the
// reason every caller here checks the field exists first instead of trusting
// the layout.
func unexportedField(v reflect.Value, name string) (reflect.Value, error) {
	if !v.IsValid() {
		return reflect.Value{}, fmt.Errorf("нет значения для поля %s", name)
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return reflect.Value{}, fmt.Errorf("нулевой указатель вместо структуры с полем %s", name)
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("%s: ожидалась структура, получено %s", name, v.Kind())
	}
	field := v.FieldByName(name)
	if !field.IsValid() {
		return reflect.Value{}, fmt.Errorf("поле %s исчезло из ядра", name)
	}
	if !field.CanAddr() {
		return reflect.Value{}, fmt.Errorf("поле %s недоступно по адресу", name)
	}
	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem(), nil
}
```

- [ ] **Step 5: Тесты проходят в обеих конфигурациях**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run 'TestAWG31' -v 2>&1 | tail -25
go test -tags="${TAGS_PLAY}" ./internal/proxy/ 2>&1 | tail -3
go build -tags="${TAGS_FULL}" ./... && go build -tags="${TAGS_PLAY}" ./... && echo BOTH BUILD OK
```

Ожидается: все `TestAWG31*` PASS, `ok` у пакета в play, обе сборки OK.

- [ ] **Step 6: Коммит**

```bash
cd /c/ResultV
git add internal/proxy/awg31.go internal/proxy/awg31_test.go internal/proxy/endpoints.go
git commit -F- <<'MSG'
feat(proxy): параметры AmneziaWG 3.1 доезжают до живого устройства

Ядро собирает UAPI-строку у себя внутри и наружу не отдаёт ни устройство, ни
хук, поэтому единственный шов — граф объектов: protocol/wireguard.Endpoint →
transport/wireguard.Endpoint → device. Каждый шаг проверяется по имени и
типу, срыв возвращается ошибкой: подъём ядра, переименовавший поле, обязан
деградировать до «3.1 не применены» строкой в журнале, а не до падения на
подключении. Раньше пользователя это поймает
TestAWG31DeviceFieldsStillExist.

Менеджер эндпоинтов приходит параметром, а не из контекста ядра, как на ПК:
на Android контекста нет ни у кого в Go — ядро поднимает libbox, и менеджер
достаёт биндинг.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
MSG
```

---

### Task 6: Мост Kotlin → Go: ключи применяются к запущенному ядру

Здесь проверяется на прочность главное допущение этой задачи: `gomobile bind` получает оба пакета одним вызовом (`./mobile` и `experimental/libbox`, см. `scripts/build-android-aar.sh:212`), а `bind/gen.go:isSupported` разрешает параметр-указатель на именованный тип из **любого** связанного пакета (`validPkg` сверяется с `g.AllPkg`). Значит `func ApplyAWG31(server *libbox.CommandServer) …` связывается. Если сборка AAR это опровергнет — **остановиться и доложить**, не изобретая обход: запасной путь (зеркало интерфейсов libbox в `mobile`) стоит примерно как весь остальной план, и решать, платить ли за него, человеку.

Узел, к которому применяются ключи, `mobile` запоминает в `buildSingBoxConfigFromEntry` — единственной воронке, через которую проходят все сборки конфига (`BuildSingBoxConfigV2` по ссылке, `BuildSingBoxConfigFromEntryV2` по записи, AUTO с подменой члена группы). Через неё же проходят и все reload-ы. Альтернатива — протащить `entryJson` в пять Kotlin-вызовов `BoxModule.start/reload` — дороже и хрупче: два из пяти вызовов (`BrowserAdBlockAttachment`) узла в руках не держат вовсе.

**Files:**
- Создаётся: `mobile/libbox_awg31.go`
- Создаётся: `mobile/libbox_awg31_test.go`
- Изменяется: `mobile/libbox.go` (в `buildSingBoxConfigFromEntry`, сразу после создания `cfg`)
- Изменяется: `android/app/src/main/java/com/resultv/android/vpn/BoxModule.kt` (в `start()` после `startOrRecover`, в `reload()` после `startOrReloadService`)
- Изменяется: `android/app/src/main/res/values/strings.xml`, `android/app/src/main/res/values-ru/strings.xml`

**Interfaces:**
- Consumes: `proxy.ApplyAWG31(adapter.EndpointManager, proxy.ProxyConfig) (string, error)` (задача 5); `libbox.CommandServer.Instance() *daemon.Instance`, `(*daemon.Instance).Box() *box.Box`, `(*box.Box).Endpoint() adapter.EndpointManager` — всё экспортировано закреплённым ядром.
- Produces: Go `func ApplyAWG31(server *libbox.CommandServer) (string, error)` → Kotlin `Mobile.applyAWG31(server): String`; `rememberBuiltNode(proxy.ProxyConfig)` / `builtNode() proxy.ProxyConfig` внутри пакета `mobile`; строки `log_awg31_applied`, `log_awg31_failed`.

- [ ] **Step 1: Написать падающий тест памяти об узле**

Создать `mobile/libbox_awg31_test.go`:

```go
// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package mobile

import (
	"strings"
	"testing"
)

// The knobs are applied to whatever node the engine was actually started with,
// and that is the node the last config was built from — including the AUTO
// path, which swaps the member without touching the user's profile. So the
// memory has to be refreshed by the one funnel every build goes through.
func TestBuiltNodeFollowsTheLastBuiltConfig(t *testing.T) {
	dir := t.TempDir()
	opts := encodeOptions(BuildOptions{})

	awg := `awg://UFJJVg@1.2.3.4:51820?address=10.0.0.2%2F32&public_key=UFVC&allowed_ips=0.0.0.0%2F0&RandomTrailers=on#awg`
	if _, err := BuildSingBoxConfigV2(awg, dir, opts); err != nil {
		t.Fatalf("сборка конфига AWG: %v", err)
	}
	if got := builtNode(); !strings.EqualFold(got.Type, "AMNEZIAWG") {
		t.Fatalf("после сборки AWG узел = %q", got.Type)
	}

	vless := `vless://11111111-1111-1111-1111-111111111111@5.6.7.8:443?encryption=none&security=tls&type=tcp#v`
	if _, err := BuildSingBoxConfigV2(vless, dir, opts); err != nil {
		t.Fatalf("сборка конфига VLESS: %v", err)
	}
	if got := builtNode(); strings.EqualFold(got.Type, "AMNEZIAWG") {
		t.Error("память об узле не обновилась — ключи 3.1 уехали бы на чужой узел")
	}
}

// Nothing built yet is not an error: applying to an empty node must be a no-op,
// not a crash on the first connect after a process restart.
func TestApplyAWG31WithoutServer(t *testing.T) {
	if _, err := ApplyAWG31(nil); err == nil {
		t.Error("без сервера должна быть ошибка, а не тишина")
	}
}
```

- [ ] **Step 2: Убедиться, что тест падает**

```bash
go test -tags="${TAGS_FULL}" ./mobile/ -run 'TestBuiltNode|TestApplyAWG31' 2>&1 | tail -10
```

Ожидается: `undefined: builtNode`, `undefined: ApplyAWG31`.

- [ ] **Step 3: Написать биндинг**

Создать `mobile/libbox_awg31.go`:

```go
// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package mobile

import (
	"fmt"
	"sync"

	"github.com/sagernet/sing-box/experimental/libbox"

	"resultproxy-wails/internal/proxy"
)

// The AmneziaWG 3.1 switches cannot travel in the engine config (see
// internal/proxy/awg31.go), so they are pushed into the running device after
// start. On Android that means reaching the box, and the box belongs to Kotlin:
// libbox.CommandServer is created there and lives for the session. gomobile
// binds both packages in one invocation (scripts/build-android-aar.sh), so a
// *libbox.CommandServer is a legal parameter here — that is the whole reason
// this file can exist.
//
// The node is not passed in. It is remembered at the moment its config is
// built, which is the one funnel every path goes through — subscription entry,
// pasted URI, AUTO member swap, and every reload. Threading the entry through
// the Kotlin call sites instead would mean touching five of them, two of which
// (the browser ad-block attachment) never hold the node at all.
var (
	builtNodeMu sync.Mutex
	builtNodeIn proxy.ProxyConfig
)

// rememberBuiltNode records the node the engine is about to be started with.
func rememberBuiltNode(p proxy.ProxyConfig) {
	builtNodeMu.Lock()
	defer builtNodeMu.Unlock()
	builtNodeIn = p
}

// builtNode returns the node of the most recently built config.
func builtNode() proxy.ProxyConfig {
	builtNodeMu.Lock()
	defer builtNodeMu.Unlock()
	return builtNodeIn
}

// ApplyAWG31 pushes the AmneziaWG 3.1 switches of the running node into its
// live WireGuard device. Returns a description of what was applied ("" when
// the node states nothing), and an error when the device could not be reached
// — which is not fatal: the session keeps running with 3.0 behaviour.
//
// Called from Kotlin right after startOrReloadService, on start and on every
// reload: the device is recreated each time, and random_trailers has to match
// the peer before the first handshake is answered.
func ApplyAWG31(server *libbox.CommandServer) (string, error) {
	if server == nil {
		return "", fmt.Errorf("AWG 3.1: сервер ядра не передан")
	}
	instance := server.Instance()
	if instance == nil || instance.Box() == nil {
		return "", fmt.Errorf("AWG 3.1: ядро не запущено")
	}
	return proxy.ApplyAWG31(instance.Box().Endpoint(), builtNode())
}
```

- [ ] **Step 4: Запомнить узел при сборке конфига**

В `mobile/libbox.go`, в `buildSingBoxConfigFromEntry`, сразу после закрывающей скобки литерала `cfg := proxy.EngineConfig{…}`:

```go
	// Узел, на котором ядро вот-вот поднимут. Нужен применению AmneziaWG 3.1
	// после старта — в конфиг эти ключи не попадают (см. awg31.go).
	rememberBuiltNode(cfg.Proxy)
```

- [ ] **Step 5: Тесты проходят**

```bash
go test -tags="${TAGS_FULL}" ./mobile/ -run 'TestBuiltNode|TestApplyAWG31' -v 2>&1 | tail -15
go test -tags="${TAGS_PLAY}" ./mobile/ 2>&1 | tail -3
```

Ожидается: PASS обоих, `ok` у пакета в play.

- [ ] **Step 6: Собрать AAR и проверить, что метод связался**

Это и есть проверка допущения задачи.

```bash
cd /c/ResultV
DIST=full ./scripts/build-android-aar.sh > /tmp/aar-full.log 2>&1
tail -5 /tmp/aar-full.log
ls -l android/libs/libbox-full.aar
mkdir -p /tmp/aarcheck && cd /tmp/aarcheck && rm -f classes.jar
unzip -o -q /c/ResultV/android/libs/libbox-full.aar classes.jar
javap -cp classes.jar mobile.Mobile | grep -i awg
```

Ожидается: строка вида `public static java.lang.String applyAWG31(libbox.CommandServer) throws java.lang.Exception;` (рядом будет уже существующий `unsupportedAWGKnobs`).

Если вместо этого `gomobile bind` упал или метода нет — **остановиться**, сохранить лог и доложить. Обход не изобретать.

- [ ] **Step 7: Позвать биндинг из Kotlin**

В `BoxModule.kt` добавить приватный метод (рядом со `startOrRecover`):

```kotlin
    /**
     * Дослать в поднятое устройство параметры AmneziaWG 3.1.
     *
     * Через конфиг они недостижимы: в option.WireGuardAmnezia таких полей нет,
     * а неизвестный ключ под "amnezia" роняет старт ядра целиком. Поэтому
     * вторым IpcSet и только после старта — устройство до него не существует,
     * а random_trailers должен совпасть с узлом раньше первого рукопожатия.
     *
     * Срыв не фатален: сессия продолжает работать по 3.0, и об этом пишется
     * строка — молчание здесь означало бы узел, который «почему-то не
     * подключается».
     */
    private fun applyAwg31(server: CommandServer) {
        val applied = try {
            Mobile.applyAWG31(server)
        } catch (t: Throwable) {
            Log.w(TAG, "AWG 3.1 not applied", t)
            AppLog.warning(
                R.string.log_awg31_failed,
                t.message ?: t.javaClass.simpleName,
                source = AppLog.resolve(R.string.log_source_proxy),
            )
            return
        }
        if (applied.isNotBlank()) {
            Log.i(TAG, "AWG 3.1 applied: $applied")
            AppLog.info(
                R.string.log_awg31_applied,
                applied,
                source = AppLog.resolve(R.string.log_source_proxy),
            )
        }
    }
```

Вызвать его в `start()` сразу после блока `try { startOrRecover(...) } catch { … }` и до `commandServer = server`, и в `reload()` сразу после `server.startOrReloadService(configJson, OverrideOptions())`.

Импорты: `com.resultv.android.R` и `mobile.Mobile` (проверить, чего в файле ещё нет — `mobile.Mobile` там пока используется по полному имени в одном месте).

- [ ] **Step 8: Добавить строки**

`android/app/src/main/res/values/strings.xml`, рядом с `log_awg_knobs_dropped`:

```xml
    <string name="log_awg31_applied">AmneziaWG 3.1: applied %1$s.</string>
    <string name="log_awg31_failed">AmneziaWG 3.1 parameters were not applied: %1$s. The tunnel runs with 3.0 behaviour — if the server adds random trailers, the handshake will not complete.</string>
```

`android/app/src/main/res/values-ru/strings.xml`, там же:

```xml
    <string name="log_awg31_applied">AmneziaWG 3.1: применены %1$s.</string>
    <string name="log_awg31_failed">Параметры AmneziaWG 3.1 не применены: %1$s. Туннель работает по 3.0 — если сервер дописывает случайные хвосты, рукопожатие не завершится.</string>
```

- [ ] **Step 9: Собрать APK и прогнать Kotlin-тесты**

```bash
cd /c/ResultV/android
./gradlew :app:assembleFullDebug -Pdebug.abi=arm64-v8a 2>&1 | tail -5
./gradlew :app:testFullDebugUnitTest :app:testPlayDebugUnitTest 2>&1 | tail -10
```

Ожидается: BUILD SUCCESSFUL везде. (Сборка APK здесь — проверка компиляции Kotlin против нового AAR, а не поставка на телефон.)

- [ ] **Step 10: Коммит**

```bash
cd /c/ResultV
git add mobile/libbox_awg31.go mobile/libbox_awg31_test.go mobile/libbox.go \
        android/app/src/main/java/com/resultv/android/vpn/BoxModule.kt \
        android/app/src/main/res/values/strings.xml android/app/src/main/res/values-ru/strings.xml
git commit -F- <<'MSG'
feat(android): AmneziaWG 3.1 применяется к поднятому ядру

На Android ядро поднимает libbox, и владеет им Kotlin — контекста ядра, из
которого ПК достаёт менеджер эндпоинтов, здесь нет ни у кого в Go. Шов
нашёлся в том, что gomobile получает оба пакета одним вызовом: указатель на
libbox.CommandServer — законный параметр биндинга, и через
server.Instance().Box().Endpoint() менеджер доезжает до Go.

Узел биндингу не передаётся: он запоминается в момент сборки конфига —
единственной воронке, через которую проходят и подписка, и вставленная
ссылка, и подмена члена AUTO, и любой reload. Протаскивание записи профиля
через пять Kotlin-вызовов было бы дороже и хрупче: два из них узла в руках
не держат вовсе.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
MSG
```

---

---

> **Задачи 7-10 исполнены и откачены (2026-09-17, решение человека).**
>
> Счётчики WG и gVisor-стека, переопределение MTU, переключатель стека TUN и
> блок «Диагностика» в настройках были сделаны целиком и работали, после чего
> откачены одним коммитом. Причина: все четыре — инструменты одного
> расследования на ПК от 14.09, а на Android то расследование закрыто приёмкой
> блока 1 (WG/AWG были прибиты к системному стеку TUN, `7c9376e`). То есть
> переносился инструмент к уже найденной причине. Отдельно про стек:
> единственное, что ряд настроек предлагал кроме дефолта, — значение `system`,
> которое на `sing-tun 0.9` заведомо убивает TCP на телефоне.
>
> Шаги ниже оставлены как есть — по ним видно, что именно было сделано, и по
> ним же это вернуть, если на телефоне всплывёт WG-баг, которому нужны
> счётчики. Приёмка (задача 11) сокращена до объёма AmneziaWG 3.1.

---

### Task 7: Счётчики WG-устройства и gVisor-стека (Go-сторона)

Вопрос, на который лог WireGuard не отвечает сам: когда туннель замолкает, ушли ли наши пакеты и пришло ли что-нибудь назад. Ядро логирует рукопожатия и keepalive, а данные — нет, поэтому сессия, переставшая возить трафик при живом с виду устройстве, выглядит как полная тишина. Счётчики устройства на это отвечают: `tx` растёт при неподвижном `rx` — пакеты уходят, ответа нет; оба стоят — до устройства они не доезжают, и виновато то, что стоит над WireGuard. Счётчики gVisor-стека внутри эндпоинта отвечают на следующий вопрос: пакеты доехали, а соединения не встают — почему.

Ровно эта пара разделила причины на ПК 14.09.2026 и привела к находке про стек TUN, которая на Android аукнулась в приёмке блока 1.

**Files:**
- Создаётся: `internal/proxy/wgdiag.go`
- Создаётся: `internal/proxy/wgdiag_test.go`

**Interfaces:**
- Consumes: `wgEndpointFrom`, `awg31DeviceInterface`, `awg31Device`, `unexportedField`, `awg31Error` (задача 5); `tcpip.Stats` из `github.com/sagernet/gvisor` (сейчас непрямая зависимость — станет прямой, `go mod tidy` перенесёт её в основной блок `go.mod`).
- Produces: `WGDiagLine(manager adapter.EndpointManager) (string, error)` — одна строка «устройство + стек»; `filterWGStats(string) string`; `wgStatsFields []string`; `wgStackStatsLine(adapter.EndpointManager) (string, error)`; `wgStackFrom(*wgprotocol.Endpoint) (statsProvider, error)`.

- [ ] **Step 1: Написать падающие тесты**

Создать `internal/proxy/wgdiag_test.go`:

```go
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
)

// The UAPI dump carries the private key and the preshared key. This line goes
// into a log the user is asked to hand to someone, so the filter is a
// whitelist, and the test pins that: a new field in the core's dump must not
// appear here by default.
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

// A node that is not WireGuard has no device to read, and that must be an
// ordinary error rather than a panic on a live session.
func TestWGDiagLineReportsMissingEndpoint(t *testing.T) {
	if _, err := WGDiagLine(nil); err == nil {
		t.Error("без менеджера эндпоинтов должна быть ошибка")
	}
}
```

- [ ] **Step 2: Убедиться, что тесты падают**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run 'TestWG(Stats|DiagLine)' 2>&1 | tail -10
```

Ожидается: `undefined: filterWGStats`, `undefined: WGDiagLine`.

- [ ] **Step 3: Написать счётчики**

Создать `internal/proxy/wgdiag.go`:

```go
// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/sagernet/gvisor/pkg/tcpip"
	"github.com/sagernet/sing-box/adapter"
	wgprotocol "github.com/sagernet/sing-box/protocol/wireguard"
)

// The question a WireGuard log cannot answer on its own: when a tunnel goes
// quiet, did our packets leave, and did anything come back?
//
// The core logs handshakes and keepalives, and nothing at all for data — so a
// session that stops carrying traffic while the device still believes it is up
// produces total silence, with the browser hammering the tunnel the whole time.
//
// The device's own counters do answer it. tx_bytes rising with rx_bytes flat
// means our packets go out and nothing returns — a path or peer problem. Both
// flat means the packets never reached the device at all, and the fault is
// above WireGuard, in the stack that feeds it. last_handshake_time_sec says
// whether the device ever tried to re-establish the session.
//
// Sampling is driven from Kotlin and only while the verbose log is on, so it
// costs nothing in an ordinary session.
const (
	wgStatsPrefix = "wg-stats"
	wgStackPrefix = "wg-stack"
)

// wgStatsFields is what gets written down. Deliberately a whitelist: the UAPI
// dump also carries private_key and preshared_key, and this line goes into a
// log the user is asked to hand to someone.
var wgStatsFields = []string{
	"endpoint",
	"last_handshake_time_sec",
	"tx_bytes",
	"rx_bytes",
	"persistent_keepalive_interval",
	"protocol_version",
}

// ipcGetter is the read half of the device's UAPI.
type ipcGetter interface {
	IpcGet() (string, error)
}

// WGDiagLine returns one line with the WireGuard device counters and, when the
// endpoint runs on the gVisor stack, its stack counters after them.
//
// An endpoint on the system stack has no stack to read; that half is dropped
// silently rather than failing the whole line, because the device counters are
// still the answer to the first question.
func WGDiagLine(manager adapter.EndpointManager) (string, error) {
	device, err := wgStatsLine(manager)
	if err != nil {
		return "", err
	}
	line := wgStatsPrefix + " " + device
	if stack, stackErr := wgStackStatsLine(manager); stackErr == nil {
		line += " | " + wgStackPrefix + " " + stack
	}
	return line, nil
}

// wgStatsLine reads the device's UAPI dump and keeps the fields worth having.
func wgStatsLine(manager adapter.EndpointManager) (string, error) {
	endpoint, err := wgEndpointFrom(manager)
	if err != nil {
		return "", err
	}
	deviceValue, err := awg31DeviceInterface(endpoint)
	if err != nil {
		return "", err
	}
	getter, ok := deviceValue.(ipcGetter)
	if !ok {
		return "", &awg31Error{"устройство не отдаёт UAPI"}
	}
	dump, err := getter.IpcGet()
	if err != nil {
		return "", &awg31Error{"UAPI не прочитан: " + err.Error()}
	}
	return filterWGStats(dump), nil
}

// filterWGStats keeps only the whitelisted keys, in one line.
func filterWGStats(dump string) string {
	var kept []string
	for _, line := range strings.Split(dump, "\n") {
		line = strings.TrimSpace(line)
		key, _, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		for _, want := range wgStatsFields {
			if key == want {
				kept = append(kept, line)
				break
			}
		}
	}
	if len(kept) == 0 {
		return "(пусто)"
	}
	return strings.Join(kept, " ")
}

// The counters above stop at the WireGuard device: they say packets left and
// packets came back, and nothing about what happened to them afterwards. The
// next step is the gVisor stack that sits on top of the device inside the
// endpoint. Received-but-invalid segments, packets dropped for a destination
// the stack does not recognise, failed connection attempts and resets each
// point at a different cause, and they are counted whether or not anything is
// logged.

// statsProvider is the gVisor stack's own accounting.
type statsProvider interface {
	Stats() tcpip.Stats
}

// wgStackStatsLine reads the endpoint's gVisor stack counters.
func wgStackStatsLine(manager adapter.EndpointManager) (string, error) {
	endpoint, err := wgEndpointFrom(manager)
	if err != nil {
		return "", err
	}
	stackValue, err := wgStackFrom(endpoint)
	if err != nil {
		return "", err
	}
	stats := stackValue.Stats()
	fields := []struct {
		name    string
		counter *tcpip.StatCounter
	}{
		{"ip_received", stats.IP.PacketsReceived},
		{"ip_delivered", stats.IP.PacketsDelivered},
		{"ip_sent", stats.IP.PacketsSent},
		{"ip_bad_dst", stats.IP.InvalidDestinationAddressesReceived},
		{"ip_malformed", stats.IP.MalformedPacketsReceived},
		{"ip_out_err", stats.IP.OutgoingPacketErrors},
		{"tcp_opened", stats.TCP.ActiveConnectionOpenings},
		{"tcp_established", stats.TCP.CurrentEstablished},
		{"tcp_failed", stats.TCP.FailedConnectionAttempts},
		{"tcp_valid_in", stats.TCP.ValidSegmentsReceived},
		{"tcp_invalid_in", stats.TCP.InvalidSegmentsReceived},
		{"tcp_sent", stats.TCP.SegmentsSent},
		{"tcp_send_err", stats.TCP.SegmentSendErrors},
		{"tcp_rst_in", stats.TCP.ResetsReceived},
		{"tcp_rst_out", stats.TCP.ResetsSent},
		{"tcp_retransmit", stats.TCP.Retransmits},
		{"tcp_timeout", stats.TCP.EstablishedTimedout},
	}
	var parts []string
	for _, field := range fields {
		if field.counter == nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%d", field.name, field.counter.Value()))
	}
	return strings.Join(parts, " "), nil
}

// wgStackFrom walks the endpoint down to the gVisor stack inside its device.
func wgStackFrom(wgEndpoint *wgprotocol.Endpoint) (statsProvider, error) {
	transportEndpoint, err := unexportedField(reflect.ValueOf(wgEndpoint), "endpoint")
	if err != nil {
		return nil, &awg31Error{"protocol/wireguard.Endpoint: " + err.Error()}
	}
	if transportEndpoint.Kind() == reflect.Pointer && transportEndpoint.IsNil() {
		return nil, &awg31Error{"устройство WireGuard ещё не создано"}
	}
	tunDevice, err := unexportedField(transportEndpoint, "tunDevice")
	if err != nil {
		return nil, &awg31Error{"transport/wireguard.Endpoint: " + err.Error()}
	}
	if !tunDevice.IsValid() || tunDevice.IsZero() {
		return nil, &awg31Error{"устройство WireGuard ещё не создано"}
	}
	// tunDevice is an interface holding *stackDevice for a gVisor endpoint, and
	// something else entirely for a system one — where there is no stack to read
	// and nothing to report.
	stackField, err := unexportedField(reflect.ValueOf(tunDevice.Interface()), "stack")
	if err != nil {
		return nil, &awg31Error{"устройство без gVisor-стека: " + err.Error()}
	}
	provider, ok := stackField.Interface().(statsProvider)
	if !ok {
		return nil, &awg31Error{"стек не отдаёт статистику"}
	}
	return provider, nil
}
```

- [ ] **Step 4: Привести go.mod в порядок и прогнать тесты**

```bash
cd /c/ResultV
go mod tidy
git diff --stat go.mod go.sum
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run 'TestWG' -v 2>&1 | tail -15
go test -tags="${TAGS_PLAY}" ./internal/proxy/ 2>&1 | tail -3
go build -tags="${TAGS_FULL}" ./... && go build -tags="${TAGS_PLAY}" ./... && echo BOTH BUILD OK
```

Ожидается: в `go.mod` `github.com/sagernet/gvisor` переезжает из блока `// indirect` в прямые — других изменений быть не должно. Тесты PASS.

Если `go mod tidy` вычистил что-то ещё — вспомнить грабли блока 1: `gomobile bind` собирает второй пакет, невидимый для `tidy`, и его зависимости держатся холостым импортом в `mobile/libbox.go`. Ничего из этого удалять нельзя; если строки исчезли — вернуть и разобраться.

- [ ] **Step 5: Коммит**

```bash
cd /c/ResultV
git add internal/proxy/wgdiag.go internal/proxy/wgdiag_test.go go.mod go.sum
git commit -F- <<'MSG'
feat(proxy): счётчики WG-устройства и gVisor-стека

Ядро логирует рукопожатия и keepalive, а данные — нет, поэтому сессия,
переставшая возить трафик при живом с виду устройстве, выглядит в журнале
как полная тишина. Счётчики на это отвечают: tx растёт при неподвижном rx —
пакеты уходят, ответа нет; оба стоят — до устройства они не доезжают, и
виновато то, что стоит над WireGuard. Счётчики стека отвечают на следующий
вопрос: пакеты доехали, а соединения не встают.

Поля UAPI отбираются белым списком: дамп несёт private_key и preshared_key,
а строка попадает в журнал, который пользователя просят кому-то отдать.

Эндпоинт на системном стеке статистики не имеет — эта половина строки просто
не пишется, счётчики устройства от этого не теряются.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
MSG
```

---

### Task 8: Счётчики в журнале телефона

Go-сторона считает, но на Android её никто не зовёт: `coreLogFile`, в который счётчики пишутся на ПК, здесь не существует. Роль стока берёт `AppLog` — тот самый журнал, который пользователь копирует и присылает. Опрос идёт раз в 5 с и только когда включён подробный журнал, иначе диагностическая строка вытеснит из журнала всё остальное.

**Files:**
- Изменяется: `mobile/libbox_awg31.go` (добавить биндинг счётчиков)
- Создаётся: `android/app/src/main/java/com/resultv/android/vpn/WgDiagSampler.kt`
- Изменяется: `android/app/src/main/java/com/resultv/android/vpn/BoxModule.kt` (пуск в `start()`, остановка в `stop()`)
- Изменяется: `android/app/src/main/res/values/strings.xml`, `values-ru/strings.xml`

**Interfaces:**
- Consumes: `proxy.WGDiagLine(adapter.EndpointManager) (string, error)` (задача 7); `SettingsRepository.state.value.logLevel` (значение `"debug"` включает подробный журнал — ряд для него добавляет задача 10).
- Produces: Go `func WGDiagLine(server *libbox.CommandServer) (string, error)` → Kotlin `Mobile.wgDiagLine(server): String`; `WgDiagSampler.start(server)` / `WgDiagSampler.stop()`; строки `log_source_wg`, `log_wg_diag_unavailable`.

- [ ] **Step 1: Добавить биндинг**

В `mobile/libbox_awg31.go`, в конец файла:

```go
// WGDiagLine returns one line of WireGuard device and gVisor stack counters for
// the running session, or an error when there is no WireGuard endpoint to read
// — which is the ordinary case for every other protocol.
//
// Sampling is driven from Kotlin (WgDiagSampler) rather than by a goroutine
// here: the desktop writes these into its core log file, and on Android the
// only sink that reaches the user is the in-app log, which lives in Kotlin.
func WGDiagLine(server *libbox.CommandServer) (string, error) {
	if server == nil {
		return "", fmt.Errorf("счётчики WG: сервер ядра не передан")
	}
	instance := server.Instance()
	if instance == nil || instance.Box() == nil {
		return "", fmt.Errorf("счётчики WG: ядро не запущено")
	}
	return proxy.WGDiagLine(instance.Box().Endpoint())
}
```

- [ ] **Step 2: Проверить, что Go собирается, и пересобрать AAR**

```bash
cd /c/ResultV
go build -tags="${TAGS_FULL}" ./... && go build -tags="${TAGS_PLAY}" ./... && echo BOTH BUILD OK
DIST=full ./scripts/build-android-aar.sh > /tmp/aar-full.log 2>&1; tail -3 /tmp/aar-full.log
cd /tmp/aarcheck && rm -f classes.jar && unzip -o -q /c/ResultV/android/libs/libbox-full.aar classes.jar
javap -cp classes.jar mobile.Mobile | grep -iE 'awg|wgDiag'
```

Ожидается: среди методов есть `wgDiagLine(libbox.CommandServer)`. Имя именно такое: gomobile гасит ведущую цепочку заглавных, возвращая последнюю (`WGDiagLine` → `wgDiagLine`), как `UnsupportedAWGKnobs` → `unsupportedAWGKnobs`.

- [ ] **Step 3: Написать сэмплер**

Создать `android/app/src/main/java/com/resultv/android/vpn/WgDiagSampler.kt`:

```kotlin
package com.resultv.android.vpn

import android.util.Log
import com.resultv.android.R
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import libbox.CommandServer
import mobile.Mobile

/**
 * Пишет в журнал счётчики WireGuard-устройства и его gVisor-стека, пока жив
 * туннель и включён подробный журнал.
 *
 * Зачем: ядро логирует рукопожатия и keepalive, а данные — нет. Сессия,
 * переставшая возить трафик при живом с виду устройстве, выглядит в журнале
 * полной тишиной, и отличить «пакеты не дошли до устройства» от «дошли, но
 * соединения не встают» больше нечем.
 *
 * Только при подробном журнале: строка длинная и идёт раз в пять секунд — в
 * обычной сессии она вытеснила бы из журнала всё остальное.
 */
internal object WgDiagSampler {

    private const val TAG = "WgDiagSampler"
    private const val INTERVAL_MS = 5_000L

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private var job: Job? = null

    @Synchronized
    fun start(server: CommandServer) {
        stop()
        if (SettingsRepository.state.value.logLevel != "debug") return
        job = scope.launch {
            while (isActive) {
                delay(INTERVAL_MS)
                val line = try {
                    Mobile.wgDiagLine(server)
                } catch (t: Throwable) {
                    // Сообщается один раз и на этом всё: у не-WireGuard узла
                    // считать нечего, а повтор каждые пять секунд похоронил бы
                    // тот самый журнал, ради которого это заведено.
                    Log.i(TAG, "counters unavailable: ${t.message}")
                    AppLog.info(
                        R.string.log_wg_diag_unavailable,
                        t.message ?: t.javaClass.simpleName,
                        source = AppLog.resolve(R.string.log_source_wg),
                    )
                    return@launch
                }
                AppLog.info(line, source = AppLog.resolve(R.string.log_source_wg))
            }
        }
    }

    @Synchronized
    fun stop() {
        job?.cancel()
        job = null
    }
}
```

- [ ] **Step 4: Завести сэмплер от жизненного цикла ядра**

В `BoxModule.kt`:
- в `start()`, сразу после `commandServer = server`: `WgDiagSampler.start(server)`;
- в `stop()`, первой строкой после `val server = commandServer ?: return`: `WgDiagSampler.stop()`.

Reload сэмплер не трогает: `CommandServer` тот же объект, а эндпоинт он ищет заново на каждом опросе.

- [ ] **Step 5: Добавить строки**

`values/strings.xml`, рядом с остальными `log_source_*`:

```xml
    <string name="log_source_wg" translatable="false">WG</string>
```

и рядом с `log_awg31_failed`:

```xml
    <string name="log_wg_diag_unavailable">WireGuard counters unavailable: %1$s. Sampling stopped for this session.</string>
```

`values-ru/strings.xml`:

```xml
    <string name="log_wg_diag_unavailable">Счётчики WireGuard недоступны: %1$s. Опрос в этой сессии остановлен.</string>
```

(`log_source_wg` не переводится — как и остальные метки источника, он лежит только в английском файле.)

- [ ] **Step 6: Собрать и прогнать Kotlin-тесты**

```bash
cd /c/ResultV/android
./gradlew :app:assembleFullDebug -Pdebug.abi=arm64-v8a 2>&1 | tail -5
./gradlew :app:testFullDebugUnitTest :app:testPlayDebugUnitTest 2>&1 | tail -10
```

Ожидается: BUILD SUCCESSFUL. Тест `SettingsStringsTest` смотрит только префикс `settings_`, так что новые `log_*` его не задевают.

- [ ] **Step 7: Коммит**

```bash
cd /c/ResultV
git add mobile/libbox_awg31.go \
        android/app/src/main/java/com/resultv/android/vpn/WgDiagSampler.kt \
        android/app/src/main/java/com/resultv/android/vpn/BoxModule.kt \
        android/app/src/main/res/values/strings.xml android/app/src/main/res/values-ru/strings.xml
git commit -F- <<'MSG'
feat(android): счётчики WireGuard пишутся в журнал телефона

На ПК они идут в диагностический лог ядра; на Android такого файла нет вовсе,
и единственный сток, доходящий до человека, — журнал приложения, а он живёт в
Kotlin. Поэтому Go отдаёт строку, а опрашивает её Kotlin.

Раз в пять секунд и только при включённом подробном журнале: строка длинная, и
в обычной сессии она вытеснила бы из журнала всё остальное. Недоступность
счётчиков сообщается один раз — у не-WireGuard узла считать нечего, а повтор
каждые пять секунд похоронил бы тот самый журнал.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
MSG
```

---

### Task 9: MTU WireGuard и стек TUN — Go-сторона

MTU — единственный параметр WireGuard, отказ по которому не виден изнутри: маленькое проходит, большое теряется, туннель выглядит живым и не везёт ничего. Проверяется это одним прогоном на меньшем MTU, а узел приезжает из подписки, где его руками не поправить. Стек TUN — вторая половина той же диагностики: приёмка блока 1 показала, что на `sing-tun 0.9` системный стек на телефоне перестал обслуживать TCP, и сравнить стеки было нечем.

На ПК оба переключаются переменными окружения. На Android переменные мертвы: Go копирует `environ` при загрузке `.so`, а `Os.setenv` из Kotlin случается позже. Поэтому оба едут полями `BuildOptions` — тем же путём, что и остальные настройки движка.

**Files:**
- Изменяется: `internal/proxy/endpoints.go` (`buildEndpoints` получает третий параметр; новая `wireguardMTU`)
- Изменяется: `internal/proxy/engine.go` (поля `WGMTU` / `TunStack` в `EngineConfig`; `effectiveTunStack`; два вызова `buildEndpoints`; выбор стека в `BuildTunnelModeConfig`)
- Изменяется: `mobile/libbox.go` (два поля `BuildOptions`, проброс в `EngineConfig`)
- Создаётся: `internal/proxy/endpoints_mtu_test.go`
- Создаётся: `internal/proxy/engine_tunstack_test.go`

**Interfaces:**
- Consumes: `EngineConfig`, `BuildTunnelModeConfig`, `buildEndpoints`.
- Produces: `wireguardMTU(configured, override int) int`; `effectiveTunStack(stack string) string` (дефолт `gvisor`); поля `EngineConfig.WGMTU int` и `EngineConfig.TunStack string`; поля `BuildOptions.WGMTU int` (`json:"wgMtu,omitempty"`) и `BuildOptions.TunStack string` (`json:"tunStack,omitempty"`).

- [ ] **Step 1: Написать падающие тесты**

Создать `internal/proxy/endpoints_mtu_test.go`:

```go
// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import "testing"

func TestWireGuardMTUOverride(t *testing.T) {
	cases := []struct {
		name       string
		override   int
		configured int
		want       int
	}{
		{"без переопределения берётся значение узла", 0, 1408, 1408},
		{"переопределение перекрывает узел", 1280, 1408, 1280},
		{"работает и при нулевом значении узла", 1280, 0, 1280},
		// Below the IPv4 minimum a path is not required to carry anything, and
		// above 1500 the override would create the very problem it exists to
		// test for.
		{"слишком маленькое игнорируется", 500, 1408, 1408},
		{"слишком большое игнорируется", 9000, 1408, 1408},
		{"граница снизу принимается", 576, 1408, 576},
		{"граница сверху принимается", 1500, 1408, 1500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := wireguardMTU(tc.configured, tc.override); got != tc.want {
				t.Errorf("wireguardMTU(%d, %d) = %d, ожидалось %d",
					tc.configured, tc.override, got, tc.want)
			}
		})
	}
}

// Переопределение должно доезжать до конфига, а не только до функции.
func TestWireGuardMTUReachesEndpoint(t *testing.T) {
	entry := ProxyConfig{
		IP: "1.2.3.4", Port: 51820, Type: "WIREGUARD",
		Extra: []byte(`{"address":["10.0.0.2/32"],"private_key":"UFJJVg","public_key":"UFVC","allowed_ips":["0.0.0.0/0"],"mtu":1408}`),
	}
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{Proxy: entry, Mode: ProxyModeTunnel, WGMTU: 1280})
	if len(cfg.Endpoints) != 1 {
		t.Fatalf("ожидался один эндпоинт, получено %d", len(cfg.Endpoints))
	}
	if cfg.Endpoints[0].MTU != 1280 {
		t.Errorf("MTU эндпоинта = %d, ожидалось 1280", cfg.Endpoints[0].MTU)
	}
	assertCoreAcceptsConfig(t, cfg)
}
```

Создать `internal/proxy/engine_tunstack_test.go`:

```go
// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import "testing"

// Дефолт на Android — gvisor, и это не вкусовщина: на sing-tun 0.9 системный
// стек на телефоне перестал обслуживать TCP (приёмка блока 1, 11.1 спека).
// Переключатель нужен, чтобы это можно было сравнить, а не чтобы выбирать.
func TestEffectiveTunStack(t *testing.T) {
	cases := []struct {
		name     string
		selected string
		want     string
	}{
		{"пусто — дефолт gvisor", "", "gvisor"},
		{"system выбирается явно", "system", "system"},
		{"gvisor выбирается явно", "gvisor", "gvisor"},
		{"регистр и пробелы не важны", "  SYSTEM ", "system"},
		{"неизвестное значение падает в дефолт", "mystack", "gvisor"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := effectiveTunStack(tc.selected); got != tc.want {
				t.Errorf("effectiveTunStack(%q) = %q, ожидалось %q", tc.selected, got, tc.want)
			}
		})
	}
}

// И выбор должен доезжать до TUN-инбаунда, а не только до функции.
func TestTunStackReachesInbound(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy:    ProxyConfig{IP: "1.2.3.4", Port: 443, Type: "VLESS", Password: "p"},
		Mode:     ProxyModeTunnel,
		TunStack: "system",
	})
	if len(cfg.Inbounds) == 0 {
		t.Fatal("нет инбаундов")
	}
	if cfg.Inbounds[0].Stack != "system" {
		t.Errorf("стек TUN = %q, ожидалось system", cfg.Inbounds[0].Stack)
	}
	assertCoreAcceptsConfig(t, cfg)
}
```

- [ ] **Step 2: Убедиться, что тесты падают**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run 'TestWireGuardMTU|TestEffectiveTunStack|TestTunStackReaches' 2>&1 | tail -10
```

Ожидается: `undefined: wireguardMTU`, `undefined: effectiveTunStack`, неизвестные поля `WGMTU` / `TunStack`.

- [ ] **Step 3: Переопределение MTU**

В `internal/proxy/endpoints.go`: сменить сигнатуру на `func buildEndpoints(proxy ProxyConfig, domainResolver string, mtuOverride int) []SBEndpoint`, заменить `MTU: intFromExtra(extra, "mtu", "MTU"),` на `MTU: wireguardMTU(intFromExtra(extra, "mtu", "MTU"), mtuOverride),` и добавить рядом с `wireguardEndpointTag`:

```go
// wireguardMTU returns the endpoint MTU, letting the user's override replace
// what the node's config asked for.
//
// The override exists because MTU is the one WireGuard parameter whose failure
// mode is invisible from the inside: small packets pass, large ones are dropped
// somewhere on the path, and the tunnel looks alive while carrying nothing.
// Answering "is it the packet size" needs one run at a smaller MTU, and the
// node arrives from a subscription, where it cannot be edited by hand.
//
// Out-of-range values are ignored rather than clamped: 576 is the IPv4 minimum
// any path must carry, and above 1500 the override would create the very
// problem it is meant to test for.
func wireguardMTU(configured, override int) int {
	if override < 576 || override > 1500 {
		return configured
	}
	return override
}
```

- [ ] **Step 4: Выбор стека TUN**

В `internal/proxy/engine.go`:
- в `EngineConfig` добавить два поля с комментариями:

```go
	// WGMTU overrides the MTU a WireGuard/AmneziaWG node asked for. Zero means
	// "use the node's own value". Diagnostic: see wireguardMTU for why the
	// failure it exists to find is invisible from the inside.
	WGMTU int
	// TunStack picks the TUN inbound stack ("gvisor" / "system"). Empty means
	// gvisor, which is what Android has always run — and on sing-tun 0.9 the
	// system stack stopped serving TCP on the phone altogether. The switch is
	// here to make that comparable, not to offer a choice.
	TunStack string
```

- заменить `tunStack := "gvisor"` на `tunStack := effectiveTunStack(cfg.TunStack)`;
- оба вызова `buildEndpoints(cfg.Proxy, nodeResolver)` — на `buildEndpoints(cfg.Proxy, nodeResolver, cfg.WGMTU)`;
- добавить рядом с `BuildTunnelModeConfig`:

```go
// effectiveTunStack resolves which TUN stack to run.
//
// Unlike the desktop, the default here is gvisor, not system: that is what
// Android has always run, and the acceptance of the 1.14 bump showed why it
// must stay so — on sing-tun 0.9 the system stack stopped opening TCP on the
// phone at all, with DNS and QUIC still alive. An unknown value falls back to
// the default rather than being passed to the core, which would refuse the
// whole config.
func effectiveTunStack(stack string) string {
	switch strings.ToLower(strings.TrimSpace(stack)) {
	case "system":
		return "system"
	default:
		return "gvisor"
	}
}
```

- [ ] **Step 5: Пробросить оба поля через BuildOptions**

В `mobile/libbox.go`, в `BuildOptions`, после `RoutingOrder`:

```go
	// WGMTU overrides the MTU a WireGuard/AmneziaWG node asked for; zero means
	// the node's own value. Diagnostic knob — the desktop has it as an
	// environment variable, which on Android is dead: Go copies environ when
	// the .so loads, long before Kotlin could set anything.
	WGMTU int `json:"wgMtu,omitempty"`
	// TunStack picks the TUN inbound stack ("gvisor" / "system"); empty means
	// gvisor. Same story as WGMTU, and the same reason it is a field.
	TunStack string `json:"tunStack,omitempty"`
```

и в `buildSingBoxConfigFromEntry`, в литерал `proxy.EngineConfig{…}`:

```go
		WGMTU:    opts.WGMTU,
		TunStack: opts.TunStack,
```

- [ ] **Step 6: Тесты проходят в обеих конфигурациях**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run 'TestWireGuardMTU|TestTunStack|TestEffectiveTunStack' -v 2>&1 | tail -25
go test -tags="${TAGS_FULL}" ./... 2>&1 | tail -10
go test -tags="${TAGS_PLAY}" ./... 2>&1 | tail -10
```

Ожидается: все PASS, `ok` по пакетам в обеих конфигурациях. Если `engine_tun_1_14_test.go` или `wg_route_exclude_test.go` знали прежнюю сигнатуру `buildEndpoints` — поправить вызовы, это механика.

- [ ] **Step 7: Коммит**

```bash
cd /c/ResultV
git add internal/proxy/endpoints.go internal/proxy/engine.go internal/proxy/endpoints_mtu_test.go \
        internal/proxy/engine_tunstack_test.go mobile/libbox.go
git commit -F- <<'MSG'
feat(proxy): MTU WireGuard и стек TUN переключаются настройкой

MTU — единственный параметр WireGuard, отказ по которому не виден изнутри:
маленькое проходит, большое теряется, туннель выглядит живым и не везёт
ничего. Проверяется это одним прогоном на меньшем MTU, а узел приезжает из
подписки, где его руками не поправить. Стек TUN — вторая половина той же
диагностики: приёмка блока 1 показала, что на sing-tun 0.9 системный стек на
телефоне перестал обслуживать TCP, и сравнить стеки было нечем.

На ПК оба переключаются переменными окружения. На Android переменные мертвы —
Go копирует environ при загрузке .so, а Os.setenv из Kotlin случается позже,
— поэтому оба едут полями BuildOptions, тем же путём, что остальные
настройки движка.

Дефолт стека остаётся gvisor, и неизвестное значение падает в него же:
передать ядру мусор значило бы уронить весь конфиг.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
MSG
```

---

### Task 10: Диагностика в настройках

Три ряда в шторке «Сеть»: подробный журнал, стек TUN, MTU WireGuard. Подробный журнал уже есть в состоянии (`SettingsState.logLevel`), но ряда под него никогда не было — а без него сэмплер счётчиков из задачи 8 включить нечем.

Все три применяются при следующем подключении: `reloadWatcher` следит только за `settings.adblock`, остальные настройки, включая IPv6 и bypass LAN, ведут себя так же.

**Files:**
- Изменяется: `android/app/src/main/java/com/resultv/android/vpn/SettingsRepository.kt`
- Изменяется: `android/app/src/main/java/com/resultv/android/vpn/BuildOptions.kt`
- Изменяется: `android/app/src/main/java/com/resultv/android/ui/screens/SettingsScreen.kt` (`NetworkGroup`)
- Изменяется: `android/app/src/main/res/values/strings.xml`, `values-ru/strings.xml`
- Изменяется: `android/app/src/test/java/com/resultv/android/vpn/` — новый тест на нормализацию MTU

**Interfaces:**
- Consumes: `SettingsRepository.state`, `ToggleRow`, `SettingIcon`, `RvCategory`, `RvColor`, `FilterChip` — всё уже используется в `NetworkGroup`.
- Produces: `SettingsState.tunStack: String` (`""` = авто/gvisor), `SettingsState.wgMtu: Int` (0 = из профиля), `SettingsRepository.setTunStack(String)`, `SettingsRepository.setWgMtu(Int)`, `SettingsRepository.setLogLevel(String)` (уже есть), `SettingsRepository.normalizeWgMtu(String): Int`; поля `tunStack` / `wgMtu` в `optionsJson`.

- [ ] **Step 1: Написать падающий тест нормализации**

Создать `android/app/src/test/java/com/resultv/android/vpn/WgMtuInputTest.kt`:

```kotlin
package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Поле MTU — свободный ввод, а значение уезжает в движок. Границы те же, что
 * в Go (576..1500): ниже — меньше минимума, который обязан нести любой путь
 * IPv4, выше — переопределение само создаст ту проблему, ради проверки которой
 * заведено. Пустое и негодное читаются как «из профиля», а не как ноль.
 */
class WgMtuInputTest {

    @Test
    fun `empty means take it from the profile`() {
        assertEquals(0, SettingsRepository.normalizeWgMtu(""))
        assertEquals(0, SettingsRepository.normalizeWgMtu("   "))
    }

    @Test
    fun `garbage is not a number`() {
        assertEquals(0, SettingsRepository.normalizeWgMtu("тысяча"))
        assertEquals(0, SettingsRepository.normalizeWgMtu("12a8"))
    }

    @Test
    fun `out of range is ignored`() {
        assertEquals(0, SettingsRepository.normalizeWgMtu("500"))
        assertEquals(0, SettingsRepository.normalizeWgMtu("9000"))
    }

    @Test
    fun `boundaries are accepted`() {
        assertEquals(576, SettingsRepository.normalizeWgMtu("576"))
        assertEquals(1500, SettingsRepository.normalizeWgMtu("1500"))
        assertEquals(1280, SettingsRepository.normalizeWgMtu(" 1280 "))
    }
}
```

- [ ] **Step 2: Убедиться, что тест падает**

```bash
cd /c/ResultV/android && ./gradlew :app:testFullDebugUnitTest --tests '*WgMtuInputTest*' 2>&1 | tail -10
```

Ожидается: не компилируется — `normalizeWgMtu` не существует.

- [ ] **Step 3: Состояние и сеттеры**

В `SettingsRepository.kt`:
- в `SettingsState` после `logLevel`:

```kotlin
    /**
     * Стек TUN-инбаунда: "" (как всегда, gvisor) или "system". Диагностика:
     * на sing-tun 0.9 системный стек на телефоне перестал обслуживать TCP, и
     * сравнить их было нечем.
     */
    val tunStack: String = "",
    /** Переопределение MTU WireGuard-узла; 0 — значение из профиля. */
    val wgMtu: Int = 0,
```

- ключи рядом с `K_LOG_LEVEL`:

```kotlin
    private const val K_TUN_STACK = "tun_stack"
    private const val K_WG_MTU = "wg_mtu"
```

- чтение в `init` рядом с `logLevel`:

```kotlin
            tunStack = prefs.getString(K_TUN_STACK, "") ?: "",
            wgMtu = prefs.getInt(K_WG_MTU, 0),
```

- сеттеры рядом с `setLogLevel`:

```kotlin
    fun setTunStack(stack: String) = mutate {
        val sane = if (stack == "system") "system" else ""
        prefs.edit().putString(K_TUN_STACK, sane).apply()
        it.copy(tunStack = sane)
    }

    fun setWgMtu(mtu: Int) = mutate {
        val sane = if (mtu in 576..1500) mtu else 0
        prefs.edit().putInt(K_WG_MTU, sane).apply()
        it.copy(wgMtu = sane)
    }

    /**
     * Прочитать введённое значение MTU. Границы те же, что в Go
     * (internal/proxy/endpoints.go, wireguardMTU): ниже 576 путь IPv4 не
     * обязан нести ничего, выше 1500 переопределение само создаст ту проблему,
     * ради проверки которой заведено. Негодное читается как «из профиля».
     */
    fun normalizeWgMtu(raw: String): Int {
        val n = raw.trim().toIntOrNull() ?: return 0
        return if (n in 576..1500) n else 0
    }
```

- [ ] **Step 4: Тест проходит**

```bash
cd /c/ResultV/android && ./gradlew :app:testFullDebugUnitTest --tests '*WgMtuInputTest*' 2>&1 | tail -5
```

Ожидается: BUILD SUCCESSFUL.

- [ ] **Step 5: Пробросить в optionsJson**

В `BuildOptions.kt`, в конец цепочки `.put(...)` перед `.toString()`:

```kotlin
            // Диагностика движка. Оба применяются при следующем подключении:
            // reloadWatcher следит только за ad-block, остальные настройки
            // доезжают на реконнекте.
            .put("tunStack", settings.tunStack)
            .put("wgMtu", settings.wgMtu)
```

- [ ] **Step 6: Добавить строки**

`values/strings.xml`, рядом с `settings_ipv6_subtitle`:

```xml
    <string name="settings_diagnostics">Diagnostics</string>
    <string name="settings_verbose_log">Verbose log</string>
    <string name="settings_verbose_log_subtitle">Engine debug level plus WireGuard counters</string>
    <string name="settings_tun_stack">TUN stack</string>
    <string name="settings_tun_stack_subtitle">gvisor is the default; system only for comparison</string>
    <string name="settings_tun_stack_auto">Default</string>
    <string name="settings_wg_mtu">WireGuard MTU</string>
    <string name="settings_wg_mtu_subtitle">576-1500; empty takes it from the profile</string>
    <string name="settings_wg_mtu_placeholder" translatable="false">1280</string>
```

`values-ru/strings.xml`:

```xml
    <string name="settings_diagnostics">Диагностика</string>
    <string name="settings_verbose_log">Подробный журнал</string>
    <string name="settings_verbose_log_subtitle">Отладочный уровень движка и счётчики WireGuard</string>
    <string name="settings_tun_stack">Стек TUN</string>
    <string name="settings_tun_stack_subtitle">По умолчанию gvisor; system — только для сравнения</string>
    <string name="settings_tun_stack_auto">По умолчанию</string>
    <string name="settings_wg_mtu">MTU WireGuard</string>
    <string name="settings_wg_mtu_subtitle">576-1500; пусто — значение из профиля</string>
```

Длина подписей: `SettingsStringsTest` держит потолок в 64 символа для `_subtitle` / `_desc` / `_hint`. Проверить обе локали перед коммитом.

- [ ] **Step 7: Три ряда в шторке «Сеть»**

В `SettingsScreen.kt`, в конец `NetworkGroup` (после ряда IPv6):

```kotlin
    HorizontalDivider(color = RvColor.whiteA10, modifier = Modifier.padding(vertical = RvSpace.nest3))
    Text(
        stringResource(R.string.settings_diagnostics),
        style = MaterialTheme.typography.titleSmall,
        color = RvColor.whiteA50,
    )
    ToggleRow(
        title = stringResource(R.string.settings_verbose_log),
        subtitle = stringResource(R.string.settings_verbose_log_subtitle),
        icon = Icons.Outlined.BugReport,
        tint = RvCategory.Amber,
        checked = settings.logLevel == "debug",
        onCheckedChange = { SettingsRepository.setLogLevel(if (it) "debug" else "info") },
    )
    HorizontalDivider(color = RvColor.whiteA10)
    Column(
        modifier = Modifier.padding(vertical = RvSpace.nest3),
        verticalArrangement = Arrangement.spacedBy(RvSpace.nest3),
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2),
        ) {
            SettingIcon(Icons.Outlined.Layers, RvCategory.Amber)
            Column {
                Text(stringResource(R.string.settings_tun_stack), style = MaterialTheme.typography.bodyLarge)
                Text(
                    stringResource(R.string.settings_tun_stack_subtitle),
                    style = MaterialTheme.typography.bodySmall,
                    color = RvColor.whiteA50,
                )
            }
        }
        androidx.compose.foundation.layout.FlowRow(
            modifier = Modifier.padding(start = 50.dp),
            horizontalArrangement = Arrangement.spacedBy(RvSpace.xs),
            verticalArrangement = Arrangement.spacedBy(RvSpace.xs),
        ) {
            listOf(
                "" to stringResource(R.string.settings_tun_stack_auto),
                "system" to "system",
            ).forEach { (key, label) ->
                FilterChip(
                    selected = settings.tunStack == key,
                    onClick = { SettingsRepository.setTunStack(key) },
                    label = { Text(label) },
                    colors = FilterChipDefaults.filterChipColors(
                        selectedContainerColor = RvColor.Main.copy(alpha = 0.2f),
                        selectedLabelColor = RvColor.Second,
                    ),
                )
            }
        }
    }
    HorizontalDivider(color = RvColor.whiteA10)
    Column(
        modifier = Modifier.padding(vertical = RvSpace.nest3),
        verticalArrangement = Arrangement.spacedBy(RvSpace.nest3),
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2),
        ) {
            SettingIcon(Icons.Outlined.Straighten, RvCategory.Amber)
            Column {
                Text(stringResource(R.string.settings_wg_mtu), style = MaterialTheme.typography.bodyLarge)
                Text(
                    stringResource(R.string.settings_wg_mtu_subtitle),
                    style = MaterialTheme.typography.bodySmall,
                    color = RvColor.whiteA50,
                )
            }
        }
        var mtuDraft by rememberSaveable(settings.wgMtu) {
            mutableStateOf(if (settings.wgMtu == 0) "" else settings.wgMtu.toString())
        }
        OutlinedTextField(
            value = mtuDraft,
            onValueChange = {
                mtuDraft = it.filter { ch -> ch.isDigit() }.take(4)
                SettingsRepository.setWgMtu(SettingsRepository.normalizeWgMtu(mtuDraft))
            },
            modifier = Modifier.fillMaxWidth().padding(start = 50.dp),
            singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
            placeholder = { Text(stringResource(R.string.settings_wg_mtu_placeholder)) },
        )
    }
```

Импорты, которых в файле может не быть: `androidx.compose.material.icons.outlined.BugReport`, `Layers`, `Straighten`, `androidx.compose.foundation.text.KeyboardOptions`, `androidx.compose.ui.text.input.KeyboardType`, `androidx.compose.runtime.mutableStateOf`. Иконки сверить с тем, что реально есть в подключённом наборе `material-icons-extended` — если какой-то нет, взять соседнюю по смыслу и не тянуть новую зависимость.

- [ ] **Step 8: Собрать, прогнать тесты, посмотреть глазами**

```bash
cd /c/ResultV/android
./gradlew :app:assembleFullDebug -Pdebug.abi=arm64-v8a 2>&1 | tail -5
./gradlew :app:testFullDebugUnitTest :app:testPlayDebugUnitTest 2>&1 | tail -10
```

Ожидается: BUILD SUCCESSFUL, включая `SettingsStringsTest` (паритет локалей и длина подписей).

Затем поставить на телефон и открыть «Настройки → Сеть»:

```bash
adb -s e3bacc6b install -r -d /c/ResultV/android/app/build/outputs/apk/full/debug/*.apk
adb -s e3bacc6b shell am start -n com.resultv.android/.MainActivity
```

Проверить глазами: блок «Диагностика» на месте, три ряда читаются, чипы переключаются, в поле MTU клавиатура цифровая. Снять скриншот (`adb -s e3bacc6b exec-out screencap -p > /tmp/diag.png`) и приложить к отчёту задачи.

- [ ] **Step 9: Коммит**

```bash
cd /c/ResultV
git add android/app/src/main/java/com/resultv/android/vpn/SettingsRepository.kt \
        android/app/src/main/java/com/resultv/android/vpn/BuildOptions.kt \
        android/app/src/main/java/com/resultv/android/ui/screens/SettingsScreen.kt \
        android/app/src/test/java/com/resultv/android/vpn/WgMtuInputTest.kt \
        android/app/src/main/res/values/strings.xml android/app/src/main/res/values-ru/strings.xml
git commit -F- <<'MSG'
feat(android): блок «Диагностика» в настройках сети

Три ряда: подробный журнал, стек TUN, MTU WireGuard. Подробный журнал уже жил
в состоянии, но ряда под него никогда не было — а без него нечем включить
счётчики WireGuard, которые пишутся только при нём.

Стек и MTU — то, чем на ПК управляют переменные окружения; на телефоне они
мертвы, поэтому это настройки. Оба применяются при следующем подключении, как
IPv6 и bypass LAN: reloadWatcher по-прежнему следит только за ad-block.

Границы MTU те же, что в Go, и в одном месте с ними же обоснованы: ниже 576
путь IPv4 не обязан нести ничего, выше 1500 переопределение само создаст ту
проблему, ради проверки которой заведено.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
MSG
```

---

### Task 11: Приёмка и документация

Блок 2 закрывается не сборкой, а телефоном. Пока приёмка не пройдена, блок 3 не начинается.

**Files:**
- Изменяется: `docs/superpowers/specs/2026-09-17-android-core-1.14-sync-design.md` (новый раздел о закрытии блока 2 + правка §12)
- Изменяется: `docs/android-pc-sync-and-play-spec.md` (раздел 6, статус блока 5)

**Interfaces:**
- Consumes: всё, сделанное задачами 2-10.
- Produces: запись «что получилось и чем доказано», на которую будет опираться блок 3.

- [ ] **Step 1: Обе конфигурации Go — сборка и тесты**

```bash
cd /c/ResultV
go build -tags="${TAGS_FULL}" ./... && echo FULL OK
go build -tags="${TAGS_PLAY}" ./... && echo PLAY OK
go test -tags="${TAGS_FULL}" ./... 2>&1 | tail -20
go test -tags="${TAGS_PLAY}" ./... 2>&1 | tail -20
```

Ожидается: ноль падений в обеих. Сравнить со списком из задачи 1.

- [ ] **Step 2: Оба AAR и выравнивание 16 КБ**

```bash
cd /c/ResultV
DIST=full ./scripts/build-android-aar.sh > /tmp/aar-full.log 2>&1; tail -3 /tmp/aar-full.log
DIST=play ./scripts/build-android-aar.sh > /tmp/aar-play.log 2>&1; tail -3 /tmp/aar-play.log
ls -l android/libs/libbox-full.aar android/libs/libbox-play.aar
for d in full play; do
  rm -rf /tmp/aar-$d && mkdir -p /tmp/aar-$d
  unzip -o -q android/libs/libbox-$d.aar -d /tmp/aar-$d
  echo "=== $d ==="
  readelf -lW /tmp/aar-$d/jni/arm64-v8a/libgojni.so | awk '/LOAD/ {print $NF}' | sort -u
done
```

Ожидается: оба AAR свежие, у всех сегментов LOAD выравнивание `0x4000`.

- [ ] **Step 3: play-граф без `internal/filter`**

```bash
cd /c/ResultV
go list -deps -tags="${TAGS_PLAY}" ./mobile | grep -c 'internal/filter' || echo "0 вхождений — ок"
go list -deps -tags="${TAGS_FULL}" ./mobile | grep -c 'internal/filter'
```

Ожидается: play — 0, full — ненулевое (в блоке 1 было 4).

- [ ] **Step 4: Kotlin-тесты обеих сборок**

```bash
cd /c/ResultV/android && ./gradlew :app:testFullDebugUnitTest :app:testPlayDebugUnitTest 2>&1 | tail -10
```

Ожидается: BUILD SUCCESSFUL, число тестов больше зафиксированного в задаче 1 ровно на добавленные (3.1 в `.conf` — 2, MTU — 4).

- [ ] **Step 5: Телефон, AmneziaWG-узел**

```bash
cd /c/ResultV/android
./gradlew :app:assembleFullDebug -Pdebug.abi=arm64-v8a 2>&1 | tail -3
adb -s e3bacc6b install -r -d app/build/outputs/apk/full/debug/*.apk
adb -s e3bacc6b logcat -c
adb -s e3bacc6b shell am start -n com.resultv.android/.MainActivity
```

Затем руками на телефоне, на рабочем AmneziaWG-профиле:

| Проверка | Чем подтверждается |
|---|---|
| Подключение к AWG-узлу живо | `curl` через туннель (или открытая страница) и `adb logcat` без `AndroidRuntime:E` |
| Ключи 3.1 применились | строка журнала «AmneziaWG 3.1: применены random_trailers=…» (если у узла их нет — временно дописать `&RandomTrailers=off` в ссылку профиля и проверить на нём: `off` так же заявлен, как `on`, и так же должен доехать) |
| Рукопожатие прошло | в логе ядра `endpoint/wireguard[proxy]` и ответ DNS через туннель |
| Пинг WG работает | строка сервера показывает задержку, а не причину отказа |
| Счётчики пишутся | включить «Подробный журнал», переподключиться, увидеть в журнале строки `wg-stats …` и `wg-stack …` раз в 5 с; выключить — строки прекращаются после переподключения |
| В счётчиках нет ключей | глазами: в строке нет `private_key` / `preshared_key` |
| MTU доезжает | поставить 1280, переподключиться, убедиться, что туннель поднимается и возит трафик |
| Стек TUN доезжает | переключить на `system`, переподключиться; ожидаемо TCP умирает (это и есть находка 11.1) — вернуть на «По умолчанию» и убедиться, что всё вернулось |
| VLESS не сломан | подключиться к VLESS-профилю: счётчики сообщают о недоступности один раз, всё остальное работает |

Каждую строку подтверждать выводом команды или скриншотом — «должно работать» в приёмке не считается.

- [ ] **Step 6: Записать результат в спек блока**

В `docs/superpowers/specs/2026-09-17-android-core-1.14-sync-design.md` добавить раздел «13. Блок 2 — что сделано и чем доказано» в том же виде, что разделы 10-11: таблица проверок с фактическими результатами, отдельно — что не проверено и почему. Обязательно записать:

- три развилки и их решения (мост через `CommandServer`; счётчики с адаптацией под Android; переключатели полями `BuildOptions`), с причиной каждой — следующий агент не должен переоткрывать;
- факт, что `internal/proxy/singbox.go` на `android` не вызывается никем, и `applyAWG31` с ПК туда ставить бессмысленно;
- что `gomobile bind` разрешает параметр из соседнего связанного пакета — это шов, которым можно пользоваться и дальше;
- чего в блоке 2 не делалось: воспроизводитель отказа AWG под тегом `awgrepro` (`abb94d2`) не переносился — он поднимает узел в режиме прокси, которого на Android нет.

В §12.2 вычеркнуть блок 2 из «осталось», в таблицу блоков (§0 основного документа) — статус «закрыт».

- [ ] **Step 7: Обновить основной документ**

В `docs/android-pc-sync-and-play-spec.md`: в таблице раздела 0 отметить блок 2 закрытым, в разделе 6 добавить подраздел «6.2 Блок 2 закрыт» на 10-15 строк по образцу 6.1 — что получилось, чем доказано, что осталось живым, и что блок 3 (настройки пинга) начинается отсюда.

- [ ] **Step 8: Коммит документации**

Папка `docs/superpowers` под `.gitignore` — коммитится принудительно, как все прежние.

```bash
cd /c/ResultV
git add -f docs/superpowers/specs/2026-09-17-android-core-1.14-sync-design.md \
           docs/superpowers/plans/2026-09-17-android-awg31-and-diagnostics.md
git add docs/android-pc-sync-and-play-spec.md
git commit -F- <<'MSG'
docs(android): блок 2 закрыт — AWG 3.1, счётчики и два переключателя

Записаны три развилки, которых в дизайне не было, и их причины: на android
нет пути, которым ключи 3.1 доезжают до устройства на ПК, — singbox.go там
не вызывается никем, ядро поднимает libbox. Шов нашёлся в том, что gomobile
получает оба пакета одним вызовом; это годится и для будущих задач.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
MSG
```

- [ ] **Step 9: Доложить и остановиться**

Блок 3 (настройки пинга) в этот план не входит. Его первый шаг — замер, доступен ли на телефоне unprivileged ICMP; до этого замера решение о поведении пикера не принимается.
