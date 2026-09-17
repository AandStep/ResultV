# Блок 4 — адаптивный Smart на Android: план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** телефон сам узнаёт, какие сайты не открываются напрямую, и водит их
через туннель — без списка, который кто-то поддерживает вручную.

**Architecture:** гонка «прямо против туннеля» живёт в HTTP-CONNECT-реле внутри
процесса приложения, куда маршрутные правила отправляют всё, о чём вердикта ещё
нет. Выученное «ходит напрямую» уезжает в плейнтекстовый локальный rule-set,
который ядро перечитывает само (`fswatch`), и мимо реле идёт уже правилом.
Выученное «через туннель» остаётся только в хешированном сторе. Кастомный
аутбаунд ядра, на котором это сделано на ПК, под libbox зарегистрировать негде —
обоснование в §7.0 спеки.

**Tech Stack:** Go 1.26 (`internal/verdict`, `internal/proxy`, `mobile` —
gomobile bind), sing-box 1.14 через libbox, Kotlin/Compose (Android).

**Spec:** `docs/superpowers/specs/2026-09-17-android-core-1.14-sync-design.md`,
разделы 7.0–7.8 (переписаны 2026-09-17) и 12.4 («чему цикл научил про Android»).

## Global Constraints

- Ветка `android`, работа прямо в `C:\ResultV`. Точка отката — `origin/android`.
- Go-код собирается и тестируется **в обеих конфигурациях тегов**:
  - full: `$(cat scripts/android-build-tags.txt)` =
    `mobile,with_gvisor,with_utls,with_clash_api,with_quic,with_wireguard,with_grpc`
  - play: те же плюс `no_mitm,no_adblock`
- Экспортируемые символы `mobile` уходят в Kotlin через gomobile: только
  `string`, `int64`, `bool`, `[]byte` или JSON-строка. Никаких map, интерфейсов,
  функций в публичной подписи.
- Правки в Go **не доедут** до телефона через `gradlew assembleDebug` — AAR
  пересобирается только `scripts/build-android-aar.sh`, и перед этим
  `set -a; . /c/ResultV/.env; set +a` (иначе `resultv://` и RVSUB1 не
  расшифруются, и реальные профили не импортируются).
- Приложение исключено из собственного VPN (`BoxModule.kt`, `tryDisallow`).
  Поэтому `net.Dialer` из нашего процесса — настоящий direct, а вход в туннель —
  только через loopback-инбаунд движка.
- Файлы вердиктов: `<dataDir>/smart/verdicts.json` (хеши, формат ПК) и
  `<dataDir>/smart/verdict-direct.json` (rule-set, плейнтекст, `"version": 3`).
- Порты — константы рядом с `BrowserAdBlockSocksPort` (18130):
  `SmartRaceInboundPort = 18131`, `SmartRelayPort = 18132`.
- Теги: инбаунд `smart-race-in`, аутбаунд `smart-relay`, rule-set
  `verdict-direct`.
- Документы в `docs/superpowers/` коммитятся `git add -f` — папка под
  `.gitignore`.
- Сообщения коммитов по-русски, в стиле ветки, с завершающей строкой
  `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>`.

---

### Task 1: `internal/verdict` — перенос и восстановление имён

Директория с ПК самодостаточна: ни строчки про sing-box, только таблица
«имя → решение». Единственная добавка — `Rehydrate`: плейнтекст имён в сторе
сессионный (`store.plain` не сохраняется), а рендер rule-set его требует после
перезапуска.

**Files:**
- Create: `internal/verdict/verdict.go`, `store.go`, `promote.go`, `persist.go`
  (копии с `dev`)
- Create: `internal/verdict/verdict_test.go`, `store_test.go`,
  `promote_test.go`, `persist_test.go` (копии с `dev`)
- Create: `internal/verdict/rehydrate.go`
- Test: `internal/verdict/rehydrate_test.go`

**Interfaces:**
- Consumes: ничего (первая задача).
- Produces: пакет `resultproxy-wails/internal/verdict` —
  `Decision` (`Unknown`/`Direct`/`Proxy`), `Source`, `Record`,
  `TTLProxy`/`TTLDirect`, `NormalizeHost(string) string`,
  `Suffixes(string) []string`, `ParentDomain(string) string`,
  `IPKey(netip.Addr) string`, `New([]byte, func() time.Time) *Store`,
  `NewSalt() ([]byte, error)`, `Load(path string, now func() time.Time) (*Store, error)`,
  `(*Store).Learn(host string, d Decision)`,
  `(*Store).Seed(host string, d Decision, src Source)`,
  `(*Store).Lookup(host string) (Record, bool)`,
  `(*Store).LookupIP(netip.Addr) (Record, bool)`,
  `(*Store).LearnIP(netip.Addr, Decision)`,
  `(*Store).Names() map[string]Record`, `(*Store).Save(path string) error`,
  `(*Store).Prune()`, и новое `(*Store).Rehydrate(names []string) int`.

- [ ] **Шаг 1: перенести директорию с `dev` как есть**

```bash
mkdir -p internal/verdict
for f in verdict.go store.go promote.go persist.go \
         verdict_test.go store_test.go promote_test.go persist_test.go; do
  git show dev:internal/verdict/$f > internal/verdict/$f
done
```

- [ ] **Шаг 2: убедиться, что перенос самодостаточен**

Run: `go test ./internal/verdict/...`
Expected: PASS, все четыре файла тестов зелёные. Если что-то не собирается —
значит директория тянет соседа, и это надо разобрать до следующего шага, а не
дописывать заглушку.

- [ ] **Шаг 3: написать падающий тест на `Rehydrate`**

Create `internal/verdict/rehydrate_test.go`:

```go
package verdict

import (
	"path/filepath"
	"testing"
	"time"
)

// Плейнтекст имён не переживает перезапуск: он лежит в store.plain, а на диск
// уходят только хеши. Rehydrate возвращает имена в стор по списку, который
// пережил перезапуск снаружи — в нашем случае в rule-set файле.
func TestRehydrate_ReturnsNamesToNamesMap(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC) }
	path := filepath.Join(t.TempDir(), "verdicts.json")

	first := New([]byte("salt"), now)
	first.Learn("example.com", Direct)
	if err := first.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	second, err := Load(path, now)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := len(second.Names()); got != 0 {
		t.Fatalf("после загрузки имён быть не должно, получено %d", got)
	}

	if n := second.Rehydrate([]string{"example.com"}); n != 1 {
		t.Fatalf("Rehydrate вернул %d, ожидался 1", n)
	}
	names := second.Names()
	rec, ok := names["example.com"]
	if !ok {
		t.Fatalf("имя не вернулось в Names(), получено %+v", names)
	}
	if rec.Decision != Direct {
		t.Errorf("решение = %v, ожидалось Direct", rec.Decision)
	}
}

// Имя, записи по которому нет (истекла или её не было), в стор не попадает:
// Rehydrate восстанавливает плейнтекст, а не создаёт вердикты.
func TestRehydrate_UnknownNameCreatesNothing(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC) }
	s := New([]byte("salt"), now)

	if n := s.Rehydrate([]string{"nobody.example"}); n != 0 {
		t.Fatalf("Rehydrate вернул %d, ожидался 0", n)
	}
	if _, ok := s.Lookup("nobody.example"); ok {
		t.Error("Rehydrate завёл запись, которой не было")
	}
	if got := len(s.Names()); got != 0 {
		t.Errorf("Names() не пуст: %d", got)
	}
}
```

- [ ] **Шаг 4: убедиться, что тест падает**

Run: `go test ./internal/verdict/ -run TestRehydrate -v`
Expected: FAIL — `s.Rehydrate undefined (type *Store has no field or method Rehydrate)`

- [ ] **Шаг 5: реализовать `Rehydrate`**

Create `internal/verdict/rehydrate.go` (шапка GPL как в соседних файлах):

```go
package verdict

// Rehydrate возвращает в стор плейнтекст имён, переживших перезапуск снаружи.
//
// На диск уходят только хеши (см. комментарий к Store): после Load стор знает
// вердикт по имени, но не знает самого имени, а значит Names() пуст и рендер
// плейнтекстового rule-set стёр бы файл, из которого имена только что и
// пришли. Rehydrate — обратный ход: по списку имён он находит уже лежащие
// записи и подписывает их.
//
// Новых записей не создаёт. Имя без живой записи — истекшее или чужое — молча
// пропускается, и при следующем рендере само выпадает из файла.
//
// Возвращает число узнанных имён: вызывающая сторона пишет его в лог, потому
// что «ноль из трёхсот» означает сменившуюся соль, а не пустой файл.
func (s *Store) Rehydrate(names []string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	space := s.spaces[s.ns]
	if space == nil {
		return 0
	}
	now := s.now()
	restored := 0
	for _, raw := range names {
		key := NormalizeHost(raw)
		if key == "" {
			continue
		}
		h := s.hash(key)
		rec, ok := space[h]
		if !ok {
			continue
		}
		if !rec.ExpiresAt.IsZero() && !rec.ExpiresAt.After(now) {
			continue
		}
		s.plain[h] = key
		restored++
	}
	return restored
}
```

- [ ] **Шаг 6: тесты проходят**

Run: `go test ./internal/verdict/... -v`
Expected: PASS, включая оба новых теста.

- [ ] **Шаг 7: коммит**

```bash
git add internal/verdict
git commit -m "$(cat <<'EOF'
feat(verdict): таблица вердиктов с ПК и восстановление имён после перезапуска

Директория переносится целиком: она ничего не знает про sing-box и проверяется
таблицей входов и ожидаемых ответов.

Rehydrate — единственная добавка, и она нужна ровно потому, что на диск уходят
хеши: после загрузки стор знает вердикт, но не знает имени, а рендер
плейнтекстового rule-set по пустому Names() стёр бы файл, из которого имена и
пришли. Новых записей функция не создаёт — только подписывает уже лежащие.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: гонка, выбор и предохранитель — перенос без сети

Три файла с ПК, которые не знают ни про sing-box, ни про Android: гонка
принимает две функции дозвона, выбор читает стор через узкий интерфейс,
предохранитель считает провалы. UDP-половина `smartchoice.go` не переносится —
решение §7.5.

**Files:**
- Create: `internal/proxy/smartrace.go`, `internal/proxy/smartrace_test.go`
- Create: `internal/proxy/smarthealth.go`, `internal/proxy/smarthealth_test.go`
- Create: `internal/proxy/smartchoice.go`, `internal/proxy/smartchoice_test.go`

**Interfaces:**
- Consumes: пакет `verdict` из Task 1.
- Produces (в пакете `proxy`, неэкспортируемое — всё внутри пакета):
  - `runSmartRace(ctx context.Context, first []byte, direct, proxy raceDialer) raceResult`,
    где `type raceDialer = func(ctx context.Context) (net.Conn, error)` и
    `raceResult{Conn net.Conn; Head []byte; ViaProxy bool; Err error}`
  - константы `smartRaceHeadStart` (700 мс), `smartDirectDialTimeout` (2 с),
    `smartRaceDeadline` (5 с), `smartFirstReadBudget` (8 КиБ)
  - `newDirectHealth(now func() time.Time) *directHealth`,
    `(*directHealth).healthy() bool`, `(*directHealth).record(host string, ok bool)`
  - `smartChoice` (`chooseRace`/`chooseDirect`/`chooseProxy`),
    `decideSmart(store smartLookup, host string, addr netip.Addr, raceAllowed bool) smartChoice`,
    `lookupSmart(store smartLookup, host string, addr netip.Addr) (verdict.Record, bool)`,
    `choiceFrom(rec verdict.Record, known, raceAllowed bool) smartChoice`

- [ ] **Шаг 1: перенести три файла и их тесты**

```bash
for f in smartrace.go smartrace_test.go \
         smarthealth.go smarthealth_test.go \
         smartchoice.go smartchoice_test.go; do
  git show dev:internal/proxy/$f > internal/proxy/$f
