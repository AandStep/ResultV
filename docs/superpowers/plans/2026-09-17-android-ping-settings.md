# Блок 5, задача 3 — настройки пинга на Android

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Пользователь выбирает, что именно меряет пинг в списке серверов — сегодняшнюю пробу по протоколу, ICMP или настоящий HTTP-запрос через сам узел, — и задаёт тестовый адрес и таймаут. Отказ пробы объясняется в строке сервера, а не превращается в «Таймаут» на все случаи.

**Architecture:** Общий `internal/config` получает три поля и их валидацию с `dev`. Новый `internal/proxy/ping_engine.go` поднимает одноразовый sing-box с петлевым mixed-инбаундом и узлом в качестве единственного аутбаунда и меряет время до ответа тестового адреса; на Android он проще, чем на ПК (нет кэша пинов сервера, нет `extendedBoxContext`). Диспетчер по типу живёт не в `manager.go` (его на `android` нет), а в биндинге `PingEntry`, который начинает принимать настройки вторым аргументом. Kotlin отдаёт настройки из `SettingsRepository`, показывает раздел «Пинг» и рисует новые причины отказа.

**Tech Stack:** Go 1.26.4, `sing-box-extended v1.14.0-extended-2.7.1`, gomobile v0.1.12, Kotlin/Compose.

**Spec:** `docs/superpowers/specs/2026-09-17-android-core-1.14-sync-design.md`, §6. Решение «показываем все четыре типа, при невозможности пробы строка рисует причину» принято человеком при брейншторме (§3.5), переоткрывать не надо.

## Что уже измерено (2026-09-17, до плана)

| Факт | Чем установлен | Что из этого следует |
|---|---|---|
| **Unprivileged ICMP на телефоне доступен** | `ping_group_range = 0 2147483647`; uid приложения 10488; `run-as com.resultv.android ping 1.1.1.1` → 59 мс; `/system/bin/ping` без setuid и без capabilities | Тип ICMP проектируется как рабочий. Ветка «No ICMP» нужна только на прошивках, где диапазон закрыт |
| `pingICMPHost` уже берёт unprivileged сокет | `SetPrivileged(runtime.GOOS == "windows")` в `ping_icmp.go` | Менять способ не надо, надо добавить бюджет времени |
| **Приложение исключено из собственного VPN** | `BoxModule.kt:380` — `tryDisallow(builder, ownPkg)` в denylist-ветке, и `applySmartAllowlist(builder, ownPkg)` в Smart | Проба «HTTP через узел» на Android **не мерит сама себя** при поднятом туннеле, и гейт `SetTunnelActive` ей не нужен — в отличие от ПК, который для этого пинит dial к физическому адаптеру. Это надо **подтвердить замером** (задача 9), а не принять на веру |
| На ПК тип пинга читает только видимый человеку пинг | комментарий у `config.PingType`: AUTO-sweep и вотчдог сохраняют свою настройку | На Android тип применяется к `PingRepository` (список серверов) и **не** трогает `resolveAutoCandidates` и `KillSwitchWatchdog` |
| Список пингуется по 16 параллельно | `PING_CONCURRENCY = 16` в `PingRepository.kt` | Проба-движок дороже сокета на порядки; нужен свой потолок, как `pingEngineMaxConcurrency = 4` на ПК |

## Global Constraints

- Ветка `android` в `C:\ResultV`. Точка отката — `3bf19e6`.
- Теги: `TAGS_FULL="$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)"`, `TAGS_PLAY="${TAGS_FULL},no_mitm,no_adblock"`. Каждый `go build` / `go test` — в обеих конфигурациях.
- AAR только через `DIST=… ./scripts/build-android-aar.sh`, и **перед сборкой для телефона** экспортировать ключ подписок: `set -a; . /c/ResultV/.env; set +a` (иначе `resultv://` и RVSUB1 не расшифровываются). Успех проверять по строке `🔐 Embedding subscription decryption key` и по времени файла, а не по коду возврата пайплайна.
- **Копированием с ПК нельзя** ни один файл этой задачи: `manager.go`, `serverpin_cache.go`, `ping_resolve.go` на `android` отсутствуют, `extendedBoxContext` приедет только с блоком 4, а `internal/config` разошёлся на 154 строки — переносится **только** то, что относится к пингу.
- Тип пинга не трогает автоподбор и кил-свитч. Если правка задевает `resolveAutoCandidates` или `KillSwitchWatchdog` — она не по этой задаче.
- Никаких попутных улучшений; сообщения коммитов на русском, последняя строка —
  `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>`.
- Не пушить в `origin` до приёмки (задача 9).

## Карта файлов

Создаются:

| Файл | За что отвечает |
|---|---|
| `internal/proxy/ping_engine.go` | конфиг одноразового движка + замер HTTP через узел + потолок одновременных движков |
| `internal/proxy/ping_engine_test.go` | форма конфига, отказ на WG/AWG, классификация ответа, потолок |
| `internal/config/config_ping_test.go` | границы таймаута и валидация тестового адреса |
| `android/app/src/test/java/com/resultv/android/vpn/PingOptionsTest.kt` | нормализация настроек пинга на стороне Kotlin |

