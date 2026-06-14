package com.resultv.android.vpn

import android.content.Context
import android.content.SharedPreferences
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

private const val PREFS = "resultv_data_usage"
private const val K_DOWN = "accumulated_down"
private const val K_UP = "accumulated_up"

/**
 * Accumulates VPN session byte totals across service restarts.
 *
 * Each time the VPN stops, [addSession] is called with the session's
 * [downBytes]/[upBytes] from libbox's CommandClient cumulative counters.
 * The running total is persisted in SharedPreferences so it survives
 * process death.
 */
object DataUsageRepository {

    data class State(val downBytes: Long = 0L, val upBytes: Long = 0L)

    private val _state = MutableStateFlow(State())
    val state: StateFlow<State> = _state.asStateFlow()

    private var prefs: SharedPreferences? = null

    @Synchronized
    fun init(ctx: Context) {
        if (prefs != null) return
        prefs = ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
        load()
    }

    /** Append one session's byte counts to the running total. */
    fun addSession(downBytes: Long, upBytes: Long) {
        if (downBytes <= 0L && upBytes <= 0L) return
        val p = prefs ?: return
        val cur = _state.value
        val next = State(
            downBytes = cur.downBytes + downBytes.coerceAtLeast(0L),
            upBytes = cur.upBytes + upBytes.coerceAtLeast(0L),
        )
        _state.value = next
        p.edit().putLong(K_DOWN, next.downBytes).putLong(K_UP, next.upBytes).apply()
    }

    /** Clear accumulated totals (user-triggered reset). */
    fun reset() {
        _state.value = State()
        prefs?.edit()?.remove(K_DOWN)?.remove(K_UP)?.apply()
    }

    private fun load() {
        val p = prefs ?: return
        _state.value = State(
            downBytes = p.getLong(K_DOWN, 0L),
            upBytes = p.getLong(K_UP, 0L),
        )
    }
}
