# Паритет главной, списка серверов и настроек Android с ПК — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Привести главный экран, список серверов и настройки Android к тому, как эти же вещи сделаны в редизайне ПК, с пересчётом размеров под телефон.

**Architecture:** Вся логика, которую можно проверить без экрана (вид под состояние, задержки волны, имя без флага, состав бейджей), выносится в чистые функции с юнит-тестами. Composable-слои остаются тонкими и проверяются на устройстве снимками. Ни один шаг не трогает Go.

**Tech Stack:** Kotlin, Jetpack Compose, Material 3, JUnit 4, Gradle (AGP), `org.json` в юнит-тестах.

**Spec:** `docs/superpowers/specs/2026-09-18-android-main-screen-pc-parity-design.md`

## Global Constraints

- Литералы `Color(0x…)` разрешены **только** в `android/app/src/main/java/com/resultv/android/theme/Tokens.kt`. Везде остальное — `RvColor`, `RvSpace`, `RvRadius`, `RvIcon`, `RvMotion`.
- Одна кривая движения на весь интерфейс: `RvMotion.durationMillis` (300 мс) и `RvMotion.easing`.
- Шаг волны — 70 мс, пять ступеней, последняя на 280 мс.
- Обводка интерактивных элементов — `Modifier.rvBorder(shape)` из `theme/Border.kt`: 1 dp, градиент слева white 10 % → справа white 5 %. Направление не меняется.
- Каждая новая строка ресурсов заводится в **обеих** локалях: `android/app/src/main/res/values/strings.xml` и `values-ru/strings.xml`.
- Строки с префиксом `settings_` не длиннее 64 символов — это проверяет `SettingsStringsTest`.
- Go-код и `mobile/` не трогаются, пересборка AAR не нужна.
- Все команды сборки и тестов выполняются из каталога `android`.
- Флейвор для проверки — `full` (в нём есть ad-block; `play` собирается тем же кодом).

**Порядок отличается от раздела «Порядок работ» спеки в одном месте:** волна переехала из середины в конец. Задерживать нечего, пока блоки, которые она задерживает, не собраны.

---

## Структура файлов

| Файл | Ответственность | Задача |
|---|---|---|
| `vpn/Profile.kt` | +`serverDisplayName()`, +`Profile.badges`, −`Profile.subtitle` | 1, 8 |
| `test/.../vpn/ProfileDisplayTest.kt` | тесты имени и бейджей | 1 |
| `ui/components/HomeStatus.kt` | **новый**: `HomeLook`, `homeLook()`, `WaveStep`, `waveDelayMillis()` | 4 |
| `test/.../ui/components/HomeStatusTest.kt` | **новый**: тесты вида и задержек | 4 |
| `ui/components/HomeHeader.kt` | **новый**: двухрядная шапка + плашка времени | 5 |
| `ui/components/PowerButton.kt` | кнопка питания; `StatusHeader` удаляется | 6 |
| `ui/components/SpeedTile.kt` | **новый**: плитка скорости, два режима | 7 |
| `ui/components/Sparkline.kt` | толщина линии | 7 |
| `ui/components/ServerRow.kt` | бейджи, геометрия, активная строка, обводка плитки | 2, 8 |
| `ui/screens/HomeScreen.kt` | сборка страницы, карточка списка | 5, 7, 9, 10 |
| `ui/screens/ProxiesScreen.kt` | вызовы `ServerRow` | 8 |
| `ui/screens/SettingsScreen.kt` | однострочная строка раздела, снятие отступа 50 dp | 3 |
| `ui/screens/RoutingProfileEditor.kt`, `RoutingProfilesSheet.kt` | обводка | 2 |
| `ui/components/SubscriptionEditSheet.kt` | снятие отступа 50 dp | 3 |
| `full/.../AdBlockSettings.kt`, `play/.../AdBlockSettings.kt` | удаление `items` | 3 |
| `MainActivity.kt` | `HomeTopBar` → `HomeHeader` | 5 |
| `res/values*/strings.xml` | `badge_auto` добавить; `status_*_subtitle`, `settings_group_*_items` удалить | 3, 5, 8 |

---

### Task 1: Чистые функции строки сервера

Имя сервера без ведущего флага и состав бейджей протокола. Обе функции чистые, UI ещё не трогаем — их подключит Task 8.

**Files:**
- Modify: `android/app/src/main/java/com/resultv/android/vpn/Profile.kt`
- Test: `android/app/src/test/java/com/resultv/android/vpn/ProfileDisplayTest.kt` (создать)

**Interfaces:**
- Produces:
  - `fun serverDisplayName(name: String, countryCode: String?): String` — верхнеуровневая функция в пакете `com.resultv.android.vpn`
  - `val Profile.badges: List<String>` — свойство тела `data class Profile`, считается один раз при создании

- [ ] **Step 1: Написать падающий тест**

Создать `android/app/src/test/java/com/resultv/android/vpn/ProfileDisplayTest.kt`:

```kotlin
package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Имя и бейджи строки сервера — перенос `formatProxyDisplayName` и
 * `getProtocolLabel` с ПК (`ResultVPC/frontend/src/utils/proxyParser.js`).
 * Обе функции чистые, поэтому проверяются на голой JVM.
 */
class ProfileDisplayTest {

    // ---- имя ----

    @Test fun stripsLeadingFlagEmoji() {
        assertEquals("Netherlands 01", serverDisplayName("🇳🇱 Netherlands 01", "NL"))
    }

    @Test fun leavesNameWithoutFlagAlone() {
        assertEquals("Netherlands 01", serverDisplayName("Netherlands 01", "NL"))
    }

    @Test fun stripsLeadingCountryCodeWhenItMatches() {
        assertEquals("Amsterdam", serverDisplayName("🇳🇱 NL Amsterdam", "nl"))
    }

    // Код страны снимается только целым словом: «NLD» — начало имени, а не код.
    @Test fun keepsCountryCodeGluedToAWord() {
        assertEquals("NLD Server", serverDisplayName("NLD Server", "NL"))
    }

    // Если после чистки не осталось ничего — возвращаем исходное имя, иначе
    // строка сервера стала бы пустой.
    @Test fun keepsOriginalWhenNothingIsLeft() {
        val onlyFlag = "🇳🇱"
        assertEquals(onlyFlag, serverDisplayName(onlyFlag, "NL"))
    }

    @Test fun unknownCountryStripsFlagOnly() {
        assertEquals("NL Amsterdam", serverDisplayName("🇳🇱 NL Amsterdam", null))
    }

    // Имя из одних пробелов возвращается как есть: чистить нечего, а пустая
    // строка на его месте выглядела бы поломкой списка.
    @Test fun blankNameIsReturnedUnchanged() {
        assertEquals("   ", serverDisplayName("   ", "NL"))
    }

    // ---- бейджи ----

    private fun profile(entry: String) = Profile.fromEntryJson("srv", entry)

    @Test fun bareTypeWithoutExtraGivesOneBadge() {
        val p = profile("""{"type":"VLESS","ip":"1.2.3.4","port":443}""")
        assertEquals(listOf("VLESS"), p.badges)
    }

    @Test fun realitySecurityAddsSecondBadge() {
        val p = profile("""{"type":"VLESS","extra":{"security":"reality"}}""")
        assertEquals(listOf("VLESS", "Reality"), p.badges)
    }

    @Test fun tlsAndGrpcGiveThreeBadges() {
        val p = profile("""{"type":"VLESS","extra":{"security":"tls","network":"grpc"}}""")
        assertEquals(listOf("VLESS", "TLS", "gRPC"), p.badges)
    }

    // Голова приводится к написанию макета, хвост уже в нужном виде.
    @Test fun headTakesTheDesignSpelling() {
        val p = profile("""{"type":"SS"}""")
        assertEquals(listOf("Shadowsocks"), p.badges)
    }

    // Подписки иногда отдают extra строкой, а не объектом — как на ПК.
    @Test fun extraAsAStringIsParsedToo() {
        val p = profile("""{"type":"VMESS","extra":"{\"network\":\"ws\"}"}""")
        assertEquals(listOf("VMess", "WS"), p.badges)
    }

    // У авто-группы бейдж рисует строка ресурсов, а не профиль.
    @Test fun autoGroupHasNoOwnBadges() {
        val p = profile("""{"type":"AUTO"}""")
        assertEquals(emptyList<String>(), p.badges)
    }

    @Test fun sectionHasNoBadges() {
        assertEquals(emptyList<String>(), Profile.section("Выберите ниже", "sub1").badges)
    }

    // tcp — это «ничего особенного», отдельного бейджа у него нет.
    @Test fun plainTcpAddsNothing() {
        val p = profile("""{"type":"TROJAN","extra":{"security":"none","network":"tcp"}}""")
        assertEquals(listOf("Trojan"), p.badges)
    }
}
```

- [ ] **Step 2: Запустить тест и убедиться, что он падает**

```bash
cd android && ./gradlew :app:testFullDebugUnitTest --tests '*ProfileDisplayTest*'
```

Ожидание: компиляция падает — `Unresolved reference: serverDisplayName`, `Unresolved reference: badges`.

- [ ] **Step 3: Добавить `serverDisplayName` в `Profile.kt`**

В конец файла `android/app/src/main/java/com/resultv/android/vpn/Profile.kt`, рядом с `computeProtocol`:

```kotlin
/*
 * Ведущий флаг в имени — пара символов regional indicator (U+1F1E6…U+1F1FF).
 * `\x{...}` в java.util.regex работает по кодовым точкам, суррогатные пары
 * руками собирать не нужно.
 */
private val FLAG_EMOJI_PREFIX = Regex("^[\\x{1F1E6}-\\x{1F1FF}]{2}\\s*")

/**
 * Имя сервера так, как оно показывается в списке: без ведущего флага и без
 * ведущего кода страны. Перенос `formatProxyDisplayName` с ПК
 * (`ResultVPC/frontend/src/utils/proxyParser.js`).
 *
 * Флаг уже стоит слева своей плиткой, и в имени он повторяется. Код страны
 * снимается только когда совпадает с тем, что мы и так знаем о профиле, и
 * только отдельным словом: иначе «NLD Server» превратился бы в «D Server».
 *
 * Если после чистки не осталось ничего — возвращается исходное имя: пустая
 * строка в списке хуже повторённого флага.
 */
fun serverDisplayName(name: String, countryCode: String?): String {
    if (name.isBlank()) return name
    var s = name.trim().replace(FLAG_EMOJI_PREFIX, "")
    val cc = countryCode?.trim()?.lowercase().orEmpty()
    if (cc.length == 2 && cc.all { it in 'a'..'z' }) {
        val next = s.replaceFirst(Regex("^$cc\\s+", RegexOption.IGNORE_CASE), "").trim()
        if (next.isNotEmpty()) s = next
    }
    return s.trim().ifEmpty { name }
}
```

