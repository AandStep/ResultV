# Профили маршрутизации, этап C — редактор и маршрутизация из подписки

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** закрыть два оставшихся пути прихода профиля — правку руками и доставку внутри подписки, — и довести экран профилей до макета ПК целиком.

**Architecture:** разбор маршрутизации подписки уже написан на этапе A (`ExtractSubscriptionRoutingLists`, `ExtractEmbeddedRoutingLists`) и никем не вызывается. Здесь появляется функция, сворачивающая их в один профиль, ответ `FetchSubscriptionV3` начинает нести ключ `routing`, а Kotlin получает редактор по макету ПК.

**Tech Stack:** Go 1.26 (теги как на этапе A), Kotlin, Jetpack Compose (Material 3).

**Spec:** `docs/superpowers/specs/2026-09-16-android-routing-profiles-design.md`
**Предыдущие этапы:** `…-stage-a.md`, `…-stage-b.md` (оба исполнены, записи в разделах 12 и 13 спеки)

## Global Constraints

- **Теги Go обязательны.** `TAGS=$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)`;
  play — те же плюс `,no_mitm,no_adblock`. Задача закрыта, только когда зелены
  обе конфигурации Go **и** обе конфигурации Kotlin.
- **Базовая линия перед этапом C:** Go — full 524, play 495; Kotlin — full 111,
  play 109; ноль падений во всех четырёх.
- **Go меняется → AAR пересобрать.** `gradlew assembleDebug` подсовывает СТАРЫЙ
  `.so`; правки Go без пересборки молча не доезжают. Оба дистрибутива:
  ```bash
  cd /c/ResultV && set -a && source .env && set +a
  DIST=full ./scripts/build-android-aar.sh && DIST=play ./scripts/build-android-aar.sh
  ```
  `.env` скрипт сам не читает — без `source` соберётся бинарь, который не
  расшифрует ни одну `resultv://`-ссылку.
- **Имена полей JSON не менять** — см. «Global Constraints» этапа B.
- **Редактор повторяет ПК** (`frontend/src/views/redesign/RoutingProfileEditor.jsx`):
  правила правятся многострочными полями по одному на строку, а не чипами;
  раздел «Стратегия» задаёт `RouteOrder`; `DomainStrategy` в редактор не
  выносится (ПК его показывает, но нигде не применяет).
- **Язык коммитов — русский.**

---

## Карта файлов

| Файл | Что делает | Задача |
|---|---|---|
| `internal/proxy/subrouting.go` | свернуть декларации и встроенные правила подписки в один профиль | 1 |
| `mobile/libbox.go` | захват заголовка и тела, ключ `routing` в ответе | 1 |
| `vpn/SubscriptionRouting.kt` | приём профиля подписки: merge, сборка, удаление вместе с подпиской | 2 |
| `vpn/DeepLinkImporter.kt`, `ui/screens/AddScreen.kt`, `vpn/SubscriptionRefresher.kt` | вызвать приём на всех трёх путях импорта | 2 |
| `ui/screens/RoutingProfileEditor.kt` | редактор по макету ПК | 3 |
| `ui/screens/RoutingProfilesScreen.kt`, `MainActivity.kt` | «Создать профиль», карандаш, маршрут редактора | 4 |

---

### Task 1: Маршрутизация подписки → один профиль

**Files:**
- Create: `internal/proxy/subrouting.go`
- Test: `internal/proxy/subrouting_test.go`
- Modify: `mobile/libbox.go` (`subscriptionFetchResult`, `fetchSubscriptionWithUA`, `fetchSubscription`, `FetchSubscriptionV3`)
- Test: `mobile/libbox_subrouting_test.go`

**Interfaces:**
- Consumes: `ExtractSubscriptionRoutingLists`, `ExtractEmbeddedRoutingLists`, `ParsedRoutingList`, `config.RoutingProfile`
- Produces: `func BuildSubscriptionRoutingProfile(subID, subName string, allowInsecure bool, headerVal, body string) (config.RoutingProfile, bool)`

- [ ] **Step 1: Написать падающий тест**

Создать `internal/proxy/subrouting_test.go`:

```go
package proxy

import (
	"encoding/base64"
	"testing"
)

func TestBuildSubscriptionRoutingProfileFromHeader(t *testing.T) {
	decl := `[{"name":"L","url":"https://panel.example/l.txt","action":"proxy"}]`
	header := base64.StdEncoding.EncodeToString([]byte(decl))

	p, ok := BuildSubscriptionRoutingProfile("sub1", "impVPN", false, header, "")
	if !ok {
		t.Fatal("профиль не собран из заголовка")
	}
	if p.Source != "subscription" || p.SubscriptionID != "sub1" {
		t.Errorf("происхождение = %q / %q", p.Source, p.SubscriptionID)
	}
	// Имя издателя — имя подписки: по нему повторная синхронизация узнаёт
	// свой профиль, а пользовательское переименование ей не мешает.
	if p.Name != "impVPN" || p.OriginName != "impVPN" {
		t.Errorf("имена = %q / %q", p.Name, p.OriginName)
	}
	if got := p.ListURLs["proxy"]; len(got) != 1 || got[0] != "https://panel.example/l.txt" {
		t.Errorf("ссылки proxy = %v", got)
	}
	// Ссылка считается за одно правило, пока её не скачали.
	if p.RuleCount("proxy") != 1 {
		t.Errorf("счётчик proxy = %d", p.RuleCount("proxy"))
	}
}

func TestBuildSubscriptionRoutingProfileFromEmbeddedXray(t *testing.T) {
	body := `{"routing":{"rules":[
		{"type":"field","outboundTag":"direct","domain":["direct.example"]},
		{"type":"field","outboundTag":"proxy","domain":["proxy.example"],"ip":["10.0.0.0/8"]},
		{"type":"field","outboundTag":"block","domain":["ads.example"]}
	]}}`
	p, ok := BuildSubscriptionRoutingProfile("sub1", "impVPN", false, "", body)
	if !ok {
		t.Fatal("профиль не собран из встроенных правил")
	}
	// Встроенные правила становятся токенами: в конфиг они влезают, а
	// ссылки на них нет — качать нечего.
	if len(p.DirectSites) == 0 || len(p.ProxySites) == 0 || len(p.BlockSites) == 0 {
		t.Errorf("токены потеряны: %v / %v / %v", p.DirectSites, p.ProxySites, p.BlockSites)
	}
	if len(p.ProxyIPs) == 0 {
		t.Errorf("подсети proxy потеряны: %v", p.ProxyIPs)
	}
	if len(p.ListURLs) != 0 {
		t.Errorf("встроенные правила стали ссылками: %v", p.ListURLs)
	}
}

func TestBuildSubscriptionRoutingProfileMergesBothSources(t *testing.T) {
	decl := `[{"name":"L","url":"https://panel.example/l.txt","action":"block"}]`
	header := base64.StdEncoding.EncodeToString([]byte(decl))
	body := `{"routing":{"rules":[{"type":"field","outboundTag":"direct","domain":["direct.example"]}]}}`

	p, ok := BuildSubscriptionRoutingProfile("sub1", "impVPN", true, header, body)
	if !ok {
		t.Fatal("профиль не собран")
	}
	if len(p.DirectSites) == 0 {
		t.Error("встроенная часть потеряна")
	}
	if len(p.ListURLs["block"]) != 1 {
		t.Error("объявленная ссылка потеряна")
	}
	// Согласие на plaintext спускается вниз к загрузкам этих ссылок.
	if !p.AllowInsecure {
		t.Error("allowInsecure не передан профилю")
	}
}

func TestBuildSubscriptionRoutingProfileEmptyWhenNothingDeclared(t *testing.T) {
	for _, tc := range []struct{ header, body string }{
		{"", ""},
		{"", "vless://11111111-1111-1111-1111-111111111111@1.2.3.4:443"},
		{"не base64", "{}"},
	} {
		if _, ok := BuildSubscriptionRoutingProfile("sub1", "S", false, tc.header, tc.body); ok {
			t.Errorf("пустая подписка дала профиль: header=%q", tc.header)
		}
	}
}

// Подписка без имени не должна давать профиль без опознавательного знака:
// по OriginName его находит следующая синхронизация.
func TestBuildSubscriptionRoutingProfileFallsBackToSubID(t *testing.T) {
	body := `{"routing":{"rules":[{"type":"field","outboundTag":"direct","domain":["a.example"]}]}}`
	p, ok := BuildSubscriptionRoutingProfile("sub1", "   ", false, "", body)
	if !ok {
		t.Fatal("профиль не собран")
	}
	if p.Name == "" || p.OriginName == "" {
		t.Errorf("имена пусты: %q / %q", p.Name, p.OriginName)
	}
}
```