done
```

- [ ] **Шаг 2: вырезать из `smartchoice.go` UDP-половину**

Удалить из `internal/proxy/smartchoice.go` всё от строки

```go
// smartUDPAction is what the smart outbound does with one UDP flow. UDP needs
```

и до конца файла — это `smartUDPAction`, её константы, `String()` и
`decideSmartUDP`. Причина в спеке §7.5: гонки по UDP нет, а резать неизвестный
QUIC на телефоне мы решили не начинать. Мёртвый код здесь стоил бы дороже
обычного — следующий агент прочтёт его как намерение.

Затем в шапке файла заменить комментарий типа `smartChoice`:

```go
// smartChoice is what the relay does with one connection.
```

(на ПК там «smart outbound»; аутбаунда здесь нет, и врать в первой же строке не
надо).

- [ ] **Шаг 3: запустить тесты в обеих конфигурациях тегов**

Run:
```bash
TAGS=$(cat scripts/android-build-tags.txt)
go test -tags "$TAGS" ./internal/proxy/ -run 'TestSmart|TestRace|TestDirectHealth|TestChoice|TestDecide' -v
go test -tags "$TAGS,no_mitm,no_adblock" ./internal/proxy/ -run 'TestSmart|TestRace|TestDirectHealth|TestChoice|TestDecide' -v
```
Expected: PASS в обеих. Если какой-то тест упоминает `decideSmartUDP` — значит
шаг 2 срезал лишнее или тест приехал не тот; сверить с `git show
dev:internal/proxy/smartchoice_test.go`.

- [ ] **Шаг 4: весь пакет цел**

Run: `go build -tags "$(cat scripts/android-build-tags.txt)" ./internal/proxy/... ./mobile/...`
Expected: exit 0.

- [ ] **Шаг 5: коммит**

```bash
git add internal/proxy/smartrace.go internal/proxy/smartrace_test.go \
        internal/proxy/smarthealth.go internal/proxy/smarthealth_test.go \
        internal/proxy/smartchoice.go internal/proxy/smartchoice_test.go
git commit -m "$(cat <<'EOF'
feat(proxy): гонка прямого пути против туннеля, выбор и предохранитель

Три файла с ПК, которым всё равно, кто их вызывает: гонка принимает две функции
дозвона, выбор читает стор через узкий интерфейс, предохранитель считает
провалы по родительским доменам.

UDP-половина smartchoice.go не поехала. Гонки по UDP нет ни там, ни здесь, а
резать неизвестный QUIC правилом — это риск замедлить видео ради обучения,
которое и так произойдёт по TCP.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: пробер — гонка не видит региональную стену

Гонка знает только «пришли байты». `claude.ai` отвечает российскому адресу
редиректом на региональную страницу за 407 мс по здоровому TCP — гонка отдаёт её
прямому пути, а сайт остаётся сломанным. Пробер спрашивает хост дважды, прямо и
через узел, и читает ответ.

На Android обе половины проще, чем на ПК: прямая — обычный `net.Dialer`
(процесс вне своего VPN, FakeIP мы не переносим, привязываться не к чему), а
половина «через узел» ходит HTTP-прокси в инбаунд `smart-race-in`, который как
`mixed` отвечает и на CONNECT.

**Files:**
- Create: `internal/proxy/blockprobe.go` (копия с `dev`, две функции переписаны)
- Create: `internal/proxy/blockprobe_limit.go`, `blockprobe_limit_test.go` (копии)
- Create: `internal/proxy/blockprobe_live.go`, `blockprobe_live_test.go` (копии)
- Create: `internal/proxy/blockprobe_test.go` (копия)
- Test: `internal/proxy/blockprobe_android_test.go` (новый)

**Interfaces:**
- Consumes: `verdict` (Task 1).
- Produces:
  - `probeHost(ctx context.Context, host string, gate *probeGate) verdict.Decision`
  - `newProbeGate(now func() time.Time) *probeGate`, `(*probeGate).allow(key string) bool`,
    `(*probeGate).record(key string, ok bool)`, `(*probeGate).open() bool`
  - `probeFetch` (var, подменяется в тестах),
    `classifyProbe(direct, viaNode probeOutcome) verdict.Decision`
  - `setProbeInboundPort(port int)` и `probeInboundPort() int` — порт инбаунда
    «через узел»; ставит его реле в Task 5.

- [ ] **Шаг 1: перенести файлы пробера**

```bash
for f in blockprobe.go blockprobe_test.go \
         blockprobe_limit.go blockprobe_limit_test.go \
         blockprobe_live.go blockprobe_live_test.go; do
  git show dev:internal/proxy/$f > internal/proxy/$f
done
```

`blockprobe_direct_test.go` НЕ переносится: он проверяет ПК-шную привязку к
LAN-адресу, которой здесь не будет.

- [ ] **Шаг 2: заменить прямую половину и источник порта**

В `internal/proxy/blockprobe.go` заменить блок `probeDirectDial` (от
комментария `// probeDirectDial opens the direct half…` до конца файла) на:

```go
// probeDirectDial открывает прямую половину пробы.
//
// На Android это обычный дозвон, и этого достаточно: приложение исключено из
// собственного VPN (BoxModule.tryDisallow), поэтому сокет нашего процесса и так
// не заходит в TUN. Ни разбора фейкового адреса, ни привязки к LAN-адресу — на
// ПК то и другое существует из-за FakeIP, которого здесь нет (спека §7.5).
//
// Оставлено переменной ради тестов, как и probeFetch.
var probeDirectDial = func(ctx context.Context, network, addr string) (net.Conn, error) {
	d := net.Dialer{Timeout: probeDirectDialTimeout}
	return d.DialContext(ctx, network, addr)
}

// probeDirectDialTimeout matches the ping probes: five seconds is already long
// enough to tell a black hole from a slow server.
const probeDirectDialTimeout = 5 * time.Second

// probeInboundPortValue — порт loopback-инбаунда, через который ходит половина
// «через узел». Ставит его реле на старте: до него инбаунда не существует, а
// проба, померившая прямой путь дважды, хуже непомеренной.
var probeInboundPortValue atomic.Int32

func setProbeInboundPort(port int) { probeInboundPortValue.Store(int32(port)) }

func probeInboundPort() int { return int(probeInboundPortValue.Load()) }
```

Добавить `"sync/atomic"` в импорты; убрать `"strings"` из импортов только если
компилятор на него пожалуется (его использует `looksLikeRegionBlock`, то есть
скорее всего он останется).

- [ ] **Шаг 3: написать падающий тест на прямую половину**

Create `internal/proxy/blockprobe_android_test.go`:

```go
package proxy

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
```

- [ ] **Шаг 4: убедиться, что тесты падают на несуществующем**

Run: `go test -tags "$(cat scripts/android-build-tags.txt)" ./internal/proxy/ -run 'TestProbeDirectDial|TestProbeFetch_NoInbound' -v`
Expected: до шага 2 — FAIL со сборкой (`setProbeInboundPort undefined`). Если шаг
2 уже сделан — оба PASS; тогда откатить шаг 2 в рабочей копии, увидеть красный,
вернуть. Красно-зелёный цикл здесь обязателен: тест, который не падал, ничего не
доказывает.

- [ ] **Шаг 5: весь пробер зелёный в обеих конфигурациях**

Run:
```bash
TAGS=$(cat scripts/android-build-tags.txt)
go test -tags "$TAGS" ./internal/proxy/ -run 'TestProbe|TestClassify|TestGate|TestBlockProbe' -v
go test -tags "$TAGS,no_mitm,no_adblock" ./internal/proxy/ -run 'TestProbe|TestClassify|TestGate|TestBlockProbe' -v
```
Expected: PASS в обеих.

- [ ] **Шаг 6: коммит**

