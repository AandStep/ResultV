package com.resultv.android.vpn

import android.util.Log
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.cancel
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import mobile.Mobile
import org.json.JSONObject

private const val TAG = "ResultV/Ping"
private const val POLL_INTERVAL_MS = 60_000L
private const val RESULT_TTL_MS = 90_000L

/**
 * Per-profile latency cache backed by `Mobile.ping`. For non-AUTO profiles
 * we probe a single (ip, port, type); for AUTO profiles we probe every
 * member and surface the best reachable one for sorting / display while
 * keeping the full list available to the connect path for failover.
 *
 * The repo holds the most-recent reading per profile in a StateFlow so
 * Compose recomposes whenever a probe completes. A periodic ticker
 * re-probes every 60s while the app process is up; UI surfaces a manual
 * refresh too.
 */
object PingRepository {

    data class Sample(
        val latencyMs: Int,
        val reachable: Boolean,
        val timestamp: Long = System.currentTimeMillis(),
    )

    /** Single proxy endpoint that can be probed individually. */
    data class Target(
        val ip: String,
        val port: Int,
        val type: String,
        /**
         * Original member JSON for AUTO entries — the connect path needs
         * the full ProxyEntry, not just (ip, port, type), to swap members
         * after a connect failure.
         */
        val entryJson: String = "",
    ) {
        /** Stable key used inside the results map for per-target storage. */
        val key: String get() = "$type|$ip|$port"
    }

    private val _results = MutableStateFlow<Map<String, Sample>>(emptyMap())
    /**
     * Map keyed by profile.id for the *best* sample of that profile
     * (single target → that sample; AUTO → min-latency reachable member).
     * UI binds here.
     */
    val results: StateFlow<Map<String, Sample>> = _results.asStateFlow()

    // Per-target raw samples, keyed by Target.key. AUTO connect reads
    // these to order members.
    private val _targetResults = MutableStateFlow<Map<String, Sample>>(emptyMap())
    val targetResults: StateFlow<Map<String, Sample>> = _targetResults.asStateFlow()

    /**
     * Profile IDs currently being probed via a user-triggered refresh. UI
     * binds here to keep showing the loading spinner during a manual
     * re-ping of already-pinged servers. Background poll ticks do NOT
     * mark inflight — the spinner would flash on every row every 60s.
     */
    private val _inflight = MutableStateFlow<Set<String>>(emptySet())
    val inflight: StateFlow<Set<String>> = _inflight.asStateFlow()

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private var pollerJob: Job? = null

    /** Resolve the probe targets for [profile]. Non-AUTO → 1 element; AUTO → N. */
    fun targetsFor(profile: Profile): List<Target> {
        if (profile.isSection) return emptyList()
        val entry = parseEntry(profile) ?: return emptyList()
        val type = entry.optString("type").uppercase()
        if (type == "AUTO") {
            val extra = entry.optJSONObject("extra") ?: return emptyList()
            val members = extra.optJSONArray("members") ?: return emptyList()
            val out = ArrayList<Target>(members.length())
            for (i in 0 until members.length()) {
                val m = members.optJSONObject(i) ?: continue
                val ip = m.optString("ip")
                val port = m.optInt("port")
                val mt = m.optString("type").uppercase()
                if (ip.isBlank() || port <= 0) continue
                out += Target(ip = ip, port = port, type = mt, entryJson = m.toString())
            }
            return out
        }
        val ip = entry.optString("ip")
        val port = entry.optInt("port")
        if (ip.isBlank() || port <= 0) return emptyList()
        return listOf(Target(ip = ip, port = port, type = type, entryJson = entry.toString()))
    }

    /** Re-probe [profile] and update the cache. Safe to call from any thread. */
    fun refresh(profile: Profile) {
        if (profile.isSection) return
        markInflight(listOf(profile.id))
        scope.launch {
            try { refreshSuspending(profile) }
            finally { clearInflight(listOf(profile.id)) }
        }
    }

    /**
     * Re-probe every passed profile in parallel (capped concurrency). Each
     * profile publishes its sample to `_results` / `_targetResults` as soon
     * as it finishes so the UI fills in progressively rather than all-at-once
     * at the end of the sweep. Marks every profile as inflight up-front so
     * already-pinged rows show the loading spinner during the refresh.
     */
    fun refreshAll(profiles: List<Profile>) {
        val real = profiles.filterNot { it.isSection }
        if (real.isEmpty()) return
        markInflight(real.map { it.id })
        scope.launch {
            real.chunked(24).forEach { chunk ->
                val jobs = chunk.map { p ->
                    launch {
                        try { refreshSuspending(p) }
                        finally { clearInflight(listOf(p.id)) }
                    }
                }
                jobs.forEach { it.join() }
            }
        }
    }

    @Synchronized
    private fun markInflight(ids: Collection<String>) {
        if (ids.isEmpty()) return
        _inflight.value = _inflight.value + ids
    }