Изменяются:

| Файл | Что меняется |
|---|---|
| `internal/config/config.go` | три поля `AppSettings`, константы типов, `DefaultPingTestURL`, границы, `ValidatePingTestURL`, `EffectivePingType`, `EffectivePingTestURL`, `EffectivePingTimeoutSec` |
| `internal/proxy/ping_icmp.go` | бюджет времени параметром (`pingICMPHostTimeout`) |
| `mobile/libbox.go` | `PingEntry` принимает вторым аргументом JSON настроек; диспетчер по типу |
| `android/.../vpn/PingRepository.kt` | отдаёт настройки в биндинг |
| `android/.../vpn/SettingsRepository.kt` | три поля состояния и сеттеры |
| `android/.../ui/screens/SettingsScreen.kt` | раздел «Пинг» в шторке «Сеть» |
| `android/.../ui/components/ServerRow.kt` | новые причины в `offlineLabel` |
| `android/app/src/main/res/values{,-ru}/strings.xml` | подписи раздела и причин |
| оба документа блока | запись о закрытии |

---

### Task 1: Зелёная база

**Files:** ничего, если база зелёная.

**Interfaces:** Produces — зафиксированные числа «до».

- [ ] **Step 1: Прогнать всё**

```bash
cd /c/ResultV
TAGS_FULL="$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)"
TAGS_PLAY="${TAGS_FULL},no_mitm,no_adblock"
go build -tags="${TAGS_FULL}" ./... && go build -tags="${TAGS_PLAY}" ./... && echo BUILD OK
go test -tags="${TAGS_FULL}" ./... 2>&1 | grep -E "^(ok|FAIL|---)"
go test -tags="${TAGS_PLAY}" ./... 2>&1 | grep -E "^(ok|FAIL|---)"
cd android && ./gradlew :app:testFullDebugUnitTest :app:testPlayDebugUnitTest 2>&1 | tail -5
```

Ожидается: 9 пакетов `ok` в обеих конфигурациях; Kotlin full 137 / play 135 (числа на конец блока 2). Расхождение — записать сюда и дальше сверяться с ним.

---

### Task 2: Поля пинга в общем конфиге

`internal/config` общий с ПК, но деревья разошлись на 154 строки, поэтому переносится только блок про пинг — дословно с `dev`, чтобы у двух клиентов совпадали и имена ключей JSON, и границы, и текст ошибок.

**Files:**
- Изменяется: `internal/config/config.go` (поля в `AppSettings` рядом с `LastChangelogVersion`; константы и функции — в конец файла)
- Создаётся: `internal/config/config_ping_test.go`

**Interfaces:**
- Produces: `AppSettings.PingType/PingTestURL/PingTimeoutSec`; `config.PingTypeAuto/PingTypeICMP/PingTypeHTTPGet/PingTypeHTTPHead`; `config.DefaultPingTestURL`; `ValidatePingTestURL(string) error`; `(AppSettings).EffectivePingType() string`, `EffectivePingTestURL() string`, `EffectivePingTimeoutSec() int`.

- [ ] **Step 1: Написать падающий тест**

Создать `internal/config/config_ping_test.go`:

```go
// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package config

import "testing"

// Пустой тип — это «авто», а не ошибка: так читается любой конфиг, написанный
// до появления поля.
func TestEffectivePingTypeFallsBackToAuto(t *testing.T) {
	for _, raw := range []string{"", "  ", "нечто"} {
		if got := (AppSettings{PingType: raw}).EffectivePingType(); got != PingTypeAuto {
			t.Errorf("EffectivePingType(%q) = %q, ожидалось %q", raw, got, PingTypeAuto)
		}
	}
	if got := (AppSettings{PingType: PingTypeHTTPHead}).EffectivePingType(); got != PingTypeHTTPHead {
		t.Errorf("заданный тип потерялся: %q", got)
	}
}

// HTTPS — требование корректности, а не вкуса: по plain-HTTP запрос идёт через
// наш же петлевой инбаунд, который на мёртвый узел отвечает собственным
// 502, и мёртвый узел прочитался бы как живой.
func TestValidatePingTestURL(t *testing.T) {
	bad := map[string]string{
		"":                             "empty",
		"   ":                          "empty",
		"http://example.com/generate_204": "https",
		"https://":                      "host",
		"://":                           "URL",
	}
	for raw := range bad {
		if err := ValidatePingTestURL(raw); err == nil {
			t.Errorf("%q должен быть отклонён", raw)
		}
	}
	if err := ValidatePingTestURL(DefaultPingTestURL); err != nil {
		t.Errorf("дефолтный адрес должен проходить: %v", err)
	}
}

// Границы: ноль значит «дефолт», а не «не ждать вовсе»; потолок не даёт одному
// мёртвому узлу растянуть весь обход списка.
func TestEffectivePingTimeoutSec(t *testing.T) {
	cases := map[int]int{0: 3, -5: 3, 1: 1, 3: 3, 10: 10, 11: 3, 1000: 3}
	for in, want := range cases {
		if got := (AppSettings{PingTimeoutSec: in}).EffectivePingTimeoutSec(); got != want {
			t.Errorf("EffectivePingTimeoutSec(%d) = %d, ожидалось %d", in, got, want)
		}
	}
}

// Пустой адрес читается как дефолтный — иначе http-тип у свежего конфига
// просто не работал бы.
func TestEffectivePingTestURL(t *testing.T) {
	if got := (AppSettings{}).EffectivePingTestURL(); got != DefaultPingTestURL {
		t.Errorf("пустой адрес = %q, ожидался дефолт", got)
	}
	const custom = "https://example.org/ping"
	if got := (AppSettings{PingTestURL: custom}).EffectivePingTestURL(); got != custom {
		t.Errorf("заданный адрес потерялся: %q", got)
	}
	// Негодный адрес не должен становиться тихим отказом на стороне движка:
	// читатель возвращает дефолт, а несогласие ловит валидация в UI.
	if got := (AppSettings{PingTestURL: "http://insecure"}).EffectivePingTestURL(); got != DefaultPingTestURL {
		t.Errorf("негодный адрес = %q, ожидался дефолт", got)
	}
}
```

