package com.resultv.android.vpn

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.util.Log
import com.resultv.android.MainActivity
import com.resultv.android.R
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.FlowPreview
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.debounce
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.flow.launchIn
import kotlinx.coroutines.flow.onEach
import kotlinx.coroutines.launch
import mobile.Mobile
import java.util.concurrent.Executors

private const val TAG = "ResultV/Service"
private const val CHANNEL_ID = "resultv_vpn"
private const val NOTIFICATION_ID = 1
// Distinct ID + channel for the revoke-prompt: the foreground status
// notification is torn down with stopForeground(REMOVE) before we post
// this one, so they must not share IDs.
private const val REVOKE_NOTIFICATION_ID = 2
private const val REVOKE_CHANNEL_ID = "resultv_vpn_revoke"

const val ACTION_START = "com.resultv.android.START"
const val ACTION_STOP = "com.resultv.android.STOP"
const val EXTRA_CONFIG_JSON = "configJson"
const val EXTRA_RECONNECT_AFTER_REVOKE = "reconnectAfterRevoke"

/**
 * VpnService host. The actual sing-box engine runs inside libbox via
 * BoxModule. This service exists only so Android trusts us with
 * VpnService.Builder and so the engine can outlive the UI process.
 */
class ResultVpnService : VpnService() {

    @Volatile var tunPfd: ParcelFileDescriptor? = null

    // libbox start/stop is synchronous and blocks (DNS, REALITY handshake,
    // tun setup) — keep it off the main thread to avoid ANR on Connect.
    private val worker = Executors.newSingleThreadExecutor { r ->
        Thread(r, "ResultV-Box").apply { isDaemon = true }
    }

    // Lifetime-scoped coroutine for live config reloads.
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    private var reloadWatcher: Job? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        // Always-on VPN starts the service directly via the
        // `android.net.VpnService` intent filter — no MainActivity is run,
        // so our repos won't have been touched yet. Ensure they're loaded
        // before we try to resolve the active profile / DNS preset.
        ensureReposReady()

