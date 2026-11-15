package com.resultv.android.vpn

import android.content.Context
import android.content.SharedPreferences
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import mobile.Mobile
import org.json.JSONArray
import org.json.JSONObject

internal data class Socks5ExitProfile(
    val enabled: Boolean = false,
    val host: String = "",
    val port: Int = 1080,
    val username: String = "",
    val password: String = "",
    val sites: List<String> = emptyList(),
    val apps: Set<String> = emptySet(),
) {
    fun toJson() = JSONObject()
        .put("enabled", enabled).put("host", host).put("port", port)
        .put("username", username).put("password", password)
        .put("sites", JSONArray(sites)).put("apps", JSONArray(apps.sorted()))

    companion object {
        fun fromJson(json: JSONObject): Socks5ExitProfile {
            val sites = json.optJSONArray("sites")
            return Socks5ExitProfile(
                json.optBoolean("enabled"),
                json.optString("host"),
                json.optInt("port", 1080),
                json.optString("username"),
                json.optString("password"),
                if (sites == null) emptyList() else (0 until sites.length()).map { sites.getString(it) },
                json.stringSet("apps"),
            )
        }
    }
}

internal data class Socks5ExitCheck(
    val inProgress: Boolean = false,
    val failed: Boolean = false,
    val ip: String = "",
    val country: String = "",
    val udp: Boolean = false,
)

internal object Socks5ExitRepository {
    private val mutableProfile = MutableStateFlow(Socks5ExitProfile())
    val profile = mutableProfile.asStateFlow()
    private val mutableCheck = MutableStateFlow(Socks5ExitCheck())
    val check = mutableCheck.asStateFlow()
    private var preferences: SharedPreferences? = null
    private var checkGeneration = 0L

    @Synchronized
    fun init(context: Context) {
        if (preferences != null) return
        val prefs = context.getSharedPreferences("socks5_exit", Context.MODE_PRIVATE)
        mutableProfile.value = runCatching {
            Socks5ExitProfile.fromJson(JSONObject(prefs.getString("profile", "{}") ?: "{}"))
        }.getOrDefault(Socks5ExitProfile())
        preferences = prefs
    }

    @Synchronized
    fun save(draft: Socks5ExitProfile) {
        val normalized = if (draft.host.isBlank() && !draft.enabled) {
            Socks5ExitProfile(sites = draft.sites, apps = draft.apps)
        } else {
            Socks5ExitProfile.fromJson(JSONObject(Mobile.normalizeSocks5Exit(draft.toJson().toString())))
        }
        if (normalized == mutableProfile.value) return
        check(preferences?.edit()?.putString("profile", normalized.toJson().toString())?.commit() == true)
        resetCheck()
        mutableProfile.value = normalized
    }

    @Synchronized
    fun resetCheck() {
        checkGeneration++
        mutableCheck.value = Socks5ExitCheck()
    }

    fun checkExit() {
        val started = synchronized(this) {
            if (!profile.value.enabled || check.value.inProgress) return
            mutableCheck.value = Socks5ExitCheck(inProgress = true)
            ++checkGeneration
        }
        val result = runCatching {
            val json = JSONObject(BoxModule.checkSocks5Exit())
            Socks5ExitCheck(ip = json.getString("ip"), country = json.optString("country"), udp = json.optBoolean("udp"))
        }.getOrElse { Socks5ExitCheck(failed = true) }
        synchronized(this) {
            if (checkGeneration == started) mutableCheck.value = result
        }
    }
}
