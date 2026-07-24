package com.resultv.android.vpn

/**
 * Decides which installed apps are "associated with a blocked resource" and
 * should therefore ride the VPN in Smart mode. Pure: no Context, no I/O.
 *
 * Precision over recall, by design. The membership it feeds excludes every
 * unmatched app from the tunnel, so a FALSE POSITIVE is actively harmful — a
 * VPN-hostile app (bank, gov, payments) wrongly pulled in would break, the very
 * thing Smart mode exists to prevent. Misses are cheap: the user adds them via
 * the manual "в VPN" list.
 *
 * Matching is therefore strict:
 * - the app's reverse-DNS registrable domain (com.instagram.android →
 *   instagram.com) must be a blocked domain, OR
 * - a curated alias maps the package to a blocked domain (for the few apps
 *   whose package vendor ≠ service brand: YouTube under com.google.*, TikTok
 *   under com.zhiliaoapp.*).
 *
 * An earlier token-overlap heuristic was removed: against the real ~86k-domain
 * RU list it matched whole vendor families (a single blocked aistudio.google.com
 * pulled every com.google.* app) and common-word apps. Reverse-DNS keys on the
 * exact registrable domain, which for that list is blocked ONLY for the real
 * targets (instagram/youtube/tiktok/x/facebook/telegram), not vendor domains
 * (google.com/yandex.ru/ozon.ru/tinkoff.ru are absent).
 */
object SmartAppMatcher {

    /**
     * Packages whose reverse-DNS registrable domain does NOT equal their blocked
     * service domain. Package → a domain that appears in the blocklist.
     */
    val DEFAULT_ALIASES: Map<String, String> = mapOf(
        "com.google.android.youtube" to "youtube.com",
        "com.google.android.youtube.tv" to "youtube.com",
        "com.google.android.apps.youtube.music" to "youtube.com",
        "com.zhiliaoapp.musically" to "tiktok.com",
        "com.zhiliaoapp.musically.go" to "tiktok.com",
        "com.ss.android.ugc.trill" to "tiktok.com",
        "org.thunderdog.challegram" to "t.me",
    )

    /**
     * The registrable domain implied by a reverse-DNS package name:
     * `com.instagram.android` → `instagram.com`, `ru.ozon.app.android` →
     * `ozon.ru`. Returns null for packages with fewer than two labels.
     *
     * Deliberately naive (first two labels = TLD.SLD): it does not consult a
     * public-suffix list, so a package like `com.co.uk.app` yields `co.uk`.
     * That only risks a rare false positive if such a suffix were itself a
     * blocked entry, which the RU list does not contain — acceptable for a
     * membership hint the user can override.
     */
    fun registrableDomain(pkg: String): String? {
        val labels = pkg.trim().lowercase().split('.')
        if (labels.size < 2) return null
        val tld = labels[0]
        val sld = labels[1]
        if (tld.isEmpty() || sld.isEmpty()) return null
        return "$sld.$tld"
    }

    fun matchedPackages(
        packages: Collection<String>,
        blockedDomains: Collection<String>,
        aliases: Map<String, String> = DEFAULT_ALIASES,
    ): Set<String> {
        val blocked = blockedDomains.mapTo(HashSet()) { it.trim().lowercase().trimStart('.') }
        val out = HashSet<String>()
        for (pkg in packages) {
            val alias = aliases[pkg]
            if (alias != null && alias in blocked) {
                out.add(pkg)
                continue
            }
            val reg = registrableDomain(pkg)
            if (reg != null && reg in blocked) out.add(pkg)
        }
        return out
    }
}