- [ ] **Step 2: Убедиться, что тест падает**

```bash
go test -tags="${TAGS_FULL}" ./internal/config/ -run 'TestEffectivePing|TestValidatePingTestURL' 2>&1 | head -8
```

Ожидается: `undefined: PingTypeAuto` и соседние.

- [ ] **Step 3: Перенести блок про пинг**

Взять с `dev` **дословно** (`cd /c/ResultVPC && git show dev:internal/config/config.go`) четыре куска: три поля `AppSettings` (`PingType`, `PingTestURL`, `PingTimeoutSec`) с их комментариями; блок констант `PingType*`; `DefaultPingTestURL`; границы `minPingTimeoutSec`/`maxPingTimeoutSec`/`defaultPingTimeoutSec`; функции `ValidatePingTestURL`, `EffectivePingType`, `EffectivePingTestURL`, `EffectivePingTimeoutSec`.

Комментарий у `PingType` на ПК говорит «только ручной пинг читает это». На Android то же самое означает `PingRepository`, то есть список серверов; автоподбор и кил-свитч тип не читают. Поправить комментарий одной фразой, назвав android-эквивалент, — иначе следующий читатель будет искать `manager.go`.

Проверить импорты: `errors`, `fmt`, `net/url`, `strings` — часть из них в android-версии файла уже есть.

- [ ] **Step 4: Тесты проходят**

```bash
go test -tags="${TAGS_FULL}" ./internal/config/ -v -run 'TestEffectivePing|TestValidatePingTestURL' 2>&1 | grep -E "^(--- |ok|FAIL)"
go test -tags="${TAGS_PLAY}" ./internal/config/ 2>&1 | tail -2
```

- [ ] **Step 5: Коммит**

```bash
git add internal/config/config.go internal/config/config_ping_test.go
git commit -F- <<'MSG'
feat(config): настройки пинга — тип, тестовый адрес, таймаут

Переносится с ПК дословно, чтобы у двух клиентов совпадали имена ключей,
границы и тексты ошибок: конфиг общий, и расхождение здесь означало бы, что
один и тот же файл настроек читается двумя клиентами по-разному.

HTTPS у тестового адреса — требование корректности, а не вкуса: по plain-HTTP
запрос идёт через наш же петлевой инбаунд, который на мёртвый узел отвечает
собственным 502, и мёртвый узел прочитался бы как живой.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
MSG
```

---

### Task 3: ICMP-проба с бюджетом

Тип «ICMP» упирается в то, что сегодняшняя проба зашита на 2 секунды. Настройка таймаута обязана её двигать, иначе она врёт пользователю.

**Files:**
- Изменяется: `internal/proxy/ping_icmp.go`
- Создаётся: тест в `internal/proxy/ping_icmp_timeout_test.go`

**Interfaces:**
- Consumes: `pingICMPHost(host, source string) (int64, bool)` — остаётся с прежней сигнатурой для нынешних вызывающих (`PingWireGuard`, `PingWireGuardLANBind`).
- Produces: `pingICMPHostTimeout(host, source string, timeout time.Duration) (int64, bool)`; `const pingICMPHostDefaultTimeout = 2 * time.Second`; `var pingICMPProbe = pingICMPHostTimeout` (шов для тестов диспетчера).

- [ ] **Step 1: Написать падающий тест**

Создать `internal/proxy/ping_icmp_timeout_test.go`:

