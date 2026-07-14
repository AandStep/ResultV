package com.resultv.android.vpn

import android.util.Log
import libbox.CommandClient
import libbox.CommandClientHandler
import libbox.CommandClientOptions
import libbox.ConnectionEvents
import libbox.Libbox
import libbox.LogIterator
import libbox.OutboundGroupIterator
import libbox.StatusMessage
import libbox.StringIterator

private const val TAG = "ResultV/Conn"
private const val CONN_SOURCE = "CONN"

// libbox's status interval is int64 NANOSECONDS (Go time.Duration). The
// connections command reuses the same interval as its push cadence; 1s matches
// the desktop's tracker granularity and the TrafficWatcher cadence.
private const val CONN_INTERVAL_NS = 1_000_000_000L

/**
 * Subscribes to sing-box's structured connection stream via libbox's
 * `CommandClient` (`Libbox.CommandConnections`) and surfaces one curated
 * `[CONN]` line per unique destination host — the mobile equivalent of the
 * desktop's sing-box Router tracker (`internal/proxy/singbox.go`).
 *
 * This replaces the old text-scraping in [EngineLog], which could only see a
 * bare token after "connection to" (dropping the port, and showing a raw IP
 * whenever the engine dialed by address). The structured `Connection` carries
 * the SNI domain, resolved destination, and outbound tag, so the line reads
 * `domain -> ip:port | via out | status: connected` like the desktop.
 *
 * Best-effort: if the connection stream can't be established (older engine, or
 * the tracker isn't wired), we log a warning and leave [EngineLog]'s
 * warning/error surfacing intact — the VPN is never affected.
 */
object ConnectionWatcher {
    @Volatile private var client: CommandClient? = null
    private val state = ConnectionLogState()

    @Synchronized
    fun start() {
        if (client != null) {
            Log.d(TAG, "already running")
            return
        }
        // Fresh session — let each host log once again.
        state.reset()
        val opts = CommandClientOptions().apply {
            statusInterval = CONN_INTERVAL_NS
            addCommand(Libbox.CommandConnections)
        }
        val c = CommandClient(ConnectionHandler(state), opts)
        try {
            c.connect()
            client = c
            Log.i(TAG, "connected to libbox connections stream")
        } catch (t: Throwable) {
            Log.w(TAG, "connections CommandClient.connect failed", t)
            try { c.disconnect() } catch (_: Throwable) {}
        }
    }

    @Synchronized
    fun stop() {
        val c = client ?: return
        client = null
        try {
            c.disconnect()
        } catch (t: Throwable) {
            Log.w(TAG, "CommandClient.disconnect threw", t)
        }
        Log.i(TAG, "disconnected")
    }
}

/**
 * Bridges libbox connection events → curated [AppLog] `[CONN]` lines. Only
 * [writeConnectionEvents] does work; the rest are inert (this client subscribes
 * to CommandConnections only). Dedup and formatting live in [ConnectionLogState]
 * so this handler stays a thin adapter over the libbox types.
 */
private class ConnectionHandler(private val state: ConnectionLogState) : CommandClientHandler {

    override fun writeConnectionEvents(events: ConnectionEvents?) {
        if (events == null) return
        val it = events.iterator()
        while (it.hasNext()) {
            val ev = it.next() ?: continue
            val conn = ev.connection ?: continue
            val line = state.onConnection(
                conn.domain.orEmpty(),
                conn.destination.orEmpty(),
                conn.outbound.orEmpty(),
            ) ?: continue
            AppLog.info(line, source = CONN_SOURCE)
        }
    }

    override fun connected() {}
    override fun disconnected(message: String?) {
        Log.d(TAG, "connections stream disconnected: $message")
    }
    override fun clearLogs() {}
    override fun initializeClashMode(modeList: StringIterator?, currentMode: String?) {}
    override fun setDefaultLogLevel(level: Int) {}
    override fun updateClashMode(newMode: String?) {}
    override fun writeGroups(message: OutboundGroupIterator?) {}
    override fun writeLogs(messageList: LogIterator?) {}
    override fun writeStatus(message: StatusMessage?) {}
}
