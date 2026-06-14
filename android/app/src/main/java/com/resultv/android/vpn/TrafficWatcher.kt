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

private const val TAG = "ResultV/Traffic"

// libbox's CommandClientOptions.StatusInterval is an int64 of NANOSECONDS
// (Go time.Duration under the hood — see sing-box daemon/started_service.go
// SubscribeStatus: `interval := time.Duration(request.Interval)`).
// The server uses this as its ticker AND derives Uplink/Downlink as
// (UplinkTotal - lastUplinkTotal) per tick, so when the value is wrong the
// reported "bps" silently becomes "bytes per tick" rather than per second.
// 1_000_000_000 ns = 1 s — one sample per second, matches desktop cadence.
private const val STATUS_INTERVAL_NS = 1_000_000_000L

/**
 * Subscribes to sing-box's internal status command via libbox's
 * `CommandClient` and pushes uplink/downlink samples into [TrafficStats]
 * once per second. Lives next to the running CommandServer (same Unix
 * socket under basePath/command.sock); attaches on [start], detaches on
 * [stop]. Safe to start/stop across multiple VPN sessions.
 */
object TrafficWatcher {
    @Volatile private var client: CommandClient? = null

    @Synchronized
    fun start() {
        if (client != null) {
            Log.d(TAG, "already running")
            return
        }
        val opts = CommandClientOptions().apply {
            statusInterval = STATUS_INTERVAL_NS
            addCommand(Libbox.CommandStatus)
        }
        val handler = StatusHandler()
        val c = CommandClient(handler, opts)
        try {
            c.connect()
            client = c
            TrafficStats.reset()
            Log.i(TAG, "connected to libbox status stream")
        } catch (t: Throwable) {
            Log.w(TAG, "CommandClient.connect failed", t)
            // Try to clean up the half-built client so we don't leak the ref.
            try {
                c.disconnect()
            } catch (_: Throwable) {
                // ignore
            }
        }
    }

    @Synchronized
    fun stop() {
        val c = client ?: return
        client = null
        // Persist session totals before dropping the connection so they
        // survive service restarts and show correct cumulative usage in Settings.
        val snap = TrafficStats.snapshot.value
        DataUsageRepository.addSession(snap.downloadBytes, snap.uploadBytes)
        try {
            c.disconnect()
        } catch (t: Throwable) {
            Log.w(TAG, "CommandClient.disconnect threw", t)
        }
        Log.i(TAG, "disconnected")
    }
}

/**
 * Bridges sing-box StatusMessage callbacks → TrafficStats StateFlow.
 * All other CommandClientHandler methods are inert — we only care about
 * the status stream right now (logs / groups / connections may land
 * later in their own watchers if needed).
 */
private class StatusHandler : CommandClientHandler {
    // Keep last totals so we can derive per-second rates if sing-box ever
    // stops populating uplink/downlink and we need a fallback. For now
    // sing-box emits both already-computed rates and totals — use rates.
    @Volatile private var lastUplinkTotal = 0L
    @Volatile private var lastDownlinkTotal = 0L

    override fun connected() {
        Log.d(TAG, "status stream connected")
    }

    override fun disconnected(message: String?) {
        Log.d(TAG, "status stream disconnected: $message")
    }

    override fun writeStatus(message: StatusMessage?) {
        if (message == null) return
        if (!message.trafficAvailable) {
            // clash_api isn't fully wired yet — nothing to publish. Keep
            // last snapshot so the UI doesn't flicker to zero.
            return
        }
        val uplink = if (message.uplink < 0L) 0L else message.uplink
        val downlink = if (message.downlink < 0L) 0L else message.downlink
        val uplinkTotal = message.uplinkTotal
        val downlinkTotal = message.downlinkTotal
        lastUplinkTotal = uplinkTotal
        lastDownlinkTotal = downlinkTotal

        // Append one sample to the rolling history; drop oldest when full.
        val prev = TrafficStats.snapshot.value
        val newDownloadHistory =
            (prev.downloadHistory + downlink).takeLast(TRAFFIC_HISTORY_SIZE)
        val newUploadHistory =
            (prev.uploadHistory + uplink).takeLast(TRAFFIC_HISTORY_SIZE)
        TrafficStats.publish(
            TrafficSnapshot(
                downloadBytes = downlinkTotal,
                uploadBytes = uplinkTotal,
                downloadBps = downlink,
                uploadBps = uplink,
                downloadHistory = newDownloadHistory,
                uploadHistory = newUploadHistory,
            )
        )
    }

    override fun clearLogs() {}
    override fun initializeClashMode(modeList: StringIterator?, currentMode: String?) {}
    override fun setDefaultLogLevel(level: Int) {}
    override fun updateClashMode(newMode: String?) {}
    override fun writeConnectionEvents(events: ConnectionEvents?) {}
    override fun writeGroups(message: OutboundGroupIterator?) {}
    override fun writeLogs(messageList: LogIterator?) {}
}
