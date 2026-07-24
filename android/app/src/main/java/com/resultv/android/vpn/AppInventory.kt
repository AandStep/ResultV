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

    /** Installed apps as (package, label). Excludes our own package. */
    fun installedApps(ctx: Context): List<AppMeta> {
        val pm = ctx.packageManager
        val own = ctx.packageName
        return pm.getInstalledApplications(PackageManager.GET_META_DATA)
            .asSequence()
            .filter { it.packageName != own }
            .map { AppMeta(it.packageName, pm.getApplicationLabel(it).toString()) }
            .toList()
    }

    /**
     * Packages that can handle a plain http(s) VIEW intent — i.e. browsers.
     * They must ride the VPN so arbitrary blocked sites open. Best-effort:
     * a query failure yields an empty set (matcher/manual list still apply).
     */
    fun browserPackages(ctx: Context): Set<String> {
        val intent = Intent(Intent.ACTION_VIEW, Uri.parse("http://example.com"))
            .addCategory(Intent.CATEGORY_BROWSABLE)
        return try {
            ctx.packageManager
                .queryIntentActivities(intent, PackageManager.MATCH_ALL)
                .mapNotNull { it.activityInfo?.packageName }
                .filter { it != ctx.packageName }
                .toSet()
        } catch (t: Throwable) {
            Log.w(TAG, "browser query failed", t)
            emptySet()
        }
    }
}
