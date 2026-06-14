package com.resultv.android.vpn

import android.content.Context
import android.util.Log
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.util.UUID

private const val TAG = "ResultV/Profiles"
private const val FILE_NAME = "profiles.json"

/**
 * A profile carries either a source URI (URL-pasted imports, share links)
 * or a parsed sing-box ProxyEntry JSON (subscription imports built from
 * Xray JSON, where there is no original URI to round-trip). At least one
 * is populated; Connect prefers entryJson when present.
 */
data class Profile(
    val id: String,
    val name: String,
    val uri: String = "",
    val entryJson: String = "",
    val isFavorite: Boolean = false,
    /** Non-empty when this profile came from a subscription import. */
    val subscriptionId: String = "",
    /** SECTION rows are labels (impVPN "выберите конфиг ниже"), not real outbounds. */
    val isSection: Boolean = false,
) {
    // Cached derived metadata — body-properties are computed once at
    // construction time and aren't part of equals/hashCode/copy/toString.
    // A .copy() reparses for the new instance (happens on rename / favourite
    // toggle, never in render paths).

    private val parsedEntry: JSONObject? = if (entryJson.isBlank()) null
        else runCatching { JSONObject(entryJson) }.getOrNull()

    /** Raw `type` from entryJson (e.g. "VLESS", "AUTO"). Empty if no entry. */
    val rawType: String = parsedEntry?.optString("type").orEmpty()

    /** ISO-2 country code from entryJson, or null for sections / URI-only. */
    val country: String? = if (isSection) null
        else parsedEntry?.optString("country")?.takeIf { it.length == 2 }

    /** True when this is an AUTO multi-target entry. */
    val isAuto: Boolean = rawType.equals("AUTO", ignoreCase = true)

    /** Normalised protocol code for the filter chips (SS→SHADOWSOCKS, WG/AWG aliases collapsed, AUTO→""). */
    val protocol: String = computeProtocol(isSection, rawType, uri)

    /** Pre-formatted subtitle string used by ServerRow — avoids per-render JSON parsing. */
    val subtitle: String = computeSubtitle()

    fun toJson(): JSONObject = JSONObject()
        .put("id", id)
        .put("name", name)
        .put("uri", uri)
        .put("entryJson", entryJson)
        .put("isFavorite", isFavorite)
        .put("subscriptionId", subscriptionId)
        .put("isSection", isSection)

    /** PingRepository reads this to avoid re-parsing entryJson on every probe. */
    internal fun cachedEntry(): JSONObject? = parsedEntry

    private fun computeSubtitle(): String {
        if (isSection) return ""
        if (subscriptionId.isNotBlank()) {
            return rawType.ifBlank { protocolFromUri(uri).orEmpty() }
        }
        if (uri.isNotBlank()) {
            // Show only the protocol (e.g. "VLESS", "AWG"), mirroring how
            // subscription rows display their type rather than the full URI.
            return rawType.ifBlank { protocolFromUri(uri).orEmpty() }
        }
        val entry = parsedEntry ?: return ""
        val type = entry.optString("type")
        val ip = entry.optString("ip")
        val port = entry.optInt("port")
        return listOfNotNull(
            type.takeIf { it.isNotBlank() },
            "$ip:$port".takeIf { ip.isNotBlank() }
        ).joinToString("  ·  ")
    }

    companion object {
        fun fromJson(o: JSONObject) = Profile(
            id = o.getString("id"),
            name = o.getString("name"),
            uri = o.optString("uri"),
            entryJson = o.optString("entryJson"),
            isFavorite = o.optBoolean("isFavorite", false),
            subscriptionId = o.optString("subscriptionId"),
            isSection = o.optBoolean("isSection", false),
        )

        fun fromUri(name: String, uri: String, subscriptionId: String = "") = Profile(
            id = UUID.randomUUID().toString(),
            name = name.ifBlank { "Untitled" },
            uri = uri,
            subscriptionId = subscriptionId,
        )

        fun fromEntryJson(name: String, entryJson: String, subscriptionId: String = "") = Profile(
            id = UUID.randomUUID().toString(),
            name = name.ifBlank { "Untitled" },
            entryJson = entryJson,
            subscriptionId = subscriptionId,
        )

        /** SECTION row — a visual separator imported from a subscription. */
        fun section(name: String, subscriptionId: String) = Profile(
            id = UUID.randomUUID().toString(),
            name = name.ifBlank { "—" },
            subscriptionId = subscriptionId,
            isSection = true,
        )
    }
}