- [ ] **Step 4: Добавить `badges` в `Profile.kt`**

В теле `data class Profile`, сразу после объявления `val protocol`:

```kotlin
    /**
     * Бейджи протокола для строки списка: тип, затем `security`, затем
     * `network` — перенос `getProtocolLabel` с ПК. У авто-группы и у
     * SECTION-строки бейджей нет: «Авто» рисует строка ресурсов, а
     * разделителю показывать нечего.
     */
    val badges: List<String> = computeBadges(isSection, isAuto, rawType, uri, parsedEntry)
```

И в конец файла, рядом с `computeProtocol`:

```kotlin
private fun computeBadges(
    isSection: Boolean,
    isAuto: Boolean,
    rawType: String,
    uri: String,
    entry: JSONObject?,
): List<String> {
    if (isSection || isAuto) return emptyList()
    val type = rawType.ifBlank { protocolFromUri(uri).orEmpty() }
    if (type.isBlank()) return emptyList()

    val out = mutableListOf(protocolCase(type))
    // Подписки отдают `extra` то объектом, то строкой с JSON внутри —
    // на ПК разбираются оба случая, здесь тоже.
    val extra = entry?.let { e ->
        e.optJSONObject("extra")
            ?: e.optString("extra").takeIf { it.isNotBlank() }
                ?.let { runCatching { JSONObject(it) }.getOrNull() }
    }
    if (extra != null) {
        when (extra.optString("security").lowercase()) {
            "reality" -> out += "Reality"
            "tls" -> out += "TLS"
        }
        when (extra.optString("network").ifBlank { "tcp" }.lowercase()) {
            "ws", "websocket" -> out += "WS"
            "grpc" -> out += "gRPC"
            "xhttp" -> out += "XHTTP"
            "h2", "http" -> out += "H2"
        }
    }
    return out
}

/*
 * Приложение отдаёт протокол капсом, а в макете он набран как имя продукта.
 * Правится только голова: хвост («Reality», «gRPC») уже в нужном виде.
 * Список — копия PROTOCOL_CASE из ResultVPC/frontend/src/views/redesign/format.js.
 */
private fun protocolCase(type: String): String = when (type.uppercase()) {
    "HYSTERIA2" -> "Hysteria2"
    "HYSTERIA" -> "Hysteria"
    "VLESS" -> "VLESS"
    "VMESS" -> "VMess"
    "TROJAN" -> "Trojan"
    "SS", "SHADOWSOCKS" -> "Shadowsocks"
    "WG", "WIREGUARD" -> "WireGuard"
    "AWG", "AMNEZIAWG", "AMNEZIA-WG" -> "AmneziaWG"
    "TUIC" -> "TUIC"
    "ANYTLS" -> "AnyTLS"
    "NAIVEPROXY", "NAIVE" -> "NaiveProxy"
    "SOCKS", "SOCKS5" -> type.uppercase()
    "HTTP", "HTTPS", "SSH" -> type.uppercase()
    else -> type.uppercase()
}
```

- [ ] **Step 5: Запустить тест и убедиться, что он проходит**

```bash
cd android && ./gradlew :app:testFullDebugUnitTest --tests '*ProfileDisplayTest*'
```

Ожидание: BUILD SUCCESSFUL, 14 тестов, 0 падений.

- [ ] **Step 6: Коммит**

```bash
git add android/app/src/main/java/com/resultv/android/vpn/Profile.kt \
        android/app/src/test/java/com/resultv/android/vpn/ProfileDisplayTest.kt
git commit -m "feat(android): имя сервера без флага и бейджи протокола"
```

---

### Task 2: Обводка всем, у кого сплошная рамка

Пять замен. Кнопка питания из списка спеки сюда не входит: её обводка переделывается целиком в Task 6, и менять её дважды незачем. `TagField` не входит вовсе — см. раздел 6 спеки: его рамка прозрачна в покое и зеленеет в фокусе, это признак фокуса, а не обводка.

**Files:**
- Modify: `android/app/src/main/java/com/resultv/android/ui/components/ServerRow.kt:107-115`
- Modify: `android/app/src/main/java/com/resultv/android/ui/screens/RoutingProfileEditor.kt:75,203,229,326`
- Modify: `android/app/src/main/java/com/resultv/android/ui/screens/RoutingProfilesSheet.kt:89,304`

**Interfaces:**
- Consumes: `Modifier.rvBorder(shape: Shape, pressed: Boolean = false)` из `com.resultv.android.theme.rvBorder` — уже существует, не меняется.

- [ ] **Step 1: Заменить рамку плитки флага в `ServerRow.kt`**

Было:

```kotlin
                .border(
                    1.dp,
                    if (isActive) RvColor.Main.copy(alpha = 0.28f)
                    else Color.White.copy(alpha = 0.09f),
                    RoundedCornerShape(RvRadius.chip)
                ),
```

Стало:

```kotlin
                .rvBorder(RoundedCornerShape(RvRadius.chip)),
```

Добавить импорт `com.resultv.android.theme.rvBorder`; удалить импорт `androidx.compose.foundation.border`, если он больше не используется в файле.

- [ ] **Step 2: Заменить три рамки в `RoutingProfileEditor.kt`**

```kotlin
// :203
.border(1.dp, EditorBorder, RoundedCornerShape(100.dp))   →   .rvBorder(RoundedCornerShape(100.dp))
// :229
.border(1.dp, EditorBorder, PanelShape)                   →   .rvBorder(PanelShape)
// :326
.border(1.dp, EditorBorder, PanelShape),                  →   .rvBorder(PanelShape),
```

Константа `private val EditorBorder = Color.White.copy(alpha = 0.06f)` на строке 75 после этого осиротеет — удалить её объявление. Добавить импорт `com.resultv.android.theme.rvBorder`, снять `androidx.compose.foundation.border`, если осиротел.

- [ ] **Step 3: Заменить рамку карточки в `RoutingProfilesSheet.kt:304`**

Было:

```kotlin
            .border(1.dp, if (isActive) ActiveBorder else CardBorder, CardShape)
```

Стало:

```kotlin
            // Активный профиль держит свой зелёный контур: это признак
            // выбора, а не обводка интерактивного элемента, и градиент его
            // стёр бы.
            .then(
                if (isActive) Modifier.border(1.dp, ActiveBorder, CardShape)
                else Modifier.rvBorder(CardShape)
            )
```

Константа `private val CardBorder = Color.White.copy(alpha = 0.06f)` на строке 89 осиротеет — удалить. `ActiveBorder` и импорт `border` остаются.

- [ ] **Step 4: Собрать и убедиться, что компилируется**

```bash
cd android && ./gradlew :app:assembleFullDebug -Pdebug.abi=arm64-v8a
```

Ожидание: BUILD SUCCESSFUL, ни одного предупреждения об неиспользуемом импорте в изменённых файлах.

- [ ] **Step 5: Поставить на телефон и посмотреть**

```bash
adb -s e3bacc6b install -r -d android/app/build/outputs/apk/full/debug/app-full-arm64-v8a-debug.apk
adb -s e3bacc6b shell am start -n com.resultv.android/.MainActivity
adb -s e3bacc6b exec-out screencap -p > /c/Users/andbe/AppData/Local/Temp/claude/borders-home.png
```

Смотреть: у плитки флага в списке серверов рамка теперь светлее слева и темнее справа, а не ровная по кругу.

- [ ] **Step 6: Коммит**

```bash
git add android/app/src/main/java/com/resultv/android/ui/
git commit -m "style(android): градиентная обводка везде, где была сплошная рамка"
```

---

### Task 3: Настройки — однострочная строка раздела и снятие отступа 50 dp

**Files:**
- Modify: `android/app/src/main/java/com/resultv/android/ui/screens/SettingsScreen.kt:73,79-111,367-400,513,532,540,620,655,680,865,869`
- Modify: `android/app/src/main/java/com/resultv/android/ui/components/SubscriptionEditSheet.kt:173,307`
- Modify: `android/app/src/full/java/com/resultv/android/ui/screens/AdBlockSettings.kt:43`
- Modify: `android/app/src/play/java/com/resultv/android/ui/screens/AdBlockSettings.kt:22`
- Modify: `android/app/src/main/res/values/strings.xml:110-120`
- Modify: `android/app/src/main/res/values-ru/strings.xml:105-115`
- Modify: `android/app/src/full/res/values/strings.xml:54`
- Modify: `android/app/src/full/res/values-ru/strings.xml:53`

**Interfaces:**
- Produces: `SettingsSubcategory` без поля `itemsRes` — конструктор становится `(labelRes, descRes, icon, tint)`.

- [ ] **Step 1: Убрать вторую строку из `SubcategoryRow`**

В `SettingsScreen.kt` функция `SubcategoryRow` — убрать `Column` с двумя `Text` и оставить один:

```kotlin
@Composable
private fun SubcategoryRow(subcategory: SettingsSubcategory, onClick: () -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .padding(horizontal = RvSpace.nest1, vertical = RvSpace.nest2),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2),
    ) {
        SettingIcon(subcategory.icon, subcategory.tint)
        Text(
            stringResource(subcategory.labelRes),
            style = MaterialTheme.typography.titleMedium,
            modifier = Modifier.weight(1f),
        )
        Icon(
            imageVector = Icons.AutoMirrored.Outlined.KeyboardArrowRight,
            contentDescription = null,
            tint = RvColor.iconDefault,
        )
    }
}
```

- [ ] **Step 2: Убрать поле `itemsRes` из перечисления**

В `SettingsScreen.kt` удалить `val itemsRes: Int,` из конструктора `SettingsSubcategory` и второй аргумент-ресурс у каждого из восьми значений. Например:

```kotlin
    Network(
        R.string.settings_group_network, R.string.settings_group_network_desc,
        Icons.Outlined.Public, RvCategory.Main,
    ),
```

В `AdBlock` аргумент `AdBlockGroupRes.items` тоже уходит.

- [ ] **Step 3: Убрать `items` из обоих флейворов**

- `src/full/.../AdBlockSettings.kt`: удалить строку `val items = R.string.settings_group_adblock_items`.
- `src/play/.../AdBlockSettings.kt`: удалить строку `val items = R.string.settings_group_security_items`.

- [ ] **Step 4: Удалить осиротевшие строки ресурсов**

Удалить из `main/res/values/strings.xml` и `main/res/values-ru/strings.xml` семь строк каждая:

```
settings_group_network_items
settings_group_routing_items
settings_group_ping_items
settings_group_experimental_items
settings_group_security_items
settings_group_appearance_items
settings_group_subscriptions_items
```

И `settings_group_adblock_items` из `full/res/values/strings.xml` и `full/res/values-ru/strings.xml`.

- [ ] **Step 5: Снять все десять отступов 50 dp**

В `SettingsScreen.kt`:

```kotlin
// :513  FlowRow чипов типа пробы
modifier = Modifier.padding(start = 50.dp),                                   → убрать modifier целиком
// :532
modifier = Modifier.fillMaxWidth().padding(start = 50.dp, top = RvSpace.nest3), → modifier = Modifier.fillMaxWidth().padding(top = RvSpace.nest3),
// :540
modifier = Modifier.padding(start = 50.dp, top = RvSpace.nest3),              → modifier = Modifier.padding(top = RvSpace.nest3),
// :620  FlowRow пинга
modifier = Modifier.padding(start = 50.dp),                                   → убрать modifier целиком
// :655, :680  поля адреса и бюджета
modifier = Modifier.fillMaxWidth().padding(start = 50.dp),                    → modifier = Modifier.fillMaxWidth(),
// :865  поле в TextFieldRow
modifier = Modifier.fillMaxWidth().padding(start = 50.dp),                    → modifier = Modifier.fillMaxWidth(),
// :869  подпись в TextFieldRow
modifier = Modifier.padding(start = 50.dp)                                    → убрать параметр modifier целиком
```

В `SubscriptionEditSheet.kt:173` и `:307` — так же: `.padding(start = 50.dp)` убрать, остальные модификаторы в цепочке сохранить.

- [ ] **Step 6: Прогнать тесты строк**

```bash
cd android && ./gradlew :app:testFullDebugUnitTest --tests '*SettingsStringsTest*'
```

Ожидание: BUILD SUCCESSFUL. Тест проверяет паритет локалей по префиксу `settings_`; строки удалены из обеих, значит паритет цел.

- [ ] **Step 7: Собрать оба флейвора**

```bash
cd android && ./gradlew :app:assembleFullDebug :app:assemblePlayDebug -Pdebug.abi=arm64-v8a
```

Ожидание: BUILD SUCCESSFUL. Оба нужны: `itemsRes` жил в обоих исходниках `AdBlockSettings.kt`.

- [ ] **Step 8: Снимок настроек на телефоне**

```bash
adb -s e3bacc6b install -r -d android/app/build/outputs/apk/full/debug/app-full-arm64-v8a-debug.apk
adb -s e3bacc6b shell am start -n com.resultv.android/.MainActivity
# перейти на вкладку настроек руками, затем:
adb -s e3bacc6b exec-out screencap -p > /c/Users/andbe/AppData/Local/Temp/claude/settings-main.png
adb -s e3bacc6b exec-out screencap -p > /c/Users/andbe/AppData/Local/Temp/claude/settings-ping.png
```

Смотреть: карточки разделов однострочные; в разделе «Пинг» чипы и поля стоят по левому краю карточки, а не под значком.

- [ ] **Step 9: Коммит**

```bash
git add android/app/src/
git commit -m "refactor(android): однострочные разделы настроек и снятие отступа под значок"
```

---

### Task 4: Вид состояния и задержки волны

Таблица `BY_STATUS` с ПК и шаг волны — чистыми функциями, до всякого UI.

**Files:**
- Create: `android/app/src/main/java/com/resultv/android/ui/components/HomeStatus.kt`
- Test: `android/app/src/test/java/com/resultv/android/ui/components/HomeStatusTest.kt` (создать)

**Interfaces:**
- Consumes: `com.resultv.android.vpn.VpnStatus` (Idle / Connecting / Connected / Error)
- Produces:
  - `enum class HomeLook { Idle, Processing, Success, Error }`
  - `fun homeLook(status: VpnStatus): HomeLook`
  - `enum class WaveStep { Power, Title, Time, Card, Speed }`
  - `const val WAVE_STEP_MILLIS: Int = 70`
  - `fun waveDelayMillis(step: WaveStep, connected: Boolean): Int`

- [ ] **Step 1: Написать падающий тест**

Создать `android/app/src/test/java/com/resultv/android/ui/components/HomeStatusTest.kt`:

```kotlin
package com.resultv.android.ui.components

import com.resultv.android.vpn.VpnStatus
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Вид главной под состояние и задержки волны — перенос BY_STATUS и блока
 * `--rv-wave` из `ResultVPC/frontend/src/views/redesign/MainPage.{jsx,css}`.
 */
class HomeStatusTest {

    @Test fun eachStatusMapsToItsLook() {
        assertEquals(HomeLook.Idle, homeLook(VpnStatus.Idle))
        assertEquals(HomeLook.Processing, homeLook(VpnStatus.Connecting))
        assertEquals(HomeLook.Success, homeLook(VpnStatus.Connected(0L)))
        assertEquals(HomeLook.Error, homeLook(VpnStatus.Error("boom")))
    }

    // Подключение: волна идёт от кнопки наружу.
    @Test fun connectingWaveRunsOutwardFromTheButton() {
        assertEquals(0, waveDelayMillis(WaveStep.Power, connected = true))
        assertEquals(70, waveDelayMillis(WaveStep.Title, connected = true))
        assertEquals(140, waveDelayMillis(WaveStep.Time, connected = true))
        assertEquals(210, waveDelayMillis(WaveStep.Card, connected = true))
        assertEquals(280, waveDelayMillis(WaveStep.Speed, connected = true))
    }

    // Отключение: гаснет сначала то, что зажглось последним.
    @Test fun disconnectingWaveRunsBackward() {
        assertEquals(0, waveDelayMillis(WaveStep.Speed, connected = false))
        assertEquals(70, waveDelayMillis(WaveStep.Card, connected = false))
        assertEquals(140, waveDelayMillis(WaveStep.Time, connected = false))
        assertEquals(210, waveDelayMillis(WaveStep.Title, connected = false))
        assertEquals(280, waveDelayMillis(WaveStep.Power, connected = false))
    }

    // Вся волна укладывается в 280 мс — иначе она читалась бы как задержка.
    @Test fun waveFitsInTwoHundredEighty() {
        val longest = WaveStep.entries.maxOf { waveDelayMillis(it, connected = true) }
        assertEquals(280, longest)
    }
}
```

- [ ] **Step 2: Запустить тест и убедиться, что он падает**

```bash
cd android && ./gradlew :app:testFullDebugUnitTest --tests '*HomeStatusTest*'
```

Ожидание: компиляция падает — `Unresolved reference: HomeLook`.

- [ ] **Step 3: Написать реализацию**

Создать `android/app/src/main/java/com/resultv/android/ui/components/HomeStatus.kt`:

```kotlin
package com.resultv.android.ui.components

import com.resultv.android.vpn.VpnStatus

/**
 * Вид главной под состояние подключения — перенос таблицы BY_STATUS из
 * `ResultVPC/frontend/src/views/redesign/MainPage.jsx`.
 *
 * Одно состояние задаёт разом четыре вещи: вариант шапки, вариант кнопки
 * питания, подсветку карточки сервера и режим плиток скорости. Поэтому это
 * одно перечисление, а не четыре набора `when` по `VpnStatus`, разъехавшихся
 * бы при первой же правке.
 */
enum class HomeLook { Idle, Processing, Success, Error }

fun homeLook(status: VpnStatus): HomeLook = when (status) {
    is VpnStatus.Idle -> HomeLook.Idle
    is VpnStatus.Connecting -> HomeLook.Processing
    is VpnStatus.Connected -> HomeLook.Success
    is VpnStatus.Error -> HomeLook.Error
}

/**
 * Ступени волны в порядке от кнопки наружу. Порядок объявления — и есть
 * порядок волны; добавлять ступени только на своё место.
 */
enum class WaveStep { Power, Title, Time, Card, Speed }

/** Шаг волны из макета. */
const val WAVE_STEP_MILLIS = 70

/**
 * Задержка ступени.
 *
 * Подключение меняет разом полстраницы: заливку кнопки, цвет заголовка,
 * плашку времени, подсветку карточки, кривые скорости. Пущенное одним
 * проблеском, это читается как мигание всего экрана. Волна разносит
 * изменение во времени: первой отзывается кнопка, последними плитки.
 *
 * При отключении порядок обратный — гаснет сначала то, что зажглось
 * последним. Задержку каждый раз берёт то состояние, в которое переходим.
 */
fun waveDelayMillis(step: WaveStep, connected: Boolean): Int {
    val order = if (connected) step.ordinal else WaveStep.entries.size - 1 - step.ordinal
    return order * WAVE_STEP_MILLIS
}
```

- [ ] **Step 4: Запустить тест и убедиться, что он проходит**

```bash
cd android && ./gradlew :app:testFullDebugUnitTest --tests '*HomeStatusTest*'
```

Ожидание: BUILD SUCCESSFUL, 4 теста, 0 падений.

- [ ] **Step 5: Коммит**

```bash
git add android/app/src/main/java/com/resultv/android/ui/components/HomeStatus.kt \
        android/app/src/test/java/com/resultv/android/ui/components/HomeStatusTest.kt
git commit -m "feat(android): вид главной под состояние и задержки волны"
```

---

### Task 5: Шапка главной

Двухрядная шапка со статусом и плашкой времени вместо логотипа со словом «ResultV» и отдельного `StatusHeader`.

