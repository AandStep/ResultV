package com.resultv.android.vpn

import android.content.Context
import android.util.Log
import com.resultv.android.R
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
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
private const val SEED_ASSET = "smart-ru.srs"
/** Re-fetch the upstream list at most once per 24h to be a polite citizen. */
private const val REFRESH_INTERVAL_MS = 24L * 60 * 60 * 1000

/**
 * Whether a freshly-fetched smart-list result (source [nextSource]) should
 * replace the current snapshot (source [curSource], [curEmpty]).
 *
 * A transient failure to reach the antizapret list makes the engine fall back to
 * the small builtin list (source "builtin"). Applying it would collapse the
 * per-app allowlist and kill traffic — so a builtin result must NOT overwrite an
 * already-loaded real ("remote"/"cache") list. On a cold start builtin IS
 * accepted — some blocking beats none.
 */
internal fun shouldReplaceSmartSnapshot(
    curSource: String,
    curEmpty: Boolean,
    nextSource: String,
): Boolean {
    if (nextSource == "builtin" && !curEmpty && curSource != "builtin") return false
    return true
}

/**
 * Tracks the Antizapret-style blocked-domain list used by Smart routing.
 *
 * The list itself lives ONLY on disk, as a compiled binary sing-box rule-set
 * (`dataDir/smart/smart.srs`, written by the engine). Kotlin keeps just
 * metadata. It used to keep all ~150k domains in memory and re-serialise them
 * into the connect config, which cost ~2s per connect (4.6 MB marshalled, three
 * JNI crossings, a full sing-box re-parse) plus a multi-second org.json parse on
 * the main thread at every app start. Both are gone.
 *
 * A seed rule-set ships in the APK, so even a fresh install has a correct list
 * before its first connect; the background refresh replaces it on the 24h TTL.
 */
object SmartListRepository {

    data class Snapshot(
        val count: Int = 0,
        val country: String = "",
        val source: String = "",
        val fetchedAt: Long = 0L,
        val lastError: String = "",
        /** A usable compiled rule-set exists on disk (seeded or downloaded). */
        val ready: Boolean = false,
    ) {
        /** No usable list — Smart routing must fall back to Global. */
        val isEmpty: Boolean get() = !ready
        val isStale: Boolean
            get() = fetchedAt <= 0 || System.currentTimeMillis() - fetchedAt > REFRESH_INTERVAL_MS
    }

    private val _state = MutableStateFlow(Snapshot())
    val state: StateFlow<Snapshot> = _state.asStateFlow()

    // Empty string = auto-detect via FetchSmartList server-side geo-detection.
    @Volatile private var country: String = ""
    @Volatile private var dataDir: String = ""
    @Volatile private var metaFile: File? = null
    // The in-flight (or already-finished) bundled-seed install kicked off by
    // init(), so a connect can await it — see awaitSeedInstall().
    @Volatile private var seedInstallJob: Job? = null

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val fetchLock = Mutex()

    /**
     * Cheap and main-thread-safe: reads a few-hundred-byte meta file and asks
     * the engine whether an SRS is on disk. The actual seed install (which
     * touches assets + disk) is dispatched to IO.
     */
    @Synchronized
    fun init(ctx: Context) {
        if (metaFile != null) return
        dataDir = ctx.filesDir.absolutePath
        val f = File(ctx.filesDir, META_FILE)
        metaFile = f
        _state.value = loadMeta(f).copy(ready = srsReady())
        val appCtx = ctx.applicationContext
        seedInstallJob = scope.launch { installSeedIfNeeded(appCtx) }
    }

    /**
     * Await the bundled-seed install kicked off by [init], if it's still in
     * flight. LOCAL DISK ONLY — an asset read, an SRS validation, and a JNI
     * install call, never a network request. [installSeedIfNeeded] catches
     * every Throwable internally, so this Job always completes normally
     * (never hangs), whether the seed installs, was already unnecessary
     * (a real list already on disk), or failed. A no-op if [init] was never
     * called.
     */
    suspend fun awaitSeedInstall() {
        seedInstallJob?.join()
    }

    /** Pin the smart-list country (ISO alpha-2). Triggers a refresh if changed. */
    fun setCountry(cc: String) {
        val normalised = cc.trim().lowercase()
        if (normalised.isBlank() || normalised == country) return
        country = normalised
        // Keep `ready` — the existing SRS still routes until the new one lands.
        _state.value = Snapshot(country = normalised, ready = srsReady())
        saveMeta()
    }

