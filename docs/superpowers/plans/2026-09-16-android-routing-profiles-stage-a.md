# Профили маршрутизации, этап A — Go-ядро и движок

**ИСПОЛНЕН 2026-09-16.** Все задачи закрыты, запись о результате — в спеке,
раздел 12. Три неточности, вскрывшиеся при исполнении, уже вправлены в текст
ниже (коммит 382dcdf), плюс четыре отступления записаны в разделе 12 спеки.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** научить Go-слой Android разбирать профиль маршрутизации, разворачивать его правила из geo-баз, компилировать их в бинарный SRS и подмешивать в конфиг sing-box — без единой строки Kotlin.

**Architecture:** пять файлов переносятся с ПК (`C:\ResultVPC`, ветка `dev`), четыре пишутся заново под мобильные реалии: SRS вместо source-JSON, чистые функции вместо методов `*App`. Правила профиля эмитятся в `mobile/libbox_routing.go` — после исключений «мимо ВПН», которые дописываются уже за пределами `buildRoute`. `internal/proxy/engine.go` не трогается.

**Tech Stack:** Go 1.26, sing-box (форк sagernet), gomobile v0.1.12, `github.com/sagernet/sing-box/common/srs`.

**Spec:** `docs/superpowers/specs/2026-09-16-android-routing-profiles-design.md`

## Global Constraints

- **Теги сборки обязательны.** Базовый набор в `scripts/android-build-tags.txt`:
  `mobile,with_gvisor,with_utls,with_clash_api,with_quic,with_wireguard,with_grpc`.
  Без них пакеты `mobile` и `internal/proxy` не собираются, а LSP покажет
  «undefined» на существующие символы — это шум, а не ошибка.
- **Две конфигурации.** `full` — базовые теги. `play` — базовые плюс
  `no_mitm,no_adblock`. Любая задача считается сделанной, только когда зелены обе.
- **Базовая линия проверена 2026-09-16:** `go build ./...` и
  `go test -count=1 ./internal/proxy/... ./mobile/...` зелены в обеих
  конфигурациях (`internal/proxy` 11.6 с, `mobile` 8.5 с).
- **`internal/proxy/engine.go` и `EngineConfig` в этом этапе не изменяются.**
  Если задача требует их правки — это ошибка в плане, остановиться и сказать.
- **Исходник переноса:** `C:\ResultVPC`, ветка `dev`, дерево чистое. Файлы
  берутся оттуда, а не из `C:\ResultV\ResultV-dev` (это копия без `.git`).
- **Комментарии в переносимых файлах не переписываются.** Они объясняют, почему
  код такой; переписывание ради стиля ломает сверку с ПК при следующем переносе.
- **Язык коммитов — русский**, как во всей ветке `android`.

**Команды, используемые во всех задачах:**

```bash
cd /c/ResultV
TAGS=$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)

# full
go build -tags="$TAGS" ./... && go test -tags="$TAGS" -count=1 ./internal/proxy/... ./mobile/...

# play
go build -tags="$TAGS,no_mitm,no_adblock" ./... && \
  go test -tags="$TAGS,no_mitm,no_adblock" -count=1 ./internal/proxy/... ./mobile/...
```

---

## Карта файлов

| Файл | Что делает | Задача |
|---|---|---|
| `internal/proxy/routinglist.go` | разбор списка доменов/CIDR, нормализация URL, порядок действий | 1 |
| `internal/proxy/geodat.go` | чтение `geosite.dat` / `geoip.dat` (protobuf вручную) | 2 |
| `internal/proxy/georesolve.go` | разворачивание xray-токенов в `ParsedRoutingList` | 2 |
| `internal/proxy/routingprofile.go` | разбор диплинка и payload профиля, лимиты | 3 |
| `internal/proxy/sublists.go` | извлечение маршрутизации из ответа подписки | 4 |
| `internal/proxy/routing_ruleset.go` | компиляция в бинарный SRS, пути кэша, удаление | 5 |
| `internal/proxy/routing_merge.go` | идентичность профиля, upsert, лимит числа профилей | 6 |
| `internal/proxy/routing_fetch.go` | загрузка списков и geo-баз с защитой от приватных адресов | 7 |
| `internal/proxy/routing_compile.go` | кэш geo-баз, сборка трёх SRS, отчёт | 8 |
| `mobile/libbox_routing.go` | биндинги gomobile + эмиссия правил в конфиг | 9, 10 |

---

### Task 1: Разбор списков маршрутизации

`georesolve.go` (задача 2) использует `looksLikeCIDROrIP` и `plausibleDomains`
отсюда, поэтому этот файл идёт первым.

**Files:**
- Create: `internal/proxy/routinglist.go`
- Test: `internal/proxy/routinglist_test.go`

**Interfaces:**
- Consumes: `compressDomainSuffixes` (`blocked_provider.go:364`), `extractDomainFromLine` (`blocked_provider.go:247`), `normalizeDomains` (`router.go:302`), `normalizeCIDRs` (`blocked_cidrs.go:45`)
- Produces: `type ParsedRoutingList struct { Domains, CIDRs, ExactDomains []string }`, `func ParseRoutingListPayload(raw []byte) ParsedRoutingList`, `func NormalizeRoutingListURL(raw string) string`, `func LooksLikeRoutingListHTML(raw []byte) bool`, `func NormalizeRoutingOrder(raw string) []string`, `var DefaultRoutingOrder = []string{"block", "proxy", "direct"}`, `func plausibleDomains(in []string) []string`, `func looksLikeCIDROrIP(s string) bool`

- [x] **Step 1: Скопировать файл и тест с ПК**

```bash
cd /c/ResultV
cp /c/ResultVPC/internal/proxy/routinglist.go internal/proxy/routinglist.go
cp /c/ResultVPC/internal/proxy/routinglist_test.go internal/proxy/routinglist_test.go
```

- [x] **Step 2: Вырезать половину, которая пишет кэш**

Удалить из `internal/proxy/routinglist.go`:

- `type RoutingListSpec struct {…}`
- `func buildRoutingListRuleSets(…) []SBRuleSet`
- `func appendRoutingListRouteRules(…) []SBRouteRule`
- `func routingListCacheReady(path string) bool`
- `const routingListsSubdir`, `const routingListRuleSetVersion` (весь блок `const`)
- `func RoutingListsDir(dataDir string) string`
- `func RoutingListCachePath(dataDir, id string) string`
- `func RoutingListRuleSetTag(id string) string`
- `func WriteRoutingListRuleSet(dataDir, id string, p ParsedRoutingList) error`

Причина: первые три собирают `SBRuleSet`/`SBLocalRuleSet` — структуру ПК
(`ResultVPC/internal/proxy/engine.go:203-229`), на `android` её нет, там плоский
`SBRouteRuleSet` (`engine.go:418-427`). Остальное пишет source-JSON, который
заменён на SRS (спека, решение 2.3).

`srcRuleSetFile` и `srcRuleSetRule` **остаются** — их читает
`parseSourceJSONRuleSet`, когда список приходит в форме sing-box source-JSON.

Привести блок импортов к тому, что осталось:

```go
import (
	"encoding/json"
	"strings"
)
```

- [x] **Step 3: Вырезать тесты удалённых функций**

Удалить из `internal/proxy/routinglist_test.go` три функции:
`TestWriteRoutingListRuleSet`, `TestWriteRoutingListRuleSetEmptyRejected`,
`TestRoutingListRuleSetTagStable` (строки 160-196 в исходнике). Их предмет
вернётся в задаче 5 в SRS-виде.

Импорты файла после этого — `encoding/json`, `os`, `testing`; первые два
становятся неиспользуемыми, остаётся только `testing`. Проверять не глазами, а
`go vet`: он называет неиспользуемый импорт по имени и строке.

- [x] **Step 4: Запустить тесты — должны пройти**

```bash
cd /c/ResultV && TAGS=$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)
go test -tags="$TAGS" -count=1 ./internal/proxy/ -run 'RoutingList' -v
```

Ожидается: 9 тестов PASS (`TestNormalizeRoutingListURL`,
`TestLooksLikeRoutingListHTML`, `TestParseRoutingListRejectsHTML`,
`TestParseRoutingListDropsJunkTokens`, `TestParseRoutingListPlainText`,
`TestParseRoutingListSourceJSON`, `TestParseRoutingListEmpty`,
`TestParseRoutingListMalformedJSONFallsBackToLines`,
`TestParseRoutingListPlainTextIPv6CIDR`).

- [x] **Step 5: Обе конфигурации целиком**

```bash
cd /c/ResultV && TAGS=$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)
go build -tags="$TAGS" ./... && go test -tags="$TAGS" -count=1 ./internal/proxy/... ./mobile/...
go build -tags="$TAGS,no_mitm,no_adblock" ./... && go test -tags="$TAGS,no_mitm,no_adblock" -count=1 ./internal/proxy/... ./mobile/...
```

Ожидается: `ok` по обоим пакетам в обеих конфигурациях.

- [x] **Step 6: Коммит**

```bash
git add internal/proxy/routinglist.go internal/proxy/routinglist_test.go
git commit -m "feat(routing): перенести разбор списков маршрутизации с ПК

Без половины, которая пишет кэш: buildRoutingListRuleSets собирает SBRuleSet,
которого на android нет, а WriteRoutingListRuleSet пишет source-JSON вместо
SRS. Обе заменяются в routing_ruleset.go.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 2: Чтение geo-баз и разворачивание токенов

**Files:**
- Create: `internal/proxy/geodat.go`
- Create: `internal/proxy/georesolve.go`
- Test: `internal/proxy/geodat_test.go`

**Interfaces:**
- Consumes: `ParsedRoutingList`, `plausibleDomains`, `looksLikeCIDROrIP` (задача 1); `normalizeDomains`, `normalizeCIDRs`, `compressDomainSuffixes`, `extractDomainFromLine`
- Produces: `func ParseGeoSiteDat(raw []byte) (map[string][]GeoDomain, int, error)`, `func ParseGeoIPDat(raw []byte) (map[string][]string, []string, error)`, `type GeoDomain struct { Value string; Exact bool }`, `type GeoDatabases struct { Sites map[string][]GeoDomain; IPs map[string][]string; SiteDropped int; InvertedIPs map[string]struct{} }`, `func ResolveGeoTokens(tokens []string, db GeoDatabases) (ParsedRoutingList, GeoResolveReport)`, `type GeoResolveReport struct { Unresolved map[string]string; DroppedFromDB, DroppedAsJunk int }`, `var ErrGeoDatMalformed`

- [x] **Step 1: Скопировать три файла без изменений**

```bash
cd /c/ResultV
cp /c/ResultVPC/internal/proxy/geodat.go      internal/proxy/geodat.go
cp /c/ResultVPC/internal/proxy/georesolve.go  internal/proxy/georesolve.go
cp /c/ResultVPC/internal/proxy/geodat_test.go internal/proxy/geodat_test.go
```

Оба файла чистые: `geodat.go` импортирует только `errors`, `fmt`, `net`,
`net/netip`, `strings`; `georesolve.go` — `fmt`, `sort`, `strings`. Править в
них нечего.

- [x] **Step 2: Вырезать два теста, чей предмет ещё не написан**

Удалить из `internal/proxy/geodat_test.go` **ровно две функции**:
`TestWriteRoutingListRuleSetKeepsExactDomainsApart` и
`TestWriteRoutingListRuleSetAcceptsExactOnlyList` (строки 350-389 в исходнике).
В задаче 5 они вернутся, проверяя SRS.

**Осторожно с хвостом.** Сразу за ними, со строки 390, идут хелперы `keysOf`,
`sortedEqual`, `sortStringsForTest` — их зовут перенесённые тесты. Резать «от
первой функции и до конца файла» нельзя: получится `undefined: keysOf`.
Вырезать надо интервал между началом первой функции и строкой
`func keysOf(m map[string][]GeoDomain) []string {`.

После удаления неиспользуемым остаётся импорт `os` — снять его, ориентируясь
на `go vet`.

- [x] **Step 3: Запустить тесты — должны пройти**

```bash
cd /c/ResultV && TAGS=$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)
go test -tags="$TAGS" -count=1 ./internal/proxy/ -run 'Geo' -v
```

Ожидается: 12 тестов PASS — семь про разбор `.dat`
(`TestParseGeoSiteDatKindsAndCase`, `…MergesRepeatedCategory`,
`…RejectsGarbage`, `…MalformedIsTyped`, `TestParseGeoIPDatAddressForms`,
`…DropsInverseMatch`, `…SkipsBadAddresses`) и пять про резолв
(`TestResolveGeoTokensExpandsCategories`, `…ReportsWhatItCannotDo`,
`…PlainForms`, `…DropsExactCoveredBySuffix`, `…WithoutDatabases`).

- [x] **Step 4: Обе конфигурации целиком**

Команды из «Global Constraints». Ожидается `ok` по обоим пакетам дважды.

- [x] **Step 5: Коммит**

```bash
git add internal/proxy/geodat.go internal/proxy/georesolve.go internal/proxy/geodat_test.go
git commit -m "feat(routing): перенести чтение geo-баз и разворачивание токенов

geosite.dat и geoip.dat читаются вручную, без зависимости на protobuf: четыре
крошечных сообщения, замороженных форматом файла. sing-box выбросил поддержку
.dat в 1.8, поэтому категории разворачиваются здесь.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 3: Разбор диплинка и payload профиля

**Files:**
- Create: `internal/proxy/routingprofile.go`
- Test: `internal/proxy/routingprofile_test.go`

