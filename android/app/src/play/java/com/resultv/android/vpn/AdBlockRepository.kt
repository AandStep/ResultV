package com.resultv.android.vpn

import android.content.Context
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/**
 * Заглушка для дистрибутива Play.
 *
 * DNS-фильтрации рекламы в этой сборке нет ни в .so (тег `no_adblock` +
 * `internal/proxy/adblock_stub.go`), ни в UI (`BuildConfig.DNS_ADBLOCK`), —
 * значит нечего и кэшировать. Реализация живёт в `src/full`.
 *
 * Класс вынесен ради ресурсов: строки журнала про блок-листы лежат в
 * `src/full/res` и в play-APK не попадают, а общий код не может ссылаться на
 * то, чего в сборке нет.
 *
 * Состояние всегда пустое, и это правда: `hasLists = false` — списков
 * действительно нет. Общие вызовы (`ResultVpnService.listsAlreadyOnDisk`,
 * `MainActivity`) читают его через флаг `SettingsRepository.adblock`, который
 * в play всегда `false`, так что до сюда доходит только `init`.
 */
object AdBlockRepository {

    data class Snapshot(
        val ready: Int = 0,
        val total: Int = 0,
        val fetchedAt: Long = 0L,
        val lastError: String = "",
    ) {
        val hasLists: Boolean get() = false
        val isStale: Boolean get() = false
    }

    private val _state = MutableStateFlow(Snapshot())
    val state: StateFlow<Snapshot> = _state.asStateFlow()

    fun init(ctx: Context) {}

    suspend fun ensureLoaded(): Snapshot = _state.value

    suspend fun refresh(): Snapshot = _state.value

    fun refreshAsync() {}

    fun ensureLoadedAsync() {}
}
