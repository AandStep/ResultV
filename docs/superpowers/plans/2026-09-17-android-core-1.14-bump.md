# Блок 5, задача 1 — ядро sing-box 1.14 на Android

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Поднять Android с `sing-box-extended 1.13.18-extended-2.6.4` до `1.14.0-extended-2.7.1` так, чтобы туннель, счётчики трафика и DNS работали на телефоне не хуже, чем до бампа.

**Architecture:** `go.mod`/`go.sum` берутся с ветки `dev` (там эта комбинация версий уже отстреляна) и надстраиваются android-спецификой. В коде правится ровно одна точка API (`RoutedFlow`) плюс шесть мест, где 1.14 сменило поведение молча или сломало его совсем. Каждое место закрывается тестом на форму JSON или на поведение счётчиков — кроме бинарной сборки, которая проверяется замером готового `.so` и телефоном.

**Tech Stack:** Go 1.26.4, `sing-box-extended` (форк shtorm-7), `sing-tun 0.9.0-beta.4`, gomobile (форк sagernet, v0.1.12), Kotlin/Compose в `android/`.

**Spec:** `docs/superpowers/specs/2026-09-17-android-core-1.14-sync-design.md`

## Global Constraints

- Ветка `android` в `C:\ResultV`. Точка отката — `origin/android` = `c59f0ae`.
- Теги сборки в этом плане:
  - `TAGS_FULL="mobile,with_gvisor,with_utls,with_clash_api,with_quic,with_wireguard,with_grpc"` — содержимое `scripts/android-build-tags.txt`, дословно.
  - `TAGS_PLAY="$TAGS_FULL,no_mitm,no_adblock"`.
- **Каждый** `go build` / `go test` в этом плане гоняется в обеих конфигурациях. Конфигурация, которую не прогнали, считается красной.
- Без тега `mobile` пакеты `mobile` и часть `internal/proxy` не собираются, и LSP будет врать про «undefined». Это шум, а не ошибка.
- AAR **никогда** не пересобирается gradle-ом: только `DIST=full ./scripts/build-android-aar.sh` и `DIST=play ./scripts/build-android-aar.sh`. `gradlew assemble*` переупаковывает уже лежащий `.so`, и правки Go молча не доедут до телефона.
- `internal/config` общий с ПК — в этом плане он не меняется вообще.
- Никаких попутных улучшений: каждая изменённая строка должна прослеживаться до задачи плана. Единственное исключение оговорено явно в задаче 3 (мёртвое поле `AddressStrategy`).
- Сообщения коммитов на русском, в формате ветки: `тип(область): описание`, тело — почему, а не что. Последняя строка:
  `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>`
- Ни один коммит этого плана не пушится в `origin` до приёмки задачи 9.

---

### Task 1: Зелёная база

Прежде чем менять ядро, надо знать, что было зелёным. Иначе первый же красный тест после бампа не отличить от красного до него.

**Files:**
- Изменяется: ничего, если база зелёная.

**Interfaces:**
- Consumes: —
- Produces: зафиксированный список падений «до», на который ссылаются все следующие задачи.

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

Ожидается: обе строки OK, код возврата 0.

- [ ] **Step 3: Прогнать тесты в обеих конфигурациях и записать результат**

```bash
go test -tags="${TAGS_FULL}" ./... 2>&1 | tail -40
go test -tags="${TAGS_PLAY}" ./... 2>&1 | tail -40
```

Ожидается: `ok` по всем пакетам. Если что-то падает — **не чинить наугад**: записать точный список падений сюда, в тело задачи 1, и дальше сверяться с ним. Красное до бампа остаётся красным после бампа и не считается регрессией.

- [ ] **Step 4: Зафиксировать текущую версию ядра**

```bash
grep -E 'sing-box|sagernet/sing |wireguard-go|tailscale' go.mod
```

Ожидается: `sing-box v1.13.18`, `sing v0.8.12`, `wireguard-go v0.0.4`, replace на `shtorm-7/sing-box-extended v1.13.18-extended-2.6.4`.

- [ ] **Step 5: Коммит — только если Step 3 потребовал правок**

Если база была зелёной, коммитить нечего, задача закрыта. Если пришлось чинить:

```bash
git add -A
git commit -m "test(proxy): зелёная база перед бампом ядра

Тело: подставить фактический вывод падающих тестов из шага 3 и объяснить,
почему падение не связано с ядром 1.14.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 2: Бамп модуля до 1.14 и метод RoutedFlow

Единственная точка API, где Android упирается в 1.14. `*Ex`-интерфейсы на ветке не используются, `box.Context` не вызывается (контекст строится через `include.Context`, `singbox.go:349`) — обе проверены поиском, трогать нечего.

**Files:**
- Modify: `go.mod`, `go.sum`
- Modify: `internal/proxy/singbox.go` (импорты; `trafficTracker` — после `RoutedPacketConnection`, около строки 487)
- Create: `internal/proxy/traffic_flow_test.go`

**Interfaces:**
- Consumes: `trafficTracker` с полями `upload`, `download`, `log`, `server`, `protocol`, `mode`, `logged`, `count`, `capped`, `isSub` (`singbox.go:204`); метод `logConnection(adapter.InboundContext, adapter.Outbound) (string, string, bool)` (`singbox.go:490`).
- Produces: `func (t *trafficTracker) RoutedFlow(context.Context, adapter.InboundContext, adapter.Rule, adapter.Outbound) tun.FlowTracker`; тип `flowCounter` с методами `AttachFlow`, `CountForward`, `CountReverse`, `FlowEstablished`, `CloseFlow`; тестовые хелперы `trackerForTest() *trafficTracker` и `stubOutbound` — задачи 3–8 ими пользуются.

- [ ] **Step 1: Поднять версии в go.mod**

В блоке прямых зависимостей заменить четыре строки:

```
	github.com/sagernet/sing v0.8.12          →  github.com/sagernet/sing v0.9.0-beta.4
	github.com/sagernet/sing-box v1.13.18     →  github.com/sagernet/sing-box v1.14.0
	github.com/sagernet/wireguard-go v0.0.4   →  github.com/sagernet/wireguard-go v0.0.5
	golang.org/x/crypto v0.53.0               →  golang.org/x/crypto v0.54.0
	golang.org/x/sys v0.46.0                  →  golang.org/x/sys v0.47.0
```

В блоке `replace` заменить четыре строки на версии с `dev`:

```
replace github.com/sagernet/sing => github.com/shtorm-7/sing v0.9.0-beta.4-extended-1.2.1

replace github.com/sagernet/wireguard-go => github.com/shtorm-7/wireguard-go v0.0.5-extended-1.6.1

replace github.com/sagernet/tailscale => github.com/shtorm-7/tailscale v1.102.1-sing-box-1.14-mod.4-extended-1.0.3

replace github.com/sagernet/sing-box v1.14.0 => github.com/shtorm-7/sing-box-extended v1.14.0-extended-2.7.1
```

Плюс одна строка `replace`, которую на `dev` подняли вместе с ядром:

```
replace github.com/Diniboy1123/connect-ip-go => github.com/shtorm-7/connect-ip-go v1.0.0-extended-1.1.1
```

**Не трогать** android-специфику: `replace` на `./third_party/gomitmproxy`, `replace` на `go-admin`, прямые `AdguardTeam/golibs`, `AdguardTeam/urlfilter`, `AdguardTeam/gomitmproxy`, `sagernet/gomobile`. Сверка с `dev` — `git show dev:go.mod | grep -A20 '^replace'`.

- [ ] **Step 2: Пересобрать граф зависимостей**

```bash
go mod tidy -compat=1.26
go build -tags="${TAGS_FULL}" ./... 2>&1 | head -20
```

Ожидается: FAIL. Ядро 1.14 добавило третий метод в `adapter.ConnectionTracker`, и сборка обязана на это указать — сообщение вида `*trafficTracker does not implement adapter.ConnectionTracker (missing method RoutedFlow)` там, где трекер передаётся ядру. Это красная фаза задачи.

Если `go mod tidy` уронил `replace` на `go-admin` как осиротевший — так и оставить (на `dev` его сняли по той же причине) и отметить это в сообщении коммита.

- [ ] **Step 3: Написать падающий тест на счётчики потока**

Создать `internal/proxy/traffic_flow_test.go`. Хелперов `trackerForTest` и `stubOutbound` на ветке нет — они объявляются здесь:

```go
// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/sagernet/sing-box/adapter"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

	"resultproxy-wails/internal/logger"
)

