package com.resultv.android.vpn

import org.json.JSONArray
import org.json.JSONObject

/**
 * The three domain rule lists. Same asymmetric invariant as [AppRulesState] —
 * see its docs. Lists (not sets): the chip order is user-visible, so insertion
 * order is part of the contract.
 */
data class DomainRulesState(
    val outOfVpn: List<String> = emptyList(),
    val intoVpn: List<String> = emptyList(),
    val blocked: List<String> = emptyList(),
) {
    fun listFor(action: RuleAction): List<String> = when (action) {
        RuleAction.OutOfVpn -> outOfVpn
        RuleAction.IntoVpn -> intoVpn
        RuleAction.Block -> blocked
    }

    /**
     * The other tab's action currently holding [domain], or null. Drives the
     * inline "moved from <tab>" hint — domains have no shared catalogue to
     * badge, so the cross-list invariant surfaces on input instead.
     */
    fun otherListHolding(domain: String, action: RuleAction): RuleAction? {
        val d = domain.trim().lowercase()
        return RuleAction.entries.firstOrNull { it != action && d in listFor(it) }
    }

    fun withAction(domain: String, action: RuleAction): DomainRulesState {
        val d = domain.trim().lowercase()
        if (d.isEmpty()) return this
        val s = when (action) {
            RuleAction.Block -> copy(outOfVpn = outOfVpn - d, intoVpn = intoVpn - d)
            RuleAction.OutOfVpn, RuleAction.IntoVpn -> copy(blocked = blocked - d)
        }
        val target = s.listFor(action)
        val added = if (d in target) target else target + d
        return s.replacing(action, added)
    }

    fun withoutAction(domain: String, action: RuleAction): DomainRulesState =
        replacing(action, listFor(action) - domain)

    private fun replacing(action: RuleAction, list: List<String>): DomainRulesState = when (action) {
        RuleAction.OutOfVpn -> copy(outOfVpn = list)
        RuleAction.IntoVpn -> copy(intoVpn = list)
        RuleAction.Block -> copy(blocked = list)
    }
}

/**
 * Reads the current format and the legacy `{mode, domainExclusions}` one.
 * `mode` is NOT read here — it stays owned by RoutingRulesRepository. Throws
 * on malformed JSON.
 *
 * The dead `domainHistory` key that older installs still carry is simply not
 * read; the next save drops it.
 */
fun decodeDomainRules(json: String): DomainRulesState {
    val root = JSONObject(json)
    if (root.has(KEY_OUT_OF_VPN) || root.has(KEY_INTO_VPN) || root.has(KEY_BLOCKED)) {
        return DomainRulesState(
            outOfVpn = root.stringList(KEY_OUT_OF_VPN),
            intoVpn = root.stringList(KEY_INTO_VPN),
            blocked = root.stringList(KEY_BLOCKED),
        )
    }
    return DomainRulesState(outOfVpn = root.stringList("domainExclusions"))
}

fun encodeDomainRules(state: DomainRulesState): String = JSONObject()
    .put(KEY_OUT_OF_VPN, JSONArray(state.outOfVpn))
    .put(KEY_INTO_VPN, JSONArray(state.intoVpn))
    .put(KEY_BLOCKED, JSONArray(state.blocked))
    .toString()

private const val KEY_OUT_OF_VPN = "outOfVpnDomains"
private const val KEY_INTO_VPN = "intoVpnDomains"
private const val KEY_BLOCKED = "blockedDomains"

private fun JSONObject.stringList(key: String): List<String> {
    val arr = optJSONArray(key) ?: return emptyList()
    return (0 until arr.length()).mapNotNull { arr.optString(it).takeIf { s -> s.isNotEmpty() } }
}