```bash
git add internal/proxy/blockprobe*.go
git commit -m "$(cat <<'EOF'
feat(proxy): пробер отличает региональную стену от рабочего сайта

Гонка знает только «пришли байты», а стена приходит байтами тоже: claude.ai
отвечает российскому адресу редиректом за 407 мс по здоровому TCP. Пробер
спрашивает хост дважды — прямо и через узел — и читает ответ; узел здесь
контроль, а не второе мнение, поэтому пока он молчит, прямая половина ничего
не доказывает.

Обе половины на Android проще, чем на ПК. Прямая — обычный дозвон: процесс вне
своего VPN, а FakeIP, ради которого на ПК заведены разбор адреса и привязка к
LAN, сюда не переносится. Половина «через узел» ходит HTTP-прокси в инбаунд
гонки, и без него отказывается мерить вовсе.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: плейнтекстовый rule-set выученных «прямых»

Файл, который ядро перечитывает само. Он же — единственное место, где имена
лежат открытым текстом, и поэтому в него попадают только вердикты `direct`
(решение §7.3).

Две тонкости, каждая ломает всё, если её пропустить:

1. **Файл обязан существовать до старта ядра.** `NewLocalRuleSet` читает его в
   конструкторе и на ошибке чтения роняет запуск (`route/rule/rule_set_local.go`,
   `reloadFile`). Пустой скелет — валиден, правил в нём ноль.
2. **Рендер до `Rehydrate` стирает файл.** После перезапуска `Names()` пуст,
   потому что плейнтекст сессионный. Порядок только такой: прочитать файл →
   `Rehydrate` → дальше можно рендерить.

**Files:**
- Create: `internal/proxy/smartdirectset.go`
- Test: `internal/proxy/smartdirectset_test.go`

**Interfaces:**
- Consumes: `verdict` (Task 1).
- Produces:
  - `SmartVerdictStorePath(dataDir string) string` → `<dataDir>/smart/verdicts.json`
  - `SmartDirectSetPath(dataDir string) string` → `<dataDir>/smart/verdict-direct.json`
  - `EnsureSmartDirectSet(path string) error` — создаёт пустой скелет, если файла нет
  - `ReadSmartDirectSet(path string) []string` — имена из файла, молча пусто при любой беде
  - `RenderSmartDirectSet(path string, names []string) error` — атомарная запись
  - `directNamesOf(store *verdict.Store) []string` — отсортированные имена с вердиктом `Direct`

- [ ] **Шаг 1: написать падающие тесты**

Create `internal/proxy/smartdirectset_test.go`:

```go
package proxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	if err := json.Unmarshal(raw, &parsed); err != nil {
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
```

- [ ] **Шаг 2: убедиться, что тесты падают**

Run: `go test -tags "$(cat scripts/android-build-tags.txt)" ./internal/proxy/ -run 'TestEnsureSmartDirectSet|TestRenderAndRead|TestReadSmartDirectSet|TestDirectNamesOf' -v`
Expected: FAIL — `undefined: EnsureSmartDirectSet` и остальные.

- [ ] **Шаг 3: реализовать**

Create `internal/proxy/smartdirectset.go` (шапка GPL как в соседях):

```go
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
```

- [ ] **Шаг 4: тесты зелёные в обеих конфигурациях**

Run:
```bash
TAGS=$(cat scripts/android-build-tags.txt)
go test -tags "$TAGS" ./internal/proxy/ -run 'SmartDirectSet|DirectNamesOf' -v
go test -tags "$TAGS,no_mitm,no_adblock" ./internal/proxy/ -run 'SmartDirectSet|DirectNamesOf' -v
```
Expected: PASS в обеих.

- [ ] **Шаг 5: коммит**

```bash
git add internal/proxy/smartdirectset.go internal/proxy/smartdirectset_test.go
git commit -m "$(cat <<'EOF'
feat(proxy): выученные «прямые» имена уезжают в rule-set, который ядро читает само

Локальный rule-set ядро держит под fswatch, поэтому выученное применяется живьём,
без перезапуска коробки — и трафик, о котором уже есть вердикт «ходит
напрямую», перестаёт ходить через реле.

Открытым текстом в файл попадают только direct-вердикты. Имена заблокированных
сайтов остаются в хешированном сторе: это и есть история посещений, и на диск
она не ложится.

Пустой файл создаётся до старта ядра не для красоты: NewLocalRuleSet читает путь
в конструкторе и на отсутствии файла роняет запуск целиком.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: реле — HTTP CONNECT, гонка, обучение

Сердце блока. Реле слушает loopback, принимает CONNECT от аутбаунда
`smart-relay`, решает по стору и — если не знает — гонит прямой путь против
туннеля.

Почему CONNECT, а не SOCKS5: аутбаунд `http` ядра посылает `CONNECT host:port`, то
есть имя доезжает целиком, а сервер протокола укладывается в полсотни строк
против `socks.HandleConnectionEx`, который требует реализовать два интерфейса
`sing`. Туннельная нога при этом ходит SOCKS5 в инбаунд `smart-race-in` — тем же
`golang.org/x/net/proxy`, которым сегодня ходит MITM (`mobile/libbox_filter.go`).

Ответ `200` отдаётся сразу после разбора CONNECT, до всякого дозвона. Иначе
первых байт клиента не увидеть, а без них гонку не запустить. Цена — провал
дозвона клиент видит закрытым соединением, а не кодом ошибки; ровно так же он
выглядит и сегодня, когда сайт не открывается.

**Files:**
- Create: `internal/proxy/smartrelay.go`
- Test: `internal/proxy/smartrelay_test.go`

**Interfaces:**
- Consumes: `verdict` (Task 1), `runSmartRace`/`newDirectHealth`/`lookupSmart`/
  `choiceFrom` (Task 2), `probeHost`/`newProbeGate`/`setProbeInboundPort` (Task 3),
  `SmartVerdictStorePath`/`SmartDirectSetPath`/`EnsureSmartDirectSet`/
  `ReadSmartDirectSet`/`RenderSmartDirectSet`/`directNamesOf` (Task 4).
- Produces:
  - `type SmartRelayOptions struct { DataDir string; ListenPort int; TunnelPort int; MemoryOnly bool }`
  - `StartSmartRelay(opts SmartRelayOptions) (*SmartRelay, error)`
  - `(*SmartRelay).Close() error`
  - `(*SmartRelay).Port() int`

- [ ] **Шаг 1: написать падающие тесты**

Create `internal/proxy/smartrelay_test.go`:

```go
package proxy

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"resultproxy-wails/internal/verdict"
)

// echoServer — «сайт»: пишет обратно то, что ему прислали, с префиксом.
func echoServer(t *testing.T, prefix string) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			c, aErr := ln.Accept()
			if aErr != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 512)
				n, rErr := c.Read(buf)
				if rErr != nil {
					return
				}
				fmt.Fprintf(c, "%s:%s", prefix, buf[:n])
			}(c)
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return ln
}

// dialThroughRelay говорит с реле как аутбаунд http ядра: CONNECT, затем
// байты вперемешку.
func dialThroughRelay(t *testing.T, relayPort int, target string) net.Conn {
	t.Helper()
	c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", relayPort))
	if err != nil {
		t.Fatalf("dial relay: %v", err)
	}
	fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	resp, err := http.ReadResponse(bufio.NewReader(c), &http.Request{Method: "CONNECT"})
	if err != nil {
		t.Fatalf("read CONNECT response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT дал %d, ожидалось 200", resp.StatusCode)
	}
	return c
}

// Известный direct-вердикт реле отрабатывает без гонки: соединение уходит
// прямым дозвоном, туннель не трогается вовсе.
func TestSmartRelay_KnownDirect_GoesStraightOut(t *testing.T) {
	site := echoServer(t, "direct")
	dir := t.TempDir()

	relay, err := StartSmartRelay(SmartRelayOptions{
		DataDir: dir, ListenPort: 0, TunnelPort: 0, MemoryOnly: true,
	})
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer relay.Close()
	relay.store.Seed(site.Addr().(*net.TCPAddr).IP.String(), verdict.Direct, verdict.SourceUser)

	c := dialThroughRelay(t, relay.Port(), site.Addr().String())
	defer c.Close()
	fmt.Fprint(c, "привет")
	got := readAll(t, c)
	if !strings.HasPrefix(got, "direct:") {
		t.Fatalf("ответ %q, ожидался прямой путь", got)
	}
}

// Незнакомое имя реле разыгрывает и записывает исход. Туннель здесь отвечает
// быстрее, потому что прямого пути к этому адресу нет вовсе.
func TestSmartRelay_UnknownRaces_AndLearns(t *testing.T) {
	tunnelSite := echoServer(t, "tunnel")
	dir := t.TempDir()

	// Инбаунд «туннеля»: SOCKS5-сервер, который всё ведёт в tunnelSite.
	tunnel := fakeSocksInbound(t, tunnelSite.Addr().String())

	relay, err := StartSmartRelay(SmartRelayOptions{
		DataDir: dir, ListenPort: 0, TunnelPort: tunnel, MemoryOnly: true,
	})
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer relay.Close()

	// Адрес, который никто не слушает: прямая нога откажет сразу.
	dead := deadAddress(t)
	c := dialThroughRelay(t, relay.Port(), dead)
	defer c.Close()
	fmt.Fprint(c, "привет")
	got := readAll(t, c)
	if !strings.HasPrefix(got, "tunnel:") {
		t.Fatalf("ответ %q, ожидался путь через туннель", got)
	}

	host, _, _ := net.SplitHostPort(dead)
	addr := netip.MustParseAddr(host)
	// Имени тут нет — только адрес, как у Telegram MTProto. Спрашиваем так же,
	// как спросит реле на следующем соединении.
	waitFor(t, func() bool {
		rec, ok := relay.store.LookupIP(addr)
		return ok && rec.Decision == verdict.Proxy
	}, "вердикт proxy не записан")
}

// Файл rule-set обновляется после того, как гонка научилась: иначе выученное
// так и осталось бы ходить через реле.
func TestSmartRelay_LearnedDirect_LandsInRuleSetFile(t *testing.T) {
	dir := t.TempDir()
	relay, err := StartSmartRelay(SmartRelayOptions{
		DataDir: dir, ListenPort: 0, TunnelPort: 0, MemoryOnly: false,
	})
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer relay.Close()

	relay.store.Learn("clean.example", verdict.Direct)
	relay.markDirty()

	path := SmartDirectSetPath(dir)
	waitFor(t, func() bool {
		for _, n := range ReadSmartDirectSet(path) {
			if n == "clean.example" {
				return true
			}
		}
		return false
	}, "имя не доехало до файла rule-set")
}

// «Только в памяти» значит именно это: ни стор, ни rule-set на диск не идут,
// но скелет остаётся — без него ядро не стартует.
func TestSmartRelay_MemoryOnly_WritesNothingButSkeleton(t *testing.T) {
	dir := t.TempDir()
	relay, err := StartSmartRelay(SmartRelayOptions{
		DataDir: dir, ListenPort: 0, TunnelPort: 0, MemoryOnly: true,
	})
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	relay.store.Learn("clean.example", verdict.Direct)
	relay.markDirty()
	// Close сам дописывает всё, что должно было дописаться, — ждать таймера
	// не нужно, и именно поэтому он тут самая строгая проверка.
	if err := relay.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if names := ReadSmartDirectSet(SmartDirectSetPath(dir)); len(names) != 0 {
		t.Errorf("в памяти-только, а в файле %v", names)
	}
	if _, err := os.Stat(SmartVerdictStorePath(dir)); err == nil {
		t.Error("в памяти-только, а стор записан на диск")
	}
}

// Перезапуск: имена из файла возвращаются в стор, и повторного обучения не
// требуется.
func TestSmartRelay_Restart_RehydratesNamesFromFile(t *testing.T) {
	dir := t.TempDir()
	first, err := StartSmartRelay(SmartRelayOptions{DataDir: dir, ListenPort: 0, TunnelPort: 0})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	first.store.Learn("clean.example", verdict.Direct)
	first.markDirty()
	if err := first.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	second, err := StartSmartRelay(SmartRelayOptions{DataDir: dir, ListenPort: 0, TunnelPort: 0})
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	defer second.Close()
	if _, ok := second.store.Names()["clean.example"]; !ok {
		t.Fatalf("имя не вернулось в стор после перезапуска: %v", second.store.Names())
	}
}
```

Вспомогательные функции для того же файла:

```go
func readAll(t *testing.T, c net.Conn) string {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 512)
	n, err := c.Read(buf)
	if err != nil && n == 0 {
		t.Fatalf("read: %v", err)
	}
	return string(buf[:n])
}

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal(msg)
}

// deadAddress возвращает адрес, который заведомо никто не слушает: порт занят
// и тут же отпущен.
func deadAddress(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

// fakeSocksInbound — заглушка инбаунда движка: принимает SOCKS5 CONNECT и
// ведёт всё в один адрес, как это сделал бы туннель.
func fakeSocksInbound(t *testing.T, target string) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, aErr := ln.Accept()
			if aErr != nil {
				return
			}
			go serveFakeSocks(c, target)
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func serveFakeSocks(c net.Conn, target string) {
	defer c.Close()
	r := bufio.NewReader(c)
	// Приветствие: версия, число методов, методы.
	head := make([]byte, 2)
	if _, err := io.ReadFull(r, head); err != nil {
		return
	}
	if _, err := io.ReadFull(r, make([]byte, int(head[1]))); err != nil {
		return
	}
	if _, err := c.Write([]byte{0x05, 0x00}); err != nil {
		return
	}
	// Запрос: ver, cmd, rsv, atyp, addr, port. Адрес нам не нужен — заглушка
	// всегда ведёт в один и тот же target.
	req := make([]byte, 4)
	if _, err := io.ReadFull(r, req); err != nil {
		return
	}
	switch req[3] {
	case 0x01:
		io.ReadFull(r, make([]byte, 4+2))
	case 0x03:
		l := make([]byte, 1)
		if _, err := io.ReadFull(r, l); err != nil {
			return
		}
		io.ReadFull(r, make([]byte, int(l[0])+2))
	case 0x04:
		io.ReadFull(r, make([]byte, 16+2))
	}
	if _, err := c.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}
	up, err := net.Dial("tcp", target)
	if err != nil {
		return
	}
	defer up.Close()
	go io.Copy(up, r)
	io.Copy(c, up)
}
```

`smartRelayFlushDelay` в тестах напрямую не ждём: `Close` дописывает файлы сам,
и проверка через него строже таймера.

- [ ] **Шаг 2: убедиться, что тесты падают**

Run: `go test -tags "$(cat scripts/android-build-tags.txt)" ./internal/proxy/ -run TestSmartRelay -v`
Expected: FAIL — `undefined: StartSmartRelay`.

- [ ] **Шаг 3: реализовать реле**

Create `internal/proxy/smartrelay.go` (шапка GPL как в соседях):

```go
package proxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	// Псевдоним обязателен: пакет здесь сам называется proxy, и импорт под тем
	// же именем читался бы как обращение к себе.
	socksproxy "golang.org/x/net/proxy"

	"resultproxy-wails/internal/verdict"
)

const (
	// smartRelayFlushDelay — задержка перед записью файла rule-set после
	// изменения. Загрузка одной страницы учит десяток имён; писать файл
	// десять раз подряд значит десять раз дёрнуть fswatch ядра.
	smartRelayFlushDelay = 2 * time.Second
	// smartRelayStoreInterval — как часто хешированный стор уходит на диск.
	// Реже, чем rule-set: его никто не читает на ходу, а запись дороже.
	smartRelayStoreInterval = 60 * time.Second
	// smartRelayHeaderTimeout — сколько ждём заголовок CONNECT. Клиент здесь
	// не человек, а аутбаунд ядра: он пишет запрос сразу.
	smartRelayHeaderTimeout = 10 * time.Second
)

// SmartRelayOptions — всё, что реле нужно знать о мире.
type SmartRelayOptions struct {
	// DataDir — корень данных приложения (filesDir). Файлы лягут в его
	// подпапку smart/, рядом со Smart-списком.
	DataDir string
	// ListenPort — порт реле; 0 означает «любой свободный» и нужен тестам.
	ListenPort int
	// TunnelPort — порт loopback-инбаунда движка, через который проходит
	// туннельная нога. 0 означает, что туннеля нет: гонка тогда вырождается в
	// прямой путь, и это честнее, чем мерить прямой путь дважды.
	TunnelPort int
	// MemoryOnly — не писать на диск ничего, кроме пустого скелета rule-set
	// (без файла ядро не стартует).
	MemoryOnly bool
}

// SmartRelay — loopback-прокси, в который маршрутные правила отправляют всё,
// о чём вердикта ещё нет.
type SmartRelay struct {
	opts     SmartRelayOptions
	listener net.Listener
	store    *verdict.Store
	health   *directHealth
	probes   *probeGate

	// probe вынесен полем, чтобы тест мог ответить без сети.
	probe func(ctx context.Context, host string, gate *probeGate) verdict.Decision

	// inflight держит хосты, по которым проба уже идёт. Загрузка страницы
	// открывает десятки соединений к одному имени разом; без этого каждое
	// начало бы свою пробу и одна страница съела бы весь бюджет намордника.
	inflight sync.Map

	dirty  chan struct{}
	closed chan struct{}
	wg     sync.WaitGroup

	closeOnce sync.Once
}

// StartSmartRelay поднимает реле и возвращает его уже слушающим.
func StartSmartRelay(opts SmartRelayOptions) (*SmartRelay, error) {
	storePath := SmartVerdictStorePath(opts.DataDir)
	setPath := SmartDirectSetPath(opts.DataDir)

	// Порядок обязателен. Сперва имена из файла, потом стор, потом Rehydrate:
	// плейнтекст в сторе сессионный, и рендер до восстановления стёр бы файл,
	// из которого имена только что пришли.
	names := ReadSmartDirectSet(setPath)
	store, err := verdict.Load(storePath, nil)
	if err != nil {
		return nil, err
	}
	store.Rehydrate(names)

	if err := EnsureSmartDirectSet(setPath); err != nil {
		return nil, err
	}

	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(opts.ListenPort))
	if err != nil {
		return nil, err
	}

	r := &SmartRelay{
		opts:     opts,
		listener: ln,
		store:    store,
		health:   newDirectHealth(nil),
		probes:   newProbeGate(nil),
		probe:    probeHost,
		dirty:    make(chan struct{}, 1),
		closed:   make(chan struct{}),
	}
	setProbeInboundPort(opts.TunnelPort)

	r.wg.Add(2)
	go r.acceptLoop()
	go r.flushLoop()
	return r, nil
}

// Port — на каком порту реле в итоге слушает. Тесты просят нулевой и узнают
// ответ здесь.
func (r *SmartRelay) Port() int { return r.listener.Addr().(*net.TCPAddr).Port }

// Close останавливает реле и сохраняет выученное.
func (r *SmartRelay) Close() error {
	var err error
	r.closeOnce.Do(func() {
		close(r.closed)
		err = r.listener.Close()
		r.wg.Wait()
		r.persist()
		setProbeInboundPort(0)
	})
	return err
}

func (r *SmartRelay) acceptLoop() {
	defer r.wg.Done()
	for {
		conn, err := r.listener.Accept()
		if err != nil {
			select {
			case <-r.closed:
				return
			default:
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return
		}
		go r.serve(conn)
	}
}

// markDirty просит записать выученное. Не блокирует: буфер на единицу, и
// второй сигнал до записи ничего не добавляет.
func (r *SmartRelay) markDirty() {
	select {
	case r.dirty <- struct{}{}:
	default:
	}
}

// flushLoop пишет файлы: rule-set — вскоре после изменения, стор — по таймеру.
func (r *SmartRelay) flushLoop() {
	defer r.wg.Done()
	var pending <-chan time.Time
	storeTicker := time.NewTicker(smartRelayStoreInterval)
	defer storeTicker.Stop()
	for {
		select {
		case <-r.closed:
			return
		case <-r.dirty:
			pending = time.After(smartRelayFlushDelay)
		case <-pending:
			pending = nil
			r.renderSet()
		case <-storeTicker.C:
			r.saveStore()
		}
	}
}

func (r *SmartRelay) renderSet() {
	if r.opts.MemoryOnly {
		return
	}
	if err := RenderSmartDirectSet(SmartDirectSetPath(r.opts.DataDir), directNamesOf(r.store)); err != nil {
		// Молча: файл — кэш. Потеря рендера стоит петли до следующего
		// изменения, отказ реле стоил бы всего трафика.
		_ = err
	}
}

func (r *SmartRelay) saveStore() {
	if r.opts.MemoryOnly {
		return
	}
	_ = r.store.Save(SmartVerdictStorePath(r.opts.DataDir))
}

func (r *SmartRelay) persist() {
	r.renderSet()
	r.saveStore()
}

// serve обслуживает одно соединение от аутбаунда ядра.
func (r *SmartRelay) serve(client net.Conn) {
	defer client.Close()

	_ = client.SetReadDeadline(time.Now().Add(smartRelayHeaderTimeout))
	reader := bufio.NewReader(client)
	target, err := readConnectRequest(reader)
	if err != nil {
		return
	}
	_ = client.SetReadDeadline(time.Time{})

	// Ответ 200 идёт до дозвона, и это осознанно: без него клиент не пришлёт
	// первых байт, а без первых байт гонку не запустить — их нечего послать
	// обеим ногам. Плата в том, что провал дозвона клиент видит закрытым
	// соединением, а не кодом ошибки. Ровно так же он видит его и сегодня,
	// когда сайт не открывается.
	if _, err := io.WriteString(client, "HTTP/1.1 200 Connection established\r\n\r\n"); err != nil {
		return
	}

	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return
	}
	addr, _ := netip.ParseAddr(host)

	rec, known := lookupSmart(r.store, host, addr)
	switch choiceFrom(rec, known, r.health.healthy()) {
	case chooseProxy:
		r.pipeVia(client, reader, target, true)
	case chooseDirect:
		r.pipeVia(client, reader, target, false)
	default:
		r.race(client, reader, target, host, portStr)
	}
}

// pipeVia ведёт соединение одной ногой, без гонки: вердикт уже есть.
func (r *SmartRelay) pipeVia(client net.Conn, buffered *bufio.Reader, target string, viaTunnel bool) {
	var (
		up  net.Conn
		err error
	)
	if viaTunnel {
		up, err = r.dialTunnel(context.Background(), target)
	} else {
		up, err = r.dialDirect(context.Background(), target)
	}
	if err != nil {
		return
	}
	defer up.Close()
	splice(client, buffered, up)
}

// race разыгрывает незнакомое назначение.
func (r *SmartRelay) race(client net.Conn, buffered *bufio.Reader, target, host, portStr string) {
	first := make([]byte, smartFirstReadBudget)
	_ = client.SetReadDeadline(time.Now().Add(smartRaceHeadStart))
	n, _ := buffered.Read(first)
	_ = client.SetReadDeadline(time.Time{})
	if n == 0 {
		// Клиент, который молчит, разыграть нельзя: нечего послать и нечем
		// различить ноги. Пусть идёт так, как ходил до появления фичи.
		r.pipeVia(client, buffered, target, false)
		return
	}

	res := runSmartRace(context.Background(), first[:n],
		func(ctx context.Context) (net.Conn, error) { return r.dialDirect(ctx, target) },
		func(ctx context.Context) (net.Conn, error) { return r.dialTunnel(ctx, target) },
	)
	if res.Err != nil {
		r.health.record(host, false)
		return
	}
	defer res.Conn.Close()

	// Выигрыш прямого пути — доказательство, что линия жива; выигрыш туннеля —
	// ещё один сайт, который сам не ответил.
	r.health.record(host, !res.ViaProxy)
	r.learn(host, portStr, res.ViaProxy)
	if !res.ViaProxy {
		// Прямой путь победил по байтам. Байты это сайт или стена — вопрос
		// другой, и отвечает на него только проба.
		r.recheckAsync(host)
	}

	if _, err := client.Write(res.Head); err != nil {
		return
	}
	splice(client, buffered, res.Conn)
}

// learn записывает, что доказала гонка.
func (r *SmartRelay) learn(host, portStr string, viaTunnel bool) {
	if !r.health.healthy() {
		// Линия лежит. Всё записанное сейчас было бы догадкой сроком до недели.
		return
	}
	d := verdict.Direct
	if viaTunnel {
		d = verdict.Proxy
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		// Голый адрес — это Telegram MTProto и голос Discord: имени они не
		// дают никогда, и записывать их приходится по адресу.
		r.store.LearnIP(addr, d)
	} else {
		r.store.Learn(host, d)
	}
	_ = portStr
	if d == verdict.Direct {
		r.markDirty()
	}
}

// recheckAsync перепроверяет имя, которое гонка отдала прямому пути.
func (r *SmartRelay) recheckAsync(host string) {
	if host == "" || r.probe == nil {
		return
	}
	if _, busy := r.inflight.LoadOrStore(host, struct{}{}); busy {
		return
	}
	go func() {
		defer r.inflight.Delete(host)
		d := r.probe(context.Background(), host, r.probes)
		if d == verdict.Unknown {
			return
		}
		r.store.Learn(host, d)
		r.markDirty()
	}()
}

func (r *SmartRelay) dialDirect(ctx context.Context, target string) (net.Conn, error) {
	d := net.Dialer{Timeout: smartDirectDialTimeout}
	return d.DialContext(ctx, "tcp", target)
}

// dialTunnel ходит SOCKS5 в loopback-инбаунд движка — тем же способом, которым
// сегодня ходит MITM (mobile/libbox_filter.go). Адрес передаётся именем, чтобы
// инбаунд увидел домен и применил к нему обычные правила.
func (r *SmartRelay) dialTunnel(ctx context.Context, target string) (net.Conn, error) {
	if r.opts.TunnelPort == 0 {
		return nil, errors.New("smart: туннельный инбаунд не поднят")
	}
	dialer, err := socksproxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", r.opts.TunnelPort), nil, socksproxy.Direct)
	if err != nil {
		return nil, err
	}
	if cd, ok := dialer.(socksproxy.ContextDialer); ok {
		return cd.DialContext(ctx, "tcp", target)
	}
	return dialer.Dial("tcp", target)
}

// readConnectRequest читает `CONNECT host:port HTTP/1.1` и его заголовки.
//
// Свой разбор, а не net/http: нам нужно ровно одно поле, а http.ReadRequest
// потащил бы за собой тело, таймауты и аллокации на каждое соединение.
func readConnectRequest(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	parts := strings.Fields(strings.TrimSpace(line))
	if len(parts) < 2 || !strings.EqualFold(parts[0], "CONNECT") {
		return "", errors.New("smart: ожидался CONNECT, получено " + strings.TrimSpace(line))
	}
	for {
		h, hErr := reader.ReadString('\n')
		if hErr != nil {
			return "", hErr
		}
		if strings.TrimSpace(h) == "" {
			break
		}
	}
	return parts[1], nil
}

// splice сводит две стороны. Буфер читателя обязателен: в нём уже могут лежать
// байты клиента, вычитанные вместе с заголовком.
func splice(client net.Conn, buffered *bufio.Reader, up net.Conn) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(up, buffered)
		if c, ok := up.(*net.TCPConn); ok {
			_ = c.CloseWrite()
		}
	}()
	_, _ = io.Copy(client, up)
	if c, ok := client.(*net.TCPConn); ok {
		_ = c.CloseWrite()
	}
	<-done
}
```

- [ ] **Шаг 4: тесты проходят в обеих конфигурациях**

Run:
```bash
TAGS=$(cat scripts/android-build-tags.txt)
go test -tags "$TAGS" ./internal/proxy/ -run TestSmartRelay -v -race
go test -tags "$TAGS,no_mitm,no_adblock" ./internal/proxy/ -run TestSmartRelay -v
```
Expected: PASS в обеих, `-race` чистый. Гонка данных здесь особенно вероятна:
стор и файл трогают и обработчик соединения, и фоновая запись.

- [ ] **Шаг 5: весь пакет цел**

Run: `go test -tags "$(cat scripts/android-build-tags.txt)" ./internal/proxy/...`
Expected: PASS, ноль падений.

- [ ] **Шаг 6: коммит**

```bash
git add internal/proxy/smartrelay.go internal/proxy/smartrelay_test.go
git commit -m "$(cat <<'EOF'
feat(proxy): реле, в котором прямой путь гонится против туннеля

Кастомный аутбаунд, которым это сделано на ПК, под libbox зарегистрировать
негде: реестр зашит в baseContext, а тот неэкспортирован. Поэтому гонка живёт
в процессе приложения, а движок отправляет в неё всё, о чём вердикта ещё нет.

Протокол — HTTP CONNECT, потому что аутбаунд http шлёт имя целиком, а сервер
укладывается в полсотни строк против двух интерфейсов sing. Туннельная нога
ходит SOCKS5 в инбаунд движка — тем же путём, которым сегодня ходит MITM.

Ответ 200 отдаётся до дозвона, иначе клиент не пришлёт первых байт, а без них
гонку не запустить. Провал дозвона клиент видит закрытым соединением — ровно
так же, как видит его сегодня, когда сайт не открывается.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: конфиг движка — инбаунд, аутбаунд, правила

Три добавки в конфиг и порядок правил, от которого зависит, закольцуется реле
или нет.

**Files:**
- Modify: `internal/proxy/engine.go` — добавить поле `Inbound` в `SBRouteRule`
- Modify: `mobile/libbox.go` — два поля `BuildOptions`, константы портов,
  инбаунд, аутбаунд, rule-set и правила
- Test: `mobile/libbox_adaptive_smart_test.go`

**Interfaces:**
- Consumes: `SmartDirectSetPath`, `EnsureSmartDirectSet` (Task 4).
- Produces:
  - `BuildOptions.AdaptiveSmart bool` (json `adaptiveSmart`),
    `BuildOptions.AdaptiveSmartMemoryOnly bool` (json `adaptiveSmartMemoryOnly`)
  - константы `SmartRaceInboundPort = 18131`, `SmartRelayPort = 18132`
  - теги `smart-race-in`, `smart-relay`, `verdict-direct` в собранном конфиге

- [ ] **Шаг 1: написать падающие тесты конфига**

Create `mobile/libbox_adaptive_smart_test.go`:

```go
package mobile

import (
	"encoding/json"
	"testing"
)

type adaptiveSmartView struct {
	Inbounds []struct {
		Type       string `json:"type"`
		Tag        string `json:"tag"`
		Listen     string `json:"listen"`
		ListenPort int    `json:"listen_port"`
	} `json:"inbounds"`
	Outbounds []struct {
		Type       string `json:"type"`
		Tag        string `json:"tag"`
		Server     string `json:"server"`
		ServerPort int    `json:"server_port"`
	} `json:"outbounds"`
	Route struct {
		Rules []struct {
			Inbound  []string `json:"inbound"`
			Network  []string `json:"network"`
			RuleSet  []string `json:"rule_set"`
			Action   string   `json:"action"`
			Outbound string   `json:"outbound"`
		} `json:"rules"`
		RuleSet []struct {
			Type   string `json:"type"`
			Tag    string `json:"tag"`
			Format string `json:"format"`
			Path   string `json:"path"`
		} `json:"rule_set"`
		Final string `json:"final"`
	} `json:"route"`
}

func buildAdaptive(t *testing.T, opts BuildOptions) adaptiveSmartView {
	t.Helper()
	b, _ := json.Marshal(opts)
	cfg, err := BuildSingBoxConfigFromEntryV2(entryFixture, t.TempDir(), string(b))
	if err != nil {
		t.Fatalf("build config: %v", err)
	}
	var parsed adaptiveSmartView
	if err := json.Unmarshal([]byte(cfg), &parsed); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	return parsed
}

func smartOpts() BuildOptions {
	return BuildOptions{SmartMode: true, AdaptiveSmart: true}
}

func TestAdaptiveSmart_AddsRaceInboundAndRelayOutbound(t *testing.T) {
	cfg := buildAdaptive(t, smartOpts())

	var foundIn bool
	for _, in := range cfg.Inbounds {
		if in.Tag == "smart-race-in" {
			foundIn = true
			if in.Type != "mixed" {
				t.Errorf("инбаунд гонки type = %q, ожидался mixed (он же отвечает проберу по CONNECT)", in.Type)
			}
			if in.Listen != "127.0.0.1" || in.ListenPort != SmartRaceInboundPort {
				t.Errorf("инбаунд гонки на %s:%d, ожидался 127.0.0.1:%d", in.Listen, in.ListenPort, SmartRaceInboundPort)
			}
		}
	}
	if !foundIn {
		t.Fatalf("инбаунда smart-race-in нет: %+v", cfg.Inbounds)
	}

	var foundOut bool
	for _, out := range cfg.Outbounds {
		if out.Tag == "smart-relay" {
			foundOut = true
			if out.Type != "http" {
				t.Errorf("аутбаунд реле type = %q, ожидался http", out.Type)
			}
			if out.Server != "127.0.0.1" || out.ServerPort != SmartRelayPort {
				t.Errorf("аутбаунд реле на %s:%d, ожидался 127.0.0.1:%d", out.Server, out.ServerPort, SmartRelayPort)
			}
		}
	}
	if !foundOut {
		t.Fatalf("аутбаунда smart-relay нет: %+v", cfg.Outbounds)
	}
}

// Правило туннельной ноги обязано стоять ДО правила, отправляющего остаток в
// реле. Иначе нога вернётся в реле и соединение закольцуется.
func TestAdaptiveSmart_TunnelLegRuleComesBeforeRelayRule(t *testing.T) {
	cfg := buildAdaptive(t, smartOpts())

	legIdx, relayIdx := -1, -1
	for i, r := range cfg.Route.Rules {
		if len(r.Inbound) == 1 && r.Inbound[0] == "smart-race-in" {
			legIdx = i
			if r.Outbound != "proxy" {
				t.Errorf("туннельная нога уходит в %q, ожидался proxy", r.Outbound)
			}
		}
		if r.Outbound == "smart-relay" {
			relayIdx = i
		}
	}
	if legIdx < 0 {
		t.Fatalf("правила туннельной ноги нет: %+v", cfg.Route.Rules)
	}
	if relayIdx < 0 {
		t.Fatalf("правила «остальное в реле» нет: %+v", cfg.Route.Rules)
	}
	if legIdx >= relayIdx {
		t.Fatalf("нога на позиции %d, реле на %d — нога обязана быть раньше, иначе петля", legIdx, relayIdx)
	}
}

// Выученное «ходит напрямую» решается правилом, а не реле: правило rule_set
// стоит перед правилом реле.
func TestAdaptiveSmart_LearnedDirectRuleComesBeforeRelayRule(t *testing.T) {
	cfg := buildAdaptive(t, smartOpts())

	directIdx, relayIdx := -1, -1
	for i, r := range cfg.Route.Rules {
		if len(r.RuleSet) == 1 && r.RuleSet[0] == "verdict-direct" {
			directIdx = i
			if r.Outbound != "direct" {
				t.Errorf("выученное прямое уходит в %q, ожидался direct", r.Outbound)
			}
		}
		if r.Outbound == "smart-relay" {
			relayIdx = i
		}
	}
	if directIdx < 0 || relayIdx < 0 || directIdx >= relayIdx {
		t.Fatalf("порядок неверен: verdict-direct=%d, smart-relay=%d", directIdx, relayIdx)
	}
}

// Реле забирает только TCP: гонки по UDP нет, и QUIC продолжает ходить как
// ходил.
func TestAdaptiveSmart_RelayRuleTakesTCPOnly(t *testing.T) {
	cfg := buildAdaptive(t, smartOpts())
	for _, r := range cfg.Route.Rules {
		if r.Outbound != "smart-relay" {
			continue
		}
		if len(r.Network) != 1 || r.Network[0] != "tcp" {
			t.Fatalf("правило реле ловит network=%v, ожидался только tcp", r.Network)
		}
		return
	}
	t.Fatal("правила реле нет")
}

// Файл rule-set обязан существовать к моменту старта ядра, иначе
// NewLocalRuleSet роняет запуск. Сборка конфига его и создаёт.
func TestAdaptiveSmart_RuleSetFileIsCreatedByConfigBuild(t *testing.T) {
	dir := t.TempDir()
	b, _ := json.Marshal(smartOpts())
	if _, err := BuildSingBoxConfigFromEntryV2(entryFixture, dir, string(b)); err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, err := os.Stat(proxy.SmartDirectSetPath(dir)); err != nil {
		t.Fatalf("после сборки конфига файла rule-set нет — ядро не стартует: %v", err)
	}
}

// Выключенный тумблер не оставляет в конфиге ни инбаунда, ни аутбаунда, ни
// правил: выключено значит выключено.
func TestAdaptiveSmart_Off_LeavesConfigUntouched(t *testing.T) {
	cfg := buildAdaptive(t, BuildOptions{SmartMode: true})
	for _, in := range cfg.Inbounds {
		if in.Tag == "smart-race-in" {
			t.Fatal("инбаунд гонки при выключенном тумблере")
		}
	}
	for _, out := range cfg.Outbounds {
		if out.Tag == "smart-relay" {
			t.Fatal("аутбаунд реле при выключенном тумблере")
		}
	}
	for _, r := range cfg.Route.Rules {
		if r.Outbound == "smart-relay" {
			t.Fatal("правило реле при выключенном тумблере")
		}
	}
}

// Вне Smart-режима фича не работает: в Global всё и так идёт через туннель,
// а гонять гонку было бы чистой тратой.
func TestAdaptiveSmart_OutsideSmartMode_IsInert(t *testing.T) {
	cfg := buildAdaptive(t, BuildOptions{AdaptiveSmart: true})
	for _, r := range cfg.Route.Rules {
		if r.Outbound == "smart-relay" {
			t.Fatal("правило реле вне Smart-режима")
		}
	}
}
```

Импорты файла: `"encoding/json"`, `"os"`, `"testing"` и
`"resultproxy-wails/internal/proxy"` — путь к файлу rule-set знает пакет
`proxy`, и дублировать его строкой в тесте значит завести второй источник
истины.

- [ ] **Шаг 2: убедиться, что тесты падают**

Run: `go test -tags "$(cat scripts/android-build-tags.txt)" ./mobile/ -run TestAdaptiveSmart -v`
Expected: FAIL — `unknown field AdaptiveSmart in struct literal`.

- [ ] **Шаг 3: поле `Inbound` в правиле маршрута**

В `internal/proxy/engine.go`, в `type SBRouteRule struct`, сразу после
`Protocol`, добавить:

```go
	// Inbound матчит по тегу инбаунда, через который соединение вошло.
	// Нужно ровно одному правилу — тому, что выводит туннельную ногу реле
	// прямо в proxy: без него нога попала бы под общее правило «остальное в
	// реле» и соединение закольцевалось бы само на себя.
	Inbound []string `json:"inbound,omitempty"`
```

- [ ] **Шаг 4: два поля в `BuildOptions`**

В `mobile/libbox.go`, в `type BuildOptions struct`, после `BrowserAdBlockPort`:

```go
	// AdaptiveSmart включает обучение: всё, о чём вердикта ещё нет, уходит в
	// loopback-реле, где прямой путь разыгрывается против туннеля, а исход
	// запоминается. Работает только вместе со SmartMode — в Global весь
	// трафик и так в туннеле, и разыгрывать нечего.
	//
	// Kotlin обязан поднять реле (Mobile.StartAdaptiveSmart) на том же флаге:
	// конфиг с аутбаундом, за которым никто не слушает, глушит весь трафик,
	// не покрытый другими правилами.
	AdaptiveSmart bool `json:"adaptiveSmart,omitempty"`
	// AdaptiveSmartMemoryOnly не даёт выученному пережить перезапуск. Пустой
	// скелет rule-set всё равно пишется: без файла по пути ядро не стартует.
	AdaptiveSmartMemoryOnly bool `json:"adaptiveSmartMemoryOnly,omitempty"`
```

- [ ] **Шаг 5: константы портов**

В `mobile/libbox.go`, рядом с `BrowserAdBlockSocksPort`:

```go
// SmartRaceInboundPort — loopback-инбаунд, через который туннельная нога реле
// входит обратно в движок. Отдельный от BrowserAdBlockSocksPort намеренно:
// трафик того инбаунда маршрутизируется обычными правилами (так задумано для
// MITM), а нога обязана уходить в proxy безусловно — иначе она вернулась бы в
// реле и соединение закольцевалось.
//
// Он же служит проберу половиной «через узел»: тип mixed отвечает и на SOCKS5,
// и на HTTP CONNECT.
const SmartRaceInboundPort = 18131

// SmartRelayPort — порт самого реле. Сюда смотрит аутбаунд smart-relay.
const SmartRelayPort = 18132
```

- [ ] **Шаг 6: инбаунд, аутбаунд и rule-set в сборке конфига**

В `mobile/libbox.go`, в `buildSingBoxConfigFromEntry`, рядом с блоком
`if opts.BrowserAdBlock { … }` (он добавляет `browser-adblock-in`):

```go
	// Адаптивный Smart: инбаунд для туннельной ноги реле, аутбаунд, смотрящий
	// на само реле, и rule-set выученных «прямых».
	//
	// adaptiveSmartActive, а не opts.AdaptiveSmart: вне Smart-режима фича
	// нерабочая, и гейт живёт здесь, в одном месте, покрытом тестом, а не в
	// Kotlin.
	if adaptiveSmartActive(opts) {
		sb.Inbounds = append(sb.Inbounds, proxy.SBInbound{
			Type:       "mixed",
			Tag:        "smart-race-in",
			Listen:     "127.0.0.1",
			ListenPort: SmartRaceInboundPort,
		})
		sb.Outbounds = append(sb.Outbounds, proxy.SBOutbound{
			Type:       "http",
			Tag:        "smart-relay",
			Server:     "127.0.0.1",
			ServerPort: SmartRelayPort,
		})
		setPath := proxy.SmartDirectSetPath(dataDir)
		// Файл создаётся здесь, а не в реле: ядро читает его в конструкторе
		// rule-set и на отсутствии роняет старт, а порядок «сначала конфиг,
		// потом реле» задаёт Kotlin.
		if err := proxy.EnsureSmartDirectSet(setPath); err == nil && sb.Route != nil {
			sb.Route.RuleSet = append(sb.Route.RuleSet, proxy.SBRouteRuleSet{
				Type:   "local",
				Tag:    "verdict-direct",
				Format: "source",
				Path:   setPath,
			})
		}
	}
```

И рядом:

```go
// adaptiveSmartActive — единственное место, где решается, жива ли фича.
// Smart-режим обязателен: в Global маршрут и так финалится в proxy.
func adaptiveSmartActive(opts BuildOptions) bool {
	return opts.AdaptiveSmart && opts.SmartMode
}
```

- [ ] **Шаг 7: правила в правильном порядке**

Два правила из трёх идут **в разные места**, и перепутать их нельзя.

Первое — туннельная нога — в `buildUserRules`, самым первым из того, что
функция собирает. Её результат вставляется в `userRuleInsertIndex`, то есть
сразу после sniff-пролога:

```go
	// Туннельная нога реле — первой строкой. Она входит своим инбаундом и
	// обязана уйти в proxy безусловно: попади она под «остальное в реле»,
	// соединение закольцевалось бы на себя и страница просто не открылась бы.
	//
	// Перед правилами блокировок она стоит намеренно: исходное соединение уже
	// прошло их до того, как попало в реле, и проверять второй раз нечего.
	//
	// Прямой ноги здесь нет и быть не может: она дозванивается из процесса
	// приложения, исключённого из своего VPN, и в TUN не заходит вовсе.
	if adaptiveSmartActive(opts) {
		rules = append(rules, proxy.SBRouteRule{
			Inbound:  []string{"smart-race-in"},
			Action:   "route",
			Outbound: "proxy",
		})
	}
```

Два остальных — **в самый хвост** `sb.Route.Rules`, в
`buildSingBoxConfigFromEntry`, после блока доменных исключений и всего, что
собрал `buildRoute`. Не в `buildUserRules`: тот вставляется ПЕРЕД встроенными
правилами Smart-списка и ad-block, и правило «остальное в реле» оттуда
проглотило бы и список, и режущие правила рекламы — то есть отменило бы обе
работающие фичи разом.

```go
	// Вся суть блока — двумя последними правилами, уже после Smart-списка,
	// ad-block и исключений. Сперва выученное «ходит напрямую» решается
	// правилом и мимо реле, потом весь остаток TCP уходит учиться.
	//
	// UDP не трогаем: гонки по нему нет, и он по-прежнему проваливается в
	// final.
	if adaptiveSmartActive(opts) && sb.Route != nil {
		sb.Route.Rules = append(sb.Route.Rules,
			proxy.SBRouteRule{
				RuleSet:  []string{"verdict-direct"},
				Action:   "route",
				Outbound: "direct",
			},
			proxy.SBRouteRule{
				Network:  []string{"tcp"},
				Action:   "route",
				Outbound: "smart-relay",
			},
		)
	}
```

Оба правила матчат домен, поэтому обязаны стоять после sniff-пролога — в хвосте
это выполняется само собой.

Добавить в `mobile/libbox_adaptive_smart_test.go` тест, который держит именно
эту границу:

```go
// Правило реле стоит ПОСЛЕ Smart-списка и правил ad-block. Встань оно раньше —
// реле проглотило бы и список, и режущие правила, то есть отменило бы обе
// работающие фичи разом.
func TestAdaptiveSmart_RelayRuleComesAfterBuiltInRules(t *testing.T) {
	opts := smartOpts()
	opts.BlockedDomains = "ads.example"
	cfg := buildAdaptive(t, opts)

	relayIdx, lastBuiltIn := -1, -1
	for i, r := range cfg.Route.Rules {
		if r.Outbound == "smart-relay" {
			relayIdx = i
		}
		if r.Action == "reject" || r.Outbound == "proxy" {
			if i > lastBuiltIn {
				lastBuiltIn = i
			}
		}
	}
	if relayIdx < 0 {
		t.Fatalf("правила реле нет: %+v", cfg.Route.Rules)
	}
	if relayIdx < lastBuiltIn {
		t.Fatalf("реле на позиции %d, а встроенное правило на %d — реле проглотит его", relayIdx, lastBuiltIn)
	}
}
```

Для этого добавить в `adaptiveSmartView` поле `Action` (оно уже есть) и
убедиться, что `BuildOptions` в тесте заполняется `BlockedDomains`.

- [ ] **Шаг 8: тесты зелёные в обеих конфигурациях**

Run:
```bash
TAGS=$(cat scripts/android-build-tags.txt)
go test -tags "$TAGS" ./mobile/ -run TestAdaptiveSmart -v
go test -tags "$TAGS,no_mitm,no_adblock" ./mobile/ -run TestAdaptiveSmart -v
go test -tags "$TAGS" ./mobile/... ./internal/proxy/...
go test -tags "$TAGS,no_mitm,no_adblock" ./mobile/... ./internal/proxy/...
```
Expected: PASS везде. Особенно смотреть на уже существующие тесты правил
(`libbox_routing_rules_test.go`, `browser_adblock_redirect_test.go`): порядок
правил менялся, и если что-то поехало — это оно.

- [ ] **Шаг 9: коммит**

```bash
git add internal/proxy/engine.go mobile/libbox.go mobile/libbox_adaptive_smart_test.go
git commit -m "$(cat <<'EOF'
feat(mobile): движок отправляет неизвестное в реле, а выученное решает правилом

Три добавки в конфиг: инбаунд для туннельной ноги, аутбаунд http на само реле и
локальный rule-set выученных «прямых», который ядро держит под fswatch.

Порядок правил здесь — не оформление. Нога входит своим инбаундом и уходит в
proxy первой строкой: попади она под «остальное в реле», соединение
закольцевалось бы на себя. Прямой ноги в правилах нет вовсе — она дозванивается
из процесса, исключённого из своего VPN, и в TUN не заходит.

Гейт на Smart-режим живёт в одной функции на стороне Go, а не в Kotlin: в Global
весь трафик и так в туннеле, и разыгрывать нечего.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: биндинги жизненного цикла

Kotlin должен поднимать и гасить реле теми же флагами, которыми собирает конфиг.
Конфиг с аутбаундом, за которым никто не слушает, глушит весь трафик, не
покрытый другими правилами, — поэтому порядок задаётся здесь, а проверяется в
Task 8.

**Files:**
- Create: `mobile/libbox_smart.go`
- Test: `mobile/libbox_smart_test.go`

**Interfaces:**
- Consumes: `proxy.StartSmartRelay`, `proxy.SmartRelayOptions`, `(*SmartRelay).Close` (Task 5);
  `SmartRaceInboundPort`, `SmartRelayPort` (Task 6).
- Produces (публичная поверхность gomobile):
  - `StartAdaptiveSmart(dataDir string, memoryOnly bool) (string, error)` —
    возвращает `{"started":true,"port":18132}`
  - `StopAdaptiveSmart()` — идемпотентно
  - `AdaptiveSmartRunning() bool`

- [ ] **Шаг 1: написать падающие тесты**

Create `mobile/libbox_smart_test.go`:

```go
package mobile

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStartAdaptiveSmart_ReportsPortAndRuns(t *testing.T) {
	defer StopAdaptiveSmart()
	out, err := StartAdaptiveSmart(t.TempDir(), true)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	var parsed struct {
		Started bool `json:"started"`
		Port    int  `json:"port"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("ответ не JSON: %v (%s)", err, out)
	}
	if !parsed.Started {
		t.Errorf("started = false, ответ %s", out)
	}
	if parsed.Port != SmartRelayPort {
		t.Errorf("port = %d, ожидался %d — на него смотрит аутбаунд конфига", parsed.Port, SmartRelayPort)
	}
	if !AdaptiveSmartRunning() {
		t.Error("AdaptiveSmartRunning() = false сразу после старта")
	}
}

