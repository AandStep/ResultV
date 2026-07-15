package com.resultv.android.vpn

import android.content.Context
import android.util.Log
import com.resultv.android.R
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import org.json.JSONArray
import org.json.JSONObject
import java.io.File

private const val TAG = "ResultV/Rules"
private const val FILE_NAME = "routing_rules.json"

/**
 * Top-level routing strategy.
 *
 * - [Global]: every packet from the device is sent through the proxy.
 * - [Smart]: only known-blocked resources go through the proxy; everything
 *   else stays direct. Backed by an Antizapret-style domain ruleset on
 *   desktop. The mobile build accepts the toggle but currently behaves the
 *   same as [Global] until the geosite ruleset is wired in.
 */
enum class RoutingMode { Global, Smart }

data class RoutingRulesState(
    val mode: RoutingMode = RoutingMode.Global,
    /** Domains that always bypass the proxy (resolved direct). */
    val domainExclusions: List<String> = listOf(
        "localhost", "127.0.0.1", "*.ru", "*.рф",
    ),
    /**
     * MRU list of domains the user has typed at least once — drives the
     * Rules screen's "Recently used" suggestion chips so the next paste of
     * a familiar pattern is a single tap. Bounded to [DOMAIN_HISTORY_MAX]
     * entries; oldest fall off when the cap is hit.
     */
    val domainHistory: List<String> = emptyList(),
)

/**
 * True when [pattern] is a strict superset of [candidate] — i.e. anything
 * matched by `candidate` is also matched by `pattern`, but the two are not
 * identical. Used by the Rules screen to warn about redundant /
 * shadow-bound exclusions.
 *
 * Recognised forms:
 * - Literal hostnames (`yandex.ru`) — only equal hostnames shadow.
 * - Suffix patterns (`*.ru`) — shadow every hostname ending with the same
 *   suffix (and the bare suffix `ru` itself).
 *
 * The check is case-insensitive and trims whitespace.
 */
fun domainPatternShadows(pattern: String, candidate: String): Boolean {
    val p = pattern.trim().lowercase()
    val c = candidate.trim().lowercase()
    if (p.isEmpty() || c.isEmpty() || p == c) return false
    if (p.startsWith("*.")) {
        val dotSuffix = p.removePrefix("*")        // ".ru"
        val bare = dotSuffix.removePrefix(".")      // "ru"
        // Candidate is shadowed if it's a sub-host of `p` (e.g. yandex.ru,
        // foo.bar.ru) or the bare apex itself ("ru" under "*.ru" reads as
        // "the .ru zone").
        return c.endsWith(dotSuffix) || c == bare
    }
    return false
}

object RoutingRulesRepository {
    private val _state = MutableStateFlow(RoutingRulesState())
    val state: StateFlow<RoutingRulesState> = _state.asStateFlow()

    @Volatile private var file: File? = null

    @Synchronized
    fun init(ctx: Context) {
        if (file != null) return
        val f = File(ctx.filesDir, FILE_NAME)
        file = f
        _state.value = load(f)
    }

    @Synchronized
    fun setMode(mode: RoutingMode) = mutate { it.copy(mode = mode) }

    @Synchronized
    fun addDomain(domain: String) = mutate {
        val trimmed = domain.trim()
        if (trimmed.isEmpty()) return@mutate it
        val nextHistory = (listOf(trimmed) + it.domainHistory.filterNot { d -> d == trimmed })
            .take(DOMAIN_HISTORY_MAX)
        if (trimmed in it.domainExclusions) it.copy(domainHistory = nextHistory)
        else it.copy(
            domainExclusions = it.domainExclusions + trimmed,
            domainHistory = nextHistory,
        )
    }

    @Synchronized
    fun removeDomain(domain: String) = mutate {
        it.copy(domainExclusions = it.domainExclusions.filterNot { d -> d == domain })
    }

    /**
     * Drop one entry from the recently-used history list (e.g. user
     * dismissed a suggestion they don't want re-suggested).
     */
    @Synchronized
    fun forgetDomainHistory(domain: String) = mutate {
        it.copy(domainHistory = it.domainHistory.filterNot { d -> d == domain })
    }

    private fun mutate(block: (RoutingRulesState) -> RoutingRulesState) {
        val next = block(_state.value)
        if (next == _state.value) return
        _state.value = next
        file?.let { save(it, next) }
    }

    private fun load(f: File): RoutingRulesState {
        if (!f.exists()) return RoutingRulesState()
        return try {
            val root = JSONObject(f.readText())
            val mode = RoutingMode.entries.firstOrNull { it.name == root.optString("mode") }
                ?: RoutingMode.Global
            val arr = root.optJSONArray("domainExclusions") ?: JSONArray()
            val domains = (0 until arr.length()).map { arr.getString(it) }
            val historyArr = root.optJSONArray("domainHistory") ?: JSONArray()
            val history = (0 until historyArr.length()).map { historyArr.getString(it) }
            RoutingRulesState(mode = mode, domainExclusions = domains, domainHistory = history)
        } catch (t: Throwable) {
            Log.w(TAG, "failed to read $f, starting empty", t)
            AppLog.warning(R.string.log_read_failed, f.name,
                source = AppLog.resolve(R.string.log_source_config))
            RoutingRulesState()
        }
    }

    private fun save(f: File, s: RoutingRulesState) {
        try {
            val arr = JSONArray()
            s.domainExclusions.forEach { arr.put(it) }
            val historyArr = JSONArray()
            s.domainHistory.forEach { historyArr.put(it) }
            val root = JSONObject()
                .put("mode", s.mode.name)
                .put("domainExclusions", arr)
                .put("domainHistory", historyArr)
            f.writeText(root.toString())
        } catch (t: Throwable) {
            Log.e(TAG, "failed to persist routing rules", t)
            AppLog.error(R.string.log_persist_failed, f.name,
                source = AppLog.resolve(R.string.log_source_config))
        }
    }
}
