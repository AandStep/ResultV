package com.resultv.android.vpn

import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.util.Log

private const val TAG = "ResultV/AppInventory"

/**
 * Thin PackageManager adapter for tunnel-membership computation. Kept Context-
 * bound and free of routing logic so SmartAppMatcher / AppTunnelMembership stay
 * pure and JVM-testable.
 */
object AppInventory {

    /**
     * Installed app package names (excluding our own). Deliberately does NOT
     * resolve labels: [SmartAppMatcher] keys on the package's reverse-DNS
     * domain, and getApplicationLabel loads each app's resources — hundreds of
     * those on the openTun hot path was the 3-5s Smart connect stall.
     */
    fun installedApps(ctx: Context): List<String> {
        val own = ctx.packageName
        return ctx.packageManager
            .getInstalledApplications(0)
            .asSequence()
            .map { it.packageName }
            .filter { it != own }
            .toList()
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
        val intent = Intent(Intent.ACTION_VIEW, Uri.parse("http://"))
            .addCategory(Intent.CATEGORY_BROWSABLE)
        return try {
            ctx.packageManager
                .queryIntentActivities(intent, PackageManager.MATCH_DEFAULT_ONLY)
                .mapNotNull { it.activityInfo?.packageName }
                .filter { it != ctx.packageName }
                .toSet()
        } catch (t: Throwable) {
            Log.w(TAG, "browser query failed", t)
            emptySet()
        }
    }
}
