package com.resultv.android.vpn

import android.util.Log
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.joinAll
import kotlinx.coroutines.launch
import mobile.Mobile
import org.json.JSONObject

private const val TAG = "ResultV/Country"
private const val MAX_ATTEMPTS = 3
private val RETRYABLE_STATUS = Regex("country api http (408|429|5[0-9]{2})")

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
 * desktop. Lookups are shared by host and projected onto the current profile
 * IDs so ServerRow recomposes its flag as results land.
 */
object CountryRepository {
    private val lookups = CountryLookups(
        scope = CoroutineScope(SupervisorJob() + Dispatchers.IO),
        detectCountry = { host, dataDir -> Mobile.detectCountry(host, dataDir) },
        onFailure = { host, error -> Log.d(TAG, "detectCountry($host) failed: ${error.message}") },
    )

    /** profile.id → lowercase 2-letter country code (only successful lookups). */
    val results: StateFlow<Map<String, String>> = lookups.results

    fun resolve(profiles: List<Profile>, dataDir: String) = lookups.resolve(profiles, dataDir)
}

internal class CountryLookups(
    private val scope: CoroutineScope,
    private val detectCountry: suspend (host: String, dataDir: String) -> String,
    private val onFailure: (host: String, error: Exception) -> Unit = { _, _ -> },
) {
    private val _results = MutableStateFlow<Map<String, String>>(emptyMap())
    val results: StateFlow<Map<String, String>> = _results.asStateFlow()

    private var hostsByProfile = emptyMap<String, String>()
    private val countriesByHost = HashMap<String, String>()
    private val inFlight = HashSet<String>()

    // Parsed-URI cache so we don't re-cross the JNI boundary to pull the host
    // out of a URI-only profile on every resolve pass.
    private val uriCache = HashMap<String, JSONObject>()

    /**
     * Resolve country codes for every passed profile that still needs one.
     * The caller supplies the current profile list. A completed lookup applies
     * only to profiles still pointing to that host.
     */
    fun resolve(profiles: List<Profile>, dataDir: String) {
        val pending = synchronized(this) {
            hostsByProfile = profiles.mapNotNull { p ->
                if (p.isSection || p.isAuto || !p.country.isNullOrBlank()) return@mapNotNull null
                hostFor(p)?.let { p.id to it }
            }.toMap()
            publishResults()
            hostsByProfile.values.filter { it !in countriesByHost && inFlight.add(it) }
        }
        if (pending.isEmpty()) return
        scope.launch {
            pending.chunked(8).forEach { chunk ->
                chunk.map { host ->
                    launch {
                        var country: String? = null
                        try {
                            country = lookup(host, dataDir)
                        } finally {
                            finish(host, country)
                        }
                    }
                }.joinAll()
            }
        }
    }

    private suspend fun lookup(host: String, dataDir: String): String? {
        repeat(MAX_ATTEMPTS) { attempt ->
            try {
                return detectCountry(host, dataDir).takeIf { it.length == 2 }?.lowercase()
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                if (attempt == MAX_ATTEMPTS - 1 || !isRetryable(error)) {
                    onFailure(host, error)
                    return null
                }
                delay(400L * (attempt + 1))
            }
        }
        return null
    }

    @Synchronized
    private fun finish(host: String, country: String?) {
        inFlight -= host
        if (country != null) {
            countriesByHost[host] = country
            publishResults()
        }
    }

    /** Caller holds the monitor. */
    private fun publishResults() {
        _results.value = buildMap {
            for ((id, host) in hostsByProfile) {
                countriesByHost[host]?.let { put(id, it) }
            }
        }
    }

    /** Pull the server host (ip/domain) out of a profile, or null if none. */
    private fun hostFor(p: Profile): String? {
        val entry = p.cachedEntry() ?: uriEntry(p) ?: return null
        if (entry.optString("type").equals("AUTO", ignoreCase = true)) return null
        return entry.optString("ip").trim().removeSurrounding("[", "]").lowercase().takeIf { it.isNotBlank() }
    }

    private fun uriEntry(p: Profile): JSONObject? {
        if (p.uri.isBlank()) return null
        uriCache[p.uri]?.let { return it }
        return runCatching {
            JSONObject(Mobile.parseProxyURI(p.uri)).also { uriCache[p.uri] = it }
        }.getOrNull()
    }
}

/** Gomobile passes Go's error message across JNI without its concrete type. */
private fun isRetryable(error: Exception): Boolean {
    val message = error.message.orEmpty().lowercase()
    return RETRYABLE_STATUS.matches(message) ||
        message.contains("timeout") ||
        message.contains("deadline exceeded") ||
        message.contains("temporary failure") ||
        message.contains("connection reset") ||
        message.contains("connection refused") ||
        message.contains("network is unreachable") ||
        message.contains("no route to host") ||
        message.endsWith("eof")
}