```go
// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"testing"
	"time"
)

// Прежние вызывающие не передают бюджет и должны получить ровно то поведение,
// что было: две секунды.
func TestPingICMPDefaultTimeoutUnchanged(t *testing.T) {
	if pingICMPHostDefaultTimeout != 2*time.Second {
		t.Errorf("дефолтный бюджет ICMP = %v, был 2s", pingICMPHostDefaultTimeout)
	}
}

// Заданный бюджет обязан ограничивать пробу: 192.0.2.1 — TEST-NET-1, он не
// отвечает никогда, поэтому проба должна вернуться по своему сроку, а не по
// чужому.
func TestPingICMPHonoursTimeout(t *testing.T) {
	start := time.Now()
	if _, ok := pingICMPHostTimeout("192.0.2.1", "", 300*time.Millisecond); ok {
		t.Skip("TEST-NET-1 неожиданно ответил — сеть подменяет ICMP, проверять нечего")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("проба заняла %v при бюджете 300ms", elapsed)
	}
}
```

- [ ] **Step 2: Убедиться, что тест падает**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run TestPingICMP 2>&1 | head -6
```

Ожидается: `undefined: pingICMPHostDefaultTimeout`, `undefined: pingICMPHostTimeout`.

- [ ] **Step 3: Развести бюджет и дефолт**

В `internal/proxy/ping_icmp.go` переименовать тело в `pingICMPHostTimeout(host, source string, timeout time.Duration)`, заменив `pinger.Timeout = 2 * time.Second` на `pinger.Timeout = timeout`, и оставить прежнюю функцию тонкой обёрткой:

```go
// pingICMPHostDefaultTimeout is what every caller that does not care gets. It
// is the value this probe has always used; the ping settings path passes its
// own budget instead.
const pingICMPHostDefaultTimeout = 2 * time.Second

// pingICMPProbe is a var so the ping dispatch can be tested without raw
// sockets, matching the pingTCPProbe pattern.
var pingICMPProbe = pingICMPHostTimeout

func pingICMPHost(host, source string) (latencyMs int64, ok bool) {
	return pingICMPHostTimeout(host, source, pingICMPHostDefaultTimeout)
}
```

Комментарий про Windows у `SetPrivileged` дополнить одной строкой: на Android это тот же unprivileged UDP-сокет, и он **проверен** — `ping_group_range` на телефоне пускает любой gid, echo из-под uid приложения уходит и возвращается (замер 2026-09-17, §6.3 спека).

- [ ] **Step 4: Тесты проходят в обеих конфигурациях**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run TestPingICMP -v 2>&1 | grep -E "^(--- |ok|FAIL)"
go test -tags="${TAGS_PLAY}" ./internal/proxy/ 2>&1 | tail -2
```

- [ ] **Step 5: Коммит**

```bash
git add internal/proxy/ping_icmp.go internal/proxy/ping_icmp_timeout_test.go
git commit -F- <<'MSG'
feat(proxy): у ICMP-пробы появился бюджет времени

Настройка таймаута обязана двигать пробу, а она была зашита на две секунды.
Прежние вызывающие получают ровно прежнее поведение через обёртку с
дефолтом — менять их поведение эта задача не должна.

Заодно записано измерение: на Android это unprivileged UDP-сокет ICMP, и он
работает — ping_group_range пускает любой gid, echo из-под uid приложения
возвращается за 59 мс. Комментарий об обратном был наследием.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
MSG
```

---

### Task 4: Одноразовый движок пробы (конфиг)

Половина первая: собрать конфиг sing-box, в котором есть только петлевой mixed-инбаунд и аутбаунд самого узла.

Почему переиспользуется `buildOutbounds`, а не пишется второй, простой сборщик: `buildOutbounds` уже знает каждый протокол и его причуды в `Extra`, а параллельная реализация разошлась бы с ним на следующем добавленном протоколе — и проба мерила бы узел, который настоящая сессия строит иначе.

**Files:**
- Создаётся: `internal/proxy/ping_engine.go`
- Создаётся: `internal/proxy/ping_engine_test.go`

**Interfaces:**
- Consumes: `buildOutbounds(ProxyConfig, string) []SBOutbound`, `serverDomainResolverTag(ProxyConfig, ProxyMode, *SBDNS) string`, `SingBoxConfig`, `SBInbound`, `SBRoute`, `SBLog`, `SBDNS`, `SBDNSServer` — всё уже есть в `internal/proxy`.
- Produces: `BuildPingProbeConfig(proxy ProxyConfig, listenPort int, bindIPv4 string) (SingBoxConfig, error)`; `errPingProbeUnsupported`; `const pingProbeInboundTag = "ping-probe-in"`.

**Отличия от ПК, принятые осознанно:**
- **Кэша пинов сервера (`serverPinnedIPs`, `serverPinDNSTag`) на `android` нет.** На ПК он нужен, потому что во время сессии системный резолвер процесса непригоден. На Android приложение исключено из собственного VPN, поэтому системный резолвер остаётся рабочим и в сессии — DNS-блок сводится к одному `local`-серверу. Если однажды окажется, что и это не так, пин-кэш переносится отдельной задачей.
- ~~`bindIPv4` в сигнатуре остаётся~~ — **отменено на исполнении:** у `SBOutbound` на `android` поля привязки к адаптеру нет вовсе (оно десктопное). Держать параметр значило бы завести поле в структуре, которое всегда пусто и попадает в конфиг каждого узла. Сигнатура — `BuildPingProbeConfig(proxy ProxyConfig, listenPort int)`, причина записана в шапке файла.

