package com.resultv.android.vpn

import android.os.Build
import mobile.Mobile
import org.json.JSONObject

/**
 * Build the `optionsJson` payload that `Mobile.buildSingBoxConfigV2*`
 * accepts. Reads live state from [SettingsRepository] and
 * [RoutingRulesRepository] so callers only need to know `dataDir` + the
 * profile they're connecting to.
 */
internal object BuildOptionsBuilder {

    fun currentOptionsJson(panic: Boolean = false): String {
        val settings = SettingsRepository.state.value
        val rules = RoutingRulesRepository.state.value
        val smartMode = rules.mode == RoutingMode.Smart
        return JSONObject()
            .put("dnsServers", SettingsRepository.resolveDnsServers())
            // "Out of VPN" domains keep the legacy transport key: the Go side
            // already knows it, and the Global-only gate lives there.
            .put("excludedDomains", RoutingRulesRepository.engineOutOfVpn().joinToString(","))
            .put("intoVpnDomains", RoutingRulesRepository.engineIntoVpn().joinToString(","))
            .put("blockedDomains", RoutingRulesRepository.engineBlocked().joinToString(","))
            .put("intoVpnApps", AppRoutingRepository.engineIntoVpn().joinToString(","))
            .put("blockedApps", AppRoutingRepository.engineBlocked().joinToString(","))
            // Honour the IPv6 toggle only when the underlying (non-VPN) network
            // actually routes IPv6. On carriers that advertise a v6 address but
            // blackhole transit, a dual-stack TUN makes every `direct` IPv6
            // connection stall ~5s (Smart mode routes non-listed hosts direct),
            // which surfaces as browser searches hanging / 502. NetworkProbe
            // reachability-tests v6 off the main thread and caches the result.
            .put("ipv6", settings.ipv6 && NetworkProbe.usableIPv6())
            .put("bypassLAN", settings.bypassLan)
            .put("logLevel", settings.logLevel)
            .put("smartMode", smartMode)
            // The blocked-domain list is NOT sent here. It lives on disk as a
            // compiled binary rule-set (dataDir/smart/smart.srs) that the engine
            // references by path. Inlining ~150k domains made this payload
            // ~4.6 MB and cost ~2s per connect (marshal + 3 JNI crossings + a
            // full sing-box re-parse). The engine keeps Global routing
            // (final=proxy) whenever no usable rule-set is on disk.
            // Ad-block (DNS+route reject via rule_set). AdBlockRepository
            // caches the SRS lists locally when present and falls back to
            // remote otherwise.
            .put("adblock", settings.adblock)
            // Browser ad-block (MITM). When on, the engine exposes a loopback
            // SOCKS inbound so the in-process MITM proxy routes its upstream
            // traffic back through the tunnel — without it the MITM dials
            // direct (this app is excluded from its own VPN) and RKN-blocked
            // sites die in the browser. Must mirror the same flag that gates
            // StartFilterProxy / setHttpProxy in ResultVpnService.
            .put("browserAdBlock", settings.browserAdBlock)
            // Kill switch: armed whenever the user enabled it; panic only while
            // the watchdog has engaged (proxy down).
            .put("killSwitchArmed", settings.killSwitch)
            .put("killSwitchPanic", panic)
            .toString()
    }

    /** Build a sing-box config for the given profile using the live settings. */
    fun buildConfig(active: Profile, dataDir: String, panic: Boolean = false): String? {
        val opts = currentOptionsJson(panic)
        return try {
            when {
                active.entryJson.isNotBlank() ->
                    Mobile.buildSingBoxConfigFromEntryV2(active.entryJson, dataDir, opts)
                active.uri.isNotBlank() ->
                    Mobile.buildSingBoxConfigV2(active.uri, dataDir, opts)
                else -> null
            }
        } catch (t: Throwable) {
            null
        }
    }

    /**
     * Build a config from a raw ProxyEntry override JSON. Used by the AUTO
     * failover path so callers can swap which member is being tried without
     * mutating the user-visible Profile, and by the AUTO-aware reload path
     * (which needs panic to reach the kill switch the same way [buildConfig]
     * does — without this parameter, routing a kill-switch reload through an
     * AUTO member would silently disarm it).
     */
    fun buildConfigFromEntry(entryJson: String, dataDir: String, panic: Boolean = false): String? {
        if (entryJson.isBlank()) return null
        return try {
            Mobile.buildSingBoxConfigFromEntryV2(entryJson, dataDir, currentOptionsJson(panic))
        } catch (t: Throwable) {
            null
        }
    }

    /**
     * Build the JSON payload for `Mobile.fetchSubscriptionV3`. Pulls the
     * user-configured UA / HWID preferences out of [SettingsRepository] and
     * tags the request with the device-id headers Happ / Remnawave panels
     * use for their device-list UI.
     */
    fun currentSubscriptionFetchOptionsJson(): String {
        val s = SettingsRepository.state.value
        return JSONObject().apply {
            put("userAgent", s.subscriptionUserAgent)
            put("sendHwid", s.subscriptionSendHwid)
            // Stable across reinstalls — keeps the panel from registering a
            // duplicate device (and eating a HWID slot) on every install.
            put("hwid", SettingsRepository.deviceHwidSource())
            put("deviceOs", "Android")
            put("osVersion", Build.VERSION.RELEASE.orEmpty())
            put("model", listOf(Build.MANUFACTURER, Build.MODEL)
                .filter { !it.isNullOrBlank() }
                .joinToString(" ")
                .trim())
        }.toString()
    }
}
