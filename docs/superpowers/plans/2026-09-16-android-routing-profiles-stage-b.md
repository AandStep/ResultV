# Профили маршрутизации, этап B — Kotlin и диплинк сквозняком

**ИСПОЛНЕН 2026-09-16.** Все задачи закрыты, запись о результате — в спеке,
раздел 13. Два отступления от плана записаны там же.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** довести профиль маршрутизации до работающего сквозного пути — пользователь открывает ссылку от панели, видит, что в ней, соглашается, и трафик начинает ходить по её правилам.

**Architecture:** хранилище и `StateFlow` на Kotlin, как у всех соседних репозиториев; вся работа с правилами — в Go через биндинги этапа A. Редактора и маршрутизации из подписки здесь нет, это этап C.

**Tech Stack:** Kotlin, Jetpack Compose (Material 3), kotlinx.coroutines, org.json, gomobile AAR.

**Spec:** `docs/superpowers/specs/2026-09-16-android-routing-profiles-design.md`
**Предыдущий этап:** `docs/superpowers/plans/2026-09-16-android-routing-profiles-stage-a.md` (исполнен, запись в разделе 12 спеки)

## Global Constraints

- **Этап A даёт семь биндингов**, все через JSON-строки:
  `Mobile.isRoutingDeepLink(url): Boolean`,
  `Mobile.previewRoutingDeepLink(url): String`,
  `Mobile.mergeRoutingProfile(storedJson, incomingJson, makeActive): String`,
  `Mobile.compileRoutingProfile(profileJson, dataDir, refreshGeo): String`,
  `Mobile.routingProfileStatus(dataDir, profileId): String`,
  `Mobile.removeRoutingProfile(dataDir, profileId)`,
  `Mobile.extractSubscriptionRouting(header, body): String` (нужен только этапу C).
- **Имена полей JSON менять нельзя.** Они — единственное, что связывает Kotlin с
  `config.RoutingProfile`; переименуй любое, и оно молча потеряется на круге.
  Ключи: `id`, `name`, `directSites`, `directIp`, `proxySites`, `proxyIp`,
  `blockSites`, `blockIp`, `routeOrder`, `domainStrategy`, `geoipUrl`,
  `geositeUrl`, `listUrls`, `allowInsecure`, `source`, `subscriptionId`,
  `originName`, `updatedAt`, `lastError`.
- **Профиль действует только в Global.** Движок это уже обеспечивает
  (`applyRoutingProfile` игнорирует `routingProfileId` при `SmartMode`), UI лишь
  не показывает того, чего нет.
- **`Mobile.compileRoutingProfile` ходит в сеть.** Никогда с главного потока.
- **Обе сборки.** Задача закрыта, только когда зелены `testFullDebugUnitTest` и
  `testPlayDebugUnitTest`. Базовая линия перед этапом B: **full 84, play 82,
  0 падений** (проверено 2026-09-16).
- **Строки — в `src/main/res`**, обе сборки (спека, решение 2.2). Русский —
  `values-ru/strings.xml`, английский — `values/strings.xml`.
- **Тестируем только чистую логику.** В этом проекте JUnit-тесты не трогают
  `Context`: чистая часть выносится в отдельные функции (образцы —
  `AppRules.kt` против `AppRouting.kt`, `decodeRoutingMode` в `RoutingRules.kt`).
  `org.json` в юнит-тестах доступен (`testImplementation("org.json:json:20240303")`).
- **Язык коммитов — русский.**

**Команды:**

```bash
cd /c/ResultV/android
./gradlew :app:testFullDebugUnitTest :app:testPlayDebugUnitTest --console=plain

# счёт тестов после прогона
for f in full play; do
  find app/build/test-results/test${f^}DebugUnitTest -name '*.xml' \
    | xargs grep -ho 'tests="[0-9]*"\|failures="[0-9]*"'
done
```

---

## Карта файлов

| Файл | Что делает | Задача |
|---|---|---|
| `vpn/RoutingProfiles.kt` | модель, разбор/запись JSON, репозиторий со `StateFlow` | 1 |
| `vpn/RoutingProfileCompiler.kt` | обёртка над Go-сборкой: очередь, журнал, запись `lastError` | 2 |
| `vpn/BuildOptions.kt` | два новых поля в `optionsJson` | 3 |
| `vpn/ResultVpnService.kt` | ключ перезапуска | 3 |
| `ui/components/RoutingDeepLinkSheet.kt` | лист превью | 4 |
| `vpn/DeepLinkImporter.kt` | развилка routing / подписка | 4 |
| `ui/screens/RoutingProfilesScreen.kt` | полноэкранный список профилей | 5 |
| `ui/screens/RulesScreen.kt` | ряд «Профили маршрутизации», только в Global | 5 |
| `MainActivity.kt` | состояние маршрута, `BackHandler`, приём intent | 5 |
| `res/values/strings.xml`, `res/values-ru/strings.xml` | строки | 4, 5 |

---

### Task 0: Пересобрать play-AAR

`android/libs/libbox-play.aar` датирован 7 сентября — он старше всей работы
этапа A, и новых биндингов в нём нет. Play-флейвор на них не соберётся.

**Files:**
- Regenerate: `android/libs/libbox-play.aar`

- [x] **Step 1: Собрать**

```bash
cd /c/ResultV
set -a && source .env && set +a   # ключ подписок; скрипт .env сам не читает
DIST=play ./scripts/build-android-aar.sh
```

Ожидается: `✅ AAR built successfully`, БЕЗ строки
`⚠️ SUBSCRIPTION_ENCRYPT_KEY not set`.

- [x] **Step 2: Убедиться, что биндинги внутри**

gomobile молча пропускает функцию с неподдерживаемой сигнатурой — успешная
сборка сама по себе ничего не доказывает.

```bash
cd "$(mktemp -d)" && unzip -q /c/ResultV/android/libs/libbox-play.aar classes.jar \
  && unzip -qo classes.jar -d cls \
  && javap -classpath cls mobile.Mobile | grep -ciE "routingProfile|RoutingDeepLink|routingProfileStatus"
```

Ожидается: не меньше 6.

- [x] **Step 3: Коммита нет**

`android/libs` в `.gitignore` (строка 16) — артефакт сборки, в репозиторий не
попадает. Переходить к задаче 1.

---

### Task 1: Модель профиля и хранилище

**Files:**
- Create: `android/app/src/main/java/com/resultv/android/vpn/RoutingProfiles.kt`
- Test: `android/app/src/test/java/com/resultv/android/vpn/RoutingProfilesTest.kt`

**Interfaces:**
- Consumes: ничего из новых
- Produces: `data class RoutingProfile(...)`, `fun RoutingProfile.toJson(): JSONObject`, `fun routingProfileFromJson(o: JSONObject): RoutingProfile`, `fun decodeRoutingProfiles(json: String): RoutingProfilesState`, `fun encodeRoutingProfiles(s: RoutingProfilesState): String`, `data class RoutingProfilesState(val profiles: List<RoutingProfile>, val activeId: String)`, `object RoutingProfileRepository`

- [x] **Step 1: Написать падающий тест**

Создать `android/app/src/test/java/com/resultv/android/vpn/RoutingProfilesTest.kt`:

```kotlin
package com.resultv.android.vpn

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class RoutingProfilesTest {

    // Имена ключей — единственное, что связывает Kotlin с config.RoutingProfile
    // в Go. Переименуй любое, и поле молча потеряется на круге через хранилище.
    @Test fun jsonRoundTripKeepsEveryField() {
        val p = RoutingProfile(
            id = "abc123",
            name = "Моя маршрутизация",
            directSites = listOf("direct.example"),
            directIp = listOf("10.0.0.0/8"),
            proxySites = listOf("proxy.example"),
            proxyIp = listOf("1.2.3.0/24"),
            blockSites = listOf("block.example"),
            blockIp = listOf("5.6.7.0/24"),
            routeOrder = "block-proxy-direct",
            domainStrategy = "IPIfNonMatch",
            geoipUrl = "https://panel.example/geoip.dat",
            geositeUrl = "https://panel.example/geosite.dat",
            listUrls = mapOf("proxy" to listOf("https://panel.example/l.txt")),
            allowInsecure = true,
            source = "deeplink",
            subscriptionId = "sub1",
            originName = "Panel A",
            updatedAt = 1788322632L,
            lastError = "что-то пошло не так",
        )
        val back = routingProfileFromJson(JSONObject(p.toJson().toString()))
        assertEquals(p, back)
    }

    @Test fun jsonUsesTheGoFieldNames() {
        val p = RoutingProfile(id = "a", name = "N", proxySites = listOf("x.example"))
        val raw = p.toJson().toString()
        for (key in listOf("id", "name", "proxySites")) {
            assertTrue("нет ключа $key в $raw", raw.contains("\"$key\""))
        }
    }

    @Test fun missingFieldsDecodeToEmptyNotCrash() {
        val p = routingProfileFromJson(JSONObject("""{"id":"a","name":"N"}"""))
        assertEquals("a", p.id)
        assertTrue(p.directSites.isEmpty())
        assertTrue(p.listUrls.isEmpty())
        assertEquals("", p.routeOrder)
        assertEquals(0L, p.updatedAt)
    }

    // Счётчик для строки «• 12 direct • 3 block». Ссылка на список считается
    // за единицу: сколько правил за ней, неизвестно, пока её не скачали.
    @Test fun ruleCountCountsTokensAndLinks() {
        val p = RoutingProfile(
            id = "a", name = "N",
            directSites = listOf("a.example", "b.example"),
            directIp = listOf("10.0.0.0/8"),
            proxySites = listOf("c.example"),
            listUrls = mapOf("proxy" to listOf("https://panel.example/l.txt")),
        )
        assertEquals(3, p.ruleCount("direct"))
        assertEquals(2, p.ruleCount("proxy"))
        assertEquals(0, p.ruleCount("block"))
        assertEquals(0, p.ruleCount("чепуха"))
    }

    @Test fun storeRoundTripKeepsActiveId() {
        val s = RoutingProfilesState(
            profiles = listOf(
                RoutingProfile(id = "a", name = "A", proxySites = listOf("a.example")),
                RoutingProfile(id = "b", name = "B", proxySites = listOf("b.example")),
            ),
            activeId = "b",
        )
        assertEquals(s, decodeRoutingProfiles(encodeRoutingProfiles(s)))
    }

    // Форма хранилища обязана совпадать с тем, что ждёт Mobile.mergeRoutingProfile:
    // {"profiles":[...],"activeId":"..."}.
    @Test fun storeJsonHasTheShapeGoExpects() {
        val raw = encodeRoutingProfiles(
            RoutingProfilesState(listOf(RoutingProfile(id = "a", name = "A")), "a")
        )
        val o = JSONObject(raw)
        assertTrue(o.has("profiles"))
        assertTrue(o.has("activeId"))
        assertEquals(1, o.getJSONArray("profiles").length())
    }

    @Test fun brokenStoreDecodesToEmptyInsteadOfThrowing() {
        assertEquals(RoutingProfilesState(), decodeRoutingProfiles("не json"))
        assertEquals(RoutingProfilesState(), decodeRoutingProfiles(""))
    }

    // Активным не может остаться профиль, которого нет: строка списка была бы
    // без выделения, а движок получил бы id, по которому нет файлов.
    @Test fun activeIdIsDroppedWhenProfileIsGone() {
        val s = decodeRoutingProfiles(
            """{"profiles":[{"id":"a","name":"A"}],"activeId":"ghost"}"""
        )
        assertEquals("", s.activeId)
    }
}
```

- [x] **Step 2: Запустить — должен упасть**

```bash
cd /c/ResultV/android && ./gradlew :app:testFullDebugUnitTest --tests "*RoutingProfilesTest" --console=plain 2>&1 | tail -20
```

Ожидается: ошибка компиляции — `Unresolved reference: RoutingProfile`.

- [x] **Step 3: Написать модель и разбор**

Создать `android/app/src/main/java/com/resultv/android/vpn/RoutingProfiles.kt`:

```kotlin
package com.resultv.android.vpn

import android.content.Context
import android.util.Log
import com.resultv.android.R
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import org.json.JSONArray
import org.json.JSONObject
import java.io.File

private const val TAG = "ResultV/RoutingProfiles"
private const val FILE_NAME = "routing_profiles.json"

/** Действия профиля, в том же порядке, что proxy.RoutingActions в Go. */
val ROUTING_ACTIONS = listOf("direct", "proxy", "block")

/**
 * Один набор правил маршрутизации — зеркало `config.RoutingProfile` из Go.
 *
 * Имена полей в JSON совпадают с тамошними буква в букву и переименованию не
 * подлежат: это единственное, что связывает две стороны, и расхождение не
 * уронит ничего — поле просто исчезнет на круге через хранилище.
 *
 * Профиль действует только в Global (см. спеку, раздел 1).
 */
data class RoutingProfile(
    val id: String,
    val name: String,
    val directSites: List<String> = emptyList(),
    val directIp: List<String> = emptyList(),
    val proxySites: List<String> = emptyList(),
    val proxyIp: List<String> = emptyList(),
    val blockSites: List<String> = emptyList(),
    val blockIp: List<String> = emptyList(),
    val routeOrder: String = "",
    /**
     * Приезжает из payload панели и переживает round-trip, но никуда не
     * применяется — ни здесь, ни на ПК (там поле объявлено и не читается
     * никем, кроме парсера). В редактор не выносится, см. спеку 7.4.
     */
    val domainStrategy: String = "",
    val geoipUrl: String = "",
    val geositeUrl: String = "",
    /** Ссылки на списки правил, по действиям: "direct"/"proxy"/"block". */
    val listUrls: Map<String, List<String>> = emptyMap(),
    val allowInsecure: Boolean = false,
    /** "manual" | "deeplink" | "subscription". */
    val source: String = "",
    val subscriptionId: String = "",
    /**
     * Имя, под которым профиль опубликовал издатель. [name] пользователь может
     * менять, это — нет: только по нему повторно открытая ссылка узнаёт свой
     * профиль (в payload нет идентификатора).
     */
    val originName: String = "",
    val updatedAt: Long = 0L,
    /** Причина последней неудавшейся сборки, пустая строка — всё хорошо. */
    val lastError: String = "",
) {
    /**
     * Сколько правил у действия — для строки «• 12 direct • 3 block».
     * Ссылка на список считается за единицу: сколько за ней правил,
     * неизвестно, пока её не скачали.
     */
    fun ruleCount(action: String): Int {
        val links = listUrls[action]?.size ?: 0
        return when (action) {
            "direct" -> links + directSites.size + directIp.size
            "proxy" -> links + proxySites.size + proxyIp.size
            "block" -> links + blockSites.size + blockIp.size
            else -> 0
        }
    }

    /** Имя издателя, если оно отличается от того, что показываем. */
    val publisherName: String
        get() = if (originName.isNotBlank() && originName != name) originName else ""

    fun toJson(): JSONObject = JSONObject()
        .put("id", id)
        .put("name", name)
        .put("directSites", JSONArray(directSites))
        .put("directIp", JSONArray(directIp))
        .put("proxySites", JSONArray(proxySites))
        .put("proxyIp", JSONArray(proxyIp))
        .put("blockSites", JSONArray(blockSites))
        .put("blockIp", JSONArray(blockIp))
        .put("routeOrder", routeOrder)
        .put("domainStrategy", domainStrategy)
        .put("geoipUrl", geoipUrl)
        .put("geositeUrl", geositeUrl)
        .put("listUrls", JSONObject(listUrls.mapValues { JSONArray(it.value) }))
        .put("allowInsecure", allowInsecure)
        .put("source", source)
        .put("subscriptionId", subscriptionId)
        .put("originName", originName)
        .put("updatedAt", updatedAt)
        .put("lastError", lastError)
}

private fun JSONObject.stringList(key: String): List<String> {
    val arr = optJSONArray(key) ?: return emptyList()
    return (0 until arr.length()).mapNotNull { arr.optString(it).takeIf { s -> s.isNotBlank() } }
}

fun routingProfileFromJson(o: JSONObject): RoutingProfile {
    val links = mutableMapOf<String, List<String>>()
    o.optJSONObject("listUrls")?.let { obj ->
        for (action in ROUTING_ACTIONS) {
            val list = obj.stringList(action)
            if (list.isNotEmpty()) links[action] = list
        }
    }
    return RoutingProfile(
        id = o.optString("id"),
        name = o.optString("name"),
        directSites = o.stringList("directSites"),
        directIp = o.stringList("directIp"),
        proxySites = o.stringList("proxySites"),
        proxyIp = o.stringList("proxyIp"),
        blockSites = o.stringList("blockSites"),
        blockIp = o.stringList("blockIp"),
        routeOrder = o.optString("routeOrder"),
        domainStrategy = o.optString("domainStrategy"),
        geoipUrl = o.optString("geoipUrl"),
        geositeUrl = o.optString("geositeUrl"),
        listUrls = links,
        allowInsecure = o.optBoolean("allowInsecure", false),
        source = o.optString("source"),
        subscriptionId = o.optString("subscriptionId"),
        originName = o.optString("originName"),
        updatedAt = o.optLong("updatedAt", 0L),
        lastError = o.optString("lastError"),
    )
}

data class RoutingProfilesState(
    val profiles: List<RoutingProfile> = emptyList(),
    val activeId: String = "",
) {
    val active: RoutingProfile? get() = profiles.firstOrNull { it.id == activeId }
}

/**
 * Разбор хранилища. Форма — та же, что ждёт и отдаёт
 * `Mobile.mergeRoutingProfile`: `{"profiles":[…],"activeId":"…"}`.
 *
 * Испорченный файл читается как пустой, а не роняет запуск: тот же выбор, что
 * у соседних репозиториев.
 */
fun decodeRoutingProfiles(json: String): RoutingProfilesState {
    return try {
        val root = JSONObject(json)
        val arr = root.optJSONArray("profiles") ?: JSONArray()
        val list = (0 until arr.length()).map { routingProfileFromJson(arr.getJSONObject(it)) }
        val active = root.optString("activeId")
        RoutingProfilesState(
            profiles = list,
            // Активным не может остаться профиль, которого нет: движок получил
            // бы id, по которому на диске нет ни одного файла.
            activeId = if (list.any { it.id == active }) active else "",
        )
    } catch (t: Throwable) {
        RoutingProfilesState()
    }
}

fun encodeRoutingProfiles(s: RoutingProfilesState): String {
    val arr = JSONArray()
    s.profiles.forEach { arr.put(it.toJson()) }
    return JSONObject().put("profiles", arr).put("activeId", s.activeId).toString()
}
```