- [ ] **Step 1: Написать падающие тесты**

Создать `internal/proxy/ping_engine_test.go`:

```go
// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"errors"
	"testing"
)

func pingProbeNode() ProxyConfig {
	return ProxyConfig{
		Type: "VLESS", IP: "203.0.113.7", Port: 443,
		Password: "11111111-1111-1111-1111-111111111111",
		Extra:    []byte(`{"security":"tls","sni":"example.com","type":"tcp"}`),
	}
}

// Конфиг пробы — это петлевой инбаунд, узел и ничего больше: всё, что в нём
// лишнее, мерилось бы вместе с узлом.
func TestPingProbeConfigShape(t *testing.T) {
	cfg, err := BuildPingProbeConfig(pingProbeNode(), 34567, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Inbounds) != 1 {
		t.Fatalf("инбаундов %d, ожидался один", len(cfg.Inbounds))
	}
	in := cfg.Inbounds[0]
	if in.Type != "mixed" || in.Listen != "127.0.0.1" || in.ListenPort != 34567 {
		t.Errorf("инбаунд = %+v", in)
	}
	if in.Tag != pingProbeInboundTag {
		t.Errorf("тег инбаунда = %q", in.Tag)
	}
	if cfg.Route == nil || cfg.Route.Final != "proxy" {
		t.Errorf("route.final = %+v, ожидался proxy", cfg.Route)
	}
	if cfg.Log == nil || cfg.Log.Level != "error" {
		t.Errorf("уровень лога = %+v: обход списка поднимает десятки таких движков", cfg.Log)
	}
	var hasProxy bool
	for _, o := range cfg.Outbounds {
		if o.Tag == "proxy" {
			hasProxy = true
		}
	}
	if !hasProxy {
		t.Error("в конфиге нет аутбаунда proxy")
	}
	assertCoreAcceptsConfig(t, cfg)
}

// WireGuard и AmneziaWG аутбаунда не имеют вовсе — они эндпоинты, и мерить
// через них HTTP нечем. Отказ должен быть назван, а не превращён в таймаут.
func TestPingProbeConfigRefusesWireGuard(t *testing.T) {
	for _, pt := range []string{"WIREGUARD", "AMNEZIAWG"} {
		node := ProxyConfig{Type: pt, IP: "203.0.113.7", Port: 51820,
			Extra: []byte(`{"private_key":"aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=","public_key":"WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=","address":["10.0.0.2/32"],"allowed_ips":["0.0.0.0/0"]}`)}
		if _, err := BuildPingProbeConfig(node, 34567, ""); !errors.Is(err, errPingProbeUnsupported) {
			t.Errorf("%s: ожидался errPingProbeUnsupported, получено %v", pt, err)
		}
	}
}

// Узел, адресованный именем, нуждается в резолвере внутри пробы — иначе
// аутбаунд не знает, куда идти.
func TestPingProbeConfigResolvesNamedNode(t *testing.T) {
	node := pingProbeNode()
	node.IP = "node.example.com"
	cfg, err := BuildPingProbeConfig(node, 34567, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DNS == nil || len(cfg.DNS.Servers) == 0 {
		t.Fatal("у узла с именем должен быть DNS-блок")
	}
	assertCoreAcceptsConfig(t, cfg)
}

// (Тест привязки к адаптеру снят вместе с параметром — см. отличия выше.
//  Вместо него проверяется, что у узла с литеральным адресом DNS-блока нет.)
```

- [ ] **Step 2: Убедиться, что тесты падают**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run TestPingProbeConfig 2>&1 | head -6
```

- [ ] **Step 3: Написать конфиг движка**

Создать `internal/proxy/ping_engine.go` с `errPingProbeUnsupported`, `pingProbeInboundTag` и `BuildPingProbeConfig` — по образцу `dev:internal/proxy/ping_engine.go`, но:
- DNS-блок для узла с именем сводится к одному серверу `{Type: "local", Tag: "local"}` и `Final: "local"` (пин-кэша на `android` нет — причина в шапке задачи);
- комментарии про `serverPinnedIPs` заменить на объяснение, **почему** на Android хватает системного резолвера (приложение исключено из своего VPN).

Проверить перед написанием, что поле `Inet4BindAddress` у `SBOutbound` на `android` называется так же (`grep -n "Inet4BindAddress" internal/proxy/engine.go`); если нет — использовать местное имя и записать это в теле задачи.

- [ ] **Step 4: Тесты проходят в обеих конфигурациях**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run TestPingProbeConfig -v 2>&1 | grep -E "^(--- |ok|FAIL)"
go test -tags="${TAGS_PLAY}" ./internal/proxy/ 2>&1 | tail -2
```

- [ ] **Step 5: Коммит**

