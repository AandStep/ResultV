package com.resultv.android.vpn

import android.content.Context
import android.util.Log
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import mobile.Mobile
import org.json.JSONArray
import org.json.JSONObject

/**
 * Periodic subscription refresher — direct mobile analogue of the desktop's
 * `useEffect` + `setInterval` in `useAppConfig.js`. Walks every
 * [SubscriptionRepository] entry on a 1-minute tick and pulls the latest
 * server list via `Mobile.fetchSubscriptionV3` for any subscription whose
 * effective interval (per-sub override → global setting) has elapsed.
 *
 * Lifetime: bound to a [CoroutineScope] passed by the caller (usually
 * [MainActivity]'s lifecycle scope). The loop is best-effort and runs only
 * while the app is in memory — same behaviour as the desktop's hook, which
 * also dies when the React tree unmounts. For background refresh (app
 * killed) a `WorkManager` job would be the next step.
 */
object SubscriptionRefresher {

    private const val TAG = "ResultV/SubRefresh"

    /**
     * Cadence at which we *check* whether anything is due. Refreshes
     * themselves still respect each sub's effective interval, so this
     * is just the resolution of "how late can a refresh be" (≤ 60s).
     */
    private const val TICK_MS = 60_000L

    private var loopJob: Job? = null
    private val mutex = Object()

    /**
     * (Re)start the periodic refresh loop. Cancels any prior loop so the
     * interval / on-off toggle changes take effect immediately when the
     * user flips the setting. Safe to call repeatedly.
     *
     * The loop ticks once a minute and refreshes only subs whose effective
     * interval has elapsed — per-sub `customRefreshIntervalMinutes` wins
     * over the global `subscriptionUpdateIntervalHours`.
     */
    fun start(scope: CoroutineScope, ctx: Context) {
        val app = ctx.applicationContext
        synchronized(mutex) {
            loopJob?.cancel()
            loopJob = scope.launch(SupervisorJob() + Dispatchers.Default) {
                // First tick happens after TICK_MS — never refresh on mount.
                // Subscriptions are freshly fetched at import time so we'd
                // otherwise hammer the panel for no reason.
                while (isActive) {
                    delay(TICK_MS)
                    val settings = SettingsRepository.state.value
                    if (!settings.subscriptionAutoUpdate) continue
                    val globalHours = settings.subscriptionUpdateIntervalHours.coerceAtLeast(1)
                    refreshDue(app, globalHours)
                }
            }
        }
    }

    fun stop() {
        synchronized(mutex) {
            loopJob?.cancel()
            loopJob = null
        }
    }

    /**
     * Re-fetch every subscription serially regardless of whether it is due.
     * Used by manual "refresh all" UI paths; the timer loop calls
     * [refreshDue] instead so per-sub intervals are honoured.
     *
     * Failures on one subscription don't halt the rest — the desktop loop
     * behaves identically.
     */
    suspend fun refreshAll(ctx: Context) {
        val dataDir = ctx.filesDir.absolutePath
        val subs = SubscriptionRepository.state.first().subs
        for (sub in subs) {
            try {
                refreshOne(sub, dataDir)
            } catch (t: Throwable) {
                Log.w(TAG, "refresh failed for ${sub.id}", t)
            }
        }
    }

    /**
     * Walk every subscription and refresh those whose effective interval
     * has elapsed since [Subscription.lastFetchedAt]. Same failure
     * semantics as [refreshAll].
     */
    private suspend fun refreshDue(ctx: Context, globalHours: Int) {
        val dataDir = ctx.filesDir.absolutePath
        val subs = SubscriptionRepository.state.value.subs
        val now = System.currentTimeMillis()
        for (sub in subs) {
            val intervalMs = effectiveIntervalMs(sub, globalHours)
            val elapsed = now - sub.lastFetchedAt
            if (elapsed < intervalMs) continue
            try {
                refreshOne(sub, dataDir)
            } catch (t: Throwable) {
                Log.w(TAG, "refresh failed for ${sub.id}", t)
            }
        }
    }

    /**
     * Compute the auto-refresh interval for [sub] in milliseconds. Per-sub
     * `customRefreshIntervalMinutes` wins; falling back to the global hours
     * setting. Coerced to ≥ 1 minute so a corrupted value can't flatline
     * to an immediate-refresh loop.
     */
    private fun effectiveIntervalMs(sub: Subscription, globalHours: Int): Long {
        val mins = if (sub.customRefreshIntervalMinutes > 0)
            sub.customRefreshIntervalMinutes
        else
            globalHours * 60
        return mins.coerceAtLeast(1) * 60_000L
    }

    /**
     * Re-fetch a single subscription. Replaces its profiles in
     * [ProfileRepository] preserving favourites by name, and updates
     * [SubscriptionRepository] with the new title / userInfo / timestamp.
     */
    suspend fun refreshOne(sub: Subscription, dataDir: String) {
        val responseJson = withContext(Dispatchers.IO) {
            Mobile.fetchSubscriptionV3(
                sub.url,
                dataDir,
                BuildOptionsBuilder.currentSubscriptionFetchOptionsJson(),
            )
        }
        val response = JSONObject(responseJson)
        val arr = response.optJSONArray("entries") ?: JSONArray()

        val existingFavouriteNames = ProfileRepository.favouriteNamesFor(sub.id)

        val fresh = (0 until arr.length()).mapNotNull { i ->
            val o = arr.getJSONObject(i)
            val type = o.optString("type")
            val name = o.optString("name").ifBlank { "Profile ${i + 1}" }
            val isSection = type.equals("SECTION", ignoreCase = true)
            when {
                isSection -> Profile.section(name = name, subscriptionId = sub.id)
                else -> {
                    val uri = o.optString("uri")
                    val base = if (uri.isNotBlank())
                        Profile.fromUri(name, uri, subscriptionId = sub.id)
                    else
                        Profile.fromEntryJson(name, o.toString(), subscriptionId = sub.id)
                    base.copy(isFavorite = base.name in existingFavouriteNames)
                }
            }
        }
        ProfileRepository.replaceForSubscription(sub.id, fresh)
        // Ping only the servers that just changed, not the whole app's
        // profile list — keeps this as fast/streaming as a manual per-row
        // ping instead of getting diluted into a full-app sweep.
        PingRepository.refreshAll(fresh)
        // Re-apply panel metadata. supportUrl tracks the Support-Url header
        // verbatim — empty in the response clears any stale value, so the
        // edit sheet's Links section stays in sync with what the panel
        // currently advertises.
        SubscriptionRepository.update(sub.id) {
            it.copy(
                title = response.optString("title").ifBlank { it.title },
                userInfo = response.optString("userInfo"),
                supportUrl = response.optString("supportUrl"),
                lastFetchedAt = System.currentTimeMillis(),
            )
        }
    }
}