- [ ] **Step 2: Запустить — должен упасть**

```bash
cd /c/ResultV && TAGS=$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)
go test -tags="$TAGS" -count=1 ./internal/proxy/ -run 'BuildSubscriptionRoutingProfile' 2>&1 | head -5
```

Ожидается: `undefined: BuildSubscriptionRoutingProfile`.

- [ ] **Step 3: Написать реализацию**

Создать `internal/proxy/subrouting.go`:

```go
// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

// Folding everything a subscription says about routing into ONE profile.
//
// A subscription used to arrive as a handful of separate routing lists, one per
// action. That was never how a user thinks about it: routing from a provider
// and routing from a link are the same thing — a set of rules that is either in
// force or not — and only one such set is in force at a time. So the provider's
// direct/proxy/block rules become one editable profile, exactly like the one a
// deep link brings. Ported from the desktop's syncSubscriptionRoutingProfile,
// minus its store.
//
// Inline rules (the provider's embedded xray routing) become tokens; rules the
// provider only links to stay links in ListURLs and are fetched at compile time
// — inlining a 74k-entry list would bloat the stored file past usefulness.

import (
	"strings"
	"time"

	"resultproxy-wails/internal/config"
)

// BuildSubscriptionRoutingProfile turns a subscription response into a profile.
//
// Returns ok=false when the provider declared no routing at all — an empty
// profile is worse than none: it would show in the list, offer to be activated,
// and route nothing.
func BuildSubscriptionRoutingProfile(
	subID, subName string,
	allowInsecure bool,
	headerVal, body string,
) (config.RoutingProfile, bool) {
	name := strings.TrimSpace(subName)
	if name == "" {
		// A profile with no handle cannot be matched on the next sync, and
		// SameRoutingProfile falls back to the displayed name. The id is not
		// pretty, but it is stable, which is what the handle is for.
		name = subID
	}
	p := config.RoutingProfile{
		Name:           name,
		OriginName:     name,
		Source:         "subscription",
		SubscriptionID: subID,
		AllowInsecure:  allowInsecure,
		UpdatedAt:      time.Now().Unix(),
		ListURLs:       map[string][]string{},
	}

	// Rules the provider linked to: kept as links.
	for _, decl := range ExtractSubscriptionRoutingLists(headerVal, body) {
		if strings.HasPrefix(decl.URL, "embedded:") {
			continue // handled below, from the body itself
		}
		p.ListURLs[decl.Action] = append(p.ListURLs[decl.Action], decl.URL)
	}

	// Rules the provider inlined into its xray config: become tokens.
	for action, parsed := range ExtractEmbeddedRoutingLists(body) {
		addSubscriptionTokens(&p, action, parsed)
	}

	if len(p.ListURLs) == 0 {
		p.ListURLs = nil
	}
	if p.RuleCount("direct")+p.RuleCount("proxy")+p.RuleCount("block") == 0 {
		return config.RoutingProfile{}, false
	}
	return p, true
}

// addSubscriptionTokens appends inline rules to the right pair of fields.
func addSubscriptionTokens(p *config.RoutingProfile, action string, parsed ParsedRoutingList) {
	// Exact hosts join the suffix list here: the profile model has one field
	// per action, and the distinction is re-derived at compile time by
	// ResolveGeoTokens, which reads a bare host as "the host and its
	// sub-domains" — the same reading the embedded-xray parser already used.
	domains := append(append([]string{}, parsed.Domains...), parsed.ExactDomains...)
	switch action {
	case "direct":
		p.DirectSites = append(p.DirectSites, domains...)
		p.DirectIPs = append(p.DirectIPs, parsed.CIDRs...)
	case "proxy":
		p.ProxySites = append(p.ProxySites, domains...)
		p.ProxyIPs = append(p.ProxyIPs, parsed.CIDRs...)
	case "block":
		p.BlockSites = append(p.BlockSites, domains...)
		p.BlockIPs = append(p.BlockIPs, parsed.CIDRs...)
	}
}
```

- [ ] **Step 4: Запустить тесты — должны пройти**

```bash
cd /c/ResultV && TAGS=$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)
go test -tags="$TAGS" -count=1 ./internal/proxy/ -run 'BuildSubscriptionRoutingProfile' -v 2>&1 | grep -E "^(--- |ok|FAIL)"
```

Ожидается: 5 PASS.

- [ ] **Step 5: Написать тест на ключ `routing` в ответе подписки**

Создать `mobile/libbox_subrouting_test.go`:

```go
package mobile

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Ответ подписки несёт маршрутизацию двумя каналами; оба должны доехать до
// Kotlin одним ключом, а не потеряться между заголовком и телом.
func TestFetchSubscriptionCarriesRouting(t *testing.T) {
	decl := `[{"name":"L","url":"https://panel.example/l.txt","action":"proxy"}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Routing-Lists", base64.StdEncoding.EncodeToString([]byte(decl)))
		w.Header().Set("Profile-Title", "impVPN")
		w.Write([]byte("vless://11111111-1111-1111-1111-111111111111@1.2.3.4:443?security=none&type=tcp#n\n"))
	}))
	defer srv.Close()

	out, err := FetchSubscriptionV3(srv.URL, t.TempDir(), "{}")
	if err != nil {
		t.Fatalf("FetchSubscriptionV3: %v", err)
	}
	var res struct {
		Entries []map[string]any `json:"entries"`
		Routing string           `json:"routing"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("ответ не JSON: %v", err)
	}
	if len(res.Entries) == 0 {
		t.Fatal("серверы потеряны")
	}
	if res.Routing == "" {
		t.Fatal("ключа routing нет")
	}
	var p map[string]any
	if err := json.Unmarshal([]byte(res.Routing), &p); err != nil {
		t.Fatalf("routing не JSON: %v", err)
	}
	if p["source"] != "subscription" {
		t.Errorf("source = %v", p["source"])
	}
}

// Подписка без маршрутизации не должна давать пустой профиль: он показался бы
// в списке, предложил себя включить и ничего бы не маршрутизировал.
func TestFetchSubscriptionWithoutRoutingLeavesKeyEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("vless://11111111-1111-1111-1111-111111111111@1.2.3.4:443?security=none&type=tcp#n\n"))
	}))
	defer srv.Close()

	out, err := FetchSubscriptionV3(srv.URL, t.TempDir(), "{}")
	if err != nil {
		t.Fatalf("FetchSubscriptionV3: %v", err)
	}
	var res struct {
		Routing string `json:"routing"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("ответ не JSON: %v", err)
	}
	if res.Routing != "" {
		t.Errorf("routing = %q, ждали пусто", res.Routing)
	}
}
```

- [ ] **Step 6: Протащить заголовок и тело до результата**

В `mobile/libbox.go`:

1. В `subscriptionFetchResult` (строка 641) добавить два поля:

```go
	// RoutingHeader и Body нужны разбору маршрутизации: провайдер объявляет её
	// либо заголовком Routing-Lists, либо ключом routingLists в JSON-теле,
	// либо встроенными xray-правилами прямо в конфигах. Тело здесь уже
	// прочитано — второй запрос за тем же ответом был бы лишним.
	RoutingHeader string
	Body          string
```

2. В `fetchSubscriptionWithUA`, в успешный `return` (около строки 723), добавить:

```go
		RoutingHeader: resp.Header.Get("Routing-Lists"),
		Body:          string(body),
```

3. В `subscriptionResult` (строка ~576) добавить `Routing string`.

4. В `fetchSubscription`, в финальный `return`, добавить:

```go
	// Маршрутизация провайдера сворачивается в один профиль здесь, а не на
	// стороне Kotlin: разбор обоих каналов уже написан на Go, и вторая его
	// копия разъехалась бы с первой.
	routing := ""
	if p, ok := proxy.BuildSubscriptionRoutingProfile(
		"", primaryRes.Title, false, primaryRes.RoutingHeader, primaryRes.Body,
	); ok {
		if blob, merr := json.Marshal(p); merr == nil {
			routing = string(blob)
		}
	}
```
и `Routing: routing` в возвращаемую структуру.

**`subID` пустой не случайно:** идентификатор подписки знает только Kotlin, и
он проставляет его сам перед merge (задача 2). Здесь его подделывать нечем.

5. В `FetchSubscriptionV3`, туда же, где кладутся `title` / `userInfo`, добавить
   `.put("routing", res.Routing)` — точную форму взять из соседних строк.

- [ ] **Step 7: Запустить тесты**

```bash
cd /c/ResultV && TAGS=$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)
go test -tags="$TAGS" -count=1 ./mobile/ -run 'FetchSubscription' -v 2>&1 | grep -E "^(--- |ok|FAIL)"
```

Ожидается: оба новых PASS, старые тесты подписки не сломаны.

- [ ] **Step 8: Обе конфигурации Go целиком**

```bash
cd /c/ResultV && TAGS=$(tr -d ' \t\r\n' < scripts/android-build-tags.txt)
go build -tags="$TAGS" ./... && go test -tags="$TAGS" -count=1 ./internal/proxy/... ./mobile/...
go build -tags="$TAGS,no_mitm,no_adblock" ./... && go test -tags="$TAGS,no_mitm,no_adblock" -count=1 ./internal/proxy/... ./mobile/...
```

- [ ] **Step 9: Пересобрать оба AAR**

```bash
cd /c/ResultV && set -a && source .env && set +a
DIST=full ./scripts/build-android-aar.sh && DIST=play ./scripts/build-android-aar.sh
```

Без этого правки Go молча не доедут до приложения.

- [ ] **Step 10: Коммит**

```bash
cd /c/ResultV
git add internal/proxy/subrouting.go internal/proxy/subrouting_test.go \
        mobile/libbox.go mobile/libbox_subrouting_test.go
git commit -m "feat(routing): свернуть маршрутизацию подписки в один профиль

Разбор обоих каналов написан на этапе A и до сих пор никем не вызывался.
Теперь ответ подписки несёт ключ routing — готовый профиль, который Kotlin
только сливает с хранилищем.

Подписка без маршрутизации даёт пустой ключ, а не пустой профиль: второй
показался бы в списке, предложил себя включить и не маршрутизировал бы
ничего.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 2: Приём профиля подписки на стороне Kotlin

**Files:**
- Create: `android/app/src/main/java/com/resultv/android/vpn/SubscriptionRouting.kt`
- Modify: `vpn/DeepLinkImporter.kt`, `ui/screens/AddScreen.kt`, `vpn/SubscriptionRefresher.kt`, `vpn/Subscription.kt`
- Test: `android/app/src/test/java/com/resultv/android/vpn/SubscriptionRoutingTest.kt`

**Interfaces:**
- Consumes: `RoutingProfile`, `routingProfileFromJson`, `RoutingProfileRepository`, `RoutingProfileCompiler`, `Mobile.mergeRoutingProfile`
- Produces: `object SubscriptionRouting` с `suspend fun accept(routingJson: String, subId: String, subName: String, dataDir: String, activate: Boolean)` и `fun forget(subId: String, dataDir: String)`, `fun tagSubscriptionProfile(json: String, subId: String, subName: String): RoutingProfile?`

- [ ] **Step 1: Написать падающий тест**

Создать `android/app/src/test/java/com/resultv/android/vpn/SubscriptionRoutingTest.kt`:

```kotlin
package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class SubscriptionRoutingTest {

    // Go отдаёт профиль без subscriptionId: его знает только Kotlin.
    // Без простановки следующая синхронизация не узнает свой профиль.
    @Test fun tagsProfileWithSubscriptionId() {
        val p = tagSubscriptionProfile(
            """{"name":"impVPN","source":"subscription","proxySites":["a.example"]}""",
            subId = "sub1",
            subName = "impVPN",
        )!!
        assertEquals("sub1", p.subscriptionId)
        assertEquals("subscription", p.source)
    }

    // Имя подписки пользователь мог поменять, а OriginName — опознавательный
    // знак, и он должен остаться тем, что прислал провайдер.
    @Test fun keepsOriginNameFromGo() {
        val p = tagSubscriptionProfile(
            """{"name":"impVPN","originName":"impVPN","source":"subscription","proxySites":["a.example"]}""",
            subId = "sub1",
            subName = "Моя подписка",
        )!!
        assertEquals("impVPN", p.originName)
    }

    // Профиль без опознавательного знака дополняется именем подписки: иначе
    // SameRoutingProfile не с чем сравнивать.
    @Test fun fillsOriginNameWhenGoLeftItEmpty() {
        val p = tagSubscriptionProfile(
            """{"name":"","source":"subscription","proxySites":["a.example"]}""",
            subId = "sub1",
            subName = "impVPN",
        )!!
        assertEquals("impVPN", p.originName)
        assertEquals("impVPN", p.name)
    }

    @Test fun emptyOrBrokenJsonIsNull() {
        assertNull(tagSubscriptionProfile("", "sub1", "S"))
        assertNull(tagSubscriptionProfile("   ", "sub1", "S"))
        assertNull(tagSubscriptionProfile("не json", "sub1", "S"))
    }

    // Профиль без единого правила принимать нельзя: он занял бы строку в
    // списке и ничего бы не маршрутизировал.
    @Test fun profileWithoutRulesIsNull() {
        assertNull(tagSubscriptionProfile("""{"name":"S","source":"subscription"}""", "sub1", "S"))
    }
}
```