type stubOutbound struct {
	adapter.Outbound
	tag string
}

func (s stubOutbound) Tag() string  { return s.tag }
func (s stubOutbound) Type() string { return s.tag }

func trackerForTest() *trafficTracker {
	return &trafficTracker{
		upload:   new(atomic.Int64),
		download: new(atomic.Int64),
		log:      logger.New(),
	}
}

// Узел WireGuard в туннельном режиме на 1.14 вообще не доходит до
// RoutedConnection: TUN спрашивает роутер о каждом новом потоке, эндпоинт
// отвечает PreMatchFlow для любой сети и реализует tun.Port, поэтому пакеты
// идут L3 и никогда не становятся net.Conn. Без RoutedFlow индикатор скорости
// и traffic veto в KillSwitchWatchdog видят ноль всю сессию.
func TestRoutedFlowCountsBytesForForwardedFlows(t *testing.T) {
	tr := trackerForTest()
	flow := tr.RoutedFlow(context.Background(), adapter.InboundContext{
		Network:     N.NetworkTCP,
		Source:      M.ParseSocksaddr("10.0.0.2:51820"),
		Destination: M.ParseSocksaddr("203.0.113.7:443"),
	}, nil, stubOutbound{tag: "proxy"})
	if flow == nil {
		t.Fatal("RoutedFlow вернул nil для TCP-потока — сессия WireGuard была бы невидима для индикатора скорости")
	}
	flow.CountForward(1500)
	flow.CountReverse(9000)
	if got := tr.upload.Load(); got != 1500 {
		t.Fatalf("upload = %d, ожидалось 1500", got)
	}
	if got := tr.download.Load(); got != 9000 {
		t.Fatalf("download = %d, ожидалось 9000", got)
	}
}

// ICMP — единственный поток, о котором ядро спрашивает и на обычном узле:
// `direct` реализует FlowOutbound исключительно ради него. В эти счётчики
// пинги не попадали и до 1.14, и начинать не должны.
func TestRoutedFlowIgnoresICMP(t *testing.T) {
	tr := trackerForTest()
	if flow := tr.RoutedFlow(context.Background(), adapter.InboundContext{
		Network:     N.NetworkICMP,
		Source:      M.ParseSocksaddr("10.0.0.2:0"),
		Destination: M.ParseSocksaddr("1.1.1.1:0"),
	}, nil, stubOutbound{tag: "direct"}); flow != nil {
		t.Fatal("RoutedFlow не должен участвовать в учёте ICMP")
	}
}
```

- [ ] **Step 4: Убедиться, что тест не компилируется**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run TestRoutedFlow 2>&1 | head -10
```

Ожидается: FAIL, `tr.RoutedFlow undefined (type *trafficTracker has no field or method RoutedFlow)`.

- [ ] **Step 5: Реализовать RoutedFlow и flowCounter**

В `internal/proxy/singbox.go` добавить импорт `"github.com/sagernet/sing-tun"` (пакет называется `tun`) в группу импортов `sagernet`, и вставить перед `func (t *trafficTracker) logConnection`:

```go
// RoutedFlow — третий метод, который sing-box 1.14 добавил в
// adapter.ConnectionTracker. Он про трафик, который роутер отдаёт аутбаунду
// сырым L3-потоком, а не соединением, и для этого клиента это не угол: TUN
// спрашивает роутер о каждом новом потоке (Router.PreMatch), а
// WireGuard-эндпоинт — а WG- и AWG-узлы у нас именно эндпоинты под тегом
// "proxy" — отвечает PreMatchFlow для любой сети и реализует tun.Port. На таких
// узлах пакеты форвардятся на L3 и никогда не становятся net.Conn:
// RoutedConnection и RoutedPacketConnection просто не вызываются, и nil отсюда
// оставил бы индикатор скорости и traffic veto вотчдога на нуле всю сессию.
// Обычные аутбаунды (VLESS, Trojan, hysteria2) не FlowOutbound и идут прежним
// путём.
//
// ICMP исключён намеренно: `direct` — FlowOutbound исключительно ради ICMP, так
// что сюда приходил бы каждый пинг. В эти счётчики байты пингов не попадали и
// до 1.14, и начинать не должны.
func (t *trafficTracker) RoutedFlow(
	_ context.Context,
	metadata adapter.InboundContext,
	_ adapter.Rule,
	matchOutbound adapter.Outbound,
) tun.FlowTracker {
	if metadata.Network == N.NetworkICMP {
		return nil
	}
	// Как в RoutedPacketConnection: logConnection зовётся ради журнала, а
	// счётчики здесь всегда общие — разделения на «трафик узла» и «весь
	// трафик» на этой ветке нет.
	t.logConnection(metadata, matchOutbound)
	return &flowCounter{
		down: []*atomic.Int64{t.download},
		up:   []*atomic.Int64{t.upload},
	}
}

// flowCounter записывает форварднутый поток в те же счётчики, что и обёрнутое
// соединение. Forward — это клиент→сервер (выгрузка), reverse — обратно; так же
// их трактует собственный flowLogger ядра (route/flow_tracker.go).
type flowCounter struct {
	down []*atomic.Int64
	up   []*atomic.Int64
}

func (f *flowCounter) AttachFlow(tun.FlowHandle) {}

func (f *flowCounter) CountForward(n int) {
	for _, c := range f.up {
		c.Add(int64(n))
	}
}

func (f *flowCounter) CountReverse(n int) {
	for _, c := range f.down {
		c.Add(int64(n))
	}
}

func (f *flowCounter) FlowEstablished() {}

func (f *flowCounter) CloseFlow(tun.FlowCloseReason) {}
```

Рядом с объявлением `type trafficTracker struct` (`singbox.go:204`) поставить пломбу, чтобы следующая смена интерфейса упёрлась в компиляцию, а не в рантайм у пользователя:

```go
var _ adapter.ConnectionTracker = (*trafficTracker)(nil)
```

- [ ] **Step 6: Прогнать тест и обе сборки**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run TestRoutedFlow -v
go test -tags="${TAGS_PLAY}" ./internal/proxy/ -run TestRoutedFlow -v
go build -tags="${TAGS_FULL}" ./... && echo "FULL OK"
go build -tags="${TAGS_PLAY}" ./... && echo "PLAY OK"
go test -tags="${TAGS_FULL}" ./... 2>&1 | grep -v '^ok' | head -30
go test -tags="${TAGS_PLAY}" ./... 2>&1 | grep -v '^ok' | head -30
```

Ожидается: оба теста PASS, обе сборки OK, в полном прогоне ничего нового по сравнению со списком из задачи 1.

- [ ] **Step 7: Коммит**

```bash
git add go.mod go.sum internal/proxy/singbox.go internal/proxy/traffic_flow_test.go
git commit -m "feat(proxy): ядро sing-box-extended 1.14.0-extended-2.7.1

Из шести точек API, которые бамп потребовал на ПК, мобильной ветки касается
одна: adapter.ConnectionTracker получил третий метод RoutedFlow. *Ex-интерфейсы
здесь не используются, box.Context не вызывается — контекст строится через
include.Context.

RoutedFlow сделан считающим, а не заглушкой. WG- и AWG-узлы у нас эндпоинты под
тегом proxy: TUN спрашивает роутер о каждом потоке, эндпоинт отвечает
PreMatchFlow и реализует tun.Port, пакеты идут L3 и не становятся net.Conn. С
nil индикатор скорости и traffic veto кил-свитча показывали бы ноль всю сессию.
ICMP исключён: в эти счётчики пинги не попадали и до 1.14.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 3: Поведение NAT и DNS на TUN сказано вслух

1.13 при отсутствии полей давал симметричный NAT, 1.14 при том же молчании даёт endpoint-independent — то есть противоположное. Молчание перестало быть нейтральным, и его надо заменить явными значениями.

**Files:**
- Modify: `internal/proxy/engine.go` (`SBInbound` — строка 177; `BuildTunnelModeConfig` — строка 529)
- Create: `internal/proxy/engine_tun_1_14_test.go`

**Interfaces:**
- Consumes: `BuildTunnelModeConfig(cfg EngineConfig) SingBoxConfig`; `EngineConfig.Proxy.Type`.
- Produces: поля `SBInbound.UDPMapping`, `SBInbound.UDPFiltering`, `SBInbound.DNSMode` — задача 5 добавляет к ним `UDPNATMax`.

- [ ] **Step 1: Написать падающий тест на форму JSON**

Создать `internal/proxy/engine_tun_1_14_test.go`:

```go
// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import "testing"

func tunInboundFor(t *testing.T, proxyType string) SBInbound {
	t.Helper()
	cfg := EngineConfig{
		Mode: ProxyModeTunnel,
		Proxy: ProxyConfig{
			Type: proxyType,
			IP:   "203.0.113.7",
			Port: 443,
		},
	}
	sb := BuildTunnelModeConfig(cfg)
	for _, in := range sb.Inbounds {
		if in.Type == "tun" {
			return in
		}
	}
	t.Fatalf("в конфиге для %s нет tun-инбаунда", proxyType)
	return SBInbound{}
}

// На 1.13 отсутствие udp_mapping/udp_filtering означало симметричный NAT, на
// 1.14 — endpoint-independent. Поведение обычного узла выбрано осознанно
// (штормы QUIC-ретраев дают меньше слотов при endpoint-independent), поэтому
// оно записано, а не унаследовано.
func TestTunNATBehaviourIsStatedForPlainNodes(t *testing.T) {
	in := tunInboundFor(t, "VLESS")
	if in.UDPMapping != "endpoint_independent" {
		t.Errorf("udp_mapping = %q, ожидалось endpoint_independent", in.UDPMapping)
	}
	if in.UDPFiltering != "endpoint_independent" {
		t.Errorf("udp_filtering = %q, ожидалось endpoint_independent", in.UDPFiltering)
	}
}

// У WG/AWG инбаунд кормит пакетами эндпоинт, который держит своё состояние
// сессии. Здесь сохраняется ровно то поведение, которое ветка имела до 1.14,
// — теперь сказанное вслух, потому что дефолт ядра из-под него ушёл.
func TestTunNATBehaviourIsStatedForWireGuard(t *testing.T) {
	for _, pt := range []string{"WIREGUARD", "AMNEZIAWG"} {
		in := tunInboundFor(t, pt)
		if in.UDPMapping != "address_and_port_dependent" {
			t.Errorf("%s: udp_mapping = %q, ожидалось address_and_port_dependent", pt, in.UDPMapping)
		}
		if in.UDPFiltering != "address_and_port_dependent" {
			t.Errorf("%s: udp_filtering = %q, ожидалось address_and_port_dependent", pt, in.UDPFiltering)
		}
	}
}

// dns_mode в 1.14 по умолчанию hijack — ровно то, на что клиент опирался
// всегда. Записано здесь, чтобы будущий дефолт не сдвинул это молча, как
// сдвинул endpoint_independent_nat. DNSAddress не задаётся намеренно: ядро
// выводит адрес перехвата из адреса TUN, как было до появления опции.
func TestTunDNSModeIsStated(t *testing.T) {
	if in := tunInboundFor(t, "VLESS"); in.DNSMode != "hijack" {
		t.Errorf("dns_mode = %q, ожидалось hijack", in.DNSMode)
	}
}
```

- [ ] **Step 2: Убедиться, что тест не компилируется**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run 'TestTun' 2>&1 | head -10
```

Ожидается: FAIL, `in.UDPMapping undefined`, `in.DNSMode undefined`.

- [ ] **Step 3: Добавить поля в SBInbound**

В `internal/proxy/engine.go`, в `type SBInbound struct` (строка 177), после `RouteExcludeAddress`:

```go
	// UDPMapping и UDPFiltering заменили endpoint_independent_nat, который
	// sing-box 1.14 оставил в схеме, но перестал читать. Молча забытая ручка
	// хуже удалённой: конфиг по-прежнему разбирается, меняется только
	// поведение. Оба принимают "endpoint_independent", "address_dependent" или
	// "address_and_port_dependent".
	//
	// Endpoint-independent позволяет нескольким назначениям делить NAT-слоты
	// одной пары (адрес источника, порт источника) вместо слота на каждое
	// назначение: под штормом QUIC-ретраев браузера по десяткам адресов CDN из
	// одного эфемерного порта это пропорционально сокращает число слотов. Оно
	// же — новый дефолт ядра, противоположный тому, что делала 1.13 при
	// отсутствии поля, поэтому обе ветки в BuildTunnelModeConfig говорят своё
	// значение вслух, а не наследуют его.
	UDPMapping   string `json:"udp_mapping,omitempty"`
	UDPFiltering string `json:"udp_filtering,omitempty"`

	// DNSMode говорит, как TUN обходится с DNS: "disabled", "native" (задать
	// DNS платформы на интерфейсе) или "hijack" (native плюс перехват
	// DNS-трафика). 1.14 по умолчанию hijack — это то, на что клиент опирался
	// всегда; записано здесь, чтобы будущий дефолт не сдвинул это молча, как
	// сдвинул endpoint_independent_nat. Адрес перехвата не задаётся: ядро
	// выводит его из адреса TUN, как было до появления опции.
	DNSMode string `json:"dns_mode,omitempty"`
```

- [ ] **Step 4: Задать значения в BuildTunnelModeConfig**

В `BuildTunnelModeConfig` инбаунд сейчас собирается литералом внутри `SingBoxConfig{...}` (около строки 585). Вынести его в переменную перед конструированием конфига и дописать ветку:

```go
	tun := SBInbound{
		Type:                "tun",
		Tag:                 "tun-in",
		Address:             tunAddresses,
		Stack:               tunStack,
		AutoRoute:           true,
		StrictRoute:         strictRoute,
		RouteExcludeAddress: routeExclude,
		DNSMode:             "hijack",
	}
	if pt != "WIREGUARD" && pt != "AMNEZIAWG" {
		tun.UDPMapping = "endpoint_independent"
		tun.UDPFiltering = "endpoint_independent"
	} else {
		// То же поведение NAT, которое эта ветка имела до 1.14, теперь сказанное
		// явно, потому что дефолт ядра из-под неё ушёл.
		tun.UDPMapping = "address_and_port_dependent"
		tun.UDPFiltering = "address_and_port_dependent"
	}
```

и подставить `Inbounds: []SBInbound{tun},` в литерал `SingBoxConfig`.

- [ ] **Step 5: Удалить мёртвое поле AddressStrategy**

Это единственное отступление от правила «не трогать лишнего», и оно оговорено: `SBDNSServer.AddressStrategy` (`engine.go:166`) не задаётся нигде в дереве (проверить `grep -rn 'AddressStrategy' internal/ mobile/` — одна строка, объявление), а на ПК его удалили тем же коммитом, что менял эти же структуры. Оставить его — значит держать в схеме поле, которого ядро 1.14 не читает.

```bash
grep -rn 'AddressStrategy' internal/ mobile/
```

Ожидается: ровно одна строка — само объявление. Если строк больше, поле используется: **не удалять**, отметить в коммите и идти дальше.

- [ ] **Step 6: Прогнать тесты**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run 'TestTun' -v
go test -tags="${TAGS_PLAY}" ./internal/proxy/ -run 'TestTun' -v
go test -tags="${TAGS_FULL}" ./... 2>&1 | grep -v '^ok' | head -30
go test -tags="${TAGS_PLAY}" ./... 2>&1 | grep -v '^ok' | head -30
```

Ожидается: три теста PASS в обеих конфигурациях, остальное без новых падений.

- [ ] **Step 7: Коммит**

```bash
git add internal/proxy/engine.go internal/proxy/engine_tun_1_14_test.go
git commit -m "fix(proxy): поведение NAT и DNS на TUN сказано вслух вместо дефолта

1.13 при пустых полях давала симметричный NAT, 1.14 при том же молчании даёт
endpoint-independent. Молчание перестало быть нейтральным, поэтому обе ветки
теперь называют своё значение: обычные узлы — endpoint_independent (под штормом
QUIC-ретраев это пропорционально сокращает число NAT-слотов), WG и AWG —
address_and_port_dependent, ровно то, что у них было до бампа.

dns_mode: hijack — тоже дефолт 1.14 и то, на что клиент опирался всегда;
записан, чтобы следующий сдвиг дефолта не прошёл молча.

Заодно удалено мёртвое поле SBDNSServer.AddressStrategy: нигде не задаётся, а
ядро 1.14 его не читает.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 4: Адрес WireGuard-узла исключается из туннеля

Самая дорогая грабля бампа. На 1.13 петля «узел внутри собственного туннеля» просто тратила работу; sing-tun 0.9 перестроил обе таблицы NAT и поставил диспетчер потоков перед каждым пакетом — и та же петля начала душить сессию.

**Files:**
- Modify: `internal/proxy/engine.go` (`BuildTunnelModeConfig`, блок `routeExclude`, строки 559–568)
- Create: `internal/proxy/wg_route_exclude_test.go`

**Interfaces:**
- Consumes: `BuildTunnelModeConfig`; `tunInboundFor` из `engine_tun_1_14_test.go` (задача 3).
- Produces: ничего нового, только изменённое поведение `RouteExcludeAddress`.

- [ ] **Step 1: Написать падающий тест**

Создать `internal/proxy/wg_route_exclude_test.go`:

```go
// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"slices"
	"testing"
)

