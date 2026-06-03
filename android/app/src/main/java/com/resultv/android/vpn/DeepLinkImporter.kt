package com.resultv.android.vpn

import android.content.Context
import android.util.Log
import android.widget.Toast
import com.resultv.android.R
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.MainScope
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import mobile.Mobile
import org.json.JSONArray
import org.json.JSONObject

private const val TAG = "ResultV/DeepLink"

/**
 * One-shot import path triggered by `resultv://` intents. The Go side
 * (`Mobile.decodeDeepLink`) handles the RVSUB1 AES-GCM ciphertext that
 * arrives in `resultv://import/<base64>` and friends; we just need to
 * interpret the *plaintext* it returns:
 *
 *   - a single http(s) URL → subscription, run through fetchSubscriptionV2.
 *   - a single proxy URI (vless://, vmess://, …) → one standalone profile.
 *   - a multi-line text body → parse each line, accumulate profiles.
 *
 * Errors surface as toasts so a bad link never silently no-ops or crashes
 * launch. SubscriptionRepository / ProfileRepository writes are
 * synchronous so the user sees imported entries immediately when the
 * Proxies tab next collects.
 */
object DeepLinkImporter {

    fun import(context: Context, rawUrl: String) {
        val appCtx = context.applicationContext
        ensureReposReady(appCtx)
        MainScope().launch {
            val decoded = withContext(Dispatchers.IO) {
                runCatching { Mobile.decodeDeepLink(rawUrl) }
            }
            if (decoded.isFailure) {
                toast(appCtx, R.string.deeplink_err_decode)
                Log.w(TAG, "decode failed", decoded.exceptionOrNull())
                return@launch
            }
            val payload = JSONObject(decoded.getOrNull()!!)
            val plain = payload.optString("plain").trim()
            val source = payload.optString("source")
            if (plain.isEmpty()) {
                toast(appCtx, R.string.deeplink_err_empty)
                return@launch
            }
            dispatch(appCtx, plain, source)
        }
    }

    /**
     * Import an already-plaintext payload (no RVSUB1 decryption). Used by
     * the QR scanner — what comes off the camera is whatever the QR encodes
     * (share-link, subscription URL, or a `resultv://…` deep-link). For the
     * latter we fall back into [import] so the AES-GCM ciphertext path still
     * runs; everything else goes through the same dispatch logic as a
     * decoded deep-link.
     */
    fun importPlain(context: Context, raw: String) {
        val appCtx = context.applicationContext
        ensureReposReady(appCtx)
        val trimmed = raw.trim()
        if (trimmed.isEmpty()) {
            toast(appCtx, R.string.deeplink_err_empty)
            return
        }
        if (trimmed.lowercase().startsWith("resultv:")) {
            import(appCtx, trimmed)
            return
        }
        MainScope().launch { dispatch(appCtx, trimmed, sourceTag = "") }
    }

    /**
     * Shared dispatch for both the deep-link and QR paths: single http URL
     * → subscription fetch, anything else → line-by-line share-link import.
     */
    private suspend fun dispatch(ctx: Context, plain: String, sourceTag: String) {
        val firstLine = plain.lineSequence().firstOrNull()?.trim().orEmpty()
        val isSingleHttpUrl = isHttpUrl(firstLine) &&
            plain.lineSequence().count { it.isNotBlank() } == 1
        if (isSingleHttpUrl) {
            importSubscription(ctx, firstLine, sourceTag)
        } else {
            importInlineUris(ctx, plain)
        }
    }

