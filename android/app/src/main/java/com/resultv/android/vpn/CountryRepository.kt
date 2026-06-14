package com.resultv.android.vpn

import android.util.Log
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.joinAll
import kotlinx.coroutines.launch
import mobile.Mobile
import org.json.JSONObject

private const val TAG = "ResultV/Country"

/**
 * Resolves each proxy server's host to an ISO-3166 alpha-2 country code via
 * `Mobile.detectCountry`, which queries the project GeoIP API and caches
 * results on disk (country.cache.json). This is how standalone / panel
 * servers whose *name* has no flag hint (e.g. "Germany #36610", a bare IP)
 * still light up with the right flag — mirroring the desktop's
 * App.DetectCountry flow.
 *
 * Entries that already carry a country (a leading flag emoji in the name,
 * surfaced as [Profile.country]) are skipped — that hint wins, same as
 * desktop. Results are keyed by profile.id and published to a StateFlow so
 * ServerRow recomposes its flag as lookups land.
 */
object CountryRepository {

    private val _results = MutableStateFlow<Map<String, String>>(emptyMap())
    /** profile.id → lowercase 2-letter country code (only successful lookups). */
    val results: StateFlow<Map<String, String>> = _results.asStateFlow()

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    // Profile ids with a lookup in flight — guards against re-dispatching the
    // same host while the network call is pending (resolve() is called on
    // every profiles change).
    private val inFlight = HashSet<String>()

    // Parsed-URI cache so we don't re-cross the JNI boundary to pull the host
    // out of a URI-only profile on every resolve pass.
    private val uriCache = HashMap<String, JSONObject>()

    /**
     * Resolve country codes for every passed profile that still needs one.
     * Safe to call repeatedly (e.g. from a LaunchedEffect keyed on the
     * profile list) — already-resolved and in-flight profiles are skipped.
     */
    fun resolve(profiles: List<Profile>, dataDir: String) {
        val pending = synchronized(this) {
            profiles.mapNotNull { p ->
                if (!needsResolution(p)) return@mapNotNull null
                val host = hostFor(p) ?: return@mapNotNull null
                inFlight += p.id
                p.id to host
            }
        }
        if (pending.isEmpty()) return
        scope.launch {
            pending.chunked(8).forEach { chunk ->
                chunk.map { (id, host) ->
                    launch {
                        try {
                            val cc = runCatching { Mobile.detectCountry(host, dataDir) }
                                .getOrElse {
                                    Log.d(TAG, "detectCountry($host) failed: ${it.message}")
                                    ""
                                }
                            if (cc.length == 2) publish(id, cc.lowercase())
                        } finally {
                            synchronized(this@CountryRepository) { inFlight -= id }
                        }
                    }
                }.joinAll()
            }
        }
    }

    @Synchronized
    private fun publish(id: String, cc: String) {
        _results.value = _results.value + (id to cc)
    }

    /** Caller holds the monitor. */
    private fun needsResolution(p: Profile): Boolean {
        if (p.isSection || p.isAuto) return false
        if (!p.country.isNullOrBlank()) return false   // name already carries a flag
        if (p.id in _results.value) return false        // already resolved
        if (p.id in inFlight) return false
        return true
    }

    /** Pull the server host (ip/domain) out of a profile, or null if none. */
    private fun hostFor(p: Profile): String? {
        val entry = p.cachedEntry() ?: uriEntry(p) ?: return null
        if (entry.optString("type").equals("AUTO", ignoreCase = true)) return null
        return entry.optString("ip").takeIf { it.isNotBlank() }
    }

    private fun uriEntry(p: Profile): JSONObject? {
        if (p.uri.isBlank()) return null
        uriCache[p.uri]?.let { return it }
        return runCatching {
            JSONObject(Mobile.parseProxyURI(p.uri)).also { uriCache[p.uri] = it }
        }.getOrNull()
    }
}