private fun computeProtocol(isSection: Boolean, rawType: String, uri: String): String {
    if (isSection) return ""
    val raw = rawType.ifBlank { protocolFromUri(uri).orEmpty() }
    val up = raw.uppercase()
    if (up == "AUTO") return ""
    return when (up) {
        "SS", "SHADOWSOCKS" -> "SHADOWSOCKS"
        "WG", "WIREGUARD" -> "WIREGUARD"
        "AWG", "AMNEZIAWG", "AMNEZIA-WG" -> "AMNEZIAWG"
        else -> up
    }
}

private fun protocolFromUri(uri: String): String? {
    val schemeEnd = uri.indexOf("://")
    if (schemeEnd <= 0) return null
    return uri.substring(0, schemeEnd).uppercase()
}

data class ProfilesState(
    val profiles: List<Profile> = emptyList(),
    val activeId: String? = null,
) {
    val active: Profile? get() = profiles.firstOrNull { it.id == activeId }
}

/**
 * Single-process JSON-backed profile store. All access is synchronized; the
 * file is small (URIs + names) so we always write the whole document.
 */
object ProfileRepository {
    private val _state = MutableStateFlow(ProfilesState())
    val state: StateFlow<ProfilesState> = _state.asStateFlow()

    @Volatile private var file: File? = null

    @Synchronized
    fun init(ctx: Context) {
        if (file != null) return
        val f = File(ctx.filesDir, FILE_NAME)
        file = f
        _state.value = load(f)
    }

    @Synchronized
    fun add(profile: Profile) = mutate { s ->
        val list = s.profiles + profile
        s.copy(profiles = list, activeId = s.activeId ?: profile.id)
    }

    @Synchronized
    fun remove(id: String) = mutate { s ->
        val list = s.profiles.filterNot { it.id == id }
        val active = if (s.activeId == id) list.firstOrNull()?.id else s.activeId
        s.copy(profiles = list, activeId = active)
    }

    /**
     * Drop every profile tagged with [subscriptionId]. Used by
     * SubscriptionRepository.delete to cascade. SECTION rows go away too.
     */
    @Synchronized
    fun removeBySubscriptionId(subscriptionId: String) = mutate { s ->
        val list = s.profiles.filterNot { it.subscriptionId == subscriptionId }
        val active = if (list.any { it.id == s.activeId }) s.activeId else list.firstOrNull()?.id
        s.copy(profiles = list, activeId = active)
    }

    /**
     * Names of the user's favourited profiles within one subscription.
     * Used to carry favourites across a refresh / re-import, where the old
     * profile records are dropped and rebuilt — favourites are matched back
     * by display name (the only stable identity an entry keeps across fetches).
     */
    fun favouriteNamesFor(subscriptionId: String): Set<String> =
        _state.value.profiles.asSequence()
            .filter { it.subscriptionId == subscriptionId && it.isFavorite }
            .map { it.name }
            .toSet()

    /** Replace all profiles for one subscription with a fresh set (used on refresh). */
    @Synchronized
    fun replaceForSubscription(subscriptionId: String, fresh: List<Profile>) = mutate { s ->
        val kept = s.profiles.filterNot { it.subscriptionId == subscriptionId }
        val list = kept + fresh
        val active = if (list.any { it.id == s.activeId }) s.activeId else list.firstOrNull()?.id
        s.copy(profiles = list, activeId = active)
    }

