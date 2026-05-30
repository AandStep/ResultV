package com.resultv.android.vpn

import android.content.Context
import android.util.Base64
import android.util.Log
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.util.UUID

private const val TAG = "ResultV/Subs"
private const val FILE_NAME = "subscriptions.json"

/**
 * One imported subscription (panel URL + parsed metadata). Profiles in
 * [ProfileRepository] reference this via [Profile.subscriptionId]; deleting
 * a Subscription cascades to its profiles.
 *
 * @property userInfo SIP008 `Subscription-Userinfo` header value, e.g.
 *   "upload=0; download=1234; total=10000000000; expire=1734567890".
 *   Parsed lazily by [SubscriptionUsage.parse] for the UI.
 * @property source Provenance tag — "rvsub" for deep links delivered via
 *   `resultv://rvsub/`, "" otherwise. Drives the impVPN logo override.
 * @property customName User-set display name override; blank falls back to
 *   the panel-supplied [name] / [title].
 * @property hiddenOnHome When true, profiles from this subscription don't
 *   appear in the Home picker (still visible on the Proxies tab).
 * @property supportUrl Panel-provided "Support-Url" header value (or empty).
 *   Surfaced in the edit sheet's Links section.
 * @property customRefreshIntervalMinutes Per-sub auto-refresh override in
 *   minutes; 0 = use the global SettingsRepository interval.
 */
data class Subscription(
    val id: String,
    val url: String,
    val name: String,
    val title: String = "",
    val userInfo: String = "",
    val source: String = "",
    val lastFetchedAt: Long = 0L,
    val customName: String = "",
    val hiddenOnHome: Boolean = false,
    val supportUrl: String = "",
    val customRefreshIntervalMinutes: Int = 0,
) {
    fun toJson(): JSONObject = JSONObject()
        .put("id", id)
        .put("url", url)
        .put("name", name)
        .put("title", title)
        .put("userInfo", userInfo)
        .put("source", source)
        .put("lastFetchedAt", lastFetchedAt)
        .put("customName", customName)
        .put("hiddenOnHome", hiddenOnHome)
        .put("supportUrl", supportUrl)
        .put("customRefreshIntervalMinutes", customRefreshIntervalMinutes)

    /**
     * Best-effort display name. Order: user override → decoded `name` field
     * → decoded panel `title` → raw URL. Decodes the `base64:` panel
     * convention transparently.
     */
    val displayName: String
        get() {
            val override = customName.trim()
            if (override.isNotEmpty()) return override
            return decodePanelTitle(name).ifBlank { decodePanelTitle(title) }.ifBlank { url }
        }

    companion object {
        fun fromJson(o: JSONObject) = Subscription(
            id = o.getString("id"),
            url = o.getString("url"),
            name = o.optString("name"),
            title = o.optString("title"),
            userInfo = o.optString("userInfo"),
            source = o.optString("source"),
            lastFetchedAt = o.optLong("lastFetchedAt", 0L),
            customName = o.optString("customName"),
            hiddenOnHome = o.optBoolean("hiddenOnHome", false),
            supportUrl = o.optString("supportUrl"),
            customRefreshIntervalMinutes = o.optInt("customRefreshIntervalMinutes", 0),
        )

        fun new(url: String, name: String, title: String = "", userInfo: String = "", source: String = "") =
            Subscription(
                id = UUID.randomUUID().toString(),
                url = url,
                name = name,
                title = title,
                userInfo = userInfo,
                source = source,
                lastFetchedAt = System.currentTimeMillis(),
            )
    }
}

