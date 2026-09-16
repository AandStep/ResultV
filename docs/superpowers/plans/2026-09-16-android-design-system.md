# Перенос дизайн-системы ПК на Android — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Перевести Android-приложение на дизайн-систему ПК — цвета, обводки, скругления, отступы, типографику — и сократить тексты настроек до двух строк.

**Architecture:** Плоский слой токенов `theme/Tokens.kt` как зеркало `tokens.css`, плюс `theme/Border.kt` с модификаторами обводки и нажатия. `theme/Theme.kt` кормит теми же значениями `ColorScheme`, `Typography` и `Shapes`, чтобы компоненты Material красились сами. Старый объект `Brand` на время перевода становится набором псевдонимов, и его удаление в последней задаче служит доказательством полноты.

**Tech Stack:** Kotlin, Jetpack Compose, Material 3, Gradle (AGP flavours `full` / `play`), JUnit 4 на JVM без Robolectric.

**Spec:** `docs/superpowers/specs/2026-09-16-android-design-system-design.md`

## Global Constraints

- Все пути даны от корня репозитория `C:\ResultV`. Модуль Android — `android/`, Gradle-проект — `android/app`.
- Go-код не трогается. Пересборка AAR (`scripts/build-android-aar.sh`) **не нужна**.
- Правки строк идут в **обе** локали одновременно: `values/strings.xml` и `values-ru/strings.xml`, в том сорссете, которому строка принадлежит (`main` или `full`).
- Любая строка `settings_*`, оканчивающаяся на `_desc`, `_subtitle`, `_hint`, `_items` или `_warning`, — **не длиннее 64 символов**.
- Комментарии в коде — по-русски, как в остальном модуле. Имена тестовых методов — английские camelCase, как в существующих тестах (`app/src/test/java/com/resultv/android/ui/components/TagFieldTest.kt`).
- Иконки остаются `androidx.compose.material.icons` (Material Symbols). MDI не подключается.
- Сборки проверяются обе: `full` и `play`.
- Коммиты — по-русски, с завершающей строкой `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>`.

## Структура файлов

| Файл | Ответственность |
|---|---|
| `android/app/src/main/java/com/resultv/android/theme/Tokens.kt` | **создаётся** — `RvColor`, `RvCategory`, `RvSpace`, `RvRadius`, `RvIcon`, `RvMotion`. Зеркало `tokens.css`. Единственное место с литералами `Color(0x…)` |
| `android/app/src/main/java/com/resultv/android/theme/Border.kt` | **создаётся** — `Modifier.rvBorder()`, `Modifier.rvPress()` |
| `android/app/src/main/java/com/resultv/android/theme/Theme.kt` | **переписывается** — `ColorScheme`, `Typography`, `Shapes` из токенов |
| `android/app/src/main/java/com/resultv/android/theme/Color.kt` | `Brand` становится псевдонимами (задача 2), **удаляется** (задача 10) |
| `android/app/src/main/res/font/benzin_bold.ttf` | **создаётся** — копия из `ResultV-dev/frontend/src/assets/fonts/benzin-bold.ttf` |
| `android/app/src/test/java/com/resultv/android/res/SettingsStringsTest.kt` | **создаётся** — длина и паритет локалей |
| `android/app/src/test/java/com/resultv/android/theme/TokensTest.kt` | **создаётся** — целостность шкал |

---

### Task 1: Тесты на тексты настроек и сокращение строк

Первой идёт именно эта задача: тесты падают на сегодняшних строках по-настоящему (`settings_dns_hint` — 158 символов при пределе 64), поэтому красно-зелёный цикл здесь не декорация.

**Files:**
- Create: `android/app/src/test/java/com/resultv/android/res/SettingsStringsTest.kt`
- Modify: `android/app/src/main/res/values/strings.xml`
- Modify: `android/app/src/main/res/values-ru/strings.xml`
- Modify: `android/app/src/full/res/values/strings.xml`
- Modify: `android/app/src/full/res/values-ru/strings.xml`

**Interfaces:**
- Consumes: ничего.
- Produces: строки `settings_group_network_items`, `settings_group_routing_items`, `settings_group_security_items`, `settings_group_adblock_items`, `settings_group_appearance_items`, `settings_group_subscriptions_items`, `settings_dns_private_warning`, `settings_cat_connection`, `settings_cat_security`, `settings_cat_app` — их читает задача 5.

- [ ] **Step 1: Написать падающий тест**

Создать `android/app/src/test/java/com/resultv/android/res/SettingsStringsTest.kt`:

```kotlin
package com.resultv.android.res

import org.junit.Assert.assertEquals
import org.junit.Test
import java.io.File

/**
 * Тексты настроек проверяются по файлу ресурсов, а не через `R.string`:
 * юнит-тесты модуля идут на голой JVM без Robolectric, и разрешать
 * идентификаторы ресурсов здесь нечем. XML разбирается регуляркой — файл
 * плоский, настоящий парсер был бы тяжелее задачи.
 *
 * Оба теста сужены до префикса `settings_`. Вне его паритет локалей УЖЕ
 * нарушен: `awg_*` (имена полей AmneziaWG) и `log_source_*` лежат только в
 * английском файле. Это не следствие этой правки, и расширять охват здесь
 * значит красить чужой забор.
 */
class SettingsStringsTest {

    private companion object {
        /**
         * Одна строка подписи — примерно 32 символа при 13sp на экране
         * 360 dp. Две строки, то есть предел из спеки, — 64.
         */
        const val MAX_LEN = 64

        val SUFFIXES = listOf("_desc", "_subtitle", "_hint", "_items", "_warning")

        val EN_RU = listOf(
            "main/res/values/strings.xml" to "main/res/values-ru/strings.xml",
            "full/res/values/strings.xml" to "full/res/values-ru/strings.xml",
        )

        val STRING_RE = Regex(
            """<string name="([^"]+)"[^>]*>(.*?)</string>""",
            RegexOption.DOT_MATCHES_ALL,
        )
        val TAG_RE = Regex("<[^>]+>")
    }

    /**
     * Рабочий каталог юнит-теста задаёт Gradle, и у AGP это каталог модуля
     * (`android/app`). Запуск из IDE иногда стартует от корня проекта,
     * поэтому проверяются оба варианта.
     */
    private fun resFile(relative: String): File =
        listOf(File("src/$relative"), File("app/src/$relative"), File("android/app/src/$relative"))
            .firstOrNull { it.isFile }
            ?: error("не найден файл ресурсов: $relative")

    private fun strings(relative: String): Map<String, String> =
        STRING_RE.findAll(resFile(relative).readText())
            .associate { it.groupValues[1] to it.groupValues[2] }

    private fun settingsNames(relative: String): Set<String> =
        strings(relative).keys.filterTo(mutableSetOf()) { it.startsWith("settings_") }

    @Test fun settingsCopyFitsTwoLines() {
        val tooLong = EN_RU.flatMap { listOf(it.first, it.second) }.flatMap { file ->
            strings(file)
                .filterKeys { name ->
                    name.startsWith("settings_") && SUFFIXES.any { name.endsWith(it) }
                }
                .mapNotNull { (name, raw) ->
                    val text = TAG_RE.replace(raw, "")
                    if (text.length > MAX_LEN) "$file · $name · ${text.length}" else null
                }
        }
        assertEquals("строки длиннее $MAX_LEN символов", emptyList<String>(), tooLong)
    }

    @Test fun bothLocalesKnowTheSameSettingsStrings() {
        val gaps = EN_RU.flatMap { (en, ru) ->
            val onlyEn = settingsNames(en) - settingsNames(ru)
            val onlyRu = settingsNames(ru) - settingsNames(en)
            onlyEn.map { "$en · только en · $it" } + onlyRu.map { "$ru · только ru · $it" }
        }
        assertEquals("набор settings_* разошёлся между локалями", emptyList<String>(), gaps)
    }
}
```

- [ ] **Step 2: Запустить тест и убедиться, что он падает**

```bash
cd android && ./gradlew :app:testFullDebugUnitTest --tests '*SettingsStringsTest*'
```

Ожидается: `settingsCopyFitsTwoLines` FAILED, в сообщении шесть строк —
`settings_dns_hint` (156 и 158), `settings_group_subscriptions_desc` (69 и 75),
`settings_sub_hwid_desc` (83 и 84), `settings_sub_ua_desc` (92 и 105),
`settings_adblock_subtitle` (112 и 123), `settings_browser_adblock_subtitle`
(228 и 221). Второй тест — PASSED: паритет сегодня цел.

- [ ] **Step 3: Сократить строки в `main`, обе локали**

В `android/app/src/main/res/values-ru/strings.xml` заменить значения:

```xml
<string name="settings_dns_hint">Пресет или свои серверы через запятую.</string>
<string name="settings_sub_hwid_desc">Идентификатор устройства — для проверки лимита.</string>
<string name="settings_sub_ua_desc">Пустое поле — значение по умолчанию.</string>
<string name="settings_sub_auto_update_desc">Обновлять все подписки по расписанию.</string>
<string name="settings_bypass_lan_subtitle">Принтеры, NAS и роутер — мимо VPN</string>
<string name="settings_ipv6_subtitle">IPv6-трафик через туннель</string>
<string name="settings_killswitch_desc">Обрыв интернета при падении прокси.</string>
<string name="settings_group_subscriptions_desc">Обновление и данные для провайдера.</string>
<string name="settings_group_security_desc">Защита соединения при сбоях.</string>
<string name="settings_group_network_desc">DNS, IPv6 и локальная сеть.</string>
<string name="settings_group_logs_desc">События подключения и приложения.</string>
```

В `android/app/src/main/res/values/strings.xml`:

```xml
<string name="settings_dns_hint">Pick a preset or enter custom servers, comma-separated.</string>
<string name="settings_sub_hwid_desc">Device identifier — for the device-limit check.</string>
<string name="settings_sub_ua_desc">Empty field — the default value.</string>
<string name="settings_sub_auto_update_desc">Refresh every subscription on a schedule.</string>
<string name="settings_bypass_lan_subtitle">Printers, NAS and router bypass the VPN</string>
<string name="settings_ipv6_subtitle">IPv6 traffic through the tunnel</string>
<string name="settings_killswitch_desc">Cuts the internet if the proxy drops.</string>
<string name="settings_group_subscriptions_desc">Refresh and the data sent to the provider.</string>
<string name="settings_group_security_desc">Connection protection when the proxy drops.</string>
<string name="settings_group_network_desc">DNS, IPv6 and local network.</string>
<string name="settings_group_logs_desc">Connection and app event history.</string>
```

- [ ] **Step 4: Сократить строки в `full`, обе локали**

В `android/app/src/full/res/values-ru/strings.xml`:

```xml
<string name="settings_adblock_subtitle">Реклама и трекеры через DNS, включая YouTube.</string>
<string name="settings_browser_adblock_subtitle">Баннеры в Chrome. Нужен сертификат; YouTube не затронут.</string>
```

В `android/app/src/full/res/values/strings.xml`:

```xml
<string name="settings_adblock_subtitle">Ads and trackers via DNS, including YouTube.</string>
<string name="settings_browser_adblock_subtitle">Banners in Chrome. Needs a certificate; not YouTube.</string>
```

Подробности из длинной подписи не теряются: их дословно повторяет мастер
установки сертификата, который открывается тут же — `cert_wizard_why_body`,
`cert_wizard_safety_body`, `cert_wizard_install_step4`, `cert_wizard_done_body`.

- [ ] **Step 5: Добавить новые строки**

В `android/app/src/main/res/values-ru/strings.xml` дописать:

```xml
<string name="settings_dns_private_warning">Не работает? Отключите частный DNS в настройках сети Android.</string>
<string name="settings_cat_connection">Соединение и маршрутизация</string>
<string name="settings_cat_security">Безопасность</string>
<string name="settings_cat_app">Приложение</string>
<string name="settings_group_network_items">• DNS • IPv6 • Прямой LAN</string>
<string name="settings_group_routing_items">• Умный режим • Домены • По приложениям</string>
<string name="settings_group_security_items">• Kill Switch</string>
<string name="settings_group_appearance_items">• Язык</string>
<string name="settings_group_subscriptions_items">• Обновление • HWID • UA</string>
```

В `android/app/src/main/res/values/strings.xml`:

```xml
<string name="settings_dns_private_warning">Not working? Set Private DNS to Off in Android network settings.</string>
<string name="settings_cat_connection">Connection and routing</string>
<string name="settings_cat_security">Security</string>
<string name="settings_cat_app">App</string>
<string name="settings_group_network_items">• DNS • IPv6 • Bypass LAN</string>
<string name="settings_group_routing_items">• Smart mode • Domains • Per-app</string>
<string name="settings_group_security_items">• Kill Switch</string>
<string name="settings_group_appearance_items">• Language</string>
<string name="settings_group_subscriptions_items">• Refresh • HWID • UA</string>
```

Строка блокировки рекламы живёт в `full` — в `android/app/src/full/res/values-ru/strings.xml`:

```xml
<string name="settings_group_adblock_items">• DNS-фильтр • Браузер • Сертификат</string>
```

и в `android/app/src/full/res/values/strings.xml`:

```xml
<string name="settings_group_adblock_items">• DNS filter • Browser • Certificate</string>
```

- [ ] **Step 6: Снять дубль заголовка и удалить осиротевшую строку**

Удалить блок `SettingsScreen.kt:109-114` целиком — `Text(stringResource(R.string.settings_title), …)`
вместе с пустой строкой после него. `TopAppBar` уже рисует «Настройки»
(`tab_settings`), и заголовок показывался дважды.

После этого `settings_title` не используется нигде: удалить её из обеих локалей
`main`.

Остальные осиротевшие строки — `settings_group_advanced`,
`settings_group_advanced_desc`, `settings_appearance_language_desc` — **здесь не
трогать**. На них ещё ссылается живой код (`SettingsSubcategory.Advanced` и
`AppearanceGroup`), и их удаление сейчас уронит компиляцию. Они уходят в задаче 5,
вместе с кодом, который их держит.

`settings_desc` и `settings_back` не трогать вовсе — они мертвы и без этой правки.

- [ ] **Step 7: Запустить тесты и убедиться, что они проходят**

```bash
cd android && ./gradlew :app:testFullDebugUnitTest --tests '*SettingsStringsTest*'
```

Ожидается: BUILD SUCCESSFUL, оба теста PASSED.

- [ ] **Step 8: Собрать обе сборки**

```bash
cd android && ./gradlew :app:assembleFullDebug :app:assemblePlayDebug -Pdebug.abi=arm64-v8a
```

Ожидается: BUILD SUCCESSFUL обеих. Провал с `Unresolved reference: settings_title`
означает, что блок в шаге 6 удалён не полностью.

- [ ] **Step 9: Коммит**

```bash
cd /c/ResultV && git add android/app/src/test/java/com/resultv/android/res/SettingsStringsTest.kt android/app/src/main/res android/app/src/full/res android/app/src/main/java/com/resultv/android/ui/screens/SettingsScreen.kt && git commit -F - <<'EOF'
fix(android): сократить тексты настроек и закрепить это тестом

Шесть строк выходили за две строки экрана, худшая — подпись браузерного
адблока на 221 символ. Она дословно дублировала мастер установки
сертификата, который открывается тут же, поэтому режется без потерь.

Тест проверяет две вещи по файлу ресурсов: длину не больше 64 символов
и совпадение набора settings_* между локалями. Охват сужен до settings_*
намеренно — вне его паритет уже нарушен на awg_* и log_source_*.

Заодно снят дубль заголовка "Настройки приложения": TopAppBar уже рисует
"Настройки".

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
```

---

### Task 2: Слой токенов

**Files:**
- Create: `android/app/src/main/java/com/resultv/android/theme/Tokens.kt`
- Create: `android/app/src/main/java/com/resultv/android/theme/Border.kt`
- Create: `android/app/src/test/java/com/resultv/android/theme/TokensTest.kt`
- Create: `android/app/src/main/res/font/benzin_bold.ttf`
- Modify: `android/app/src/main/java/com/resultv/android/theme/Theme.kt` (переписывается целиком)
- Modify: `android/app/src/main/java/com/resultv/android/theme/Color.kt` (`Brand` → псевдонимы)

**Interfaces:**
- Consumes: ничего.
- Produces: `RvColor`, `RvCategory`, `RvSpace`, `RvRadius`, `RvIcon`, `RvMotion`, `RvType`, `Modifier.rvBorder(shape: Shape)`, `Modifier.rvPress(pressed: Boolean, shape: Shape)`, `BenzinBold: FontFamily`. Всё это читают задачи 3–9.

- [ ] **Step 1: Написать падающий тест на целостность шкал**

Создать `android/app/src/test/java/com/resultv/android/theme/TokensTest.kt`:

```kotlin
package com.resultv.android.theme

import androidx.compose.ui.unit.dp
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Шкалы токенов — двадцать с лишним чисел, набранных руками с таблицы. Тест
 * ловит не «неправильное значение» (это было бы повторением кода другими
 * словами), а сломанную ЛОГИКУ шкалы: ступень, которая не убывает, или две
 * совпавшие ступени. Совпадение особенно коварно: если фон карточки случайно
 * станет равен фону экрана, разница исчезнет молча, без единой ошибки.
 */
class TokensTest {

    private fun <T : Comparable<T>> assertDescendingAndDistinct(name: String, ladder: List<T>) {
        assertEquals("$name: ступени должны убывать", ladder.sortedDescending(), ladder)
        assertEquals("$name: ступени должны различаться", ladder.distinct(), ladder)
    }

    @Test fun spacingLadderDescends() {
        assertDescendingAndDistinct(
            "RvSpace",
            listOf(RvSpace.page, RvSpace.nest1, RvSpace.nest2, RvSpace.nest3, RvSpace.xs),
        )
    }

    @Test fun radiusLadderDescends() {
        assertDescendingAndDistinct(
            "RvRadius",
            listOf(
                RvRadius.panel, RvRadius.card, RvRadius.control,
                RvRadius.chip, RvRadius.small, RvRadius.hairline,
            ),
        )
    }

    /**
     * У TextUnit сравниваются `.value`, а не сами значения: оператор
     * `compareTo` у него есть, но интерфейс `Comparable<TextUnit>` он не
     * реализует, и `sortedDescending()` на списке TextUnit не компилируется.
     */
    @Test fun typeLadderDescends() {
        assertDescendingAndDistinct(
            "RvType",
            listOf(RvType.h1Size, RvType.titleSize, RvType.btnSize, RvType.regularSize, RvType.chipSize)
                .map { it.value },
        )
    }

    /**
     * Фоновая лестница: экран темнее карточки, карточка темнее ступени над
     * ней. Сравниваются сырые значения цвета — Color по яркости не
     * упорядочивается, а здесь все четыре оттенка серые и растут ровно.
     */
    @Test fun backgroundLadderHasFourDistinctSteps() {
        val ladder = listOf(RvColor.Black, RvColor.DarkGrey, RvColor.Grey, RvColor.LightGray)
        assertEquals("фоновые ступени должны различаться", ladder.distinct(), ladder)
    }

    @Test fun iconTileIsBiggerThanItsGlyph() {
        assertEquals(true, RvIcon.tile > RvIcon.glyph)
    }

    @Test fun motionMatchesTheDesktopCurve() {
        assertEquals(300, RvMotion.durationMillis)
    }

    @Test fun categoryTilesAreAllDistinct() {
        val tints = listOf(
            RvCategory.Blue, RvCategory.Red, RvCategory.Amber,
            RvCategory.Violet, RvCategory.Slate, RvCategory.Cyan, RvCategory.Emerald,
        ).map { it.glyph }
        assertEquals("цвета плиток должны различаться", tints.distinct(), tints)
    }

    @Test fun spacingMatchesTheSpecLadder() {
        assertEquals(
            listOf(24.dp, 16.dp, 12.dp, 8.dp, 4.dp),
            listOf(RvSpace.page, RvSpace.nest1, RvSpace.nest2, RvSpace.nest3, RvSpace.xs),
        )
    }
}
```

- [ ] **Step 2: Запустить тест и убедиться, что он падает**

```bash
cd android && ./gradlew :app:testFullDebugUnitTest --tests '*TokensTest*'
```

Ожидается: FAILED на компиляции — `Unresolved reference: RvSpace` и прочие. Это
и есть красная фаза: типов ещё нет.

- [ ] **Step 3: Создать `Tokens.kt`**

```kotlin
package com.resultv.android.theme

import androidx.compose.animation.core.FastOutSlowInEasing
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

/**
 * Токены дизайн-системы, зеркало `ResultV-dev/frontend/src/design/tokens.css`.
 * Источник — Figma "ResultV" -> страница App UI-kit (node 0:1).
 *
 * Правило то же, что на ПК: сюда не попадает ни одно значение, которого нет в
 * макете. Всё, что решено без макета, выписано в разделе «Пробелы» спеки
 * docs/superpowers/specs/2026-09-16-android-design-system-design.md, а не
 * додумывается здесь.
 *
 * Объект плоский, а не CompositionLocal: тема одна и тёмная, светлой в макете
 * нет вовсе. CompositionLocal платил бы сложностью за возможность, которой не
 * пользуются; надстроить его сверху, если светлая тема появится, ничто не
 * мешает.
 *
 * Это единственный файл модуля с литералами Color(0x…). Проверяется гревом,
 * см. спеку, раздел «Проверка».
 */
object RvColor {
    val White = Color(0xFFFFFFFF)
    val Black = Color(0xFF141414)      // фон экрана
    val DarkGrey = Color(0xFF171717)
    val Grey = Color(0xFF1A1A1A)       // карточка
    val LightGray = Color(0xFF1F1F1F)

    val Main = Color(0xFF007E3A)
    val Second = Color(0xFF00A819)
    val Warning = Color(0xFFF2CC0D)
    val Errors = Color(0xFFF20D46)

    // Прозрачные ступени. В Figma это не отдельные переменные, а базовый цвет
    // с непрозрачностью слоя; значения сняты с компонентов кита.
    val whiteA05 = White.copy(alpha = 0.05f)
    val whiteA10 = White.copy(alpha = 0.10f)
    val whiteA15 = White.copy(alpha = 0.15f)
    val whiteA20 = White.copy(alpha = 0.20f)
    val whiteA50 = White.copy(alpha = 0.50f)
    val whiteA80 = White.copy(alpha = 0.80f)

    val blackA20 = Black.copy(alpha = 0.20f)
    val blackA25 = Black.copy(alpha = 0.25f)
    val blackA80 = Black.copy(alpha = 0.80f)

    val mainA08 = Main.copy(alpha = 0.08f)
    val mainA10 = Main.copy(alpha = 0.10f)
    val mainA20 = Main.copy(alpha = 0.20f)
    val mainA50 = Main.copy(alpha = 0.50f)

    val warningA08 = Warning.copy(alpha = 0.08f)
    val warningA10 = Warning.copy(alpha = 0.10f)
    val warningA20 = Warning.copy(alpha = 0.20f)
    val warningA50 = Warning.copy(alpha = 0.50f)

    val errorsA08 = Errors.copy(alpha = 0.08f)
    val errorsA10 = Errors.copy(alpha = 0.10f)
    val errorsA20 = Errors.copy(alpha = 0.20f)
    val errorsA50 = Errors.copy(alpha = 0.50f)

    val secondA10 = Second.copy(alpha = 0.10f)
    val secondA50 = Second.copy(alpha = 0.50f)

    val lightGrayA50 = LightGray.copy(alpha = 0.50f)

    // Сняты с компонентов, переменными Figma не являются.
    val iconDefault = whiteA50
    val overlay = Color(0x80000000)
}

/**
 * Цвета плиток категорий. В палитре Figma их нет — это решение для телефона,
 * где список длиннее и цвет помогает найти строку глазом (пробел G-4 спеки).
 * Подложка у всех 18 %, глиф — светлый парный тон.
 */
data class CategoryTint(val tile: Color, val glyph: Color)

object RvCategory {
    val Main = CategoryTint(RvColor.Main.copy(alpha = 0.18f), RvColor.Second)
    val Blue = CategoryTint(Color(0xFF3B82F6).copy(alpha = 0.18f), Color(0xFF60A5FA))
    val Red = CategoryTint(Color(0xFFEF4444).copy(alpha = 0.18f), Color(0xFFF87171))
    val Amber = CategoryTint(Color(0xFFF59E0B).copy(alpha = 0.18f), Color(0xFFFBBF24))
    val Violet = CategoryTint(Color(0xFF8B5CF6).copy(alpha = 0.18f), Color(0xFFA78BFA))
    val Slate = CategoryTint(Color(0xFF64748B).copy(alpha = 0.18f), Color(0xFF94A3B8))
    val Cyan = CategoryTint(Color(0xFF06B6D4).copy(alpha = 0.18f), Color(0xFF22D3EE))
    val Emerald = CategoryTint(Color(0xFF10B981).copy(alpha = 0.18f), Color(0xFF34D399))
}

/**
 * Отступы. Шкала ПК (32 / 18 / 16 / 8 / 4), сдвинутая на одну ступень вниз:
 * экран телефона уже, а числа при этом остаются со своей же шкалы, а не
 * выдумываются.
 */
object RvSpace {
    val page = 24.dp
    val nest1 = 16.dp
    val nest2 = 12.dp
    val nest3 = 8.dp
    val xs = 4.dp
}

/** Скругления. Шкала ПК (32 / 24 / 16 / 14 / 8 / 2) ступенью ниже. */
object RvRadius {
    val panel = 24.dp
    val card = 20.dp
    val control = 14.dp
    val chip = 12.dp
    val small = 8.dp
    val hairline = 2.dp
}

/** Плитка значка: на ПК 68 с глифом 36, здесь ступенью ниже. */
object RvIcon {
    val tile = 40.dp
    val glyph = 20.dp
}

/**
 * Движение. Одна кривая на весь интерфейс: быстрый старт и мягкое торможение.
 * FastOutSlowInEasing — это ровно cubic-bezier(0.4, 0, 0.2, 1) из макета.
 */
object RvMotion {
    const val durationMillis = 300
    val easing = FastOutSlowInEasing
}

/**
 * Текстовые стили. Сдвиг работает не так, как в геометрии: отступы ужимаются,
 * потому что экран уже, а текст — нет, потому что глаз тот же. Садятся только
 * верхние стили.
 *
 * Стилей пять, а не шесть: `title-sm` макета после сдвига встал бы на 16sp —
 * туда же, где `btn`, который не сдвигается. Разница межстрочного, 22 против
 * 21, это шум. См. G-9 спеки.
 */
object RvType {
    val h1Size = 28.sp
    val h1Line = 34.sp

    val titleSize = 18.sp
    val titleLine = 25.sp

    val btnSize = 16.sp
    val btnLine = 22.sp

    val regularSize = 14.sp
    val regularLine = 20.sp

    val chipSize = 13.sp
    val chipLine = 15.sp
}
```