    /** Fetch the list if missing or stale. Concurrent calls coalesce. */
    suspend fun ensureLoaded(): Snapshot {
        val cur = _state.value
        if (!cur.isEmpty && !cur.isStale) {
            AppLog.info(
                R.string.log_smart_loaded,
                cur.country.uppercase().ifBlank { "?" }, cur.count,
                source = AppLog.resolve(R.string.log_source_smart),
            )
            return cur
        }
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
            Log.w(TAG, "fetchSmartList($cc) failed", t)
            AppLog.warning(R.string.log_smart_update_failed,
                t.message ?: t.javaClass.simpleName,
                source = AppLog.resolve(R.string.log_source_smart))
            val next = _state.value.copy(
                lastError = t.message ?: t.javaClass.simpleName,
                ready = srsReady(),
            )
            _state.value = next
            return@withLock next
        }
        val parsed = runCatching { parseSnapshot(raw) }.getOrNull()
        if (parsed == null) {
            AppLog.warning(R.string.log_smart_update_failed,
                "could not parse engine response",
                source = AppLog.resolve(R.string.log_source_smart))
            val next = _state.value.copy(
                lastError = "could not parse engine response",
                ready = srsReady(),
            )
            _state.value = next
            return@withLock next
        }
        val cur = _state.value
        if (!shouldReplaceSmartSnapshot(cur.source, cur.isEmpty, parsed.source)) {
            Log.i(TAG, "ignoring builtin smart-list (${parsed.count}); keeping ${cur.source} (${cur.count})")
            return@withLock cur
        }
        if (parsed.country.isNotBlank()) country = parsed.country
        _state.value = parsed
        saveMeta()
        AppLog.info(R.string.log_smart_updated,
            parsed.country.uppercase().ifBlank { "?" }, parsed.count,
            source = AppLog.resolve(R.string.log_source_smart))
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

    /** A usable compiled rule-set is on disk — Smart routing can engage. */
    fun isReady(): Boolean = _state.value.ready

    // ───────────────────────── Internals ─────────────────────────

    /**
     * Install the APK-bundled rule-set when nothing usable is cached yet, so the
     * FIRST connect on a fresh install already routes Smart correctly instead of
     * falling back to Global (every app in the tunnel) while a download runs.
     * A no-op once a real list exists — the engine refuses to overwrite it.
     */
    private fun installSeedIfNeeded(ctx: Context) {
        val dd = dataDir
        if (dd.isBlank()) return
        try {
            if (srsReady()) return
            val bytes = ctx.assets.open(SEED_ASSET).use { it.readBytes() }
            val installed = Mobile.installSmartSRSSeed(dd, bytes)
            if (installed) {
                Log.i(TAG, "installed bundled smart seed (${bytes.size} bytes)")
                _state.value = _state.value.copy(ready = true, source = "seed")
            }
        } catch (t: Throwable) {
            // Fail-safe: no seed just means the first connect uses Global
            // routing until the download lands. Never break startup for this.
            Log.w(TAG, "smart seed install failed", t)
        }
    }

    private fun srsReady(): Boolean {
        val dd = dataDir
        if (dd.isBlank()) return false
        return try {
            JSONObject(Mobile.smartListStatus(dd)).optBoolean("srsReady", false)
        } catch (t: Throwable) {
            Log.w(TAG, "smartListStatus failed", t)
            false
        }
    }

    private fun parseSnapshot(rawJson: String): Snapshot {
        val o = JSONObject(rawJson)
        return Snapshot(
            count = o.optInt("count", 0),
            country = o.optString("country").ifBlank { country },
            source = o.optString("source"),
            fetchedAt = System.currentTimeMillis(),
            lastError = o.optString("error"),
            ready = o.optBoolean("srsReady", false) || srsReady(),
        )
    }

    private fun loadMeta(f: File): Snapshot {
        if (!f.exists()) return Snapshot(country = country)
        return try {
            val o = JSONObject(f.readText())
            country = o.optString("country").ifBlank { country }
            Snapshot(
                count = o.optInt("count", 0),
                country = country,
                source = o.optString("source"),
                fetchedAt = o.optLong("fetchedAt", 0L),
                lastError = o.optString("lastError"),
            )
        } catch (t: Throwable) {
            Log.w(TAG, "failed to read $f, starting empty", t)
            AppLog.warning(R.string.log_read_failed, f.name,
                source = AppLog.resolve(R.string.log_source_config))
            Snapshot(country = country)
        }
    }

    private fun saveMeta() {
        val f = metaFile ?: return
        val s = _state.value
        try {
            val o = JSONObject()
                .put("country", s.country.ifBlank { country })
                .put("count", s.count)
                .put("source", s.source)
                .put("fetchedAt", s.fetchedAt)
                .put("lastError", s.lastError)
            f.writeText(o.toString())
        } catch (t: Throwable) {
            Log.e(TAG, "failed to persist smart-list meta", t)
            AppLog.error(R.string.log_persist_failed, f.name,
                source = AppLog.resolve(R.string.log_source_config))
        }
    }
}