- [x] **Step 4: Запустить тесты — должны пройти**

```bash
cd /c/ResultV/android && ./gradlew :app:testFullDebugUnitTest --tests "*RoutingProfilesTest" --console=plain 2>&1 | tail -10
```

Ожидается: BUILD SUCCESSFUL, 8 тестов.

- [x] **Step 5: Дописать репозиторий**

В конец `RoutingProfiles.kt`:

```kotlin
/**
 * Хранилище профилей: файл `routing_profiles.json` и `StateFlow` над ним.
 *
 * ОТЛИЧИЕ ОТ СОСЕДЕЙ: запись идёт на IO, а не прямо в `mutate`. У соседних
 * репозиториев файл — единицы килобайт, у этого профиль подписки со встроенными
 * xray-правилами тянет на сотни, и синхронная запись оказалась бы на главном
 * потоке. Состояние обновляется сразу, запись выстраивается мьютексом.
 */
object RoutingProfileRepository {
    private val _state = MutableStateFlow(RoutingProfilesState())
    val state: StateFlow<RoutingProfilesState> = _state.asStateFlow()

    /**
     * Счётчик удачных сборок активного профиля. Ключ перезапуска движка
     * (см. ResultVpnService.startReloadWatcher) смотрит на него, а не на весь
     * стейт: правка неактивного профиля не должна рвать соединение.
     *
     * Живёт только в памяти, полем модели не становится: после перезапуска
     * процесса конфиг всё равно собирается заново на коннекте, и перезапускать
     * соединение из-за сборки прошлой жизни приложения — ровно то лишнее
     * действие, от которого этот ключ и защищает.
     */
    private val _compileGeneration = MutableStateFlow(0)
    val compileGeneration: StateFlow<Int> = _compileGeneration.asStateFlow()

    @Volatile private var file: File? = null
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val writeLock = Mutex()

    @Synchronized
    fun init(ctx: Context) {
        if (file != null) return
        val f = File(ctx.filesDir, FILE_NAME)
        file = f
        _state.value = if (f.exists()) {
            try {
                decodeRoutingProfiles(f.readText())
            } catch (t: Throwable) {
                Log.w(TAG, "failed to read $f, starting empty", t)
                AppLog.warning(R.string.log_read_failed, f.name,
                    source = AppLog.resolve(R.string.log_source_config))
                RoutingProfilesState()
            }
        } else {
            RoutingProfilesState()
        }
    }

    /** Текущее хранилище в том виде, в каком его ждёт Mobile.mergeRoutingProfile. */
    fun storeJson(): String = encodeRoutingProfiles(_state.value)

    /** Применить результат merge: весь новый список плюс активный. */
    @Synchronized
    fun replaceAll(profiles: List<RoutingProfile>, activeId: String) =
        mutate { RoutingProfilesState(profiles, activeId) }

    @Synchronized
    fun setActive(id: String) = mutate { s ->
        if (id.isNotEmpty() && s.profiles.none { it.id == id }) s else s.copy(activeId = id)
    }

    /**
     * Удалить профиль. Активный НЕ передаёт эстафету соседу, а оставляет
     * маршрутизацию без профиля: чьи правила в силе — не то, что решают за
     * пользователя. Кэш правил сносит вызывающая сторона (Go знает, где он).
     */
    @Synchronized
    fun delete(id: String) = mutate { s ->
        s.copy(
            profiles = s.profiles.filterNot { it.id == id },
            activeId = if (s.activeId == id) "" else s.activeId,
        )
    }

    @Synchronized
    fun setLastError(id: String, message: String) = mutate { s ->
        s.copy(profiles = s.profiles.map { if (it.id == id) it.copy(lastError = message) else it })
    }

    /** Отметить удачную сборку активного профиля — повод пересобрать конфиг. */
    fun noteCompiled(id: String) {
        if (id.isNotEmpty() && id == _state.value.activeId) {
            _compileGeneration.value = _compileGeneration.value + 1
        }
    }

    fun byId(id: String): RoutingProfile? = _state.value.profiles.firstOrNull { it.id == id }

    private fun mutate(block: (RoutingProfilesState) -> RoutingProfilesState) {
        val next = block(_state.value)
        if (next == _state.value) return
        _state.value = next
        val f = file ?: return
        val blob = encodeRoutingProfiles(next)
        scope.launch {
            writeLock.withLock {
                try {
                    f.writeText(blob)
                } catch (t: Throwable) {
                    Log.e(TAG, "failed to persist routing profiles", t)
                    AppLog.error(R.string.log_persist_failed, f.name,
                        source = AppLog.resolve(R.string.log_source_config))
                }
            }
        }
    }
}
```

- [x] **Step 6: Обе конфигурации**

```bash
cd /c/ResultV/android && ./gradlew :app:testFullDebugUnitTest :app:testPlayDebugUnitTest --console=plain 2>&1 | tail -6
```

Ожидается: BUILD SUCCESSFUL. Счёт: full 92, play 90 (было 84 / 82, добавилось 8).

- [x] **Step 7: Коммит**

```bash
cd /c/ResultV
git add android/app/src/main/java/com/resultv/android/vpn/RoutingProfiles.kt \
        android/app/src/test/java/com/resultv/android/vpn/RoutingProfilesTest.kt
git commit -m "feat(routing): модель профиля и хранилище на Kotlin

Имена полей JSON совпадают с config.RoutingProfile буква в букву — это
единственное, что связывает две стороны, и расхождение ничего не уронит:
поле просто исчезнет на круге. На это есть тест.

Запись на IO, в отличие от соседних репозиториев: у них файл в единицы
килобайт, здесь профиль подписки тянет на сотни, и синхронная запись
оказалась бы на главном потоке.

Активным не остаётся профиль, которого нет в списке: движок получил бы id,
по которому на диске нет ни одного файла.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 2: Сборка правил

**Files:**
- Create: `android/app/src/main/java/com/resultv/android/vpn/RoutingProfileCompiler.kt`
- Test: `android/app/src/test/java/com/resultv/android/vpn/RoutingCompileReportTest.kt`

**Interfaces:**
- Consumes: `RoutingProfile`, `RoutingProfileRepository` (задача 1), `Mobile.compileRoutingProfile`, `Mobile.routingProfileStatus`, `Mobile.removeRoutingProfile`
- Produces: `data class RoutingCompileOutcome(val counts: Map<String, Int>, val unresolved: Map<String, String>, val error: String)`, `fun parseRoutingCompileReport(json: String): RoutingCompileOutcome`, `object RoutingProfileCompiler` с `suspend fun compile(profile: RoutingProfile, dataDir: String, refreshGeo: Boolean = false): RoutingCompileOutcome`, `fun statusOf(dataDir: String, id: String): Map<String, Boolean>`, `fun forget(dataDir: String, id: String)`

- [x] **Step 1: Написать падающий тест**

Создать `android/app/src/test/java/com/resultv/android/vpn/RoutingCompileReportTest.kt`:

```kotlin
package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class RoutingCompileReportTest {

    @Test fun parsesCountsAndUnresolved() {
        val r = parseRoutingCompileReport(
            """{"counts":{"direct":12,"proxy":3,"block":0},
                "unresolved":{"regexp:.*":"regular expressions are not supported"}}"""
        )
        assertEquals(12, r.counts["direct"])
        assertEquals(3, r.counts["proxy"])
        assertEquals(0, r.counts["block"])
        assertEquals(1, r.unresolved.size)
        assertTrue(r.error.isEmpty())
    }

    @Test fun emptyReportIsNotAnError() {
        val r = parseRoutingCompileReport("""{"counts":{},"unresolved":{}}""")
        assertTrue(r.counts.isEmpty())
        assertTrue(r.unresolved.isEmpty())
        assertTrue(r.error.isEmpty())
    }

    // Go отдаёт ошибку исключением, а не полем отчёта; но если отчёт придёт
    // нечитаемым, это тоже отказ, а не «ноль правил».
    @Test fun brokenReportBecomesAnError() {
        val r = parseRoutingCompileReport("не json")
        assertTrue(r.error.isNotEmpty())
        assertTrue(r.counts.isEmpty())
    }

    @Test fun totalCountsEveryAction() {
        val r = parseRoutingCompileReport("""{"counts":{"direct":2,"proxy":3,"block":5}}""")
        assertEquals(10, r.total)
    }
}
```

- [x] **Step 2: Запустить — должен упасть**

```bash
cd /c/ResultV/android && ./gradlew :app:testFullDebugUnitTest --tests "*RoutingCompileReportTest" --console=plain 2>&1 | tail -10
```

Ожидается: `Unresolved reference: parseRoutingCompileReport`.

- [x] **Step 3: Написать реализацию**

Создать `android/app/src/main/java/com/resultv/android/vpn/RoutingProfileCompiler.kt`:

```kotlin
package com.resultv.android.vpn