- [ ] **Step 4: Создать `Border.kt`**

```kotlin
package com.resultv.android.theme

import androidx.compose.foundation.border
import androidx.compose.foundation.layout.padding
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.unit.dp

/**
 * Градиентная обводка интерактивных элементов, перенос `.rv-border` из
 * `ResultV-dev/frontend/src/design/borders.css`.
 *
 * Градиент идёт СТРОГО ПОПЕРЁК элемента слева направо: у левого края белый
 * 10 %, у правого 5 %. Горизонталь — не догадка: на ПК она снята с рендера
 * макета попиксельно (у сайдбара 74x740, тумблера 256x61 и карточки сервера
 * 778x118 верхняя и нижняя грани светятся одинаково, а левая и правая
 * отличаются вдвое). Вертикальной составляющей у градиента нет.
 *
 * Обводку получают элементы, с которыми можно взаимодействовать.
 */
private val BorderWidth = 1.dp

private val BaseBrush = Brush.horizontalGradient(
    listOf(RvColor.whiteA10, RvColor.whiteA05),
)

/**
 * В макете второй слой (20 % -> 15 %) включает НАВЕДЕНИЕ. На телефоне
 * наведения нет, поэтому он отдан нажатию: повод другой, рисунок тот же.
 * См. G-3 спеки.
 */
private val PressedBrush = Brush.horizontalGradient(
    listOf(RvColor.whiteA20, RvColor.whiteA15),
)

fun Modifier.rvBorder(shape: Shape, pressed: Boolean = false): Modifier =
    border(BorderWidth, if (pressed) PressedBrush else BaseBrush, shape)

/**
 * Нажатие: содержимое садится внутрь на 1 dp, габарит при этом не меняется —
 * соседи в ряду не разъезжаются и окно не дёргается по высоте. На ПК то же
 * самое делает `clip-path: inset(...)`, здесь — обрезка по форме.
 *
 * Padding идёт ПОСЛЕ clip: обрезается внешний контур, а содержимое отступает
 * внутрь от него.
 */
fun Modifier.rvPress(pressed: Boolean, shape: Shape): Modifier =
    clip(shape).then(if (pressed) Modifier.padding(BorderWidth) else Modifier)
```

- [ ] **Step 5: Положить шрифт Benzin**

```bash
mkdir -p /c/ResultV/android/app/src/main/res/font
cp /c/ResultV/ResultV-dev/frontend/src/assets/fonts/benzin-bold.ttf \
   /c/ResultV/android/app/src/main/res/font/benzin_bold.ttf
```

Имя файла обязано быть `benzin_bold.ttf`: Android допускает в именах ресурсов
только строчные буквы, цифры и подчёркивание — дефис из исходного имени сборку
уронит.

- [ ] **Step 6: Переписать `Theme.kt`**

```kotlin
package com.resultv.android.theme

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import com.resultv.android.R

/** Benzin Bold. Им набрано слово «ResultV» — и больше ничего. */
val BenzinBold = FontFamily(Font(R.font.benzin_bold, FontWeight.Bold))

/**
 * Все слоты выставлены явно: любой незаданный уезжает в фиолетовую тональную
 * палитру Material по умолчанию, и тогда Switch, SegmentedButton или индикатор
 * NavigationBar оказываются не нашего цвета.
 */
private val ResultVColors = darkColorScheme(
    primary = RvColor.Main,
    onPrimary = RvColor.White,
    primaryContainer = RvColor.mainA20,
    onPrimaryContainer = RvColor.Main,
    inversePrimary = RvColor.Second,

    secondary = RvColor.Second,
    onSecondary = RvColor.Black,
    secondaryContainer = RvColor.secondA10,
    onSecondaryContainer = RvColor.Second,

    tertiary = RvColor.Warning,
    onTertiary = RvColor.Black,
    tertiaryContainer = RvColor.warningA10,
    onTertiaryContainer = RvColor.Warning,

    error = RvColor.Errors,
    onError = RvColor.White,
    errorContainer = RvColor.errorsA10,
    onErrorContainer = RvColor.Errors,

    background = RvColor.Black,
    onBackground = RvColor.White,
    surface = RvColor.Grey,
    onSurface = RvColor.White,
    surfaceVariant = RvColor.LightGray,
    onSurfaceVariant = RvColor.whiteA50,
    surfaceTint = RvColor.Main,

    inverseSurface = RvColor.White,
    inverseOnSurface = RvColor.Black,

    outline = RvColor.whiteA10,
    outlineVariant = RvColor.whiteA05,

    scrim = RvColor.overlay,

    surfaceBright = RvColor.LightGray,
    surfaceDim = RvColor.Black,
    surfaceContainerLowest = RvColor.Black,
    surfaceContainerLow = RvColor.DarkGrey,
    surfaceContainer = RvColor.Grey,
    surfaceContainerHigh = RvColor.LightGray,
    surfaceContainerHighest = RvColor.LightGray,
)

/**
 * Пять стилей макета разложены по слотам Material, а не живут рядом с ними:
 * компоненты M3 читают MaterialTheme.typography сами. Слоты, которым в макете
 * ничего не соответствует, получают ближайший стиль — а не выдуманное значение.
 */
private fun style(size: androidx.compose.ui.unit.TextUnit, line: androidx.compose.ui.unit.TextUnit, weight: FontWeight) =
    TextStyle(fontSize = size, lineHeight = line, fontWeight = weight)

private val H1 = style(RvType.h1Size, RvType.h1Line, FontWeight.Bold)
private val Title = style(RvType.titleSize, RvType.titleLine, FontWeight.Bold)
private val Btn = style(RvType.btnSize, RvType.btnLine, FontWeight.Bold)
private val Regular = style(RvType.regularSize, RvType.regularLine, FontWeight.Medium)
private val Chip = style(RvType.chipSize, RvType.chipLine, FontWeight.Medium)

private val ResultVTypography = Typography(
    displayLarge = H1, displayMedium = H1, displaySmall = H1,
    headlineLarge = H1, headlineMedium = Title, headlineSmall = Title,
    titleLarge = Title, titleMedium = Btn, titleSmall = Btn,
    bodyLarge = Regular, bodyMedium = Chip, bodySmall = Chip,
    labelLarge = Btn, labelMedium = Chip, labelSmall = Chip,
)

private val ResultVShapes = Shapes(
    extraSmall = RoundedCornerShape(RvRadius.small),
    small = RoundedCornerShape(RvRadius.chip),
    medium = RoundedCornerShape(RvRadius.control),
    large = RoundedCornerShape(RvRadius.card),
    extraLarge = RoundedCornerShape(RvRadius.panel),
)

@Composable
fun ResultVTheme(content: @Composable () -> Unit) {
    MaterialTheme(
        colorScheme = ResultVColors,
        typography = ResultVTypography,
        shapes = ResultVShapes,
        content = content,
    )
}
```

- [ ] **Step 7: Превратить `Brand` в псевдонимы**

Заменить содержимое `android/app/src/main/java/com/resultv/android/theme/Color.kt`:

```kotlin
package com.resultv.android.theme

/**
 * Псевдонимы на новые токены — строительные леса на время перевода экранов.
 *
 * Существуют ровно для того, чтобы все 23 файла продолжали собираться, пока
 * экраны переводятся по одному, а не разом. Удаляются последней задачей плана,
 * и это удаление служит доказательством полноты перевода: если `Brand` уходит
 * и сборка проходит, значит на старые значения не осталось ни одной ссылки.
 *
 * НОВЫЙ КОД СЮДА НЕ ПИШЕТСЯ. Берите RvColor.
 */
@Deprecated("Строительные леса перевода на RvColor; удаляются в конце", ReplaceWith("RvColor"))
object Brand {
    val Green = RvColor.Main
    val GreenLight = RvColor.Second
    val GreenDark = RvColor.mainA50

    val Danger = RvColor.Errors
    val Warning = RvColor.Warning
    val Favorite = RvColor.Warning

    val Bg = RvColor.Black
    val Surface = RvColor.Grey
    val SurfaceHigh = RvColor.LightGray
    val SurfaceBorder = RvColor.whiteA10

    val MutedText = RvColor.whiteA50
    val SecondaryText = RvColor.whiteA50
}
```

