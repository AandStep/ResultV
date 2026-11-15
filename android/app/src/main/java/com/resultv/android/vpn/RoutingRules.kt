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
 *   else stays direct, driven by the Antizapret-style rule-set compiled into
 *   `smart/smart.srs` (see SmartListRepository).
 *
 * Smart is the default — same as the desktop's `DefaultConfig()`.
 */
enum class RoutingMode { Global, Smart }

/**
 * Read the routing mode out of a persisted `routing_rules.json`.
 *
 * A file that names no mode we recognise migrates to [RoutingMode.Smart],
 * mirroring the desktop's `ensureDefaults`: an absent mode means "the user
 * never chose", not "the user chose Global". An explicitly stored mode is the
 * user's decision and is returned untouched — which is why changing the
 * default moves nobody who already has the app.
 */
fun decodeRoutingMode(json: String): RoutingMode {
    val name = runCatching { JSONObject(json).optString("mode") }.getOrNull()
    return RoutingMode.entries.firstOrNull { it.name == name } ?: RoutingMode.Smart
}

data class RoutingRulesState(
    val mode: RoutingMode = RoutingMode.Smart,
    val smartDomains: DomainRulesState = DomainRulesState(),
    val globalDomains: DomainRulesState = DomainRulesState(outOfVpn = listOf("localhost", "127.0.0.1")),
) {
    val domains: DomainRulesState get() = if (mode == RoutingMode.Smart) smartDomains else globalDomains

    fun editDomains(edit: (DomainRulesState) -> DomainRulesState): RoutingRulesState =
        if (mode == RoutingMode.Smart) copy(smartDomains = edit(smartDomains))
        else copy(globalDomains = edit(globalDomains))
}

internal fun decodeModeRoutingRules(text: String): RoutingRulesState {
    val root = JSONObject(text)
    val mode = decodeRoutingMode(text)
    if (root.has("smart") || root.has("global")) {
        return RoutingRulesState(
            mode,
            decodeDomainRules(root.getJSONObject("smart").toString()),
            decodeDomainRules(root.getJSONObject("global").toString()),
        )
    }

    val legacy = decodeDomainRules(text)
    val normalized = legacy.outOfVpn.map { it.trim().lowercase().trim('*', '.') }.filter { it.isNotEmpty() }.distinct()
    val (direct, vpn) = legacy.outOfVpn.partition { entry ->
        val name = entry.trim().lowercase().trim('*', '.')
        // Legacy nested exclusions alternated between direct and VPN.
        normalized.count { name == it || name.endsWith(".$it") } % 2 == 1
    }
    return RoutingRulesState(
        mode,
        legacy.copy(outOfVpn = emptyList()),
        legacy.copy(outOfVpn = direct, intoVpn = vpn),
    )
}

internal fun encodeModeRoutingRules(state: RoutingRulesState): String = JSONObject()
    .put("mode", state.mode.name)
    .put("smart", JSONObject(encodeDomainRules(state.smartDomains)))
    .put("global", JSONObject(encodeDomainRules(state.globalDomains)))
    .toString()

private fun DomainRulesState.selectDomain(domain: String, action: RuleAction): DomainRulesState {
    val normalized = domain.trim().lowercase()
    return when (action) {
        RuleAction.IntoVpn -> withoutAction(normalized, RuleAction.OutOfVpn)
        RuleAction.OutOfVpn -> withoutAction(normalized, RuleAction.IntoVpn)
        RuleAction.Block -> this
    }.withAction(normalized, action)
}

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
        mutate { it.editDomains { rules -> rules.selectDomain(domain, action) } }

    @Synchronized
    fun removeDomain(domain: String, action: RuleAction) =
        mutate { it.editDomains { rules -> rules.withoutAction(domain, action) } }

    /** Очищает один список целиком — корзина в шапке секции. */
    @Synchronized
    fun clearDomains(action: RuleAction) = mutate { state ->
        state.editDomains { rules ->
            rules.listFor(action).fold(rules) { list, domain -> list.withoutAction(domain, action) }
        }
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
            decodeModeRoutingRules(f.readText())
        } catch (t: Throwable) {
            Log.w(TAG, "failed to read $f, starting empty", t)
            AppLog.warning(R.string.log_read_failed, f.name,
                source = AppLog.resolve(R.string.log_source_config))
            // Empty lists, NOT the fresh-install defaults: a corrupt file must
            // not silently re-add exclusions the user may have removed. The
            // mode does fall back to the default (Smart), same as the desktop
            // dropping to DefaultConfig() when the config fails to decrypt —
            // an unreadable file tells us nothing about what was chosen.
            RoutingRulesState(globalDomains = DomainRulesState())
        }
    }

    private fun save(f: File, s: RoutingRulesState) {
        try {
            replaceRepositoryJson(f, encodeModeRoutingRules(s), ::decodeModeRoutingRules)
        } catch (t: Throwable) {
            Log.e(TAG, "failed to persist routing rules", t)
            AppLog.error(R.string.log_persist_failed, f.name,
                source = AppLog.resolve(R.string.log_source_config))
        }
    }
}