// Без исключения собственный UDP узла входит в TUN и выпускается обратно
// правилом маршрутизации, совпадающим с адресом сервера, — каждый байт
// пересекает инбаунд дважды: как полезная нагрузка и как несущий её
// зашифрованный пакет. На 1.13 это тратило работу, на sing-tun 0.9 это душит
// сессию: загрузки не стартуют, мелкие запросы истекают, tcp_established не
// уходит с нуля.
func TestWireGuardServerAddressIsExcludedFromTun(t *testing.T) {
	for _, pt := range []string{"WIREGUARD", "AMNEZIAWG"} {
		in := tunInboundFor(t, pt)
		if !slices.Contains(in.RouteExcludeAddress, "203.0.113.7/32") {
			t.Errorf("%s: route_exclude_address = %v, нет адреса узла 203.0.113.7/32",
				pt, in.RouteExcludeAddress)
		}
	}
}

// Прежнее поведение остальных протоколов не меняется — оно и было правильным.
func TestPlainNodeAddressStaysExcluded(t *testing.T) {
	in := tunInboundFor(t, "VLESS")
	if !slices.Contains(in.RouteExcludeAddress, "203.0.113.7/32") {
		t.Errorf("route_exclude_address = %v, нет адреса узла", in.RouteExcludeAddress)
	}
}
```

- [ ] **Step 2: Убедиться, что первый тест падает, а второй проходит**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run 'ServerAddressIsExcluded|PlainNodeAddressStays' -v
```

Ожидается: `TestWireGuardServerAddressIsExcludedFromTun` FAIL (для обоих протоколов список пуст), `TestPlainNodeAddressStaysExcluded` PASS. Если второй тоже падает — значит сломано что-то другое, остановиться и разобраться, не правя код этой задачи.

- [ ] **Step 3: Снять условие на тип протокола**

В `BuildTunnelModeConfig` заменить блок:

```go
	var routeExclude []string
	if pt != "WIREGUARD" && pt != "AMNEZIAWG" {
		if serverIP := net.ParseIP(cfg.Proxy.IP); serverIP != nil {
			cidr := cfg.Proxy.IP + "/32"
			if serverIP.To4() == nil {
				cidr = cfg.Proxy.IP + "/128"
			}
			routeExclude = append(routeExclude, cidr)
		}
	}
```

на:

```go
	// Адрес узла исключается из TUN для ЛЮБОГО протокола, включая WireGuard и
	// AmneziaWG. Раньше они были вычтены из этого правила, и на ядре 1.14 это
	// клало туннель: без исключения собственный UDP узла входит в TUN и
	// выпускается обратно правилом, совпадающим с адресом сервера, — ядро
	// говорит это прямым текстом, "inbound packet connection to <server>:...",
	// — так что каждый байт пересекает инбаунд дважды: как полезная нагрузка и
	// как несущий её зашифрованный пакет. На 1.13 это лишь тратило работу;
	// sing-tun 0.9 перестроил обе таблицы NAT и поставил диспетчер потоков
	// перед каждым пакетом, и та же петля теперь душит сессию.
	//
	// Цена у исключения есть: узел, который CDN увёл бы на бэкенд вне
	// известного адреса, отправил бы своё рукопожатие в туннель, который сам же
	// и поднимает. Этот риск идентичен для всех остальных протоколов здесь, и
	// они живут с этим исключением давно.
	var routeExclude []string
	if serverIP := net.ParseIP(cfg.Proxy.IP); serverIP != nil {
		cidr := cfg.Proxy.IP + "/32"
		if serverIP.To4() == nil {
			cidr = cfg.Proxy.IP + "/128"
		}
		routeExclude = append(routeExclude, cidr)
	}
```

- [ ] **Step 4: Прогнать тесты**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run 'ServerAddressIsExcluded|PlainNodeAddressStays' -v
go test -tags="${TAGS_PLAY}" ./internal/proxy/ -run 'ServerAddressIsExcluded|PlainNodeAddressStays' -v
go test -tags="${TAGS_FULL}" ./... 2>&1 | grep -v '^ok' | head -30
go test -tags="${TAGS_PLAY}" ./... 2>&1 | grep -v '^ok' | head -30
```

Ожидается: оба PASS в обеих конфигурациях. Отдельно проверить, что не упали существующие тесты WG-маршрутизации — если упали, читать их: возможно, они фиксировали именно прежнее (теперь неверное) поведение, и тогда правится тест с объяснением, а не код.

- [ ] **Step 5: Коммит**

```bash
git add internal/proxy/engine.go internal/proxy/wg_route_exclude_test.go
git commit -m "fix(proxy): адрес WireGuard-узла исключается из туннеля

WG и AWG были вычтены из правила, исключающего адрес сервера из TUN. На 1.13
петля «узел внутри собственного туннеля» лишь тратила работу: каждый байт шёл
через инбаунд дважды — как нагрузка и как несущий её пакет. sing-tun 0.9
перестроил обе таблицы NAT и поставил диспетчер потоков перед каждым пакетом,
и та же петля стала душить сессию.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 5: Потолок таблицы UDP NAT

Таймаут ограничивает, сколько мёртвый поток лежит; он не ограничивает, как быстро таблица растёт. Под штормом ретраев она растёт быстрее, чем дренируется.

**Files:**
- Modify: `internal/proxy/engine.go` (`SBInbound`; `BuildTunnelModeConfig`)
- Modify: `internal/proxy/engine_tun_1_14_test.go`

**Interfaces:**
- Consumes: `tun` — переменная инбаунда из задачи 3.
- Produces: `SBInbound.UDPNATMax uint32`.

- [ ] **Step 1: Дописать падающий тест**

В `internal/proxy/engine_tun_1_14_test.go` добавить:

```go
// Потолок ставится только обычным узлам. WG и AWG держат своё состояние сессии,
// и голодание таблицы инбаунда у них — та же ошибка, что однажды уронила живой
// туннель таймаутом.
func TestUDPNATCeilingIsSetForPlainNodesOnly(t *testing.T) {
	if in := tunInboundFor(t, "VLESS"); in.UDPNATMax != 8192 {
		t.Errorf("udp_nat_max = %d, ожидалось 8192", in.UDPNATMax)
	}
	for _, pt := range []string{"WIREGUARD", "AMNEZIAWG"} {
		if in := tunInboundFor(t, pt); in.UDPNATMax != 0 {
			t.Errorf("%s: udp_nat_max = %d, у эндпоинтов потолок не ставится", pt, in.UDPNATMax)
		}
	}
}
```

- [ ] **Step 2: Убедиться, что тест падает**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run TestUDPNATCeiling 2>&1 | head -10
```

Ожидается: FAIL, `in.UDPNATMax undefined`.

- [ ] **Step 3: Добавить поле и значение**

В `type SBInbound struct`, следом за `UDPFiltering`:

```go
	// UDPNATMax ограничивает, сколько слотов UDP NAT инбаунд TUN держит
	// одновременно. Таймаут ограничивает только время жизни мёртвого потока;
	// под штормами ретраев, в которых этот клиент живёт — браузер
	// переоткрывает рукопожатия QUIC, которые роняют на UDP/443, — таблица
	// растёт быстрее, чем дренируется. Эндпоинтам не ставится: они держат своё
	// состояние сессии, и голодание таблицы инбаунда у них — та же ошибка, что
	// однажды уронила живой туннель таймаутом.
	UDPNATMax uint32 `json:"udp_nat_max,omitempty"`
```

В `BuildTunnelModeConfig`, в ветку обычных протоколов рядом с `UDPMapping`:

```go
		// 8192 — отправная величина, а не измеренная: она много выше того, что
		// держит обычный сёрфинг, поэтому потолок кусается только в шторм. Если
		// живой трафик начнёт в него упираться — UDP отваливается, а TCP цел —
		// поднять и записать, что заставило.
		tun.UDPNATMax = 8192
