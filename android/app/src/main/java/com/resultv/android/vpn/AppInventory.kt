package com.resultv.android.vpn

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.content.pm.ApplicationInfo
import android.content.pm.PackageManager
import android.net.Uri
import android.util.Log
import androidx.core.content.ContextCompat

private const val TAG = "ResultV/AppInventory"

/**
 * Thin PackageManager adapter for tunnel-membership computation. Kept Context-
 * bound and free of routing logic so SmartAppMembership / AppTunnelMembership
 * stay pure and JVM-testable.
 *
 * Results are memoized to keep PackageManager queries off the reconnect path.
 * [init] invalidates the cache when installed packages or components change.
 */
object AppInventory {

    @Volatile private var cachedSmartCandidates: List<String>? = null
    @Volatile private var cachedBrowsers: Set<String>? = null
    @Volatile private var receiverRegistered = false

    /**
     * Registers a package-add/remove receiver so the memoized caches below
     * can never outlive their validity for the life of the process.
     *
     * Without this, a user installing an app associated with a blocked
     * domain while Smart mode's VPN service is running would have that app
     * silently routed OUTSIDE the tunnel — unprotected — until the process
     * restarts: ResultVpnService has no `android:process`, so it shares the
     * JVM with MainActivity and can outlive it by a long margin as a
     * foreground service.
     *
     * Idempotent and safe to call from multiple entry points — mirrors every
     * other `*Repository.init(ctx)` in this codebase (SmartListRepository,
     * SettingsRepository, ...). Call from both MainActivity.onCreate and
     * ResultVpnService.ensureReposReady: the always-on-VPN path starts the
     * service directly and never runs MainActivity.
     *
     * Registered against applicationContext and deliberately NEVER
     * unregistered: the caches are scoped to the process, so the receiver
     * must be too. Tying registration/unregistration to an Activity or to
     * the VpnService's onCreate/onDestroy would reopen the same bug for
     * installs that land while disconnected — the receiver would be torn
     * down exactly when it's needed to keep the cache honest for the NEXT
     * connect. There's no Application subclass in this app to hang a single
     * process-wide registration off of, so AppInventory does it itself.
     *
     * Registration failure is caught and logged, never thrown: worst case we
     * are back to today's staleness, which must not block app startup.
     */
    @Synchronized
    fun init(ctx: Context) {
        if (receiverRegistered) return
        try {
            val filter = IntentFilter().apply {
                addAction(Intent.ACTION_PACKAGE_ADDED)
                addAction(Intent.ACTION_PACKAGE_CHANGED)
                addAction(Intent.ACTION_PACKAGE_REMOVED)
                addAction(Intent.ACTION_PACKAGE_FULLY_REMOVED)
                addDataScheme("package")
            }
            ContextCompat.registerReceiver(
                ctx.applicationContext,
                object : BroadcastReceiver() {
                    override fun onReceive(context: Context, intent: Intent) {
                        Log.i(TAG, "package change (${intent.action}) — invalidating app cache")
                        invalidate()
                    }
                },
                filter,
                ContextCompat.RECEIVER_NOT_EXPORTED,
            )
            // Only mark registered on success: setting this unconditionally
            // before the try block meant a thrown registerReceiver call left
            // receiverRegistered stuck true, so no later init() call would
            // ever retry — package-change invalidation would be dead for the
            // rest of the process's life instead of just until the next call.
            receiverRegistered = true
        } catch (t: Throwable) {
            // Fail-safe: no receiver just means the caches can go stale again,
            // exactly as before this fix. Never break startup for this, and
            // leave receiverRegistered=false so a later init() call can retry.
            Log.w(TAG, "package-change receiver registration failed", t)
        }
    }

    /** Drop the memoized lists (call on package install/uninstall). */
    @Synchronized
    fun invalidate() {
        cachedSmartCandidates = null
        cachedBrowsers = null
    }

    /**
     * Packages eligible for Smart's domain-based detection. A system utility
     * with no launcher entry must not inherit its manufacturer's web block.
     * Browsers and manually selected packages are handled separately.
     */
    fun smartDomainCandidates(ctx: Context): List<String> {
        cachedSmartCandidates?.let { return it }
        val own = ctx.packageName
        val pm = ctx.packageManager
        val launcher = Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_LAUNCHER)
        val launchablePackages = pm.queryIntentActivities(launcher, 0)
            .mapNotNull { it.activityInfo?.packageName }
            .toSet()
        val systemFlags = ApplicationInfo.FLAG_SYSTEM or ApplicationInfo.FLAG_UPDATED_SYSTEM_APP
        val apps = pm.getInstalledApplications(0)
            .asSequence()
            .filter { app ->
                app.packageName != own &&
                    ((app.flags and systemFlags) == 0 || app.packageName in launchablePackages)
            }
            .map { it.packageName }
            .toList()
        Log.i(TAG, "Smart domain candidates: ${apps.size}")
        cachedSmartCandidates = apps
        return apps
    }

    /**
     * Real web browsers only — apps that handle an arbitrary http URL.
     *
     * The probe URI is host-LESS (`http://`) on purpose: a browser accepts any
     * web URL, while a deep-link app scopes its intent filter to a concrete host
     * (ozon.ru, wildberries.ru …) and will not resolve a hostless one. Combined
     * with MATCH_DEFAULT_ONLY this excludes the link-handling apps that a
     * `http://example.com` + MATCH_ALL query wrongly pulled in (Ozon, WB, mail,
     * banks — which then leaked into the tunnel as "browsers"). Best-effort: a
     * query failure yields an empty set.
     */
    fun browserPackages(ctx: Context): Set<String> {
        cachedBrowsers?.let { return it }
        val intent = Intent(Intent.ACTION_VIEW, Uri.parse("http://"))
            .addCategory(Intent.CATEGORY_BROWSABLE)
        val found = try {
            ctx.packageManager
                .queryIntentActivities(intent, PackageManager.MATCH_DEFAULT_ONLY)
                .mapNotNull { it.activityInfo?.packageName }
                .filter { it != ctx.packageName }
                .toSet()
        } catch (t: Throwable) {
            Log.w(TAG, "browser query failed", t)
            emptySet()
        }
        cachedBrowsers = found
        return found
    }
}
