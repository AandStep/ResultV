package com.resultv.android.vpn

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

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

    internal fun set(s: VpnStatus) { _status.value = s }

    private val _killSwitchEngaged = MutableStateFlow(false)
    /** True while the kill switch has blocked all traffic (proxy down). */
    val killSwitchEngaged: StateFlow<Boolean> = _killSwitchEngaged.asStateFlow()

    internal fun setKillSwitchEngaged(v: Boolean) { _killSwitchEngaged.value = v }
}