```

- [ ] **Step 4: Прогнать тесты**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run 'TestTun|TestUDPNAT' -v
go test -tags="${TAGS_PLAY}" ./internal/proxy/ -run 'TestTun|TestUDPNAT' -v
```

Ожидается: все PASS в обеих конфигурациях.

- [ ] **Step 5: Коммит**

```bash
git add internal/proxy/engine.go internal/proxy/engine_tun_1_14_test.go
git commit -m "feat(proxy): потолок таблицы UDP NAT на TUN-инбаунде

Таймаут ограничивает время жизни мёртвого потока, но не скорость роста таблицы.
Под штормом QUIC-ретраев она растёт быстрее, чем дренируется. Эндпоинтам
потолок не ставится — они держат своё состояние сессии.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 6: Оптимистичный DNS-кэш

**Files:**
- Modify: `internal/proxy/engine.go` (`SBDNS`; `buildDNS` — оба места, где собирается `&SBDNS{...}`)
- Create: `internal/proxy/engine_dns_optimistic_test.go`

**Interfaces:**
- Consumes: `buildDNS(cfg EngineConfig) *SBDNS`.
- Produces: `newSBDNS(servers []SBDNSServer) *SBDNS`; тип `SBDNSOptimistic`. Задачи 7 и 8 собирают DNS через `newSBDNS`, а не литералом.

- [ ] **Step 1: Написать падающий тест**

Создать `internal/proxy/engine_dns_optimistic_test.go`:

```go
// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import "testing"

// Окно названо, а не унаследовано: ядро отдавало бы протухший ответ трое суток,
// что переживает любую смену сети. Шесть часов покрывают ночь со спящим
// телефоном, а переход между Wi-Fi и мобильной сетью обновляет кэш задолго до
// исхода окна.
func TestDNSCacheIsOptimisticWithSixHourWindow(t *testing.T) {
	for _, mode := range []ProxyMode{ProxyModeTunnel, ProxyModeProxy} {
		dns := buildDNS(EngineConfig{
			Mode:  mode,
			Proxy: ProxyConfig{Type: "VLESS", IP: "203.0.113.7", Port: 443},
		})
		if dns == nil {
			t.Fatalf("%v: buildDNS вернул nil", mode)
		}
		if dns.Optimistic == nil {
			t.Fatalf("%v: optimistic не задан — резолвер будет блокироваться на протухшей записи", mode)
		}
		if !dns.Optimistic.Enabled {
			t.Errorf("%v: optimistic.enabled = false", mode)
		}
		if dns.Optimistic.Timeout != "6h" {
			t.Errorf("%v: optimistic.timeout = %q, ожидалось 6h", mode, dns.Optimistic.Timeout)
		}
	}
}
```

- [ ] **Step 2: Убедиться, что тест падает**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run TestDNSCacheIsOptimistic 2>&1 | head -10
```

Ожидается: FAIL, `dns.Optimistic undefined`.

- [ ] **Step 3: Добавить поле, тип и конструктор**

В `type SBDNS struct` (`engine.go:150`), после `Strategy`:

```go
	// Optimistic меняет немного протухшести на резолвер, который никогда не
	// блокируется на истёкшей записи. Задаётся только через newSBDNS.
	Optimistic *SBDNSOptimistic `json:"optimistic,omitempty"`
```

Следом за объявлением `SBDNS`:

```go
// SBDNSOptimistic настраивает оптимистичный DNS-кэш sing-box 1.14: истёкшая
// запись отдаётся сразу, а обновление идёт фоном. Ядро отвергает эту опцию
// вместе с disable_cache и disable_expire — ни ту, ни другую клиент не эмитит.
type SBDNSOptimistic struct {
	Enabled bool   `json:"enabled,omitempty"`
	Timeout string `json:"timeout,omitempty"`
}

// newSBDNS собирает блок DNS так, чтобы общие для всех режимов опции нельзя
// было забыть в одном из выходов buildDNS.
//
// Окно названо, а не оставлено на дефолт: ядро отдавало бы протухший ответ трое
// суток, что переживает любую смену сети, которую делает человек. Шесть часов
// покрывают ночь со спящим телефоном, а переход между Wi-Fi и мобильной сетью
// обновляет кэш задолго до исхода окна.
func newSBDNS(servers []SBDNSServer) *SBDNS {
	return &SBDNS{
		Servers:    servers,
		Optimistic: &SBDNSOptimistic{Enabled: true, Timeout: "6h"},
	}
}
```

- [ ] **Step 4: Перевести buildDNS на конструктор**

Найти все места, где собирается блок DNS:

```bash
grep -n '&SBDNS{' internal/proxy/engine.go
```

Каждое `&SBDNS{Servers: servers}` (в том числе многострочное на строке 686) заменить на `newSBDNS(servers)`. Если у литерала были заданы другие поля (`Final`, `Strategy`, `Rules`), они присваиваются после вызова, как и раньше, — конструктор задаёт только `Servers` и `Optimistic`.

- [ ] **Step 5: Прогнать тесты**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run TestDNSCacheIsOptimistic -v
go test -tags="${TAGS_PLAY}" ./internal/proxy/ -run TestDNSCacheIsOptimistic -v
go test -tags="${TAGS_FULL}" ./... 2>&1 | grep -v '^ok' | head -30
go test -tags="${TAGS_PLAY}" ./... 2>&1 | grep -v '^ok' | head -30
```

Ожидается: PASS в обеих конфигурациях, остальное без новых падений.

- [ ] **Step 6: Коммит**

```bash
git add internal/proxy/engine.go internal/proxy/engine_dns_optimistic_test.go
git commit -m "feat(proxy): оптимистичный DNS-кэш с окном 6 часов

Истёкшая запись отдаётся сразу, обновление идёт фоном — на мобильной сети с
частыми разрывами это основной выгодоприобретатель. Окно названо, а не оставлено
на дефолт: ядро держало бы протухший ответ трое суток.

Блок DNS теперь собирается конструктором newSBDNS, чтобы общие опции нельзя
было забыть в одном из выходов buildDNS.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 7: Адрес узла резолвится названным сервером

На 1.14 дайлер для сервера, адресованного именем, без названного резолвера объявлен удалённым. На форке откат ещё жив, но опираться на него — значит везти на телефон мину.

**Files:**
- Modify: `internal/proxy/engine.go` (`SBDNSServer` — строка 160; `SBOutbound` — 189; `SBEndpoint` — 257; `buildOutbounds` — 604; `BuildProxyModeConfig` — 503; `BuildTunnelModeConfig` — 529)
- Modify: `internal/proxy/endpoints.go` (`buildEndpoints` — строка 31; функция живёт здесь, а не в `engine.go`)
- Create: `internal/proxy/engine_domain_resolver_test.go`

**Interfaces:**
- Consumes: `buildOutbounds(proxy ProxyConfig) []SBOutbound`, `buildEndpoints(proxy ProxyConfig) []SBEndpoint` — обе меняют сигнатуру.
- Produces: `serverDomainResolverTag(proxy ProxyConfig, mode ProxyMode, dns *SBDNS) string`; поля `SBOutbound.DomainResolver`, `SBEndpoint.DomainResolver`, `SBDNSServer.DomainResolver`; новые сигнатуры `buildOutbounds(proxy ProxyConfig, domainResolver string)` и `buildEndpoints(proxy ProxyConfig, domainResolver string)`.

- [ ] **Step 1: Написать падающие тесты**

Создать `internal/proxy/engine_domain_resolver_test.go`:

