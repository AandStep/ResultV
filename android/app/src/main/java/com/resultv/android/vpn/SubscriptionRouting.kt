package com.resultv.android.vpn

import android.util.Log
import com.resultv.android.R
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import mobile.Mobile
import org.json.JSONObject

private const val TAG = "ResultV/SubRouting"

/**
 * Проставить профилю подписки то, чего Go знать не может.
 *
 * `subscriptionId` живёт только на стороне Kotlin, а `originName` — тот
 * опознавательный знак, по которому следующая синхронизация находит свой
 * профиль вместо того, чтобы завести второй.
 *
 * Возвращает `null`, если принимать нечего: профиль без единого правила занял
 * бы строку в списке, предложил себя включить и ничего бы не маршрутизировал.
 */
fun tagSubscriptionProfile(json: String, subId: String, subName: String): RoutingProfile? {
    val raw = json.trim()
    if (raw.isEmpty()) return null
    val parsed = runCatching { routingProfileFromJson(JSONObject(raw)) }.getOrNull() ?: return null
    // Панели отдают Profile-Title как `base64:<UTF-8>`, чтобы безопасно провезти
    // эмодзи, и Go кладёт заголовок в профиль как есть — про эту условность он
    // не знает. Без раскодирования в списке стояло бы
    // «base64:8J+agCBpbXBWUE4…» вместо названия. Раскодируется и имя, и
    // опознавательный знак: сравнивай их следующая синхронизация в разном виде,
    // она завела бы второй профиль.
    val handle = decodePanelTitle(
        parsed.originName.ifBlank { parsed.name }.ifBlank { subName }
    ).trim()
    val tagged = parsed.copy(
        name = decodePanelTitle(parsed.name).trim().ifBlank { handle },
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
 * Профиль уже собран на стороне Go (`BuildSubscriptionRoutingProfile`) и
 * приехал ключом `routing` в ответе `FetchSubscriptionV3`; здесь он только
 * помечается своей подпиской, сливается с хранилищем и собирается.
 */
object SubscriptionRouting {

    // Своя область для вызовов из мест, которые не suspend, — так же устроен
    // SmartListRepository.refreshAsync. Приём переживает закрытие экрана: он
    // не про UI, а про то, что уже скачано.
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    /** [accept] для вызывающих, которые не suspend. */
    fun acceptAsync(
        routingJson: String,
        subId: String,
        subName: String,
        dataDir: String,
        activate: Boolean,
    ) {
        scope.launch { accept(routingJson, subId, subName, dataDir, activate) }
    }

    /**
     * @param activate только когда пользователь сам согласился на эту подписку.
     *   Фоновое обновление профиль обновляет, но активным не делает: иначе оно
     *   перебивало бы выбор, сделанный с тех пор.
     */
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
        // Профиль уходит БЕЗ id: это публикация, а не правка. Своё сохранённое
        // он найдёт по subscriptionId (SameRoutingProfile), и слияние обновит
        // ему правила и ссылки на списки, сберегая имя, если его меняли руками.
        // Подсунуть сюда id сохранённого значило бы выдать синхронизацию за
        // правку — а правка бережёт у сохранённого ровно то, что подписка и
        // должна обновлять: ссылки на списки и имя издателя.
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
        AppLog.info(
            R.string.log_routing_from_subscription, subName,
            source = AppLog.resolve(R.string.log_source_config),
        )
        RoutingProfileCompiler.compile(saved, dataDir)
    }

    /**
     * Убрать профиль подписки вместе с его кэшем правил. Кэш сносится ПЕРВЫМ:
     * иначе он остаётся без владельца и продолжает маршрутизировать.
     */
    fun forget(subId: String, dataDir: String) {
        val victim = RoutingProfileRepository.state.value.profiles.firstOrNull {
            it.source == "subscription" && it.subscriptionId == subId
        } ?: return
        RoutingProfileCompiler.forget(dataDir, victim.id)
        RoutingProfileRepository.delete(victim.id)
    }
}