// Повторный старт не поднимает второе реле и не отдаёт «занят порт»: Kotlin
// зовёт это на каждом подключении, включая перезагрузку конфига.
func TestStartAdaptiveSmart_TwiceIsIdempotent(t *testing.T) {
	defer StopAdaptiveSmart()
	dir := t.TempDir()
	if _, err := StartAdaptiveSmart(dir, true); err != nil {
		t.Fatalf("первый старт: %v", err)
	}
	if _, err := StartAdaptiveSmart(dir, true); err != nil {
		t.Fatalf("второй старт: %v", err)
	}
	if !AdaptiveSmartRunning() {
		t.Error("реле не работает после второго старта")
	}
}

func TestStopAdaptiveSmart_WhenNotRunningIsSafe(t *testing.T) {
	StopAdaptiveSmart()
	StopAdaptiveSmart()
	if AdaptiveSmartRunning() {
		t.Error("AdaptiveSmartRunning() = true после двух остановок")
	}
}

func TestStartAdaptiveSmart_EmptyDataDirRefuses(t *testing.T) {
	out, err := StartAdaptiveSmart("  ", false)
	if err == nil {
		t.Fatalf("ожидался отказ на пустом dataDir, получено %q", out)
	}
	if !strings.Contains(err.Error(), "dataDir") {
		t.Errorf("причина отказа не названа: %v", err)
	}
}
```

- [ ] **Шаг 2: убедиться, что тесты падают**

Run: `go test -tags "$(cat scripts/android-build-tags.txt)" ./mobile/ -run AdaptiveSmart -v`
Expected: FAIL — `undefined: StartAdaptiveSmart`.

- [ ] **Шаг 3: реализовать биндинги**

Create `mobile/libbox_smart.go` (шапка GPL как в соседях):

```go
package mobile