import android.util.Log
import com.resultv.android.R
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import mobile.Mobile
import org.json.JSONObject

private const val TAG = "ResultV/RoutingCompile"

/**
 * Что сказала сборка: сколько правил получилось у каждого действия и что она
 * не смогла выразить.
 *
 * [unresolved] — карта, а не число: профиль, у которого приняли половину
 * правил, обязан уметь сказать КАКУЮ половину и почему, иначе трафик пойдёт не
 * туда без единого объяснения.
 */
data class RoutingCompileOutcome(
    val counts: Map<String, Int> = emptyMap(),
    val unresolved: Map<String, String> = emptyMap(),
    val error: String = "",
) {
    val ok: Boolean get() = error.isEmpty()
    val total: Int get() = counts.values.sum()
}

fun parseRoutingCompileReport(json: String): RoutingCompileOutcome {
    return try {
        val o = JSONObject(json)
        val counts = mutableMapOf<String, Int>()
        o.optJSONObject("counts")?.let { c ->
            for (action in ROUTING_ACTIONS) {
                if (c.has(action)) counts[action] = c.optInt(action, 0)
            }
        }
        val unresolved = mutableMapOf<String, String>()
        o.optJSONObject("unresolved")?.let { u ->
            val keys = u.keys()
            while (keys.hasNext()) {
                val k = keys.next()
                unresolved[k] = u.optString(k)
            }
        }
        RoutingCompileOutcome(counts, unresolved)
    } catch (t: Throwable) {
        RoutingCompileOutcome(error = t.message ?: t.javaClass.simpleName)
    }
}

/**
 * Сборка правил профиля в кэш, который читает движок.
 *
 * Всё тяжёлое — в Go: скачать geo-базы, развернуть `geosite:`-токены, дописать
 * связанные списки, скомпилировать три rule-set'а. Здесь остаются очередь,
 * журнал и запись причины отказа в профиль.
 *
 * Сборки выстроены мьютексом: две одновременные полезли бы в один кэш geo-баз,
 * а сборка и без того идёт в сеть — параллелить тут нечего.
 */
object RoutingProfileCompiler {
    private val lock = Mutex()

    /**
     * Собрать профиль. НИКОГДА с главного потока: ходит в сеть.
     *
     * Отказ не откатывает ничего — профиль остаётся сохранённым, причина
     * ложится в его `lastError`, и собрать можно повторно, не открывая ссылку
     * заново. Это поведение ПК (app_routingprofile.go:92-97).
     */
    suspend fun compile(
        profile: RoutingProfile,
        dataDir: String,
        refreshGeo: Boolean = false,
    ): RoutingCompileOutcome = lock.withLock {
        val raw = try {
            withContext(Dispatchers.IO) {
                Mobile.compileRoutingProfile(profile.toJson().toString(), dataDir, refreshGeo)
            }
        } catch (t: Throwable) {
            val message = t.message ?: t.javaClass.simpleName
            Log.w(TAG, "compile(${profile.id}) failed", t)
            RoutingProfileRepository.setLastError(profile.id, message)
            AppLog.error(R.string.log_routing_compile_failed, profile.name, message,
                source = AppLog.resolve(R.string.log_source_config))
            return@withLock RoutingCompileOutcome(error = message)
        }
        val outcome = parseRoutingCompileReport(raw)
        if (!outcome.ok) {
            RoutingProfileRepository.setLastError(profile.id, outcome.error)
            return@withLock outcome
        }
        RoutingProfileRepository.setLastError(profile.id, "")
        RoutingProfileRepository.noteCompiled(profile.id)
        AppLog.info(
            R.string.log_routing_compiled,
            profile.name,
            outcome.counts["direct"] ?: 0,
            outcome.counts["proxy"] ?: 0,
            outcome.counts["block"] ?: 0,
            source = AppLog.resolve(R.string.log_source_config),
        )
        if (outcome.unresolved.isNotEmpty()) {
            AppLog.warning(R.string.log_routing_unresolved, profile.name, outcome.unresolved.size,
                source = AppLog.resolve(R.string.log_source_config))
        }
        outcome
    }

    /** Какие действия профиля уже собраны. Дёшево: только stat по трём файлам. */
    fun statusOf(dataDir: String, id: String): Map<String, Boolean> = try {
        val o = JSONObject(Mobile.routingProfileStatus(dataDir, id))
        ROUTING_ACTIONS.associateWith { o.optBoolean(it, false) }
    } catch (t: Throwable) {
        Log.w(TAG, "routingProfileStatus($id) failed", t)
        ROUTING_ACTIONS.associateWith { false }
    }

    /** Снести кэш правил удалённого профиля — иначе он продолжит маршрутизировать. */
    fun forget(dataDir: String, id: String) {
        try {
            Mobile.removeRoutingProfile(dataDir, id)
        } catch (t: Throwable) {
            Log.w(TAG, "removeRoutingProfile($id) failed", t)
        }
    }
}
```

- [x] **Step 4: Добавить строки журнала**

В `android/app/src/main/res/values/strings.xml` (английский, дефолтный):

```xml
<string name="log_routing_compiled">Routing profile \"%1$s\" built: %2$d direct, %3$d proxy, %4$d block</string>
<string name="log_routing_unresolved">Routing profile \"%1$s\": %2$d rules could not be applied</string>
<string name="log_routing_compile_failed">Routing profile \"%1$s\" was saved, but its rules were not built: %2$s</string>
```

В `android/app/src/main/res/values-ru/strings.xml`:

```xml
<string name="log_routing_compiled">Профиль маршрутизации «%1$s» собран: %2$d direct, %3$d proxy, %4$d block</string>
<string name="log_routing_unresolved">Профиль маршрутизации «%1$s»: не принято правил — %2$d</string>
<string name="log_routing_compile_failed">Профиль «%1$s» сохранён, но правила не собраны: %2$s</string>
```

- [x] **Step 5: Запустить тесты — должны пройти**

```bash
cd /c/ResultV/android && ./gradlew :app:testFullDebugUnitTest :app:testPlayDebugUnitTest --console=plain 2>&1 | tail -6
```

Ожидается: BUILD SUCCESSFUL. Счёт: full 96, play 94.

- [x] **Step 6: Коммит**

```bash
cd /c/ResultV
git add android/app/src/main/java/com/resultv/android/vpn/RoutingProfileCompiler.kt \
        android/app/src/test/java/com/resultv/android/vpn/RoutingCompileReportTest.kt \
        android/app/src/main/res/values/strings.xml \
        android/app/src/main/res/values-ru/strings.xml
git commit -m "feat(routing): сборка правил профиля — очередь, журнал, причина отказа

Тяжёлое целиком в Go; здесь очередь под мьютексом (две сборки полезли бы в
один кэш geo-баз), журнал и запись причины в lastError профиля.

Отказ ничего не откатывает: профиль остаётся сохранённым и его можно собрать
повторно, не открывая ссылку заново. Поведение ПК.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 3: Передача профиля в движок

**Files:**
- Modify: `android/app/src/main/java/com/resultv/android/vpn/BuildOptions.kt`
- Modify: `android/app/src/main/java/com/resultv/android/vpn/ResultVpnService.kt:530-551`
- Test: `android/app/src/test/java/com/resultv/android/vpn/RoutingReloadKeyTest.kt`

**Interfaces:**
- Consumes: `RoutingProfileRepository`, `RoutingMode`
- Produces: `fun routingReloadKey(mode: RoutingMode, activeId: String, compileGeneration: Int): String`

- [x] **Step 1: Написать падающий тест**

Создать `android/app/src/test/java/com/resultv/android/vpn/RoutingReloadKeyTest.kt`:

```kotlin
package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Test

class RoutingReloadKeyTest {

    @Test fun switchingActiveProfileChangesTheKey() {
        assertNotEquals(
            routingReloadKey(RoutingMode.Global, "a", 1),
            routingReloadKey(RoutingMode.Global, "b", 1),
        )
    }

    @Test fun rebuildingTheActiveProfileChangesTheKey() {
        assertNotEquals(
            routingReloadKey(RoutingMode.Global, "a", 1),
            routingReloadKey(RoutingMode.Global, "a", 2),
        )
    }

    // Главное, ради чего этот ключ существует: правка НЕактивного профиля не
    // должна рвать живое соединение. Она не двигает ни activeId, ни счётчик
    // сборок активного, значит и ключ не двигает.
    @Test fun keyIsStableWhenNothingRelevantMoved() {
        assertEquals(
            routingReloadKey(RoutingMode.Global, "a", 1),
            routingReloadKey(RoutingMode.Global, "a", 1),
        )
    }

    // В Smart профиль не действует, поэтому его смена не повод перезапускаться.
    @Test fun profileIsInvisibleInSmart() {
        assertEquals(
            routingReloadKey(RoutingMode.Smart, "a", 1),
            routingReloadKey(RoutingMode.Smart, "b", 7),
        )
    }

    // Но смена режима — повод: в Global правила появляются, в Smart исчезают.
    @Test fun changingModeChangesTheKey() {
        assertNotEquals(
            routingReloadKey(RoutingMode.Global, "a", 1),
            routingReloadKey(RoutingMode.Smart, "a", 1),
        )
    }
}
```

- [x] **Step 2: Запустить — должен упасть**

```bash
cd /c/ResultV/android && ./gradlew :app:testFullDebugUnitTest --tests "*RoutingReloadKeyTest" --console=plain 2>&1 | tail -10
```

Ожидается: `Unresolved reference: routingReloadKey`.

- [x] **Step 3: Написать функцию ключа**

В конец `android/app/src/main/java/com/resultv/android/vpn/RoutingProfiles.kt`:

```kotlin
/**
 * Ключ, по которому сервис решает, пересобирать ли конфиг из-за профиля.
 *
 * Не весь стейт репозитория: правка НЕактивного профиля не должна рвать живое
 * соединение. Двигают ключ только три вещи — режим (в Smart профиля нет
 * вовсе), выбор активного и удачная пересборка именно его правил.
 */
fun routingReloadKey(mode: RoutingMode, activeId: String, compileGeneration: Int): String =
    if (mode == RoutingMode.Smart) "smart" else "global:$activeId:$compileGeneration"
```

- [x] **Step 4: Передать профиль в движок**

В `android/app/src/main/java/com/resultv/android/vpn/BuildOptions.kt`, в
`currentOptionsJson`, после `.put("smartMode", smartMode)`:

```kotlin
            // Профиль маршрутизации: через JNI едет только его id и порядок
            // действий. Правила движок находит на диске сам — профиль на
            // 20 000 токенов иначе означал бы мегабайты на каждый коннект,
            // ровно ту ошибку, которую Smart-список уже однажды исправлял.
            //
            // В Smart не передаётся вовсе. Движок это тоже проверяет
            // (applyRoutingProfile), здесь — чтобы в конфиге не было мусорного
            // поля, а в журнале — повода думать, что профиль работает.
            .put("routingProfileId", if (smartMode) "" else routingProfiles.activeId)
            .put("routingOrder", if (smartMode) "" else (routingProfiles.active?.routeOrder ?: ""))
```

И в начале той же функции, рядом с `val rules = RoutingRulesRepository.state.value`:

```kotlin
        val routingProfiles = RoutingProfileRepository.state.value
```

- [x] **Step 5: Добавить ключ в сторож перезапуска**

В `android/app/src/main/java/com/resultv/android/vpn/ResultVpnService.kt`,
в `startReloadWatcher` (строки 530-551), заменить четырёхместный `combine` на
шестиместный. Имена типов проверены в дереве: `RoutingRulesState`
(`RoutingRules.kt:46`), `AppRulesState` (`AppRules.kt:25`), `ProfilesState`
(`Profile.kt:144`), `SettingsState` (`SettingsRepository.kt:31`),
`RoutingProfilesState` (задача 1).

`combine` на пять и более потоков отдаёт `Array<*>` без типов, поэтому
приведения обязательны:

```kotlin
            combine(
                RoutingRulesRepository.state,
                AppRoutingRepository.state,
                ProfileRepository.state,
                SettingsRepository.state,
                RoutingProfileRepository.state,
                RoutingProfileRepository.compileGeneration,
            ) { values ->
                val rules = values[0] as RoutingRulesState
                val app = values[1] as AppRulesState
                val profiles = values[2] as ProfilesState
                val settings = values[3] as SettingsState
                val routing = values[4] as RoutingProfilesState
                val generation = values[5] as Int
                // Key on the active profile + everything that changes routing.
                // From settings we only watch ad-block (it rebuilds the route
                // rules); other settings keep applying on reconnect.
                //
                // The routing profile contributes a KEY, not its whole state:
                // editing an INACTIVE profile must not tear down a live
                // connection. See routingReloadKey.
                listOf(
                    rules, app, profiles.activeId, settings.adblock,
                    routingReloadKey(rules.mode, routing.activeId, generation),
                )
            }
```

Импорт `RoutingProfilesState`, `RoutingProfileRepository` и `routingReloadKey`
не нужен — тот же пакет `com.resultv.android.vpn`.

- [x] **Step 6: Инициализировать репозиторий**

`RoutingProfileRepository.init(ctx)` добавить всюду, где инициализируются
соседи. Найти места:

```bash
cd /c/ResultV && grep -rn "RoutingRulesRepository.init" --include=*.kt android/app/src/
```

Добавить рядом с каждым вызовом.

- [x] **Step 7: Прогнать обе конфигурации**

```bash
cd /c/ResultV/android && ./gradlew :app:testFullDebugUnitTest :app:testPlayDebugUnitTest --console=plain 2>&1 | tail -6
```

Ожидается: BUILD SUCCESSFUL. Счёт: full 101, play 99.

- [x] **Step 9: Коммит**

```bash
cd /c/ResultV
git add android/app/src/main/java/com/resultv/android/vpn/RoutingProfiles.kt \
        android/app/src/main/java/com/resultv/android/vpn/BuildOptions.kt \
        android/app/src/main/java/com/resultv/android/vpn/ResultVpnService.kt \
        android/app/src/main/java/com/resultv/android/MainActivity.kt \
        android/app/src/test/java/com/resultv/android/vpn/RoutingReloadKeyTest.kt
git commit -m "feat(routing): передать активный профиль движку и применять сразу

Через JNI едет id и порядок действий, не правила: профиль на 20 000 токенов
означал бы мегабайты на каждый коннект.

Ключ перезапуска смотрит на режим, активный id и счётчик удачных сборок
именно активного профиля — правка неактивного не рвёт живое соединение.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 4: Лист превью и развилка диплинка

**Files:**
- Create: `android/app/src/main/java/com/resultv/android/ui/components/RoutingDeepLinkSheet.kt`
- Modify: `android/app/src/main/java/com/resultv/android/vpn/DeepLinkImporter.kt`
- Modify: `android/app/src/main/res/values/strings.xml`, `values-ru/strings.xml`
- Test: `android/app/src/test/java/com/resultv/android/vpn/RoutingPreviewTest.kt`

**Interfaces:**
- Consumes: `RoutingProfile`, `routingProfileFromJson`, `RoutingProfileRepository`, `RoutingProfileCompiler`, `Mobile.isRoutingDeepLink`, `Mobile.previewRoutingDeepLink`, `Mobile.mergeRoutingProfile`
- Produces: `fun parseRoutingMergeResult(json: String): RoutingProfilesState?`, `object PendingRoutingImport` со `StateFlow<RoutingProfile?>`, `@Composable fun RoutingDeepLinkSheet(...)`

- [x] **Step 1: Написать падающий тест**

Создать `android/app/src/test/java/com/resultv/android/vpn/RoutingPreviewTest.kt`:

```kotlin
package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class RoutingPreviewTest {

    // Что отдаёт Mobile.mergeRoutingProfile: весь новый список, активный и
    // сохранённый профиль отдельно.
    @Test fun parsesMergeResult() {
        val s = parseRoutingMergeResult(
            """{"profiles":[{"id":"a","name":"A"},{"id":"b","name":"B"}],
                "activeId":"b",
                "saved":{"id":"b","name":"B"}}"""
        )!!
        assertEquals(2, s.profiles.size)
        assertEquals("b", s.activeId)
    }

    @Test fun brokenMergeResultIsNullNotEmpty() {
        // Пустое хранилище вместо сломанного разбора выбросило бы все профили.
        assertNull(parseRoutingMergeResult("не json"))
        assertNull(parseRoutingMergeResult(""))
    }

    @Test fun mergeResultWithoutProfilesIsRejected() {
        assertNull(parseRoutingMergeResult("""{"activeId":"a"}"""))
    }
}
```

- [x] **Step 2: Запустить — должен упасть**

```bash
cd /c/ResultV/android && ./gradlew :app:testFullDebugUnitTest --tests "*RoutingPreviewTest" --console=plain 2>&1 | tail -10
```

Ожидается: `Unresolved reference: parseRoutingMergeResult`.

- [x] **Step 3: Написать разбор результата merge**

В конец `RoutingProfiles.kt`:

```kotlin
/**
 * Разбор ответа `Mobile.mergeRoutingProfile`.
 *
 * `null` при любой беде, и это важно: подставить сюда пустое хранилище значило
 * бы выбросить все профили пользователя из-за одной нечитаемой строки.
 */