- [ ] **Step 2: Запустить — должен упасть**

```bash
cd /c/ResultV/android && ./gradlew :app:testFullDebugUnitTest --tests "*SubscriptionRoutingTest" --console=plain 2>&1 | tail -6
```

Ожидается: `Unresolved reference: tagSubscriptionProfile`.

- [ ] **Step 3: Написать реализацию**

Создать `android/app/src/main/java/com/resultv/android/vpn/SubscriptionRouting.kt`:

```kotlin
package com.resultv.android.vpn

import android.util.Log
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import mobile.Mobile
import org.json.JSONObject

private const val TAG = "ResultV/SubRouting"

/**
 * Проставить профилю подписки то, чего Go знать не может.
 *
 * `subscriptionId` живёт только на стороне Kotlin, а `originName` — тот
 * опознавательный знак, по которому следующая синхронизация находит свой
 * профиль. Возвращает `null`, если принимать нечего: профиль без единого
 * правила занял бы строку в списке и ничего бы не маршрутизировал.
 */
fun tagSubscriptionProfile(json: String, subId: String, subName: String): RoutingProfile? {
    val raw = json.trim()
    if (raw.isEmpty()) return null
    val parsed = runCatching { routingProfileFromJson(JSONObject(raw)) }.getOrNull() ?: return null
    val handle = parsed.originName.ifBlank { parsed.name }.ifBlank { subName.trim() }
    val tagged = parsed.copy(
        name = parsed.name.ifBlank { handle },
        originName = handle,
        source = "subscription",
        subscriptionId = subId,
    )
    val total = ROUTING_ACTIONS.sumOf { tagged.ruleCount(it) }
    return if (total == 0) null else tagged
}

/**
 * Приём маршрутизации, пришедшей внутри подписки.
 *
 * `activate` — только когда пользователь сам согласился на эту подписку.
 * Фоновое обновление профиль обновляет, но активным не делает: иначе оно
 * перебивало бы выбор, сделанный с тех пор.
 */
object SubscriptionRouting {

    suspend fun accept(
        routingJson: String,
        subId: String,
        subName: String,
        dataDir: String,
        activate: Boolean,
    ) {
        val incoming = tagSubscriptionProfile(routingJson, subId, subName)
        if (incoming == null) {
            // Провайдер перестал присылать маршрутизацию — держать прежний
            // профиль значило бы маршрутизировать по правилам, которых больше
            // нет.
            forget(subId, dataDir)
            return
        }
        val merged = withContext(Dispatchers.IO) {
            runCatching {
                Mobile.mergeRoutingProfile(
                    RoutingProfileRepository.storeJson(),
                    incoming.toJson().toString(),
                    activate,
                )
            }.getOrNull()
        }
        val state = merged?.let { parseRoutingMergeResult(it) }
        if (state == null) {
            Log.w(TAG, "merge failed for subscription $subId")
            return
        }
        RoutingProfileRepository.replaceAll(state.profiles, state.activeId)
        val saved = state.profiles.firstOrNull {
            it.source == "subscription" && it.subscriptionId == subId
        } ?: return
        RoutingProfileCompiler.compile(saved, dataDir)
    }

    /** Убрать профиль подписки вместе с её кэшем правил. */
    fun forget(subId: String, dataDir: String) {
        val victim = RoutingProfileRepository.state.value.profiles.firstOrNull {
            it.source == "subscription" && it.subscriptionId == subId
        } ?: return
        RoutingProfileCompiler.forget(dataDir, victim.id)
        RoutingProfileRepository.delete(victim.id)
    }
}
```

- [ ] **Step 4: Вызвать приём на всех трёх путях**

Найти места:

```bash
cd /c/ResultV && grep -rn "replaceForSubscription\|SubscriptionRepository.upsert\|SubscriptionRepository.delete" --include=*.kt android/app/src/main/
```

В каждом из них, после `ProfileRepository.replaceForSubscription(...)`:

```kotlin
        // Маршрутизация провайдера приходит тем же ответом. activate=true
        // только здесь: пользователь только что сам согласился на эту
        // подписку. Фоновое обновление зовёт то же самое с false.
        SubscriptionRouting.accept(
            routingJson = response.optString("routing"),
            subId = subId,
            subName = title.ifBlank { subUrl },
            dataDir = dataDir,
            activate = true,
        )
```

В `SubscriptionRefresher` — то же с `activate = false`.
В каскадном удалении (`SubscriptionRepository.delete`) — `SubscriptionRouting.forget(id, dataDir)` до удаления записи.

**Осторожно с `SubscriptionRepository.delete`:** он `@Synchronized` и не
`suspend`, а `forget` синхронный — это подходит. `dataDir` туда придётся
передать параметром либо запомнить в `init`; выбрать по месту, но НЕ звать
`Context` из репозитория, который его не держит.

- [ ] **Step 5: Прогнать обе конфигурации Kotlin**

```bash
cd /c/ResultV/android && ./gradlew :app:testFullDebugUnitTest :app:testPlayDebugUnitTest \
    :app:assembleFullDebug :app:assemblePlayDebug --console=plain 2>&1 | tail -6
```

Ожидается: BUILD SUCCESSFUL, счёт full 116, play 114.

- [ ] **Step 6: Коммит**

