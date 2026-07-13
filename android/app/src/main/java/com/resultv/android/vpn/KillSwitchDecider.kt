package com.resultv.android.vpn

/** Engage/disengage decision returned by [KillSwitchDecider.onTick]. */
enum class KsAction { NONE, ENGAGE, DISENGAGE }

/**
 * One tick's outcome: the action plus the probe bookkeeping the watchdog
 * needs to log desktop-style "(N/M)" probe lines without duplicating the
 * decision logic.
 */
data class KsTick(
    val action: KsAction,
    val failCount: Int,
    val deferredByTraffic: Boolean,
)

/**
 * Pure state machine mirroring the desktop watchdog (manager.go monitorHealth):
 * declare the proxy dead after [failuresBeforeDead] consecutive failed health
 * probes, unless real proxy traffic moved this interval (liveness veto). When
 * dead, stay engaged until a probe succeeds, then disengage.
 *
 * IO-free so it can be unit-tested on the JVM; [KillSwitchWatchdog] feeds it
 * the urltest delay and the per-interval traffic delta.
 */
class KillSwitchDecider(
    private val failuresBeforeDead: Int = 3,
    private val trafficAliveBytes: Long = 2048,
) {
    private var consecutiveFails = 0
    private var engaged = false

    /** Probe-failure ceiling, exposed so log lines can render "(N/M)". */
    val failureThreshold: Int get() = failuresBeforeDead

    /**
     * @param delayMs urltest result for this tick; <= 0 means the probe failed.
     * @param trafficDelta bytes the proxy moved since the last tick.
     */
    fun onTick(delayMs: Int, trafficDelta: Long): KsTick {
        val alive = delayMs > 0
        if (alive) {
            consecutiveFails = 0
            if (engaged) {
                engaged = false
                return KsTick(KsAction.DISENGAGE, 0, false)
            }
            return KsTick(KsAction.NONE, 0, false)
        }
        if (engaged) return KsTick(KsAction.NONE, consecutiveFails, false)
        // Probe failed but the proxy carried real traffic → transient; hold off
        // but keep counting so a genuine outage still trips later.
        if (trafficDelta >= trafficAliveBytes) {
            consecutiveFails++
            return KsTick(KsAction.NONE, consecutiveFails, true)
        }
        consecutiveFails++
        if (consecutiveFails >= failuresBeforeDead) {
            engaged = true
            return KsTick(KsAction.ENGAGE, consecutiveFails, false)
        }
        return KsTick(KsAction.NONE, consecutiveFails, false)
    }

    val isEngaged: Boolean get() = engaged
}