    @Synchronized
    fun setActive(id: String) = mutate { s ->
        val target = s.profiles.firstOrNull { it.id == id } ?: return@mutate s
        // SECTION rows are labels, never selectable. Guard here too so a
        // stale UI tap can't poison activeId.
        if (target.isSection) return@mutate s
        s.copy(activeId = id)
    }

    @Synchronized
    fun toggleFavorite(id: String) = mutate { s ->
        val list = s.profiles.map {
            if (it.id == id) it.copy(isFavorite = !it.isFavorite) else it
        }
        s.copy(profiles = list)
    }

    @Synchronized
    fun rename(id: String, newName: String) = mutate { s ->
        val trimmed = newName.trim().ifBlank { return@mutate s }
        val list = s.profiles.map {
            if (it.id == id) it.copy(name = trimmed) else it
        }
        s.copy(profiles = list)
    }

    /**
     * Replace a standalone profile's name + share-URI in place (used by the
     * full "Edit" sheet). Keeps id / favourite / subscription so the row's
     * identity, ordering and active-selection survive the edit. entryJson is
     * cleared because the new URI is the source of truth for these profiles.
     */
    @Synchronized
    fun updateUri(id: String, newName: String, newUri: String) = mutate { s ->
        val name = newName.trim().ifBlank { return@mutate s }
        val uri = newUri.trim().ifBlank { return@mutate s }
        val list = s.profiles.map {
            if (it.id == id) it.copy(name = name, uri = uri, entryJson = "") else it
        }
        s.copy(profiles = list)
    }

    /**
     * Swap the profile with id [id] one step up (delta=-1) or down (delta=+1)
     * inside its own "bucket": same [Profile.subscriptionId] and excluding
     * SECTION rows (those are visual labels and stay where they are).
     * Sub-imported groups therefore reorder within their group; standalone
     * profiles reorder within the standalone bucket. No-op at the edges.
     */
    @Synchronized
    fun move(id: String, delta: Int) = mutate { s ->
        if (delta == 0) return@mutate s
        val list = s.profiles.toMutableList()
        val idx = list.indexOfFirst { it.id == id }
        if (idx < 0) return@mutate s
        val target = list[idx]
        if (target.isSection) return@mutate s
        // Walk in the direction of delta until we land on another profile
        // in the same bucket (non-section, same subscriptionId). Sections
        // between us and the next sibling are skipped, not crossed.
        val swapIdx = idx + if (delta < 0) -1 else 1
        if (swapIdx !in list.indices) return@mutate s
        val neighbor = list[swapIdx]
        // Reorder never crosses a section label or a different subscription
        // bucket — those are visual boundaries.
        if (neighbor.isSection || neighbor.subscriptionId != target.subscriptionId) {
            return@mutate s
        }
        list[idx] = neighbor
        list[swapIdx] = target
        s.copy(profiles = list)
    }

    private fun mutate(block: (ProfilesState) -> ProfilesState) {
        val next = block(_state.value)
        if (next === _state.value) return
        _state.value = next
        file?.let { save(it, next) }
    }

    private fun load(f: File): ProfilesState {
        if (!f.exists()) return ProfilesState()
        return try {
            val root = JSONObject(f.readText())
            val arr = root.optJSONArray("profiles") ?: JSONArray()
            val list = (0 until arr.length()).map { Profile.fromJson(arr.getJSONObject(it)) }
            ProfilesState(profiles = list, activeId = root.optString("activeId").takeIf { it.isNotEmpty() })
        } catch (t: Throwable) {
            Log.w(TAG, "failed to read $f, starting empty", t)
            ProfilesState()
        }
    }

    private fun save(f: File, s: ProfilesState) {
        try {
            val arr = JSONArray()
            s.profiles.forEach { arr.put(it.toJson()) }
            val root = JSONObject()
                .put("profiles", arr)
                .put("activeId", s.activeId ?: JSONObject.NULL)
            f.writeText(root.toString())
        } catch (t: Throwable) {
            Log.e(TAG, "failed to persist profiles", t)
        }
    }
}
