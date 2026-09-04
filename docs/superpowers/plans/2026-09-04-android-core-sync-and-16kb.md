# Ядро с ПК на Android + выравнивание 16 КБ — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Перенести на ветку `android` парсер ссылок и подписок с ветки `dev` (mKCP, port hopping, SIP003, VLESS Encryption, multiplex, xhttp-обфускация, Host-заголовки, структурные AUTO-группы) и собрать AAR с 16-килобайтным выравниванием, которого требует Google Play.

**Architecture:** Ядро у Android и ПК общее — `mobile/libbox.go` импортирует `internal/proxy` и `internal/config`. Но деревья разъехались: на `android` нет десктопных файлов, на `dev` нет мобильных. Поэтому перенос пофайловый: семь файлов копируются целиком, а `engine.go`, `singbox.go`, `endpoints.go` и `crypto.go` правятся точечно. Проверено экспериментом в отдельном worktree: после шести правок сборка зелёная, список правок исчерпывающий.

**Tech Stack:** Go 1.26.4, gomobile (форк sagernet), sing-box-extended, Android NDK r27+, Gradle 9.0 / AGP 8.13.2, Kotlin 2.0.21.

**Spec:** `docs/android-pc-sync-and-play-spec.md` (разделы 1–3)

## Global Constraints

- Пакеты `internal/proxy` и `mobile` **не собираются без `-tags=mobile`**. Любая команда `go build` / `go test` / `go vet` по ним обязана нести этот флаг. Ошибки «undefined» без него — шум, а не сигнал.
- Исходник для копирования — **`C:\ResultVPC\internal\...`** (ветка `dev`, дерево чистое). Папка `C:\ResultV\ResultV-dev` содержит то же самое побайтово, но **своего `.git` не имеет**: git-команды из неё отвечают за `C:\ResultV` (ветка `android`).
- `docs/superpowers/` закрыт `.gitignore` (строка 34, шаблон `superpowers/`). Файлы оттуда добавлять **только** `git add -f`, иначе коммит уйдёт пустым.
- `engine.go` и `singbox.go` **не копировать** с `dev` ни при каких условиях. Из них берутся только определения структур, перечисленные в задаче 3.
- AAR пересобирается **только** через `bash scripts/build-android-aar.sh`. `gradlew assembleDebug` упаковывает уже лежащий `android/libs/libbox.aar` и Go не трогает — изменения молча не доедут.
- Установка на устройство: `adb install -r -d`. **Никогда не `adb uninstall`** — это стирает профили пользователя.
- Сообщения коммитов заканчиваются строкой:
  `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>`

---

### Task 1: Устаревший тест RU-источников

`TestDefaultPublicSourceTemplatesRU` ждёт в списке для RU две ссылки на citizenlab, которых там нет намеренно. Комментарий в `blocked_provider.go:325-331` объясняет почему: citizenlab test-lists — это списки для *измерения* цензуры, они содержат ~2000 разрешённых российских доменов (ok.ru, mail.ru, vk.com, yandex.ru, kremlin.ru), и как блок-лист они и гнали эти сайты через прокси, и на Android затягивали их приложения в Smart-разрешения. Тест не обновили вместе с кодом.

**Files:**
- Modify: `internal/proxy/blocked_provider_test.go:170-200`

**Interfaces:**
- Consumes: `defaultPublicSourceTemplates(country string) []string` (`blocked_provider.go:319`)
- Produces: ничего, задача только про тест

- [ ] **Step 1: Убедиться, что тест падает именно так**

Run: `go test -tags=mobile -count=1 -run TestDefaultPublicSourceTemplatesRU ./internal/proxy/`

Expected: FAIL, две строки `ru sources missing "citizenlab/..."`

- [ ] **Step 2: Заменить ожидания на актуальные**

В `internal/proxy/blocked_provider_test.go` убрать из `wantContains` две первые строки и добавить проверку намеренного отсутствия citizenlab. Список `wantContains` становится таким:

```go
	wantContains := []string{
		"itdoginfo/allow-domains/main/Russia/inside-raw.lst",
		"1andrevich/Re-filter-lists/main/domains_all.lst",
		"1andrevich/Re-filter-lists/main/community.lst",
		"itdoginfo/allow-domains/main/Services/discord.lst",
		"itdoginfo/allow-domains/main/Services/youtube.lst",
		"itdoginfo/allow-domains/main/Services/google_ai.lst",
		"Flowseal/zapret-discord-youtube/main/lists/list-general.txt",
	}
```

Сразу после цикла `for _, frag := range wantContains` добавить:

```go
	// citizenlab для RU исключён намеренно: это список для ИЗМЕРЕНИЯ цензуры,
	// он содержит ~2000 разрешённых доменов (ok.ru, vk.com, yandex.ru), и как
	// блок-лист гнал бы их через прокси. См. blocked_provider.go:325.
	for _, s := range sources {
		if strings.Contains(s, "citizenlab") {
			t.Errorf("ru sources must not contain citizenlab probe lists; got %q", s)
		}
	}
```

