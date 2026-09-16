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

/** Действия профиля, в том же порядке, что `proxy.RoutingActions` в Go. */
val ROUTING_ACTIONS = listOf("direct", "proxy", "block")

/**
 * Один набор правил маршрутизации — зеркало `config.RoutingProfile` из Go.
 *
 * Имена полей в JSON совпадают с тамошними буква в букву и переименованию не
 * подлежат: это единственное, что связывает две стороны, и расхождение ничего
 * не уронит — поле просто исчезнет на круге через хранилище.
 *
 * Профиль действует только в режиме Global. Это обеспечивает движок
 * (`applyRoutingProfile` в mobile/libbox_routing.go игнорирует профиль при
 * SmartMode), а UI лишь не показывает того, чего нет.
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
     * применяется — ни здесь, ни на ПК, где поле объявлено и не читается никем,
     * кроме парсера. В редактор не выносится: контрол, который ничего не
     * делает, хуже отсутствующего.
     */
    val domainStrategy: String = "",
    val geoipUrl: String = "",
    val geositeUrl: String = "",
    /** Ссылки на списки правил, по действиям: "direct" / "proxy" / "block". */
    val listUrls: Map<String, List<String>> = emptyMap(),
    val allowInsecure: Boolean = false,
    /** "manual" | "deeplink" | "subscription". */
    val source: String = "",
    val subscriptionId: String = "",
    /**
     * Имя, под которым профиль опубликовал издатель. [name] пользователь может
     * менять, это — нет: только по нему повторно открытая ссылка узнаёт свой
     * профиль, потому что идентификатора в payload нет вовсе.
     */
    val originName: String = "",
    val updatedAt: Long = 0L,
    /** Причина последней неудавшейся сборки; пустая строка — всё хорошо. */
    val lastError: String = "",
) {
    /**
     * Сколько правил у действия — для строки «• 12 direct • 3 block».
     *
     * Ссылка на список считается за единицу: сколько правил за ней, неизвестно,
     * пока её не скачали, а скомпилированный rule-set обратно не читается.
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

    /**
     * Имя издателя, когда оно отличается от показываемого. Совпадает — пусто:
     * строка «Издатель: Х» под заголовком «Х» не сообщает ничего.
     */
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
    return (0 until arr.length()).mapNotNull { i -> arr.optString(i).takeIf { it.isNotBlank() } }
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
            // бы id, по которому на диске нет ни одного файла, а строка списка
            // осталась бы без выделения.
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

/**
 * Ключ, по которому сервис решает, пересобирать ли конфиг из-за профиля.
 *
 * Не весь стейт репозитория: правка НЕактивного профиля не должна рвать живое
 * соединение. Двигают ключ только три вещи — режим (в Smart профиля нет
 * вовсе), выбор активного и удачная пересборка именно его правил.
 */
fun routingReloadKey(mode: RoutingMode, activeId: String, compileGeneration: Int): String =
    if (mode == RoutingMode.Smart) "smart" else "global:$activeId:$compileGeneration"

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

/**
 * Хранилище профилей: файл `routing_profiles.json` и `StateFlow` над ним.
 *
 * ОТЛИЧИЕ ОТ СОСЕДЕЙ: запись идёт на IO, а не прямо в [mutate]. У соседних
 * репозиториев файл — единицы килобайт, у этого профиль подписки со встроенными
 * xray-правилами тянет на сотни, и синхронная запись оказалась бы на главном
 * потоке. Состояние обновляется сразу, записи выстраивает мьютекс.
 */
object RoutingProfileRepository {
    private val _state = MutableStateFlow(RoutingProfilesState())
    val state: StateFlow<RoutingProfilesState> = _state.asStateFlow()

    /**
     * Счётчик удачных сборок активного профиля. На него смотрит ключ
     * перезапуска движка (см. [routingReloadKey]).
     *
     * Живёт только в памяти и полем модели не становится: после перезапуска
     * процесса конфиг всё равно собирается заново на коннекте, а рвать
     * соединение из-за сборки, случившейся в прошлой жизни приложения, — ровно
     * то лишнее действие, от которого этот ключ и защищает.
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
                AppLog.warning(
                    R.string.log_read_failed, f.name,
                    source = AppLog.resolve(R.string.log_source_config),
                )
                RoutingProfilesState()
            }
        } else {
            RoutingProfilesState()
        }
    }

    /** Текущее хранилище в том виде, в каком его ждёт `Mobile.mergeRoutingProfile`. */
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
     * пользователя. Кэш правил сносит вызывающая сторона: где он лежит, знает
     * только Go.
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
                    AppLog.error(
                        R.string.log_persist_failed, f.name,
                        source = AppLog.resolve(R.string.log_source_config),
                    )
                }
            }
        }
    }
}

/**
 * Список правил как текст для редактора: одно правило на строку.
 *
 * Так правит ПК (`RoutingProfileEditor.jsx:242-247`), и это же снимает вопрос
 * двадцати тысяч токенов: многострочное поле — один элемент, а не 20 000 чипов
 * в `FlowRow`.
 */
fun routingLinesOf(list: List<String>): String = list.joinToString("\n")

/**
 * Обратно: непустые строки без окружающих пробелов.
 *
 * `\r` срезается вместе с ними — список, скопированный из письма или с сайта,
 * приходит с CRLF, и хвостовой возврат каретки превратил бы каждый токен в
 * мусор, который потом молча не совпал бы ни с чем.
 */
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
