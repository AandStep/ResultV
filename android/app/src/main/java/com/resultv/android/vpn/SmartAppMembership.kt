package com.resultv.android.vpn

import android.util.Log
import mobile.Mobile

private const val TAG = "ResultV/SmartMembership"

/**
 * Smart tunnel membership, resolved by the engine.
 *
 * The blocklist lives in the on-disk rule-set, so matching happens in Go: we
 * send ~200 package names (~8 KB) and get the matched subset back. Kotlin used
 * to hold all ~150k domains and rebuild a HashSet on every connect.
 *
 * Matching rules (see internal/proxy/smart_apps.go) are unchanged: an app
 * matches when its reverse-DNS registrable domain is blocked, or a curated alias
 * maps it to a blocked domain.
 */
object SmartAppMembership {

    /** Blocked-associated packages among [packages]. Empty when no list is ready. */
    fun matchedPackages(dataDir: String, packages: Collection<String>): Set<String> {
        if (dataDir.isBlank() || packages.isEmpty()) return emptySet()
        return try {
            val csv = Mobile.matchSmartApps(packages.joinToString(","), dataDir)
            if (csv.isNullOrBlank()) emptySet()
            else csv.split(',').mapNotNull { it.trim().ifBlank { null } }.toSet()
        } catch (t: Throwable) {
            Log.w(TAG, "matchSmartApps failed", t)
            emptySet()
        }
    }
}