fun parseRoutingMergeResult(json: String): RoutingProfilesState? {
    return try {
        val root = JSONObject(json)
        if (!root.has("profiles")) return null
        val arr = root.getJSONArray("profiles")
        val list = (0 until arr.length()).map { routingProfileFromJson(arr.getJSONObject(it)) }
        val active = root.optString("activeId")
        RoutingProfilesState(list, if (list.any { it.id == active }) active else "")
    } catch (t: Throwable) {
        null
    }
}
```

- [x] **Step 4: Добавить строки**

`values/strings.xml`:

```xml
<string name="routing_sheet_title">Routing profile</string>
<string name="routing_sheet_publisher">Published by %1$s</string>
<string name="routing_sheet_counts">%1$d direct · %2$d proxy · %3$d block</string>
<string name="routing_sheet_geo">Rule databases will be downloaded after you accept</string>
<string name="routing_sheet_accept">Add and enable</string>
<string name="routing_sheet_decline">Cancel</string>
<string name="routing_import_failed">Could not read the routing link: %1$s</string>
<string name="routing_import_done">Routing profile \"%1$s\" added</string>
<string name="routing_import_built_partly">Profile added, but %1$d rules could not be applied</string>
```

`values-ru/strings.xml`:

```xml
<string name="routing_sheet_title">Профиль маршрутизации</string>
<string name="routing_sheet_publisher">Издатель: %1$s</string>
<string name="routing_sheet_counts">%1$d direct · %2$d proxy · %3$d block</string>
<string name="routing_sheet_geo">Базы правил скачаются после подтверждения</string>
<string name="routing_sheet_accept">Добавить и включить</string>
<string name="routing_sheet_decline">Отмена</string>
<string name="routing_import_failed">Не удалось прочитать ссылку маршрутизации: %1$s</string>
<string name="routing_import_done">Профиль маршрутизации «%1$s» добавлен</string>
<string name="routing_import_built_partly">Профиль добавлен, но не принято правил: %1$d</string>
```

- [x] **Step 5: Написать лист превью**

Создать `android/app/src/main/java/com/resultv/android/ui/components/RoutingDeepLinkSheet.kt`:

```kotlin
package com.resultv.android.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.BottomSheetDefaults
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.resultv.android.R
import com.resultv.android.theme.Brand
import com.resultv.android.vpn.RoutingProfile

/**
 * Что принесла ссылка — до того, как что-нибудь применится.
 *
 * Сеть здесь не трогается: `Mobile.previewRoutingDeepLink` только разбирает
 * payload. Правила скачиваются после согласия, и про это сказано прямым
 * текстом — иначе список geo-баз выглядел бы как уже загруженный.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun RoutingDeepLinkSheet(
    profile: RoutingProfile,
    busy: Boolean,
    onAccept: () -> Unit,
    onDismiss: () -> Unit,
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        containerColor = Brand.Surface,
        dragHandle = { BottomSheetDefaults.DragHandle() },
    ) {
        DarkSheetSystemBars()
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp, vertical = 8.dp)
                .padding(bottom = 24.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Text(
                stringResource(R.string.routing_sheet_title),
                style = MaterialTheme.typography.titleLarge,
                fontWeight = FontWeight.Bold,
            )
            Text(profile.name, style = MaterialTheme.typography.titleMedium)
            if (profile.publisherName.isNotEmpty()) {
                Text(
                    stringResource(R.string.routing_sheet_publisher, profile.publisherName),
                    style = MaterialTheme.typography.bodySmall,
                    color = Brand.SecondaryText,
                )
            }
            Text(
                stringResource(
                    R.string.routing_sheet_counts,
                    profile.ruleCount("direct"),
                    profile.ruleCount("proxy"),
                    profile.ruleCount("block"),
                ),
                style = MaterialTheme.typography.bodyMedium,
            )
            if (profile.geositeUrl.isNotEmpty() || profile.geoipUrl.isNotEmpty()) {
                Text(
                    stringResource(R.string.routing_sheet_geo),
                    style = MaterialTheme.typography.bodySmall,
                    color = Brand.SecondaryText,
                )
            }
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                TextButton(onClick = onDismiss, enabled = !busy) {
                    Text(stringResource(R.string.routing_sheet_decline))
                }
                Button(onClick = onAccept, enabled = !busy) {
                    Text(stringResource(R.string.routing_sheet_accept))
                }
            }
        }
    }
}
```

- [x] **Step 6: Развилка в `DeepLinkImporter`**

В `DeepLinkImporter.kt`:

1. В `ensureReposReady` добавить `RoutingProfileRepository.init(ctx)`.
2. В начало `import(context, rawUrl)`, сразу после `ensureReposReady(appCtx)`:

```kotlin
        // Ветка маршрутизации отделяется ДО расшифровки: routing-ссылка не
        // шифруется (профиль публичен), и decodeDeepLink на ней споткнулся бы.
        // Разделение делает Go: префиксов три, и копия их списка на Kotlin
        // разъехалась бы с парсером.
        if (runCatching { Mobile.isRoutingDeepLink(rawUrl) }.getOrDefault(false)) {
            previewRouting(appCtx, rawUrl)
            return
        }
```

3. Добавить метод:

```kotlin
    /**
     * Показать, что принесла ссылка, и ждать решения. Ничего не сохраняется и
     * ничего не качается: `previewRoutingDeepLink` только разбирает payload.
     */
    private fun previewRouting(ctx: Context, rawUrl: String) {
        val decoded = runCatching { Mobile.previewRoutingDeepLink(rawUrl) }
        val json = decoded.getOrNull()
        if (json == null) {
            val why = decoded.exceptionOrNull()?.message ?: "decode error"
            Toast.makeText(ctx, ctx.getString(R.string.routing_import_failed, why),
                Toast.LENGTH_LONG).show()
            AppLog.error(R.string.log_routing_compile_failed, "-", why)
            return
        }
        val profile = runCatching { routingProfileFromJson(JSONObject(json)) }.getOrNull()
        if (profile == null) {
            Toast.makeText(ctx, ctx.getString(R.string.routing_import_failed, "bad payload"),
                Toast.LENGTH_LONG).show()
            return
        }
        PendingRoutingImport.offer(profile)
    }
```

4. Добавить приёмник в конец файла (вне `DeepLinkImporter`):

```kotlin
/**
 * Разобранный профиль, ждущий решения пользователя.
 *
 * Отдельным объектом, потому что путь диплинка начинается в сервисе намерений,
 * а спрашивать некому, пока не открыта активность: MainActivity подписывается и
 * показывает лист, когда сможет.
 */
object PendingRoutingImport {
    private val _pending = MutableStateFlow<RoutingProfile?>(null)
    val pending: StateFlow<RoutingProfile?> = _pending.asStateFlow()

    fun offer(p: RoutingProfile) { _pending.value = p }
    fun clear() { _pending.value = null }
}
```

- [x] **Step 7: Запустить тесты**

```bash
cd /c/ResultV/android && ./gradlew :app:testFullDebugUnitTest :app:testPlayDebugUnitTest --console=plain 2>&1 | tail -6
```

Ожидается: BUILD SUCCESSFUL. Счёт: full 104, play 102.

- [x] **Step 8: Проверить ресурсы на опечатки**

```bash
cd /c/ResultV/android && ./gradlew :app:assembleFullDebug --console=plain 2>&1 | tail -6
```

Ожидается: BUILD SUCCESSFUL. Юнит-тесты ресурсы не собирают — опечатка в
strings.xml всплывает только здесь, поэтому шаг отдельный.

- [x] **Step 9: Коммит**

```bash
cd /c/ResultV
git add android/app/src/main/java/com/resultv/android/ui/components/RoutingDeepLinkSheet.kt \
        android/app/src/main/java/com/resultv/android/vpn/DeepLinkImporter.kt \
        android/app/src/main/java/com/resultv/android/vpn/RoutingProfiles.kt \
        android/app/src/main/res/values/strings.xml \
        android/app/src/main/res/values-ru/strings.xml \
        android/app/src/test/java/com/resultv/android/vpn/RoutingPreviewTest.kt
git commit -m "feat(routing): лист превью диплинка и развилка импорта