`GreenDark` и `Favorite` в макете пары не имеют: первый был ховером, которого на
телефоне нет, второй — жёлтой звездой избранного, и это `Warning`. Оба уйдут
вместе с объектом.

- [ ] **Step 8: Запустить тесты и убедиться, что они проходят**

```bash
cd android && ./gradlew :app:testFullDebugUnitTest --tests '*TokensTest*'
```

Ожидается: BUILD SUCCESSFUL, 8 тестов PASSED.

- [ ] **Step 9: Проверить обе сборки**

```bash
cd android && ./gradlew :app:assembleFullDebug :app:assemblePlayDebug -Pdebug.abi=arm64-v8a
```

Ожидается: BUILD SUCCESSFUL. Предупреждения о `Brand` как deprecated — ожидаемы,
их 23 файла; они исчезнут по ходу задач 3–9.

- [ ] **Step 10: Коммит**

```bash
cd /c/ResultV && git add android/app/src/main/java/com/resultv/android/theme android/app/src/main/res/font android/app/src/test/java/com/resultv/android/theme && git commit -F - <<'EOF'
feat(android): слой токенов дизайн-системы ПК

Tokens.kt — зеркало tokens.css: цвета один в один, геометрия той же
шкалой ступенью ниже, пять текстовых стилей, движение 300 мс
FastOutSlowInEasing. Border.kt даёт градиентную обводку поперёк
элемента и посадку при нажатии.

Theme.kt кормит теми же значениями ColorScheme, Typography и Shapes,
чтобы компоненты Material красились сами.

Brand на время перевода становится псевдонимами: все экраны продолжают
собираться, пока переводятся по одному. Удаление Brand в конце плана
служит доказательством полноты.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
```

---

### Task 3: Общие компоненты

**Files:**
- Modify: `android/app/src/main/java/com/resultv/android/ui/components/SettingIcon.kt`
- Modify: `android/app/src/main/java/com/resultv/android/ui/components/ServerRow.kt`
- Modify: `android/app/src/main/java/com/resultv/android/ui/components/PowerButton.kt`
- Modify: `android/app/src/main/java/com/resultv/android/ui/components/TagField.kt`
- Modify: `android/app/src/main/java/com/resultv/android/ui/components/ProtocolFilter.kt`
- Modify: `android/app/src/main/java/com/resultv/android/ui/components/ProfileSort.kt`
- Modify: `android/app/src/main/java/com/resultv/android/ui/components/SubscriptionLogo.kt`
- Test: `android/app/src/test/java/com/resultv/android/ui/components/TagFieldTest.kt` (не меняется, должен продолжать проходить)

**Interfaces:**
- Consumes: `RvColor`, `RvCategory`, `RvSpace`, `RvRadius`, `RvIcon`, `Modifier.rvBorder` из задачи 2.
- Produces: `SettingIcon(icon: ImageVector, tint: CategoryTint)` — новая сигнатура, два параметра вместо трёх. Её зовут задачи 5, 7, 8, 9.

- [ ] **Step 1: Сменить сигнатуру `SettingIcon`**

Заменить тело `SettingIcon.kt`:

```kotlin
package com.resultv.android.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.vector.ImageVector
import com.resultv.android.theme.CategoryTint
import com.resultv.android.theme.RvIcon
import com.resultv.android.theme.RvRadius

/**
 * Цветная плитка со значком — ведущий элемент строк настроек и шапок шторок.
 * Единственный источник правды, чтобы настройки, правила и шторка правки
 * подписки рисовали один и тот же квадрат.
 *
 * Пара цветов приходит одним значением: раздельные bg и tint разъезжались —
 * подложка одной категории вставала под глиф другой.
 */
@Composable
fun SettingIcon(icon: ImageVector, tint: CategoryTint) {
    Box(
        modifier = Modifier
            .size(RvIcon.tile)
            .clip(RoundedCornerShape(RvRadius.chip))
            .background(tint.tile),
        contentAlignment = Alignment.Center,
    ) {
        Icon(
            imageVector = icon,
            contentDescription = null,
            tint = tint.glyph,
            modifier = Modifier.size(RvIcon.glyph),
        )
    }
}
```

- [ ] **Step 2: Перевести шесть остальных компонентов**

В каждом из `ServerRow.kt`, `PowerButton.kt`, `TagField.kt`, `ProtocolFilter.kt`,
`ProfileSort.kt`, `SubscriptionLogo.kt`:

1. Заменить импорт `com.resultv.android.theme.Brand` на `com.resultv.android.theme.RvColor` (и `RvSpace` / `RvRadius`, если понадобятся).
2. Заменить обращения по таблице: `Brand.Green` → `RvColor.Main`, `Brand.GreenLight` → `RvColor.Second`, `Brand.Danger` → `RvColor.Errors`, `Brand.Warning` и `Brand.Favorite` → `RvColor.Warning`, `Brand.Bg` → `RvColor.Black`, `Brand.Surface` → `RvColor.Grey`, `Brand.SurfaceHigh` → `RvColor.LightGray`, `Brand.SurfaceBorder` → `RvColor.whiteA10`, `Brand.MutedText` и `Brand.SecondaryText` → `RvColor.whiteA50`, `Brand.GreenDark` → `RvColor.mainA50`.
3. Заменить числовые `.dp` отступов на `RvSpace`: `24.dp` → `RvSpace.page`, `16.dp`/`18.dp` → `RvSpace.nest1`, `12.dp`/`14.dp` → `RvSpace.nest2`, `8.dp`/`10.dp` → `RvSpace.nest3`, `4.dp`/`6.dp` → `RvSpace.xs`.
4. Заменить `RoundedCornerShape(n.dp)` на шкалу: 20–24 → `RvRadius.card`, 16–18 → `RvRadius.control`, 10–14 → `RvRadius.chip`, 6–8 → `RvRadius.small`.
5. Карточке `ServerRow` добавить обводку: `Modifier.rvBorder(RoundedCornerShape(RvRadius.card))`.

Размеры, привязанные к смыслу, а не к шкале (высота спарклайна, размер флага,
диаметр кнопки питания), **не трогать** — они не отступы.

- [ ] **Step 3: Проверить, что старый тест компонента жив**

```bash
cd android && ./gradlew :app:testFullDebugUnitTest --tests '*TagFieldTest*'
```

Ожидается: BUILD SUCCESSFUL, 5 тестов PASSED. `TagFieldTest` проверяет логику
черновика, и перекраска её задеть не должна — если он упал, правка залезла не
туда.

- [ ] **Step 4: Проверить, что ссылок на `Brand` в этих файлах не осталось**

```bash
cd /c/ResultV && grep -rn "Brand\." --include=*.kt android/app/src/main/java/com/resultv/android/ui/components/
```

Ожидается: пусто.

- [ ] **Step 5: Собрать**

```bash
cd android && ./gradlew :app:assembleFullDebug :app:assemblePlayDebug -Pdebug.abi=arm64-v8a
```

Ожидается: BUILD SUCCESSFUL. `SettingIcon` сменил сигнатуру, и вызовы в
`SettingsScreen.kt`, `RulesScreen.kt`, `SubscriptionEditSheet.kt` теперь не
компилируются — **это ожидаемо**, и здесь их надо поправить механически:
`SettingIcon(icon, bg, tint)` → `SettingIcon(icon, RvCategory.<Цвет>)`, подбирая
имя по таблице цветов в `Tokens.kt`. Полная перекраска этих экранов — задачи 5,
7 и 8.

- [ ] **Step 6: Коммит**

```bash
cd /c/ResultV && git add android/app/src/main/java/com/resultv/android/ui && git commit -F - <<'EOF'
feat(android): перевести общие компоненты на токены

SettingIcon берёт пару цветов одним значением CategoryTint: раздельные
bg и tint разъезжались — подложка одной категории вставала под глиф
другой. Плитка выросла до 40 со скруглением 12, как в шкале.

Остальные шесть компонентов переведены на RvColor, RvSpace и RvRadius.
Карточка сервера получила градиентную обводку.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
```

---

### Task 4: Каркас приложения

**Files:**
- Modify: `android/app/src/main/java/com/resultv/android/MainActivity.kt`

**Interfaces:**
- Consumes: `RvColor`, `RvSpace`, `BenzinBold` из задачи 2.
- Produces: ничего нового.