        when (intent?.action) {
            ACTION_STOP -> {
                Log.i(TAG, "received STOP")
                reloadWatcher?.cancel(); reloadWatcher = null
                TrafficWatcher.stop()
                // Close the tun fd up front — this drops the system VPN
                // lock icon immediately. libbox.closeService() takes a
                // couple of seconds to drain connections, so push it to
                // the worker and let the user see Idle right away.
                closeTun()
                VpnState.set(VpnStatus.Idle)
                stopForeground(STOP_FOREGROUND_REMOVE)
                worker.execute { BoxModule.stop() }
                stopSelf()
                return START_NOT_STICKY
            }
            else -> {
                // EXTRA_CONFIG_JSON is set on the UI-initiated path
                // (MainActivity.connect). Always-on VPN supplies no extras
                // and only fires the SERVICE_INTERFACE intent — fall back
                // to rebuilding the config from the persisted active profile.
                val config = intent?.getStringExtra(EXTRA_CONFIG_JSON) ?: buildConfigFromActiveProfile()
                if (config.isNullOrEmpty()) {
                    Log.e(TAG, "no config available (no extra, no active profile) — stopping")
                    stopSelf()
                    return START_NOT_STICKY
                }
                VpnState.set(VpnStatus.Connecting)
                startForeground(NOTIFICATION_ID, buildNotification(VpnStatus.Connecting))
                worker.execute {
                    try {
                        BoxModule.start(this, config)
                        val connectedAt = System.currentTimeMillis()
                        val connected = VpnStatus.Connected(connectedAt)
                        VpnState.set(connected)
                        renotify(buildNotification(connected))
                        startReloadWatcher()
                        // Subscribe to libbox status stream so HomeScreen's
                        // traffic cards show real uplink/downlink instead of
                        // the placeholder zeros.
                        TrafficWatcher.start()
                    } catch (t: Throwable) {
                        Log.e(TAG, "BoxModule.start failed", t)
                        VpnState.set(VpnStatus.Error(t.message ?: t.javaClass.simpleName))
                        closeTun()
                        stopForeground(STOP_FOREGROUND_REMOVE)
                        stopSelf()
                    }
                }
                return START_STICKY
            }
        }
    }

    /**
     * Lazy init for the always-on VPN path. MainActivity normally initialises
     * these on startup, but the OS can launch this service standalone via
     * android.net.VpnService when the user enables "Always-on VPN" in
     * system settings.
     */
    private fun ensureReposReady() {
        val app = applicationContext
        ProfileRepository.init(app)
        SubscriptionRepository.init(app)
        SettingsRepository.init(app)
        RoutingRulesRepository.init(app)
        AppRoutingRepository.init(app)
        SmartListRepository.init(app)
    }

    /** Rebuild a sing-box config from whatever profile is currently active. */
    private fun buildConfigFromActiveProfile(): String? {
        val active = ProfileRepository.state.value.active ?: return null
        return BuildOptionsBuilder.buildConfig(active, filesDir.absolutePath)
    }

    override fun onRevoke() {
        // OS-initiated revoke: another VPN app took over, or the user
        // toggled VPN off in system settings. We MUST stop the tunnel
        // (the OS already pulled the fd out from under us), but instead
        // of disappearing silently, post a Reconnect prompt so the user
        // can re-establish — addresses the Phase-3 plan tail.
        Log.i(TAG, "VPN permission revoked")
        reloadWatcher?.cancel(); reloadWatcher = null
        TrafficWatcher.stop()
        closeTun()
        VpnState.set(VpnStatus.Idle)
        stopForeground(STOP_FOREGROUND_REMOVE)
        worker.execute { BoxModule.stop() }
        postReconnectPromptNotification()
        stopSelf()
    }

    override fun onDestroy() {
        reloadWatcher?.cancel(); reloadWatcher = null
        TrafficWatcher.stop()
        scope.cancel()
        closeTun()
        VpnState.set(VpnStatus.Idle)
        worker.execute { BoxModule.stop() }
        worker.shutdown()
        super.onDestroy()
    }

    /**
     * Watch routing-rule + per-app-routing + active-profile state and ask
     * libbox to swap the running config in-place when anything changes.
     * Drops the very first emission (that's the state at start time, which
     * is already wired into the running engine).
     *
     * Debounce coalesces rapid edits — if the user types several domain
     * patterns in quick succession we rebuild once, not once per keystroke.
     */
    @OptIn(FlowPreview::class)
    private fun startReloadWatcher() {
        reloadWatcher?.cancel()
        reloadWatcher = scope.launch {
            combine(
                RoutingRulesRepository.state,
                AppRoutingRepository.state,
                ProfileRepository.state,
            ) { rules, app, profiles -> Triple(rules, app, profiles.activeId) }
                .distinctUntilChanged()
                .drop(1)
                .debounce(300)
                .onEach { triggerReload() }
                .launchIn(this)
        }
    }

    private fun triggerReload() {
        val active = ProfileRepository.state.value.active ?: return
        val configJson = BuildOptionsBuilder.buildConfig(active, filesDir.absolutePath)
        if (configJson == null) {
            Log.w(TAG, "rebuild config for reload failed or profile empty")
            return
        }
        worker.execute {
            if (!BoxModule.reload(configJson)) {
                Log.w(TAG, "reload skipped — no running server")
            }
        }
    }

    private fun closeTun() {
        val pfd = tunPfd ?: return
        tunPfd = null
        try {
            pfd.close()
        } catch (t: Throwable) {
            Log.w(TAG, "tun pfd close threw", t)
        }
    }

    private fun renotify(n: Notification) {
        getSystemService(NotificationManager::class.java).notify(NOTIFICATION_ID, n)
    }

    private fun postReconnectPromptNotification() {
        val nm = getSystemService(NotificationManager::class.java)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val ch = NotificationChannel(
                REVOKE_CHANNEL_ID,
                getString(R.string.vpn_revoke_channel_name),
                NotificationManager.IMPORTANCE_DEFAULT,
            )
            nm.createNotificationChannel(ch)
        }
        val reopen = PendingIntent.getActivity(
            this, 2,
            Intent(this, MainActivity::class.java).apply {
                action = Intent.ACTION_MAIN
                addCategory(Intent.CATEGORY_LAUNCHER)
                flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
                putExtra(EXTRA_RECONNECT_AFTER_REVOKE, true)
            },
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
        val notification = Notification.Builder(this, REVOKE_CHANNEL_ID)
            .setContentTitle(getString(R.string.vpn_revoke_title))
            .setContentText(getString(R.string.vpn_revoke_text))
            .setSmallIcon(R.drawable.ic_notification)
            .setContentIntent(reopen)
            .setAutoCancel(true)
            .addAction(
                Notification.Action.Builder(
                    null, getString(R.string.vpn_revoke_action), reopen,
                ).build()
            )
            .build()
        nm.notify(REVOKE_NOTIFICATION_ID, notification)
    }

    private fun buildNotification(status: VpnStatus): Notification {
        val nm = getSystemService(NotificationManager::class.java)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val ch = NotificationChannel(
                CHANNEL_ID,
                getString(R.string.vpn_channel_name),
                NotificationManager.IMPORTANCE_LOW,
            )
            nm.createNotificationChannel(ch)
        }
        val openApp = PendingIntent.getActivity(
            this, 0,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE,
        )
        val stopIntent = PendingIntent.getService(
            this, 1,
            Intent(this, ResultVpnService::class.java).apply { action = ACTION_STOP },
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
        val text = when (status) {
            VpnStatus.Connecting -> getString(R.string.vpn_status_connecting)
            is VpnStatus.Connected -> getString(R.string.vpn_status_connected)
            VpnStatus.Idle -> getString(R.string.vpn_status_idle)
            is VpnStatus.Error -> getString(R.string.vpn_status_error, status.message)
        }
        return Notification.Builder(this, CHANNEL_ID)
            .setContentTitle(getString(R.string.app_name))
            .setContentText(text)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentIntent(openApp)
            .setOngoing(true)
            .addAction(
                Notification.Action.Builder(
                    null, getString(R.string.vpn_action_disconnect), stopIntent,
                ).build()
            )
            .build()
    }
}