**Interfaces:**
- Consumes: `IsDeepLink` (`deeplink.go:26`), `DeepLinkScheme` (`deeplink.go:19`), `deepLinkSchemeOpaque` (`deeplink.go:23`), `sanitizeBase64` (`deeplink.go:152`), `config.RoutingProfile`
- Produces: `func IsRoutingDeepLink(rawURL string) bool`, `func DeepLinkKind(rawURL string) string`, `func DecodeRoutingDeepLink(rawURL string) (config.RoutingProfile, error)`, `func ParseRoutingProfileJSON(blob []byte) (config.RoutingProfile, error)`, `func RoutingProfileTokens(p config.RoutingProfile, action string) []string`, `const DeepLinkKindSubscription = "subscription"`, `const DeepLinkKindRouting = "routing"`, `const MaxRoutingProfileTokens = 20000`, `const MaxRoutingProfileNameLen = 200`, `const MaxRoutingDeepLinkPayload = 4 << 20`, `var ErrNotRoutingDeepLink`

- [x] **Step 1: Скопировать файл и тест без изменений**

```bash
cd /c/ResultV
cp /c/ResultVPC/internal/proxy/routingprofile.go      internal/proxy/routingprofile.go
cp /c/ResultVPC/internal/proxy/routingprofile_test.go internal/proxy/routingprofile_test.go
```

Все четыре зависимости на `android` есть — проверено. Править нечего.

- [x] **Step 2: Запустить тесты — должны пройти**

```bash
cd /c/ResultV && TAGS=$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)
go test -tags="$TAGS" -count=1 ./internal/proxy/ -run 'Routing(Profile|DeepLink)|DeepLinkKind' -v
```

Ожидается: все PASS, ноль FAIL.

- [x] **Step 3: Добавить тест на чужой вход**

Проверить сперва, что уже покрыто:

```bash
cd /c/ResultV && grep -n "func Test" internal/proxy/routingprofile_test.go
```

Перенесённый файл уже содержит `TestRoutingDeepLinkAcceptedSpellings` (восемь
форм ссылки: `onadd`, `add`, голый `routing/`, опаковая схема, три вида base64,
завершающий слеш, верхний регистр) и `TestSubscriptionLinksAreNotRouting` (пять
форм ссылки подписки плюс `ErrNotRoutingDeepLink`). Писать это заново не надо.

Не покрыт ровно один случай: вход, который не является `resultv://`-ссылкой
вовсе. На ПК такого вопроса не возникает — его импортёр доходит до этого кода
уже со ссылкой в руках. На Android поле вставки (`AddScreen`, этап C) отдаст
сюда что угодно. Дописать в `internal/proxy/routingprofile_test.go`:

```go
// Input that is not a resultv:// link at all must not be classified as
// routing. The desktop never asks that question — its importer only reaches
// this code with a resultv:// link in hand. The Android paste field does: it
// hands whatever the user pasted to IsRoutingDeepLink before anything else has
// looked at it.
func TestIsRoutingDeepLinkIgnoresForeignInput(t *testing.T) {
	for _, raw := range []string{
		"",
		"   ",
		"https://panel.example/routing/resultv/whitelist",
		"vless://11111111-1111-1111-1111-111111111111@1.2.3.4:443",
		"routing/onadd/eyJ9",
		"example.com",
	} {
		if IsRoutingDeepLink(raw) {
			t.Errorf("IsRoutingDeepLink(%q) = true, ждали false", raw)
		}
	}
}
```

- [x] **Step 4: Запустить новый тест и проверить его red-green**

```bash
cd /c/ResultV && TAGS=$(tr -d ' 	
' < scripts/android-build-tags.txt)
go test -tags="$TAGS" -count=1 ./internal/proxy/ -run 'TestIsRoutingDeepLinkIgnoresForeignInput' -v
```

Ожидается: PASS.

Тест, прошедший сразу, ничего не доказывает, пока не показано, что он умеет
падать. Здесь одиночной мутации мало: вход отсекают **две** независимые
проверки в `routingDeepLinkBody` — ранний выход по `IsDeepLink` и ветка
`default` в разборе схемы. Сломать надо обе сразу:

```bash
cp internal/proxy/routingprofile.go /tmp/rp.bak
# в routingDeepLinkBody: "if !IsDeepLink(rawURL) {" -> "if false {"
#                        "default:
		return \"\", false" -> "default:
		body = rawURL"
go test -tags="$TAGS" -count=1 ./internal/proxy/ -run 'TestIsRoutingDeepLinkIgnoresForeignInput'
# ждём: FAIL на "routing/onadd/eyJ9" = true
cp /tmp/rp.bak internal/proxy/routingprofile.go
go test -tags="$TAGS" -count=1 ./internal/proxy/ -run 'TestIsRoutingDeepLinkIgnoresForeignInput'
# ждём: ok
```

- [x] **Step 5: Обе конфигурации целиком**

Команды из «Global Constraints».

- [x] **Step 6: Коммит**

```bash
git add internal/proxy/routingprofile.go internal/proxy/routingprofile_test.go
git commit -m "feat(routing): перенести разбор диплинка профиля

Плюс единственный тест, которого на ПК нет: вход, не являющийся
resultv-ссылкой вовсе. Там его никто не задаёт — настольный импортёр доходит
до этого кода уже со ссылкой в руках, а поле вставки на Android отдаст сюда
что угодно.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 4: Извлечение маршрутизации из ответа подписки

Файл переносится сейчас, хотя его биндинг и захват заголовков — этап C: он
чистый, собирается вместе с соседями и тянет их хелперы, так что откладывать
его значит откладывать проверку, что они совместимы.

**Files:**
- Create: `internal/proxy/sublists.go`
- Test: `internal/proxy/sublists_test.go`

**Interfaces:**
- Consumes: `NormalizeRoutingListURL`, `ParsedRoutingList`, `compressDomainSuffixes`, `plausibleDomains`, `normalizeDomains`, `normalizeCIDRs`, `normalizeRule`, `config.RoutingList`
- Produces: `func ExtractSubscriptionRoutingLists(headerVal, body string) []config.RoutingList`, `func ExtractEmbeddedRoutingLists(body string) map[string]ParsedRoutingList`, `const MaxSubscriptionRoutingLists = 10`

- [x] **Step 1: Скопировать файл и тест без изменений**

```bash
cd /c/ResultV
cp /c/ResultVPC/internal/proxy/sublists.go      internal/proxy/sublists.go
cp /c/ResultVPC/internal/proxy/sublists_test.go internal/proxy/sublists_test.go
```

- [x] **Step 2: Запустить тесты**

```bash
cd /c/ResultV && TAGS=$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)
go test -tags="$TAGS" -count=1 ./internal/proxy/ -run 'Subscription|Embedded' -v
```

Ожидается: все PASS.

- [x] **Step 3: Обе конфигурации целиком**

Команды из «Global Constraints».

- [x] **Step 4: Коммит**

```bash
git add internal/proxy/sublists.go internal/proxy/sublists_test.go
git commit -m "feat(routing): перенести извлечение маршрутизации из подписки

Заголовок Routing-Lists, ключ routingLists в JSON-теле и встроенные
xray-правила. Биндинг и захват заголовков — этап C; здесь только разбор.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 5: Компиляция правил в бинарный SRS

Замена `WriteRoutingListRuleSet`. Образец — `internal/proxy/smart_ruleset.go`.

**Files:**
- Create: `internal/proxy/routing_ruleset.go`
- Test: `internal/proxy/routing_ruleset_test.go`

**Interfaces:**
- Consumes: `ParsedRoutingList` (задача 1), `validateSRS` (`srs_validate.go:27`)
- Produces: `func RoutingRuleSetDir(dataDir string) string`, `func RoutingProfileSRSPath(dataDir, profileID, action string) string`, `func RoutingProfileRuleSetTag(profileID, action string) string`, `func CompileRoutingSRS(p ParsedRoutingList, path string) error`, `func RoutingProfileSRSReady(dataDir, profileID, action string) bool`, `func RemoveRoutingProfileSRS(dataDir, profileID string)`, `func ValidRoutingProfileID(id string) bool`, `var RoutingActions = []string{"direct", "proxy", "block"}`

- [x] **Step 1: Написать падающий тест**

Создать `internal/proxy/routing_ruleset_test.go`:

```go
package proxy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompileRoutingSRSRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := RoutingProfileSRSPath(dir, "abc123", "proxy")
	p := ParsedRoutingList{
		Domains:      []string{"example.com", "example.org"},
		ExactDomains: []string{"only.example.net"},
		CIDRs:        []string{"10.0.0.0/8"},
	}
	if err := CompileRoutingSRS(p, path); err != nil {
		t.Fatalf("CompileRoutingSRS: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("читаем результат: %v", err)
	}
	if err := validateSRS(data); err != nil {
		t.Fatalf("ядро не принимает свой же SRS: %v", err)
	}
	if !RoutingProfileSRSReady(dir, "abc123", "proxy") {
		t.Error("RoutingProfileSRSReady = false сразу после записи")
	}
}

func TestCompileRoutingSRSRejectsEmptyAndKeepsPrevious(t *testing.T) {
	dir := t.TempDir()
	path := RoutingProfileSRSPath(dir, "abc123", "direct")
	good := ParsedRoutingList{Domains: []string{"example.com"}}
	if err := CompileRoutingSRS(good, path); err != nil {
		t.Fatalf("первая запись: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("читаем первую запись: %v", err)
	}
	if err := CompileRoutingSRS(ParsedRoutingList{}, path); err == nil {
		t.Fatal("пустой список записался, ждали ошибку")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("прежний файл исчез после неудачной записи: %v", err)
	}
	if string(before) != string(after) {
		t.Error("неудачная запись затёрла прежний SRS")
	}
}

func TestCompileRoutingSRSAcceptsExactOnlyList(t *testing.T) {
	dir := t.TempDir()
	path := RoutingProfileSRSPath(dir, "exactonly", "block")
	p := ParsedRoutingList{ExactDomains: []string{"only.example.net"}}
	if err := CompileRoutingSRS(p, path); err != nil {
		t.Fatalf("список из одних точных имён отвергнут: %v", err)
	}
	if !RoutingProfileSRSReady(dir, "exactonly", "block") {
		t.Error("RoutingProfileSRSReady = false для точного списка")
	}
}

func TestRoutingProfileRuleSetTagStable(t *testing.T) {
	if got := RoutingProfileRuleSetTag("abc123", "proxy"); got != "prof-abc123-proxy" {
		t.Errorf("тег = %q, ждали prof-abc123-proxy", got)
	}
}

func TestRoutingProfileSRSPathUnderRoutingDir(t *testing.T) {
	got := RoutingProfileSRSPath("/data", "abc123", "block")
	want := filepath.Join("/data", "routing", "prof-abc123-block.srs")
	if got != want {
		t.Errorf("путь = %q, ждали %q", got, want)
	}
}

func TestValidRoutingProfileIDRejectsSeparators(t *testing.T) {
	for _, bad := range []string{"", "..", "a/b", `a\b`, "a b", "../../etc/passwd", strings.Repeat("a", 65)} {
		if ValidRoutingProfileID(bad) {
			t.Errorf("ValidRoutingProfileID(%q) = true, ждали false", bad)
		}
	}
	for _, ok := range []string{"abc123", "0123456789abcdef"} {
		if !ValidRoutingProfileID(ok) {
			t.Errorf("ValidRoutingProfileID(%q) = false, ждали true", ok)
		}
	}
}

func TestCompileRoutingSRSRefusesBadID(t *testing.T) {
	dir := t.TempDir()
	if err := CompileRoutingSRS(
		ParsedRoutingList{Domains: []string{"example.com"}},
		RoutingProfileSRSPath(dir, "../escape", "proxy"),
	); err == nil {
		t.Fatal("id с обходом каталога принят")
	}
}

func TestRemoveRoutingProfileSRSDeletesAllThree(t *testing.T) {
	dir := t.TempDir()
	for _, action := range RoutingActions {
		if err := CompileRoutingSRS(
			ParsedRoutingList{Domains: []string{"example.com"}},
			RoutingProfileSRSPath(dir, "gone", action),
		); err != nil {
			t.Fatalf("подготовка %s: %v", action, err)
		}
	}
	RemoveRoutingProfileSRS(dir, "gone")
	for _, action := range RoutingActions {
		if RoutingProfileSRSReady(dir, "gone", action) {
			t.Errorf("SRS для %s пережил удаление", action)
		}
	}
}
```

- [x] **Step 2: Запустить — должен упасть на отсутствии функций**

```bash
cd /c/ResultV && TAGS=$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)
go test -tags="$TAGS" -count=1 ./internal/proxy/ -run 'RoutingSRS|RoutingProfileSRS|RoutingProfileRuleSetTag|ValidRoutingProfileID|RemoveRoutingProfileSRS' 2>&1 | head -20
```

Ожидается: `undefined: CompileRoutingSRS`, `undefined: RoutingProfileSRSPath` и
так далее — сборка не проходит.

- [x] **Step 3: Написать реализацию**

Создать `internal/proxy/routing_ruleset.go`:

```go
// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

// Compiled rule-sets for routing profiles.
//
// A profile's rules are stored as a compiled binary sing-box rule-set, not as
// the source-format JSON the desktop writes. The reason is the one already
// measured for the Smart list (see smart_ruleset.go): a profile that expands
// "geosite:whitelist" carries tens of thousands of domains, and the core would
// re-parse that JSON on every connect. The SRS is ~70 KB and loads in ~17 ms.
//
// The trade: an SRS stores a compiled succinct trie, so the domains cannot be
// read back out. Counts shown in the UI therefore come from the stored tokens,
// never from the compiled file.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sagernet/sing-box/common/srs"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
)

const (
	routingRuleSetsSubdir = "routing"
	// minRoutingSRSBytes guards against referencing a truncated / half-written
	// SRS. A valid but empty rule-set is ~30 bytes; anything under this is
	// certainly junk. Same rationale as minSmartSRSBytes.
	minRoutingSRSBytes = 32
	// maxRoutingProfileIDLen bounds the id used as a file name.
	maxRoutingProfileIDLen = 64
)

// RoutingActions are the three actions a profile can assign, in the order the
// compiler walks them. NOT the order rules are emitted in — that is the
// profile's RouteOrder, see NormalizeRoutingOrder.
var RoutingActions = []string{"direct", "proxy", "block"}

// RoutingRuleSetDir is where compiled profile rule-sets live.
func RoutingRuleSetDir(dataDir string) string {
	return filepath.Join(dataDir, routingRuleSetsSubdir)
}

// ValidRoutingProfileID reports whether id is safe to use as a file name.
//
// The id is generated here (newRoutingProfileID, routing_merge.go), but it
// makes the round trip through a Kotlin-side JSON file before coming back, so
// it is checked rather than trusted: it lands in a path, and a "../" in it
// would write outside the cache directory.
func ValidRoutingProfileID(id string) bool {
	if id == "" || len(id) > maxRoutingProfileIDLen {
		return false
	}
	if id == "." || id == ".." {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// RoutingProfileRuleSetTag is the sing-box rule_set tag for one action of one
// profile. It is also the cache file's base name, so the two can never drift.
func RoutingProfileRuleSetTag(profileID, action string) string {
	return "prof-" + profileID + "-" + action
}

// RoutingProfileSRSPath is the cache file for one action of one profile.
func RoutingProfileSRSPath(dataDir, profileID, action string) string {
	return filepath.Join(RoutingRuleSetDir(dataDir),
		RoutingProfileRuleSetTag(profileID, action)+".srs")
}

// CompileRoutingSRS writes the parsed rules as a binary rule-set at path.
//
// An empty list is an error and nothing is written: callers rely on a failed
// compile leaving any previous cache intact, so a profile that briefly fails to
// resolve keeps routing by what it last resolved to.
//
// The write is atomic (temp + rename). A half-written SRS referenced as a local
// rule_set fails sing-box startup outright, which would break the connection
// rather than just the profile.
func CompileRoutingSRS(p ParsedRoutingList, path string) error {
	if len(p.Domains) == 0 && len(p.ExactDomains) == 0 && len(p.CIDRs) == 0 {
		return fmt.Errorf("routing SRS: empty rule list")
	}
	base := strings.TrimSuffix(filepath.Base(path), ".srs")
	if base == "" || strings.ContainsAny(base, `/\`) || strings.Contains(base, "..") {
		return fmt.Errorf("routing SRS: unsafe cache path %q", path)
	}
	ruleSet := option.PlainRuleSet{
		Rules: []option.HeadlessRule{{
			Type: C.RuleTypeDefault,
			DefaultOptions: option.DefaultHeadlessRule{
				Domain:       p.ExactDomains,
				DomainSuffix: p.Domains,
				IPCIDR:       p.CIDRs,
			},
		}},
	}
	var buf bytes.Buffer
	if err := srs.Write(&buf, ruleSet, C.RuleSetVersion3); err != nil {
		return fmt.Errorf("routing SRS: writing: %w", err)
	}
	// Validate our own output before it reaches disk: an invalid local
	// rule_set fails the engine's start, and finding that out at connect time
	// gives no clue which profile did it.
	if err := validateSRS(buf.Bytes()); err != nil {
		return fmt.Errorf("routing SRS: self-validation: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("routing SRS: creating dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "routing-*.srs")
	if err != nil {
		return fmt.Errorf("routing SRS: creating temp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("routing SRS: writing temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("routing SRS: closing temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("routing SRS: renaming: %w", err)
	}
	return nil
}

// RoutingProfileSRSReady reports whether one action of one profile has a usable
// compiled rule-set on disk.
func RoutingProfileSRSReady(dataDir, profileID, action string) bool {
	if !ValidRoutingProfileID(profileID) {
		return false
	}
	return localRoutingSRSUsable(RoutingProfileSRSPath(dataDir, profileID, action))
}

// localRoutingSRSUsable reports whether the SRS at path can be referenced as a
// `local` rule_set. On a failed validation the file is deleted (best effort) so
// the next compile writes a clean copy instead of the engine refusing to start.
func localRoutingSRSUsable(path string) bool {
	st, err := os.Stat(path)
	if err != nil || st.Size() < minRoutingSRSBytes {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	if err := validateSRS(data); err != nil {
		_ = os.Remove(path) // self-heal
		return false
	}
	return true
}

// RemoveRoutingProfileSRS deletes a profile's cached rule-sets. Best effort: a
// file that will not go away is not worth failing a delete over, and the config
// entry is gone either way — buildRoutingProfileRules only emits rules for a
// profile the caller names.
func RemoveRoutingProfileSRS(dataDir, profileID string) {
	if !ValidRoutingProfileID(profileID) {
		return
	}
	for _, action := range RoutingActions {
		_ = os.Remove(RoutingProfileSRSPath(dataDir, profileID, action))
	}
}
```

- [x] **Step 4: Запустить тесты — должны пройти**

```bash
cd /c/ResultV && TAGS=$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)
go test -tags="$TAGS" -count=1 ./internal/proxy/ -run 'RoutingSRS|RoutingProfileSRS|RoutingProfileRuleSetTag|ValidRoutingProfileID|RemoveRoutingProfileSRS' -v
```

Ожидается: 8 тестов PASS.

- [x] **Step 5: Вернуть два теста, вырезанных в задаче 2**

Дописать в `internal/proxy/routing_ruleset_test.go` — предмет тот же, что у
`TestWriteRoutingListRuleSetKeepsExactDomainsApart` на ПК, но проверяется через
компиляцию: точное имя не должно превращаться в суффикс, иначе правило молча
расширяется на все поддомены.

```go
func TestCompileRoutingSRSKeepsExactDomainsApart(t *testing.T) {
	dir := t.TempDir()
	path := RoutingProfileSRSPath(dir, "apart", "proxy")
	p := ParsedRoutingList{
		Domains:      []string{"suffix.example"},
		ExactDomains: []string{"exact.example"},
	}
	if err := CompileRoutingSRS(p, path); err != nil {
		t.Fatalf("CompileRoutingSRS: %v", err)
	}
	m, err := LoadRoutingDomainMatcher(path)
	if err != nil {
		t.Fatalf("LoadRoutingDomainMatcher: %v", err)
	}
	if !m.Match("exact.example") {
		t.Error("точное имя не совпало само с собой")
	}
	if m.Match("sub.exact.example") {
		t.Error("точное имя поймало поддомен — правило расширилось молча")
	}
	if !m.Match("sub.suffix.example") {
		t.Error("суффикс не поймал поддомен")
	}
}
```

- [x] **Step 6: Добавить `LoadRoutingDomainMatcher`**

Тест шага 5 требует читать SRS обратно. Дописать в
`internal/proxy/routing_ruleset.go`:

```go
// LoadRoutingDomainMatcher reads a compiled rule-set back as a domain matcher.
//
// Only the compiled trie comes back — srs.Read returns DomainMatcher with the
// Domain/DomainSuffix lists empty (see LoadSmartDomainMatcher for the same
// note). So this answers "does this host match", never "what is in the list".
// Used by tests; the engine references the file by path and never reads it.
func LoadRoutingDomainMatcher(path string) (*domain.Matcher, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	compat, err := srs.Read(bytes.NewReader(data), false)
	if err != nil {
		return nil, fmt.Errorf("routing SRS: reading: %w", err)
	}
	plain, err := compat.Upgrade()
	if err != nil {
		return nil, fmt.Errorf("routing SRS: upgrading: %w", err)
	}
	if len(plain.Rules) == 0 || plain.Rules[0].DefaultOptions.DomainMatcher == nil {
		return nil, fmt.Errorf("routing SRS: no domain matcher in rule-set")
	}
	return plain.Rules[0].DefaultOptions.DomainMatcher, nil
}
```

Добавить в импорты `"github.com/sagernet/sing/common/domain"`.

- [x] **Step 7: Запустить все тесты файла**

```bash
cd /c/ResultV && TAGS=$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)
go test -tags="$TAGS" -count=1 ./internal/proxy/ -run 'RoutingSRS|RoutingProfileSRS|RoutingProfileRuleSetTag|ValidRoutingProfileID|RemoveRoutingProfileSRS|CompileRouting' -v
```

Ожидается: 9 тестов PASS.

- [x] **Step 8: Обе конфигурации целиком**

Команды из «Global Constraints».

- [x] **Step 9: Коммит**

```bash
git add internal/proxy/routing_ruleset.go internal/proxy/routing_ruleset_test.go
git commit -m "feat(routing): компилировать правила профиля в бинарный SRS

Вместо source-JSON, который пишет ПК. Замер на Smart-списке в этом же
репозитории: 150 тыс. доменов — 4.6 МБ и ~2 с на коннект против ~70 КБ и
~17 мс. Запись атомарная и с самопроверкой: невалидный local rule_set валит
старт ядра, а не только профиль.

Идентификатор профиля проверяется, а не принимается на веру: он делает круг
через JSON-файл на стороне Kotlin и попадает в путь.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 6: Идентичность профиля и лимиты

Перенос `sameRoutingProfile` / `upsertRoutingProfile` из `app_routingprofile.go`
в чистые функции. Это то место, ради которого выбран гибридный подход: правило
«входящий профиль обновляет вон тот» неочевидно, и его тесты уже написаны.

**Files:**
- Create: `internal/proxy/routing_merge.go`
- Test: `internal/proxy/routing_merge_test.go`

**Interfaces:**
- Consumes: `config.RoutingProfile`
- Produces: `func NewRoutingProfileID() string`, `func SameRoutingProfile(stored, incoming config.RoutingProfile) bool`, `func UpsertRoutingProfile(stored []config.RoutingProfile, incoming config.RoutingProfile, activeID string, makeActive bool) ([]config.RoutingProfile, string, config.RoutingProfile, error)`, `const MaxRoutingProfiles = 50`

- [x] **Step 1: Написать падающий тест**

Создать `internal/proxy/routing_merge_test.go`:

```go
package proxy

import (
	"testing"

	"resultproxy-wails/internal/config"
)