**Files:**
- Create: `android/app/src/main/java/com/resultv/android/ui/components/HomeHeader.kt`
- Modify: `android/app/src/main/java/com/resultv/android/MainActivity.kt:297-343` (`HomeTopBar`), `:385-395` (слот `topBar`)
- Modify: `android/app/src/main/java/com/resultv/android/ui/components/PowerButton.kt:174-215` (удалить `StatusHeader`)
- Modify: `android/app/src/main/java/com/resultv/android/ui/screens/HomeScreen.kt:132` (снять вызов `StatusHeader`), `:150-165` (снять `UptimeChip` из панели), `:255-300` (перенести `UptimeChip` и `formatDuration`)
- Modify: `android/app/src/main/res/values/strings.xml:51-52`, `values-ru/strings.xml:47-48`

**Interfaces:**
- Consumes: `homeLook(status)`, `HomeLook` из Task 4
- Produces: `@Composable fun HomeHeader(onOpenWebsite: () -> Unit, onOpenTelegram: () -> Unit)` — статус и время читает сам из `VpnState.status`

- [ ] **Step 1: Создать `HomeHeader.kt`**

```kotlin
package com.resultv.android.ui.components

import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Language
import androidx.compose.material.icons.outlined.Schedule
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.resultv.android.R
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.RvMotion
import com.resultv.android.theme.RvRadius
import com.resultv.android.theme.RvSpace
import com.resultv.android.vpn.VpnState
import com.resultv.android.vpn.VpnStatus

/**
 * Шапка главной — перенос `Header` (Figma 6521:263) из кита ПК.
 *
 * На ПК это один ряд: слева плашка времени, по центру заголовок стилем H1,
 * справа сайт и телеграм. На 360 dp «Что-то пошло не так» между плашкой и
 * двумя иконками не помещается, поэтому здесь два ряда: служебный сверху,
 * заголовок под ним во всю ширину. Заголовок при этом центрируется честно,
 * а не «по остатку», как на ПК.
 *
 * Логотип остаётся, слова «ResultV» больше нет: на ПК его в шапке не было
 * вовсе, оно жило в сайдбаре, которого на телефоне нет.
 *
 * Подзаголовка нет ни в одном состоянии.
 */
@Composable
fun HomeHeader(
    onOpenWebsite: () -> Unit,
    onOpenTelegram: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val status by VpnState.status.collectAsStateWithLifecycle()
    val look = homeLook(status)

    val titleColor by animateColorAsState(
        targetValue = when (look) {
            HomeLook.Idle -> RvColor.whiteA50
            HomeLook.Processing -> RvColor.Warning
            HomeLook.Success -> RvColor.Main
            HomeLook.Error -> RvColor.Errors
        },
        animationSpec = tween(RvMotion.durationMillis, easing = RvMotion.easing),
        label = "headerTitle",
    )

    Column(
        modifier = modifier
            .fillMaxWidth()
            .background(RvColor.Black)
            .windowInsetsPadding(WindowInsets.statusBars)
            .padding(horizontal = RvSpace.nest1, vertical = RvSpace.nest3),
    ) {
        Row(
            modifier = Modifier.fillMaxWidth().height(40.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(RvSpace.nest3),
        ) {
            androidx.compose.foundation.Image(
                painter = painterResource(R.drawable.resultv_logo),
                contentDescription = null,
                modifier = Modifier.size(24.dp),
            )
            (status as? VpnStatus.Connected)?.let { UptimeChip(connectedAt = it.connectedAt) }
            Spacer(Modifier.weight(1f))
            IconButton(onClick = onOpenWebsite, modifier = Modifier.size(36.dp)) {
                Icon(
                    imageVector = Icons.Outlined.Language,
                    contentDescription = stringResource(R.string.header_open_website),
                    tint = RvColor.whiteA50,
                    modifier = Modifier.size(20.dp),
                )
            }
            IconButton(onClick = onOpenTelegram, modifier = Modifier.size(36.dp)) {
                Icon(
                    painter = painterResource(R.drawable.ic_telegram),
                    contentDescription = stringResource(R.string.header_open_telegram),
                    tint = RvColor.whiteA50,
                    modifier = Modifier.size(20.dp),
                )
            }
        }

        Text(
            text = stringResource(
                when (look) {
                    HomeLook.Idle -> R.string.status_unprotected
                    HomeLook.Processing -> R.string.status_connecting
                    HomeLook.Success -> R.string.status_protected
                    HomeLook.Error -> R.string.status_error
                }
            ),
            style = MaterialTheme.typography.headlineSmall,
            fontWeight = FontWeight.Bold,
            color = titleColor,
            textAlign = TextAlign.Center,
            modifier = Modifier.fillMaxWidth().padding(top = RvSpace.nest3),
        )
    }
}

/**
 * Плашка времени соединения — `rv-header__time` с ПК.
 *
 * Отступы несимметричны по горизонтали, как в макете (9/18): слева стоит
 * значок часов со своим воздухом внутри рисунка, и равные отступы читались
 * бы как сдвиг текста влево.
 *
 * Тикает раз в секунду своим `LaunchedEffect`, чтобы остальная шапка не
 * пересобиралась вместе с таймером.
 */
@Composable
private fun UptimeChip(connectedAt: Long) {
    var now by remember { mutableLongStateOf(System.currentTimeMillis()) }
    LaunchedEffect(connectedAt) {
        while (true) {
            now = System.currentTimeMillis()
            kotlinx.coroutines.delay(1000L)
        }
    }
    val elapsedSec = ((now - connectedAt).coerceAtLeast(0L) / 1000L)

    Row(
        modifier = Modifier
            .clip(RoundedCornerShape(RvRadius.chip))
            .background(RvColor.Grey)
            .padding(start = 8.dp, top = 8.dp, bottom = 8.dp, end = 16.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        Icon(
            imageVector = Icons.Outlined.Schedule,
            contentDescription = null,
            tint = RvColor.whiteA50,
            modifier = Modifier.size(14.dp),
        )
        Text(
            text = formatUptime(elapsedSec),
            style = MaterialTheme.typography.labelMedium,
            color = RvColor.whiteA50,
        )
    }
}

internal fun formatUptime(totalSec: Long): String {
    val h = totalSec / 3600
    val m = (totalSec % 3600) / 60
    val s = totalSec % 60
    return if (h > 0) String.format("%d:%02d:%02d", h, m, s)
    else String.format("%02d:%02d", m, s)
}
```

- [ ] **Step 2: Подключить шапку в `MainActivity.kt`**

Удалить функцию `HomeTopBar` целиком. В слоте `topBar` у `Scaffold`:

```kotlin
        topBar = {
            if (tab == Tab.Home) {
                HomeHeader(
                    onOpenWebsite = { openUrl(ctx, WEBSITE_URL) },
                    onOpenTelegram = { openUrl(ctx, TELEGRAM_URL) },
                )
            } else {
                CenterAlignedTopAppBar(
                    title = { Text(text = stringResource(tab.titleRes)) },
                    colors = TopAppBarDefaults.centerAlignedTopAppBarColors(
                        containerColor = RvColor.Black,
                    ),
                )
            }
        },
```

Добавить импорт `com.resultv.android.ui.components.HomeHeader`. Удалить осиротевшие импорты: `BenzinBold`, `FontWeight`, `Image`, `painterResource`, `Icons.Outlined.Language`, `TopAppBar` — по факту того, что перестало использоваться в файле.

- [ ] **Step 3: Удалить `StatusHeader`**

В `PowerButton.kt` удалить функцию `StatusHeader` целиком вместе с осиротевшими импортами (`stringResource` останется — его использует сама кнопка).

В `HomeScreen.kt` удалить строку вызова `StatusHeader(status = status, activeProfileName = active?.name)` и импорт `com.resultv.android.ui.components.StatusHeader`.

- [ ] **Step 4: Убрать старый `UptimeChip` из `HomeScreen.kt`**

Удалить приватную функцию `UptimeChip` и `formatDuration` из `HomeScreen.kt` (переехали в `HomeHeader.kt` под именем `formatUptime`). Из панели инструментов убрать блок:

```kotlin
            (status as? VpnStatus.Connected)?.let { connected ->
                UptimeChip(connectedAt = connected.connectedAt)
            }
            Spacer(Modifier.weight(1f))
```

заменив его на один `Spacer(Modifier.weight(1f))`. Сама панель с кнопками замера и сортировки пока остаётся — её разберёт Task 9.

- [ ] **Step 5: Удалить осиротевшие строки статуса**

Из `main/res/values/strings.xml` удалить:

```xml
    <string name="status_traffic_routed_via">Traffic routed via %1$s</string>
    <string name="status_unprotected_subtitle">Your connection is not protected</string>
```

Из `main/res/values-ru/strings.xml` — те же два имени.

- [ ] **Step 6: Собрать**

```bash
cd android && ./gradlew :app:assembleFullDebug -Pdebug.abi=arm64-v8a
```

Ожидание: BUILD SUCCESSFUL. Если компилятор ругается на `status_unprotected_subtitle` — значит остался вызов, который надо снять, а не строку вернуть.

- [ ] **Step 7: Снимки четырёх состояний**

```bash
adb -s e3bacc6b install -r -d android/app/build/outputs/apk/full/debug/app-full-arm64-v8a-debug.apk
adb -s e3bacc6b shell am start -n com.resultv.android/.MainActivity
adb -s e3bacc6b exec-out screencap -p > /c/Users/andbe/AppData/Local/Temp/claude/header-idle.png
# нажать подключение, снять в процессе и после
adb -s e3bacc6b exec-out screencap -p > /c/Users/andbe/AppData/Local/Temp/claude/header-connected.png
```

Смотреть: слова «ResultV» нет; заголовок по центру; подзаголовка нет ни в одном состоянии; плашка времени появляется только при подключении и не налезает на иконки.

- [ ] **Step 8: Коммит**

```bash
git add android/app/src/
git commit -m "feat(android): шапка главной со статусом и плашкой времени, как на ПК"
```

---

### Task 6: Кнопка питания

**Files:**
- Modify: `android/app/src/main/java/com/resultv/android/ui/components/PowerButton.kt` (переписать функцию `PowerButton` целиком)
- Modify: `android/app/src/main/java/com/resultv/android/ui/screens/HomeScreen.kt:134-138` (вызов)