```go
// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import "testing"

func proxyOutboundFor(t *testing.T, sb SingBoxConfig) SBOutbound {
	t.Helper()
	for _, out := range sb.Outbounds {
		if out.Tag == "proxy" {
			return out
		}
	}
	t.Fatal("в конфиге нет аутбаунда с тегом proxy")
	return SBOutbound{}
}

// Узел, адресованный именем, на 1.14 не получает дайлера, пока ему не назван
// резолвер. В туннельном режиме ответ повторяет правило, которое buildDNS уже
// эмитит для того же домена: системный резолвер. Он не должен ехать по
// туннелю — этот туннель и открывается данным вызовом.
func TestDomainAddressedNodeNamesItsResolverInTunnel(t *testing.T) {
	sb := BuildTunnelModeConfig(EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{Type: "VLESS", IP: "node.example.com", Port: 443},
	})
	if got := proxyOutboundFor(t, sb).DomainResolver; got != "local" {
		t.Errorf("domain_resolver = %q, ожидалось local", got)
	}
}

// У литерального адреса резолвить нечего, и ядро вообще не строит resolve-дайлер
// — поле остаётся пустым, иначе оно назвало бы сервер без нужды.
func TestLiteralAddressedNodeNamesNoResolver(t *testing.T) {
	sb := BuildTunnelModeConfig(EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{Type: "VLESS", IP: "203.0.113.7", Port: 443},
	})
	if got := proxyOutboundFor(t, sb).DomainResolver; got != "" {
		t.Errorf("domain_resolver = %q, у литерального адреса поле должно быть пустым", got)
	}
}

// В proxy-режиме нет TUN, системный резолвер никуда не перенаправлен, и
// назвать "local" значило бы отдать домен узла провайдеру открытым текстом.
// Тег — тот, который ядро выбрало бы само: первый зарегистрированный транспорт.
func TestDomainAddressedNodeUsesFirstTransportInProxyMode(t *testing.T) {
	cfg := EngineConfig{
		Mode:  ProxyModeProxy,
		Proxy: ProxyConfig{Type: "VLESS", IP: "node.example.com", Port: 443},
	}
	sb := BuildProxyModeConfig(cfg)
	if sb.DNS == nil || len(sb.DNS.Servers) == 0 {
		t.Fatal("в proxy-режиме пустой список серверов DNS — тег назвать не из чего")
	}
	want := sb.DNS.Servers[0].Tag
	if got := proxyOutboundFor(t, sb).DomainResolver; got != want {
		t.Errorf("domain_resolver = %q, ожидалось %q", got, want)
	}
}

// route.default_domain_resolver не задаётся намеренно. Названный там резолвер
// увёл бы каждый внутренний резолв прямо в этот транспорт, мимо dns.rules, — а
// обход правил и есть то, чем заблокированный домен резолвится через туннель,
// а не через цензурируемый локальный резолвер.
func TestRouteNeverNamesADefaultDomainResolver(t *testing.T) {
	sb := BuildTunnelModeConfig(EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{Type: "VLESS", IP: "node.example.com", Port: 443},
	})
	data, err := marshalConfigForTest(sb)
	if err != nil {
		t.Fatalf("маршалинг конфига: %v", err)
	}
	if containsForTest(data, "default_domain_resolver") {
		t.Error("в конфиге появился default_domain_resolver — внутренние резолвы пойдут мимо dns.rules")
	}
}
```

Вспомогательные функции `marshalConfigForTest` и `containsForTest` объявить в том же файле:

```go
func marshalConfigForTest(sb SingBoxConfig) (string, error) {
	b, err := json.MarshalIndent(sb, "", "  ")
	return string(b), err
}

func containsForTest(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
```

с импортами `"encoding/json"` и `"strings"`.

- [ ] **Step 2: Убедиться, что тесты падают**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run 'DomainResolver|DomainAddressedNode|LiteralAddressedNode|RouteNeverNames' 2>&1 | head -15
```

Ожидается: FAIL, `out.DomainResolver undefined`.

- [ ] **Step 3: Добавить поля в структуры**

`SBOutbound` (`engine.go:189`) — первым полем после `ServerPort`:

```go
	// DomainResolver называет сервер DNS, который резолвит Server, когда тот
	// домен, а не литерал. Какой именно тег сюда попадает и почему поле вообще
	// существует — см. serverDomainResolverTag. Для литерального адреса пусто:
	// ядро тогда не строит resolve-дайлер вовсе.
	DomainResolver string `json:"domain_resolver,omitempty"`
```

То же поле с тем же комментарием (сокращённым до ссылки на `serverDomainResolverTag`) добавить в `SBEndpoint` — найти его объявление:

```bash
grep -n 'type SBEndpoint struct' -A20 internal/proxy/engine.go
```

И в `SBDNSServer` — для случая, когда пользователь задал собственный DNS по имени:

```go
	// DomainResolver поднимает резолвер, который сам адресован именем. Без
	// детура, за которым можно спрятаться, sing-box 1.14 отказывается строить
	// для такого сервера дайлер вообще — "missing domain resolver for domain
	// server address", — так что это не предупреждение, а разница между
	// движком, который стартует, и движком, который нет.
	DomainResolver string `json:"domain_resolver,omitempty"`
```

- [ ] **Step 4: Добавить serverDomainResolverTag**

В `engine.go`, рядом с `buildOutbounds`:

```go
// serverDomainResolverTag называет сервер DNS, который обязан ответить за
// собственный адрес узла, — для поля domain_resolver на том аутбаунде или
// эндпоинте, который этот адрес набирает. Пусто, когда узел задан литералом:
// резолвить нечего, и ядро не строит resolve-дайлер.
//
// Зачем поле. sing-box 1.14 объявил удалённым набор домена без названного
// резолвера. На форке откат ещё жив и, более того, именно на его семантике
// стоит остальной конфиг: откат идёт по dns.rules, а это и есть то, чем
// заблокированный домен уходит через туннель. Поэтому
// route.default_domain_resolver намеренно не задаётся (см.
// TestRouteNeverNamesADefaultDomainResolver), а узел — единственный набор, где
// правильный ответ известен до всяких правил, — называется явно.
//
// Какой тег. В туннельном режиме ответ повторяет правило, которое buildDNS уже
// эмитит для этого же домена: системный резолвер. Он не должен ехать по
// туннелю — туннель и открывается этим набором.
//
// В proxy-режиме нет TUN, системный резолвер никуда не перенаправлен, и "local"
// отдал бы домен узла провайдеру открытым текстом. Тег — тот, что ядро выбрало
// бы само: первый зарегистрированный транспорт. Включение поля там не меняет
// ничего, кроме снятия deprecation.
func serverDomainResolverTag(proxy ProxyConfig, mode ProxyMode, dns *SBDNS) string {
	if proxy.IP == "" || net.ParseIP(proxy.IP) != nil {
		return ""
	}
	if mode == ProxyModeTunnel {
		return "local"
	}
	if dns != nil && len(dns.Servers) > 0 {
		return dns.Servers[0].Tag
	}
	return ""
}
```

- [ ] **Step 5: Протянуть тег через сборщики**

Сигнатуры:

```go
func buildOutbounds(proxy ProxyConfig, domainResolver string) []SBOutbound
func buildEndpoints(proxy ProxyConfig, domainResolver string) []SBEndpoint
```

В `buildOutbounds` тег ставится **только** на аутбаунд узла (тег `proxy`); `direct` набирает всё, что ему отдаёт роутер, и единственного правильного резолвера у него нет — он остаётся на обходе правил осознанно. В `buildEndpoints` — на эндпоинт узла.

В `BuildTunnelModeConfig` тег вычисляется до сборки DNS (он не зависит от списка серверов):

```go
	nodeResolver := serverDomainResolverTag(cfg.Proxy, ProxyModeTunnel, nil)
	outbounds := buildOutbounds(cfg.Proxy, nodeResolver)
```

и `Endpoints: buildEndpoints(cfg.Proxy, nodeResolver)`.

В `BuildProxyModeConfig` — наоборот, после сборки DNS, потому что тег берётся из списка серверов:

```go
	dns := buildDNS(cfg)
	nodeResolver := serverDomainResolverTag(cfg.Proxy, ProxyModeProxy, dns)
```

и далее `DNS: dns`, `Outbounds: buildOutbounds(cfg.Proxy, nodeResolver)`, `Endpoints: buildEndpoints(cfg.Proxy, nodeResolver)`.

Найти все остальные вызовы и поправить:

```bash
grep -rn 'buildOutbounds(\|buildEndpoints(' internal/ mobile/
```

- [ ] **Step 6: Прогнать тесты**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run 'DomainAddressedNode|LiteralAddressedNode|RouteNeverNames' -v
go test -tags="${TAGS_PLAY}" ./internal/proxy/ -run 'DomainAddressedNode|LiteralAddressedNode|RouteNeverNames' -v
go test -tags="${TAGS_FULL}" ./... 2>&1 | grep -v '^ok' | head -30
go test -tags="${TAGS_PLAY}" ./... 2>&1 | grep -v '^ok' | head -30
```

Ожидается: четыре теста PASS в обеих конфигурациях.

- [ ] **Step 7: Коммит**

