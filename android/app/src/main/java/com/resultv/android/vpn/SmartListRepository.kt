package com.resultv.android.vpn

import android.content.Context
import android.util.Log
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

private const val TAG = "ResultV/SmartList"
private const val META_FILE = "smart-list.meta.json"
/** Re-fetch the upstream list at most once per 24h to be a polite citizen. */
private const val REFRESH_INTERVAL_MS = 24L * 60 * 60 * 1000

/**
 * Holds the Antizapret-style blocked-domain list used by Smart routing.
 *
 * Engine side ([mobile.Mobile.fetchSmartList]) handles the actual download
 * and caches the JSON next to the user data in `dataDir/smart-blocked.json`.
 * The Kotlin side keeps a thin in-memory snapshot of the parsed result so
 * the connect path can hand the engine `\n`-separated domains via
 * [BuildOptionsBuilder] without re-reading the cache file on every call.
 *
 * Country is currently pinned to "ru" — Smart mode is Antizapret-style and
 * makes no sense outside that context until the Settings UI adds a region
 * picker. Override via [setCountry] for testing.
 */
object SmartListRepository {

    data class Snapshot(
        val domains: List<String> = emptyList(),
        val country: String = "",
        val source: String = "",
        val fetchedAt: Long = 0L,
        val lastError: String = "",
    ) {
        val isEmpty: Boolean get() = domains.isEmpty()
        val isStale: Boolean
            get() = fetchedAt <= 0 || System.currentTimeMillis() - fetchedAt > REFRESH_INTERVAL_MS
    }

    private val _state = MutableStateFlow(Snapshot())
    val state: StateFlow<Snapshot> = _state.asStateFlow()

    @Volatile private var country: String = "ru"
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

    /** Pin the smart-list country (ISO alpha-2). Triggers a refresh if changed. */
    fun setCountry(cc: String) {
        val normalised = cc.trim().lowercase()
        if (normalised.isBlank() || normalised == country) return
        country = normalised
        // Wipe stale data so the next ensureLoaded refetches for the new country.
        _state.value = Snapshot(country = normalised)
        saveMeta()
    }

    /**
     * Block-fetch the list if missing or stale. Safe to call from any thread;
     * concurrent calls coalesce via [fetchLock]. Returns the new snapshot.
     */
    suspend fun ensureLoaded(): Snapshot {
        val cur = _state.value
        if (!cur.isEmpty && !cur.isStale) return cur
        return refresh()
    }

    /** Force a refetch ignoring TTL. */
    suspend fun refresh(): Snapshot = fetchLock.withLock {
        val dd = dataDir
        if (dd.isBlank()) {
            Log.w(TAG, "refresh() called before init")
            return@withLock _state.value
        }
        val cc = country
        val raw = try {
            withContext(Dispatchers.IO) { Mobile.fetchSmartList(cc, dd) }
        } catch (t: Throwable) {
            Log.w(TAG, "fetchSmartList(${cc}) failed", t)
            val next = _state.value.copy(lastError = t.message ?: t.javaClass.simpleName)
            _state.value = next
            return@withLock next
        }
        val parsed = runCatching { parseSnapshot(raw) }.getOrNull()
        if (parsed == null) {
            val next = _state.value.copy(lastError = "could not parse engine response")
            _state.value = next
            return@withLock next
        }
        _state.value = parsed
        saveMeta()
        parsed
    }

    /** Fire-and-forget refresh for UI use (Rules toggle, manual refresh). */
    fun refreshAsync() {
        scope.launch { refresh() }
    }

    /** Fire-and-forget TTL-respecting load (app startup). */
    fun ensureLoadedAsync() {
        scope.launch { ensureLoaded() }
    }

    /** Engine wire format: newline-separated, one domain per line. */
    fun toEngineList(): String {
        val s = _state.value
        if (s.domains.isEmpty()) return ""
        return s.domains.joinToString("\n")
    }

    // ───────────────────────── Internals ─────────────────────────

    private fun parseSnapshot(rawJson: String): Snapshot {
        val o = JSONObject(rawJson)
        val arr = o.optJSONArray("domains")
        val list = if (arr == null) emptyList()
        else (0 until arr.length()).mapNotNull { arr.optString(it).ifBlank { null } }
        return Snapshot(
            domains = list,
            country = o.optString("country").ifBlank { country },
            source = o.optString("source"),
            fetchedAt = System.currentTimeMillis(),
            lastError = o.optString("error"),
        )
    }

    private fun loadMeta(f: File): Snapshot {
        if (!f.exists()) return Snapshot(country = country)
        return try {
            val o = JSONObject(f.readText())
            country = o.optString("country").ifBlank { country }
            val arr = o.optJSONArray("domains")
            val list = if (arr == null) emptyList()
            else (0 until arr.length()).mapNotNull { arr.optString(it).ifBlank { null } }
            Snapshot(
                domains = list,
                country = country,
                source = o.optString("source"),
                fetchedAt = o.optLong("fetchedAt", 0L),
                lastError = o.optString("lastError"),
            )
        } catch (t: Throwable) {
            Log.w(TAG, "failed to read $f, starting empty", t)
            Snapshot(country = country)
        }
    }

    private fun saveMeta() {
        val f = metaFile ?: return
        val s = _state.value
        try {
            val arr = org.json.JSONArray()
            s.domains.forEach { arr.put(it) }
            val o = JSONObject()
                .put("country", s.country.ifBlank { country })
                .put("domains", arr)
                .put("source", s.source)
                .put("fetchedAt", s.fetchedAt)
                .put("lastError", s.lastError)
            f.writeText(o.toString())
        } catch (t: Throwable) {
            Log.e(TAG, "failed to persist smart-list meta", t)
        }
    }
}
