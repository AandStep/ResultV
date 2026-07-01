package com.resultv.android.vpn

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.Handler
import android.os.Looper
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
const val EXTRA_IS_RELOAD = "isReload"

/**
 * VpnService host. The actual sing-box engine runs inside libbox via
 * BoxModule. This service exists only so Android trusts us with
 * VpnService.Builder and so the engine can outlive the UI process.
 */
class ResultVpnService : VpnService() {

    @Volatile var tunPfd: ParcelFileDescriptor? = null

    // When true, onDestroy is part of a reload cycle (stopSelf + scheduled
    // restart) and must NOT flip VpnState to Idle — UI should stay in
    // Connecting through the gap.
    @Volatile private var reloadInProgress = false

    // libbox start/stop is synchronous and blocks (DNS, REALITY handshake,
    // tun setup) — keep it off the main thread to avoid ANR on Connect.
    private val worker = Executors.newSingleThreadExecutor { r ->
        Thread(r, "ResultV-Box").apply { isDaemon = true }
    }

    // Lifetime-scoped coroutine for live config reloads.
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    private var reloadWatcher: Job? = null
    @Volatile private var killSwitchWatchdog: KillSwitchWatchdog? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        // Always-on VPN starts the service directly via the
        // `android.net.VpnService` intent filter — no MainActivity is run,
        // so our repos won't have been touched yet. Ensure they're loaded
        // before we try to resolve the active profile / DNS preset.
        ensureReposReady()