    /**
     * Subscription URL path — re-uses Mobile.fetchSubscriptionV2 so we get
     * the same Subscription-Userinfo / Profile-Title metadata as the
     * AddScreen flow.
     */
    private suspend fun importSubscription(ctx: Context, subUrl: String, sourceTag: String) {
        val dataDir = ctx.filesDir.absolutePath
        val responseJson = withContext(Dispatchers.IO) {
            runCatching {
                Mobile.fetchSubscriptionV3(
                    subUrl,
                    dataDir,
                    BuildOptionsBuilder.currentSubscriptionFetchOptionsJson(),
                )
            }
        }
        if (responseJson.isFailure) {
            toast(ctx, R.string.deeplink_err_fetch)
            Log.w(TAG, "fetch failed", responseJson.exceptionOrNull())
            return
        }
        val response = JSONObject(responseJson.getOrNull()!!)
        val arr = response.optJSONArray("entries") ?: JSONArray()
        val title = response.optString("title")
        val userInfo = response.optString("userInfo")
        val supportUrl = response.optString("supportUrl")

        // Re-importing the same URL updates the existing record in place
        // rather than spawning a duplicate; a fresh URL inserts a new one.
        val subId = SubscriptionRepository.upsert(
            url = subUrl,
            name = title.ifBlank { subUrl.substringAfter("://").substringBefore('/') },
            title = title,
            userInfo = userInfo,
            supportUrl = supportUrl,
            source = sourceTag,
        )
        val favouriteNames = ProfileRepository.favouriteNamesFor(subId)

        val profiles = (0 until arr.length()).map { i ->
            val o = arr.getJSONObject(i)
            val type = o.optString("type")
            val name = o.optString("name").ifBlank { "Profile ${i + 1}" }
            if (type.equals("SECTION", ignoreCase = true)) {
                Profile.section(name, subscriptionId = subId)
            } else {
                val uri = o.optString("uri")
                val base = if (uri.isNotBlank())
                    Profile.fromUri(name, uri, subscriptionId = subId)
                else
                    Profile.fromEntryJson(name, o.toString(), subscriptionId = subId)
                if (base.name in favouriteNames) base.copy(isFavorite = true) else base
            }
        }
        ProfileRepository.replaceForSubscription(subId, profiles)
        val imported = profiles.count { !it.isSection }
        toast(ctx, R.string.deeplink_imported_subscription, imported)
    }

    /**
     * Inline URI list — each non-empty line is parsed as a share-link and,
     * if it validates, becomes a standalone profile (subscriptionId="").
     */
    private fun importInlineUris(ctx: Context, body: String) {
        var added = 0
        body.lineSequence().forEach { raw ->
            val trimmed = raw.trim()
            if (trimmed.isEmpty()) return@forEach
            runCatching {
                Mobile.parseProxyURI(trimmed)
                ProfileRepository.add(Profile.fromUri(nameFromShareUri(trimmed), trimmed))
                added++
            }
        }
        if (added == 0) {
            toast(ctx, R.string.deeplink_err_no_uris)
        } else {
            toast(ctx, R.string.deeplink_imported_profiles, added)
        }
    }

    private fun isHttpUrl(line: String): Boolean {
        val low = line.lowercase()
        return low.startsWith("http://") || low.startsWith("https://")
    }

    /** Best-effort: pull a friendly name from the URI fragment, fall back to scheme. */
    private fun nameFromShareUri(uri: String): String {
        val frag = uri.substringAfter('#', "").trim()
        if (frag.isNotEmpty()) {
            return runCatching { java.net.URLDecoder.decode(frag, "UTF-8") }
                .getOrDefault(frag)
        }
        val scheme = uri.substringBefore("://", "").ifBlank { return "Profile" }
        return scheme.uppercase()
    }

    private fun ensureReposReady(ctx: Context) {
        ProfileRepository.init(ctx)
        SubscriptionRepository.init(ctx)
        SettingsRepository.init(ctx)
        RoutingRulesRepository.init(ctx)
        AppRoutingRepository.init(ctx)
    }

    private fun toast(ctx: Context, resId: Int, vararg args: Any) {
        val msg = if (args.isEmpty()) ctx.getString(resId) else ctx.getString(resId, *args)
        Toast.makeText(ctx, msg, Toast.LENGTH_LONG).show()
    }
}