```bash
git add internal/proxy/ping_engine.go internal/proxy/ping_engine_test.go
git commit -F- <<'MSG'
feat(proxy): конфиг одноразового движка для пробы через узел

Проба переиспользует buildOutbounds, а не заводит второй сборщик: тот уже
знает каждый протокол и его причуды в Extra, а параллельная реализация
разошлась бы с ним на следующем добавленном протоколе — и проба мерила бы
узел, который настоящая сессия строит иначе.

От версии ПК отличается DNS-блоком: кэша пинов сервера на android нет и не
нужен — приложение исключено из собственного VPN, поэтому системный резолвер
остаётся рабочим и во время сессии.

WireGuard и AmneziaWG отклоняются по имени причины, а не таймаутом: у них
вовсе нет аутбаунда, через который можно вести HTTP.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
MSG
```

---

### Task 5: Замер HTTP через узел

Половина вторая: поднять движок, сходить тестовым адресом через него и вернуть время.

**Files:**
- Изменяется: `internal/proxy/ping_engine.go`
- Изменяется: `internal/proxy/ping_engine_test.go`

**Interfaces:**
- Consumes: `BuildPingProbeConfig` (задача 4), `getFreeLocalPort(int) int`, `include.Context`, `pingReasonFromError(error) string`.
- Produces: `pingThroughNode(ctx context.Context, proxy ProxyConfig, method, testURL, bindIPv4 string) (int64, bool, string)`; `var pingThroughNodeProbe = pingThroughNode`; `classifyPingFetch(*http.Response, error) (bool, string)`; `const pingProbeEngineCeiling = 5 * time.Second`; `const pingEngineMaxConcurrency = 4`; `var pingEngineSem chan struct{}`.

**Отличия от ПК:**
- `extendedBoxContext` приедет только с блоком 4 — здесь контекст строится как в `singbox.go`: `include.Context(ctx)`.
- `closeInstanceBounded` на `android` нет; нужен местный `closePingProbeBounded(instance, ceiling)` на десяток строк: закрыть в горутине, подождать потолок, дальше бросить. Логгер ему не нужен и не передаётся — на ПК именно строка на каждый снос засорила пользовательский журнал при обходе списка (`880c389`).

- [ ] **Step 1: Дописать падающие тесты**

Дописать в `internal/proxy/ping_engine_test.go`:

```go

// Любой статус — успех, и это намеренно: запрос идёт по HTTPS через CONNECT,
// сертификат проверяется внутри процесса, поэтому сам факт ответа доказывает,
// что байты дошли до настоящего хоста. Отдельно стоит 407: это отказ прокси,
// а не ответ сайта.
func TestClassifyPingFetch(t *testing.T) {
	ok, reason := classifyPingFetch(&http.Response{StatusCode: 204}, nil)
	if !ok || reason != "" {
		t.Errorf("204 = (%v, %q), ожидался успех", ok, reason)
	}
	if ok, _ := classifyPingFetch(&http.Response{StatusCode: 500}, nil); !ok {
		t.Error("500 тоже доказывает, что байты дошли")
	}
	if ok, reason := classifyPingFetch(&http.Response{StatusCode: 407}, nil); ok || reason != "proxy_auth_required" {
		t.Errorf("407 = (%v, %q)", ok, reason)
	}
	if ok, reason := classifyPingFetch(nil, errors.New("i/o timeout")); ok || reason != "timeout" {
		t.Errorf("таймаут = (%v, %q)", ok, reason)
	}
	if ok, reason := classifyPingFetch(nil, nil); ok || reason == "" {
		t.Errorf("пустой ответ без ошибки должен быть назван: (%v, %q)", ok, reason)
	}
}

// Узел без аутбаунда обязан ответить именем причины, а не поднимать движок.
func TestPingThroughNodeRefusesWireGuard(t *testing.T) {
	node := ProxyConfig{Type: "AMNEZIAWG", IP: "203.0.113.7", Port: 51820,
		Extra: []byte(`{"private_key":"aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=","public_key":"WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=","address":["10.0.0.2/32"],"allowed_ips":["0.0.0.0/0"]}`)}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, ok, reason := pingThroughNode(ctx, node, http.MethodHead, config.DefaultPingTestURL, "")
	if ok || reason != "unsupported_for_protocol" {
		t.Errorf("получено (%v, %q)", ok, reason)
	}
}

// Негодный тестовый адрес — это ошибка запроса, а не узла.
func TestPingThroughNodeRejectsBadURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, ok, reason := pingThroughNode(ctx, pingProbeNode(), http.MethodGet, "http://\x7f", "")
	if ok || reason != "bad_test_url" {
		t.Errorf("получено (%v, %q)", ok, reason)
	}
}

// Потолок одновременных движков — не украшение: список пингуется по 16
// параллельно, а движок дороже сокета на порядки.
func TestPingEngineConcurrencyIsCapped(t *testing.T) {
	if cap(pingEngineSem) != pingEngineMaxConcurrency {
		t.Errorf("ёмкость семафора %d, ожидалась %d", cap(pingEngineSem), pingEngineMaxConcurrency)
	}
	if pingEngineMaxConcurrency >= 16 {
		t.Errorf("потолок %d не ниже параллелизма списка (16) — значит не ограничивает", pingEngineMaxConcurrency)
	}
}
```

