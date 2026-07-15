package com.resultv.android.vpn

/**
 * What a rule does with a package's or a domain's traffic.
 *
 * [OutOfVpn] is Global-mode only, [IntoVpn] is Smart-mode only, [Block] applies
 * in both — see AppRulesState for the invariant that follows from this.
 */
enum class RuleAction { OutOfVpn, IntoVpn, Block }