import (
	"fmt"
	"strings"
	"sync"

	"resultproxy-wails/internal/proxy"
)

var (
	smartRelayMu sync.Mutex
	smartRelay   *proxy.SmartRelay
)

// StartAdaptiveSmart поднимает реле адаптивного Smart на loopback.
//
// Зовётся ПЕРЕД стартом движка и на том же флаге, на котором собран конфиг:
// аутбаунд smart-relay, за которым никто не слушает, проглотит весь трафик,
// не покрытый другими правилами.
//
// Повторный вызов ничего не делает: Kotlin зовёт это на каждом подключении, а
// перезагрузка конфига не повод терять выученное этой сессией.
//
// Возвращает JSON {"started":true,"port":<порт>} — тот же контракт
// «JSON или ошибка», что у остальных биндингов этого файла.
func StartAdaptiveSmart(dataDir string, memoryOnly bool) (string, error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", fmt.Errorf("dataDir is required")
	}
	smartRelayMu.Lock()
	defer smartRelayMu.Unlock()
	if smartRelay != nil {
		return fmt.Sprintf(`{"started":true,"port":%d}`, smartRelay.Port()), nil
	}
	relay, err := proxy.StartSmartRelay(proxy.SmartRelayOptions{
		DataDir:    dataDir,
		ListenPort: SmartRelayPort,
		TunnelPort: SmartRaceInboundPort,
		MemoryOnly: memoryOnly,
	})
	if err != nil {
		return "", err
	}
	smartRelay = relay
	return fmt.Sprintf(`{"started":true,"port":%d}`, relay.Port()), nil
}

