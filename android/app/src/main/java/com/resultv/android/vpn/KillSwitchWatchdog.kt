package com.resultv.android.vpn

import android.util.Log
import com.resultv.android.R
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
private const val PROXY_TAG = "proxy"
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
    private var scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    private val decider = KillSwitchDecider()
    private var client: CommandClient? = null
    private var tickJob: Job? = null

    @Volatile private var lastDelayMs: Int = -1
    private var lastTrafficTotal = 0L

    fun start() {
        if (client != null) return
        if (!scope.isActive) scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
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
            AppLog.warning(
                R.string.log_ks_monitor_failed,
                t.message ?: t.javaClass.simpleName,
                source = AppLog.resolve(R.string.log_source_killswitch),
            )
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

        val tick = decider.onTick(lastDelayMs, delta)
        when (tick.action) {
            KsAction.ENGAGE -> {
                Log.w(TAG, "proxy dead → ENGAGE kill switch")
                onEngage()
            }
            KsAction.DISENGAGE -> {
                Log.i(TAG, "proxy recovered → DISENGAGE kill switch")
                onDisengage()
            }
            KsAction.NONE -> {
                // Mirror the desktop's per-probe lines: each failed probe while
                // not yet engaged is user-visible with its (N/M) counter, and
                // the traffic veto is called out explicitly. ENGAGE/DISENGAGE
                // themselves are logged by the service (log_killswitch_*), so
                // no duplicate here.
                if (tick.failCount > 0 && !decider.isEngaged) {
                    val src = AppLog.resolve(R.string.log_source_killswitch)
                    if (tick.deferredByTraffic) {
                        AppLog.warning(R.string.log_ks_probe_deferred, source = src)
                    } else {
                        AppLog.warning(
                            R.string.log_ks_probe_failed,
                            tick.failCount, decider.failureThreshold,
                            source = src,
                        )
                    }
                }
            }
        }
        // Do NOT reset lastDelayMs here. The group command pushes ~every second
        // (server ticks on statusInterval plus urlTestUpdate), and on a real
        // outage sing-box DELETES the failed member's urltest history so the
        // pushed proxy delay drops to 0 on its own. Resetting to -1 each tick
        // would instead manufacture failures whenever a push simply hadn't
        // landed in a given 5 s window — the original false-engage bug. A dead
        // stream is handled by GroupHandler.disconnected → onDelay(0).
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

/** Reads the ks-test group's "proxy" member delay out of writeGroups. */
private class GroupHandler(private val onDelay: (Int) -> Unit) : CommandClientHandler {
    override fun writeGroups(message: OutboundGroupIterator?) {
        if (message == null) return
        while (message.hasNext()) {
            val g = message.next()
            if (g.tag != GROUP_TAG) continue
            // ks-test has two members (proxy + a block filler that only exists
            // so sing-box reports the group at all). Read the "proxy" item
            // specifically — the block item's delay is always 0 and taking the
            // last item would read it, masking a healthy proxy.
            val items = g.items
            var proxyDelay = 0
            while (items.hasNext()) {
                val item = items.next()
                if (item.tag == PROXY_TAG) proxyDelay = item.urlTestDelay
            }
            onDelay(proxyDelay)
        }
    }

    override fun connected() {}
    // Stream died: treat as a failed probe so a broken command channel can't
    // leave a stale positive delay wedged in and mask a real outage.
    override fun disconnected(message: String?) { onDelay(0) }
    override fun clearLogs() {}
    override fun initializeClashMode(modeList: StringIterator?, currentMode: String?) {}
    override fun setDefaultLogLevel(level: Int) {}
    override fun updateClashMode(newMode: String?) {}
    override fun writeConnectionEvents(events: ConnectionEvents?) {}
    override fun writeLogs(messageList: LogIterator?) {}
    override fun writeStatus(message: StatusMessage?) {}
}