Ветка маршрутизации отделяется до расшифровки: routing-ссылка не шифруется,
профиль публичен, и decodeDeepLink на ней споткнулся бы. Разделение делает Go
— префиксов три, копия их списка на Kotlin разъехалась бы с парсером.

Молча не применяется ничего: сперва лист с именем, издателем и счётчиками,
правила качаются только после согласия.

Сломанный ответ merge читается как null, а не как пустое хранилище: второе
выбросило бы все профили пользователя из-за одной нечитаемой строки.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 5: Экран списка, ряд в правилах, маршрут

**Files:**
- Create: `android/app/src/main/java/com/resultv/android/ui/screens/RoutingProfilesScreen.kt`
- Modify: `android/app/src/main/java/com/resultv/android/ui/screens/RulesScreen.kt`
- Modify: `android/app/src/main/java/com/resultv/android/ui/screens/SettingsScreen.kt`
- Modify: `android/app/src/main/java/com/resultv/android/MainActivity.kt`
- Modify: `android/app/src/main/res/values/strings.xml`, `values-ru/strings.xml`

**Interfaces:**
- Consumes: всё из задач 1-4, `NavRow` (`SettingsScreen.kt:241`, `internal`)
- Produces: `@Composable fun RoutingProfilesScreen(dataDir: String, onClose: () -> Unit)`

- [x] **Step 1: Добавить строки**

`values/strings.xml`:

```xml
<string name="routing_profiles_title">Routing profiles</string>
<string name="routing_profiles_row">Routing profiles</string>
<string name="routing_profiles_empty">No routing profiles yet. Open a link from your provider to add one.</string>
<string name="routing_profiles_built">Built</string>
<string name="routing_profiles_not_built">Not built</string>
<string name="routing_profiles_rebuild">Build again</string>
<string name="routing_profiles_delete">Delete</string>
<string name="routing_profiles_delete_confirm">Delete profile \"%1$s\"?</string>
<string name="routing_profiles_off">Routing without a profile</string>
```

`values-ru/strings.xml`:

```xml
<string name="routing_profiles_title">Профили маршрутизации</string>
<string name="routing_profiles_row">Профили маршрутизации</string>
<string name="routing_profiles_empty">Профилей пока нет. Откройте ссылку от провайдера, чтобы добавить.</string>
<string name="routing_profiles_built">Собран</string>
<string name="routing_profiles_not_built">Не собран</string>
<string name="routing_profiles_rebuild">Собрать ещё раз</string>
<string name="routing_profiles_delete">Удалить</string>
<string name="routing_profiles_delete_confirm">Удалить профиль «%1$s»?</string>
<string name="routing_profiles_off">Маршрутизация без профиля</string>
```

- [x] **Step 2: Написать экран списка**

Создать `android/app/src/main/java/com/resultv/android/ui/screens/RoutingProfilesScreen.kt`.
Оболочка — та же, что у мастера сертификата (`CertWizardScreen.kt:97-113`):
`Scaffold` с `Brand.Bg` и `TopAppBar` со стрелкой назад.

```kotlin
package com.resultv.android.ui.screens

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.DeleteOutline
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.resultv.android.R
import com.resultv.android.theme.Brand
import com.resultv.android.vpn.ROUTING_ACTIONS
import com.resultv.android.vpn.RoutingProfile
import com.resultv.android.vpn.RoutingProfileCompiler
import com.resultv.android.vpn.RoutingProfileRepository
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * Список профилей маршрутизации: какой в силе, что в каждом и собран ли он.
 *
 * Открывается только из «Правил» и только в Global — там профиль и действует.
 * Редактора здесь нет, он этапа C: пока профили приходят ссылкой.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun RoutingProfilesScreen(dataDir: String, onClose: () -> Unit) {
    val state by RoutingProfileRepository.state.collectAsStateWithLifecycle()
    val generation by RoutingProfileRepository.compileGeneration.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var busyId by remember { mutableStateOf("") }
    var confirmDelete by remember { mutableStateOf<RoutingProfile?>(null) }

    // Готовность читается с диска — три stat на профиль. Раз на изменение
    // списка или на удачную сборку, а не на каждую рекомпозицию.
    var ready by remember { mutableStateOf<Map<String, Map<String, Boolean>>>(emptyMap()) }
    LaunchedEffect(state.profiles, generation) {
        ready = withContext(Dispatchers.IO) {
            state.profiles.associate { it.id to RoutingProfileCompiler.statusOf(dataDir, it.id) }
        }
    }

    Scaffold(
        containerColor = Brand.Bg,
        topBar = {
            TopAppBar(
                title = {
                    Text(
                        stringResource(R.string.routing_profiles_title),
                        fontWeight = FontWeight.Bold,
                    )
                },
                navigationIcon = {
                    IconButton(onClick = onClose) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = null)
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = Brand.Bg),
            )
        },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            if (state.profiles.isEmpty()) {
                Text(
                    stringResource(R.string.routing_profiles_empty),
                    style = MaterialTheme.typography.bodyMedium,
                    color = Brand.SecondaryText,
                    modifier = Modifier.padding(vertical = 24.dp),
                )
                return@Column
            }

            Card(
                shape = RoundedCornerShape(20.dp),
                colors = CardDefaults.cardColors(containerColor = Brand.Surface),
                modifier = Modifier.fillMaxWidth(),
            ) {
                Column {
                    // «Без профиля» — это выбор, а не его отсутствие: выключить
                    // маршрутизацию по профилю, ничего не удаляя.
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clickable { RoutingProfileRepository.setActive("") }
                            .padding(horizontal = 16.dp, vertical = 14.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Text(
                            stringResource(R.string.routing_profiles_off),
                            style = MaterialTheme.typography.bodyLarge,
                            modifier = Modifier.weight(1f),
                        )
                        RadioButton(
                            selected = state.activeId.isEmpty(),
                            onClick = { RoutingProfileRepository.setActive("") },
                        )
                    }
                    state.profiles.forEach { profile ->
                        HorizontalDivider(color = Brand.SurfaceHigh)
                        RoutingProfileRow(
                            profile = profile,
                            isActive = profile.id == state.activeId,
                            ready = ready[profile.id].orEmpty(),
                            busy = busyId == profile.id,
                            onSelect = { RoutingProfileRepository.setActive(profile.id) },
                            onRebuild = {
                                busyId = profile.id
                                scope.launch {
                                    RoutingProfileCompiler.compile(profile, dataDir, refreshGeo = true)
                                    busyId = ""
                                }
                            },
                            onDelete = { confirmDelete = profile },
                        )
                    }
                }
            }
        }
    }

    confirmDelete?.let { victim ->
        AlertDialog(
            onDismissRequest = { confirmDelete = null },
            title = { Text(stringResource(R.string.routing_profiles_delete_confirm, victim.name)) },
            confirmButton = {
                TextButton(onClick = {
                    confirmDelete = null
                    scope.launch {
                        // Кэш сносится ДО записи конфига: иначе он остаётся без
                        // владельца и продолжает маршрутизировать.
                        withContext(Dispatchers.IO) {
                            RoutingProfileCompiler.forget(dataDir, victim.id)
                        }
                        RoutingProfileRepository.delete(victim.id)
                    }
                }) { Text(stringResource(R.string.routing_profiles_delete)) }
            },
            dismissButton = {
                TextButton(onClick = { confirmDelete = null }) {
                    Text(stringResource(R.string.routing_sheet_decline))
                }
            },
            containerColor = Brand.Surface,
        )
    }
}

@Composable
private fun RoutingProfileRow(
    profile: RoutingProfile,
    isActive: Boolean,
    ready: Map<String, Boolean>,
    busy: Boolean,
    onSelect: () -> Unit,
    onRebuild: () -> Unit,
    onDelete: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(enabled = !busy, onClick = onSelect)
            .padding(start = 16.dp, end = 8.dp, top = 12.dp, bottom = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(profile.name, style = MaterialTheme.typography.bodyLarge)
            if (profile.publisherName.isNotEmpty()) {
                Text(
                    stringResource(R.string.routing_sheet_publisher, profile.publisherName),
                    style = MaterialTheme.typography.bodySmall,
                    color = Brand.MutedText,
                )
            }
            Text(
                stringResource(
                    R.string.routing_sheet_counts,
                    profile.ruleCount("direct"),
                    profile.ruleCount("proxy"),
                    profile.ruleCount("block"),
                ),
                style = MaterialTheme.typography.bodySmall,
                color = Brand.SecondaryText,
            )
            when {
                profile.lastError.isNotEmpty() -> {
                    Text(
                        profile.lastError,
                        style = MaterialTheme.typography.bodySmall,
                        color = Color(0xFFf87171),
                    )
                    TextButton(onClick = onRebuild, enabled = !busy) {
                        Text(stringResource(R.string.routing_profiles_rebuild))
                    }
                }
                // «Собран» означает, что на диске есть хоть один пригодный
                // rule-set: у профиля из одних proxy-правил два других действия
                // пусты законно.
                ROUTING_ACTIONS.any { ready[it] == true } -> Text(
                    stringResource(R.string.routing_profiles_built),
                    style = MaterialTheme.typography.bodySmall,
                    color = Brand.SecondaryText,
                )
                else -> {
                    Text(
                        stringResource(R.string.routing_profiles_not_built),
                        style = MaterialTheme.typography.bodySmall,
                        color = Brand.MutedText,
                    )
                    TextButton(onClick = onRebuild, enabled = !busy) {
                        Text(stringResource(R.string.routing_profiles_rebuild))
                    }
                }
            }
        }
        IconButton(onClick = onDelete, enabled = !busy) {
            Icon(
                Icons.Outlined.DeleteOutline,
                contentDescription = stringResource(R.string.routing_profiles_delete),
                tint = Brand.MutedText,
            )
        }
        RadioButton(selected = isActive, onClick = onSelect, enabled = !busy)
    }
}
```

