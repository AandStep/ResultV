package com.resultv.android.vpn

/** Package + human label of one installed app. Android-free so the matcher
 *  is JVM-testable (the unit-test source set has no Robolectric). */
data class AppMeta(val packageName: String, val label: String)

/**
 * Decides which installed apps are "associated with a blocked resource" and
 * should therefore ride the VPN in Smart mode. Pure: no Context, no I/O.
 *
 * Matching is deliberately heuristic — the analogue of the domain blocklist
 * at package granularity, where no canonical list exists. Recall boosters:
 * package segments, human label, and a small alias table for rebrands /
 * short names the ≥3-char brand rule can't reach. Misses are recoverable via
 * the manual "в VPN" list; false positives only over-tunnel an app.
 */
object SmartAppMatcher {

    /** Rebrands / short names where neither package nor label yields a
     *  ≥3-char brand present in the list. Package → a domain in the blocklist. */
    val DEFAULT_ALIASES: Map<String, String> = mapOf(
        "com.twitter.android" to "x.com",
        "com.zhiliaoapp.musically" to "tiktok.com",
        "com.zhiliaoapp.musically.go" to "tiktok.com",
        "org.telegram.messenger" to "t.me",
        "org.telegram.messenger.web" to "t.me",
        "org.thunderdog.challegram" to "t.me",
    )

    // Structural / vendor package segments that are never brands.
    private val STOP_SEGMENTS = setOf(
        "com", "org", "net", "io", "co", "app", "apps", "android",
        "mobile", "client", "free", "pro", "www", "the",
    )

    /** Second-level labels (≥3 chars) of the blocklist, e.g. youtube.com → "youtube". */
    fun brandsFrom(domains: Collection<String>): Set<String> {
        val out = HashSet<String>()
        for (raw in domains) {
            val host = raw.trim().lowercase().trimStart('.')
            if (host.isEmpty()) continue
            val labels = host.split('.')
            if (labels.size < 2) continue
            val sld = labels[labels.size - 2]
            if (sld.length >= 3) out.add(sld)
        }
        return out
    }

    fun matchedPackages(
        apps: List<AppMeta>,
        blockedDomains: Collection<String>,
        aliases: Map<String, String> = DEFAULT_ALIASES,
    ): Set<String> {
        val brands = brandsFrom(blockedDomains)
        val blockedSet = blockedDomains.mapTo(HashSet()) { it.trim().lowercase().trimStart('.') }
        val out = HashSet<String>()
        for (app in apps) {
            if (matches(app, brands, blockedSet, aliases)) out.add(app.packageName)
        }
        return out
    }

    private fun matches(
        app: AppMeta,
        brands: Set<String>,
        blockedDomains: Set<String>,
        aliases: Map<String, String>,
    ): Boolean {
        aliases[app.packageName]?.let { domain ->
            if (domain.trim().lowercase().trimStart('.') in blockedDomains) return true
        }
        for (tok in candidateTokens(app)) {
            if (tok in brands) return true
        }
        return false
    }

    private fun candidateTokens(app: AppMeta): Set<String> {
        val out = HashSet<String>()
        for (seg in app.packageName.lowercase().split('.')) {
            if (seg.length >= 3 && seg !in STOP_SEGMENTS) out.add(seg)
        }
        for (word in app.label.lowercase().split(Regex("[^a-z0-9]+"))) {
            if (word.length >= 3 && word !in STOP_SEGMENTS) out.add(word)
        }
        return out
    }
}