/** Parsed `Subscription-Userinfo` header. All numeric fields in bytes / unix seconds. */
data class SubscriptionUsage(
    val upload: Long = 0,
    val download: Long = 0,
    val total: Long = 0,
    val expireUnix: Long = 0,
) {
    val used: Long get() = upload + download
    val hasQuota: Boolean get() = total > 0
    val hasExpiry: Boolean get() = expireUnix > 0
    val expired: Boolean get() = hasExpiry && expireUnix * 1000L < System.currentTimeMillis()

    companion object {
        /** Parse `upload=…; download=…; total=…; expire=…`. Missing parts → 0. */
        fun parse(raw: String): SubscriptionUsage {
            if (raw.isBlank()) return SubscriptionUsage()
            val pairs = raw.split(";", ",").map { it.trim() }
            var up = 0L
            var dn = 0L
            var total = 0L
            var expire = 0L
            for (p in pairs) {
                val eq = p.indexOf('=')
                if (eq <= 0) continue
                val k = p.substring(0, eq).trim().lowercase()
                val v = p.substring(eq + 1).trim().toLongOrNull() ?: continue
                when (k) {
                    "upload" -> up = v
                    "download" -> dn = v
                    "total" -> total = v
                    "expire" -> expire = v
                }
            }
            return SubscriptionUsage(up, dn, total, expire)
        }
    }
}

/**
 * Some xray panels emit Profile-Title (and sometimes the user-saved name)
 * as `base64:<UTF-8 bytes>` to safely carry emoji / non-ASCII. The desktop
 * decodes this transparently; mirror that here so the UI never shows the
 * raw `base64:` prefix. Trailing/leading whitespace is tolerated.
 */
internal fun decodePanelTitle(raw: String): String {
    val s = raw.trim()
    if (!s.startsWith("base64:", ignoreCase = true)) return s
    val payload = s.substring("base64:".length).trim()
    if (payload.isEmpty()) return ""
    // Try standard (`+/`) alphabet first, then URL-safe (`-_`). OR-ing the
    // flag bits doesn't fall back — URL_SAFE strictly rejects `+`
    // characters that show up in xray-panel base64 titles.
    return runCatching {
        String(Base64.decode(payload, Base64.DEFAULT), Charsets.UTF_8).trim()
    }.recoverCatching {
        String(Base64.decode(payload, Base64.URL_SAFE), Charsets.UTF_8).trim()
    }.getOrDefault(s)
}

data class SubscriptionsState(
    val subs: List<Subscription> = emptyList(),
)

object SubscriptionRepository {
    private val _state = MutableStateFlow(SubscriptionsState())
    val state: StateFlow<SubscriptionsState> = _state.asStateFlow()

    @Volatile private var file: File? = null

    @Synchronized
    fun init(ctx: Context) {
        if (file != null) return
        val f = File(ctx.filesDir, FILE_NAME)
        file = f
        _state.value = load(f)
    }

    @Synchronized
    fun add(sub: Subscription) = mutate { s ->
        s.copy(subs = s.subs + sub)
    }

    @Synchronized
    fun update(id: String, transform: (Subscription) -> Subscription) = mutate { s ->
        s.copy(subs = s.subs.map { if (it.id == id) transform(it) else it })
    }

    /**
     * Cascade-delete: drop the subscription record and every profile tagged
     * with this subscriptionId. Active profile is cleared if it belonged here.
     */
    @Synchronized
    fun delete(id: String) {
        // Profiles first so the UI never sees an orphan with a dangling id.
        ProfileRepository.removeBySubscriptionId(id)
        mutate { s -> s.copy(subs = s.subs.filterNot { it.id == id }) }
    }

    fun byId(id: String): Subscription? = _state.value.subs.firstOrNull { it.id == id }

    private fun mutate(block: (SubscriptionsState) -> SubscriptionsState) {
        val next = block(_state.value)
        if (next === _state.value) return
        _state.value = next
        file?.let { save(it, next) }
    }

    private fun load(f: File): SubscriptionsState {
        if (!f.exists()) return SubscriptionsState()
        return try {
            val root = JSONObject(f.readText())
            val arr = root.optJSONArray("subs") ?: JSONArray()
            val list = (0 until arr.length()).map { Subscription.fromJson(arr.getJSONObject(it)) }
            SubscriptionsState(subs = list)
        } catch (t: Throwable) {
            Log.w(TAG, "failed to read $f, starting empty", t)
            SubscriptionsState()
        }
    }

    private fun save(f: File, s: SubscriptionsState) {
        try {
            val arr = JSONArray()
            s.subs.forEach { arr.put(it.toJson()) }
            val root = JSONObject().put("subs", arr)
            f.writeText(root.toString())
        } catch (t: Throwable) {
            Log.e(TAG, "failed to persist subscriptions", t)
        }
    }
}
