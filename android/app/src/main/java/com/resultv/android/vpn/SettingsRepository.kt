package com.resultv.android.vpn

import android.content.Context
import android.content.SharedPreferences
import android.provider.Settings
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/**
 * Live UI settings backed by SharedPreferences. Distinct from
 * [ProfileRepository] / [RoutingRulesRepository] / [AppRoutingRepository]
 * because these flags are app chrome (theme, language, DNS pick) rather
 * than per-profile or per-rule data.
 *
 * The placeholder `Kill Switch` toggle lives here too even though it isn't
 * wired to the engine yet — that way when its engine path lands we don't have
 * to re-thread state plumbing. (`Ad blocking` / `YouTube` ARE wired — see
 * [BuildOptionsBuilder] and the Go `adblock` / `youtubeUnblock` options.)
 */
data class SettingsState(
    /** "Auto" / "Google" / "Cloudflare" / "Quad9" / "Custom". */
    val dnsPreset: String = "Cloudflare",
    /** Comma-separated server list when [dnsPreset] == "Custom". */
    val dnsCustom: String = "",
    val killSwitch: Boolean = false,
    val adblock: Boolean = false,
    /**
     * YouTube geo-split: video routes through the proxy (dodges RKN
     * throttling) while the player/API routes direct so Google sees the user's
     * Russian IP and serves no ads. Only effective for users physically in
     * Russia. See [BuildOptionsBuilder] + Go `youtube_rules.go`.
     */
    val youtubeUnblock: Boolean = false,
    val ipv6: Boolean = false,
    /** RFC1918 / link-local / multicast traffic bypasses the proxy. */
    val bypassLan: Boolean = true,
    /** sing-box log.level — "info" for ship, "debug" for protocol bring-up. */
    val logLevel: String = "info",
    /** Auto-refresh subscriptions on the timer. */
    val subscriptionAutoUpdate: Boolean = true,
    /** Hours between auto-refresh cycles when [subscriptionAutoUpdate] is on. */
    val subscriptionUpdateIntervalHours: Int = 6,
    /**
     * Send the stable device fingerprint (x-hwid header + x-device-* tags)
     * with subscription fetches. Panels that gate by HWID need this; users
     * who care about telemetry can disable it.
     */
    val subscriptionSendHwid: Boolean = true,
    /**
     * Override the User-Agent sent with subscription fetches. Blank =
     * builder-default ("ResultV/<ver>/Android/<api>") — matches desktop.
     */
    val subscriptionUserAgent: String = "",
)

object SettingsRepository {
    private const val PREFS = "resultv_settings"
    private const val K_DNS_PRESET = "dns_preset"
    private const val K_DNS_CUSTOM = "dns_custom"
    private const val K_KILL_SWITCH = "kill_switch"
    private const val K_ADBLOCK = "adblock"
    private const val K_YOUTUBE = "youtube_unblock"
    private const val K_IPV6 = "ipv6"
    private const val K_BYPASS_LAN = "bypass_lan"
    private const val K_LOG_LEVEL = "log_level"
    private const val K_SUB_AUTO = "sub_auto_update"
    private const val K_SUB_INTERVAL = "sub_update_interval_hours"
    private const val K_SUB_HWID = "sub_send_hwid"
    private const val K_SUB_UA = "sub_user_agent"

    private lateinit var prefs: SharedPreferences

    /**
     * Stable per-device fingerprint source for the subscription `x-hwid`
     * header. Settings.Secure.ANDROID_ID survives app reinstalls (it's keyed
     * to device + signing key + user), unlike the Go-side fallback file in
     * filesDir which is wiped on reinstall/clear-data — relying on that file
     * registered a fresh panel device on every install and burned HWID slots.
     * Captured once at [init]; Go hashes it with the shared HWID salt.
     */
    @Volatile
    private var hwidSource: String = ""

    /** Stable device id passed to `Mobile.fetchSubscriptionV3` as `hwid`. */
    fun deviceHwidSource(): String = hwidSource

    private val _state = MutableStateFlow(SettingsState())
    val state: StateFlow<SettingsState> = _state.asStateFlow()

    fun init(context: Context) {
        if (::prefs.isInitialized) return
        val app = context.applicationContext
        hwidSource = runCatching {
            Settings.Secure.getString(app.contentResolver, Settings.Secure.ANDROID_ID)
        }.getOrNull().orEmpty()
        prefs = app.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
        _state.value = SettingsState(
            dnsPreset = prefs.getString(K_DNS_PRESET, "Cloudflare") ?: "Cloudflare",
            dnsCustom = prefs.getString(K_DNS_CUSTOM, "") ?: "",
            killSwitch = prefs.getBoolean(K_KILL_SWITCH, false),
            adblock = prefs.getBoolean(K_ADBLOCK, false),
            youtubeUnblock = prefs.getBoolean(K_YOUTUBE, false),
            ipv6 = prefs.getBoolean(K_IPV6, false),
            bypassLan = prefs.getBoolean(K_BYPASS_LAN, true),
            logLevel = prefs.getString(K_LOG_LEVEL, "info") ?: "info",
            subscriptionAutoUpdate = prefs.getBoolean(K_SUB_AUTO, true),
            subscriptionUpdateIntervalHours = prefs.getInt(K_SUB_INTERVAL, 6).coerceAtLeast(1),
            subscriptionSendHwid = prefs.getBoolean(K_SUB_HWID, true),
            subscriptionUserAgent = prefs.getString(K_SUB_UA, "") ?: "",
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

    fun setYoutubeUnblock(enabled: Boolean) = mutate {
        prefs.edit().putBoolean(K_YOUTUBE, enabled).apply()
        it.copy(youtubeUnblock = enabled)
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

    fun setSubscriptionAutoUpdate(enabled: Boolean) = mutate {
        prefs.edit().putBoolean(K_SUB_AUTO, enabled).apply()
        it.copy(subscriptionAutoUpdate = enabled)
    }

    fun setSubscriptionUpdateIntervalHours(hours: Int) = mutate {
        val sane = hours.coerceAtLeast(1)
        prefs.edit().putInt(K_SUB_INTERVAL, sane).apply()
        it.copy(subscriptionUpdateIntervalHours = sane)
    }

    fun setSubscriptionSendHwid(enabled: Boolean) = mutate {
        prefs.edit().putBoolean(K_SUB_HWID, enabled).apply()
        it.copy(subscriptionSendHwid = enabled)
    }

    fun setSubscriptionUserAgent(ua: String) = mutate {
        val trimmed = ua.trim()
        prefs.edit().putString(K_SUB_UA, trimmed).apply()
        it.copy(subscriptionUserAgent = trimmed)
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
