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
import mobile.Mobile
import org.json.JSONObject

private const val TAG = "ResultV/FilterWatchdog"
private const val TICK_MS = 15_000L

/**
 * Polls Mobile.filterStatus() while the browser MITM ad-block feature is
 * running and detects it dying mid-session — a case the one-shot startup
 * check in ResultVpnService.startBrowserAdBlockIfEnabled() cannot see,
 * since that check only runs once, before the tunnel comes up.
 *
 * Deliberately much simpler than KillSwitchWatchdog: no libbox
 * CommandClient probing needed, since filter.Manager.Status().Enabled
 * already reflects whether the Go-side MITM goroutine is alive. A single
 * enabled=true -> enabled=false transition while we still expect it to be
 * on is treated as a crash — the only code path that stops it on purpose
 * (ResultVpnService's disconnect/onDestroy) already flips
 * BoxModule.filterProxyRunning to false BEFORE calling StopMITM, so by the
 * time a legitimate stop shows up here as enabled=false, this watchdog has
 * already been stop()'d itself and isn't ticking anymore.
 */
class FilterProxyWatchdog(
    private val dataDir: String,
    private val onUnhealthy: () -> Unit,
) {
    private var scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    private var tickJob: Job? = null
    @Volatile private var sawRunning = false

    fun start() {
        if (tickJob != null) return
        if (!scope.isActive) scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
        sawRunning = false
        tickJob = scope.launch {
            while (isActive) {
                delay(TICK_MS)
                tick()
            }
        }
        Log.i(TAG, "watchdog started")
    }

    private fun tick() {
        val enabled = try {
            JSONObject(Mobile.filterStatus(dataDir)).optBoolean("enabled", false)
        } catch (t: Throwable) {
            Log.w(TAG, "filterStatus poll failed", t)
            return
        }
        if (enabled) {
            sawRunning = true
            return
        }
        if (sawRunning) {
            Log.w(TAG, "browser ad-block proxy stopped unexpectedly")
            sawRunning = false
            onUnhealthy()
        }
    }

    fun stop() {
        tickJob?.cancel(); tickJob = null
        scope.cancel()
        Log.i(TAG, "watchdog stopped")
    }
}
