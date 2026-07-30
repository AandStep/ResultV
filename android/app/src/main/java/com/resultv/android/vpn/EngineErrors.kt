package com.resultv.android.vpn

/**
 * Recognises engine (sing-box) startup failures that self-healing recovery can
 * handle, and names the on-disk artifacts recovery removes.
 */
object EngineErrors {

    /**
     * sing-box's cache_file, relative to the app's filesDir. MUST stay in sync
     * with internal/proxy/engine.go:
     *   filepath.Join(dataDir, "sing-box-cache.db")
     */
    const val SINGBOX_CACHE_FILE = "sing-box-cache.db"

    /**
     * True when [msg] is sing-box failing to restore a *corrupt cached
     * rule-set* — e.g. a truncated ad-list download whose zlib stream no longer
     * matches its checksum:
     *
     *   initialize rule-set[1]: restore cached rule-set:
     *       read rule[0] zlib invalid checksum
     *
     * That state is sticky: once the poisoned blob is in sing-box-cache.db every
     * connect fails identically. Deleting the cache and retrying recovers it.
     *
     * Matching is deliberately narrow — "invalid checksum" alone (TLS, etc.) is
     * not enough; the error must be about a rule-set.
     */
    fun isCorruptRuleSetCacheError(msg: String?): Boolean {
        if (msg.isNullOrEmpty()) return false
        val m = msg.lowercase()
        if ("restore cached rule-set" in m) return true
        return "rule-set" in m && ("invalid checksum" in m || "zlib" in m)
    }
}