func TestUpsertAddsAndAssignsID(t *testing.T) {
	in := config.RoutingProfile{Name: "Panel A", OriginName: "Panel A", Source: "deeplink"}
	out, activeID, saved, err := UpsertRoutingProfile(nil, in, "", false)
	if err != nil {
		t.Fatalf("UpsertRoutingProfile: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("профилей %d, ждали 1", len(out))
	}
	if saved.ID == "" {
		t.Error("id не назначен")
	}
	if !ValidRoutingProfileID(saved.ID) {
		t.Errorf("назначенный id %q не проходит ValidRoutingProfileID", saved.ID)
	}
	// Первый профиль становится активным, даже когда makeActive=false:
	// иначе импорт выглядел бы как "ничего не произошло".
	if activeID != saved.ID {
		t.Errorf("activeID = %q, ждали %q", activeID, saved.ID)
	}
}

func TestUpsertReplacesByOriginNameAndKeepsUserRename(t *testing.T) {
	stored := []config.RoutingProfile{{
		ID:         "keepme",
		Name:       "Моя маршрутизация",
		OriginName: "Panel A",
		Source:     "deeplink",
		ProxySites: []string{"old.example"},
	}}
	in := config.RoutingProfile{
		Name:       "Panel A",
		OriginName: "Panel A",
		Source:     "deeplink",
		ProxySites: []string{"new.example"},
	}
	out, _, saved, err := UpsertRoutingProfile(stored, in, "keepme", false)
	if err != nil {
		t.Fatalf("UpsertRoutingProfile: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("профиль раздвоился: %d записей", len(out))
	}
	if saved.ID != "keepme" {
		t.Errorf("id сменился на %q", saved.ID)
	}
	if saved.Name != "Моя маршрутизация" {
		t.Errorf("переименование затёрто: %q", saved.Name)
	}
	if len(saved.ProxySites) != 1 || saved.ProxySites[0] != "new.example" {
		t.Errorf("правила не обновились: %v", saved.ProxySites)
	}
}

func TestUpsertDoesNotMatchAcrossSources(t *testing.T) {
	stored := []config.RoutingProfile{{
		ID: "sub1", Name: "Panel A", OriginName: "Panel A",
		Source: "subscription", SubscriptionID: "s1",
	}}
	in := config.RoutingProfile{Name: "Panel A", OriginName: "Panel A", Source: "deeplink"}
	out, _, _, err := UpsertRoutingProfile(stored, in, "sub1", false)
	if err != nil {
		t.Fatalf("UpsertRoutingProfile: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("профилей %d, ждали 2: подписочный и диплинковый — разные", len(out))
	}
}

func TestUpsertFallsBackToNameWhenNoOrigin(t *testing.T) {
	stored := []config.RoutingProfile{{ID: "old", Name: "Panel A", Source: "manual"}}
	in := config.RoutingProfile{Name: "panel a", Source: "manual"}
	out, _, saved, err := UpsertRoutingProfile(stored, in, "", false)
	if err != nil {
		t.Fatalf("UpsertRoutingProfile: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("профилей %d, ждали 1: сопоставление имён не зависит от регистра", len(out))
	}
	if saved.ID != "old" {
		t.Errorf("id сменился на %q", saved.ID)
	}
}

func TestUpsertHonoursMakeActive(t *testing.T) {
	stored := []config.RoutingProfile{{ID: "a", Name: "A", OriginName: "A", Source: "manual"}}
	in := config.RoutingProfile{Name: "B", OriginName: "B", Source: "deeplink"}
	_, activeID, saved, err := UpsertRoutingProfile(stored, in, "a", true)
	if err != nil {
		t.Fatalf("UpsertRoutingProfile: %v", err)
	}
	if activeID != saved.ID {
		t.Errorf("activeID = %q, ждали новый %q", activeID, saved.ID)
	}
}

func TestUpsertLeavesActiveAloneWhenNotAsked(t *testing.T) {
	stored := []config.RoutingProfile{{ID: "a", Name: "A", OriginName: "A", Source: "manual"}}
	in := config.RoutingProfile{Name: "B", OriginName: "B", Source: "deeplink"}
	_, activeID, _, err := UpsertRoutingProfile(stored, in, "a", false)
	if err != nil {
		t.Fatalf("UpsertRoutingProfile: %v", err)
	}
	if activeID != "a" {
		t.Errorf("activeID = %q, ждали a — фоновое обновление не меняет выбор", activeID)
	}
}

func TestUpsertRefusesPastTheCap(t *testing.T) {
	stored := make([]config.RoutingProfile, MaxRoutingProfiles)
	for i := range stored {
		stored[i] = config.RoutingProfile{
			ID:         NewRoutingProfileID(),
			Name:       string(rune('a'+i%26)) + string(rune('0'+i/26)),
			OriginName: string(rune('a'+i%26)) + string(rune('0'+i/26)),
			Source:     "manual",
		}
	}
	in := config.RoutingProfile{Name: "ещё один", OriginName: "ещё один", Source: "deeplink"}
	if _, _, _, err := UpsertRoutingProfile(stored, in, "", false); err == nil {
		t.Fatal("профиль сверх лимита принят")
	}
}

func TestUpsertDoesNotAliasCallerSlice(t *testing.T) {
	stored := []config.RoutingProfile{{
		ID: "a", Name: "A", OriginName: "A", Source: "manual",
		ProxySites: []string{"old.example"},
	}}
	in := config.RoutingProfile{
		Name: "A", OriginName: "A", Source: "manual",
		ProxySites: []string{"new.example"},
	}
	if _, _, _, err := UpsertRoutingProfile(stored, in, "a", false); err != nil {
		t.Fatalf("UpsertRoutingProfile: %v", err)
	}
	if stored[0].ProxySites[0] != "old.example" {
		t.Error("исходный срез изменён на месте")
	}
}
```

- [x] **Step 2: Запустить — должен упасть**

```bash
cd /c/ResultV && TAGS=$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)
go test -tags="$TAGS" -count=1 ./internal/proxy/ -run 'Upsert' 2>&1 | head -10
```

Ожидается: `undefined: UpsertRoutingProfile`, `undefined: NewRoutingProfileID`.

- [x] **Step 3: Написать реализацию**

Создать `internal/proxy/routing_merge.go`:

```go
// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

// Deciding whether an arriving profile is a new version of a stored one.
//
// Ported from the desktop's app_routingprofile.go, minus its *App receiver: the
// store lives on the Kotlin side here, so this layer is handed the slice and
// hands back a new one. It never touches disk and never reaches the network.
//
// Everything below builds fresh slices rather than editing in place. The caller
// marshals its own state into these, and an in-place edit would mutate what it
// still holds.

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"resultproxy-wails/internal/config"
)

// MaxRoutingProfiles caps how many profiles are kept. Deep links are
// attacker-reachable: without a cap, repeatedly opening one would grow the
// stored file without end.
const MaxRoutingProfiles = 50

// NewRoutingProfileID mints an id for a profile. Hex only, so it is always a
// safe file-name component (see ValidRoutingProfileID).
func NewRoutingProfileID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand.Read does not fail in practice, and on this platform a
		// failure kills the process before returning (see the Go >= 1.24
		// getrandom note in the parent spec). Fall back to the clock rather
		// than mint an empty id that would fail ValidRoutingProfileID.
		return fmt.Sprintf("t%015x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// SameRoutingProfile decides whether an arriving profile is a new version of a
// stored one.
//
// The handle is the PUBLISHED name plus the origin, never the displayed one: a
// panel keeps its name stable across republishes, and it is the only identity
// the payload offers — the JSON carries no id. Comparing displayed names would
// fork a profile in two the first time the user renamed it and reopened the
// link, which is exactly what a re-import is supposed to avoid.
func SameRoutingProfile(stored, incoming config.RoutingProfile) bool {
	if stored.Source != incoming.Source {
		return false
	}
	if stored.SubscriptionID != incoming.SubscriptionID {
		return false
	}
	return strings.EqualFold(
		strings.TrimSpace(routingProfileHandle(stored)),
		strings.TrimSpace(routingProfileHandle(incoming)))
}

// routingProfileHandle falls back to the displayed name for profiles stored
// before OriginName existed, and for hand-made ones that never had a publisher.
func routingProfileHandle(p config.RoutingProfile) string {
	if p.OriginName != "" {
		return p.OriginName
	}
	return p.Name
}

// UpsertRoutingProfile stores a profile, replacing the one it matches.
//
// Returns the new slice, the new active id, and the profile as stored (with its
// id filled in). activeID is the caller's current choice; makeActive asks for
// the incoming profile to take over. A first profile always becomes active —
// importing one and having nothing happen reads as a failure.
func UpsertRoutingProfile(
	stored []config.RoutingProfile,
	incoming config.RoutingProfile,
	activeID string,
	makeActive bool,
) ([]config.RoutingProfile, string, config.RoutingProfile, error) {
	out := make([]config.RoutingProfile, len(stored))
	copy(out, stored)

	replaced := false
	for i, existing := range out {
		if !SameRoutingProfile(existing, incoming) {
			continue
		}
		// The user may have renamed it; a republish of the same profile must
		// not undo that.
		incoming.ID = existing.ID
		if existing.Name != "" {
			incoming.Name = existing.Name
		}
		out[i] = incoming
		replaced = true
		break
	}
	if !replaced {
		if len(out) >= MaxRoutingProfiles {
			return nil, activeID, config.RoutingProfile{}, fmt.Errorf(
				"хранится уже %d профилей маршрутизации — удалите лишние", len(out))
		}
		if incoming.ID == "" || !ValidRoutingProfileID(incoming.ID) {
			incoming.ID = NewRoutingProfileID()
		}
		out = append(out, incoming)
	}
	if makeActive || activeID == "" {
		activeID = incoming.ID
	}
	return out, activeID, incoming, nil
}
```

- [x] **Step 4: Запустить тесты — должны пройти**

```bash
cd /c/ResultV && TAGS=$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)
go test -tags="$TAGS" -count=1 ./internal/proxy/ -run 'Upsert|SameRoutingProfile' -v
```

Ожидается: 8 тестов PASS.

- [x] **Step 5: Обе конфигурации целиком**

Команды из «Global Constraints».

- [x] **Step 6: Коммит**

```bash
git add internal/proxy/routing_merge.go internal/proxy/routing_merge_test.go
git commit -m "feat(routing): идентичность профиля и лимит их числа

Перенос sameRoutingProfile и upsertRoutingProfile с ПК в чистые функции:
хранилище на Android держит Kotlin, а правило сопоставления остаётся здесь,
одним экземпляром с перенесёнными тестами.

Сопоставление идёт по OriginName, а не по отображаемому имени: иначе профиль
раздваивался бы ровно тогда, когда его переименовали и снова открыли ссылку.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 7: Загрузка списков и geo-баз

**Files:**
- Create: `internal/proxy/routing_fetch.go`
- Test: `internal/proxy/routing_fetch_test.go`

**Interfaces:**
- Consumes: `NormalizeRoutingListURL` (задача 1)
- Produces: `func FetchRoutingPayload(ctx context.Context, rawURL string, allowInsecure bool) ([]byte, error)`, `const routingFetchMaxBytes = 8 << 20`

- [x] **Step 1: Написать падающий тест**

Создать `internal/proxy/routing_fetch_test.go`:

```go
package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchRoutingPayloadRefusesPrivateTargets(t *testing.T) {
	// httptest слушает на 127.0.0.1 — ровно тот класс адресов, который
	// защита обязана отсечь. Диплинк приходит от атакующего и несёт URL
	// geo-баз, поэтому запрос не должен дойти до сокета.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("запрос дошёл до приватного адреса")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := FetchRoutingPayload(context.Background(), srv.URL, true)
	if err == nil {
		t.Fatal("загрузка с loopback прошла, ждали отказ")
	}
}

func TestFetchRoutingPayloadRefusesPlaintextWithoutConsent(t *testing.T) {
	_, err := FetchRoutingPayload(context.Background(), "http://example.com/list.txt", false)
	if err == nil {
		t.Fatal("http:// без согласия принят")
	}
	if !strings.Contains(err.Error(), "insecure") {
		t.Errorf("причина отказа не про plaintext: %v", err)
	}
}

func TestFetchRoutingPayloadRefusesOtherSchemes(t *testing.T) {
	for _, u := range []string{
		"file:///etc/passwd",
		"javascript:alert(1)",
		"ftp://example.com/list.txt",
		"",
	} {
		if _, err := FetchRoutingPayload(context.Background(), u, true); err == nil {
			t.Errorf("схема %q принята", u)
		}
	}
}

func TestFetchRoutingPayloadRewritesGitHubBlobURL(t *testing.T) {
	// Проверяется только переписывание: сеть тут не нужна, а ошибка вернётся
	// в любом случае. Достаточно, что в ней стоит raw-хост, а не github.com.
	_, err := FetchRoutingPayload(context.Background(),
		"https://github.com/o/r/blob/main/list.txt", false)
	if err == nil {
		t.Skip("сеть доступна — проверка переписывания требует оффлайна")
	}
	if strings.Contains(err.Error(), "github.com/o/r/blob") {
		t.Errorf("blob-URL не переписан в raw: %v", err)
	}
}
```

- [x] **Step 2: Запустить — должен упасть**

```bash
cd /c/ResultV && TAGS=$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)
go test -tags="$TAGS" -count=1 ./internal/proxy/ -run 'FetchRoutingPayload' 2>&1 | head -10
```

Ожидается: `undefined: FetchRoutingPayload`.

- [x] **Step 3: Написать реализацию**

Создать `internal/proxy/routing_fetch.go`:

```go
// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

// Fetching what a routing profile points at: rule lists and geo databases.
//
// Every URL here arrives from outside — a deep link anyone can send, or a
// provider's subscription. So the fetch is bounded three ways: only http(s),
// plaintext only with explicit consent, and never to a private or loopback
// address. The last check sits on the dialer's Control hook, AFTER name
// resolution: a hostname that resolves to 127.0.0.1 passes a string check and
// would otherwise reach the device's own services.

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"
)

const (
	// routingFetchMaxBytes bounds a response body. Real geo databases are
	// ~0.5 MiB and rule lists smaller; 8 MiB is roomy and still finite.
	routingFetchMaxBytes = 8 << 20
	// A list host (often a CDN edge) occasionally stalls before sending
	// headers. Retry a few times with a short per-attempt timeout so a one-off
	// stall recovers in a second or two instead of failing the whole compile.
	routingFetchAttempts       = 3
	routingFetchAttemptTimeout = 15 * time.Second
)

// safeRoutingDialer refuses to connect to private, loopback, link-local or
// unspecified addresses. The check runs in Control, which fires after the
// address is resolved and before the socket connects.
func safeRoutingDialer() *net.Dialer {
	return &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("routing fetch: bad address %q", address)
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("routing fetch: cannot parse %q", host)
			}
			if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
				ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
				return fmt.Errorf("routing fetch: blocked private/loopback target %s", ip)
			}
			return nil
		},
	}
}

// FetchRoutingPayload downloads one rule list or geo database.
//
// allowInsecure carries the consent the user already gave for a provider down
// to its links; it never grants itself one.
func FetchRoutingPayload(ctx context.Context, rawURL string, allowInsecure bool) ([]byte, error) {
	// Rewrite a GitHub blob (web page) URL to its raw form so an ordinary
	// github.com link fetches the file, not the HTML page. Idempotent.
	u := NormalizeRoutingListURL(strings.TrimSpace(rawURL))
	if u == "" {
		return nil, fmt.Errorf("routing fetch: empty URL")
	}
	lower := strings.ToLower(u)
	isHTTP := strings.HasPrefix(lower, "http://")
	isHTTPS := strings.HasPrefix(lower, "https://")
	if isHTTP && !allowInsecure {
		return nil, fmt.Errorf("routing fetch: insecure http:// requires explicit consent")
	}
	if !isHTTP && !isHTTPS {
		return nil, fmt.Errorf("routing fetch: unsupported URL scheme")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	client := &http.Client{
		Timeout:   routingFetchAttemptTimeout,
		Transport: &http.Transport{DialContext: safeRoutingDialer().DialContext},
	}
	var lastErr error
	for attempt := 1; attempt <= routingFetchAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		body, status, err := tryFetchRoutingPayload(ctx, client, u)
		if err == nil && status == http.StatusOK {
			return body, nil
		}
		if err != nil {
			lastErr = err
			continue
		}
		lastErr = fmt.Errorf("routing fetch: http %d", status)
		// A definitive client error (not found / auth / gone) will not fix
		// itself on retry. Retry only on 5xx and 429.
		if status >= 400 && status < 500 && status != http.StatusTooManyRequests {
			break
		}
	}
	return nil, lastErr
}

func tryFetchRoutingPayload(ctx context.Context, client *http.Client, u string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, routingFetchMaxBytes))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}
```

- [x] **Step 4: Запустить тесты — должны пройти**

```bash
cd /c/ResultV && TAGS=$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)
go test -tags="$TAGS" -count=1 ./internal/proxy/ -run 'FetchRoutingPayload' -v
```

Ожидается: 4 PASS (последний может быть SKIP при наличии сети — это
предусмотрено тестом).

- [x] **Step 5: Обе конфигурации целиком**

Команды из «Global Constraints».

- [x] **Step 6: Коммит**

```bash
git add internal/proxy/routing_fetch.go internal/proxy/routing_fetch_test.go
git commit -m "feat(routing): загрузка списков и geo-баз с отсечкой приватных адресов

Проверка стоит на Dialer.Control, после резолва: строковая проверка
isPrivateOrLoopbackHost пропустила бы имя, резолвящееся в 127.0.0.1, а URL
здесь приходит из диплинка, доступного атакующему.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 8: Сборка профиля

**Files:**
- Create: `internal/proxy/routing_compile.go`
- Test: `internal/proxy/routing_compile_test.go`

**Interfaces:**
- Consumes: `FetchRoutingPayload` (7), `ParseRoutingListPayload`, `LooksLikeRoutingListHTML` (1), `ParseGeoSiteDat`, `ParseGeoIPDat`, `ResolveGeoTokens`, `GeoDatabases` (2), `RoutingProfileTokens` (3), `CompileRoutingSRS`, `RoutingProfileSRSPath`, `RemoveRoutingProfileSRS`, `RoutingActions`, `ValidRoutingProfileID` (5), `config.RoutingProfile`
- Produces: `type RoutingCompileReport struct { Counts map[string]int; Unresolved map[string]string }`, `func CompileRoutingProfile(ctx context.Context, p config.RoutingProfile, dataDir string, refreshGeo bool) (RoutingCompileReport, error)`, `func GeoCachePath(dataDir, kind, url string) string`, `func ProfileNeedsGeo(p config.RoutingProfile) (site, ip bool)`

- [x] **Step 1: Написать падающий тест**

Создать `internal/proxy/routing_compile_test.go`:

```go
package proxy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"resultproxy-wails/internal/config"
)

func TestProfileNeedsGeoOnlyWhenReferenced(t *testing.T) {
	plain := config.RoutingProfile{ProxySites: []string{"example.com"}}
	if site, ip := ProfileNeedsGeo(plain); site || ip {
		t.Error("профиль из обычных доменов просит geo-базы")
	}
	withSite := config.RoutingProfile{
		ProxySites: []string{"geosite:whitelist"},
		GeoSiteURL: "https://panel.example/geosite.dat",
	}
	if site, ip := ProfileNeedsGeo(withSite); !site || ip {
		t.Errorf("geosite=%v geoip=%v, ждали true/false", site, ip)
	}
	// Ссылки нет — качать нечего, значит и просить нечего.
	noURL := config.RoutingProfile{ProxySites: []string{"geosite:whitelist"}}
	if site, _ := ProfileNeedsGeo(noURL); site {
		t.Error("geosite запрошен без ссылки на базу")
	}
}

func TestGeoCachePathKeyedByURLNotProfile(t *testing.T) {
	a := GeoCachePath("/data", "geosite", "https://a.example/geosite.dat")
	b := GeoCachePath("/data", "geosite", "https://b.example/geosite.dat")
	if a == b {
		t.Error("разные ссылки дали один путь кэша")
	}
	again := GeoCachePath("/data", "geosite", " https://a.example/geosite.dat ")
	if a != again {
		t.Error("та же ссылка с пробелами дала другой путь")
	}
	if dir := filepath.Dir(a); filepath.Base(dir) != "geo" {
		t.Errorf("кэш geo лежит не в routing/geo: %s", a)
	}
}

func TestCompileRoutingProfilePlainTokensNoNetwork(t *testing.T) {
	dir := t.TempDir()
	p := config.RoutingProfile{
		ID:          "plainprof",
		Name:        "Plain",
		DirectSites: []string{"direct.example"},
		ProxySites:  []string{"domain:proxy.example"},
		BlockIPs:    []string{"10.0.0.0/8"},
	}
	rep, err := CompileRoutingProfile(context.Background(), p, dir, false)
	if err != nil {
		t.Fatalf("CompileRoutingProfile: %v", err)
	}
	for _, action := range RoutingActions {
		if rep.Counts[action] == 0 {
			t.Errorf("действие %s собралось в ноль", action)
		}
		if !RoutingProfileSRSReady(dir, "plainprof", action) {
			t.Errorf("SRS для %s не готов", action)
		}
	}
	if len(rep.Unresolved) != 0 {
		t.Errorf("непринятые токены на чистом профиле: %v", rep.Unresolved)
	}
}

func TestCompileRoutingProfileReportsUnexpressibleTokens(t *testing.T) {
	dir := t.TempDir()
	p := config.RoutingProfile{
		ID:         "reportprof",
		Name:       "Report",
		ProxySites: []string{"ok.example", "regexp:.*\\.example", "keyword:ads"},
	}
	rep, err := CompileRoutingProfile(context.Background(), p, dir, false)
	if err != nil {
		t.Fatalf("CompileRoutingProfile: %v", err)
	}
	if len(rep.Unresolved) != 2 {
		t.Errorf("непринятых токенов %d, ждали 2: %v", len(rep.Unresolved), rep.Unresolved)
	}
	if rep.Counts["proxy"] == 0 {
		t.Error("выразимая часть профиля потеряна вместе с невыразимой")
	}
}

func TestCompileRoutingProfileFailsWhenNothingCompiles(t *testing.T) {
	dir := t.TempDir()
	p := config.RoutingProfile{
		ID:         "emptyprof",
		Name:       "Empty",
		ProxySites: []string{"geosite:whitelist"}, // базы нет — развернуть нечем
	}
	if _, err := CompileRoutingProfile(context.Background(), p, dir, false); err == nil {
		t.Fatal("профиль, из которого не собралось ни одного правила, принят")
	}
}

func TestCompileRoutingProfileClearsStaleAction(t *testing.T) {
	dir := t.TempDir()
	first := config.RoutingProfile{
		ID: "staleprof", Name: "Stale",
		ProxySites:  []string{"proxy.example"},
		DirectSites: []string{"direct.example"},
	}
	if _, err := CompileRoutingProfile(context.Background(), first, dir, false); err != nil {
		t.Fatalf("первая сборка: %v", err)
	}
	if !RoutingProfileSRSReady(dir, "staleprof", "direct") {
		t.Fatal("подготовка не удалась")
	}
	second := first
	second.DirectSites = nil
	if _, err := CompileRoutingProfile(context.Background(), second, dir, false); err != nil {
		t.Fatalf("вторая сборка: %v", err)
	}
	if RoutingProfileSRSReady(dir, "staleprof", "direct") {
		t.Error("SRS выброшенного действия пережил пересборку — маршрут остался бы жить")
	}
}

func TestCompileRoutingProfileRefusesBadID(t *testing.T) {
	dir := t.TempDir()
	p := config.RoutingProfile{ID: "../escape", ProxySites: []string{"a.example"}}
	if _, err := CompileRoutingProfile(context.Background(), p, dir, false); err == nil {
		t.Fatal("профиль с обходом каталога в id собран")
	}
}

func TestValidateGeoBlobRejectsNonDatabase(t *testing.T) {
	if err := validateGeoBlob("geosite", []byte("<html>404</html>")); err == nil {
		t.Fatal("страница ошибки принята за базу geosite")
	}
	if err := validateGeoBlob("geoip", []byte("nope")); err == nil {
		t.Fatal("мусор принят за базу geoip")
	}
}

func TestGeoCacheNotWrittenForGarbage(t *testing.T) {
	dir := t.TempDir()
	path := GeoCachePath(dir, "geosite", "https://panel.example/geosite.dat")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("кэш существует до загрузки: %v", err)
	}
	// Ссылка недостижима (приватные адреса отсечены, домена нет), значит
	// ensureGeoFile обязан вернуть ошибку и ничего не записать.
	p := config.RoutingProfile{
		ID: "geoprof", ProxySites: []string{"geosite:whitelist"},
		GeoSiteURL: "https://panel.invalid./geosite.dat",
	}
	if _, err := CompileRoutingProfile(context.Background(), p, dir, false); err == nil {
		t.Fatal("сборка прошла без доступной базы")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("неудачная загрузка оставила файл в кэше")
	}
	if !strings.Contains(RoutingRuleSetDir(dir), "routing") {
		t.Error("каталог кэша не routing/")
	}
}
```

- [x] **Step 2: Запустить — должен упасть**

```bash
cd /c/ResultV && TAGS=$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)
go test -tags="$TAGS" -count=1 ./internal/proxy/ -run 'CompileRoutingProfile|ProfileNeedsGeo|GeoCache|validateGeoBlob' 2>&1 | head -10
```

Ожидается: `undefined: CompileRoutingProfile` и соседи.

- [x] **Step 3: Написать реализацию**

Создать `internal/proxy/routing_compile.go`:

```go
// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

