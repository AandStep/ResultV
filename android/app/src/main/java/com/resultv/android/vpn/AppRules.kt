package com.resultv.android.vpn

import org.json.JSONArray
import org.json.JSONObject

/**
 * The three per-app rule lists. Pure data + pure transitions: no Context, no
 * file I/O, no AppLog — that lives in AppRoutingRepository. Keeping this half
 * Android-free is what makes the invariant and the migration testable on the
 * JVM (the unit-test source set has no Robolectric).
 *
 * The invariant is asymmetric, and the asymmetry is load-bearing:
 * - [blocked] intersects neither [outOfVpn] nor [intoVpn] — blocking applies in
 *   both routing modes, so an app cannot be blocked AND routed at once.
 * - [outOfVpn] and [intoVpn] MAY intersect — Global's "out of VPN" and Smart's
 *   "into VPN" are never active simultaneously, so "bank bypasses VPN in
 *   Global" and "bank goes through VPN in Smart" are two independent, sensible
 *   facts. Mirrors the desktop, where `whitelist` and `customBlockedDomains`
 *   are independent config fields for exactly this reason.
 *
 * [migratedFromAllowList] is transient — set by [decodeAppRules] when the
 * legacy `AllowList` mode was converted, so the repository can log it once. It
 * is deliberately NOT persisted by [encodeAppRules].
 */
data class AppRulesState(
    val outOfVpn: Set<String> = emptySet(),
    val intoVpn: Set<String> = emptySet(),
    val blocked: Set<String> = emptySet(),
    val migratedFromAllowList: Boolean = false,
) {
    /** The action in effect for [pkg] in [mode], or null if none. */
    fun actionOf(pkg: String, mode: RoutingMode): RuleAction? = when {
        pkg in blocked -> RuleAction.Block
        mode == RoutingMode.Global && pkg in outOfVpn -> RuleAction.OutOfVpn
        mode == RoutingMode.Smart && pkg in intoVpn -> RuleAction.IntoVpn
        else -> null
    }

    fun withAction(pkg: String, action: RuleAction): AppRulesState = when (action) {
        // Block is the only action that evicts from BOTH routing lists.
        RuleAction.Block -> copy(
            outOfVpn = outOfVpn - pkg,
            intoVpn = intoVpn - pkg,
            blocked = blocked + pkg,
        )
        // The routing actions clear `blocked` only — touching the other
        // routing list would silently wipe the other mode's setting.
        RuleAction.OutOfVpn -> copy(blocked = blocked - pkg, outOfVpn = outOfVpn + pkg)
        RuleAction.IntoVpn -> copy(blocked = blocked - pkg, intoVpn = intoVpn + pkg)
    }

    /** Unchecking a row clears only the list of the tab it was unchecked in. */
    fun withoutAction(pkg: String, action: RuleAction): AppRulesState = when (action) {
        RuleAction.OutOfVpn -> copy(outOfVpn = outOfVpn - pkg)
        RuleAction.IntoVpn -> copy(intoVpn = intoVpn - pkg)
        RuleAction.Block -> copy(blocked = blocked - pkg)
    }

    fun without(pkg: String): AppRulesState =
        copy(outOfVpn = outOfVpn - pkg, intoVpn = intoVpn - pkg, blocked = blocked - pkg)
}

/**
 * Reads both the current format and the legacy `{mode, packages}` one. Absence
 * of the new keys IS the legacy marker — no version flag needed.
 *
 * Legacy mapping:
 * - `All`          → empty (behaviour unchanged).
 * - `DisallowList` → outOfVpn (behaviour unchanged: same addDisallowedApplication).
 * - `AllowList`    → intoVpn + [AppRulesState.migratedFromAllowList] (NOT an
 *   equivalent — see the AllowList note in the spec).
 *
 * Throws on malformed JSON; the repository catches and falls back to empty.
 */
fun decodeAppRules(json: String): AppRulesState {
    val root = JSONObject(json)
    if (root.has(KEY_OUT_OF_VPN) || root.has(KEY_INTO_VPN) || root.has(KEY_BLOCKED)) {
        return AppRulesState(
            outOfVpn = root.stringSet(KEY_OUT_OF_VPN),
            intoVpn = root.stringSet(KEY_INTO_VPN),
            blocked = root.stringSet(KEY_BLOCKED),
        )
    }
    val packages = root.stringSet("packages")
    return when (root.optString("mode")) {
        "DisallowList" -> AppRulesState(outOfVpn = packages)
        "AllowList" -> AppRulesState(intoVpn = packages, migratedFromAllowList = true)
        else -> AppRulesState()
    }
}

fun encodeAppRules(state: AppRulesState): String = JSONObject()
    .put(KEY_OUT_OF_VPN, JSONArray(state.outOfVpn.sorted()))
    .put(KEY_INTO_VPN, JSONArray(state.intoVpn.sorted()))
    .put(KEY_BLOCKED, JSONArray(state.blocked.sorted()))
    .toString()

private const val KEY_OUT_OF_VPN = "outOfVpnApps"
private const val KEY_INTO_VPN = "intoVpnApps"
private const val KEY_BLOCKED = "blockedApps"

internal fun JSONObject.stringSet(key: String): Set<String> {
    val arr = optJSONArray(key) ?: return emptySet()
    return (0 until arr.length()).mapNotNull { arr.optString(it).takeIf { s -> s.isNotEmpty() } }.toSet()
}