// StopAdaptiveSmart гасит реле и сохраняет выученное. Безопасно звать, когда
// оно не поднято.
func StopAdaptiveSmart() {
	smartRelayMu.Lock()
	relay := smartRelay
	smartRelay = nil
	smartRelayMu.Unlock()
	if relay != nil {
		_ = relay.Close()
	}
}

// AdaptiveSmartRunning — поднято ли реле. Нужно Kotlin для строки в логе и
// тестам, чтобы не гадать по порту.
func AdaptiveSmartRunning() bool {
	smartRelayMu.Lock()
	defer smartRelayMu.Unlock()
	return smartRelay != nil
}
```

- [ ] **Шаг 4: тесты зелёные в обеих конфигурациях**

Run:
```bash
TAGS=$(cat scripts/android-build-tags.txt)
go test -tags "$TAGS" ./mobile/ -run AdaptiveSmart -v
go test -tags "$TAGS,no_mitm,no_adblock" ./mobile/ -run AdaptiveSmart -v
```
Expected: PASS в обеих.

- [ ] **Шаг 5: проверить, что play-граф не потянул лишнего**

Run:
```bash
go list -deps -tags "$(cat scripts/android-build-tags.txt),no_mitm,no_adblock" ./mobile/ | grep -E 'internal/(filter|adblock)' || echo "чисто"
```
Expected: `чисто`. Реле не должно затащить фильтрацию в play-сборку.

- [ ] **Шаг 6: коммит**

```bash
git add mobile/libbox_smart.go mobile/libbox_smart_test.go
git commit -m "$(cat <<'EOF'
feat(mobile): биндинги старта и остановки реле адаптивного Smart