```bash
cd /c/ResultV
git add android/app/src/main/java/com/resultv/android/vpn/SubscriptionRouting.kt \
        android/app/src/test/java/com/resultv/android/vpn/SubscriptionRoutingTest.kt \
        android/app/src/main/java/com/resultv/android/vpn/DeepLinkImporter.kt \
        android/app/src/main/java/com/resultv/android/vpn/SubscriptionRefresher.kt \
        android/app/src/main/java/com/resultv/android/vpn/Subscription.kt \
        android/app/src/main/java/com/resultv/android/ui/screens/AddScreen.kt
git commit -m "feat(routing): принимать маршрутизацию, пришедшую с подпиской

Активируется только когда пользователь сам согласился на подписку; фоновое
обновление профиль обновляет, но активным не делает — иначе оно перебивало бы
выбор, сделанный с тех пор.

Провайдер, переставший присылать маршрутизацию, уносит свой профиль: держать
прежний значило бы маршрутизировать по правилам, которых больше нет.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 3: Редактор профиля

**Files:**
- Create: `android/app/src/main/java/com/resultv/android/ui/screens/RoutingProfileEditor.kt`
- Test: `android/app/src/test/java/com/resultv/android/vpn/RoutingEditorFormTest.kt`

**Interfaces:**
- Consumes: `RoutingProfile`, `ROUTING_ACTIONS`
- Produces: `fun routingLinesOf(list: List<String>): String`, `fun routingTokensOf(text: String): List<String>`, `fun normalizeRouteOrder(order: List<String>): String`, `@Composable fun RoutingProfileEditorScreen(profile: RoutingProfile?, busy: Boolean, onSave: (RoutingProfile) -> Unit, onClose: () -> Unit)`

- [ ] **Step 1: Написать падающий тест**

Создать `android/app/src/test/java/com/resultv/android/vpn/RoutingEditorFormTest.kt`:

```kotlin
package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class RoutingEditorFormTest {

    // Правила правятся текстом, одно правило на строку — как на ПК
    // (linesOf / tokensOf в RoutingProfileEditor.jsx).
    @Test fun linesRoundTrip() {
        val list = listOf("geosite:private", "example.com", "domain:nalog.ru")
        assertEquals(list, routingTokensOf(routingLinesOf(list)))
    }

    @Test fun tokensIgnoreBlankAndWhitespace() {
        val text = "  example.com  \n\n\t\n  10.0.0.0/8\n"
        assertEquals(listOf("example.com", "10.0.0.0/8"), routingTokensOf(text))
    }

    @Test fun emptyTextGivesEmptyList() {
        assertTrue(routingTokensOf("").isEmpty())
        assertTrue(routingTokensOf("   \n  \n").isEmpty())
    }

    @Test fun linesOfEmptyListIsEmptyText() {
        assertEquals("", routingLinesOf(emptyList()))
    }

    // Порядок правил — три метки через дефис, ровно как ждёт Go
    // (NormalizeRoutingOrder).
    @Test fun orderJoinsWithDashes() {
        assertEquals("block-proxy-direct", normalizeRouteOrder(listOf("block", "proxy", "direct")))
        assertEquals("direct-proxy-block", normalizeRouteOrder(listOf("direct", "proxy", "block")))
    }

    // Неполный или повторяющийся набор — не порядок. Пустая строка означает
    // «умолчание», и Go подставит своё, а не будет гадать.
    @Test fun brokenOrderBecomesEmpty() {
        assertEquals("", normalizeRouteOrder(listOf("block", "block", "direct")))
        assertEquals("", normalizeRouteOrder(listOf("block", "proxy")))
        assertEquals("", normalizeRouteOrder(emptyList()))
        assertEquals("", normalizeRouteOrder(listOf("block", "proxy", "чепуха")))
    }
}
```

- [ ] **Step 2: Запустить — должен упасть**

```bash
cd /c/ResultV/android && ./gradlew :app:testFullDebugUnitTest --tests "*RoutingEditorFormTest" --console=plain 2>&1 | tail -6
```

Ожидается: `Unresolved reference: routingTokensOf`.

- [ ] **Step 3: Написать чистую часть формы**

Дописать в `android/app/src/main/java/com/resultv/android/vpn/RoutingProfiles.kt`:

```kotlin
/**
 * Список правил как текст для редактора: одно правило на строку.
 *
 * Так правит ПК (`RoutingProfileEditor.jsx:242-247`), и это же снимает вопрос
 * двадцати тысяч токенов: многострочное поле — один элемент, а не 20 000 чипов.
 */
fun routingLinesOf(list: List<String>): String = list.joinToString("\n")

/** Обратно: непустые строки без окружающих пробелов. */
fun routingTokensOf(text: String): List<String> =
    text.split("\n").map { it.trim() }.filter { it.isNotEmpty() }

/**
 * Порядок разбора правил в форме, которую ждёт Go (`NormalizeRoutingOrder`).
 *
 * Неполный или повторяющийся набор даёт пустую строку — «умолчание»: порядок
 * решает, какое правило выигрывает при нескольких совпадениях, и половинчатое
 * значение там опаснее отсутствующего.
 */
fun normalizeRouteOrder(order: List<String>): String {
    if (order.size != ROUTING_ACTIONS.size) return ""
    if (order.toSet() != ROUTING_ACTIONS.toSet()) return ""
    return order.joinToString("-")
}
```

- [ ] **Step 4: Запустить тесты — должны пройти**

```bash
cd /c/ResultV/android && ./gradlew :app:testFullDebugUnitTest --tests "*RoutingEditorFormTest" --console=plain 2>&1 | tail -4
```

Ожидается: BUILD SUCCESSFUL, 6 тестов.

- [ ] **Step 5: Написать экран редактора**

Создать `android/app/src/main/java/com/resultv/android/ui/screens/RoutingProfileEditor.kt`.

Раскладка повторяет окно ПК (`RoutingProfileEditor.jsx:314-410`), метрики берутся
те же, что у экрана списка (задача 5 этапа B): разделы с зазором 16, подписи
белым 50 %, скругление 24, отступы 16.

```kotlin
package com.resultv.android.ui.screens

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.ExpandLess
import androidx.compose.material.icons.outlined.ExpandMore
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.resultv.android.R
import com.resultv.android.theme.Brand
import com.resultv.android.ui.components.SettingIcon
import com.resultv.android.vpn.ROUTING_ACTIONS
import com.resultv.android.vpn.RoutingProfile
import com.resultv.android.vpn.normalizeRouteOrder
import com.resultv.android.vpn.routingLinesOf
import com.resultv.android.vpn.routingTokensOf

/*
 * Правка профиля. Повторяет окно ПК (RoutingProfileEditor.jsx, Figma 6648:4105
 * «Добавление профиля» и 6636:4310 «Изменение профиля»): одно окно в двух
 * ролях, отличаются заголовок, подпись и значок.
 *
 * Внутри: название, три раздела действий со складными панелями «Домены» и
 * «IP адреса», «Стратегия», «Geo данные», «Действия».
 *
 * Правила — многострочный текст, одно правило на строку. Чипы здесь не годятся:
 * профиль может нести до 20 000 токенов, и это 20 000 элементов вместо одного.
 */

private val EditorShape = RoundedCornerShape(24.dp)
private val EditorFill = Brand.Green.copy(alpha = 0.10f)
private val EditorBorder = Color.White.copy(alpha = 0.10f)
private val EditorLabel = Color.White.copy(alpha = 0.50f)

private fun actionLabel(action: String): Int = when (action) {
    "direct" -> R.string.routing_editor_direct
    "proxy" -> R.string.routing_editor_proxy
    else -> R.string.routing_editor_block
}

private fun actionColor(action: String): Color = when (action) {
    "direct" -> Brand.Green
    "proxy" -> Brand.GreenLight
    else -> Brand.Danger
}

