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
import kotlinx.coroutines.withContext
import mobile.Mobile
import org.json.JSONObject
import java.io.File

private const val TAG = "ResultV/AdBlock"
private const val META_FILE = "adblock.meta.json"
/** Re-download the SRS lists at most once per 24h. */
private const val REFRESH_INTERVAL_MS = 24L * 60 * 60 * 1000

/**
 * Manages the ad-blocking rule-set (SRS) cache used by the engine's DNS+route
 * `reject` rules.
 *
 * The actual download lives on the Go side ([mobile.Mobile.fetchAdBlockLists]),
 * which writes the binary SRS files into `dataDir/adblock/` and reports how
 * many lists are cached. The engine references those local files when present
 * and falls back to a remote rule_set otherwise — so blocking still works (just
 * slower to warm up) even if this repository never ran, and a failed download
 * never blocks a connection from coming up.
 *
 * Kotlin keeps a thin status snapshot (ready/total) so the Settings UI can show
 * progress and so we only re-download on the 24h TTL.
 */
object AdBlockRepository {

    data class Snapshot(
        val ready: Int = 0,
        val total: Int = 0,
        val fetchedAt: Long = 0L,
        val lastError: String = "",
    ) {
        val hasLists: Boolean get() = ready > 0
        val isStale: Boolean
            get() = fetchedAt <= 0 || System.currentTimeMillis() - fetchedAt > REFRESH_INTERVAL_MS
    }

    private val _state = MutableStateFlow(Snapshot())
    val state: StateFlow<Snapshot> = _state.asStateFlow()

    @Volatile private var dataDir: String = ""
    @Volatile private var metaFile: File? = null

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val fetchLock = Mutex()

    @Synchronized
    fun init(ctx: Context) {
        if (metaFile != null) return
        dataDir = ctx.filesDir.absolutePath
        val f = File(ctx.filesDir, META_FILE)
        metaFile = f
        _state.value = loadMeta(f)
    }

    /** Block-download the lists if missing or stale. Concurrent calls coalesce. */
    suspend fun ensureLoaded(): Snapshot {
        val cur = _state.value
        if (cur.hasLists && !cur.isStale) {
            // Cache hit — surface the loaded lists like the desktop does at
            // startup, so the warm-cache path isn't silent (refresh() logs the
            // download path).
            AppLog.info(
                R.string.log_adblock_loaded, cur.ready, cur.total,
                source = AppLog.resolve(R.string.log_source_adblock),
            )
            return cur
        }
        return refresh()
    }

    /** Force a re-download ignoring the TTL. */
    suspend fun refresh(): Snapshot = fetchLock.withLock {
        val dd = dataDir
        if (dd.isBlank()) {
            Log.w(TAG, "refresh() called before init")
            return@withLock _state.value
        }
        val raw = try {
            withContext(Dispatchers.IO) { Mobile.fetchAdBlockLists(dd) }
        } catch (t: Throwable) {
            Log.w(TAG, "fetchAdBlockLists failed", t)
            AppLog.warning(R.string.log_adblock_update_failed,
                t.message ?: t.javaClass.simpleName,
                source = AppLog.resolve(R.string.log_source_adblock))
            val next = _state.value.copy(lastError = t.message ?: t.javaClass.simpleName)
            _state.value = next
            return@withLock next
        }
        val parsed = runCatching { parseSnapshot(raw) }.getOrNull()
        if (parsed == null) {
            AppLog.warning(R.string.log_adblock_update_failed,
                "could not parse engine response",
                source = AppLog.resolve(R.string.log_source_adblock))
            val next = _state.value.copy(lastError = "could not parse engine response")
            _state.value = next
            return@withLock next
        }
        _state.value = parsed
        saveMeta()
        AppLog.success(R.string.log_adblock_updated, parsed.ready, parsed.total,
            source = AppLog.resolve(R.string.log_source_adblock))
        parsed
    }

    /** Fire-and-forget refresh for UI use (Settings toggle, manual refresh). */
    fun refreshAsync() {
        scope.launch { refresh() }
    }

    /** Fire-and-forget TTL-respecting load (app startup when ad-block is on). */
    fun ensureLoadedAsync() {
        scope.launch { ensureLoaded() }
    }

    // ───────────────────────── Internals ─────────────────────────

    private fun parseSnapshot(rawJson: String): Snapshot {
        val o = JSONObject(rawJson)
        return Snapshot(
            ready = o.optInt("ready", 0),
            total = o.optInt("total", 0),
            fetchedAt = System.currentTimeMillis(),
            lastError = o.optString("error"),
        )
    }

    private fun loadMeta(f: File): Snapshot {
        if (!f.exists()) return Snapshot()
        return try {
            val o = JSONObject(f.readText())
            Snapshot(
                ready = o.optInt("ready", 0),
                total = o.optInt("total", 0),
                fetchedAt = o.optLong("fetchedAt", 0L),
                lastError = o.optString("lastError"),
            )
        } catch (t: Throwable) {
            Log.w(TAG, "failed to read $f, starting empty", t)
            AppLog.warning(R.string.log_read_failed, f.name,
                source = AppLog.resolve(R.string.log_source_config))
            Snapshot()
        }
    }

    private fun saveMeta() {
        val f = metaFile ?: return
        val s = _state.value
        try {
            val o = JSONObject()
                .put("ready", s.ready)
                .put("total", s.total)
                .put("fetchedAt", s.fetchedAt)
                .put("lastError", s.lastError)
            f.writeText(o.toString())
        } catch (t: Throwable) {
            Log.e(TAG, "failed to persist adblock meta", t)
            AppLog.error(R.string.log_persist_failed, f.name,
                source = AppLog.resolve(R.string.log_source_config))
        }
    }
}