// Turning a stored routing profile into rules the engine can run.
//
// A profile lists its rules the way its author wrote them — mostly references
// into two geo databases. So applying one means: fetch those databases, expand
// the references, and compile the result into the binary rule-sets the router
// consumes (see routing_ruleset.go).
//
// Compilation happens when the profile changes — imported, edited, activated —
// and never at connect time. Connecting only stats the cached files.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"resultproxy-wails/internal/config"
)

const (
	geoSubdir   = "geo"
	geoKindSite = "geosite"
	geoKindIP   = "geoip"
)

// RoutingCompileReport is what a compile has to say for itself: how many rules
// each action ended up with, and every token it could not express.
//
// Unresolved is a map rather than a count because a profile that imports half
// its rules must be able to say WHICH half and why — otherwise the user sees
// traffic take the wrong route with no explanation anywhere.
type RoutingCompileReport struct {
	Counts     map[string]int    `json:"counts"`
	Unresolved map[string]string `json:"unresolved"`
}

// geoDataDir sits beside the rule-set cache.
func geoDataDir(dataDir string) string {
	return filepath.Join(RoutingRuleSetDir(dataDir), geoSubdir)
}

// GeoCachePath keys the cache by the URL, not by the profile: two profiles
// pointing at the same database share one file, and a profile that changes its
// URL fetches afresh instead of silently reusing the old contents.
func GeoCachePath(dataDir, kind, url string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(url)))
	return filepath.Join(geoDataDir(dataDir), kind+"-"+hex.EncodeToString(sum[:8])+".dat")
}

// ProfileNeedsGeo reports which databases the profile actually references, so a
// profile of plain domains never downloads half a megabyte it will not read.
func ProfileNeedsGeo(p config.RoutingProfile) (site, ip bool) {
	for _, action := range RoutingActions {
		for _, token := range RoutingProfileTokens(p, action) {
			low := strings.ToLower(strings.TrimSpace(token))
			if strings.HasPrefix(low, "geosite:") {
				site = true
			}
			if strings.HasPrefix(low, "geoip:") {
				ip = true
			}
		}
	}
	return site && p.GeoSiteURL != "", ip && p.GeoIPURL != ""
}

func validateGeoBlob(kind string, blob []byte) error {
	var err error
	switch kind {
	case geoKindSite:
		_, _, err = ParseGeoSiteDat(blob)
	case geoKindIP:
		_, _, err = ParseGeoIPDat(blob)
	default:
		return fmt.Errorf("unknown geo database kind %q", kind)
	}
	if err != nil {
		return fmt.Errorf("по ссылке не база %s: %w", kind, err)
	}
	return nil
}

// ensureGeoFile returns the cached bytes, fetching them first if the cache is
// cold. force re-fetches even when a cache exists.
func ensureGeoFile(ctx context.Context, dataDir, kind, url string, allowInsecure, force bool) ([]byte, error) {
	if url == "" {
		return nil, nil
	}
	path := GeoCachePath(dataDir, kind, url)
	if !force {
		if blob, err := os.ReadFile(path); err == nil && len(blob) > 0 {
			return blob, nil
		}
	}
	blob, err := FetchRoutingPayload(ctx, url, allowInsecure)
	if err != nil {
		return nil, fmt.Errorf("не удалось скачать %s: %w", kind, err)
	}
	// Refuse to cache something that is not a geo database. Without this, a
	// panel's error page would be stored and every later compile would fail
	// against it while looking like a cache hit.
	if err := validateGeoBlob(kind, blob); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(geoDataDir(dataDir), 0o700); err != nil {
		return nil, err
	}
	if err := writeGeoFileAtomic(path, blob); err != nil {
		return nil, err
	}
	return blob, nil
}