    @Synchronized
    private fun clearInflight(ids: Collection<String>) {
        if (ids.isEmpty()) return
        _inflight.value = _inflight.value - ids.toSet()
    }

    /**
     * Probe every member of [profile] (AUTO or single) and return the
     * targets sorted by reachable-then-latency ascending. Used by the
     * connect path for AUTO failover.
     */
    suspend fun probeAndSort(profile: Profile): List<Target> {
        val targets = targetsFor(profile)
        if (targets.isEmpty()) return emptyList()
        val samples = HashMap<String, Sample>()
        for (chunk in targets.chunked(24)) {
            val jobs = chunk.map { t ->
                scope.launch {
                    val s = probeOne(t)
                    synchronized(samples) { samples[t.key] = s }
                }
            }
            jobs.forEach { it.join() }
        }
        publishTargetSamples(samples)
        publishProfileBest(profile.id, targets, samples)
        return targets.sortedWith(
            compareByDescending<Target> { samples[it.key]?.reachable == true }
                .thenBy { samples[it.key]?.latencyMs ?: Int.MAX_VALUE },
        )
    }

    /** Start the 60s background ticker; idempotent. */
    @Synchronized
    fun startPolling(profiles: () -> List<Profile>) {
        if (pollerJob?.isActive == true) return
        pollerJob = scope.launch {
            while (isActive) {
                try {
                    // Background sweep — don't mark inflight; the UI spinner
                    // is for user-triggered refreshes only.
                    val real = profiles().filterNot { it.isSection }
                    real.chunked(24).forEach { chunk ->
                        val jobs = chunk.map { p -> launch { refreshSuspending(p) } }
                        jobs.forEach { it.join() }
                    }
                } catch (t: Throwable) {
                    Log.w(TAG, "poll tick failed", t)
                }
                delay(POLL_INTERVAL_MS)
            }
        }
    }

    @Synchronized
    fun stopPolling() {
        pollerJob?.cancel()
        pollerJob = null
    }

    // ──────────────────────── Internals ────────────────────────

    private suspend fun refreshSuspending(profile: Profile) = coroutineScope {
        val targets = targetsFor(profile)
        if (targets.isEmpty()) return@coroutineScope
        
        val deferreds = targets.map { t ->
            async { t to probeOne(t) }
        }
        val results = deferreds.awaitAll()
        val samples = results.associate { it.first.key to it.second }
        
        publishTargetSamples(samples)
        publishProfileBest(profile.id, targets, samples)
    }

    private suspend fun probeOne(t: Target): Sample = withContext(Dispatchers.IO) {
        // 5s freshness cache absorbs duplicate refreshes — the sort menu
        // recompose, manual refresh, and poll tick can all collide.
        val cached = _targetResults.value[t.key]
        if (cached != null && System.currentTimeMillis() - cached.timestamp < 5_000L) {
            return@withContext cached
        }
        val raw = try {
            Mobile.ping(t.ip, t.port.toLong(), t.type)
        } catch (e: Throwable) {
            Log.w(TAG, "ping ${t.ip}:${t.port} (${t.type}) failed: ${e.message}")
            return@withContext Sample(latencyMs = 0, reachable = false)
        }
        val o = runCatching { JSONObject(raw) }.getOrNull()
        Sample(
            latencyMs = o?.optLong("latencyMs", 0L)?.toInt() ?: 0,
            reachable = o?.optBoolean("reachable", false) ?: false,
        )
    }

    private fun publishTargetSamples(fresh: Map<String, Sample>) {
        if (fresh.isEmpty()) return
        _targetResults.value = (_targetResults.value + fresh).pruned()
    }

    private fun publishProfileBest(
        profileId: String,
        targets: List<Target>,
        samples: Map<String, Sample>,
    ) {
        val best = targets
            .mapNotNull { samples[it.key] }
            .filter { it.reachable }
            .minByOrNull { it.latencyMs }
            ?: targets.firstNotNullOfOrNull { samples[it.key] }
            ?: return
        _results.value = (_results.value + (profileId to best))
    }

    private fun Map<String, Sample>.pruned(): Map<String, Sample> {
        val now = System.currentTimeMillis()
        // Drop entries older than TTL so a removed profile doesn't keep
        // stale data forever. Cheap enough to walk every publish.
        return filter { now - it.value.timestamp < RESULT_TTL_MS * 4 }
    }

    /** Parse the profile's ProxyEntry JSON, falling back to URI parsing. */
    private fun parseEntry(p: Profile): JSONObject? {
        // entryJson-based profiles use the parse cached on Profile itself —
        // saves an O(N) parse per refresh cycle for subscription profiles.
        val cached = p.cachedEntry()
        if (cached != null) return cached
        if (p.uri.isBlank()) return null
        val uriCached = uriCache[p.uri]
        if (uriCached != null) return uriCached
        return runCatching {
            JSONObject(Mobile.parseProxyURI(p.uri)).also { uriCache[p.uri] = it }
        }.getOrNull()
    }

    private val uriCache = HashMap<String, JSONObject>()
}