Реле поднимается перед движком и на том же флаге, которым собран конфиг:
аутбаунд, за которым никто не слушает, проглотил бы весь трафик, не покрытый
другими правилами.

Повторный старт ничего не делает. Kotlin зовёт это на каждом подключении, а
перезагрузка конфига — не повод терять выученное этой сессией.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: Kotlin — настройка и проводка в жизненный цикл

**Files:**
- Modify: `android/app/src/main/java/com/resultv/android/vpn/SettingsRepository.kt`
- Modify: `android/app/src/main/java/com/resultv/android/vpn/BuildOptions.kt`
- Modify: `android/app/src/main/java/com/resultv/android/vpn/ResultVpnService.kt`
- Test: `android/app/src/test/java/com/resultv/android/vpn/AdaptiveSmartOptionsTest.kt`

**Interfaces:**
- Consumes: `Mobile.startAdaptiveSmart(dataDir, memoryOnly)`,
  `Mobile.stopAdaptiveSmart()` (Task 7 — gomobile отдаёт их в Kotlin с маленькой
  буквы), поля `adaptiveSmart` / `adaptiveSmartMemoryOnly` в `optionsJson` (Task 6).
- Produces: `SettingsState.adaptiveSmart: Boolean`,
  `SettingsState.adaptiveSmartMemoryOnly: Boolean`,
  `SettingsRepository.setAdaptiveSmart(Boolean)`,
  `SettingsRepository.setAdaptiveSmartMemoryOnly(Boolean)`.

- [ ] **Шаг 1: написать падающий тест на JSON настроек**

Create `android/app/src/test/java/com/resultv/android/vpn/AdaptiveSmartOptionsTest.kt`:

```kotlin
package com.resultv.android.vpn

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Имена ключей в optionsJson — контракт с Go (BuildOptions). Опечатка здесь
 * молча выключает фичу: неизвестное поле JSON Go просто не заполнит, конфиг
 * соберётся без реле, и понять это можно будет только по отсутствию строки в
 * логе.
 */
class AdaptiveSmartOptionsTest {

    @Test
    fun `ключи адаптивного Smart совпадают с ожидаемыми Go`() {
        val json = JSONObject()
            .put("adaptiveSmart", true)
            .put("adaptiveSmartMemoryOnly", false)

        assertTrue(json.getBoolean("adaptiveSmart"))
        assertEquals(false, json.getBoolean("adaptiveSmartMemoryOnly"))
    }

    @Test
    fun `состояние по умолчанию выключено`() {
        val state = SettingsState()
        assertEquals(false, state.adaptiveSmart)
        assertEquals(false, state.adaptiveSmartMemoryOnly)
    }
}
```

- [ ] **Шаг 2: убедиться, что тест падает**

Run: `cd android && ./gradlew :app:testFullDebugUnitTest --tests '*AdaptiveSmartOptionsTest*'`
Expected: FAIL — компиляция, `Unresolved reference: adaptiveSmart`.

- [ ] **Шаг 3: поля и ключи в `SettingsRepository.kt`**

В `data class SettingsState`, после `pingTimeoutSec`:

```kotlin
    /**
     * Адаптивный Smart: движок сам узнаёт, какие сайты не открываются напрямую,
     * разыгрывая прямой путь против туннеля в loopback-реле и запоминая исход.
     * Работает только в Smart-режиме — в Global весь трафик и так в туннеле.
     */
    val adaptiveSmart: Boolean = false,
    /** Выученное не переживает перезапуск. */
    val adaptiveSmartMemoryOnly: Boolean = false,
```

Рядом с остальными ключами:

```kotlin
    private const val K_ADAPTIVE_SMART = "adaptive_smart"
    private const val K_ADAPTIVE_SMART_MEMORY = "adaptive_smart_memory_only"
```

В `init`, рядом с `pingTimeoutSec`:

```kotlin
            adaptiveSmart = prefs.getBoolean(K_ADAPTIVE_SMART, false),
            adaptiveSmartMemoryOnly = prefs.getBoolean(K_ADAPTIVE_SMART_MEMORY, false),
```

И сеттеры рядом с `setIpv6`:

```kotlin
    fun setAdaptiveSmart(enabled: Boolean) = mutate {
        prefs.edit().putBoolean(K_ADAPTIVE_SMART, enabled).apply()
        it.copy(adaptiveSmart = enabled)
    }

    fun setAdaptiveSmartMemoryOnly(enabled: Boolean) = mutate {
        prefs.edit().putBoolean(K_ADAPTIVE_SMART_MEMORY, enabled).apply()
        it.copy(adaptiveSmartMemoryOnly = enabled)
    }
```

Гейта на `BuildConfig` здесь нет и не нужно: фича в обеих сборках (решение §3.3
спеки), фильтрацию она не трогает.

- [ ] **Шаг 4: флаги в `BuildOptions.kt`**

В `currentOptionsJson`, после `.put("browserAdBlockPort", BROWSER_ADBLOCK_PORT)`:

```kotlin
            // Адаптивный Smart. Тот же флаг поднимает реле в
            // ResultVpnService: конфиг с аутбаундом, за которым никто не
            // слушает, проглотит весь трафик, не покрытый другими правилами.
            .put("adaptiveSmart", settings.adaptiveSmart)
            .put("adaptiveSmartMemoryOnly", settings.adaptiveSmartMemoryOnly)
```

- [ ] **Шаг 5: поднять и погасить реле в `ResultVpnService.kt`**

В `startWithConfig`, **до** `BoxModule.start(...)`, рядом со строкой
`BoxModule.filterProxyRunning = false`:

```kotlin
            // Реле адаптивного Smart поднимается ДО движка: конфиг уже
            // содержит аутбаунд, который в него смотрит, и первый же
            // незнакомый хост пошёл бы в никуда.
            startAdaptiveSmartIfEnabled()
```

Метод рядом с `startBrowserAdBlockIfEnabled`:

```kotlin
    /**
     * Поднять реле адаптивного Smart, если тумблер включён.
     *
     * Срыв не фатален и не отменяет подключение: движок в этом случае просто
     * не научится ничему новому. Но молчать нельзя — иначе «фича включена, а
     * ничего не происходит» будет выглядеть как поломка маршрутизации.
     */
    private fun startAdaptiveSmartIfEnabled() {
        val settings = SettingsRepository.state.value
        if (!settings.adaptiveSmart) {
            mobile.Mobile.stopAdaptiveSmart()
            return
        }
        try {
            mobile.Mobile.startAdaptiveSmart(
                filesDir.absolutePath,
                settings.adaptiveSmartMemoryOnly,
            )
            Log.i(TAG, "adaptive smart relay started")
        } catch (t: Throwable) {
            Log.w(TAG, "adaptive smart relay failed", t)
            AppLog.warning(
                R.string.log_adaptive_smart_failed,
                t.message ?: t.javaClass.simpleName,
                source = AppLog.resolve(R.string.log_source_proxy),
            )
        }
    }
```

В обоих местах остановки (там, где сейчас зовётся `BoxModule.stop()` по
отключению — строки около 489 и 515) добавить сразу после:

```kotlin
            mobile.Mobile.stopAdaptiveSmart()
```

В `triggerReload` (около строки 667) этого делать НЕ надо: перезагрузка конфига
не должна терять выученное этой сессией, а повторный `startAdaptiveSmart`
идемпотентен.

- [ ] **Шаг 6: строки лога**

В `android/app/src/main/res/values/strings.xml`:

```xml
    <string name="log_adaptive_smart_failed">Adaptive Smart is on, but its relay did not start: %1$s. Routing is unchanged; nothing new will be learned.</string>
```

В `android/app/src/main/res/values-ru/strings.xml`:

```xml
    <string name="log_adaptive_smart_failed">Адаптивный Smart включён, но реле не поднялось: %1$s. Маршрутизация прежняя, новое не выучится.</string>
```

- [ ] **Шаг 7: тесты Kotlin зелёные в обеих сборках**

Run:
```bash
cd android
./gradlew :app:testFullDebugUnitTest
./gradlew :app:testPlayDebugUnitTest
```
Expected: BUILD SUCCESSFUL, число тестов не меньше прежнего (было full 140,
play 138) плюс два новых.

- [ ] **Шаг 8: коммит**

```bash
git add android/app/src/main/java/com/resultv/android/vpn/SettingsRepository.kt \
        android/app/src/main/java/com/resultv/android/vpn/BuildOptions.kt \
        android/app/src/main/java/com/resultv/android/vpn/ResultVpnService.kt \
        android/app/src/main/res/values/strings.xml \
        android/app/src/main/res/values-ru/strings.xml \
        android/app/src/test/java/com/resultv/android/vpn/AdaptiveSmartOptionsTest.kt
git commit -m "$(cat <<'EOF'
feat(android): адаптивный Smart доезжает от настройки до реле

Один флаг поднимает реле и собирает конфиг, и порядок между ними не
переставляется: аутбаунд, за которым никто не слушает, проглотил бы весь
трафик, не покрытый другими правилами.

Перезагрузка конфига реле не гасит. Выученное этой сессией не повод терять
из-за смены списка или срабатывания кил-свитча, а повторный старт ничего не
делает.

Срыв старта реле подключение не отменяет, но пишет строку: «включено, а ничего
не происходит» иначе читалось бы как поломка маршрутизации.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 9: UI — тумблер и подтумблер

**Files:**
- Modify: `android/app/src/main/java/com/resultv/android/ui/screens/SettingsScreen.kt`
- Modify: `android/app/src/main/res/values/strings.xml`
- Modify: `android/app/src/main/res/values-ru/strings.xml`

**Interfaces:**
- Consumes: `SettingsState.adaptiveSmart`, `setAdaptiveSmart`,
  `adaptiveSmartMemoryOnly`, `setAdaptiveSmartMemoryOnly` (Task 8).
- Produces: UI, других потребителей нет.

- [ ] **Шаг 1: строки**

`values/strings.xml`:

```xml
    <string name="settings_adaptive_smart">Adaptive Smart</string>
    <string name="settings_adaptive_smart_subtitle">Learns which sites need the tunnel instead of waiting for the list. Smart mode only.</string>
    <string name="settings_adaptive_smart_memory">Memory only</string>
    <string name="settings_adaptive_smart_memory_subtitle">Forget everything learned when the app closes.</string>
```

`values-ru/strings.xml`:

```xml
    <string name="settings_adaptive_smart">Адаптивный Smart</string>
    <string name="settings_adaptive_smart_subtitle">Сам узнаёт, каким сайтам нужен туннель, не дожидаясь списка. Только в режиме Smart.</string>
    <string name="settings_adaptive_smart_memory">Только в памяти</string>
    <string name="settings_adaptive_smart_memory_subtitle">Забывать выученное при закрытии приложения.</string>
```

- [ ] **Шаг 2: тумблеры в группе «Сеть»**

В `SettingsScreen.kt`, в `NetworkGroup`, между тумблером IPv6 и вызовом
`PingGroup(settings)`:

```kotlin
    HorizontalDivider(color = RvColor.whiteA10)
    ToggleRow(
        title = stringResource(R.string.settings_adaptive_smart),
        subtitle = stringResource(R.string.settings_adaptive_smart_subtitle),
        icon = Icons.Outlined.AutoAwesome,
        tint = RvCategory.Green,
        checked = settings.adaptiveSmart,
        onCheckedChange = { SettingsRepository.setAdaptiveSmart(it) },
    )
    // Подтумблер показывается только при включённом основном: висящая в
    // интерфейсе настройка того, чего нет, — это вопрос, на который человеку
    // приходится отвечать зря.
    if (settings.adaptiveSmart) {
        HorizontalDivider(color = RvColor.whiteA10)
        ToggleRow(
            title = stringResource(R.string.settings_adaptive_smart_memory),
            subtitle = stringResource(R.string.settings_adaptive_smart_memory_subtitle),
            icon = Icons.Outlined.Memory,
            tint = RvCategory.Violet,
            checked = settings.adaptiveSmartMemoryOnly,
            onCheckedChange = { SettingsRepository.setAdaptiveSmartMemoryOnly(it) },
        )
    }
```

Импорты `androidx.compose.material.icons.outlined.AutoAwesome` и
`androidx.compose.material.icons.outlined.Memory` — добавить к существующим
импортам иконок. Если какой-то из них отсутствует в подключённом наборе
`material-icons-extended`, взять ближайший из уже используемых в файле
(`Icons.Outlined.Bolt`, `Icons.Outlined.Storage`) — но НЕ добавлять новую
зависимость ради иконки.

- [ ] **Шаг 3: сборка и тесты обеих сборок**

Run:
```bash
cd android
./gradlew :app:assembleFullDebug :app:assemblePlayDebug
./gradlew :app:testFullDebugUnitTest :app:testPlayDebugUnitTest
```
Expected: BUILD SUCCESSFUL на всех четырёх задачах.

- [ ] **Шаг 4: коммит**

```bash
git add android/app/src/main/java/com/resultv/android/ui/screens/SettingsScreen.kt \
        android/app/src/main/res/values/strings.xml \
        android/app/src/main/res/values-ru/strings.xml
git commit -m "$(cat <<'EOF'
feat(android): тумблер адаптивного Smart в настройках

В группе «Сеть», рядом с IPv6 и обходом LAN: это тумблер поведения движка, а не
пользовательское правило, и в шторке «Правила» его искали бы зря.

Подписью сказано, что фича работает только в Smart-режиме. Догадываться об этом
по отсутствию эффекта человек не должен.

Подтумблер «только в памяти» показывается лишь при включённом основном:
настройка того, чего нет, — это вопрос, на который приходится отвечать зря.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 10: устаревший комментарий, AAR и приёмка на телефоне

Последняя задача — единственная, которую нельзя закрыть зелёными тестами.

**Files:**
- Modify: `internal/proxy/ping_engine.go` (комментарий, который стал неправдой)
- Modify: `docs/superpowers/specs/2026-09-17-android-core-1.14-sync-design.md`
  (раздел «Блок 4 — что сделано и чем доказано»)

- [ ] **Шаг 1: поправить комментарий про расширенный реестр**

В `internal/proxy/ping_engine.go`, около строки 191, заменить:

```go
	// include.Context, not the extended one: the custom outbound registry
	// arrives with the adaptive Smart block. When it does, this is the line to
	// change — the probe must build a node the same way the session does.
	boxCtx = include.Context(boxCtx)
```

на:

```go
	// include.Context — и другого здесь не будет. Расширенного реестра
	// аутбаундов на Android не появится: коробку сессии строит libbox, а
	// реестр зашит в его baseContext (спека §7.0). Адаптивный Smart поэтому
	// живёт вне ядра, в loopback-реле, и узел проба строит ровно так же, как
	// его строит сессия.
	boxCtx = include.Context(boxCtx)
```

Run: `go build -tags "$(cat scripts/android-build-tags.txt)" ./internal/proxy/...`
Expected: exit 0.

- [ ] **Шаг 2: собрать AAR ключом подписок**

```bash
set -a; . /c/ResultV/.env; set +a
scripts/build-android-aar.sh 2>&1 | tail -20
ls -la android/app/libs/*.aar
```
Expected: строка `✅` и свежее время файла `.aar`. Скрипт через пайп отдаёт код 0
и на упавшей сборке (§12.3) — проверять глазами по строке и по времени файла.

- [ ] **Шаг 3: поставить на телефон**

```bash
cd android && ./gradlew :app:assembleFullDebug -Pdebug.abi=arm64-v8a
adb -s e3bacc6b install -r -d app/build/outputs/apk/full/debug/app-full-debug.apk
```
Expected: `Success`. Приложение НЕ удалять — на телефоне реальные профили.

- [ ] **Шаг 4: приёмка (спека §7.8), по пунктам**

Включить Smart-режим и тумблер «Адаптивный Smart», подключиться к рабочему узлу.

1. **Незнакомое имя разыгрывается и запоминается.** Открыть сайт, которого нет в
   Smart-списке. В логе — строка подключения. Затем:
   ```bash
   adb -s e3bacc6b shell run-as com.resultv.android cat files/smart/verdict-direct.json
   ```
   Expected: имена, которые гонка отдала прямому пути. Заблокированного сайта
   здесь быть не должно — он в хешированном сторе.
2. **Второй заход идёт мимо реле.** Повторно открыть тот же сайт. Проверить, что
   имя уже в `verdict-direct.json` — значит решение принимает правило, а не реле.
3. **Заблокированный сайт открывается.** Взять сайт, заблокированный в РФ и
   отсутствующий в Smart-списке. Первый заход — гонка, дальше страница
   открывается. Это то, ради чего блок существует.
4. **Браузерный ad-block не пострадал** (`full`): баннеры по-прежнему режутся,
   страницы не падают, видео играет.
5. **Перезапуск.** Отключиться, убить приложение, подключиться снова. Файл
   `verdict-direct.json` на месте и не пуст, повторный заход на выученный сайт
   идёт правилом.
6. **Живой AWG-узел.** Переключиться на AmneziaWG-узел и повторить пункты 1–3:
   ограничение с ПК снято (§7.5), и это единственное место, где оно
   проверяется.
7. **`play`-сборка.** Собрать и поставить play-вариант, повторить пункты 1–3 без
   ad-block.

При любом отказе — **не чинить наугад**: `superpowers:systematic-debugging`,
фаза 1. Лог одноразового движка снимается временно через
`SBLog{Level:"debug", Output:"<filesDir>/…log"}` и `run-as … cat` (§12.4.5).

- [ ] **Шаг 5: записать, что доказано**

Дописать в спеку раздел «## 15. Блок 4 — что сделано и чем доказано
(дата)» по образцу §13 и §14: развилка, чем доказано (команды и их вывод), что
нашла приёмка, что сознательно не поехало, что осталось непроверенным. Обновить
строку статуса в шапке документа и §12.1.

- [ ] **Шаг 6: коммит**

```bash
git add internal/proxy/ping_engine.go
git add -f docs/superpowers/specs/2026-09-17-android-core-1.14-sync-design.md \
           docs/superpowers/plans/2026-09-17-android-adaptive-smart.md
git commit -m "$(cat <<'EOF'
docs(android): блок 4 закрыт — адаптивный Smart на телефоне

Комментарий в ping_engine.go обещал расширенный реестр аутбаундов «когда
приедет адаптивный Smart». Он не приедет: коробку сессии строит libbox, реестр
зашит в его baseContext. Обещание заменено на причину.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

## Что этот план сознательно не делает

Записано здесь, а не потеряно: следующий агент прочтёт «этого нет» как «забыли»,
если не сказать иначе.

- **FakeIP** — §7.5 спеки. Sniff уже даёт имя, реле получает его в CONNECT.
- **Политика UDP** — §7.5. QUIC ходит как ходил.
- **Учёт трафика по фактическому выбору** — §7.5. Счётчики берутся из `clash_api`.
- **Запрет на WG/AWG** — §7.5, снят; проверяется пунктом 6 приёмки.
- **Namespace по сети** (`store.SetNamespace`) — стор умеет, но на Android никто
  не сообщает ему о смене сети. Все вердикты в `default`. Это осознанный
  недобор: смена Wi-Fi ↔ LTE меняет, что заблокировано, и однажды это
  понадобится — но сперва надо увидеть, что фича вообще работает.
- **Показ выученного в UI** («почему этот сайт в туннеле») — на ПК это есть,
  здесь нет. Отдельная работа, и она не нужна, чтобы блок заработал.
