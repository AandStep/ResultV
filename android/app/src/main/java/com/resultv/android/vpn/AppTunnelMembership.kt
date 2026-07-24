package com.resultv.android.vpn

/**
 * Pure composition of the Smart-mode tunnel allowlist and the per-app
 * include/exclude transitions the UI drives. No Context, no I/O.
 *
 * Membership layer only: which apps enter the TUN. Routing (which of their
 * traffic hits the proxy) is the engine's job, unchanged.
 */
object AppTunnelMembership {

    /** Packages to hand VpnService.Builder.addAllowedApplication in Smart. */
    fun smartAllowlist(
        matched: Set<String>,
        browsers: Set<String>,
        intoVpn: Set<String>,
        outOfVpn: Set<String>,
        ownPackage: String,
    ): Set<String> {
        val allow = HashSet<String>(matched.size + browsers.size + intoVpn.size)
        allow.addAll(matched)
        allow.addAll(browsers)
        allow.addAll(intoVpn)
        allow.removeAll(outOfVpn)
        allow.remove(ownPackage)
        return allow
    }

    fun isAuto(pkg: String, matched: Set<String>, browsers: Set<String>): Boolean =
        pkg in matched || pkg in browsers

    /** Effective "rides the VPN in Smart" state for a single app. */
    fun isInSmart(
        pkg: String,
        matched: Set<String>,
        browsers: Set<String>,
        rules: AppRulesState,
    ): Boolean {
        if (pkg in rules.outOfVpn) return false
        return isAuto(pkg, matched, browsers) || pkg in rules.intoVpn
    }

    /**
     * Toggle one app's Smart membership. Reuses the AppRulesState transitions
     * so the block/route invariants hold:
     * - wantIn : clear any exclusion; if not auto, record a manual include.
     * - wantOut: clear any manual include; if auto, record a manual exclusion.
     */
    fun setSmartMembership(
        rules: AppRulesState,
        pkg: String,
        wantIn: Boolean,
        isAuto: Boolean,
    ): AppRulesState = if (wantIn) {
        val cleared = rules.withoutAction(pkg, RuleAction.OutOfVpn)
        if (isAuto) cleared else cleared.withAction(pkg, RuleAction.IntoVpn)
    } else {
        val cleared = rules.withoutAction(pkg, RuleAction.IntoVpn)
        if (isAuto) cleared.withAction(pkg, RuleAction.OutOfVpn) else cleared
    }
}
