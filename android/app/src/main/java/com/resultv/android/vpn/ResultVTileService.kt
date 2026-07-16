package com.resultv.android.vpn

import android.app.PendingIntent
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.service.quicksettings.Tile
import android.service.quicksettings.TileService
import android.util.Log
import com.resultv.android.MainActivity
import com.resultv.android.R
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.launchIn
import kotlinx.coroutines.flow.onEach
import kotlinx.coroutines.launch

private const val TAG = "ResultV/Tile"

/**
 * Quick-Settings tile that toggles the VPN with one tap.
 *
 * Click rules:
 *   - VPN already running → send STOP intent (no consent dialog needed).
 *   - VPN idle + consent already granted → start the service directly.
 *   - Consent not yet granted, or device is locked → bounce to MainActivity
 *     (TileService cannot show a system dialog itself).
 */
class ResultVTileService : TileService() {

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private var watcher: Job? = null

    override fun onStartListening() {
        super.onStartListening()
        // Mirror VpnState into the tile while the QS panel is visible.
        watcher?.cancel()
        watcher = scope.launch {
            VpnState.status
                .onEach { syncTile(it) }
                .launchIn(this)
        }
    }

    override fun onStopListening() {
        watcher?.cancel(); watcher = null
        super.onStopListening()
    }

    override fun onDestroy() {
        scope.cancel()
        super.onDestroy()
    }

    override fun onClick() {
        super.onClick()
        when (VpnState.status.value) {
            is VpnStatus.Connected, is VpnStatus.Connecting -> stopVpn()
            is VpnStatus.Idle, is VpnStatus.Error -> startVpn()
        }
    }

    private fun stopVpn() {
        startService(
            Intent(this, ResultVpnService::class.java).apply { action = ACTION_STOP }
        )
    }

    private fun startVpn() {
        // Need consent dialog → tile can't show one; open MainActivity which
        // is the only context the system trusts to launch VpnService.prepare.
        if (VpnService.prepare(this) != null || isLocked) {
            launchMainActivity()
            return
        }
        // Bounce to the app when there's no active profile — empty state shows
        // the "Add server" CTA. The config itself is NOT built or passed here:
        // with the expanded Smart list it reaches ~4.6 MB and would blow the
        // ~1 MB Binder transaction limit. The service rebuilds it from the same
        // persisted state.
        val service = ResultVpnService::class.java
        if (!hasActiveProfile()) {
            launchMainActivity()
            return
        }
        val intent = Intent(this, service).apply { action = ACTION_START }
        // startForegroundService is required because the tile process is
        // background-restricted on API 26+; the service immediately calls
        // startForeground inside onStartCommand.
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            startForegroundService(intent)
        } else {
            startService(intent)
        }
    }

    private fun launchMainActivity() {
        val pi = PendingIntent.getActivity(
            this, 0,
            Intent(this, MainActivity::class.java).apply {
                addCategory(Intent.CATEGORY_LAUNCHER)
                flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
            },
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            startActivityAndCollapse(pi)
        } else {
            @Suppress("DEPRECATION")
            startActivityAndCollapse(
                Intent(this, MainActivity::class.java).apply {
                    addCategory(Intent.CATEGORY_LAUNCHER)
                    flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
                }
            )
        }
    }

    private fun hasActiveProfile(): Boolean {
        val app = applicationContext
        ProfileRepository.init(app)
        SettingsRepository.init(app)
        RoutingRulesRepository.init(app)
        return ProfileRepository.state.value.active != null
    }

    private fun syncTile(status: VpnStatus) {
        val tile = qsTile ?: return
        tile.state = when (status) {
            is VpnStatus.Connected -> Tile.STATE_ACTIVE
            is VpnStatus.Connecting -> Tile.STATE_UNAVAILABLE
            is VpnStatus.Idle, is VpnStatus.Error -> Tile.STATE_INACTIVE
        }
        tile.label = getString(R.string.app_name)
        tile.contentDescription = getString(
            when (status) {
                is VpnStatus.Connected -> R.string.tile_label_connected
                is VpnStatus.Connecting -> R.string.tile_label_connecting
                is VpnStatus.Idle -> R.string.tile_label_idle
                is VpnStatus.Error -> R.string.tile_label_idle
            }
        )
        tile.updateTile()
    }
}
