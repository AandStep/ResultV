package com.resultv.android.vpn

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import mobile.Mobile

sealed interface VpnStatus {
    data object Idle : VpnStatus
    data object Connecting : VpnStatus
    /** [connectedAt] is wall-clock millis at the moment the tunnel came up. */
    data class Connected(val connectedAt: Long) : VpnStatus
    data class Error(val message: String) : VpnStatus
}

object VpnState {
    private val _status = MutableStateFlow<VpnStatus>(VpnStatus.Idle)
    val status: StateFlow<VpnStatus> = _status.asStateFlow()

    internal fun set(s: VpnStatus) {
        _status.value = s
        // Mirror the tunnel lifecycle into the Go bindings so PingEntry can
        // refuse the keyed WireGuard/AmneziaWG handshake probe while a tunnel
        // is up: that probe handshakes with the profile's own private key, so
        // the server sees the same peer as the live session and re-points the
        // peer endpoint at the probe's throwaway socket — traffic keeps being
        // sent but nothing comes back (see Mobile.setTunnelActive).
        //
        // Connecting counts as active: the engine is already handshaking by
        // then, and a ping sweep landing mid-connect is what made AmneziaWG
        // need several attempts to come up at all.
        //
        // Hooked here rather than at each VpnState.set call site so no
        // transition can miss it. runCatching keeps JVM unit tests (no native
        // library loaded) from blowing up on the binding call.
        val active = s is VpnStatus.Connecting || s is VpnStatus.Connected
        runCatching { Mobile.setTunnelActive(active) }
    }

    private val _killSwitchEngaged = MutableStateFlow(false)
    /** True while the kill switch has blocked all traffic (proxy down). */
    val killSwitchEngaged: StateFlow<Boolean> = _killSwitchEngaged.asStateFlow()

    internal fun setKillSwitchEngaged(v: Boolean) { _killSwitchEngaged.value = v }
}
