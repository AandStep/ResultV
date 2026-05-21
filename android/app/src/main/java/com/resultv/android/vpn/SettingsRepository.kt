package com.resultv.android.vpn

import android.content.Context
import android.content.SharedPreferences
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/**
 * Live UI settings backed by SharedPreferences. Distinct from
 * [ProfileRepository] / [RoutingRulesRepository] / [AppRoutingRepository]
 * because these flags are app chrome (theme, language, DNS pick) rather
 * than per-profile or per-rule data.
 *
 * The placeholder toggles (`Kill Switch`, `Ad blocking`, `IPv6`) live here
 * too even though they aren't wired to the engine yet — that way when the
 * engine path lands we don't have to re-thread state plumbing.
 */
data class SettingsState(
    /** "Auto" / "Google" / "Cloudflare" / "Quad9" / "Custom". */
    val dnsPreset: String = "Cloudflare",
    /** Comma-separated server list when [dnsPreset] == "Custom". */
    val dnsCustom: String = "",
    val killSwitch: Boolean = false,
    val adblock: Boolean = false,
    val ipv6: Boolean = false,
    /** RFC1918 / link-local / multicast traffic bypasses the proxy. */
    val bypassLan: Boolean = true,
    /** sing-box log.level — "info" for ship, "debug" for protocol bring-up. */
    val logLevel: String = "info",
)

object SettingsRepository {
    private const val PREFS = "resultv_settings"
    private const val K_DNS_PRESET = "dns_preset"
    private const val K_DNS_CUSTOM = "dns_custom"
    private const val K_KILL_SWITCH = "kill_switch"
    private const val K_ADBLOCK = "adblock"
    private const val K_IPV6 = "ipv6"
    private const val K_BYPASS_LAN = "bypass_lan"
    private const val K_LOG_LEVEL = "log_level"

    private lateinit var prefs: SharedPreferences

    private val _state = MutableStateFlow(SettingsState())
    val state: StateFlow<SettingsState> = _state.asStateFlow()

    fun init(context: Context) {
        if (::prefs.isInitialized) return
        prefs = context.applicationContext.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
        _state.value = SettingsState(
            dnsPreset = prefs.getString(K_DNS_PRESET, "Cloudflare") ?: "Cloudflare",
            dnsCustom = prefs.getString(K_DNS_CUSTOM, "") ?: "",
            killSwitch = prefs.getBoolean(K_KILL_SWITCH, false),
            adblock = prefs.getBoolean(K_ADBLOCK, false),
            ipv6 = prefs.getBoolean(K_IPV6, false),
            bypassLan = prefs.getBoolean(K_BYPASS_LAN, true),
            logLevel = prefs.getString(K_LOG_LEVEL, "info") ?: "info",
        )
    }

    fun setDnsPreset(preset: String, custom: String = "") = mutate {
        prefs.edit()
            .putString(K_DNS_PRESET, preset)
            .putString(K_DNS_CUSTOM, custom)
            .apply()
        it.copy(dnsPreset = preset, dnsCustom = custom)
    }

    fun setKillSwitch(enabled: Boolean) = mutate {
        prefs.edit().putBoolean(K_KILL_SWITCH, enabled).apply()
        it.copy(killSwitch = enabled)
    }

    fun setAdblock(enabled: Boolean) = mutate {
        prefs.edit().putBoolean(K_ADBLOCK, enabled).apply()
        it.copy(adblock = enabled)
    }

    fun setIpv6(enabled: Boolean) = mutate {
        prefs.edit().putBoolean(K_IPV6, enabled).apply()
        it.copy(ipv6 = enabled)
    }

    fun setBypassLan(enabled: Boolean) = mutate {
        prefs.edit().putBoolean(K_BYPASS_LAN, enabled).apply()
        it.copy(bypassLan = enabled)
    }

    fun setLogLevel(level: String) = mutate {
        prefs.edit().putString(K_LOG_LEVEL, level).apply()
        it.copy(logLevel = level)
    }

    /**
     * Resolve the active DNS server string for the engine.
     * Returns `""` when "Auto" is selected — the Go-side builder treats
     * an empty string as "use built-in defaults".
     *
     * Visible to MainActivity / ResultVpnService for buildSingBoxConfig.
     */
    fun resolveDnsServers(): String {
        val s = _state.value
        return when (s.dnsPreset) {
            "Auto" -> ""
            "Google" -> "8.8.8.8, 8.8.4.4"
            "Cloudflare" -> "1.1.1.1, 1.0.0.1"
            "Quad9" -> "9.9.9.9, 149.112.112.112"
            "Custom" -> s.dnsCustom
            else -> "1.1.1.1, 1.0.0.1"
        }
    }

    private inline fun mutate(crossinline f: (SettingsState) -> SettingsState) {
        _state.value = f(_state.value)
    }
}
