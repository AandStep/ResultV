package com.resultv.android.vpn

/**
 * Pure, IO-free dedup + formatting for the connection log, mirroring the
 * desktop tracker's one-line-per-host output
 * (`internal/proxy/singbox.go`: "[CONN] domain -> dest | via out | status: connected").
 *
 * Kept free of libbox types so it can be unit-tested on the JVM;
 * [ConnectionWatcher] feeds it the fields it reads off each libbox `Connection`.
 *
 * Dedup is by domain for the lifetime of a connection session (reset only on
 * engine start via [reset], NOT on the stream's own reset flag). That makes the
 * output identical whether libbox pushes full snapshots each tick or incremental
 * deltas: a host is logged the first time it appears and never again until the
 * next VPN session — matching the desktop's per-host behaviour and avoiding a
 * per-tick flood that would evict the 500-entry [AppLog] buffer.
 */
class ConnectionLogState(private val cap: Int = 1024) {

    private val seen = HashSet<String>()

    /** Clear per-session dedup state. Called on each engine start. */
    fun reset() = seen.clear()

    /**
     * Returns the log line for a newly-seen [domain], or null when the
     * connection should be skipped: blank domain, already logged this session,
     * or the dedup cap was hit (bounds memory and log volume under churn).
     */
    fun onConnection(domain: String, destination: String, outbound: String): String? {
        val host = domain.trim()
        if (host.isEmpty()) return null
        if (seen.size >= cap) return null
        if (!seen.add(host)) return null

        val dest = destination.trim()
        val destPart = if (dest.isEmpty()) "" else " -> $dest"
        val out = outbound.trim()
        val viaPart = if (out.isEmpty() || out == "direct") "" else " | via $out"
        return "$host$destPart$viaPart | status: connected"
    }
}