@Composable
fun RoutingProfileEditorScreen(
    profile: RoutingProfile?,
    busy: Boolean,
    onSave: (RoutingProfile) -> Unit,
    onClose: () -> Unit,
) {
    val isEdit = profile != null && profile.id.isNotEmpty()
    var name by remember(profile) { mutableStateOf(profile?.name.orEmpty()) }
    var geoip by remember(profile) { mutableStateOf(profile?.geoipUrl.orEmpty()) }
    var geosite by remember(profile) { mutableStateOf(profile?.geositeUrl.orEmpty()) }

    // Ключи полей: "<действие>-sites" и "<действие>-ips", как на ПК.
    val fields = remember(profile) {
        mutableStateMapOf<String, String>().apply {
            put("direct-sites", routingLinesOf(profile?.directSites.orEmpty()))
            put("direct-ips", routingLinesOf(profile?.directIp.orEmpty()))
            put("proxy-sites", routingLinesOf(profile?.proxySites.orEmpty()))
            put("proxy-ips", routingLinesOf(profile?.proxyIp.orEmpty()))
            put("block-sites", routingLinesOf(profile?.blockSites.orEmpty()))
            put("block-ips", routingLinesOf(profile?.blockIp.orEmpty()))
        }
    }
    // В макете раскрыта первая панель Direct, остальные свёрнуты.
    val opened = remember(profile) { mutableStateMapOf("direct-sites" to true) }
    val order = remember(profile) {
        mutableStateListOf<String>().apply {
            val stored = profile?.routeOrder.orEmpty().split("-").filter { it.isNotBlank() }
            addAll(if (stored.toSet() == ROUTING_ACTIONS.toSet()) stored else listOf("block", "proxy", "direct"))
        }
    }

    val empty = fields.values.all { routingTokensOf(it).isEmpty() }
    val canSave = !busy && name.isNotBlank() && !empty

    Scaffold(containerColor = Brand.Bg) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp, vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(14.dp),
            ) {
                SettingIcon(
                    icon = if (isEdit) Icons.Outlined.Edit else Icons.Outlined.Add,
                    bg = EditorFill,
                    tint = Brand.Green,
                )
                Column(modifier = Modifier.weight(1f)) {
                    Text(
                        stringResource(
                            if (isEdit) R.string.routing_editor_edit_title
                            else R.string.routing_editor_create_title
                        ),
                        style = MaterialTheme.typography.titleLarge,
                        fontWeight = FontWeight.Bold,
                    )
                    Text(
                        stringResource(
                            if (isEdit) R.string.routing_editor_edit_subtitle
                            else R.string.routing_editor_create_subtitle
                        ),
                        style = MaterialTheme.typography.bodyMedium,
                        color = Brand.SecondaryText,
                    )
                }
                IconButton(onClick = onClose) {
                    Icon(
                        Icons.Outlined.Close,
                        contentDescription = stringResource(R.string.action_close),
                        tint = EditorLabel,
                    )
                }
            }

            EditorSection(stringResource(R.string.routing_editor_name)) {
                OutlinedTextField(
                    value = name,
                    onValueChange = { name = it },
                    placeholder = { Text(stringResource(R.string.routing_editor_name_hint)) },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
            }

            ROUTING_ACTIONS.forEach { action ->
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(
                        stringResource(actionLabel(action)),
                        fontSize = 14.sp,
                        fontWeight = FontWeight.Medium,
                        color = actionColor(action),
                    )
                    RulePanel(
                        label = stringResource(R.string.routing_editor_domains),
                        hint = stringResource(R.string.routing_editor_domains_hint),
                        value = fields["$action-sites"].orEmpty(),
                        onChange = { fields["$action-sites"] = it },
                        open = opened["$action-sites"] == true,
                        onToggle = { opened["$action-sites"] = opened["$action-sites"] != true },
                    )
                    RulePanel(
                        label = stringResource(R.string.routing_editor_ips),
                        hint = stringResource(R.string.routing_editor_ips_hint),
                        value = fields["$action-ips"].orEmpty(),
                        onChange = { fields["$action-ips"] = it },
                        open = opened["$action-ips"] == true,
                        onToggle = { opened["$action-ips"] = opened["$action-ips"] != true },
                    )
                }
            }

            EditorSection(stringResource(R.string.routing_editor_strategy)) {
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    order.forEach { action ->
                        Text(
                            action,
                            fontSize = 14.sp,
                            fontWeight = FontWeight.Medium,
                            color = actionColor(action),
                            modifier = Modifier
                                .clip(RoundedCornerShape(100.dp))
                                .background(Brand.SurfaceHigh)
                                // Перетаскивание ради трёх элементов — лишняя
                                // механика на телефоне. Нажатие отправляет метку
                                // в конец; порядок меняется теми же тремя
                                // движениями, а попасть проще.
                                .clickable(enabled = !busy) {
                                    order.remove(action)
                                    order.add(action)
                                }
                                .padding(horizontal = 12.dp, vertical = 8.dp),
                        )
                    }
                }
                Text(
                    stringResource(R.string.routing_editor_strategy_hint),
                    fontSize = 14.sp,
                    color = EditorLabel,
                )
            }

            EditorSection(stringResource(R.string.routing_editor_geo)) {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(EditorShape)
                        .border(1.dp, EditorBorder, EditorShape)
                        .padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Text(stringResource(R.string.routing_editor_geoip), fontSize = 14.sp, color = EditorLabel)
                    OutlinedTextField(
                        value = geoip,
                        onValueChange = { geoip = it },
                        placeholder = { Text("https://example.com") },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth(),
                    )
                    Text(stringResource(R.string.routing_editor_geosite), fontSize = 14.sp, color = EditorLabel)
                    OutlinedTextField(
                        value = geosite,
                        onValueChange = { geosite = it },
                        placeholder = { Text("https://example.com") },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth(),
                    )
                }
            }

            EditorSection(stringResource(R.string.routing_profiles_actions)) {
                Button(
                    onClick = {
                        val base = profile ?: RoutingProfile(id = "", name = "", source = "manual")
                        onSave(
                            base.copy(
                                name = name.trim(),
                                directSites = routingTokensOf(fields["direct-sites"].orEmpty()),
                                directIp = routingTokensOf(fields["direct-ips"].orEmpty()),
                                proxySites = routingTokensOf(fields["proxy-sites"].orEmpty()),
                                proxyIp = routingTokensOf(fields["proxy-ips"].orEmpty()),
                                blockSites = routingTokensOf(fields["block-sites"].orEmpty()),
                                blockIp = routingTokensOf(fields["block-ips"].orEmpty()),
                                routeOrder = normalizeRouteOrder(order.toList()),
                                geoipUrl = geoip.trim(),
                                geositeUrl = geosite.trim(),
                                lastError = "",
                            )
                        )
                    },
                    // Профиль без имени или без единого правила сохранять
                    // нечего — Go его всё равно отклонит, и лучше это видно до
                    // нажатия.
                    enabled = canSave,
                    modifier = Modifier.fillMaxWidth().height(56.dp),
                    shape = EditorShape,
                    colors = ButtonDefaults.buttonColors(
                        containerColor = EditorFill,
                        contentColor = Brand.Green,
                    ),
                ) {
                    Text(
                        stringResource(R.string.action_save),
                        fontSize = 16.sp,
                        fontWeight = FontWeight.Bold,
                    )
                }
            }
        }
    }
}

@Composable
private fun EditorSection(label: String, content: @Composable () -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(label, fontSize = 14.sp, color = EditorLabel)
        content()
    }
}

