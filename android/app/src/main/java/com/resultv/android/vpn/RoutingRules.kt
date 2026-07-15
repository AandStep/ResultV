package com.resultv.android.vpn

import android.content.Context
import android.util.Log
import com.resultv.android.R
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
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

/**
 * Top-level routing state. The domain lists live in [DomainRulesState]; `mode`
 * selects which of them is active (Global → outOfVpn, Smart → intoVpn), while
 * `blocked` applies in both.
 */
data class RoutingRulesState(
    val mode: RoutingMode = RoutingMode.Global,
    val domains: DomainRulesState = DomainRulesState(
        // Same defaults as before the three-list split — fresh installs only.
        outOfVpn = listOf("localhost", "127.0.0.1", "*.ru", "*.рф"),
    ),
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
    fun addDomain(domain: String, action: RuleAction) =
        mutate { it.copy(domains = it.domains.withAction(domain, action)) }

    @Synchronized
    fun removeDomain(domain: String, action: RuleAction) =
        mutate { it.copy(domains = it.domains.withoutAction(domain, action)) }

    @Synchronized
    fun forgetDomainHistory(domain: String) = mutate {
        it.copy(domains = it.domains.copy(history = it.domains.history.filterNot { d -> d == domain }))
    }

    /** Domains sent to the engine as `domain_suffix` → direct (Global only). */
    fun engineOutOfVpn(): List<String> = _state.value.domains.outOfVpn

    /** Domains sent as `domain_suffix` → proxy (Smart only). */
    fun engineIntoVpn(): List<String> = _state.value.domains.intoVpn

    /** Domains sent as `domain_suffix` → reject (both modes). */
    fun engineBlocked(): List<String> = _state.value.domains.blocked

    private fun mutate(block: (RoutingRulesState) -> RoutingRulesState) {
        val next = block(_state.value)
        if (next == _state.value) return
        _state.value = next
        file?.let { save(it, next) }
    }

    private fun load(f: File): RoutingRulesState {
        if (!f.exists()) return RoutingRulesState()
        return try {
            val mode = RoutingMode.entries.firstOrNull { it.name == JSONObject(f.readText()).optString("mode") }
                ?: RoutingMode.Global
            RoutingRulesState(mode = mode, domains = decodeDomainRules(f.readText()))
        } catch (t: Throwable) {
            Log.w(TAG, "failed to read $f, starting empty", t)
            AppLog.warning(R.string.log_read_failed, f.name,
                source = AppLog.resolve(R.string.log_source_config))
            // Empty, NOT the fresh-install defaults: a corrupt file must not
            // silently re-add exclusions the user may have removed.
            RoutingRulesState(domains = DomainRulesState())
        }
    }

    private fun save(f: File, s: RoutingRulesState) {
        try {
            val root = JSONObject(encodeDomainRules(s.domains)).put("mode", s.mode.name)
            f.writeText(root.toString())
        } catch (t: Throwable) {
            Log.e(TAG, "failed to persist routing rules", t)
            AppLog.error(R.string.log_persist_failed, f.name,
                source = AppLog.resolve(R.string.log_source_config))
        }
    }
}
