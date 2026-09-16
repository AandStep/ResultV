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
 * Что сказала сборка: сколько правил получилось у каждого действия и что она не
 * смогла выразить.
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

/**
 * Разбор отчёта `Mobile.compileRoutingProfile`.
 *
 * Нечитаемый отчёт — это отказ, а не пустой результат: второе выглядело бы как
 * успешно собранный профиль без единого правила.
 */
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
     * Отказ ничего не откатывает — профиль остаётся сохранённым, причина
     * ложится в его `lastError`, и собрать можно повторно, не открывая ссылку
     * заново. Это поведение ПК (`app_routingprofile.go:92-97`): сборка ходит в
     * сеть, и её отказ — не повод терять то, что пользователь уже принял.
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
            AppLog.error(
                R.string.log_routing_compile_failed, profile.name, message,
                source = AppLog.resolve(R.string.log_source_config),
            )
            return@withLock RoutingCompileOutcome(error = message)
        }
        val outcome = parseRoutingCompileReport(raw)
        if (!outcome.ok) {
            RoutingProfileRepository.setLastError(profile.id, outcome.error)
            AppLog.error(
                R.string.log_routing_compile_failed, profile.name, outcome.error,
                source = AppLog.resolve(R.string.log_source_config),
            )
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
            AppLog.warning(
                R.string.log_routing_unresolved, profile.name, outcome.unresolved.size,
                source = AppLog.resolve(R.string.log_source_config),
            )
        }
        outcome
    }

    /** Какие действия профиля уже собраны. Дёшево: три `stat` по файлам. */
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
