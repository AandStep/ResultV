package com.resultv.android.vpn

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

data class TrafficSnapshot(
    val downloadBytes: Long = 0,
    val uploadBytes: Long = 0,
    val downloadBps: Long = 0,
    val uploadBps: Long = 0,
    /** Recent download bytes/sec samples, oldest → newest. Capped at HISTORY_SIZE. */
    val downloadHistory: List<Long> = emptyList(),
    val uploadHistory: List<Long> = emptyList(),
)

const val TRAFFIC_HISTORY_SIZE = 60

/**
 * Live traffic-stats source. [TrafficWatcher] subscribes to sing-box's
 * libbox status stream and pushes uplink/downlink samples through
 * [publish] once per second. UI consumes [snapshot] as a flow.
 */
object TrafficStats {
    private val _snapshot = MutableStateFlow(TrafficSnapshot())
    val snapshot: StateFlow<TrafficSnapshot> = _snapshot.asStateFlow()

    /** Replace the snapshot — called from TrafficWatcher on each tick. */
    fun publish(s: TrafficSnapshot) {
        _snapshot.value = s
    }

    /** Reset counters when a new connection starts. */
    fun reset() {
        _snapshot.value = TrafficSnapshot()
    }
}