Импорты теста дополнить: `context`, `net/http`, `time`, `resultproxy-wails/internal/config`.

- [ ] **Step 2: Убедиться, что тесты падают**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run 'TestClassifyPingFetch|TestPingThroughNode|TestPingEngineConcurrency' 2>&1 | head -8
```

- [ ] **Step 3: Написать замер**

Дописать в `internal/proxy/ping_engine.go` `classifyPingFetch`, `pingThroughNode`, `pingThroughNodeProbe`, `closePingProbeBounded`, `pingProbeEngineCeiling`, `pingEngineMaxConcurrency`, `pingEngineSem` — по образцу `dev`, с тремя правками:
- контекст ядра: `include.Context(boxCtx)` вместо `extendedBoxContext`;
- снос инстанса: местный `closePingProbeBounded` без логгера;
- семафор: занимать **до** сборки конфига и отпускать после сноса, чтобы потолок считал именно живые движки.

Часы включаются после старта движка и до `client.Do`: старт — наша цена, а не задержка узла.

- [ ] **Step 4: Тесты проходят в обеих конфигурациях**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run 'TestClassifyPingFetch|TestPingThroughNode|TestPingEngine' -v 2>&1 | grep -E "^(--- |ok|FAIL)"
go test -tags="${TAGS_FULL}" ./internal/proxy/ 2>&1 | tail -2
go test -tags="${TAGS_PLAY}" ./internal/proxy/ 2>&1 | tail -2
```

- [ ] **Step 5: Коммит**

```bash
git add internal/proxy/ping_engine.go internal/proxy/ping_engine_test.go
git commit -F- <<'MSG'
feat(proxy): замер задержки HTTP-запросом через сам узел

Это единственная проба, которая отвечает на вопрос «узел реально возит
трафик», а не «порт открыт»: часы покрывают рукопожатие узла, CONNECT, TLS до
цели и ожидание заголовков — всё, чего человек действительно ждёт. Старт
движка в цифру не входит: это наша цена, а не задержка узла.

Потолок одновременных движков нужен здесь, а не в UI: список пингуется по 16
параллельно, и шестнадцать одноразовых sing-box на телефоне — это не проба, а
диверсия.

Снос инстанса без логгера намеренно: на ПК строка на каждый снос превратила
обход списка в стену записей в пользовательском журнале.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
MSG
```

---

### Task 6: Диспетчер по типу в биндинге

На ПК это `Manager.Ping`; на `android` `manager.go` нет, и роль диспетчера играет `PingEntry`.

**Files:**
- Изменяется: `mobile/libbox.go`
- Создаётся: `mobile/libbox_ping_options_test.go`

**Interfaces:**
- Consumes: `config.PingType*`, `config.DefaultPingTestURL`, `ValidatePingTestURL`, `pingThroughNodeProbe`, `pingICMPProbe`, `proxy.PingWireGuardHandshake`, `proxy.PingWireGuard`, `proxy.PingHysteria2QUIC`, `proxy.PingProxy`, `keyedWGProbeAllowed()`.
- Produces: `PingEntry(entryJSON, optionsJSON string) (string, error)` — **второй аргумент новый**; `type PingOptions struct {Type, TestURL string; TimeoutSec int}` с JSON-ключами `pingType` / `pingTestUrl` / `pingTimeoutSec`; `decodePingOptions(string) PingOptions`.

**Решения:**
- Гейт `SetTunnelActive` остаётся **только** у keyed-пробы WG. Типу «HTTP через узел» он не нужен: приложение исключено из собственного VPN, поэтому проба не мерит сама себя (проверяется замером в задаче 9).
- Тип `icmp` применим к любому протоколу — он мерит путь, а не транспорт.
- Тип `http_*` на WG/AWG невозможен: `pingThroughNode` вернёт `unsupported_for_protocol`, и строка сервера так и напишет.
- Пустой `optionsJSON` читается как «авто, дефолтный адрес, 3 секунды» — иначе старый вызов сломался бы молча.

- [ ] **Step 1: Написать падающие тесты**

Создать `mobile/libbox_ping_options_test.go` с проверками: пустой JSON даёт авто/дефолт/3; негодный тип падает в авто; негодный адрес падает в дефолт; таймаут вне 1..10 падает в 3; для `http_get` выбирается метод GET, для `http_head` — HEAD. Диспетчер тестируется через подмену `proxy.pingThroughNodeProbe`/`proxy.pingICMPProbe` — оба уже объявлены как `var` ровно для этого, но они в другом пакете, поэтому в `internal/proxy` понадобится экспортированный шов для тестов: `SetPingProbesForTest(...)` не заводить, а перенести диспетчер в `internal/proxy` и тестировать там же, оставив в `mobile` только разбор JSON.

**Решение по размещению (принять до кода):** диспетчер живёт в `internal/proxy/ping_dispatch.go` как `PingNode(entry config.ProxyEntry, opts PingOptions) (latencyMs int64, reachable bool, reason, checkType string)`, а `mobile.PingEntry` только разбирает JSON, зовёт его и сериализует ответ. Так тестируется и диспетчер, и разбор, и ни один шов не экспортируется наружу ради теста.

