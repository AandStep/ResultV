package com.resultv.android.vpn

import android.util.Log
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import libbox.CommandClient
import libbox.CommandClientHandler
import libbox.CommandClientOptions
import libbox.ConnectionEvents
import libbox.Libbox
import libbox.LogIterator
import libbox.OutboundGroupIterator
import libbox.StatusMessage
import libbox.StringIterator

private const val TAG = "ResultV/KillSwitch"
private const val GROUP_TAG = "ks-test"
private const val TICK_MS = 5_000L

/**
 * Monitors proxy health from inside the engine and engages/disengages the
 * kill switch. Detection cannot use an app-level socket (our package is
 * excluded from the tun), so it drives sing-box's urltest group "ks-test"
 * via a libbox CommandClient and reads the resulting URLTestDelay.
 *
 * [onEngage]/[onDisengage] run on the watchdog's coroutine (Default
 * dispatcher); the service hops to its worker for the actual BoxModule.reload.
 */
class KillSwitchWatchdog(
    private val onEngage: () -> Unit,
    private val onDisengage: () -> Unit,
) {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    private val decider = KillSwitchDecider()
    private var client: CommandClient? = null
    private var tickJob: Job? = null

    @Volatile private var lastDelayMs: Int = -1
    private var lastTrafficTotal = 0L

    fun start() {
        val opts = CommandClientOptions().apply {
            statusInterval = 1_000_000_000L // 1s, ns (Go time.Duration)
            addCommand(Libbox.CommandGroup)
        }
        val c = CommandClient(GroupHandler { delay -> lastDelayMs = delay }, opts)
        try {
            c.connect()
            client = c
        } catch (t: Throwable) {
            Log.w(TAG, "group CommandClient connect failed", t)
            return
        }
        tickJob = scope.launch {
            while (isActive) {
                delay(TICK_MS)
                tick()
            }
        }
        Log.i(TAG, "watchdog started")
    }

    private fun tick() {
        // Ask the engine to (re)probe the proxy this interval.
        try {
            client?.urlTest(GROUP_TAG)
        } catch (t: Throwable) {
            Log.w(TAG, "urlTest threw", t)
        }
        val snap = TrafficStats.snapshot.value
        val total = snap.downloadBytes + snap.uploadBytes
        val delta = (total - lastTrafficTotal).coerceAtLeast(0)
        lastTrafficTotal = total

        when (decider.onTick(lastDelayMs, delta)) {
            KsAction.ENGAGE -> {
                Log.w(TAG, "proxy dead → ENGAGE kill switch")
                onEngage()
            }
            KsAction.DISENGAGE -> {
                Log.i(TAG, "proxy recovered → DISENGAGE kill switch")
                onDisengage()
            }
            KsAction.NONE -> {}
        }
        // Reset the reading so a missing writeGroups next tick counts as a fail.
        lastDelayMs = -1
    }

    fun stop() {
        tickJob?.cancel(); tickJob = null
        try { client?.disconnect() } catch (_: Throwable) {}
        client = null
        scope.cancel()
        Log.i(TAG, "watchdog stopped")
    }

    val isEngaged: Boolean get() = decider.isEngaged
}

/** Reads the ks-test group's selected item delay out of writeGroups. */
private class GroupHandler(private val onDelay: (Int) -> Unit) : CommandClientHandler {
    override fun writeGroups(message: OutboundGroupIterator?) {
        if (message == null) return
        while (message.hasNext()) {
            val g = message.next()
            if (g.tag != GROUP_TAG) continue
            val items = g.items
            var delay = 0
            while (items.hasNext()) {
                val item = items.next()
                // Single-member group; take the (only) member's delay.
                delay = item.urlTestDelay
            }
            onDelay(delay)
        }
    }

    override fun connected() {}
    override fun disconnected(message: String?) {}
    override fun clearLogs() {}
    override fun initializeClashMode(modeList: StringIterator?, currentMode: String?) {}
    override fun setDefaultLogLevel(level: Int) {}
    override fun updateClashMode(newMode: String?) {}
    override fun writeConnectionEvents(events: ConnectionEvents?) {}
    override fun writeLogs(messageList: LogIterator?) {}
    override fun writeStatus(message: StatusMessage?) {}
}