- [ ] **Step 1: Перевести цвета каркаса**

В `MainActivity.kt` заменить шесть обращений к `Brand` по таблице из задачи 3.
`TopAppBar` и `CenterAlignedTopAppBar` берут `containerColor = RvColor.Black`,
`NavigationBar` — `containerColor = RvColor.Grey`.

Системные панели **не трогать**: `enableEdgeToEdge` уже ставит их прозрачными
(`MainActivity.kt:168-171`), и новый фон подхватится сам.

- [ ] **Step 2: Набрать «ResultV» гарнитурой Benzin**

В `HomeTopBar` (`MainActivity.kt:316-319`) заменить:

```kotlin
                Text(
                    text = stringResource(R.string.app_name),
                    fontFamily = com.resultv.android.theme.BenzinBold,
                    fontWeight = FontWeight.Bold,
                )
```

Это единственное место во всём модуле, где применяется Benzin.

- [ ] **Step 3: Собрать**

```bash
cd android && ./gradlew :app:assembleFullDebug -Pdebug.abi=arm64-v8a
```

Ожидается: BUILD SUCCESSFUL.

- [ ] **Step 4: Поставить на телефон и убедиться, что шрифт подхватился**

```bash
cd android && adb -s e3bacc6b install -r -d app/build/outputs/apk/full/debug/app-full-debug.apk
adb -s e3bacc6b shell am start -n com.resultv.android/.MainActivity
adb -s e3bacc6b exec-out screencap -p > /c/Users/andbe/AppData/Local/Temp/claude/C--ResultV/09575581-dcca-469c-9d34-60a6e4f39d58/scratchpad/task4-home.png
```

Открыть скриншот и убедиться: слово «ResultV» в шапке набрано Benzin (широкие
прямоугольные литеры), фон экрана стал `#141414` вместо почти чёрного, нижняя
панель — `#1A1A1A`. Если шрифт не подхватился, буквы останутся Roboto — проверьте
имя файла в `res/font`.

- [ ] **Step 5: Коммит**

```bash
cd /c/ResultV && git add android/app/src/main/java/com/resultv/android/MainActivity.kt && git commit -F - <<'EOF'
feat(android): каркас на токенах, слово ResultV гарнитурой Benzin

Верхняя панель и нижняя навигация берут цвета из RvColor. Системные
панели не трогаются: enableEdgeToEdge уже ставит их прозрачными, и
новый фон подхватывается сам.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
```

---

### Task 5: Экран настроек

Самая содержательная задача: здесь и перекраска, и структурные правки.

**Files:**
- Modify: `android/app/src/main/java/com/resultv/android/ui/screens/SettingsScreen.kt`
- Modify: `android/app/src/main/res/values/strings.xml`
- Modify: `android/app/src/main/res/values-ru/strings.xml`

**Interfaces:**
- Consumes: строки из задачи 1, токены из задачи 2, `SettingIcon(icon, tint)` из задачи 3.
- Produces: ничего нового.

- [ ] **Step 1: Добавить подпись в `SettingsSubcategory`**

Заменить перечисление (`SettingsScreen.kt:65-80`). Обратите внимание: `Advanced`
исчезает, а `iconBg`/`iconTint` схлопываются в `tint`:

```kotlin
private enum class SettingsSubcategory(
    val labelRes: Int,
    val descRes: Int,
    val itemsRes: Int,
    val icon: ImageVector,
    val tint: CategoryTint,
) {
    Network(
        R.string.settings_group_network, R.string.settings_group_network_desc,
        R.string.settings_group_network_items, Icons.Outlined.Public, RvCategory.Main,
    ),
    Routing(
        R.string.tab_rules, R.string.rules_section_smart_subtitle,
        R.string.settings_group_routing_items, Icons.Outlined.AltRoute, RvCategory.Blue,
    ),
    Security(
        R.string.settings_group_security, R.string.settings_group_security_desc,
        R.string.settings_group_security_items, Icons.Outlined.Security, RvCategory.Red,
    ),
    AdBlock(
        AdBlockGroupRes.label, AdBlockGroupRes.desc,
        AdBlockGroupRes.items, Icons.Outlined.Block, RvCategory.Red,
    ),
    Subscriptions(
        R.string.settings_group_subscriptions, R.string.settings_group_subscriptions_desc,
        R.string.settings_group_subscriptions_items, Icons.Outlined.RssFeed, RvCategory.Amber,
    ),
    Appearance(
        R.string.settings_group_appearance, R.string.settings_group_appearance_desc,
        R.string.settings_group_appearance_items, Icons.Outlined.Palette, RvCategory.Violet,
    ),
}
```

- [ ] **Step 2: Добавить `items` в `AdBlockGroupRes` обоих сорссетов**

В `android/app/src/full/java/com/resultv/android/ui/screens/AdBlockSettings.kt`
(строки 40-43):

```kotlin
internal object AdBlockGroupRes {
    val label = R.string.settings_group_adblock
    val desc = R.string.settings_group_adblock_desc
    val items = R.string.settings_group_adblock_items
}
```

В `android/app/src/play/java/com/resultv/android/ui/screens/AdBlockSettings.kt`
(строки 16-22) — другое значение, и это не небрежность: строки блокировки
рекламы лежат в `src/full/res` и в play-APK не попадают, поэтому заглушка
указывает на первую попавшуюся живую строку. Сама подкатегория в play не
рисуется — и ряд, и ветка листа стоят за `BuildConfig.DNS_ADBLOCK`, который
здесь `false`. Метки существуют лишь для того, чтобы общий `enum` скомпилировался:

```kotlin
internal object AdBlockGroupRes {
    val label = R.string.settings_group_security
    val desc = R.string.settings_group_security_desc
    val items = R.string.settings_group_security_items
}
```

- [ ] **Step 3: Перевести раздел «Дополнительные» — удалить, IPv6 в «Сеть»**

1. Удалить функцию `AdvancedGroup` целиком.
2. Удалить ветку `SettingsSubcategory.Advanced -> AdvancedGroup(settings)` из `when (activeSheet)`.
3. Удалить строку `SubcategoryRow(SettingsSubcategory.Advanced) { … }` и один соседний `HorizontalDivider` из карточки «Приложение».
4. В конец `NetworkGroup` дописать тумблер IPv6:

```kotlin
    HorizontalDivider(color = RvColor.whiteA10)
    ToggleRow(
        title = stringResource(R.string.settings_ipv6),
        subtitle = stringResource(R.string.settings_ipv6_subtitle),
        icon = Icons.Outlined.Language,
        tint = RvCategory.Blue,
        checked = settings.ipv6,
        onCheckedChange = { SettingsRepository.setIpv6(it) },
    )
```

- [ ] **Step 4: Перевести заголовки категорий на ресурсы**

Три вызова `CategoryHeader` сейчас принимают захардкоженные русские строки
(`SettingsScreen.kt:116`, `123`, `132`), поэтому в английской локали остаются
русскими. Заменить:

```kotlin
        CategoryHeader(stringResource(R.string.settings_cat_connection))
        …
        CategoryHeader(stringResource(R.string.settings_cat_security))
        …
        CategoryHeader(stringResource(R.string.settings_cat_app))
```

- [ ] **Step 5: Добавить подпись-список в строку категории**

Заменить `SubcategoryRow`:

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
        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(RvSpace.xs),
        ) {
            Text(
                stringResource(subcategory.labelRes),
                style = MaterialTheme.typography.titleMedium,
            )
            // Одна строка с обрезкой: состав раздела виден без захода внутрь,
            // а длинный список не разгоняет строку по высоте.
            Text(
                stringResource(subcategory.itemsRes),
                style = MaterialTheme.typography.bodyMedium,
                color = RvColor.whiteA50,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
        Icon(
            imageVector = Icons.AutoMirrored.Outlined.KeyboardArrowRight,
            contentDescription = null,
            tint = RvColor.iconDefault,
        )
    }
}
```

Дописать импорт `androidx.compose.ui.text.style.TextOverflow`.

`NavRow` (строка «Логи») оставить без подписи — это переход, а не раздел; в нём
заменить только `iconBg`/`iconTint` на `tint: CategoryTint` и цвета на токены.

- [ ] **Step 6: Развести DNS-подсказку на две строки**

В `NetworkGroup`, под полем ввода своих серверов, добавить предупреждение:

```kotlin
        Text(
            stringResource(R.string.settings_dns_private_warning),
            style = MaterialTheme.typography.bodyMedium,
            color = RvColor.whiteA50,
            modifier = Modifier.padding(start = 50.dp, top = RvSpace.nest3),
        )