// writeGeoFileAtomic writes through a temp file and a rename, so a failed or
// interrupted write never leaves a half-file behind for the next read to trust.
func writeGeoFileAtomic(path string, blob []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(blob); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// loadGeoDatabases fetches and parses whatever databases the profile references.
// A profile with no geo tokens needs neither file and gets empty databases —
// its plain domains and CIDRs still resolve.
func loadGeoDatabases(ctx context.Context, p config.RoutingProfile, dataDir string, force bool) (GeoDatabases, error) {
	var db GeoDatabases
	needSite, needIP := ProfileNeedsGeo(p)

	if needSite {
		blob, err := ensureGeoFile(ctx, dataDir, geoKindSite, p.GeoSiteURL, p.AllowInsecure, force)
		if err != nil {
			return db, err
		}
		if len(blob) > 0 {
			sites, dropped, perr := ParseGeoSiteDat(blob)
			if perr != nil {
				return db, perr
			}
			db.Sites = sites
			db.SiteDropped = dropped
		}
	}
	if needIP {
		blob, err := ensureGeoFile(ctx, dataDir, geoKindIP, p.GeoIPURL, p.AllowInsecure, force)
		if err != nil {
			return db, err
		}
		if len(blob) > 0 {
			ips, inverted, perr := ParseGeoIPDat(blob)
			if perr != nil {
				return db, perr
			}
			db.IPs = ips
			db.InvertedIPs = make(map[string]struct{}, len(inverted))
			for _, n := range inverted {
				db.InvertedIPs[n] = struct{}{}
			}
		}
	}
	return db, nil
}

// fetchProfileList downloads one linked rule list and parses it. Same guard and
// bounds as everything else here — a profile's link is no more trusted for
// coming from a provider.
func fetchProfileList(ctx context.Context, listURL string, allowInsecure bool) (ParsedRoutingList, error) {
	body, err := FetchRoutingPayload(ctx, listURL, allowInsecure)
	if err != nil {
		return ParsedRoutingList{}, err
	}
	if LooksLikeRoutingListHTML(body) {
		return ParsedRoutingList{}, fmt.Errorf(
			"ссылка вернула веб-страницу, а не список — для GitHub нужна ссылка Raw")
	}
	parsed := ParseRoutingListPayload(body)
	if len(parsed.Domains) == 0 && len(parsed.CIDRs) == 0 && len(parsed.ExactDomains) == 0 {
		return ParsedRoutingList{}, fmt.Errorf("в списке не найдено доменов или подсетей")
	}
	return parsed, nil
}

// CompileRoutingProfile expands a profile's rules into cached rule-sets and
// reports what it could not express.
func CompileRoutingProfile(
	ctx context.Context,
	p config.RoutingProfile,
	dataDir string,
	refreshGeo bool,
) (RoutingCompileReport, error) {
	rep := RoutingCompileReport{
		Counts:     map[string]int{},
		Unresolved: map[string]string{},
	}
	if !ValidRoutingProfileID(p.ID) {
		return rep, fmt.Errorf("routing profile: invalid id %q", p.ID)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	db, err := loadGeoDatabases(ctx, p, dataDir, refreshGeo)
	if err != nil {
		return rep, err
	}

	wrote := 0
	for _, action := range RoutingActions {
		path := RoutingProfileSRSPath(dataDir, p.ID, action)
		tokens := RoutingProfileTokens(p, action)
		if len(tokens) == 0 && len(p.ListURLs[action]) == 0 {
			// An action the profile does not use must leave no stale file from
			// a previous version of it behind, or it would keep routing.
			_ = os.Remove(path)
			rep.Counts[action] = 0
			continue
		}
		parsed, report := ResolveGeoTokens(tokens, db)
		for token, reason := range report.Unresolved {
			rep.Unresolved[token] = reason
		}
		// Rules the provider only linked to are fetched now and merged in. A
		// failed fetch is reported, not fatal: the rest of the profile still
		// routes, and saying nothing would leave traffic quietly unrouted.
		for _, listURL := range p.ListURLs[action] {
			extra, ferr := fetchProfileList(ctx, listURL, p.AllowInsecure)
			if ferr != nil {
				rep.Unresolved[listURL] = ferr.Error()
				continue
			}
			parsed.Domains = append(parsed.Domains, extra.Domains...)
			parsed.ExactDomains = append(parsed.ExactDomains, extra.ExactDomains...)
			parsed.CIDRs = append(parsed.CIDRs, extra.CIDRs...)
		}
		total := len(parsed.Domains) + len(parsed.ExactDomains) + len(parsed.CIDRs)
		rep.Counts[action] = total
		if total == 0 {
			_ = os.Remove(path)
			continue
		}
		if err := CompileRoutingSRS(parsed, path); err != nil {
			return rep, err
		}
		wrote++
	}

	if wrote == 0 {
		return rep, fmt.Errorf(
			"ни одно правило профиля не удалось применить — проверьте ссылки на geo-базы")
	}
	return rep, nil
}
```

- [x] **Step 4: Запустить тесты — должны пройти**

```bash
cd /c/ResultV && TAGS=$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)
go test -tags="$TAGS" -count=1 ./internal/proxy/ -run 'CompileRoutingProfile|ProfileNeedsGeo|GeoCache|validateGeoBlob' -v
```

Ожидается: 9 тестов PASS.

- [x] **Step 5: Обе конфигурации целиком**

Команды из «Global Constraints».

- [x] **Step 6: Коммит**

```bash
git add internal/proxy/routing_compile.go internal/proxy/routing_compile_test.go
git commit -m "feat(routing): сборка профиля — geo-кэш, резолв токенов, три SRS

Кэш geo-баз ключуется хэшем ссылки, а не профилем: две ссылки на одну базу
делят файл, сменившаяся качает заново. Страница ошибки вместо базы не
кэшируется — иначе каждая следующая сборка падала бы, выглядя как попадание
в кэш.

Действие, которое профиль перестал использовать, теряет свой SRS: оставленный
файл продолжал бы маршрутизировать.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 9: Биндинги gomobile

**Files:**
- Create: `mobile/libbox_routing.go`
- Test: `mobile/libbox_routing_test.go`

**Interfaces:**
- Consumes: всё из задач 1-8
- Produces: `func IsRoutingDeepLink(rawURL string) bool`, `func PreviewRoutingDeepLink(rawURL string) (string, error)`, `func MergeRoutingProfile(storedJSON, incomingJSON string, makeActive bool) (string, error)`, `func CompileRoutingProfile(profileJSON, dataDir string, refreshGeo bool) (string, error)`, `func RoutingProfileStatus(dataDir, profileID string) (string, error)`, `func RemoveRoutingProfile(dataDir, profileID string) error`, `func ExtractSubscriptionRouting(headerVal, body string) (string, error)`

- [x] **Step 1: Написать падающий тест**

Создать `mobile/libbox_routing_test.go`:

```go
package mobile

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

const routingPayload = `{
  "Name": "Panel Routing",
  "RouteOrder": "block-proxy-direct",
  "DirectSites": ["direct.example"],
  "ProxySites": ["proxy.example"],
  "BlockSites": ["block.example"],
  "LastUpdated": "1788322632"
}`

func routingLink() string {
	return "resultv://routing/onadd/" +
		base64.RawURLEncoding.EncodeToString([]byte(routingPayload))
}

func TestIsRoutingDeepLinkSplitsKinds(t *testing.T) {
	if !IsRoutingDeepLink(routingLink()) {
		t.Error("routing-ссылка не опознана")
	}
	if IsRoutingDeepLink("resultv://import/AAAA") {
		t.Error("ссылка подписки опознана как routing")
	}
}

func TestPreviewRoutingDeepLinkReturnsProfileWithoutID(t *testing.T) {
	out, err := PreviewRoutingDeepLink(routingLink())
	if err != nil {
		t.Fatalf("PreviewRoutingDeepLink: %v", err)
	}
	var p map[string]any
	if err := json.Unmarshal([]byte(out), &p); err != nil {
		t.Fatalf("результат не JSON: %v", err)
	}
	if p["name"] != "Panel Routing" {
		t.Errorf("name = %v", p["name"])
	}
	if p["originName"] != "Panel Routing" {
		t.Errorf("originName = %v", p["originName"])
	}
	if id, ok := p["id"]; ok && id != "" {
		t.Errorf("превью назначило id %v — это задача merge", id)
	}
	if p["routeOrder"] != "block-proxy-direct" {
		t.Errorf("routeOrder = %v", p["routeOrder"])
	}
}

func TestPreviewRoutingDeepLinkRejectsJunk(t *testing.T) {
	for _, bad := range []string{
		"resultv://routing/onadd/!!!!",
		"resultv://routing/onadd/" + base64.RawURLEncoding.EncodeToString([]byte(`{"Name":"x"}`)),
		"resultv://import/AAAA",
		"",
	} {
		if _, err := PreviewRoutingDeepLink(bad); err == nil {
			t.Errorf("принята негодная ссылка %q", bad)
		}
	}
}

func TestMergeRoutingProfileAssignsIDAndActivatesFirst(t *testing.T) {
	incoming, err := PreviewRoutingDeepLink(routingLink())
	if err != nil {
		t.Fatalf("подготовка: %v", err)
	}
	out, err := MergeRoutingProfile(`{"profiles":[],"activeId":""}`, incoming, false)
	if err != nil {
		t.Fatalf("MergeRoutingProfile: %v", err)
	}
	var res struct {
		Profiles []map[string]any `json:"profiles"`
		ActiveID string           `json:"activeId"`
		Saved    map[string]any   `json:"saved"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("результат не JSON: %v", err)
	}
	if len(res.Profiles) != 1 {
		t.Fatalf("профилей %d, ждали 1", len(res.Profiles))
	}
	id, _ := res.Saved["id"].(string)
	if id == "" {
		t.Fatal("id не назначен")
	}
	if res.ActiveID != id {
		t.Errorf("activeId = %q, ждали %q", res.ActiveID, id)
	}
}

func TestMergeRoutingProfileTolerantToEmptyStore(t *testing.T) {
	incoming, err := PreviewRoutingDeepLink(routingLink())
	if err != nil {
		t.Fatalf("подготовка: %v", err)
	}
	for _, store := range []string{"", "   ", "{}"} {
		if _, err := MergeRoutingProfile(store, incoming, false); err != nil {
			t.Errorf("пустое хранилище %q отвергнуто: %v", store, err)
		}
	}
}

func TestCompileRoutingProfileAndStatus(t *testing.T) {
	dir := t.TempDir()
	profile := `{"id":"bindprof","name":"B","proxySites":["proxy.example"],"directSites":["direct.example"]}`
	out, err := CompileRoutingProfile(profile, dir, false)
	if err != nil {
		t.Fatalf("CompileRoutingProfile: %v", err)
	}
	var rep struct {
		Counts     map[string]int    `json:"counts"`
		Unresolved map[string]string `json:"unresolved"`
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("отчёт не JSON: %v", err)
	}
	if rep.Counts["proxy"] == 0 || rep.Counts["direct"] == 0 {
		t.Errorf("счётчики пусты: %v", rep.Counts)
	}
	statusJSON, err := RoutingProfileStatus(dir, "bindprof")
	if err != nil {
		t.Fatalf("RoutingProfileStatus: %v", err)
	}
	var st map[string]bool
	if err := json.Unmarshal([]byte(statusJSON), &st); err != nil {
		t.Fatalf("статус не JSON: %v", err)
	}
	if !st["proxy"] || !st["direct"] {
		t.Errorf("статус = %v, ждали proxy и direct готовыми", st)
	}
	if st["block"] {
		t.Error("block отмечен готовым, хотя правил для него не было")
	}
}

func TestRemoveRoutingProfileDropsCache(t *testing.T) {
	dir := t.TempDir()
	profile := `{"id":"goneprof","name":"G","proxySites":["proxy.example"]}`
	if _, err := CompileRoutingProfile(profile, dir, false); err != nil {
		t.Fatalf("подготовка: %v", err)
	}
	if err := RemoveRoutingProfile(dir, "goneprof"); err != nil {
		t.Fatalf("RemoveRoutingProfile: %v", err)
	}
	statusJSON, err := RoutingProfileStatus(dir, "goneprof")
	if err != nil {
		t.Fatalf("RoutingProfileStatus: %v", err)
	}
	if strings.Contains(statusJSON, "true") {
		t.Errorf("после удаления статус = %s", statusJSON)
	}
	if entries, err := os.ReadDir(dir + "/routing"); err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "prof-goneprof-") {
				t.Errorf("файл пережил удаление: %s", e.Name())
			}
		}
	}
}

func TestExtractSubscriptionRoutingFromJSONBody(t *testing.T) {
	body := `{"routingLists":[{"name":"L","url":"https://panel.example/l.txt","action":"proxy"}]}`
	out, err := ExtractSubscriptionRouting("", body)
	if err != nil {
		t.Fatalf("ExtractSubscriptionRouting: %v", err)
	}
	if !strings.Contains(out, "panel.example/l.txt") {
		t.Errorf("список не извлечён: %s", out)
	}
}
```

- [x] **Step 2: Запустить — должен упасть**

```bash
cd /c/ResultV && TAGS=$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)
go test -tags="$TAGS" -count=1 ./mobile/ -run 'Routing' 2>&1 | head -10
```

Ожидается: `undefined: PreviewRoutingDeepLink` и соседи.

- [x] **Step 3: Написать реализацию**

Создать `mobile/libbox_routing.go`:

```go
// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package mobile

// gomobile bindings for routing profiles.
//
// Everything crosses as JSON strings: gomobile binds only basic types, so a
// []string or a struct cannot make the trip. The shape on the wire is exactly
// what config.RoutingProfile marshals to — renaming a field here would lose it
// silently on the round trip through the Kotlin store.
//
// This layer holds no state. The profile list lives in Kotlin
// (routing_profiles.json), matching every other repository on this platform;
// what stays here is the part that must not be written twice — the rule that
// decides whether an arriving profile replaces a stored one.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/proxy"
)