- [ ] **Step 2: Убедиться, что тесты падают** — `undefined: PingNode`.

- [ ] **Step 3: Написать диспетчер и разбор**

`internal/proxy/ping_dispatch.go`: `PingOptions`, `PingNode` по образцу `Manager.Ping` — `http_get`/`http_head` → `pingThroughNodeProbe`; `icmp` → `pingICMPProbe` с бюджетом; иначе — сегодняшняя раскладка по протоколу (keyed WG под гейтом, QUIC у Hysteria2, TCP у остальных). Гейт WG приходит параметром `keyedWGAllowed bool`, чтобы `internal/proxy` не знал про состояние туннеля.

`mobile/libbox.go`: `PingEntry(entryJSON, optionsJSON string)` разбирает оба JSON и зовёт `proxy.PingNode(entry, opts, keyedWGProbeAllowed())`.

- [ ] **Step 4: Тесты проходят** (обе конфигурации, весь `./...`).

- [ ] **Step 5: Коммит.**

---

### Task 7: Настройки пинга в Kotlin

**Files:**
- Изменяется: `android/.../vpn/SettingsRepository.kt`, `android/.../vpn/PingRepository.kt`
- Создаётся: `android/app/src/test/java/com/resultv/android/vpn/PingOptionsTest.kt`

**Interfaces:**
- Produces: `SettingsState.pingType: String`, `pingTestUrl: String`, `pingTimeoutSec: Int`; сеттеры; `SettingsRepository.pingOptionsJson(): String`; `PingRepository` зовёт `Mobile.pingEntry(entryJson, SettingsRepository.pingOptionsJson())`.

Нормализация на стороне Kotlin делает ровно одно: не пускает в биндинг заведомо негодное (пустой адрес, таймаут вне 1..10). Настоящая проверка — в Go, и повторяется она потому, что поле ввода живёт в Kotlin и должно краснеть до нажатия, а не после.

- [ ] **Step 1: Тест нормализации** → **Step 2: падает** → **Step 3: поля и сеттеры** → **Step 4: проходит** → **Step 5: коммит.**

---

### Task 8: Раздел «Пинг» и причины отказа

**Files:**
- Изменяется: `android/.../ui/screens/SettingsScreen.kt` (раздел в шторке «Сеть», по образцу блока DNS), `android/.../ui/components/ServerRow.kt`, `values{,-ru}/strings.xml`

**Interfaces:**
- Produces: строки `settings_ping*`, `ping_no_icmp`, `ping_na`, `ping_bad_url`, `ping_auth`; новые ветки в `offlineLabel`.

Ряды: чипы типа (Авто / ICMP / HTTP GET / HTTP HEAD), поле тестового адреса с валидацией https (красная подпись при негодном), поле таймаута 1–10. Подписи держать в пределах 64 символов — `SettingsStringsTest` проверяет и длину, и паритет локалей; непереводимых `settings_*` не заводить.

Новые причины в `offlineLabel`: `icmp_unavailable` → «Нет ICMP», `unsupported_for_protocol` → «Н/Д», `bad_test_url` → «URL», `proxy_auth_required` → «Авторизация», `engine_start_failed` → «Ошибка движка».

- [ ] **Step 1: строки** → **Step 2: сборка** → **Step 3: UI** → **Step 4: тесты Kotlin (обе сборки)** → **Step 5: коммит.**

---

### Task 9: Приёмка

- [ ] **Step 1:** `go build` и `go test ./...` в обеих конфигурациях — зелено.
- [ ] **Step 2:** Kotlin-тесты обеих сборок; число тестов выросло ровно на добавленные.
- [ ] **Step 3:** оба AAR с ключом подписок, `LOAD align = 0x4000`, `internal/filter` в play-графе — 0.
- [ ] **Step 4: телефон, тип «Авто»** — список пингуется как раньше (AWG keyed ≈150 мс, VLESS/Trojan TCP, Hysteria2 QUIC).
- [ ] **Step 5: телефон, тип «ICMP»** — все типы узлов отдают RTT; сверить порядок величины с `run-as com.resultv.android ping <хост>`.
- [ ] **Step 6: телефон, тип «HTTP GET» при ВЫКЛЮЧЕННОМ туннеле** — узлы отдают задержку; AWG-узел отдаёт «Н/Д» (`unsupported_for_protocol`).
- [ ] **Step 7: телефон, тип «HTTP GET» при ВКЛЮЧЁННОМ туннеле** — **ключевая проверка допущения:** цифры должны остаться того же порядка, что в шаге 6, и не схлопнуться в «время до себя». Если схлопнулись — допущение «приложение исключено из своего VPN» неверно для этого пути, и тип надо гейтить, как на ПК; записать факт и остановиться.
- [ ] **Step 8:** негодный тестовый адрес (`http://…`) — поле краснеет в настройках, в список уходит прежнее значение.
- [ ] **Step 9:** записи в спек (§14) и в основной документ; коммит через `git add -f`.