- [x] **Step 3: Ряд в правилах, только в Global**

В `RulesScreen.kt`, в первую секцию (после `RoutingModeSelector`), добавить:

```kotlin
                // Профиль действует только в Global (спека, раздел 1). В Smart
                // ряда нет вовсе: объяснять, почему он неактивен, не нужно,
                // если его не показывать.
                if (rules.mode == RoutingMode.Global) {
                    NavRow(
                        label = stringResource(R.string.routing_profiles_row),
                        icon = Icons.Outlined.AltRoute,
                        iconBg = Color(0xFF3b82f6).copy(alpha = 0.18f),
                        iconTint = Color(0xFF60a5fa),
                        onClick = onOpenRoutingProfiles,
                    )
                }
```

`RulesScreen` получает параметр `onOpenRoutingProfiles: () -> Unit = {}`.
`SettingsScreen` прокидывает его так же, как `onOpenCertWizard`: **сперва гасит
лист**, потом зовёт (`activeSheet = null; onOpenRoutingProfiles()`) — лист это
подокно над `Scaffold`, полноэкранный маршрут его не перекрывает
(`SettingsScreen.kt:173-180`).

- [x] **Step 4: Маршрут и приём диплинка в `MainActivity`**

По образцу `showCertWizard` (`MainActivity.kt:341, 428-430`):

```kotlin
    var showRoutingProfiles by rememberSaveable { mutableStateOf(false) }
```

прокинуть `onOpenRoutingProfiles = { showRoutingProfiles = true }` в
`SettingsScreen`, и в конец корневого composable:

```kotlin
    if (showRoutingProfiles) {
        BackHandler { showRoutingProfiles = false }
        RoutingProfilesScreen(dataDir = dataDir, onClose = { showRoutingProfiles = false })
    }

    // Лист превью диплинка. Показывается поверх чего угодно: ссылка могла
    // прийти, пока открыт любой экран.
    val pendingRouting by PendingRoutingImport.pending.collectAsStateWithLifecycle()
    var importBusy by remember { mutableStateOf(false) }
    pendingRouting?.let { profile ->
        val scope = rememberCoroutineScope()
        val ctx = LocalContext.current
        RoutingDeepLinkSheet(
            profile = profile,
            busy = importBusy,
            onDismiss = { if (!importBusy) PendingRoutingImport.clear() },
            onAccept = {
                importBusy = true
                scope.launch {
                    val merged = withContext(Dispatchers.IO) {
                        runCatching {
                            Mobile.mergeRoutingProfile(
                                RoutingProfileRepository.storeJson(),
                                profile.toJson().toString(),
                                true,
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
                        val saved = state.profiles.firstOrNull { it.id == state.activeId }
                        if (saved != null) {
                            val outcome = RoutingProfileCompiler.compile(saved, dataDir)
                            val msg = when {
                                !outcome.ok -> ctx.getString(
                                    R.string.routing_import_failed, outcome.error)
                                outcome.unresolved.isNotEmpty() -> ctx.getString(
                                    R.string.routing_import_built_partly,
                                    outcome.unresolved.size)
                                else -> ctx.getString(R.string.routing_import_done, saved.name)
                            }
                            Toast.makeText(ctx, msg, Toast.LENGTH_LONG).show()
                        }
                    }
                    importBusy = false
                    PendingRoutingImport.clear()
                }
            },
        )
    }
```

`dataDir` в `MainActivity` уже есть — им пользуется `CertWizardScreen`.

- [x] **Step 5: Прогнать обе конфигурации**

```bash
cd /c/ResultV/android && ./gradlew :app:testFullDebugUnitTest :app:testPlayDebugUnitTest \
    :app:assembleFullDebug :app:assemblePlayDebug --console=plain 2>&1 | tail -8
```

Ожидается: BUILD SUCCESSFUL, счёт тестов не изменился с задачи 4
(full 104, play 102 — UI юнит-тестами не покрывается).

- [x] **Step 6: Коммит**

```bash
cd /c/ResultV
git add android/app/src/main/java/com/resultv/android/ui/screens/RoutingProfilesScreen.kt \
        android/app/src/main/java/com/resultv/android/ui/screens/RulesScreen.kt \
        android/app/src/main/java/com/resultv/android/ui/screens/SettingsScreen.kt \
        android/app/src/main/java/com/resultv/android/MainActivity.kt \
        android/app/src/main/res/values/strings.xml \
        android/app/src/main/res/values-ru/strings.xml
git commit -m "feat(routing): экран профилей, ряд в правилах и приём диплинка

Ряд виден только в Global, как на ПК (SmartRulesPage.jsx:175): в Smart
профиль не действует, и объяснять неактивный ряд не нужно, если его нет.

Экран — полноэкранный маршрут по образцу мастера сертификата; лист настроек
гасится до открытия, иначе экран ушёл бы под него.

Удаление активного профиля не назначает соседа активным: чьи правила в силе —
не то, что решают за пользователя.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 6: Приёмка на устройстве

Только AVD `Pixel_9_Pro` (android-36) или телефон на ядре < 6.11. **Не
`Medium_Phone`**: сборка правил — это TLS-запросы, а на 16-килобайтном AVD
любое TLS-действие убивает процесс (родительская спека, 4.1), и выглядеть это
будет как «профиль не собирается».

- [x] **Step 1: Собрать и поставить**

```bash
cd /c/ResultV/android && ./gradlew :app:assembleFullDebug --console=plain 2>&1 | tail -3
adb install -r app/build/outputs/apk/full/debug/app-full-debug.apk
```

- [x] **Step 2: Подготовить тестовую ссылку**

Собрать диплинк из payload по образцу `docs/ROUTING-DEEPLINK.md`. Нужна живая
`geosite.dat` (см. спеку, раздел 10, пункт 1) либо профиль из обычных доменов —
тогда geo-базы не потребуются вовсе:

```bash
python - <<'PY'
import base64, json
payload = {
  "Name": "Тестовый профиль",
  "RouteOrder": "block-proxy-direct",
  "DirectSites": ["example.com"],
  "ProxySites": ["ifconfig.me"],
  "BlockSites": ["ads.example"],
}
blob = base64.urlsafe_b64encode(json.dumps(payload, ensure_ascii=False).encode()).decode().rstrip("=")
print("resultv://routing/onadd/" + blob)
PY
```

Открыть на устройстве:

```bash
adb shell am start -a android.intent.action.VIEW -d '<ссылка>'
```

- [x] **Step 3: Пройти таблицу 8.4 спеки, строки 1-5**

| # | Проверка | Чем доказывается |
|---|---|---|
| 1 | Ряд профилей виден только в Global | переключить режим в «Правилах» туда и обратно |
| 2 | Диплинк открывает лист, а не применяется молча | лист со счётчиками до согласия |
| 3 | Профиль маршрутизирует | `adb logcat` → `match[N] … rule_set=prof-<id>-proxy => route(proxy)` при заходе на `ifconfig.me` |
| 4 | «Мимо ВПН» сильнее профиля | добавить `ifconfig.me` в «мимо ВПН», проверить `=> route(direct)` |
| 5 | Профиль игнорируется в Smart | переключить в Smart, в конфиге нет правил `prof-` |

Конфиг движка для строки 5 достать так:

```bash
adb shell run-as com.resultv.android cat files/last-config.json 2>/dev/null | grep -c "prof-"
```

Если файла нет — смотреть журнал приложения, где конфиг печатается при
`logLevel=debug`.

- [x] **Step 4: Записать результат**

Дописать в спеку раздел «Этап B закрыт» по образцу раздела 12: что получилось,
чем доказано, что осталось. Коммит с записью.

---

## Готовность этапа B

- [x] `testFullDebugUnitTest` и `testPlayDebugUnitTest` зелены
- [x] `assembleFullDebug` и `assemblePlayDebug` собираются
- [x] Строки 1-5 таблицы 8.4 пройдены на устройстве с доказательствами
- [x] Редактор профиля и маршрутизация из подписки НЕ трогались — это этап C