**Interfaces:**
- Consumes: `HomeLook`, `homeLook(status)` из Task 4; `Modifier.rvBorder` из `theme/Border.kt`
- Produces: `@Composable fun PowerButton(look: HomeLook, enabled: Boolean, onClick: () -> Unit, modifier: Modifier = Modifier)`

- [ ] **Step 1: Переписать `PowerButton`**

Заменить тело файла `PowerButton.kt` (кроме заголовка пакета) на:

```kotlin
package com.resultv.android.ui.components

import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.PowerSettingsNew
import androidx.compose.material3.Icon
import androidx.compose.material3.Surface
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.draw.scale
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.resultv.android.R
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.RvMotion
import com.resultv.android.theme.rvBorder

/**
 * Кнопка питания — перенос `PowerButton` (Figma 6481:7) из кита ПК.
 *
 * Размер взят не пропорцией с ПК (220 из 862 дали бы здесь 80 dp), а
 * мобильный: на телефоне эта кнопка и есть экран. Отношение иконки к кругу
 * при этом из макета — ровно половина.
 *
 * Кольца прогресса нет намеренно. На ПК признак работы в том, что кнопка
 * ОСТАЁТСЯ вдавленной, пока идёт подключение, и распрямляется в момент
 * успеха: вдавливание и распрямление сами по себе движение. Кольцо обещало
 * бы прогресс, которого никто не считает.
 */
private val CIRCLE = 200.dp
private val GLYPH = 100.dp
private val HALO = 240.dp

@Composable
fun PowerButton(
    look: HomeLook,
    enabled: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val interaction = remember { MutableInteractionSource() }
    val touched by interaction.collectIsPressedAsState()
    // Пока идёт подключение, кнопка вдавлена независимо от касания.
    val pressed = touched || look == HomeLook.Processing

    val motion = tween<Color>(RvMotion.durationMillis, easing = RvMotion.easing)

    val fill by animateColorAsState(
        targetValue = if (look == HomeLook.Success) RvColor.Main else RvColor.DarkGrey,
        animationSpec = motion, label = "powerFill",
    )
    val glyph by animateColorAsState(
        targetValue = when (look) {
            HomeLook.Idle -> RvColor.whiteA50
            HomeLook.Processing -> RvColor.Warning
            HomeLook.Success -> RvColor.Black
            HomeLook.Error -> RvColor.Errors
        },
        animationSpec = motion, label = "powerGlyph",
    )
    val glow by animateColorAsState(
        targetValue = when (look) {
            HomeLook.Idle -> Color.Transparent
            HomeLook.Processing -> RvColor.warningA20
            HomeLook.Success -> RvColor.mainA50
            HomeLook.Error -> RvColor.errorsA20
        },
        animationSpec = motion, label = "powerGlow",
    )
    val scale by animateFloatAsState(
        targetValue = if (pressed) 0.9727f else 1f,
        animationSpec = tween(RvMotion.durationMillis, easing = RvMotion.easing),
        label = "powerScale",
    )
    // Внутренняя тень нажатия. На ПК это `inset -8px -8px 16px`; в Compose
    // inset-shadow нет, рисуется радиальным градиентом поверх заливки.
    val inset = if (look == HomeLook.Success) RvColor.blackA25 else RvColor.blackA80

    Box(modifier = modifier.size(HALO), contentAlignment = Alignment.Center) {
        // Свечение — отдельный диск позади кнопки, плавно уходящий в ноль,
        // чтобы у ореола не было видимого края.
        Box(
            modifier = Modifier
                .size(HALO)
                .background(
                    Brush.radialGradient(
                        colorStops = arrayOf(
                            0f to glow,
                            0.35f to glow.copy(alpha = glow.alpha * 0.55f),
                            0.7f to glow.copy(alpha = glow.alpha * 0.15f),
                            1f to Color.Transparent,
                        ),
                    ),
                    CircleShape,
                ),
        )
        Surface(
            onClick = onClick,
            enabled = enabled,
            interactionSource = interaction,
            shape = CircleShape,
            color = fill,
            contentColor = glyph,
            modifier = Modifier
                .size(CIRCLE)
                .scale(scale)
                .then(
                    when (look) {
                        // У нейтральной кнопки обводка общая, градиентная.
                        HomeLook.Idle -> Modifier.rvBorder(CircleShape)
                        // Жёлтая обводка в макете сплошная и непрозрачная.
                        HomeLook.Processing -> Modifier.border(1.dp, RvColor.Warning, CircleShape)
                        // У зелёной и красной обводки нет вовсе.
                        else -> Modifier
                    }
                )
                .drawWithContent {
                    drawContent()
                    if (pressed) {
                        drawCircle(
                            Brush.radialGradient(
                                colorStops = arrayOf(
                                    0.55f to Color.Transparent,
                                    1f to inset,
                                ),
                                radius = size.minDimension / 2f,
                            )
                        )
                    }
                },
        ) {
            Box(contentAlignment = Alignment.Center) {
                Icon(
                    imageVector = Icons.Filled.PowerSettingsNew,
                    contentDescription = stringResource(
                        if (look == HomeLook.Success) R.string.action_disconnect
                        else R.string.action_connect,
                    ),
                    modifier = Modifier.size(GLYPH),
                )
            }
        }
    }
}
```

- [ ] **Step 2: Обновить вызов в `HomeScreen.kt`**

```kotlin
        PowerButton(
            look = homeLook(status),
            enabled = canConnect || canDisconnect,
            onClick = onPowerPressed,
        )
```

Добавить импорт `com.resultv.android.ui.components.homeLook`.

- [ ] **Step 3: Собрать**

```bash
cd android && ./gradlew :app:assembleFullDebug -Pdebug.abi=arm64-v8a
```

Ожидание: BUILD SUCCESSFUL.

- [ ] **Step 4: Проверить на телефоне все четыре вида**

```bash
adb -s e3bacc6b install -r -d android/app/build/outputs/apk/full/debug/app-full-arm64-v8a-debug.apk
adb -s e3bacc6b shell am start -n com.resultv.android/.MainActivity
adb -s e3bacc6b exec-out screencap -p > /c/Users/andbe/AppData/Local/Temp/claude/power-idle.png
# нажать и держать — снять вдавленную; затем подключение и успех
adb -s e3bacc6b exec-out screencap -p > /c/Users/andbe/AppData/Local/Temp/claude/power-connecting.png
adb -s e3bacc6b exec-out screencap -p > /c/Users/andbe/AppData/Local/Temp/claude/power-connected.png
```

Смотреть: кольца прогресса нет; во время подключения кнопка вдавлена и жёлтая; в момент успеха распрямляется и зеленеет; соседи под кнопкой при нажатии не прыгают.

- [ ] **Step 5: Коммит**

```bash
git add android/app/src/main/java/com/resultv/android/ui/
git commit -m "feat(android): кнопка питания по макету ПК, без кольца прогресса"
```

---

### Task 7: Плитки скорости

**Files:**
- Create: `android/app/src/main/java/com/resultv/android/ui/components/SpeedTile.kt`
- Modify: `android/app/src/main/java/com/resultv/android/ui/components/Sparkline.kt:32`
- Modify: `android/app/src/main/java/com/resultv/android/ui/screens/HomeScreen.kt:198-200` (условие показа), `:302-372` (`TrafficStatsRow`, `StatCard` — удалить)

**Interfaces:**
- Produces: `@Composable fun SpeedTile(label: String, rate: String, total: String, history: List<Float>, color: Color, active: Boolean, modifier: Modifier = Modifier)`

- [ ] **Step 1: Создать `SpeedTile.kt`**

```kotlin
package com.resultv.android.ui.components

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.RvMotion
import com.resultv.android.theme.RvRadius
import com.resultv.android.theme.RvSpace

/**
 * Плитка скорости — перенос `Speed` (Figma 6528:714) из кита ПК.
 *
 * Плитка видна всегда, как на ПК: без соединения блок цифр приглушён до
 * 50 %, а вместо кривой стоит плоская линия. Иначе страница меняла бы
 * высоту в момент подключения — ровно тогда, когда на неё смотрят.
 *
 * Высота фиксированная: пропорция ПК (158 из 423) дала бы здесь 56 dp, в
 * которые содержимое не влезает.
 */
private val TILE_HEIGHT = 112.dp
private val CHART_HEIGHT = 28.dp

@Composable
fun SpeedTile(
    label: String,
    rate: String,
    total: String,
    history: List<Float>,
    color: Color,
    active: Boolean,
    modifier: Modifier = Modifier,
) {
    val dataAlpha by animateFloatAsState(
        targetValue = if (active) 1f else 0.5f,
        animationSpec = tween(RvMotion.durationMillis, easing = RvMotion.easing),
        label = "speedData",
    )

    Card(
        modifier = modifier.height(TILE_HEIGHT),
        shape = RoundedCornerShape(RvRadius.card),
        colors = CardDefaults.cardColors(containerColor = RvColor.Grey),
    ) {
        Column(
            modifier = Modifier.fillMaxWidth().padding(RvSpace.nest1),
            verticalArrangement = Arrangement.SpaceBetween,
        ) {
            Column(modifier = Modifier.alpha(dataAlpha)) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween,
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(label, style = MaterialTheme.typography.labelMedium, color = RvColor.whiteA50)
                    Text(rate, style = MaterialTheme.typography.labelMedium, color = RvColor.whiteA50)
                }
                Text(
                    total,
                    style = MaterialTheme.typography.titleLarge,
                    fontWeight = FontWeight.Bold,
                    color = color,
                )
            }

            Box(
                modifier = Modifier.fillMaxWidth().height(CHART_HEIGHT),
                contentAlignment = Alignment.BottomStart,
            ) {
                if (active) {
                    Sparkline(
                        values = history,
                        color = color,
                        modifier = Modifier.fillMaxWidth().height(CHART_HEIGHT),
                    )
                } else {
                    // «Трафика не было» — отрезок в цвет плитки на 50 %,
                    // ровно там, где пошла бы кривая.
                    Box(
                        modifier = Modifier
                            .fillMaxWidth()
                            .height(2.dp)
                            .background(color.copy(alpha = 0.5f)),
                    )
                }
            }
        }
    }
}
```

- [ ] **Step 2: Поправить толщину линии в `Sparkline.kt`**

```kotlin
    strokeWidthPx: Float = 2.25f,   →   strokeWidthPx: Float = 2f,
```