/** Складная панель со списком правил (Figma 6648:4159). */
@Composable
private fun RulePanel(
    label: String,
    hint: String,
    value: String,
    onChange: (String) -> Unit,
    open: Boolean,
    onToggle: () -> Unit,
) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(EditorShape)
            .border(1.dp, EditorBorder, EditorShape),
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .clickable(onClick = onToggle)
                .padding(horizontal = 16.dp, vertical = 14.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(label, fontSize = 14.sp, modifier = Modifier.weight(1f))
            Icon(
                if (open) Icons.Outlined.ExpandLess else Icons.Outlined.ExpandMore,
                contentDescription = null,
                tint = EditorLabel,
            )
        }
        AnimatedVisibility(visible = open) {
            OutlinedTextField(
                value = value,
                onValueChange = onChange,
                placeholder = { Text(hint) },
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(min = 120.dp)
                    .padding(horizontal = 16.dp)
                    .padding(bottom = 16.dp),
            )
        }
    }
}
```

- [ ] **Step 6: Добавить строки**

`values/strings.xml`:

```xml
<string name="routing_editor_create_title">Add a profile</string>
<string name="routing_editor_create_subtitle">Create a new routing profile</string>
<string name="routing_editor_edit_title">Edit profile</string>
<string name="routing_editor_edit_subtitle">Editing the profile</string>
<string name="routing_editor_name">Name</string>
<string name="routing_editor_name_hint">My rules</string>
<string name="routing_editor_direct">Direct</string>
<string name="routing_editor_proxy">Proxy</string>
<string name="routing_editor_block">Block</string>
<string name="routing_editor_domains">Domains</string>
<string name="routing_editor_ips">IP addresses</string>
<string name="routing_editor_domains_hint">geosite:private\nexample.com\ndomain:nalog.ru</string>
<string name="routing_editor_ips_hint">geoip:private\n10.0.0.0/8</string>
<string name="routing_editor_strategy">Strategy</string>
<string name="routing_editor_strategy_hint">Rule order — tap a label to move it last</string>
<string name="routing_editor_geo">Geo data</string>
<string name="routing_editor_geoip">GeoIP URL</string>
<string name="routing_editor_geosite">GeoSite URL</string>
```

`values-ru/strings.xml`:

```xml
<string name="routing_editor_create_title">Добавление профиля</string>
<string name="routing_editor_create_subtitle">Создание нового профиля маршрутизации</string>
<string name="routing_editor_edit_title">Изменение профиля</string>
<string name="routing_editor_edit_subtitle">Редактирование профиля</string>
<string name="routing_editor_name">Название</string>
<string name="routing_editor_name_hint">Мои правила</string>
<string name="routing_editor_direct">Direct (напрямую)</string>
<string name="routing_editor_proxy">Proxy (через прокси)</string>
<string name="routing_editor_block">Block (блокировать)</string>
<string name="routing_editor_domains">Домены</string>
<string name="routing_editor_ips">IP адреса</string>
<string name="routing_editor_domains_hint">geosite:private\nexample.com\ndomain:nalog.ru</string>
<string name="routing_editor_ips_hint">geoip:private\n10.0.0.0/8</string>
<string name="routing_editor_strategy">Стратегия</string>
<string name="routing_editor_strategy_hint">Порядок разбора правил — нажмите метку, чтобы отправить её в конец</string>
<string name="routing_editor_geo">Geo данные</string>
<string name="routing_editor_geoip">URL GeoIP</string>
<string name="routing_editor_geosite">URL GeoSite</string>
```

- [ ] **Step 7: Собрать и прогнать**

```bash
cd /c/ResultV/android && ./gradlew :app:testFullDebugUnitTest :app:testPlayDebugUnitTest \
    :app:assembleFullDebug :app:assemblePlayDebug --console=plain 2>&1 | tail -6
```

Ожидается: BUILD SUCCESSFUL, счёт full 122, play 120.

- [ ] **Step 8: Коммит**

```bash
cd /c/ResultV
git add android/app/src/main/java/com/resultv/android/ui/screens/RoutingProfileEditor.kt \
        android/app/src/main/java/com/resultv/android/vpn/RoutingProfiles.kt \
        android/app/src/test/java/com/resultv/android/vpn/RoutingEditorFormTest.kt \
        android/app/src/main/res/values/strings.xml \
        android/app/src/main/res/values-ru/strings.xml
git commit -m "feat(routing): редактор профиля по макету ПК

Одно окно в двух ролях, как на ПК: отличаются заголовок, подпись и значок.
Внутри — название, три раздела действий со складными панелями «Домены» и
«IP адреса», «Стратегия», «Geo данные», «Действия». Раскрыта первая панель.

Правила правятся многострочным текстом, одно правило на строку, а не чипами:
профиль может нести до 20 000 токенов, и это 20 000 элементов вместо одного.
Так же на ПК — linesOf / tokensOf.

Порядок правил меняется нажатием на метку: перетаскивание ради трёх
элементов — лишняя механика на телефоне, а попадает палец хуже. Неполный
набор даёт пустую строку, то есть умолчание Go, а не половинчатый порядок.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 4: Подключить редактор к экрану профилей

**Files:**
- Modify: `ui/screens/RoutingProfilesScreen.kt`, `MainActivity.kt`

- [ ] **Step 1: Карандаш в строке и кнопка «Создать профиль»**

В `RoutingProfilesScreen.kt`:

1. `RoutingProfilesScreen` получает `onEdit: (RoutingProfile?) -> Unit`.
2. `ProfileRow` получает `onEdit: (() -> Unit)?`; карандаш рисуется только
   когда он не `null`:

```kotlin
        if (onEdit != null) {
            IconButton(onClick = onEdit, enabled = !busy) {
                Icon(
                    Icons.Outlined.Edit,
                    contentDescription = stringResource(R.string.routing_editor_edit_title),
                    tint = LabelColor,
                )
            }
        }
```

3. Профилю из подписки карандаш **не даётся**: его правила приходят готовыми и
   перезаписываются следующей синхронизацией, так что правка обещала бы то, что
   не переживёт обновления. Это правило ПК (`ProfileItem.jsx`: «Правка есть не у
   всякой строки»). То есть `onEdit = if (profile.source == "subscription") null
   else { { onEdit(profile) } }`.

4. В разделе «Действия» — вторая кнопка, слева от импорта, как в макете:

```kotlin
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Button(
                        onClick = { onEdit(null) },
                        modifier = Modifier.weight(1f).height(56.dp),
                        shape = RowShape,
                        colors = ButtonDefaults.buttonColors(
                            containerColor = Brand.Surface,
                            contentColor = Color.White,
                        ),
                    ) { Text(stringResource(R.string.routing_profiles_create), fontSize = 16.sp, fontWeight = FontWeight.Bold) }
                    Button(
                        onClick = { showImport = true },
                        modifier = Modifier.weight(1f).height(56.dp),
                        shape = RowShape,
                        colors = ButtonDefaults.buttonColors(
                            containerColor = ActiveFill,
                            contentColor = Brand.Green,
                        ),
                    ) { Text(stringResource(R.string.routing_profiles_import), fontSize = 16.sp, fontWeight = FontWeight.Bold) }
                }
```

