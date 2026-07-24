package com.resultv.android.vpn

/**
 * What a rule does with a package's or a domain's traffic.
 *
 * [OutOfVpn] excludes an app from the tunnel in BOTH modes (the shared "never
 * tunnel this app" list): in Global it bypasses the proxy, in Smart it also
 * removes the app from the allowlist so VPN-hostile apps can't detect the VPN.
 * [IntoVpn] is Smart-only (force into the tunnel/proxy). [Block] applies in
 * both — see AppRulesState for the invariant that follows from this.
 */
enum class RuleAction { OutOfVpn, IntoVpn, Block }