        when (intent?.action) {
            ACTION_STOP -> {
                Log.i(TAG, "received STOP")
                AppLog.info(getString(R.string.log_disconnecting))
                reloadWatcher?.cancel(); reloadWatcher = null
                stopKillSwitchWatchdog()
                TrafficWatcher.stop()
                // Close the tun fd up front — this drops the system VPN
                // lock icon immediately. libbox.closeService() takes a
                // couple of seconds to drain connections, so push it to
                // the worker and let the user see Idle right away.
                closeTun()
                VpnState.set(VpnStatus.Idle)
                stopForeground(STOP_FOREGROUND_REMOVE)
                // Resolve the string before the service is torn down, then log
                // once the engine has actually drained on the worker thread.
                val disconnectedMsg = getString(R.string.log_disconnected)
                worker.execute {
                    BoxModule.filterProxyRunning = false
                    mobile.Mobile.stopFilterProxy()
                    BoxModule.stop()
                    AppLog.info(disconnectedMsg)
                }
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
                // A reload is an internal stop+start of a fresh service
                // instance — triggerReload already logged "Applying changes…",
                // so suppress the duplicate connect/connected pair here. The
                // flag rides on the restart intent because the new instance
                // can't see the old instance's fields.
                val isReload = intent?.getBooleanExtra(EXTRA_IS_RELOAD, false) ?: false
                if (!isReload) {
                    val serverName = ProfileRepository.state.value.active?.name ?: ""
                    AppLog.info(getString(R.string.log_connecting, serverName))
                }
                val connectedMsg = getString(R.string.log_connected)
                worker.execute {
                    try {
                        startBrowserAdBlockIfEnabled()
                        BoxModule.start(this, config)
                        val connectedAt = System.currentTimeMillis()
                        val connected = VpnStatus.Connected(connectedAt)
                        VpnState.set(connected)
                        if (!isReload) AppLog.success(connectedMsg)
                        renotify(buildNotification(connected))
                        startReloadWatcher()
                        startKillSwitchWatchdog()
                        // Subscribe to libbox status stream so HomeScreen's
                        // traffic cards show real uplink/downlink instead of
                        // the placeholder zeros.
                        TrafficWatcher.start()
                    } catch (t: Throwable) {
                        Log.e(TAG, "BoxModule.start failed", t)
                        val msg = t.message ?: t.javaClass.simpleName
                        VpnState.set(VpnStatus.Error(msg))
                        AppLog.error(getString(R.string.log_conn_failed, msg))
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

    /**
     * Best-effort: browser ad-block is a bonus feature layered on top of
     * the VPN tunnel, never a reason to fail the whole connect. MUST run
     * before BoxModule.start() — see the comment on this call site above —
     * and MUST leave BoxModule.filterProxyRunning=false on any failure (no
     * lists downloaded yet, port in use, etc.) so openTun() never applies
     * setHttpProxy to a dead proxy and breaks Chrome's HTTPS traffic.
     */
    private fun startBrowserAdBlockIfEnabled() {
        BoxModule.filterProxyRunning = false
        if (android.os.Build.VERSION.SDK_INT < android.os.Build.VERSION_CODES.Q) return
        if (!SettingsRepository.state.value.browserAdBlock) return
        try {
            mobile.Mobile.startFilterProxy(filesDir.absolutePath, BROWSER_ADBLOCK_PORT.toLong())
            BoxModule.filterProxyRunning = true
        } catch (t: Throwable) {
            Log.w(TAG, "browser ad-block proxy failed to start; Chrome will use normal routing", t)
        }
    }

    override fun onRevoke() {
        // OS-initiated revoke: another VPN app took over, or the user
        // toggled VPN off in system settings. We MUST stop the tunnel
        // (the OS already pulled the fd out from under us), but instead
        // of disappearing silently, post a Reconnect prompt so the user
        // can re-establish — addresses the Phase-3 plan tail.
        Log.i(TAG, "VPN permission revoked")
        AppLog.warning(getString(R.string.log_revoked), getString(R.string.log_source_system))
        reloadWatcher?.cancel(); reloadWatcher = null
        stopKillSwitchWatchdog()
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
        stopKillSwitchWatchdog()
        TrafficWatcher.stop()
        scope.cancel()
        closeTun()
        if (!reloadInProgress) {
            VpnState.set(VpnStatus.Idle)
        }
        worker.execute {
            BoxModule.filterProxyRunning = false
            mobile.Mobile.stopFilterProxy()
            BoxModule.stop()
        }
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
                SettingsRepository.state,
            ) { rules, app, profiles, settings ->
                // Key on the active profile + everything that changes routing.
                // From settings we only watch ad-block (it rebuilds the route
                // rules); other settings keep applying on reconnect.
                listOf(rules, app, profiles.activeId, settings.adblock)
            }
                .distinctUntilChanged()
                .drop(1)
                .debounce(300)
                .onEach { triggerReload() }
                .launchIn(this)
        }
    }

    private fun startKillSwitchWatchdog() {
        if (!SettingsRepository.state.value.killSwitch) return
        killSwitchWatchdog?.stop()
        killSwitchWatchdog = KillSwitchWatchdog(
            onEngage = { reloadKillSwitch(panic = true) },
            onDisengage = { reloadKillSwitch(panic = false) },
        ).also { it.start() }
    }

    private fun stopKillSwitchWatchdog() {
        killSwitchWatchdog?.stop()
        killSwitchWatchdog = null
        VpnState.setKillSwitchEngaged(false)
    }

    /**
     * Engage/disengage via an IN-PLACE BoxModule.reload (keeps the tun up — no
     * leak window). MUST NOT use triggerReload, which closes the tun and
     * restarts the service.
     */
    private fun reloadKillSwitch(panic: Boolean) {
        val active = ProfileRepository.state.value.active ?: return
        val cfg = BuildOptionsBuilder.buildConfig(active, filesDir.absolutePath, panic = panic)
        if (cfg == null) {
            Log.w(TAG, "kill switch reload skipped — config build failed")
            return
        }
        // Resolve the localized log line before hopping to the worker (getString
        // needs the Context, which is cleaner to touch on the calling thread).
        val logMsg =
            if (panic) getString(R.string.log_killswitch_engaged)
            else getString(R.string.log_killswitch_released)
        worker.execute {
            BoxModule.reload(cfg)
            VpnState.setKillSwitchEngaged(panic)
            // Surface it in the in-app log — the user must see WHY traffic
            // stopped, not just that it did.
            if (panic) AppLog.warning(logMsg) else AppLog.success(logMsg)
            Log.i(TAG, "kill switch ${if (panic) "ENGAGED (block)" else "released (normal)"}")
            Handler(Looper.getMainLooper()).post {
                renotify(buildNotification(VpnState.status.value))
            }
        }
    }

    private fun triggerReload() {
        val active = ProfileRepository.state.value.active ?: return
        val configJson = BuildOptionsBuilder.buildConfig(active, filesDir.absolutePath)
        if (configJson == null) {
            Log.w(TAG, "rebuild config for reload failed or profile empty")
            return
        }
        if (!BoxModule.isRunning) {
            Log.w(TAG, "reload skipped — no running server")
            return
        }

        // Any in-process restart of sing-box (libbox reload or stop/start in
        // the same service instance) leaves Android's VPN NetworkAgent in a
        // half-dead state — symptoms include "upload counts climb but
        // downloads never arrive". The only reliable approach is to mimic
        // what the user does manually: fully stop the VpnService, wait for
        // libbox's closeService to fully drain (it can take "a couple of
        // seconds"), give Android more time to tear down ConnectivityService
        // state, then start a brand-new service instance with the new config.
        Log.i(TAG, "triggerReload: full service restart for config change")
        AppLog.info(getString(R.string.log_reapplying), getString(R.string.log_source_system))

        reloadInProgress = true
        TrafficStats.reset()
        TrafficWatcher.stop()
        reloadWatcher?.cancel(); reloadWatcher = null
        stopKillSwitchWatchdog()

        // Keep UI/notification in Connecting through the gap so the user
        // doesn't see a flash of Idle.
        VpnState.set(VpnStatus.Connecting)
        renotify(buildNotification(VpnStatus.Connecting))

        val ctx = applicationContext
        val restartIntent = Intent(ctx, ResultVpnService::class.java).apply {
            action = ACTION_START
            putExtra(EXTRA_CONFIG_JSON, configJson)
            putExtra(EXTRA_IS_RELOAD, true)
        }

        // Tear down on the worker so we can SYNCHRONOUSLY wait for libbox's
        // closeService to drain before scheduling the restart — otherwise
        // the new BoxModule.start can race the old sing-box's shutdown and
        // get a broken outbound (upload-only / no-download symptom).
        worker.execute {
            Log.i(TAG, "triggerReload: closing TUN + stopping libbox (sync)")
            closeTun()
            val t0 = System.currentTimeMillis()
            BoxModule.stop()
            Log.i(TAG, "triggerReload: BoxModule.stop took ${System.currentTimeMillis() - t0}ms")

            // Schedule the fresh start on the main looper so it survives
            // this service instance's onDestroy. 1500ms past the stop gives
            // Android time to fully tear down the VPN NetworkAgent.
            Handler(Looper.getMainLooper()).postDelayed({
                Log.i(TAG, "triggerReload: dispatching ACTION_START to fresh service")
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                    ctx.startForegroundService(restartIntent)
                } else {
                    ctx.startService(restartIntent)
                }
            }, 1500)

            // Stop the current service on the main thread.
            Handler(Looper.getMainLooper()).post {
                stopForeground(STOP_FOREGROUND_REMOVE)
                stopSelf()
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
            is VpnStatus.Connected ->
                if (VpnState.killSwitchEngaged.value)
                    getString(R.string.vpn_status_killswitch)
                else getString(R.string.vpn_status_connected)
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