- [ ] **Step 2: Маршрут редактора в `MainActivity`**

Рядом с `showRoutingProfiles`:

```kotlin
    // null — редактор закрыт; профиль — правка; RoutingProfile пустышка с
    // пустым id — создание. Отдельный флаг «создаём» не нужен: пустой id и
    // есть признак нового профиля, тот же, по которому его различает Go.
    var editingProfile by remember { mutableStateOf<RoutingProfile?>(null) }
    var editorOpen by remember { mutableStateOf(false) }
    var editorBusy by remember { mutableStateOf(false) }
```

и рендер поверх экрана списка:

```kotlin
    if (editorOpen) {
        BackHandler { if (!editorBusy) editorOpen = false }
        val scope = rememberCoroutineScope()
        val ctx = LocalContext.current
        RoutingProfileEditorScreen(
            profile = editingProfile,
            busy = editorBusy,
            onClose = { if (!editorBusy) editorOpen = false },
            onSave = { edited ->
                editorBusy = true
                scope.launch {
                    val merged = withContext(Dispatchers.IO) {
                        runCatching {
                            Mobile.mergeRoutingProfile(
                                RoutingProfileRepository.storeJson(),
                                edited.toJson().toString(),
                                false,
                            )
                        }.getOrNull()
                    }
                    val state = merged?.let { parseRoutingMergeResult(it) }
                    if (state == null) {
                        Toast.makeText(ctx,
                            ctx.getString(R.string.routing_import_failed, "merge failed"),
                            Toast.LENGTH_LONG).show()
                    } else {
                        RoutingProfileRepository.replaceAll(state.profiles, state.activeId)
                        // Ищем по OriginName, а не по id: у нового профиля id
                        // назначает Go, и до merge его здесь неоткуда взять.
                        val saved = state.profiles.firstOrNull {
                            it.name == edited.name && it.source == edited.source
                        }
                        if (saved != null) RoutingProfileCompiler.compile(saved, dataDir)
                        editorOpen = false
                    }
                    editorBusy = false
                }
            },
        )
    }
```

`RoutingProfilesScreen` получает `onEdit = { p -> editingProfile = p; editorOpen = true }`.

**Порядок рендера важен:** редактор идёт ПОСЛЕ экрана списка, иначе список
нарисуется поверх него.

- [ ] **Step 3: Собрать и прогнать обе конфигурации**

```bash
cd /c/ResultV/android && ./gradlew :app:testFullDebugUnitTest :app:testPlayDebugUnitTest \
    :app:assembleFullDebug :app:assemblePlayDebug --console=plain 2>&1 | tail -6
```

- [ ] **Step 4: Коммит**

```bash
cd /c/ResultV
git add android/app/src/main/java/com/resultv/android/ui/screens/RoutingProfilesScreen.kt \
        android/app/src/main/java/com/resultv/android/MainActivity.kt
git commit -m "feat(routing): подключить редактор — карандаш и «Создать профиль»

Профилю из подписки карандаша не даётся: его правила приходят готовыми и
перезаписываются следующей синхронизацией, так что правка обещала бы то, что
не переживёт обновления. Правило ПК.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 5: Развилка в поле вставки

**Files:**
- Modify: `ui/screens/AddScreen.kt`

- [ ] **Step 1: Найти путь вставки**

```bash
cd /c/ResultV && grep -n "parseProxyBlob\|importBlob\|onPaste\|fun AddScreen" android/app/src/main/java/com/resultv/android/ui/screens/AddScreen.kt | head
```

- [ ] **Step 2: Добавить проверку перед разбором**

В начало обработчика вставленного текста, до `Mobile.parseProxyBlob`:

```kotlin
            // Вставленная ссылка маршрутизации иначе ушла бы в parseProxyBlob
            // и умерла как «не удалось разобрать»: это не конфиг и не подписка.
            if (runCatching { Mobile.isRoutingDeepLink(trimmed) }.getOrDefault(false)) {
                DeepLinkImporter.import(ctx, trimmed)
                onDone()
                return@launch
            }
```

- [ ] **Step 3: Собрать, прогнать, закоммитить**

```bash
cd /c/ResultV/android && ./gradlew :app:assembleFullDebug :app:assemblePlayDebug --console=plain 2>&1 | tail -3
cd /c/ResultV
git add android/app/src/main/java/com/resultv/android/ui/screens/AddScreen.kt
git commit -m "feat(routing): распознавать ссылку маршрутизации в поле вставки

Иначе она уходила в parseProxyBlob и умирала как «не удалось разобрать»: это
не конфиг и не подписка.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 6: Приёмка на устройстве

Только AVD `Pixel_9_Pro` (android-36) или телефон на ядре < 6.11.

- [ ] **Step 1: Поставить свежую сборку**

```bash
cd /c/ResultV/android && ./gradlew :app:assembleFullDebug --console=plain 2>&1 | tail -3
adb install -r app/build/outputs/apk/full/debug/app-full-universal-debug.apk
```

- [ ] **Step 2: Пройти строки 6-8 таблицы 8.4 спеки**

| # | Проверка | Чем доказывается |
|---|---|---|
| 6 | Повторный импорт не раздваивает | открыть routing-ссылку, переименовать профиль редактором, открыть ту же ссылку снова: профиль ОДИН, имя пользователя сохранено, правила обновлены |
| 7 | Маршрутизация из подписки | добавить подписку, отдающую `Routing-Lists`: в списке появился профиль с именем подписки; `adb shell run-as com.resultv.android cat files/routing_profiles.json` показывает `"source":"subscription"` и её `subscriptionId` |
| 8 | Удаление подписки уносит профиль | удалить подписку: профиля нет в `routing_profiles.json`, файлов `prof-<id>-*.srs` нет в `files/routing/` |

Плюс проверка редактора:

| Проверка | Чем доказывается |
|---|---|
| Созданный руками профиль маршрутизирует | создать профиль с `proxySites = ifconfig.me`, включить, в журнале `match[N] rule_set=prof-<id>-proxy => route(proxy)` |
| Порядок правил доезжает | поставить `direct` первым, сохранить, проверить `"routeOrder":"direct-…"` в `routing_profiles.json` |
| Профиль подписки не правится | у его строки нет карандаша |

- [ ] **Step 3: Записать результат**

Дописать в спеку раздел 14 «Этап C закрыт» по образцу разделов 12 и 13: что
получилось, чем доказано, что осталось. Обновить строку «Статус» в шапке спеки
и статус блока 4 в `docs/android-pc-sync-and-play-spec.md`.

---

## Готовность этапа C

- [ ] Go зелёный в обеих конфигурациях, оба AAR пересобраны
- [ ] Kotlin зелёный в обеих конфигурациях, оба APK собираются
- [ ] Строки 6-8 таблицы 8.4 пройдены на устройстве с доказательствами
- [ ] Редактор проверен на устройстве: созданный профиль маршрутизирует, порядок доезжает
- [ ] Блок 4 закрыт целиком — записать это в родительскую спеку
