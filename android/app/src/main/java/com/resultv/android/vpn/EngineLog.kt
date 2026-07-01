package com.resultv.android.vpn

import java.util.concurrent.ConcurrentHashMap

/**
 * Turns the raw libbox log stream (delivered one plain string at a time via
 * `CommandServerHandler.writeDebugMessage`) into curated [AppLog] entries.
 *
 * Android can't see individual connections the way the desktop can — sing-box
 * runs inside libbox, so this text stream is the only window into engine
 * activity. We keep it readable by mirroring what the desktop surfaces
 * (`internal/proxy/singbox.go`): warnings/errors, plus one line per unique
 * destination host (the desktop's `[CONN]` behavior), and drop routine noise
 * (DNS queries, routing internals).
 *
 * The message is a pre-formatted sing-box line with no separate level field,
 * so classification is by keyword. The per-session host set dedups so a page
 * that opens fifty sockets to the same host logs once.
 */
object EngineLog {

    private const val SEEN_CAP = 1024
    private val seenHosts = ConcurrentHashMap.newKeySet<String>()

    private val errorToken = Regex("""\b(error|fatal|panic)\b""", RegexOption.IGNORE_CASE)
    private val warnToken = Regex("""\bwarn""", RegexOption.IGNORE_CASE)
    // sing-box outbound line: "… outbound connection to youtube.com:443".
    private val destRegex = Regex("""connection to ([^\s:]+)""", RegexOption.IGNORE_CASE)

    /** Reset per-session dedup state. Called on each engine start/stop. */
    fun resetSession() = seenHosts.clear()

    /** Route one raw engine line into [AppLog], or drop it as noise. */
    fun ingest(raw: String?) {
        val msg = raw?.trim().orEmpty()
        if (msg.isEmpty()) return

        when {
            errorToken.containsMatchIn(msg) -> AppLog.error(msg, source = ENGINE)
            warnToken.containsMatchIn(msg) -> AppLog.warning(msg, source = ENGINE)
            else -> {
                val host = destRegex.find(msg)?.groupValues?.getOrNull(1)?.takeIf { it.isNotBlank() }
                    ?: return
                // First time we see this host this session → log it once.
                if (seenHosts.size < SEEN_CAP && seenHosts.add(host)) {
                    AppLog.info(host, source = CONN)
                }
            }
        }
    }

    private const val ENGINE = "ENGINE"
    private const val CONN = "CONN"
}