```bash
git add internal/proxy/engine.go internal/proxy/engine_domain_resolver_test.go
git commit -m "fix(proxy): адрес узла резолвится названным сервером

1.14 объявила удалённым набор домена без названного резолвера. На форке откат
ещё жив, и на его семантике стоит остальной конфиг — откат идёт по dns.rules,
чем заблокированный домен и уходит через туннель. Поэтому
route.default_domain_resolver намеренно не задан, а узел — единственный набор,
где правильный ответ известен заранее, — назван явно: системный резолвер в
туннеле, первый транспорт в proxy-режиме.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 8: DNS через узел идёт по DoH

Самое опасное место бампа для этой ветки. Туннельный DNS на Android — это серверы `type: "tcp"` через детур `proxy` (`engine.go:658-683`), ровно тот транспорт, который на 1.14 замолкает.

**Files:**
- Modify: `internal/proxy/engine.go` (`SBDNSServer`; `buildDNS`, туннельная ветка)
- Create: `internal/proxy/engine_dns_multiplexer_test.go`

**Interfaces:**
- Consumes: `buildDNS`; `newSBDNS` из задачи 6.
- Produces: `tunnelDNSResolver(tag, server string, port int, detour string) []SBDNSServer`; поля `SBDNSServer.Servers`, `.Strategy`, `.Timeout`, приватное `throughDetour`.

- [ ] **Step 1: Написать падающий тест**

Создать `internal/proxy/engine_dns_multiplexer_test.go`:

```go
// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import "testing"

// sing-box 1.14 поставил мультиплексор запросов перед транспортами tcp, tls и
// udp: как только фоновая проба решает, что резолвер поддерживает переиспользование,
// все запросы переезжают на одно общее долгоживущее соединение, чья проверка
// живости — `conn != nil`. Через прокси-аутбаунд это соединение заклинивает, и
// запросы перестают возвращаться совсем: ни ответа, ни ошибки. Транспорт https
// — единственный удалённый, которого мультиплексор не касается, поэтому DoH
// идёт первым.
func TestTunnelDNSLeadsWithDoH(t *testing.T) {
	dns := buildDNS(EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{Type: "VLESS", IP: "203.0.113.7", Port: 443},
	})
	if dns == nil {
		t.Fatal("buildDNS вернул nil")
	}

	var sawHTTPS, sawFallback bool
	for _, s := range dns.Servers {
		if s.Detour == "" {
			continue // local и прочие нетуннельные — не про этот тест
		}
		switch s.Type {
		case "https":
			sawHTTPS = true
		case "fallback":
			sawFallback = true
			if len(s.Servers) < 2 {
				t.Errorf("fallback %q ссылается на %d сервер(ов), ожидалось минимум два", s.Tag, len(s.Servers))
			}
			if s.Timeout == "" {
				t.Errorf("fallback %q без таймаута — заклинившая нога будет висеть вечно", s.Tag)
			}
		}
	}
	if !sawHTTPS {
		t.Error("среди туннельных серверов DNS нет ни одного https — на 1.14 tcp через прокси замолкает")
	}
	if !sawFallback {
		t.Error("нет обёртки fallback — правила некуда направлять")
	}
}

// Каждое правило DNS обязано указывать на существующий тег. Тег, которого нет
// в списке серверов, — это мёртвый движок, а не предупреждение.
func TestEveryDNSRuleNamesARegisteredServer(t *testing.T) {
	dns := buildDNS(EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{Type: "VLESS", IP: "node.example.com", Port: 443},
	})
	if dns == nil {
		t.Fatal("buildDNS вернул nil")
	}
	tags := map[string]bool{}
	for _, s := range dns.Servers {
		tags[s.Tag] = true
	}
	for i, r := range dns.Rules {
		if r.Server == "" {
			continue
		}
		if !tags[r.Server] {
			t.Errorf("правило %d указывает на сервер %q, которого нет в списке", i, r.Server)
		}
	}
	if dns.Final != "" && !tags[dns.Final] {
		t.Errorf("final указывает на сервер %q, которого нет в списке", dns.Final)
	}
}
```

- [ ] **Step 2: Убедиться, что первый тест падает**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run 'TestTunnelDNSLeadsWithDoH|TestEveryDNSRuleNames' -v 2>&1 | head -20
```

Ожидается: `TestTunnelDNSLeadsWithDoH` FAIL («нет ни одного https», «нет обёртки fallback»). `TestEveryDNSRuleNamesARegisteredServer` должен пройти уже сейчас — он пломба, которая обязана остаться зелёной после правки.

- [ ] **Step 3: Добавить поля fallback-сервера**

В `type SBDNSServer struct`, после `Detour`:

```go
	// Servers/Strategy/Timeout настраивают сервер типа "fallback": своего
	// адреса у него нет, только теги серверов, которые пробуются по порядку,
	// каждый в границах Timeout. См. tunnelDNSResolver.
	Servers  []string `json:"servers,omitempty"`
	Strategy string   `json:"strategy,omitempty"`
	Timeout  string   `json:"timeout,omitempty"`

	// throughDetour помечает fallback-обёртку, все ноги которой идут через
	// этот детур. Никогда не сериализуется — ядро добирается до ног по тегам, —
	// нужен для того, чтобы поиск по детуру называл обёртку, а не одну из ног.
	throughDetour string `json:"-"`
```

- [ ] **Step 4: Добавить tunnelDNSResolver**

Рядом с `buildDNS`:

```go
// tunnelDNSResolver эмитит серверы, несущие один резолвер через туннель: DoH
// первым, обычный DNS-over-TCP вторым и обёртку "fallback", на которую
// указывают правила.
//
// Раньше это был один сервер DNS-over-TCP, и на ядре 1.14 он перестал отвечать.
// sing-box 1.14 поставил мультиплексор запросов (dns/transport/multiplexer.go)
// перед транспортами tcp, tls и udp: как только фоновая проба решает, что
// резолвер поддерживает переиспользование, все запросы переезжают на одно
// общее долгоживущее соединение, чья проверка живости — `conn != nil`. Через
// прокси-аутбаунд это соединение заклинивает: запросы уходят и не возвращаются
// — ни ответа, ни ошибки, ни строчки «lookup failed», — пока движок не
// остановят.
//
// Транспорт https — единственный удалённый, которого мультиплексор не касается,
// поэтому DoH идёт первым. Нога tcp остаётся, а не удаляется: у резолвера, не
// умеющего DoH, иначе не было бы пути вовсе. Обёртка ограничивает каждую ногу
// таймаутом, так что даже заклинившая нога стоит одного таймаута, а не вечного
// ожидания.
//
// port относится только к ноге tcp. DoH — это HTTPS, ему нужен 443, поэтому
// резолвер, прибитый к нестандартному порту DNS, сохраняет этот порт на ноге
// tcp, пока DoH пробует стандартный и, не преуспев, передаёт ход.
func tunnelDNSResolver(tag, server string, port int, detour string) []SBDNSServer {
	return []SBDNSServer{
		{Type: "https", Tag: tag + "-doh", Server: server, Detour: detour},
		{Type: "tcp", Tag: tag + "-tcp", Server: server, ServerPort: port, Detour: detour},
		{
			Type:          "fallback",
			Tag:           tag,
			Servers:       []string{tag + "-doh", tag + "-tcp"},
			Strategy:      "sequential",
			Timeout:       "5s",
			throughDetour: detour,
		},
	}
}
```

- [ ] **Step 5: Перевести туннельную ветку buildDNS на новый резолвер**

В `buildDNS`, в ветке `cfg.Mode == ProxyModeTunnel`:

- пользовательские серверы: вместо одного `SBDNSServer{Type: srvType, Tag: fmt.Sprintf("custom-%d", i+1), ...}` — `servers = append(servers, tunnelDNSResolver(fmt.Sprintf("custom-%d", i+1), server, port, detour)...)`. Переменная `srvType` и её вычисление уходят: тип теперь задаёт `tunnelDNSResolver`;
- дефолтные серверы: пары `google-tcp` / `cloudflare-tcp` заменяются на `tunnelDNSResolver("google", "8.8.8.8", 0, detour)` и `tunnelDNSResolver("cloudflare", "1.1.1.1", 0, detour)`. Существующие серверы `type: "tls"` (`google-tls`, `cloudflare-tls`) **удаляются**: tls — ровно тот транспорт, который мультиплексор и заклинивает, и после появления ноги DoH они лишние;
- сервер `{Type: "local", Tag: "local"}` остаётся как был — он не через детур и мультиплексора не касается;
- порядок обязателен: обёртка эмитится **после** своих ног, ядро разрешает members по тегу в момент конструирования.

Найти места, где тег туннельного резолвера называется в правилах, и убедиться, что они указывают на обёртку, а не на ногу:

```bash
grep -n 'firstUpstreamDNSTag\|Server: *"google\|Server: *"cloudflare\|Server: *"custom' internal/proxy/engine.go
```

