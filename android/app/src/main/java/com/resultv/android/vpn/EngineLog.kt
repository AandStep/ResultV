package com.resultv.android.vpn

/**
 * Classifies the raw libbox log stream (delivered one plain string at a time via
 * `CommandServerHandler.writeDebugMessage`) into curated [AppLog] warnings and
 * errors — the mobile mirror of the desktop's `singBoxLogWriter`
 * (`internal/proxy/singbox.go`), which forwards only warn/error engine lines.
 *
 * Per-connection `[CONN]` lines are NOT derived here: they come from
 * [ConnectionWatcher], which reads sing-box's structured connection stream
 * (domain, resolved destination, outbound) instead of scraping this text log.
 * The text stream carries no port and shows a bare IP whenever the engine dials
 * by address, which made a poor connection log.
 *
 * The message is a pre-formatted sing-box line with no separate level field, so
 * classification is by keyword.
 */
object EngineLog {

    private val errorToken = Regex("""\b(error|fatal|panic)\b""", RegexOption.IGNORE_CASE)
    private val warnToken = Regex("""\bwarn""", RegexOption.IGNORE_CASE)

    /** Route one raw engine line into [AppLog] if it's a warning/error, else drop it. */
    fun ingest(raw: String?) {
        val msg = raw?.trim().orEmpty()
        if (msg.isEmpty()) return

        when {
            errorToken.containsMatchIn(msg) -> AppLog.error(msg, source = ENGINE)
            warnToken.containsMatchIn(msg) -> AppLog.warning(msg, source = ENGINE)
            // Routine engine chatter (routing internals, DNS queries, raw
            // per-socket connection lines) is dropped — connections are surfaced
            // by ConnectionWatcher from the structured stream.
        }
    }

    // Public: BoxModule tags its engine-lifecycle entries with the same source.
    const val ENGINE = "ENGINE"
}