```

- [ ] **Step 7: Убрать описание у строки языка**

В `AppearanceGroup` удалить второй `Text` с `settings_appearance_language_desc`:
название «Язык» и значение справа говорят всё сами.

- [ ] **Step 8: Перекрасить остальное на экране**

Заменить все оставшиеся `Brand.*` и литералы `Color(0x…)` по таблицам задач 2 и 3.
`SettingsCard` берёт `RoundedCornerShape(RvRadius.card)`, `containerColor = RvColor.Grey`
и получает обводку `Modifier.rvBorder(RoundedCornerShape(RvRadius.card))`. Внешний
`Column` экрана — `padding(horizontal = RvSpace.page, vertical = RvSpace.nest2)`,
зазор `Arrangement.spacedBy(RvSpace.nest1)`. Разделители — `RvColor.whiteA10`.
Функции `ToggleRow`, `IntervalRow`, `TextFieldRow` принимают `tint: CategoryTint`
вместо пары `iconBg`/`iconTint`.

- [ ] **Step 9: Удалить строки, осиротевшие этой задачей**

Из обеих локалей `main` удалить `settings_group_advanced`,
`settings_group_advanced_desc` и `settings_appearance_language_desc`.

- [ ] **Step 10: Запустить тесты ресурсов**

```bash
cd android && ./gradlew :app:testFullDebugUnitTest --tests '*SettingsStringsTest*'
```

Ожидается: BUILD SUCCESSFUL, оба теста PASSED. Если `bothLocalesKnowTheSameSettingsStrings`
упал — строку удалили из одной локали и забыли в другой.

- [ ] **Step 11: Собрать и посмотреть на телефоне**

```bash
cd android && ./gradlew :app:assembleFullDebug -Pdebug.abi=arm64-v8a
adb -s e3bacc6b install -r -d app/build/outputs/apk/full/debug/app-full-debug.apk
adb -s e3bacc6b shell am start -n com.resultv.android/.MainActivity
```

Открыть вкладку «Настройки», снять экран и каждую из шести шторок:

```bash
adb -s e3bacc6b exec-out screencap -p > .../task5-settings.png
```

Проверить глазами: заголовок «Настройки» ровно один; под каждой категорией —
односточный список; раздела «Дополнительные» нет, IPv6 стоит в «Сети»;
ни одна подпись не занимает больше двух строк.

- [ ] **Step 12: Проверить английскую локаль**

Переключить язык в разделе «Оформление» на English и снять экран настроек
повторно. Заголовки категорий обязаны стать английскими — до этой задачи они
оставались русскими при любой локали.

- [ ] **Step 13: Коммит**

```bash
cd /c/ResultV && git add android/app/src && git commit -F - <<'EOF'
feat(android): экран настроек по дизайн-системе ПК

Строка категории получила односточный список пунктов — состав виден без
захода внутрь. Заголовки категорий переехали в ресурсы: они были зашиты
русскими строками и в английской локали оставались русскими.

Раздел "Дополнительные настройки" удалён: внутри был только IPv6, а
описание обещало запуск, TUN и фильтрацию, которых на Android нет. IPv6
переехал в "Сеть".

Предупреждение про частный DNS отделено от описания поля — оно не
описание, а действие, и теперь стоит там, где по нему действуют.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
```

---

### Task 6: Главная и серверы

**Files:**
- Modify: `android/app/src/main/java/com/resultv/android/ui/screens/HomeScreen.kt`
- Modify: `android/app/src/main/java/com/resultv/android/ui/screens/ProxiesScreen.kt`

**Interfaces:**
- Consumes: токены задачи 2, компоненты задачи 3.
- Produces: ничего нового.

- [ ] **Step 1: Перевести оба экрана**

Применить таблицы замен из задачи 3 (цвета, отступы, скругления) к обоим файлам.
Карточки берут `RvRadius.card` и обводку `Modifier.rvBorder`. Панели во всю
ширину — `RvRadius.panel`.

- [ ] **Step 2: Убедиться, что `Brand` из этих файлов ушёл**

```bash
cd /c/ResultV && grep -n "Brand\." android/app/src/main/java/com/resultv/android/ui/screens/HomeScreen.kt android/app/src/main/java/com/resultv/android/ui/screens/ProxiesScreen.kt
```

Ожидается: пусто.

- [ ] **Step 3: Собрать и снять оба экрана**

```bash
cd android && ./gradlew :app:assembleFullDebug -Pdebug.abi=arm64-v8a
adb -s e3bacc6b install -r -d app/build/outputs/apk/full/debug/app-full-debug.apk
```

Снять «Главную» и «Серверы». На телефоне есть реальные профили — проверять на
них, не на пустом списке.

- [ ] **Step 4: Коммит**

```bash
cd /c/ResultV && git add android/app/src/main/java/com/resultv/android/ui/screens && git commit -F - <<'EOF'
feat(android): главная и список серверов на токенах

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
```

---

### Task 7: Добавление, правила, логи, ручной ввод

**Files:**
- Modify: `android/app/src/main/java/com/resultv/android/ui/screens/AddScreen.kt`
- Modify: `android/app/src/main/java/com/resultv/android/ui/screens/RulesScreen.kt`
- Modify: `android/app/src/main/java/com/resultv/android/ui/screens/LogsScreen.kt`
- Modify: `android/app/src/main/java/com/resultv/android/ui/screens/ManualPane.kt`

**Interfaces:**
- Consumes: токены задачи 2, `SettingIcon(icon, tint)` задачи 3.
- Produces: ничего нового.

- [ ] **Step 1: Перевести четыре экрана**

Те же таблицы замен. В `RulesScreen.kt` восемь литералов `Color(0x…)` — это
плитки значков; заменить на `RvCategory.*`, подбирая по оттенку: синий →
`RvCategory.Blue`, красный → `Red`, янтарный → `Amber`, фиолетовый → `Violet`.

- [ ] **Step 2: Проверить**

```bash
cd /c/ResultV && grep -n "Brand\.\|Color(0x" android/app/src/main/java/com/resultv/android/ui/screens/AddScreen.kt android/app/src/main/java/com/resultv/android/ui/screens/RulesScreen.kt android/app/src/main/java/com/resultv/android/ui/screens/LogsScreen.kt android/app/src/main/java/com/resultv/android/ui/screens/ManualPane.kt
```

Ожидается: пусто.

- [ ] **Step 3: Собрать и снять четыре экрана**

«Добавить», «Правила» (включая обе вкладки), «Логи» и ручной ввод конфигурации.

- [ ] **Step 4: Коммит**

```bash
cd /c/ResultV && git add android/app/src/main/java/com/resultv/android/ui/screens && git commit -F - <<'EOF'
feat(android): добавление, правила, логи и ручной ввод на токенах

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
```

---

### Task 8: Шторки

**Files:**
- Modify: `android/app/src/main/java/com/resultv/android/ui/screens/RoutingProfilesSheet.kt`
- Modify: `android/app/src/main/java/com/resultv/android/ui/screens/RoutingProfileEditor.kt`
- Modify: `android/app/src/main/java/com/resultv/android/ui/components/ProfileEditSheet.kt`
- Modify: `android/app/src/main/java/com/resultv/android/ui/components/SubscriptionEditSheet.kt`
- Modify: `android/app/src/main/java/com/resultv/android/ui/components/RoutingDeepLinkSheet.kt`

**Interfaces:**
- Consumes: токены задачи 2, `SettingIcon(icon, tint)` задачи 3.
- Produces: ничего нового.

- [ ] **Step 1: Перевести пять шторок**

Те же таблицы. Все вызовы `ModalBottomSheet` берут `containerColor = RvColor.Grey`.
`DarkSheetSystemBars()` остаётся первым вызовом в содержимом каждой шторки —
**не удалять**: без него в системной светлой теме под листом появляется белая
полоса навигации.

- [ ] **Step 2: Проверить**

```bash
cd /c/ResultV && grep -rn "Brand\.\|Color(0x" --include=*.kt android/app/src/main/java/com/resultv/android/ui/ | grep -v "theme/"
```

Ожидается: пусто.

- [ ] **Step 3: Собрать и снять пять шторок**

Профили маршрутизации, редактор профиля, правка профиля, правка подписки, приём
диплинка. Последняя открывается по ссылке `resultv://` — если воспроизвести
трудно, достаточно убедиться, что файл собрался и остальные четыре выглядят
верно.

Отдельно проверить, что у шторок снизу **нет белой полосы**: `DarkSheetSystemBars`
на месте.

- [ ] **Step 4: Коммит**

```bash
cd /c/ResultV && git add android/app/src/main/java/com/resultv/android/ui && git commit -F - <<'EOF'
feat(android): пять шторок на токенах

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
```

---

### Task 9: Сорссеты full и play