`firstUpstreamDNSTag` (`engine.go:842`) сейчас возвращает тег первого сервера, у которого `Type != "local"`. После правки это будет `google-doh` — нога, а не обёртка, и правило ad-block, направленное на ногу, потеряло бы вторую. Заменить тело на поиск обёртки с откатом на прежнее поведение:

```go
// firstUpstreamDNSTag называет сервер, на который указывают правила,
// отправляющие домен «вверх по течению». Обёртка fallback предпочитается её
// ногам: нога — это один транспорт из двух, и правило, указавшее на неё,
// потеряло бы второй. Откат на первый нелокальный сервер сохранён для
// конфигураций, где обёртки нет вовсе (proxy-режим без детура).
func firstUpstreamDNSTag(servers []SBDNSServer) string {
	for _, s := range servers {
		if s.Type == "fallback" && s.Tag != "" {
			return s.Tag
		}
	}
	for _, s := range servers {
		if s.Type != "local" && s.Tag != "" {
			return s.Tag
		}
	}
	return ""
}
```

- [ ] **Step 6: Прогнать тесты**

```bash
go test -tags="${TAGS_FULL}" ./internal/proxy/ -run 'TestTunnelDNSLeadsWithDoH|TestEveryDNSRuleNames' -v
go test -tags="${TAGS_PLAY}" ./internal/proxy/ -run 'TestTunnelDNSLeadsWithDoH|TestEveryDNSRuleNames' -v
go test -tags="${TAGS_FULL}" ./... 2>&1 | grep -v '^ok' | head -40
go test -tags="${TAGS_PLAY}" ./... 2>&1 | grep -v '^ok' | head -40
```

Ожидается: оба PASS в обеих конфигурациях. Тесты ad-block, которые называют теги DNS, — главные кандидаты на падение: если упали, читать, на какой тег они рассчитывают, и чинить тег, а не тест.

- [ ] **Step 7: Коммит**

```bash
git add internal/proxy/engine.go internal/proxy/engine_dns_multiplexer_test.go
git commit -m "fix(proxy): DNS через узел идёт по DoH

Туннельный DNS на этой ветке — серверы type: tcp через детур proxy, ровно тот
транспорт, который 1.14 закрыла мультиплексором запросов: после фоновой пробы
все запросы переезжают на одно долгоживущее соединение с проверкой живости
`conn != nil`, и через прокси-аутбаунд оно заклинивает — запросы уходят и не
возвращаются совсем.

https — единственный удалённый транспорт, которого мультиплексор не касается,
поэтому DoH ведёт, нога tcp остаётся для резолверов без DoH, а обёртка fallback
ограничивает каждую ногу таймаутом. Серверы type: tls убраны — это тот же
мультиплексируемый транспорт, и после DoH они лишние.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 9: Сборка, выравнивание и приёмка на телефоне

Go-тесты не доказывают, что ядро поднимается на устройстве. Здесь блок либо закрывается, либо откатывается.

**Files:**
- Изменяется: `android/libs/libbox-full.aar`, `android/libs/libbox-play.aar` (артефакты, в git не идут)
- Modify: `docs/android-pc-sync-and-play-spec.md` (запись о закрытии), `docs/superpowers/specs/2026-09-17-android-core-1.14-sync-design.md` (статус блока 1)

**Interfaces:**
- Consumes: всё, что сделали задачи 2–8.
- Produces: решение «блок 1 принят» — без него задачи блока 2 не начинаются.

- [ ] **Step 1: Собрать оба AAR**

```bash
cd /c/ResultV
DIST=full ./scripts/build-android-aar.sh
DIST=play ./scripts/build-android-aar.sh
ls -la android/libs/
```

Ожидается: оба скрипта завершаются с кодом 0, в `android/libs/` лежат свежие `libbox-full.aar` и `libbox-play.aar`.

- [ ] **Step 2: Проверить выравнивание 16 КБ в обоих артефактах**

```bash
cd /c/Users/andbe/AppData/Local/Temp/claude/C--ResultV/*/scratchpad
for d in full play; do
  rm -rf aar_$d && mkdir -p aar_$d && cd aar_$d
  unzip -oq /c/ResultV/android/libs/libbox-$d.aar
  echo "=== $d arm64-v8a ==="
  readelf -lW jni/arm64-v8a/libgojni.so | grep -E '^\s+LOAD' | head -5
  cd ..
done
```

Ожидается: у каждого сегмента `LOAD` выравнивание `0x4000`. Если хоть у одного `0x1000` — блок не принят; разбираться с флагом `-extldflags=-Wl,-z,max-page-size=16384` и версией NDK, а не идти дальше.

- [ ] **Step 3: Подтвердить, что в play-сборке по-прежнему нет фильтрации**

```bash
cd /c/ResultV
go list -deps -tags="${TAGS_PLAY}" ./mobile/ | grep -c 'internal/filter' || echo "internal/filter отсутствует — верно"
```

Ожидается: `internal/filter отсутствует — верно`. Бамп ядра не должен был вернуть его в граф; если вернул — новая зависимость тянет его транзитивно, и это блокер для Google Play.

- [ ] **Step 4: Собрать и поставить APK на телефон**

```bash
cd /c/ResultV/android
./gradlew assembleFullDebug -Pdebug.abi=arm64-v8a
adb -s e3bacc6b install -r -d app/build/outputs/apk/full/debug/app-full-arm64-v8a-debug.apk
```

Ожидается: `Success`. Приложение **не удалять** — на устройстве лежат рабочие профили.

- [ ] **Step 5: Приёмка на устройстве**

Прогнать по пунктам, записывая результат каждого:

1. Подключение к рабочему серверу VLESS — соединение поднимается, сайты открываются.
2. Индикатор скорости показывает ненулевые цифры под нагрузкой (проверяет `RoutedFlow` на обычном узле).
3. Подключение к профилю WireGuard или AmneziaWG — соединение поднимается, **загрузка большого файла идёт и не встаёт** (проверяет задачу 4: до неё на 1.14 сессия душила сама себя).
4. Индикатор скорости на WG-профиле ненулевой (проверяет `RoutedFlow` на эндпоинте — именно тот путь, где `RoutedConnection` не вызывается вовсе).
5. DNS: открыть сайт по имени, которого нет в кэше, на WG-профиле (проверяет задачу 8 — до неё на 1.14 запросы уходили и не возвращались).
6. Умный режим и per-app — прежнее поведение.
7. Кил-свитч: включить, оборвать сеть, убедиться, что трафик встал, вернуть сеть (проверяет traffic veto, который кормится счётчиками из `RoutedFlow`).

- [ ] **Step 6: Записать итог в документы**

В `docs/superpowers/specs/2026-09-17-android-core-1.14-sync-design.md` — раздел «Блок 1 закрыт» с таблицей «проверка → результат» по пунктам Step 5 и замерами из Step 2. В `docs/android-pc-sync-and-play-spec.md`, раздел 6, — строка о закрытии блока 1 со ссылкой.

Если хоть один пункт Step 5 не прошёл — блок **не** закрывается: записать симптом, вернуться к systematic-debugging, не начинать блок 2.

- [ ] **Step 7: Коммит и пуш**

```bash
cd /c/ResultV
git add -f docs/android-pc-sync-and-play-spec.md docs/superpowers/specs/2026-09-17-android-core-1.14-sync-design.md
git commit -m "docs(android): блок 1 закрыт — ядро 1.14 принято на телефоне

Тело: подставить таблицу из шага 6 — семь пунктов приёмки с фактическим
результатом каждого и замеры выравнивания из шага 2.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
git push origin android
```

---

## Что этот план сознательно не делает

- **Не трогает `internal/config`.** Поля пинга и адаптивного Smart приезжают туда в блоках 3 и 4.
- **Не переносит `smartoutbound.go`, `internal/verdict`, FakeIP.** Это блок 4, и восьмиаргументный `box.Context` появляется там же — пока контекст остаётся на `include.Context`.
- **Не переносит AWG 3.1.** Это блок 2; `wireguard-go` поднимается здесь только потому, что от него зависит комбинация версий с ПК, и android-only `ping_wg_handshake.go` проверяется прогоном тестов, а не отдельной задачей.
- **Не добавляет `UDPTimeout` на TUN-инбаунд.** На ПК он есть, на этой ветке его никогда не было; вводить его — отдельное решение с отдельной приёмкой, а не часть бампа.
- **Не трогает gRPC.** Код на ветках идентичен (спека §2).
