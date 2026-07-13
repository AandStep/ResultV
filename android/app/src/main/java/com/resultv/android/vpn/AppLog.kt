package com.resultv.android.vpn

import android.content.Context
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import com.resultv.android.locale.LocaleManager

/** Severity of a log entry — mirrors the desktop logger's four types. */
enum class LogLevel { Info, Success, Warning, Error }

/**
 * One curated, human-readable app event. [message] is already localized at
 * emit time; [source] mirrors the desktop's bracketed prefixes (e.g.
 * "СИСТЕМА", "СЕТЬ") or is blank for plain events.
 */
data class LogEntry(
    val timestamp: Long,
    val level: LogLevel,
    val source: String,
    val message: String,
)

/**
 * In-memory event log mirroring the desktop's `internal/logger`. Holds the
 * last [CAPACITY] entries newest-first and exposes them as a [StateFlow] so
 * Compose recomposes automatically — no manual event subscription like the
 * Wails `EventsOn("log")` bridge.
 *
 * Entries are emitted from the VPN worker thread, coroutines and the main
 * thread, so mutation is `synchronized`. Storage is in-memory only and is
 * cleared on process death — matching the desktop, which keeps logs in a ring
 * buffer and does not persist them.
 */
object AppLog {
    private const val CAPACITY = 500

    private val lock = Any()
    private val _entries = MutableStateFlow<List<LogEntry>>(emptyList())
    val entries: StateFlow<List<LogEntry>> = _entries.asStateFlow()

    // Application context for resource-based overloads. Set once from any
    // entry point (MainActivity, VpnService, DeepLinkImporter); null in JVM
    // unit tests, where resource-based calls become silent no-ops.
    @Volatile private var appContext: Context? = null

    /** Wire a Context so resource-based overloads can resolve strings. Idempotent. */
    fun init(context: Context) {
        if (appContext == null) appContext = context.applicationContext
    }

    /**
     * Resolve [resId] in the user-chosen locale (LocaleManager wraps the app
     * context the same way MainActivity.attachBaseContext does). Returns ""
     * before [init] — callers use this for source tokens.
     */
    fun resolve(resId: Int): String {
        val ctx = appContext ?: return ""
        return LocaleManager.wrap(ctx).getString(resId)
    }

    fun info(message: String, source: String = "") = add(LogLevel.Info, source, message)
    fun success(message: String, source: String = "") = add(LogLevel.Success, source, message)
    fun warning(message: String, source: String = "") = add(LogLevel.Warning, source, message)
    fun error(message: String, source: String = "") = add(LogLevel.Error, source, message)

    fun info(resId: Int, vararg args: Any, source: String = "") =
        addRes(LogLevel.Info, resId, args, source)
    fun success(resId: Int, vararg args: Any, source: String = "") =
        addRes(LogLevel.Success, resId, args, source)
    fun warning(resId: Int, vararg args: Any, source: String = "") =
        addRes(LogLevel.Warning, resId, args, source)
    fun error(resId: Int, vararg args: Any, source: String = "") =
        addRes(LogLevel.Error, resId, args, source)

    fun clear() {
        synchronized(lock) { _entries.value = emptyList() }
    }

    private fun add(level: LogLevel, source: String, message: String) {
        val entry = LogEntry(
            timestamp = System.currentTimeMillis(),
            level = level,
            source = source,
            message = message,
        )
        synchronized(lock) {
            // Newest-first; cap at CAPACITY by dropping the oldest tail.
            val next = ArrayList<LogEntry>(minOf(_entries.value.size + 1, CAPACITY))
            next.add(entry)
            for (e in _entries.value) {
                if (next.size >= CAPACITY) break
                next.add(e)
            }
            _entries.value = next
        }
    }

    private fun addRes(level: LogLevel, resId: Int, args: Array<out Any>, source: String) {
        val ctx = appContext ?: return // unit tests / not yet initialized
        val localized = LocaleManager.wrap(ctx)
        val msg = if (args.isEmpty()) localized.getString(resId)
                  else localized.getString(resId, *args)
        add(level, source, msg)
    }
}