**Files:**
- Modify: `android/app/src/full/java/com/resultv/android/ui/screens/AdBlockSettings.kt`
- Modify: `android/app/src/play/java/com/resultv/android/ui/screens/AdBlockSettings.kt`
- Modify: `android/app/src/full/java/com/resultv/android/ui/screens/CertWizardScreen.kt`

**Interfaces:**
- Consumes: токены задачи 2, `SettingIcon(icon, tint)` задачи 3, `ToggleRow(tint = …)` задачи 5.
- Produces: ничего нового.

- [ ] **Step 1: Перевести три файла**

Те же таблицы. В `full/AdBlockSettings.kt` шесть литералов `Color(0x…)` — плитки
значков, заменить на `RvCategory.*`.

Файлы **не перемещать** между сорссетами: перенос `.kt` из `main` в `full`/`play`
ломает инкрементальную компиляцию так, что Kotlin сообщает `Unresolved reference`
на существующий файл, и лечится это только `--rerun-tasks`.

- [ ] **Step 2: Собрать обе сборки**

```bash
cd android && ./gradlew :app:assembleFullDebug :app:assemblePlayDebug -Pdebug.abi=arm64-v8a
```

Ожидается: BUILD SUCCESSFUL обеих. Сорссет `play` собирается отдельной задачей
именно потому, что его код компилятор при сборке `full` не видит вовсе — ошибку
там легко не заметить.

- [ ] **Step 3: Снять экраны блокировки рекламы и мастер сертификата**

Поставить `full`-сборку, открыть «Блокировка рекламы» и мастер установки
сертификата, снять оба.

- [ ] **Step 4: Коммит**

```bash
cd /c/ResultV && git add android/app/src/full android/app/src/play && git commit -F - <<'EOF'
feat(android): блокировка рекламы и мастер сертификата на токенах

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
```

---

### Task 10: Удалить леса и проверить целиком

Удаление `Brand` — не уборка, а доказательство: если объект уходит и сборка
проходит, значит на старые значения не осталось ни одной ссылки.

**Files:**
- Delete: `android/app/src/main/java/com/resultv/android/theme/Color.kt`

**Interfaces:**
- Consumes: всё предыдущее.
- Produces: ничего.

- [ ] **Step 1: Убедиться, что ссылок не осталось**

```bash
cd /c/ResultV && grep -rn "Brand\." --include=*.kt android/app/src
```

Ожидается: пусто. Если что-то нашлось — перевести на `RvColor` прямо здесь и
записать, какой файл был пропущен.

- [ ] **Step 2: Удалить файл**

```bash
rm /c/ResultV/android/app/src/main/java/com/resultv/android/theme/Color.kt
```

- [ ] **Step 3: Собрать обе сборки**

```bash
cd android && ./gradlew :app:assembleFullDebug :app:assemblePlayDebug -Pdebug.abi=arm64-v8a
```

Ожидается: BUILD SUCCESSFUL обеих. Провал означает пропущенную ссылку — вернитесь
к шагу 1.

- [ ] **Step 4: Прогнать все тесты**

```bash
cd android && ./gradlew :app:testFullDebugUnitTest :app:testPlayDebugUnitTest
```

Ожидается: 0 failures. Тестов в модуле 21 плюс два новых файла.

- [ ] **Step 5: Проверить, что литералы цвета остались в одном месте**

```bash
cd /c/ResultV && grep -rn "Color(0x" --include=*.kt android/app/src
```

Ожидается: совпадения только в `android/app/src/main/java/com/resultv/android/theme/Tokens.kt`.

- [ ] **Step 6: Полный обход на телефоне**

Поставить `full`-сборку и пройти все экраны подряд: главная, серверы, добавление,
правила, настройки со всеми шестью шторками, логи, профили маршрутизации,
редактор, правка подписки, блокировка рекламы, мастер сертификата. Снять каждый.

Проверить по списку:
- фон экрана `#141414`, карточки `#1A1A1A` — не прежний почти-чёрный;
- обводка у карточек видна и светлее слева, чем справа;
- ни одна подпись в настройках не длиннее двух строк;
- «ResultV» в шапке набрано Benzin;
- у шторок снизу нет белой полосы;
- подключение VPN работает — перекраска не должна была задеть логику, и это
  стоит подтвердить, а не предположить.

- [ ] **Step 7: Повторить обход в английской локали**

Переключить язык и пройти настройки ещё раз: русский текст длиннее английского,
поэтому если что-то не влезало — оно уже нашлось, а здесь проверяется обратное:
что английские строки не оказались обрезаны по-своему.

- [ ] **Step 8: Коммит**

```bash
cd /c/ResultV && git add -A android/app/src && git commit -F - <<'EOF'
refactor(android): удалить Brand — перевод на токены завершён

Объект был строительными лесами: он держал сборку, пока экраны
переводились по одному. Его удаление при проходящей сборке и есть
доказательство, что на старые значения не осталось ни одной ссылки.

Литералы Color(0x…) остались ровно в одном файле — Tokens.kt.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
```

---

## Таблица замен (справочник для задач 3–9)

Цвета:

| Было | Стало |
|---|---|
| `Brand.Green` | `RvColor.Main` |
| `Brand.GreenLight` | `RvColor.Second` |
| `Brand.GreenDark` | `RvColor.mainA50` |
| `Brand.Danger` | `RvColor.Errors` |
| `Brand.Warning`, `Brand.Favorite` | `RvColor.Warning` |
| `Brand.Bg` | `RvColor.Black` |
| `Brand.Surface` | `RvColor.Grey` |
| `Brand.SurfaceHigh` | `RvColor.LightGray` |
| `Brand.SurfaceBorder` | `RvColor.whiteA10` |
| `Brand.MutedText`, `Brand.SecondaryText` | `RvColor.whiteA50` |
| `Color(0xFF3b82f6)` + `Color(0xFF60a5fa)` | `RvCategory.Blue` |
| `Color(0xFFef4444)` + `Color(0xFFf87171)` | `RvCategory.Red` |
| `Color(0xFFf59e0b)` + `Color(0xFFfbbf24)` | `RvCategory.Amber` |
| `Color(0xFF8b5cf6)` + `Color(0xFFa78bfa)` | `RvCategory.Violet` |
| `Color(0xFF64748b)` + `Color(0xFF94a3b8)` | `RvCategory.Slate` |
| `Color(0xFF06b6d4)` + `Color(0xFF22d3ee)` | `RvCategory.Cyan` |
| `Color(0xFF10b981)` + `Color(0xFF34d399)` | `RvCategory.Emerald` |

Отступы и скругления:

| Было | Стало |
|---|---|
| `24.dp` отступ | `RvSpace.page` |
| `16.dp`, `18.dp` отступ | `RvSpace.nest1` |
| `12.dp`, `14.dp` отступ | `RvSpace.nest2` |
| `8.dp`, `10.dp` отступ | `RvSpace.nest3` |
| `4.dp`, `6.dp` отступ | `RvSpace.xs` |
| `RoundedCornerShape(20..24.dp)` | `RoundedCornerShape(RvRadius.card)` |
| `RoundedCornerShape(16..18.dp)` | `RoundedCornerShape(RvRadius.control)` |
| `RoundedCornerShape(10..14.dp)` | `RoundedCornerShape(RvRadius.chip)` |
| `RoundedCornerShape(6..8.dp)` | `RoundedCornerShape(RvRadius.small)` |

Размеры, привязанные к смыслу, а не к шкале — диаметр кнопки питания, высота
спарклайна, размер флага, ширина переключателя — **не трогать**.

Движение:

| Было | Стало |
|---|---|
| `tween(<любое число>)` | `tween(RvMotion.durationMillis, easing = RvMotion.easing)` |
| `animateColorAsState(...)`, `animateDpAsState(...)` и прочие `animate*AsState` без явного `animationSpec` | дописать `animationSpec = tween(RvMotion.durationMillis, easing = RvMotion.easing)` |

Правило одно на весь интерфейс: всё, что меняется **от состояния**, едет 300 мс
по одной кривой. Новых анимаций не добавлять — только привести к общей кривой
те, что уже есть.

Приводить (задача 3, `PowerButton.kt`) — три перехода цвета по состоянию
подключения:

```
PowerButton.kt:64   tween(350)  ->  tween(RvMotion.durationMillis, easing = RvMotion.easing)
PowerButton.kt:76   tween(350)  ->  то же
PowerButton.kt:87   tween(350)  ->  то же
```

**Не трогать** — это не переходы состояния, а самостоятельные движения, и общая
кривая им только помешает:

```
PowerButton.kt:104         tween(600)              — разрастание ореола
CertWizardScreen.kt:119    fadeIn(220)/fadeOut(160) — несимметричная смена шага
```

На ПК то же исключение сделано для `rv-spin` и `rv-blink`: у вращения и мигания
нет ни начала, ни конца, и общая кривая интерфейса им не нужна.