- [ ] **Step 3: Прогнать тест**

Run: `go test -tags=mobile -count=1 -run TestDefaultPublicSourceTemplatesRU ./internal/proxy/`

Expected: PASS

- [ ] **Step 4: Прогнать весь пакет — убедиться, что больше в нём ничего не красное**

Run: `go test -tags=mobile -count=1 ./internal/proxy/`

Expected: `ok resultproxy-wails/internal/proxy`

- [ ] **Step 5: Коммит**

```bash
git add internal/proxy/blocked_provider_test.go
git commit -m "$(cat <<'EOF'
test(proxy): привести тест RU-источников к тому, что код делает намеренно

citizenlab test-lists исключены из RU-набора осознанно — это списки для
измерения цензуры, они тянут ~2000 разрешённых доменов. Тест не обновили
вместе с кодом и он падал. Теперь он проверяет их отсутствие.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: `TestUserRuleOrder` — тесту нужен настоящий SRS на диске

Тест строит конфиг с `AdBlock: true` и ищет правило с `rule_set`. Правила ad-block ссылаются только на те SRS-теги, файлы которых реально лежат на диске **и разбираются парсером sing-box** (`buildAdBlockRuleSets` → `localAdBlockSRSUsable`, `adblock_rules.go:88,137`) — так сделано, чтобы битый rule_set не ронял старт движка. Тестовый `dataDir` — это пустой `t.TempDir()`, поэтому тегов нет и правило не эмитится. Тест сломался, когда правила перевели на «только закэшированные теги».

Пустышкой не обойтись: `localAdBlockSRSUsable` требует размер ≥ 512 байт (`minLocalSRSBytes`) и прогоняет содержимое через `srs.Read`. Валидный SRS умеет писать экспортированная `proxy.CompileSmartSRS`. Замерено: 200 доменов дают 1395 байт, 50 доменов — 488 байт, то есть 200 берётся с запасом, а 50 не хватило бы.

**Files:**
- Modify: `mobile/libbox_rules_test.go:1-28`

**Interfaces:**
- Consumes: `proxy.CompileSmartSRS(domains []string, path string) error` (`internal/proxy/smart_ruleset.go:58`) — сама создаёт директорию
- Produces: хелпер `seedAdBlockSRS(t *testing.T, dataDir string)` для тестов пакета `mobile`

- [ ] **Step 1: Убедиться, что тест падает именно так**

Run: `go test -tags=mobile -count=1 -run TestUserRuleOrder ./mobile/`

Expected: FAIL, `no rule_set rule emitted despite AdBlock: true`

- [ ] **Step 2: Добавить хелпер и засеять SRS в `buildRules`**

В `mobile/libbox_rules_test.go` расширить импорты:

```go
import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"resultproxy-wails/internal/proxy"
)
```

Добавить хелпер сразу после константы `rulesTestURI`:

```go
// seedAdBlockSRS кладёт в dataDir валидные SRS ad-block списков.
//
// buildAdBlockRuleSets ссылается только на SRS, который лежит на диске И
// разбирается парсером sing-box (adblock_rules.go:88, 137) — иначе битый
// rule_set уронил бы старт движка. Пустой t.TempDir() поэтому даёт конфиг
// вообще без rule_set, и проверка порядка правил теряет свой якорь.
//
// 200 доменов дают ~1.4 КБ, минимум minLocalSRSBytes — 512 байт.
func seedAdBlockSRS(t *testing.T, dataDir string) {
	t.Helper()
	domains := make([]string, 0, 200)
	for i := 0; i < 200; i++ {
		domains = append(domains, fmt.Sprintf("ad-%d-tracker-%d.example%d.com", i, i*7919, i%97))
	}
	for _, name := range []string{"ads.srs", "ads-ru.srs"} {
		if err := proxy.CompileSmartSRS(domains, filepath.Join(dataDir, "adblock", name)); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
}
```

Переписать начало `buildRules` — засеваем только когда ad-block включён, чтобы остальные тесты видели ровно тот же конфиг, что и раньше:

```go
func buildRules(t *testing.T, opts BuildOptions) []map[string]any {
	t.Helper()
	dir := t.TempDir()
	if opts.AdBlock {
		seedAdBlockSRS(t, dir)
	}
	out, err := BuildSingBoxConfigV2(rulesTestURI, dir, encodeOptions(opts))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
```

Остальная часть `buildRules` не меняется.

- [ ] **Step 3: Прогнать тест**

Run: `go test -tags=mobile -count=1 -run TestUserRuleOrder ./mobile/`

Expected: PASS

- [ ] **Step 4: Прогнать весь пакет — засев не должен сдвинуть другие проверки порядка**

Run: `go test -tags=mobile -count=1 ./mobile/`

Expected: `ok resultproxy-wails/mobile`

- [ ] **Step 5: Зафиксировать зелёную базу целиком**

Run: `go test -tags=mobile -count=1 ./internal/... ./mobile/...`

Expected: ни одного FAIL. Это точка отсчёта для всего дальнейшего переноса.

- [ ] **Step 6: Коммит**

```bash
git add mobile/libbox_rules_test.go
git commit -m "$(cat <<'EOF'
test(mobile): дать TestUserRuleOrder настоящий SRS на диске

Правила ad-block ссылаются только на закэшированные и валидные SRS-теги,
поэтому в пустом t.TempDir() rule_set не эмитился вовсе и тест терял свой
якорь. Засеваем валидный SRS через CompileSmartSRS, когда AdBlock включён.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Подготовить приёмник — структуры, стаб отпечатка, ключи AWG

Перенесённые `uriparser.go` и `outbound.go` опираются на то, чего в дереве `android` пока нет: поля sing-box для mKCP / xhttp / multiplex / port hopping, функция отпечатка, взятая из десктопного `internal/system`, и функция нормализации ключей AWG. Задача добавляет всё это заранее, чтобы следующая свелась к копированию файлов.

Всё добавляемое — либо поля с `omitempty` (JSON существующих outbound не меняется ни на байт), либо новые функции, которые пока никто не вызывает. Поведение остаётся прежним, тесты обязаны остаться зелёными.

**Files:**
- Modify: `internal/proxy/engine.go:189-220` (структура `SBOutbound`), `engine.go` рядом с `SBHysteria2Obfs` (новый тип `SBMultiplex`), структура `SBOutboundTransport`
- Modify: `internal/proxy/mobile_stubs.go` (в конец)
- Modify: `internal/proxy/endpoints.go` (в конец)

**Interfaces:**
- Produces:
  - `type SBMultiplex struct` с полями `Enabled bool`, `Protocol string`, `MaxConnections int`, `MinStreams int`, `MaxStreams int`, `Padding bool`
  - поля `SBOutbound`: `Plugin string`, `PluginOptions string`, `Encryption string`, `ServerPorts []string`, `HopInterval string`, `Multiplex *SBMultiplex`
  - `SBOutboundTransport` в версии `dev` (26 дополнительных полей)
  - `func defaultUTLSFingerprint() string` — только под `//go:build mobile`
  - `var awg3Keys []string` и `func normalizeAWGKey(s string) string`

- [ ] **Step 1: Добавить тип `SBMultiplex` и поля в `SBOutbound`**

В `internal/proxy/engine.go` перед строкой `type SBHysteria2Obfs struct {` вставить:

```go
type SBMultiplex struct {
	Enabled        bool   `json:"enabled,omitempty"`
	Protocol       string `json:"protocol,omitempty"`
	MaxConnections int    `json:"max_connections,omitempty"`
	MinStreams     int    `json:"min_streams,omitempty"`
	MaxStreams     int    `json:"max_streams,omitempty"`
	Padding        bool   `json:"padding,omitempty"`
}

```

В структуре `SBOutbound` найти блок полей группы urltest (он идёт последним и начинается с комментария про kill switch):

```go
	// urltest/selector group fields (mobile kill-switch only; omitempty keeps
	// every existing outbound's JSON byte-identical).
	Outbounds []string `json:"outbounds,omitempty"`
```

Эти три поля (`Outbounds`, `URL`, `Interval`) есть только на `android` и сохраняются как есть. Непосредственно **перед** этим комментарием вставить:

```go
	// Shadowsocks SIP003 plugin.
	Plugin        string `json:"plugin,omitempty"`
	PluginOptions string `json:"plugin_opts,omitempty"`
	// Строка VLESS Encryption — раньше терялась молча.
	Encryption string `json:"encryption,omitempty"`
	// Hysteria2 port hopping.
	ServerPorts []string     `json:"server_ports,omitempty"`
	HopInterval string       `json:"hop_interval,omitempty"`
	Multiplex   *SBMultiplex `json:"multiplex,omitempty"`

```

- [ ] **Step 2: Заменить `SBOutboundTransport` версией с `dev`**

У `android` в этой структуре нет ни одного своего поля, у `dev` их на 26 больше (весь mKCP, весь блок `XPadding*`, `Session*`, `Seq*`, `UplinkData*`). Замену делать программно, а не переписывая руками — в комментариях `dev` есть длинные тире, и ручной перенос их портит:

```bash
python - <<'PY'
def block(path):
    src = open(path, encoding='utf-8').read()
    i = src.index('type SBOutboundTransport struct')
    j = src.index('\n}\n', i) + 3
    return src[i:j]
dst = r'C:\ResultV\internal\proxy\engine.go'
s = open(dst, encoding='utf-8').read()
s = s.replace(block(dst), block(r'C:\ResultVPC\internal\proxy\engine.go'), 1)
open(dst, 'w', encoding='utf-8').write(s)
print('SBOutboundTransport replaced')
PY
```

Проверить глазами, что `json` остался в импортах `engine.go` (новые поля используют `json.RawMessage`).

- [ ] **Step 3: Добавить стаб отпечатка uTLS**

Версия `outbound.go` с `dev` берёт отпечаток по умолчанию из десктопного `internal/system`, которого на `android` нет. Стаб кладётся в `internal/proxy/mobile_stubs.go` — файл заведён ровно для таких случаев и уже несёт `//go:build mobile`. Дописать в конец:

```go

// defaultUTLSFingerprint is "chrome" on mobile: there is no Edge WebView2 to
// match against, and chrome is the safest mainstream fingerprint that still
// passes Reality's masquerade check.
func defaultUTLSFingerprint() string { return "chrome" }
```

- [ ] **Step 4: Свести имена ключей AWG 3.0 и добавить нормализацию**

`dev` называет список `awg3Keys`, `android` — `awg3DeviceKnobs`; содержимое идентично, сверено построчно. Переименовываем на `android`, чтобы следующие переносы не требовали правки:

```bash
sed -i 's/\bawg3DeviceKnobs\b/awg3Keys/g' internal/proxy/endpoints.go internal/proxy/config_validation.go internal/proxy/uriparser.go internal/proxy/ping_wg_handshake.go
```

В конец `internal/proxy/endpoints.go` дописать функцию, которой на `android` нет и которая нужна перенесённому парсеру:

```go

// normalizeAWGKey folds the spellings providers use for the same knob:
// "HeaderProtectionKey" (amneziawg .conf style), "header_protection_key"
// (JSON subscriptions) and "header-protection-key" all collapse to one form.
func normalizeAWGKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if r != '_' && r != '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
```

Проверить, что `strings` есть в импортах `endpoints.go`.

- [ ] **Step 5: Собрать и прогнать тесты — поведение не должно измениться**

Run:
```bash
go build -tags=mobile ./internal/... ./mobile/...
go test -tags=mobile -count=1 ./internal/... ./mobile/...
```

Expected: сборка exit 0, тесты без FAIL. Новые поля несут `omitempty`, новые функции пока никем не вызываются — если что-то покраснело, значит `sed` из шага 4 задел лишнее.

- [ ] **Step 6: Коммит**

```bash
git add internal/proxy/engine.go internal/proxy/mobile_stubs.go internal/proxy/endpoints.go internal/proxy/config_validation.go internal/proxy/uriparser.go internal/proxy/ping_wg_handshake.go
git commit -m "$(cat <<'EOF'
feat(proxy): подготовить структуры и хелперы под перенос парсера с ПК

Поля sing-box для mKCP, xhttp-обфускации, multiplex, SIP003 и port hopping;
стаб отпечатка uTLS вместо десктопного internal/system; awg3Keys вместо
awg3DeviceKnobs и normalizeAWGKey. Всё с omitempty — JSON существующих
outbound не меняется.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Перенести ядро парсера с `dev`

Основная задача. Семь файлов копируются целиком, четыре места правятся. Список правок получен экспериментом (сборка в отдельном worktree до зелёного) и является исчерпывающим — других ошибок компиляции не будет.

**Files:**
- Overwrite: `internal/proxy/uriparser.go`, `internal/proxy/outbound.go`, `internal/config/*.go`
- Create: `internal/proxy/autogroup.go`, `internal/proxy/probe_udp_relay.go`, `internal/proxy/doh.go`, `internal/proxy/ping_resolve.go`
- Modify: `internal/config/crypto.go` (вернуть `HashHWIDSource`), `mobile/libbox.go:563`, `internal/proxy/lcp_test.go:135`, `internal/proxy/uriparser.go` (вернуть j-ключи)

**Interfaces:**
- Consumes: всё из задачи 3
- Produces:
  - `proxy.SplitAutoEntries(entries []config.ProxyEntry) (groups []AutoGroup, individual []config.ProxyEntry, ok bool)` — замена `SplitAutoEntriesMulti`
  - `config.ProxyEntry.AutoGroup string` — имя пула, объявленного провайдером
  - `config.HashHWIDSource(source string) string` — сохраняется как было

- [ ] **Step 1: Скопировать файлы**

```bash
D=/c/ResultVPC
cp $D/internal/proxy/uriparser.go $D/internal/proxy/outbound.go $D/internal/proxy/autogroup.go internal/proxy/
cp $D/internal/proxy/probe_udp_relay.go $D/internal/proxy/doh.go $D/internal/proxy/ping_resolve.go internal/proxy/
cp $D/internal/config/*.go internal/config/
```

Ничего больше не копировать. `engine.go` и `singbox.go` — под запретом (см. Global Constraints).

- [ ] **Step 2: Убрать десктопный импорт из `outbound.go`**

Удалить строку `"resultproxy-wails/internal/system"` из блока импортов и заменить единственный вызов:

```bash
sed -i '/"resultproxy-wails\/internal\/system"/d' internal/proxy/outbound.go
sed -i 's|return system\.WebViewFingerprint()|return defaultUTLSFingerprint()|' internal/proxy/outbound.go
```

- [ ] **Step 3: Вернуть `HashHWIDSource` в `internal/config/crypto.go`**

Это единственная функция, которая есть на `android` и отсутствует на `dev`; копирование директории её стирает, и `mobile/libbox.go:478` перестаёт собираться. Дописать в конец `internal/config/crypto.go`:

```go

// HashHWIDSource hashes a caller-supplied stable OS identifier into the HWID
// the subscription panel sees.
//
// Mobile callers pass an OS-level id (Android: Settings.Secure.ANDROID_ID)
// that survives app reinstalls. The dataDir fallback file StableHardwareID
// relies on does NOT survive a reinstall / clear-data, so without this the
// panel would register a fresh "device" on every install and burn the
// provider's HWID-limit slots. Empty source returns "" (caller skips x-hwid).
func HashHWIDSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return ""
	}
	h := sha256.Sum256([]byte(source + hwidSalt))
	return hex.EncodeToString(h[:])
}
```

Проверить, что в импортах `crypto.go` есть `crypto/sha256`, `encoding/hex`, `strings`, и что константа `hwidSalt` не потерялась при копировании — если её нет, взять из `git show HEAD:internal/config/crypto.go`.

- [ ] **Step 4: Обновить два вызова `SplitAutoEntriesMulti`**

У версии с `dev` три возвращаемых значения вместо двух. Структура `AutoGroup` идентична (`Name string`, `Members []config.ProxyEntry`), поэтому тела вызывающих функций не меняются.

```bash
sed -i 's|groups, individuals := proxy\.SplitAutoEntriesMulti(entries)|groups, individuals, _ := proxy.SplitAutoEntries(entries)|' mobile/libbox.go
sed -i 's|groups, individuals := SplitAutoEntriesMulti(tc\.entries)|groups, individuals, _ := SplitAutoEntries(tc.entries)|' internal/proxy/lcp_test.go
```

- [ ] **Step 5: Собрать**

Run: `go build -tags=mobile ./internal/... ./mobile/...`

Expected: exit 0. Если появились ошибки — их нет в проверенном списке, значит дерево `dev` изменилось с 2026-09-04; остановиться и разобраться, а не глушить симптом.

- [ ] **Step 6: Прогнать тесты и увидеть единственную регрессию**

Run: `go test -tags=mobile -count=1 ./internal/... ./mobile/...`

Expected: ровно один FAIL —

```
--- FAIL: TestUnsupportedAWGKnobsFromParsedURI
    libbox_awg_knobs_test.go:59: knobs = "", want "j1,itime"
```

Соседний `TestUnsupportedAWGKnobs` (вариант с готовым entry-JSON) проходит: расходится только путь через URI.

- [ ] **Step 7: Вернуть джанк-ключи AWG в разбор**

Парсер с `dev` выбрасывает `j1`/`j2`/`j3`/`itime` прямо при разборе, а `android` их сохраняет, чтобы `UnsupportedAWGKnobs` мог предупредить пользователя, что конфиг будет работать с ослабленной защитой от DPI. Это была цель коммита `7b4b1c5`, и поведение `android` здесь правильнее. Ключи выпали из двух пар списков.

В `internal/proxy/uriparser.go` в **обоих** местах, где объявлен список целочисленных ключей amnezia, добавить `"itime"` в конец:

```go
			intKeys := []string{"jc", "jmin", "jmax", "s1", "s2", "s3", "s4", "itime"}
```

```go
	amneziaIntKeys := []string{"jc", "jmin", "jmax", "s1", "s2", "s3", "s4", "itime"}
```

И в **обоих** местах со строковыми ключами добавить `"j1", "j2", "j3"`:

```go
			strKeys := []string{"i1", "i2", "i3", "i4", "i5", "j1", "j2", "j3"}
```

```go
	amneziaStringKeys := []string{"i1", "i2", "i3", "i4", "i5", "j1", "j2", "j3"}
```

Над каждой парой оставить комментарий, чтобы следующий перенос не срезал их снова:

```go
	// j1-j3 и itime движок исполнить не может (см. unsupportedAmneziaKnobs),
	// но разбираем их намеренно: UnsupportedAWGKnobs предупреждает
	// пользователя, что DPI-защита конфига будет неполной. Молча выбросить
	// их — значит скрыть от него деградацию.
```

- [ ] **Step 8: Прогнать тесты — должно стать полностью зелено**

Run: `go test -tags=mobile -count=1 ./internal/... ./mobile/...`

Expected: ни одного FAIL.

- [ ] **Step 9: Коммит**

```bash
git add internal/proxy/ internal/config/ mobile/libbox.go
git commit -m "$(cat <<'EOF'
feat(proxy): перенести парсер ссылок и подписок с ветки dev

mKCP, Hysteria2 port hopping, Shadowsocks SIP003, VLESS Encryption,
sing-box multiplex, xhttp-обфускация, Host-заголовки на ws/httpupgrade/http,
сохранение embedded-extra, структурные AUTO-группы по объявленному
провайдером балансировщику. Плюс пробы UDP-релея и резолв через DoH.

Джанк-ключи AWG (j1-j3, itime) продолжаем разбирать, чтобы предупреждать
о неполной DPI-защите — версия с dev выбрасывала их молча.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: AUTO-группы — структурная стратегия плюс флаг-фолбэк

После задачи 4 группировка AUTO определяется полем `AutoGroup`, которое парсер ставит из объявленного провайдером xray-балансировщика. Это строго лучше прежнего: не ломается при переименовании группы и переживает кириллическое «Авто».

Но у стратегии `dev` есть откат по именам (`splitAutoEntriesByName`), и он **ограничен одним пулом**: берёт все записи со словом «auto»/«авто» и делает из них одну группу. У `android` откат был другой — он раскладывал записи по ведущему флаг-эмодзи, и несколько пулов были возможны. Для строчных подписок (просто список `vless://`, никакой структуры) это регрессия: несколько флаг-групп схлопнутся в одну.

Задача переносит флаг-раскладку `android` внутрь отката `dev`. Структурная стратегия остаётся первой и нетронутой.

**Files:**
- Modify: `internal/proxy/autogroup.go:181-199` (`splitAutoEntriesByName`)
- Modify: `internal/proxy/lcp_test.go`

**Interfaces:**
- Consumes: `StripLeadingFlagEmoji(s string) (emoji, rest string)`, `ExtractAutoGroupName(entries []config.ProxyEntry) (string, bool)`, `containsWordAuto(s string) bool` — все уже в пакете после задачи 4
- Produces: `splitAutoEntriesByName` с прежней сигнатурой `([]AutoGroup, []config.ProxyEntry, bool)`, но с несколькими пулами

- [ ] **Step 1: Написать падающий тест на несколько флаг-групп**

В `internal/proxy/lcp_test.go` добавить:

```go
// Строчная подписка без структуры: два флага — два пула. Откат по именам,
// пришедший с dev, схлопывал их в один; флаг-раскладка это чинит.
func TestSplitAutoEntriesByNameKeepsFlagGroups(t *testing.T) {
	entries := []config.ProxyEntry{
		{Name: "🇳🇱 Auto NL", IP: "1.1.1.1"},
		{Name: "🇳🇱 Auto NL 2", IP: "1.1.1.2"},
		{Name: "🇩🇪 Auto DE", IP: "2.2.2.1"},
		{Name: "🇩🇪 Auto DE 2", IP: "2.2.2.2"},
		{Name: "Amsterdam #1", IP: "3.3.3.1"},
	}
	groups, individual, ok := SplitAutoEntries(entries)
	if !ok {
		t.Fatal("SplitAutoEntries: ok=false, want true")
	}
	if len(groups) != 2 {
		t.Fatalf("groups=%d, want 2 (по одному на флаг)", len(groups))
	}
	// Имена берутся из общей части имён участников — проверено на реальных
	// StripLeadingFlagEmoji / ExtractAutoGroupName.
	wantNames := []string{"Auto NL", "Auto DE"}
	for i, g := range groups {
		if g.Name != wantNames[i] {
			t.Errorf("groups[%d].Name = %q, want %q", i, g.Name, wantNames[i])
		}
		if len(g.Members) != 2 {
			t.Errorf("groups[%d] (%q) has %d members, want 2", i, g.Name, len(g.Members))
		}
	}
	if len(individual) != 1 {
		t.Fatalf("individual=%d, want 1", len(individual))
	}
}

// Структурная стратегия должна побеждать: когда провайдер объявил
// балансировщик, флаги имён значения не имеют.
func TestSplitAutoEntriesPrefersDeclaredGroup(t *testing.T) {
	entries := []config.ProxyEntry{
		{Name: "🇳🇱 Auto NL", IP: "1.1.1.1", AutoGroup: "Pool A"},
		{Name: "🇩🇪 Auto DE", IP: "2.2.2.1", AutoGroup: "Pool A"},
		{Name: "Amsterdam #1", IP: "3.3.3.1"},
	}
	groups, individual, ok := SplitAutoEntries(entries)
	if !ok || len(groups) != 1 {
		t.Fatalf("groups=%d ok=%v, want 1 group", len(groups), ok)
	}
	if groups[0].Name != "Pool A" {
		t.Errorf("group name = %q, want %q", groups[0].Name, "Pool A")
	}
	if len(individual) != 1 {
		t.Errorf("individual=%d, want 1", len(individual))
	}
}
```

Проверить, что `config` есть в импортах `lcp_test.go`; если нет — добавить `"resultproxy-wails/internal/config"`.

- [ ] **Step 2: Запустить — первый тест должен упасть, второй пройти**

Run: `go test -tags=mobile -count=1 -run 'TestSplitAutoEntries' ./internal/proxy/`

Expected: `TestSplitAutoEntriesByNameKeepsFlagGroups` FAIL (`groups=1, want 2`), `TestSplitAutoEntriesPrefersDeclaredGroup` PASS.

- [ ] **Step 3: Переписать `splitAutoEntriesByName` на флаг-раскладку**

В `internal/proxy/autogroup.go` заменить тело функции целиком:

```go
// splitAutoEntriesByName — откат для подписок без структуры (список
// vless://-строк, где никакого объявления балансировщика нет).
//
// Записи со словом «auto»/«авто» раскладываются по ведущему флаг-эмодзи:
// провайдеры, раздающие несколько авто-секций строками, различают их именно
// флагом. Раскладка на один пул схлопывала «🇳🇱 Auto» и «🇩🇪 Auto» в одну
// группу, и пользователь терял выбор страны.
//
// Пул от двух участников: одно имя ничего не доказывает — в отличие от
// структурной стратегии, где провайдер объявил группу явно и одного члена
// достаточно.
func splitAutoEntriesByName(entries []config.ProxyEntry) ([]AutoGroup, []config.ProxyEntry, bool) {
	const noFlag = "\x00" // сентинел для записей без флага

	var order []string
	buckets := make(map[string][]config.ProxyEntry)
	individual := make([]config.ProxyEntry, 0, len(entries))

	for _, e := range entries {
		flag, base := StripLeadingFlagEmoji(e.Name)
		if !containsWordAuto(base) {
			individual = append(individual, e)
			continue
		}
		key := flag
		if key == "" {
			key = noFlag
		}
		if _, seen := buckets[key]; !seen {
			order = append(order, key)
		}
		buckets[key] = append(buckets[key], e)
	}

	var groups []AutoGroup
	for _, key := range order {
		members := buckets[key]
		if len(members) < 2 {
			individual = append(individual, members...)
			continue
		}
		name, ok := ExtractAutoGroupName(members)
		if !ok {
			individual = append(individual, members...)
			continue
		}
		groups = append(groups, AutoGroup{Name: name, Members: members})
	}

	if len(groups) == 0 {
		return nil, entries, false
	}
	return groups, individual, true
}
```

- [ ] **Step 4: Прогнать тесты**

Run: `go test -tags=mobile -count=1 ./internal/proxy/ ./mobile/`

Expected: PASS обоих новых тестов и всех прежних, включая табличные случаи в `lcp_test.go`.

- [ ] **Step 5: Коммит**

```bash
git add internal/proxy/autogroup.go internal/proxy/lcp_test.go
git commit -m "$(cat <<'EOF'
fix(auto): вернуть флаг-раскладку в откат по именам

Структурная стратегия (объявленный провайдером балансировщик) остаётся
первой. Но её откат для строчных подписок делал один пул на всё, схлопывая
«🇳🇱 Auto» и «🇩🇪 Auto» в одну группу — пользователь терял выбор страны.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Выравнивание 16 КБ в AAR

Google Play с 1 ноября 2025 отклоняет приложения с нативными библиотеками, не поддерживающими 16-килобайтный размер страницы. Замер текущего артефакта: у `jni/arm64-v8a/libgojni.so` все сегменты `LOAD` идут с `align = 0x1000` (4 КБ). AGP (8.13.2) тут ни при чём — выравнивание задаётся линковщику при сборке Go.

**Files:**
- Modify: `scripts/build-android-aar.sh:103`

**Interfaces:**
- Consumes: ничего
- Produces: `android/libs/libbox.aar` с `LOAD align = 0x4000`

- [ ] **Step 1: Зафиксировать исходное состояние замером**

```bash
SC="${TMPDIR:-/tmp}/align" && mkdir -p "$SC"
unzip -o -j android/libs/libbox.aar 'jni/arm64-v8a/libgojni.so' -d "$SC"
python - "$SC/libgojni.so" <<'PY'
import struct, sys
f = open(sys.argv[1], 'rb').read()
off  = struct.unpack_from('<Q', f, 0x20)[0]
size = struct.unpack_from('<H', f, 0x36)[0]
num  = struct.unpack_from('<H', f, 0x38)[0]
for i in range(num):
    o = off + i * size
    if struct.unpack_from('<I', f, o)[0] == 1:   # PT_LOAD
        print('LOAD align =', hex(struct.unpack_from('<Q', f, o + 48)[0]))
PY
```

Expected: четыре строки `LOAD align = 0x1000`. Это и есть блокер публикации.

- [ ] **Step 2: Добавить флаг линковки**

В `scripts/build-android-aar.sh` строку 103 заменить, сохранив комментарий выше:

```bash
# -checklinkname=0 — базовая потребность gomobile. max-page-size=16384 —
# требование Google Play с 1 ноября 2025: без него сегменты .so выходят с
# выравниванием 4 КБ и Play блокирует релиз. Проверяется замером program
# headers готового .so, а не наличием флага в скрипте.
LDFLAGS="-checklinkname=0 -extldflags=-Wl,-z,max-page-size=16384"
```

- [ ] **Step 3: Проверить версию NDK**

Флагу нужен NDK r27 или новее. Скрипт сам берёт новейший из `$ANDROID_HOME/ndk`:

```bash
ls "$(grep -E '^sdk\.dir=' android/local.properties | cut -d= -f2- | tr -d '\r')/ndk"
```

Expected: есть каталог версии 27 или выше. Если нет — поставить через SDK Manager, иначе флаг молча не даст эффекта и это вскроется только на шаге 5.

- [ ] **Step 4: Пересобрать AAR**

Run: `bash scripts/build-android-aar.sh`

Expected: сборка завершается, `android/libs/libbox.aar` обновился (проверить `ls -la android/libs/libbox.aar`). Сборка долгая — это нормально.

- [ ] **Step 5: Замерить снова — это критерий приёмки**

Повторить команду из шага 1.

Expected: четыре строки `LOAD align = 0x4000`. Если осталось `0x1000` — флаг не доехал до линковщика; проверить, что `-ldflags="${LDFLAGS}"` на строке 182 не переопределяется ниже по скрипту.

- [ ] **Step 6: Коммит**

```bash
git add scripts/build-android-aar.sh
git commit -m "$(cat <<'EOF'
build(android): собирать .so с выравниванием 16 КБ

Google Play с 1 ноября 2025 отклоняет нативные библиотеки с 4-килобайтным
выравниванием. Замер текущего артефакта давал LOAD align = 0x1000; после
-Wl,-z,max-page-size=16384 — 0x4000.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: Приёмка на устройстве

Зелёные тесты доказывают, что код собирается и ведёт себя как задумано на JVM. Они не доказывают, что провайдерская ссылка с mKCP реально поднимает туннель на телефоне. Эта задача — единственная, где проверяется то, ради чего всё делалось.

**Files:** изменений нет, только проверка

**Interfaces:**
- Consumes: AAR из задачи 6

- [ ] **Step 1: Собрать и поставить APK**

```bash
cd android && ./gradlew assembleDebug -Pdebug.abi=arm64-v8a
adb install -r -d app/build/outputs/apk/debug/app-arm64-v8a-debug.apk
```

Флаг `-r` обязателен. `adb uninstall` не использовать — сотрёт профили.

- [ ] **Step 2: Проверить ссылки, которые раньше не работали**

По одной импортировать через экран «Добавить» → вставка ссылки, подключиться, открыть любой сайт:

| Что вставить | Что доказывает |
|---|---|
| `vless://…?type=kcp&seed=…&headerType=…` | mKCP больше не откатывается на голый TCP |
| `hy2://…?mport=443-450` | port hopping доезжает как `server_ports` |
| `ss://…?plugin=obfs-local;obfs=http` | SIP003 не срезается |
| `vless://…?encryption=…` | строка VLESS Encryption доезжает |

Для каждой: соединение поднимается, в логе (экран «Логи») нет ошибки старта движка.

- [ ] **Step 3: Проверить подписку с балансировщиком**

Импортировать подписку провайдера, который объявляет xray `routing.balancers`. Ожидается: авто-секции показываются одной карточкой AUTO на секцию, а не россыпью отдельных серверов. Это и есть эффект поля `AutoGroup`.

- [ ] **Step 4: Проверить строчную подписку с флагами**

Импортировать подписку, отдающую список `vless://` с именами вида `🇳🇱 Auto …` и `🇩🇪 Auto …`. Ожидается: две отдельные карточки AUTO. Одна карточка — значит откат из задачи 5 не сработал.

- [ ] **Step 5: Проверить группировку в Kotlin**

Появление `AutoGroup` меняет то, какие группы видит UI. Прощёлкать: сортировку профилей (`ProfileSort.kt`), выбор члена AUTO при подключении (`AutoSelection.kt`), список на экране «Прокси» (`ProxiesScreen.kt`). Ожидается: переключение между членами AUTO работает, при падении члена происходит failover.

- [ ] **Step 6: Проверить предупреждение об AWG**

Импортировать `awg://`-ссылку с параметрами `j1` и `itime`. Ожидается: приложение показывает предупреждение о неподдерживаемых ключах — то самое поведение, которое возвращали на шаге 7 задачи 4.

- [ ] **Step 7: Зафиксировать результат**

Если что-то из шагов 2–6 не подтвердилось — это не «доделаем потом», а незакрытая задача. Записать, что именно не сработало, и вернуться к соответствующей задаче.

---

## Готовность блока

- [ ] `go build -tags=mobile ./internal/... ./mobile/...` — exit 0
- [ ] `go test -tags=mobile -count=1 ./internal/... ./mobile/...` — без FAIL
- [ ] `LOAD align = 0x4000` в свежем `android/libs/libbox.aar`
- [ ] Все шаги задачи 7 подтверждены на реальном устройстве

После этого блок 1 закрыт. Следующий — блок 2 (Play-комплаенс) по разделу 4 спека: флейворы `play`/`full`, targetSdk 36, `<queries>` вместо `QUERY_ALL_PACKAGES`, AAB.