// IsRoutingDeepLink reports whether a resultv:// link carries a routing
// profile rather than a subscription. The split is decided here rather than by
// a prefix check in Kotlin: there are three accepted prefixes, and a Kotlin
// copy of that list would drift from the parser.
func IsRoutingDeepLink(rawURL string) bool {
	return proxy.IsRoutingDeepLink(rawURL)
}

// PreviewRoutingDeepLink decodes a routing link WITHOUT storing anything and
// without reaching the network, so the UI can show what it is about to add and
// let the user refuse. The returned profile has no id — that is assigned by
// MergeRoutingProfile, which is the only place that knows what is already
// stored.
func PreviewRoutingDeepLink(rawURL string) (string, error) {
	p, err := proxy.DecodeRoutingDeepLink(rawURL)
	if err != nil {
		return "", err
	}
	blob, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("marshaling routing profile: %w", err)
	}
	return string(blob), nil
}

// routingStore is the JSON shape of the Kotlin-side profile store.
type routingStore struct {
	Profiles []config.RoutingProfile `json:"profiles"`
	ActiveID string                  `json:"activeId"`
}

// routingMergeResult is what MergeRoutingProfile hands back: the whole new
// store plus the profile as stored, so Kotlin does not have to find it again.
type routingMergeResult struct {
	Profiles []config.RoutingProfile `json:"profiles"`
	ActiveID string                  `json:"activeId"`
	Saved    config.RoutingProfile   `json:"saved"`
}

// MergeRoutingProfile folds an incoming profile into the stored list.
//
// storedJSON may be empty or "{}" — a first import has no store yet.
func MergeRoutingProfile(storedJSON, incomingJSON string, makeActive bool) (string, error) {
	var store routingStore
	if s := strings.TrimSpace(storedJSON); s != "" {
		if err := json.Unmarshal([]byte(s), &store); err != nil {
			return "", fmt.Errorf("parsing routing store: %w", err)
		}
	}
	var incoming config.RoutingProfile
	if err := json.Unmarshal([]byte(incomingJSON), &incoming); err != nil {
		return "", fmt.Errorf("parsing routing profile: %w", err)
	}
	profiles, activeID, saved, err := proxy.UpsertRoutingProfile(
		store.Profiles, incoming, store.ActiveID, makeActive)
	if err != nil {
		return "", err
	}
	blob, err := json.Marshal(routingMergeResult{
		Profiles: profiles, ActiveID: activeID, Saved: saved,
	})
	if err != nil {
		return "", fmt.Errorf("marshaling routing store: %w", err)
	}
	return string(blob), nil
}

// CompileRoutingProfile expands a profile's rules into cached rule-sets.
// Reaches the network when the profile references geo databases or linked
// lists, so callers must keep it off the main thread.
func CompileRoutingProfile(profileJSON, dataDir string, refreshGeo bool) (string, error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", fmt.Errorf("dataDir is required on mobile (pass context.filesDir)")
	}
	var p config.RoutingProfile
	if err := json.Unmarshal([]byte(profileJSON), &p); err != nil {
		return "", fmt.Errorf("parsing routing profile: %w", err)
	}
	rep, err := proxy.CompileRoutingProfile(context.Background(), p, dataDir, refreshGeo)
	if err != nil {
		return "", err
	}
	blob, merr := json.Marshal(rep)
	if merr != nil {
		return "", fmt.Errorf("marshaling compile report: %w", merr)
	}
	return string(blob), nil
}

// RoutingProfileStatus reports which of the three actions have a usable
// compiled rule-set on disk. This is how the UI answers "собран / не собран"
// without storing a flag that could drift from the files.
func RoutingProfileStatus(dataDir, profileID string) (string, error) {
	out := map[string]bool{}
	for _, action := range proxy.RoutingActions {
		out[action] = proxy.RoutingProfileSRSReady(dataDir, profileID, action)
	}
	blob, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("marshaling routing status: %w", err)
	}
	return string(blob), nil
}

// RemoveRoutingProfile deletes a profile's cached rule-sets. The config entry
// is Kotlin's to remove; a stale cache left here would keep routing traffic by
// a profile that is gone.
func RemoveRoutingProfile(dataDir, profileID string) error {
	if strings.TrimSpace(dataDir) == "" {
		return fmt.Errorf("dataDir is required")
	}
	proxy.RemoveRoutingProfileSRS(dataDir, profileID)
	return nil
}

// ExtractSubscriptionRouting pulls provider-declared routing out of a
// subscription response: the Routing-Lists header (base64 JSON) or a
// routingLists key in a JSON body. Returns "[]" when there is none.
func ExtractSubscriptionRouting(headerVal, body string) (string, error) {
	lists := proxy.ExtractSubscriptionRoutingLists(headerVal, body)
	if lists == nil {
		lists = []config.RoutingList{}
	}
	blob, err := json.Marshal(lists)
	if err != nil {
		return "", fmt.Errorf("marshaling subscription routing: %w", err)
	}
	return string(blob), nil
}
```

- [x] **Step 4: Запустить тесты — должны пройти**

```bash
cd /c/ResultV && TAGS=$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)
go test -tags="$TAGS" -count=1 ./mobile/ -run 'Routing' -v
```

Ожидается: 9 тестов PASS.

- [x] **Step 5: Обе конфигурации целиком**

Команды из «Global Constraints».

- [x] **Step 6: Коммит**

```bash
git add mobile/libbox_routing.go mobile/libbox_routing_test.go
git commit -m "feat(routing): биндинги gomobile для профилей маршрутизации

Семь функций, всё через JSON-строки. Слой без состояния: список профилей
держит Kotlin, как все остальные репозитории на этой платформе, а здесь
остаётся то, что нельзя писать дважды — правило замены профиля.

Форма JSON — ровно то, во что маршалится config.RoutingProfile: переименуй
поле здесь, и оно молча потеряется на круге через хранилище.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 10: Правила профиля в конфиге sing-box

**Files:**
- Modify: `mobile/libbox.go:756-808` (добавить два поля в `BuildOptions`)
- Modify: `mobile/libbox.go:1600-1613` (вызвать эмиссию после блока исключений)
- Modify: `mobile/libbox_routing.go` (дописать эмиссию правил)
- Test: `mobile/libbox_routing_rules_test.go`

**Interfaces:**
- Consumes: `proxy.RoutingProfileSRSReady`, `proxy.RoutingProfileSRSPath`, `proxy.RoutingProfileRuleSetTag`, `proxy.NormalizeRoutingOrder`, `proxy.DefaultRoutingOrder`, `proxy.SBRoute`, `proxy.SBRouteRule`, `proxy.SBRouteRuleSet`
- Produces: `func applyRoutingProfile(sb *proxy.SingBoxConfig, dataDir string, opts BuildOptions)`, поля `BuildOptions.RoutingProfileID`, `BuildOptions.RoutingOrder`

- [x] **Step 1: Написать падающий тест**

Создать `mobile/libbox_routing_rules_test.go`:

```go
package mobile

import (
	"encoding/json"
	"strings"
	"testing"

	"resultproxy-wails/internal/proxy"
)

// compileProfileForRules собирает SRS всех трёх действий, чтобы правила было
// на что ссылаться: эмиссия пропускает действие без готового файла.
func compileProfileForRules(t *testing.T, dir, id string) {
	t.Helper()
	profile := `{"id":"` + id + `","name":"R",` +
		`"directSites":["direct.example"],` +
		`"proxySites":["proxy.example"],` +
		`"blockSites":["block.example"]}`
	if _, err := CompileRoutingProfile(profile, dir, false); err != nil {
		t.Fatalf("подготовка SRS: %v", err)
	}
}

func buildConfigWithProfile(t *testing.T, dir string, opts BuildOptions) map[string]any {
	t.Helper()
	entry := `{"ip":"1.2.3.4","port":443,"type":"vless","name":"n",` +
		`"uri":"vless://11111111-1111-1111-1111-111111111111@1.2.3.4:443?security=none&type=tcp#n"}`
	out, err := BuildSingBoxConfigFromEntryV2(entry, dir, encodeOptions(opts))
	if err != nil {
		t.Fatalf("BuildSingBoxConfigFromEntryV2: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatalf("конфиг не JSON: %v", err)
	}
	return cfg
}

func routeRules(t *testing.T, cfg map[string]any) []map[string]any {
	t.Helper()
	route, ok := cfg["route"].(map[string]any)
	if !ok {
		t.Fatal("в конфиге нет route")
	}
	raw, _ := route["rules"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		if m, ok := r.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func ruleSetTags(t *testing.T, cfg map[string]any) []string {
	t.Helper()
	route, _ := cfg["route"].(map[string]any)
	raw, _ := route["rule_set"].([]any)
	var out []string
	for _, r := range raw {
		if m, ok := r.(map[string]any); ok {
			if tag, ok := m["tag"].(string); ok {
				out = append(out, tag)
			}
		}
	}
	return out
}

// indexOfProfileRule возвращает позицию первого правила, ссылающегося на
// rule_set профиля, и -1 если такого нет.
func indexOfProfileRule(rules []map[string]any, tag string) int {
	for i, r := range rules {
		set, _ := r["rule_set"].([]any)
		for _, s := range set {
			if s == tag {
				return i
			}
		}
	}
	return -1
}

func TestProfileRulesEmittedInGlobal(t *testing.T) {
	dir := t.TempDir()
	compileProfileForRules(t, dir, "ruleprof")
	cfg := buildConfigWithProfile(t, dir, BuildOptions{RoutingProfileID: "ruleprof"})

	tags := ruleSetTags(t, cfg)
	for _, action := range proxy.RoutingActions {
		want := proxy.RoutingProfileRuleSetTag("ruleprof", action)
		found := false
		for _, tag := range tags {
			if tag == want {
				found = true
			}
		}
		if !found {
			t.Errorf("rule_set %q не зарегистрирован: %v", want, tags)
		}
	}

	rules := routeRules(t, cfg)
	for _, action := range proxy.RoutingActions {
		tag := proxy.RoutingProfileRuleSetTag("ruleprof", action)
		if indexOfProfileRule(rules, tag) < 0 {
			t.Errorf("нет правила для %q", tag)
		}
	}
}

func TestProfileRulesIgnoredInSmart(t *testing.T) {
	dir := t.TempDir()
	compileProfileForRules(t, dir, "smartprof")
	cfg := buildConfigWithProfile(t, dir, BuildOptions{
		RoutingProfileID: "smartprof",
		SmartMode:        true,
	})
	rules := routeRules(t, cfg)
	for _, action := range proxy.RoutingActions {
		tag := proxy.RoutingProfileRuleSetTag("smartprof", action)
		if indexOfProfileRule(rules, tag) >= 0 {
			t.Errorf("правило %q попало в Smart — профиль там не действует", tag)
		}
	}
	if len(ruleSetTags(t, cfg)) > 0 {
		for _, tag := range ruleSetTags(t, cfg) {
			if strings.HasPrefix(tag, "prof-") {
				t.Errorf("rule_set %q зарегистрирован в Smart", tag)
			}
		}
	}
}

func TestProfileRulesSkipActionsWithoutSRS(t *testing.T) {
	dir := t.TempDir()
	// Только proxy — два других действия без правил, файлов не будет.
	profile := `{"id":"partial","name":"P","proxySites":["proxy.example"]}`
	if _, err := CompileRoutingProfile(profile, dir, false); err != nil {
		t.Fatalf("подготовка: %v", err)
	}
	cfg := buildConfigWithProfile(t, dir, BuildOptions{RoutingProfileID: "partial"})
	for _, tag := range ruleSetTags(t, cfg) {
		if tag == proxy.RoutingProfileRuleSetTag("partial", "direct") ||
			tag == proxy.RoutingProfileRuleSetTag("partial", "block") {
			t.Errorf("зарегистрирован rule_set без файла: %q", tag)
		}
	}
	rules := routeRules(t, cfg)
	if indexOfProfileRule(rules, proxy.RoutingProfileRuleSetTag("partial", "proxy")) < 0 {
		t.Error("правило proxy потеряно")
	}
}

func TestProfileRulesComeAfterExcludedDomains(t *testing.T) {
	dir := t.TempDir()
	compileProfileForRules(t, dir, "orderprof")
	cfg := buildConfigWithProfile(t, dir, BuildOptions{
		RoutingProfileID: "orderprof",
		ExcludedDomains:  "bank.example",
	})
	rules := routeRules(t, cfg)

	excluded := -1
	for i, r := range rules {
		suffixes, _ := r["domain_suffix"].([]any)
		for _, s := range suffixes {
			if s == "bank.example" {
				excluded = i
			}
		}
	}
	if excluded < 0 {
		t.Fatal("правило исключения не найдено")
	}
	profileAt := indexOfProfileRule(rules, proxy.RoutingProfileRuleSetTag("orderprof", "proxy"))
	if profileAt < 0 {
		t.Fatal("правило профиля не найдено")
	}
	if profileAt < excluded {
		t.Errorf("правило профиля (%d) стоит выше исключения (%d) — "+
			"вручную добавленный домен должен быть сильнее", profileAt, excluded)
	}
}

func TestProfileRuleOrderHonoursRouteOrder(t *testing.T) {
	dir := t.TempDir()
	compileProfileForRules(t, dir, "roprof")
	cfg := buildConfigWithProfile(t, dir, BuildOptions{
		RoutingProfileID: "roprof",
		RoutingOrder:     "direct-proxy-block",
	})
	rules := routeRules(t, cfg)
	d := indexOfProfileRule(rules, proxy.RoutingProfileRuleSetTag("roprof", "direct"))
	p := indexOfProfileRule(rules, proxy.RoutingProfileRuleSetTag("roprof", "proxy"))
	b := indexOfProfileRule(rules, proxy.RoutingProfileRuleSetTag("roprof", "block"))
	if d < 0 || p < 0 || b < 0 {
		t.Fatalf("не все правила на месте: direct=%d proxy=%d block=%d", d, p, b)
	}
	if !(d < p && p < b) {
		t.Errorf("порядок direct=%d proxy=%d block=%d, ждали direct<proxy<block", d, p, b)
	}
}

func TestProfileRuleOrderFallsBackOnGarbage(t *testing.T) {
	dir := t.TempDir()
	compileProfileForRules(t, dir, "badorder")
	cfg := buildConfigWithProfile(t, dir, BuildOptions{
		RoutingProfileID: "badorder",
		RoutingOrder:     "proxy-proxy-direct",
	})
	rules := routeRules(t, cfg)
	b := indexOfProfileRule(rules, proxy.RoutingProfileRuleSetTag("badorder", "block"))
	p := indexOfProfileRule(rules, proxy.RoutingProfileRuleSetTag("badorder", "proxy"))
	d := indexOfProfileRule(rules, proxy.RoutingProfileRuleSetTag("badorder", "direct"))
	if !(b < p && p < d) {
		t.Errorf("порядок block=%d proxy=%d direct=%d, ждали умолчание block<proxy<direct", b, p, d)
	}
}

func TestProfileBlockRuleRejects(t *testing.T) {
	dir := t.TempDir()
	compileProfileForRules(t, dir, "blockprof")
	cfg := buildConfigWithProfile(t, dir, BuildOptions{RoutingProfileID: "blockprof"})
	rules := routeRules(t, cfg)
	at := indexOfProfileRule(rules, proxy.RoutingProfileRuleSetTag("blockprof", "block"))
	if at < 0 {
		t.Fatal("правило block не найдено")
	}
	if rules[at]["action"] != "reject" {
		t.Errorf("action = %v, ждали reject", rules[at]["action"])
	}
	if _, has := rules[at]["outbound"]; has {
		t.Error("у reject-правила остался outbound")
	}
}

func TestProfileRulesRejectedInKillSwitchPanic(t *testing.T) {
	dir := t.TempDir()
	compileProfileForRules(t, dir, "panicprof")
	cfg := buildConfigWithProfile(t, dir, BuildOptions{
		RoutingProfileID: "panicprof",
		KillSwitchArmed:  true,
		KillSwitchPanic:  true,
	})
	rules := routeRules(t, cfg)
	for _, action := range proxy.RoutingActions {
		at := indexOfProfileRule(rules, proxy.RoutingProfileRuleSetTag("panicprof", action))
		if at < 0 {
			t.Fatalf("правило %s исчезло в panic", action)
		}
		if rules[at]["action"] != "reject" {
			t.Errorf("в panic правило %s = %v, ждали reject", action, rules[at]["action"])
		}
	}
}

func TestProfileConfigAcceptedByPinnedCore(t *testing.T) {
	dir := t.TempDir()
	compileProfileForRules(t, dir, "coreprof")
	entry := `{"ip":"1.2.3.4","port":443,"type":"vless","name":"n",` +
		`"uri":"vless://11111111-1111-1111-1111-111111111111@1.2.3.4:443?security=none&type=tcp#n"}`
	out, err := BuildSingBoxConfigFromEntryV2(entry, dir,
		encodeOptions(BuildOptions{RoutingProfileID: "coreprof"}))
	if err != nil {
		t.Fatalf("BuildSingBoxConfigFromEntryV2: %v", err)
	}
	// Ядро декодирует с DisallowUnknownFields: подложный ключ здесь — отказ.
	if err := proxy.ValidateSingBoxJSON([]byte(out)); err != nil {
		t.Fatalf("закреплённое ядро не принимает конфиг: %v", err)
	}
}
```