и в KDoc заменить «2.25 px stroke» на «2 dp stroke — толщина 3 из макета, пересчитанная под плитку телефона».

- [ ] **Step 3: Заменить `TrafficStatsRow` в `HomeScreen.kt`**

Удалить приватную `StatCard` целиком. `TrafficStatsRow` переписать:

```kotlin
@Composable
private fun TrafficStatsRow(active: Boolean) {
    val stats by com.resultv.android.vpn.TrafficStats.snapshot.collectAsStateWithLifecycle()
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2),
    ) {
        SpeedTile(
            label = stringResource(R.string.home_stat_download),
            rate = formatBps(stats.downloadBps),
            total = formatBytes(stats.downloadBytes),
            history = stats.downloadHistory.map { it.toFloat() },
            color = RvColor.Main,
            active = active,
            modifier = Modifier.weight(1f),
        )
        SpeedTile(
            label = stringResource(R.string.home_stat_upload),
            rate = formatBps(stats.uploadBps),
            total = formatBytes(stats.uploadBytes),
            history = stats.uploadHistory.map { it.toFloat() },
            color = RvColor.Second,
            active = active,
            modifier = Modifier.weight(1f),
        )
    }
}
```

`formatBytes` и `formatBps` уже есть в файле — не трогать.

- [ ] **Step 4: Показывать плитки всегда**

В теле `HomeScreen` заменить:

```kotlin
        if (status is VpnStatus.Connected) {
            TrafficStatsRow()
        }
```

на:

```kotlin
        TrafficStatsRow(active = status is VpnStatus.Connected)
```

Добавить импорт `com.resultv.android.ui.components.SpeedTile`; снять импорт `Sparkline`, если он в `HomeScreen.kt` осиротел.

- [ ] **Step 5: Собрать**

```bash
cd android && ./gradlew :app:assembleFullDebug -Pdebug.abi=arm64-v8a
```

Ожидание: BUILD SUCCESSFUL.

- [ ] **Step 6: Проверить на телефоне**

```bash
adb -s e3bacc6b install -r -d android/app/build/outputs/apk/full/debug/app-full-arm64-v8a-debug.apk
adb -s e3bacc6b shell am start -n com.resultv.android/.MainActivity
adb -s e3bacc6b exec-out screencap -p > /c/Users/andbe/AppData/Local/Temp/claude/speed-idle.png
# подключиться, снять ещё раз
adb -s e3bacc6b exec-out screencap -p > /c/Users/andbe/AppData/Local/Temp/claude/speed-active.png
```

Смотреть: плитки видны и до подключения; без соединения цифры приглушены и стоит плоская линия; страница при подключении не меняет высоту.

- [ ] **Step 7: Коммит**

```bash
git add android/app/src/main/java/com/resultv/android/ui/
git commit -m "feat(android): плитки скорости по макету ПК, видны всегда"
```

---

### Task 8: Строка сервера

**Files:**
- Modify: `android/app/src/main/java/com/resultv/android/ui/components/ServerRow.kt` (переписать сигнатуру и тело)
- Modify: `android/app/src/main/java/com/resultv/android/ui/screens/HomeScreen.kt:516-528` (вызов), `:691` (удалить `profileSubtitle`)
- Modify: `android/app/src/main/java/com/resultv/android/ui/screens/ProxiesScreen.kt:453-465`, `:585-597`
- Modify: `android/app/src/main/java/com/resultv/android/vpn/Profile.kt` (удалить `subtitle` и `computeSubtitle`)
- Modify: `android/app/src/main/res/values/strings.xml`, `values-ru/strings.xml` (добавить `badge_auto`)

**Interfaces:**
- Consumes: `serverDisplayName()`, `Profile.badges` из Task 1; `HomeLook` из Task 4
- Produces:
  ```kotlin
  @Composable fun ServerRow(
      name: String,
      badges: List<String>,
      countryCode: String?,
      isAuto: Boolean,
      isActive: Boolean,
      isFavorite: Boolean,
      onClick: () -> Unit,
      accent: HomeLook = HomeLook.Idle,
      trailing: @Composable (() -> Unit)? = null,
      latencyMs: Int? = null,
      offlineReason: String? = null,
      isLoading: Boolean = false,
      onLongClick: (() -> Unit)? = null,
  )
  ```
  плюс `@Composable fun ProtocolBadge(text: String, first: Boolean, accent: HomeLook)` в том же файле.

- [ ] **Step 1: Добавить строку «Авто»**

В `main/res/values/strings.xml`:

```xml
    <string name="badge_auto">Auto</string>
```

В `main/res/values-ru/strings.xml`:

```xml
    <string name="badge_auto">Авто</string>
```

- [ ] **Step 2: Добавить бейдж в `ServerRow.kt`**

```kotlin
/**
 * Бейдж протокола — перенос `Badge` (Figma 6503:3035) из кита ПК.
 *
 * Первый бейдж ярче остальных: в макете это два разных варианта, First и
 * Second. Разрядка 2 % — правило дизайнера поверх макета: имена протоколов
 * набраны латиницей в верхнем регистре и без неё слипаются.
 */
@Composable
fun ProtocolBadge(text: String, first: Boolean, accent: HomeLook) {
    val bg = when (accent) {
        HomeLook.Success -> RvColor.mainA10
        HomeLook.Processing -> RvColor.warningA10
        HomeLook.Error -> RvColor.errorsA10
        HomeLook.Idle -> if (first) RvColor.LightGray else RvColor.lightGrayA50
    }
    val fg = when (accent) {
        HomeLook.Success -> RvColor.Main
        HomeLook.Processing -> RvColor.Warning
        HomeLook.Error -> RvColor.Errors
        HomeLook.Idle -> RvColor.whiteA50
    }
    Box(
        modifier = Modifier
            .height(20.dp)
            .clip(RoundedCornerShape(percent = 50))
            .background(bg)
            .padding(horizontal = RvSpace.nest3),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            text = text,
            style = MaterialTheme.typography.labelSmall,
            fontWeight = FontWeight.SemiBold,
            letterSpacing = 0.02.em,
            color = fg,
            maxLines = 1,
        )
    }
}
```

Нужные импорты: `androidx.compose.ui.text.font.FontWeight`, `androidx.compose.ui.unit.em`, `androidx.compose.foundation.layout.height`.

- [ ] **Step 3: Переписать `ServerRow`**

Заменить сигнатуру и тело:

```kotlin
@OptIn(ExperimentalFoundationApi::class)
@Composable
fun ServerRow(
    name: String,
    badges: List<String>,
    countryCode: String?,
    isAuto: Boolean,
    isActive: Boolean,
    isFavorite: Boolean,
    onClick: () -> Unit,
    /** Подсветка плитки флага и бейджей под состояние подключения. */
    accent: HomeLook = HomeLook.Idle,
    trailing: @Composable (() -> Unit)? = null,
    latencyMs: Int? = null,
    offlineReason: String? = null,
    isLoading: Boolean = false,
    onLongClick: (() -> Unit)? = null,
) {
    // Подключённый сервер выходит из прозрачности на ту же подложку, что и
    // остальные строки под касанием, — по ней его и находят глазами среди
    // прочих (ResultV-dev ServerItem.css:86-96). Зелёным его метят плитка
    // флага и бейдж, а не цвет имени.
    val bg = if (isActive) RvColor.DarkGrey else RvColor.Black.copy(alpha = 0.7f)
    val tile = when {
        accent == HomeLook.Success -> RvColor.mainA10
        accent == HomeLook.Processing -> RvColor.warningA10
        accent == HomeLook.Error -> RvColor.errorsA10
        else -> RvColor.LightGray
    }

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .height(64.dp)
            .background(bg)
            .let { base ->
                if (onLongClick != null)
                    base.combinedClickable(onClick = onClick, onLongClick = onLongClick)
                else
                    base.clickable(onClick = onClick)
            }
            .padding(horizontal = RvSpace.nest2),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2),
    ) {
        Box(
            modifier = Modifier
                .size(44.dp)
                .clip(RoundedCornerShape(RvRadius.chip))
                .background(tile)
                .rvBorder(RoundedCornerShape(RvRadius.chip)),
            contentAlignment = Alignment.Center,
        ) {
            when {
                isAuto -> Icon(
                    imageVector = Icons.Filled.Bolt,
                    contentDescription = null,
                    tint = if (accent == HomeLook.Idle) RvColor.Second else RvColor.Main,
                    modifier = Modifier.size(22.dp),
                )
                countryCode != null -> Text(
                    text = flagFromCountry(countryCode),
                    style = MaterialTheme.typography.titleLarge,
                )
                else -> Icon(
                    imageVector = Icons.Outlined.Public,
                    contentDescription = null,
                    tint = RvColor.whiteA50,
                    modifier = Modifier.size(22.dp),
                )
            }
        }

        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(RvSpace.xs),
        ) {
            val shown = if (isAuto) listOf(stringResource(R.string.badge_auto)) else badges
            if (shown.isNotEmpty()) {
                Row(horizontalArrangement = Arrangement.spacedBy(RvSpace.xs)) {
                    shown.forEachIndexed { i, b ->
                        ProtocolBadge(text = b, first = i == 0, accent = accent)
                    }
                }
            }
            Text(
                text = name,
                color = RvColor.White,
                style = MaterialTheme.typography.titleSmall,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }

        if (isFavorite) {
            Icon(
                imageVector = Icons.Filled.Star,
                contentDescription = stringResource(R.string.action_unfavorite),
                tint = RvColor.Warning,
                modifier = Modifier.size(14.dp),
            )
        }

        // Задержка набрана одним цветом, как на ПК: белым 50 %. Цветовая
        // шкала по порогам снята осознанно, см. P-4 спеки.
        when {
            isLoading -> androidx.compose.material3.CircularProgressIndicator(
                modifier = Modifier.size(14.dp),
                color = RvColor.whiteA50,
                strokeWidth = 2.dp,
            )
            latencyMs != null -> Text(
                text = if (latencyMs <= 0) stringResource(R.string.ping_online) else "$latencyMs ms",
                style = MaterialTheme.typography.labelMedium,
                color = RvColor.whiteA50,
            )
            offlineReason != null -> Text(
                text = offlineLabel(offlineReason),
                style = MaterialTheme.typography.labelMedium,
                color = RvColor.Errors,
            )
            else -> androidx.compose.material3.CircularProgressIndicator(
                modifier = Modifier.size(14.dp),
                color = RvColor.whiteA50,
                strokeWidth = 2.dp,
            )
        }

        if (trailing != null) trailing()
    }
}
```

