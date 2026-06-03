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

    fun currentOptionsJson(): String {
        val settings = SettingsRepository.state.value
        val rules = RoutingRulesRepository.state.value
        val smartMode = rules.mode == RoutingMode.Smart
        return JSONObject()
            .put("dnsServers", SettingsRepository.resolveDnsServers())
            .put("excludedDomains", rules.domainExclusions.joinToString(","))
            .put("ipv6", settings.ipv6)
            .put("bypassLAN", settings.bypassLan)
            .put("logLevel", settings.logLevel)
            .put("smartMode", smartMode)
            // Hand the engine the resolved blocked-domain list. Empty when
            // Smart is off OR when the first download hasn't completed yet
            // — engine treats empty list as "no domains, behave like Global
            // → direct" which is the safe fallback while a fetch is pending.
            .put("smartBlockedDomainsList", if (smartMode) SmartListRepository.toEngineList() else "")
            .toString()
    }

    /** Build a sing-box config for the given profile using the live settings. */
    fun buildConfig(active: Profile, dataDir: String): String? {
        val opts = currentOptionsJson()
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
     * mutating the user-visible Profile.
     */
    fun buildConfigFromEntry(entryJson: String, dataDir: String): String? {
        if (entryJson.isBlank()) return null
        return try {
            Mobile.buildSingBoxConfigFromEntryV2(entryJson, dataDir, currentOptionsJson())
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