- [x] **Step 2: Никакой новой функции валидации не писать**

Приём уже применяется в `mobile/browser_adblock_redirect_test.go:206-225` —
разбор через `singjson.UnmarshalContext` с контекстом `include.Context`. Это
именно то, что делает `startInstance`, и другой способ дал бы другой ответ.
Поэтому последний тест задачи 10 пишется так (в `mobile/libbox_routing_rules_test.go`):

```go
func TestProfileConfigAcceptedByPinnedCore(t *testing.T) {
	dir := t.TempDir()
	compileProfileForRules(t, dir, "coreprof")
	for _, tc := range []struct {
		name string
		opts BuildOptions
	}{
		{"обычный", BuildOptions{RoutingProfileID: "coreprof"}},
		{"порядок", BuildOptions{RoutingProfileID: "coreprof", RoutingOrder: "direct-proxy-block"}},
		{"panic", BuildOptions{RoutingProfileID: "coreprof", KillSwitchArmed: true, KillSwitchPanic: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := BuildSingBoxConfigV2(routingTestURI, dir, encodeOptions(tc.opts))
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			var parsed option.Options
			ctx := include.Context(context.Background())
			if err := singjson.UnmarshalContext(ctx, []byte(out), &parsed); err != nil {
				t.Fatalf("закреплённое ядро отвергло конфиг: %v
config: %s", err, out)
			}
		})
	}
}
```

Импорты файла дополнить так же, как в образце:

```go
import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"

	"resultproxy-wails/internal/proxy"
)

const routingTestURI = "vless://11111111-1111-1111-1111-111111111111@1.2.3.4:443?security=tls&type=tcp#routing"
```

И тогда `buildConfigWithProfile` из шага 1 строится на том же URI:

```go
func buildConfigWithProfile(t *testing.T, dir string, opts BuildOptions) map[string]any {
	t.Helper()
	out, err := BuildSingBoxConfigV2(routingTestURI, dir, encodeOptions(opts))
	if err != nil {
		t.Fatalf("BuildSingBoxConfigV2: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatalf("конфиг не JSON: %v", err)
	}
	return cfg
}
```

Версию `buildConfigWithProfile` и `TestProfileConfigAcceptedByPinnedCore` из
шага 1, собиравшие конфиг из `entryJson`, заменить на эти: `BuildSingBoxConfigV2`
короче и это тот же путь, которым уже пользуются соседние тесты.

- [x] **Step 3: Запустить — должен упасть**

```bash
cd /c/ResultV && TAGS=$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)
go test -tags="$TAGS" -count=1 ./mobile/ -run 'Profile' 2>&1 | head -10
```

Ожидается: `unknown field RoutingProfileID in struct literal` — поля ещё нет.

- [x] **Step 4: Добавить два поля в `BuildOptions`**

В `mobile/libbox.go`, в конец структуры `BuildOptions` (после `BlockedApps`):

```go
	// RoutingProfileID names the active routing profile. The engine looks up
	// its compiled rule-sets on disk by this id; the rules themselves never
	// cross JNI. A profile can carry 20 000 tokens — shipping those on every
	// connect is the mistake the Smart list already had to undo (see
	// SmartBlockedDomainsList above).
	//
	// Empty means no profile. Ignored in Smart mode: there the client works out
	// routing itself, and a profile would be pulling against it. The gate lives
	// here rather than in Kotlin so it is a single place covered by a test.
	RoutingProfileID string `json:"routingProfileId,omitempty"`
	// RoutingOrder is the active profile's RouteOrder, e.g.
	// "block-proxy-direct". Anything that is not a permutation of the three
	// actions falls back to DefaultRoutingOrder rather than being guessed at:
	// the order decides which rule wins when several match.
	RoutingOrder string `json:"routingOrder,omitempty"`
```

- [x] **Step 5: Дописать эмиссию правил**

В конец `mobile/libbox_routing.go`:

```go
// applyRoutingProfile registers the active profile's compiled rule-sets and
// appends its route rules.
//
// Placement: LAST, after the excluded-domain block that buildSingBoxConfigFromEntry
// appends. That is deliberate and is the one place this diverges from the
// desktop, where a profile outranks the user's own "out of VPN" domains. A
// domain the user typed by hand is a fresher and more specific intent than a
// provider's list covering half the internet, so it wins here.
//
// Registration and reference live together on purpose: a rule pointing at an
// unregistered rule_set fails the core's start outright, and splitting the two
// across files is how that bug gets written.
func applyRoutingProfile(sb *proxy.SingBoxConfig, dataDir string, opts BuildOptions) {
	id := strings.TrimSpace(opts.RoutingProfileID)
	if id == "" || sb == nil || sb.Route == nil {
		return
	}
	// Smart decides routing itself; a profile there would fight it.
	if opts.SmartMode {
		return
	}
	ready := make(map[string]bool, len(proxy.RoutingActions))
	for _, action := range proxy.RoutingActions {
		if !proxy.RoutingProfileSRSReady(dataDir, id, action) {
			continue
		}
		ready[action] = true
		sb.Route.RuleSet = append(sb.Route.RuleSet, proxy.SBRouteRuleSet{
			Type:   "local",
			Tag:    proxy.RoutingProfileRuleSetTag(id, action),
			Format: "binary",
			Path:   proxy.RoutingProfileSRSPath(dataDir, id, action),
		})
	}
	if len(ready) == 0 {
		return
	}
	order := proxy.DefaultRoutingOrder
	if strings.TrimSpace(opts.RoutingOrder) != "" {
		order = proxy.NormalizeRoutingOrder(opts.RoutingOrder)
	}
	for _, action := range order {
		if !ready[action] {
			continue
		}
		rule := proxy.SBRouteRule{RuleSet: []string{proxy.RoutingProfileRuleSetTag(id, action)}}
		switch action {
		case "block":
			rule.Action = "reject"
		case "proxy":
			rule.Action = "route"
			rule.Outbound = "proxy"
		default: // "direct"
			rule.Action = "route"
			rule.Outbound = "direct"
		}
		sb.Route.Rules = append(sb.Route.Rules, rule)
	}
}
```

Добавить в импорты `mobile/libbox_routing.go` пакет `proxy` уже есть; `strings`
уже есть.

- [x] **Step 6: Вызвать эмиссию в нужном месте**

В `mobile/libbox.go`, в `buildSingBoxConfigFromEntry`, сразу **после**
закрывающей скобки блока исключённых доменов и **до** `if sb.DNS != nil {`
(ориентир — строка 1613, конец блока `if opts.ExcludedDomains != "" && !opts.SmartMode {`):

```go
	}

	// Routing profile: registered and emitted last, after the excluded-domain
	// rules above. See applyRoutingProfile for why that order and not the
	// desktop's.
	applyRoutingProfile(&sb, dataDir, opts)

	// `type: local` resolves through the system resolver, which on Android
```

- [x] **Step 7: Запустить тесты — должны пройти**

```bash
cd /c/ResultV && TAGS=$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)
go test -tags="$TAGS" -count=1 ./mobile/ -run 'Profile' -v
```

Ожидается: 9 тестов PASS. Если `TestProfileRulesRejectedInKillSwitchPanic`
падает — смотреть `applyKillSwitch` (`libbox.go:1376`): правило профиля должно
попасть в ветку `r.Outbound == "proxy" || r.Outbound == "direct"`, а
block-правило уже `reject` и не трогается.

- [x] **Step 8: Обе конфигурации целиком**

Команды из «Global Constraints».

- [x] **Step 9: Коммит**

```bash
git add mobile/libbox.go mobile/libbox_routing.go mobile/libbox_routing_rules_test.go
git commit -m "feat(routing): правила активного профиля в конфиге sing-box

Эмиссия последней, после исключений «мимо ВПН»: вручную добавленный домен
сильнее правила профиля — расхождение с ПК, принятое сознательно.

engine.go не тронут. Регистрация rule_set и ссылка на него лежат в одном
файле: правило на незарегистрированный rule_set валит старт ядра, и разносить
половины по файлам значит заводить эту ошибку заранее.

Через JNI ездит только id профиля и порядок действий; правила движок находит
на диске сам.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

## Почему нет отдельного теста «профиль после ad-block»

Спека (5.5) требует, чтобы правила ad-block и редирект браузерного ad-block
стояли выше правил профиля. Отдельной проверки на это не нужно, и вот почему:
и те и другие эмитятся внутри `buildRoute`, а профиль дописывается последним,
уже после блока исключённых доменов. `TestProfileRulesComeAfterExcludedDomains`
доказывает, что профиль ниже исключений; исключения, в свою очередь,
дописываются после всего, что вернул `buildRoute`. Значит профиль ниже и
ad-block, и редиректа — структурно, а не по совпадению.

Отдельный тест пришлось бы писать под `AdBlock: true`, а в конфигурации `play`
константа `adBlockSupported` равна `false` и правила не эмитятся вовсе — тест
проходил бы в одной сборке и был бы бессмысленным в другой.

---

## Готовность этапа A

- [x] `go build` зелёный в обеих конфигурациях
- [x] `go test -count=1 ./internal/proxy/... ./mobile/...` зелёный в обеих конфигурациях
- [x] Конфиг с правилами профиля принят закреплённым ядром (`TestProfileConfigAcceptedByPinnedCore`)
- [x] `internal/proxy/engine.go` не изменён: `git diff --stat main..HEAD -- internal/proxy/engine.go` пуст
- [x] Kotlin не изменён: `git diff --stat main..HEAD -- android/` пуст

Этап B (Kotlin, диплинк, экраны) начинается отсюда.