Функция `offlineLabel` и `flagFromCountry` в файле остаются без изменений. Снять импорты `border` и `RvRadius.card`, если осиротели; добавить `com.resultv.android.theme.rvBorder`, `androidx.compose.foundation.layout.height`.

- [ ] **Step 4: Добавить `accent` в `ProfileDropdown` и обновить вызов на главной**

В `HomeScreen.kt` сигнатура `ProfileDropdown` получает новый параметр — сразу после `activeId`:

```kotlin
private fun ProfileDropdown(
    profiles: List<Profile>,
    subscriptions: List<Subscription>,
    activeId: String?,
    /** Состояние подключения — им подсвечивается строка выбранного сервера. */
    accent: HomeLook,
    pings: Map<String, PingRepository.Sample>,
    pingInflight: Set<String>,
    countries: Map<String, String>,
    sortMode: ProfileSortMode,
    onSelect: (Profile) -> Unit,
    onLongPress: (Profile) -> Unit,
) {
```

Вызов `ProfileDropdown` в теле `HomeScreen` получает `accent = homeLook(status),`.

Внутри `ProfileDropdown` строка списка:

```kotlin
                        ServerRow(
                            name = serverDisplayName(p.name, p.country ?: countries[p.id]),
                            badges = p.badges,
                            countryCode = p.country ?: countries[p.id],
                            isAuto = p.isAuto,
                            isActive = p.id == activeId,
                            isFavorite = p.isFavorite,
                            accent = if (p.id == activeId) accent else HomeLook.Idle,
                            onClick = { onSelect(p) },
                            onLongClick = { onLongPress(p) },
                            latencyMs = pings[p.id]?.takeIf { it.reachable }?.latencyMs,
                            offlineReason = pings[p.id]?.takeUnless { it.reachable }?.reason,
                            isLoading = p.id in pingInflight,
                        )
```

Импорты в `HomeScreen.kt`: `com.resultv.android.vpn.serverDisplayName`, `com.resultv.android.ui.components.HomeLook`, `com.resultv.android.ui.components.homeLook`.

- [ ] **Step 5: Обновить два вызова в `ProxiesScreen.kt`**

В обоих местах (`ProfileCard` и блок строки подписки) заменить:

```kotlin
        name = profile.name,
        subtitle = profile.subtitle,
```

на:

```kotlin
        name = serverDisplayName(profile.name, country),
        badges = profile.badges,
```

и добавить `accent = if (profile.id == activeId) HomeLook.Success else HomeLook.Idle,`. Импорты: `serverDisplayName`, `HomeLook`.

- [ ] **Step 6: Удалить осиротевшее**

- В `Profile.kt`: удалить свойство `val subtitle` и приватную функцию `computeSubtitle()`.
- В `HomeScreen.kt`: удалить `internal fun profileSubtitle(p: Profile): String = p.subtitle`.

`profileProtocol` не трогать: он был мёртвым и до этой правки — не наша уборка.

- [ ] **Step 7: Собрать и прогнать все тесты**

```bash
cd android && ./gradlew :app:testFullDebugUnitTest :app:assembleFullDebug -Pdebug.abi=arm64-v8a
```

Ожидание: BUILD SUCCESSFUL, все тесты зелёные. `ProfileDisplayTest` из Task 1 продолжает проходить — `badges` никуда не делся.

- [ ] **Step 8: Проверить на телефоне**

```bash
adb -s e3bacc6b install -r -d android/app/build/outputs/apk/full/debug/app-full-arm64-v8a-debug.apk
adb -s e3bacc6b shell am start -n com.resultv.android/.MainActivity
adb -s e3bacc6b exec-out screencap -p > /c/Users/andbe/AppData/Local/Temp/claude/rows-home.png
# вкладка «Прокси»
adb -s e3bacc6b exec-out screencap -p > /c/Users/andbe/AppData/Local/Temp/claude/rows-proxies.png
```

Смотреть: у строк бейджи над именем; флагов в именах нет; подключённая строка светлее прочих, её плитка и бейдж зелёные, имя белое.

- [ ] **Step 9: Коммит**

```bash
git add android/app/src/
git commit -m "feat(android): строка сервера с бейджами и подсветкой по макету ПК"
```

---

### Task 9: Карточка списка на главной

**Files:**
- Modify: `android/app/src/main/java/com/resultv/android/ui/screens/HomeScreen.kt:148-190` (панель инструментов и `Card`), `:400-465` (`ActiveProfileRow`), `:590-640` (`HomeGroupHeader`)

**Interfaces:**
- Consumes: `HomeLook`, `serverDisplayName`, `Profile.badges`, `ProtocolBadge` из предыдущих задач

- [ ] **Step 1: Перекрасить карточку**

В `HomeScreen.kt` у `Card` списка заменить `containerColor = RvColor.Grey` на `RvColor.Black`, форму — на `RoundedCornerShape(RvRadius.panel)`, и добавить обрезку содержимого внутренней обёрткой:

```kotlin
        val listShape = RoundedCornerShape(RvRadius.panel)
        Card(
            shape = listShape,
            colors = CardDefaults.cardColors(containerColor = RvColor.Black),
            modifier = Modifier.fillMaxWidth().rvBorder(listShape),
        ) {
            // Обрезка живёт внутри, а не на карточке: обрезка по внешнему
            // краю съела бы собственную обводку вместе со сглаживанием.
            Column(modifier = Modifier.clip(listShape)) {
                ActiveProfileRow(
                    active = active,
                    activeCountry = active?.let { it.country ?: countries[it.id] },
                    accent = homeLook(status),
                    expanded = dropdownOpen,
                    onToggle = { dropdownOpen = !dropdownOpen },
                    onPing = { PingRepository.refreshAll(profilesState.profiles) },
                    sortMode = sortMode,
                    onSortModeChange = { sortMode = it },
                )
                AnimatedVisibility(visible = dropdownOpen) {
                    ProfileDropdown(
                        profiles = visibleHomeProfiles,
                        subscriptions = subsState.subs,
                        activeId = profilesState.activeId,
                        accent = homeLook(status),
                        pings = pings,
                        pingInflight = pingInflight,
                        countries = countries,
                        sortMode = sortMode,
                        onSelect = {
                            ProfileRepository.setActive(it.id)
                            dropdownOpen = false
                        },
                        onLongPress = { editingProfileId = it.id },
                    )
                }
            }
        }
```

- [ ] **Step 2: Перенести кнопки замера и сортировки в шапку карточки**

Удалить из `HomeScreen` блок `Row(modifier = Modifier.fillMaxWidth().height(36.dp)) { ... }` целиком вместе с кнопкой `IconButton` замера и `ProfileSortMenu`.

`ActiveProfileRow` получает три новых параметра и рисует их справа, только когда список раскрыт:

```kotlin
@Composable
private fun ActiveProfileRow(
    active: Profile?,
    activeCountry: String?,
    accent: HomeLook,
    expanded: Boolean,
    onToggle: () -> Unit,
    onPing: () -> Unit,
    sortMode: ProfileSortMode,
    onSortModeChange: (ProfileSortMode) -> Unit,
) {
```

В правой части строки, перед шевроном:

```kotlin
        if (expanded) {
            IconButton(onClick = onPing, modifier = Modifier.size(36.dp)) {
                Icon(
                    imageVector = Icons.Outlined.Bolt,
                    contentDescription = stringResource(R.string.ping_refresh_cd),
                    tint = RvColor.whiteA50,
                    modifier = Modifier.size(20.dp),
                )
            }
            ProfileSortMenu(mode = sortMode, onModeChange = onSortModeChange)
        }
```

Шеврон разворачивается поворотом:

```kotlin
        Icon(
            imageVector = Icons.Outlined.ExpandMore,
            contentDescription = stringResource(
                if (expanded) R.string.action_collapse else R.string.action_expand,
            ),
            tint = RvColor.whiteA50,
            modifier = Modifier.graphicsLayer { rotationZ = if (expanded) 180f else 0f },
        )
```

- [ ] **Step 3: Привести шапку карточки к макету**

`ActiveProfileRow`: заливка `RvColor.Grey`, высота 72 dp, плитка флага 48 dp с `rvBorder`, подпись «Текущий сервер» убирается, вместо неё бейджи активного профиля над именем:

```kotlin
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .height(72.dp)
            .background(RvColor.Grey)
            .clickable(onClick = onToggle)
            .padding(horizontal = RvSpace.nest1),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2),
    ) {
```

Плитка флага — 48 dp, цвет подложки из `accent`:

```kotlin
        val tile = when (accent) {
            HomeLook.Success -> RvColor.mainA10
            HomeLook.Processing -> RvColor.warningA10
            HomeLook.Error -> RvColor.errorsA10
            HomeLook.Idle -> RvColor.LightGray
        }
        Box(
            modifier = Modifier
                .size(48.dp)
                .clip(RoundedCornerShape(RvRadius.chip))
                .background(tile)
                .rvBorder(RoundedCornerShape(RvRadius.chip)),
            contentAlignment = Alignment.Center,
        ) {
            val country = activeCountry
            when {
                active == null -> Icon(
                    imageVector = Icons.Outlined.Public,
                    contentDescription = null,
                    tint = RvColor.whiteA50,
                    modifier = Modifier.size(24.dp),
                )
                active.isAuto -> Icon(
                    imageVector = Icons.Filled.Bolt,
                    contentDescription = null,
                    tint = if (accent == HomeLook.Idle) RvColor.Second else RvColor.Main,
                    modifier = Modifier.size(24.dp),
                )
                country != null -> Text(
                    text = flagFromCountry(country),
                    style = MaterialTheme.typography.headlineSmall,
                )
                else -> Icon(
                    imageVector = Icons.Outlined.Public,
                    contentDescription = null,
                    tint = RvColor.whiteA50,
                    modifier = Modifier.size(24.dp),
                )
            }
        }
```

Текстовый столбец:

```kotlin
        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(RvSpace.xs),
        ) {
            val shown = when {
                active == null -> emptyList()
                active.isAuto -> listOf(stringResource(R.string.badge_auto))
                else -> active.badges
            }
            if (shown.isNotEmpty()) {
                Row(horizontalArrangement = Arrangement.spacedBy(RvSpace.xs)) {
                    shown.forEachIndexed { i, b ->
                        ProtocolBadge(text = b, first = i == 0, accent = accent)
                    }
                }
            }
            Text(
                text = active?.let { serverDisplayName(it.name, activeCountry) }
                    ?: stringResource(R.string.home_no_profile_selected),
                style = MaterialTheme.typography.titleMedium,
                color = RvColor.White,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
```

Строка `home_current_server` после этого осиротеет — удалить её из обеих локалей.

- [ ] **Step 4: Поправить подписи групп**

В `HomeGroupHeader` заменить отступ и цвет:

```kotlin
        modifier = Modifier
            .fillMaxWidth()
            .background(RvColor.Black)
            .padding(start = RvSpace.nest1, end = RvSpace.nest1, top = RvSpace.nest2, bottom = RvSpace.xs),
```

и у обоих `Text` внутри — `color = RvColor.whiteA20`.

- [ ] **Step 5: Убрать внутренний отступ у списка**

У `Column` внутри `ProfileDropdown` снять `.padding(RvSpace.nest3)` и `verticalArrangement = Arrangement.spacedBy(RvSpace.xs)`: строки на ПК стоят вплотную, разделяет их заливка, а не воздух.

- [ ] **Step 6: Собрать**

```bash
cd android && ./gradlew :app:assembleFullDebug -Pdebug.abi=arm64-v8a
```

Ожидание: BUILD SUCCESSFUL.

- [ ] **Step 7: Проверить на телефоне**

```bash
adb -s e3bacc6b install -r -d android/app/build/outputs/apk/full/debug/app-full-arm64-v8a-debug.apk
adb -s e3bacc6b shell am start -n com.resultv.android/.MainActivity
adb -s e3bacc6b exec-out screencap -p > /c/Users/andbe/AppData/Local/Temp/claude/card-collapsed.png
# раскрыть список
adb -s e3bacc6b exec-out screencap -p > /c/Users/andbe/AppData/Local/Temp/claude/card-expanded.png
```

Смотреть: свёрнутая карточка — одна строка с бейджами; раскрытая показывает кнопки замера и сортировки в шапке, шеврон смотрит вверх, строки идут вплотную, обводка карточки цела по всему контуру.

- [ ] **Step 8: Коммит**

```bash
git add android/app/src/
git commit -m "feat(android): карточка списка серверов на главной по макету ПК"
```

---

### Task 10: Волна

Свести задержки из Task 4 во все четыре блока.

**Files:**
- Modify: `android/app/src/main/java/com/resultv/android/ui/components/HomeHeader.kt` (заголовок и плашка времени)
- Modify: `android/app/src/main/java/com/resultv/android/ui/components/ServerRow.kt` (плитка флага и бейджи)
- Modify: `android/app/src/main/java/com/resultv/android/ui/components/SpeedTile.kt` (блок цифр и линия)

**Interfaces:**
- Consumes: `waveDelayMillis(step, connected)`, `WaveStep` из Task 4

- [ ] **Step 1: Задержать заголовок и время в `HomeHeader.kt`**

```kotlin
    val connected = look == HomeLook.Success
    val waveEnabled = rememberWaveEnabled()
    val titleColor by animateColorAsState(
        targetValue = when (look) {
            HomeLook.Idle -> RvColor.whiteA50
            HomeLook.Processing -> RvColor.Warning
            HomeLook.Success -> RvColor.Main
            HomeLook.Error -> RvColor.Errors
        },
        animationSpec = tween(
            durationMillis = RvMotion.durationMillis,
            delayMillis = if (waveEnabled) waveDelayMillis(WaveStep.Title, connected) else 0,
            easing = RvMotion.easing,
        ),
        label = "headerTitle",
    )
```

- [ ] **Step 2: Уронить плашку времени сверху**

На ПК время не проявляется, а выпадает сверху из-за края окна. Обернуть `UptimeChip` в `AnimatedVisibility`:

```kotlin
            androidx.compose.animation.AnimatedVisibility(
                visible = status is VpnStatus.Connected,
                enter = androidx.compose.animation.slideInVertically(
                    animationSpec = tween(
                        durationMillis = RvMotion.durationMillis,
                        delayMillis = waveDelayMillis(WaveStep.Time, connected = true),
                        easing = RvMotion.easing,
                    ),
                    initialOffsetY = { -it },
                ),
                exit = androidx.compose.animation.slideOutVertically(
                    animationSpec = tween(
                        durationMillis = RvMotion.durationMillis,
                        delayMillis = waveDelayMillis(WaveStep.Time, connected = false),
                        easing = RvMotion.easing,
                    ),
                    targetOffsetY = { -it },
                ),
            ) {
                (status as? VpnStatus.Connected)?.let { UptimeChip(connectedAt = it.connectedAt) }
            }
```

`AnimatedVisibility` обрезает по своим границам — путь плашки не наезжает на заголовок, как и `clip-path` на ПК.

- [ ] **Step 3: Задержать подсветку строки в `ServerRow.kt`**

Плитку флага и бейджи перевести на анимированный цвет с задержкой `WaveStep.Card`:

```kotlin
    val connected = accent == HomeLook.Success
    val tileColor by animateColorAsState(
        targetValue = tile,
        animationSpec = tween(
            durationMillis = RvMotion.durationMillis,
            delayMillis = waveDelayMillis(WaveStep.Card, connected),
            easing = RvMotion.easing,
        ),
        label = "rowTile",
    )
```

и использовать `tileColor` вместо `tile` в `.background(tileColor)` у плитки флага.

В `ProtocolBadge` — так же для обоих цветов:

```kotlin
@Composable
fun ProtocolBadge(text: String, first: Boolean, accent: HomeLook) {
    val waveEnabled = rememberWaveEnabled()
    val delay = if (waveEnabled) {
        waveDelayMillis(WaveStep.Card, connected = accent == HomeLook.Success)
    } else 0
    val spec = tween<Color>(RvMotion.durationMillis, delay, RvMotion.easing)

    val bg by animateColorAsState(
        targetValue = when (accent) {
            HomeLook.Success -> RvColor.mainA10
            HomeLook.Processing -> RvColor.warningA10
            HomeLook.Error -> RvColor.errorsA10
            HomeLook.Idle -> if (first) RvColor.LightGray else RvColor.lightGrayA50
        },
        animationSpec = spec, label = "badgeBg",
    )
    val fg by animateColorAsState(
        targetValue = when (accent) {
            HomeLook.Success -> RvColor.Main
            HomeLook.Processing -> RvColor.Warning
            HomeLook.Error -> RvColor.Errors
            HomeLook.Idle -> RvColor.whiteA50
        },
        animationSpec = spec, label = "badgeFg",
    )
```

дальше тело бейджа из Task 8 без изменений.

- [ ] **Step 4: Задержать плитки скорости**

В `SpeedTile.kt` у `dataAlpha` добавить задержку:

```kotlin
        animationSpec = tween(
            durationMillis = RvMotion.durationMillis,
            delayMillis = waveDelayMillis(WaveStep.Speed, connected = active),
            easing = RvMotion.easing,
        ),
```

- [ ] **Step 5: Уважить системную «уменьшенную анимацию»**

В `HomeStatus.kt` добавить чтение системного масштаба и обнулять задержки, когда анимации выключены. Файл при этом впервые получает Compose-импорты — `androidx.compose.runtime.Composable`, `androidx.compose.runtime.remember`, `androidx.compose.ui.platform.LocalContext`:

```kotlin
/**
 * Кому движение мешает — тому его не показываем. На ПК это
 * `prefers-reduced-motion`, здесь — системный масштаб анимаций: при нуле
 * волна схлопывается в мгновенную смену состояния.
 */
@Composable
fun rememberWaveEnabled(): Boolean {
    val ctx = LocalContext.current
    return remember(ctx) {
        android.provider.Settings.Global.getFloat(
            ctx.contentResolver,
            android.provider.Settings.Global.ANIMATOR_DURATION_SCALE,
            1f,
        ) != 0f
    }
}
```

В трёх местах выше задержку брать как `if (waveEnabled) waveDelayMillis(...) else 0`, где `val waveEnabled = rememberWaveEnabled()`.

- [ ] **Step 6: Собрать и прогнать все тесты**

```bash
cd android && ./gradlew :app:testFullDebugUnitTest :app:testPlayDebugUnitTest :app:assembleFullDebug :app:assemblePlayDebug -Pdebug.abi=arm64-v8a
```

Ожидание: BUILD SUCCESSFUL, все тесты зелёные, оба флейвора собираются.

- [ ] **Step 7: Проверить волну на телефоне**

```bash
adb -s e3bacc6b install -r -d android/app/build/outputs/apk/full/debug/app-full-arm64-v8a-debug.apk
adb -s e3bacc6b shell am start -n com.resultv.android/.MainActivity
adb -s e3bacc6b shell screenrecord --time-limit 8 /sdcard/wave.mp4 &
# нажать подключение, дождаться успеха, затем отключить
MSYS_NO_PATHCONV=1 adb -s e3bacc6b pull /sdcard/wave.mp4 /c/Users/andbe/AppData/Local/Temp/claude/wave.mp4
```

Смотреть: при подключении сначала отзывается кнопка, последними плитки; при отключении порядок обратный; вся волна читается как порядок, а не как задержка.

- [ ] **Step 8: Коммит**

```bash
git add android/app/src/main/java/com/resultv/android/ui/
git commit -m "feat(android): волна смены состояния на главной, шаг 70 мс"
```

---

## Приёмка целиком

После Task 10 пройти список из раздела «Проверка» спеки:

```bash
cd android && ./gradlew :app:testFullDebugUnitTest :app:testPlayDebugUnitTest
```

Снимки семи экранов: idle, connecting, connected, error, раскрытый список серверов, главная страница настроек, раздел пинга.

Глазами проверить то, что тестом не берётся:

- страница не меняет высоту при подключении;
- заголовок «Что-то пошло не так» помещается в ряд;
- волна читается как порядок;
- обводка карточки списка цела по всему контуру, включая углы.
