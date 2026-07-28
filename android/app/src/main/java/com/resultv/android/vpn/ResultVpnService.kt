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
import kotlinx.coroutines.withContext
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
    @Volatile private var filterProxyWatchdog: FilterProxyWatchdog? = null

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
                stopFilterProxyWatchdog()
                TrafficWatcher.stop()
                ConnectionWatcher.stop()
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
                    // Stop the box FIRST: closeService closes libbox's dup of
                    // the tun fd, which is what actually drops the VPN network
                    // (and its setHttpProxy) — our closeTun() above only closed
                    // our own pfd. If the MITM close below is ever slow, the
                    // browser must not be left on a dead proxy meanwhile.
                    BoxModule.stop()
                    mobile.Mobile.stopFilterProxy()
                    AppLog.info(disconnectedMsg)
                }
                stopSelf()
                return START_NOT_STICKY
            }
            else -> {
                // Need an active profile to connect. Cheap check up front so we
                // can promise foreground immediately, then do the (possibly slow)
                // list warm-up + config build off the main thread. The config is
                // ALWAYS rebuilt (never passed in): with the expanded Smart list
                // it reaches ~4.6 MB and blows the ~1 MB Binder limit.
                if (ProfileRepository.state.value.active == null) {
                    Log.e(TAG, "no active profile — stopping")
                    stopSelf()
                    return START_NOT_STICKY
                }
                // A reload is an internal stop+start of a fresh service instance —
                // triggerReload already logged "Applying changes…"; suppress the
                // duplicate connect/connected pair. The flag rides on the restart
                // intent because the new instance can't see the old's fields.
                val isReload = intent?.getBooleanExtra(EXTRA_IS_RELOAD, false) ?: false
                VpnState.set(VpnStatus.Connecting)
                startForeground(NOTIFICATION_ID, buildNotification(VpnStatus.Connecting))
                if (!isReload) {
                    val serverName = ProfileRepository.state.value.active?.name ?: ""
                    AppLog.info(getString(R.string.log_connecting, serverName))
                }
                val connectedMsg = getString(R.string.log_connected)
                scope.launch {
                    // Connect NEVER waits on a list. The Smart rule-set is
                    // already on disk (bundled seed on a fresh install, or a
                    // previous download) and ad-block rule_sets are local-only,
                    // so the config we build below references local files.
                    //
                    // This used to be a bounded warm-up, but the bound was a
                    // lie: coroutine cancellation cannot interrupt the blocking
                    // JNI fetch it wrapped, so a fresh install waited out 7
                    // sequential downloads (5-13s). Refresh happens in the
                    // background and applies via scheduleListReadyReload().
                    val listsReady = if (isReload) true else listsAlreadyOnDisk()
                    if (!isReload) kickBackgroundListRefresh()
                    val tCfg = System.currentTimeMillis()
                    val config = buildConfigFromActiveProfile()
                    Log.i(TAG, "connect timing: buildConfig=${System.currentTimeMillis() - tCfg}ms, size=${config?.length ?: 0}")
                    if (config.isNullOrEmpty()) {
                        Log.e(TAG, "no config available — stopping")
                        withContext(Dispatchers.Main) { stopSelf() }
                        return@launch
                    }
                    worker.execute {
                        val t0 = System.currentTimeMillis()
                        try {
                            // openTun during start() must see filterProxyRunning=false
                            // (browser ad-block attaches after, off the critical path).
                            BoxModule.filterProxyRunning = false
                            BoxModule.start(this@ResultVpnService, config)
                            val connectedAt = System.currentTimeMillis()
                            Log.i(TAG, "connect timing: BoxModule.start=${connectedAt - t0}ms")
                            val connected = VpnStatus.Connected(connectedAt)
                            VpnState.set(connected)
                            if (!isReload) {
                                AppLog.success(connectedMsg)
                                AppLog.info(
                                    R.string.log_connect_timing,
                                    connectedAt - t0,
                                    source = AppLog.resolve(R.string.log_source_proxy),
                                )
                            }
                            renotify(buildNotification(connected))
                            startReloadWatcher()
                            startKillSwitchWatchdog()
                            // Subscribe to libbox status stream so HomeScreen's
                            // traffic cards show real uplink/downlink.
                            TrafficWatcher.start()
                            // Structured per-host [CONN] lines (domain -> ip:port).
                            ConnectionWatcher.start()
                            // Attach browser ad-block now that the tunnel is up.
                            attachBrowserAdBlockAsync()
                            // Fallback: connected before the lists finished warming.
                            // Finish warming them and apply on the fly — no reconnect.
                            if (!listsReady) scheduleListReadyReload()
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
        AppLog.init(applicationContext)
        val app = applicationContext
        ProfileRepository.init(app)
        SubscriptionRepository.init(app)
        SettingsRepository.init(app)
        RoutingRulesRepository.init(app)
        AppRoutingRepository.init(app)
        AppInventory.init(app)
        SmartListRepository.init(app)
        // Needed so listsAlreadyOnDisk / kickBackgroundListRefresh can see the
        // ad-block SRS cache on the always-on path too (MainActivity, which
        // normally inits it, never ran).
        AdBlockRepository.init(app)
        // Refresh underlying IPv6 reachability so reload/always-on configs pick
        // the effective IPv6 flag (see NetworkProbe / BuildOptionsBuilder).
        NetworkProbe.init(app)
        NetworkProbe.refreshAsync()
    }

    /** Rebuild a sing-box config from whatever profile is currently active. */
    private fun buildConfigFromActiveProfile(): String? {
        val active = ProfileRepository.state.value.active ?: return null
        return BuildOptionsBuilder.buildConfig(active, filesDir.absolutePath)
    }

    /**
     * Whether the enabled lists are already usable from disk. Pure local checks
     * (a stat + an SRS validation) — no network, no blocking, microseconds.
     */
    private fun listsAlreadyOnDisk(): Boolean {
        val needAdBlock = SettingsRepository.state.value.adblock
        val needSmart = RoutingRulesRepository.state.value.mode == RoutingMode.Smart
        if (needAdBlock && !AdBlockRepository.state.value.hasLists) return false
        if (needSmart && !SmartListRepository.isReady()) return false
        return true
    }

    /**
     * Kick a TTL-respecting refresh of the enabled lists in the background. Never
     * awaited by the connect path — a fresh list applies via the reload in
     * [scheduleListReadyReload] (or simply on the next connect).
     */
    private fun kickBackgroundListRefresh() {
        if (SettingsRepository.state.value.adblock) AdBlockRepository.ensureLoadedAsync()
        if (RoutingRulesRepository.state.value.mode == RoutingMode.Smart) {
            SmartListRepository.ensureLoadedAsync()
        }
    }

    /**
     * Fallback for a connect that came up before a background list refresh
     * finished. Waits (without a deadline) for whatever [kickBackgroundListRefresh]
     * kicked off, then rebuilds the config and reloads in place so ad-block +
     * the full smart list activate — no user reconnect. Reloads at most once,
     * and only if a list actually became available. The existing reloadWatcher
     * does not observe the list repos, so this cannot double-fire with it.
     */
    private fun scheduleListReadyReload() {
        scope.launch {
            val ab = if (SettingsRepository.state.value.adblock) AdBlockRepository.ensureLoaded() else null
            val sl = if (RoutingRulesRepository.state.value.mode == RoutingMode.Smart) {
                SmartListRepository.ensureLoaded()
            } else null
            val gotAdBlock = ab?.hasLists == true
            val gotSmart = sl != null && sl.ready && sl.source != "builtin"
            if (!BoxModule.isRunning || (!gotAdBlock && !gotSmart)) return@launch
            val active = ProfileRepository.state.value.active ?: return@launch
            val cfg = BuildOptionsBuilder.buildConfig(active, filesDir.absolutePath) ?: return@launch
            Log.i(TAG, "lists ready post-connect — reloading (adblock=$gotAdBlock smart=$gotSmart)")
            worker.execute { BoxModule.reload(cfg) }
        }
    }

    /**
     * Attach the browser ad-block MITM proxy AFTER the tunnel is already up,
     * off the connect critical path. Starting it (urlfilter engine build + TLS
     * self-test) takes seconds; doing it before BoxModule.start() used to add
     * that to every connect. Here we start it on the worker (so it queues
     * behind the just-finished connect task) and, once the proxy is live, force
     * an in-place reload so openTun() re-runs and applies setHttpProxy — the
     * mirror image of onFilterProxyUnhealthy(), which reloads to REMOVE it.
     *
     * The tunnel stays up throughout; there's only a brief window right after
     * connect where the browser isn't yet filtered. The cached urlfilter engine
     * (Manager reuses it across connects) keeps that window short after the
     * first connect.
     */
    private fun attachBrowserAdBlockAsync() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.Q) return
        if (!SettingsRepository.state.value.browserAdBlock) return
        worker.execute {
            startBrowserAdBlockIfEnabled()
            // Only reload if the proxy actually came up (trusted cert + healthy);
            // startBrowserAdBlockIfEnabled leaves filterProxyRunning=false otherwise.
            if (BoxModule.filterProxyRunning && BoxModule.isRunning) {
                val active = ProfileRepository.state.value.active ?: return@execute
                val cfg = BuildOptionsBuilder.buildConfig(active, filesDir.absolutePath) ?: return@execute
                BoxModule.reload(cfg)
            }
        }
    }

    /**
     * Best-effort: browser ad-block is a bonus feature layered on top of
     * the VPN tunnel, never a reason to fail the whole connect. Runs AFTER
     * BoxModule.start() (see attachBrowserAdBlockAsync) and MUST leave
     * BoxModule.filterProxyRunning=false on any failure (no lists downloaded
     * yet, port in use, etc.) so the follow-up reload's openTun() never applies
     * setHttpProxy to a dead proxy and breaks Chrome's HTTPS traffic.
     */
    private fun startBrowserAdBlockIfEnabled() {
        BoxModule.filterProxyRunning = false
        if (android.os.Build.VERSION.SDK_INT < android.os.Build.VERSION_CODES.Q) return
        if (!SettingsRepository.state.value.browserAdBlock) return
        try {
            mobile.Mobile.startFilterProxy(filesDir.absolutePath, BROWSER_ADBLOCK_PORT.toLong())
            when (CertSelfTest.run(BROWSER_ADBLOCK_PORT)) {
                CertSelfTest.Result.PASS -> {
                    BoxModule.filterProxyRunning = true
                    SettingsRepository.setCertTrustState(com.resultv.android.vpn.CertTrustState.TRUSTED)
                    filterProxyWatchdog?.stop()
                    filterProxyWatchdog = FilterProxyWatchdog(filesDir.absolutePath) {
                        onFilterProxyUnhealthy()
                    }.also { it.start() }
                }
                CertSelfTest.Result.CERT_UNTRUSTED -> {
                    // Tear down; leave filterProxyRunning=false so setHttpProxy is NOT applied.
                    mobile.Mobile.stopFilterProxy()
                    SettingsRepository.setCertTrustState(com.resultv.android.vpn.CertTrustState.UNTRUSTED)
                    SettingsRepository.setBrowserAdBlock(false)
                    AppLog.warning(getString(R.string.log_browser_adblock_cert_untrusted))
                }
                CertSelfTest.Result.INCONCLUSIVE -> {
                    // Don't leave an unused proxy running; keep the toggle for next time.
                    // certTrustState is deliberately NOT touched here — a transient
                    // network hiccup must not overwrite the last known-good/known-bad
                    // determination (see CertTrustState's doc comment).
                    mobile.Mobile.stopFilterProxy()
                    AppLog.warning(getString(R.string.log_browser_adblock_selftest_inconclusive))
                }
            }
        } catch (t: Throwable) {
            BoxModule.filterProxyRunning = false
            try { mobile.Mobile.stopFilterProxy() } catch (_: Throwable) {}
            Log.w(TAG, "browser ad-block proxy failed to start; Chrome will use normal routing", t)
            AppLog.warning(
                R.string.log_adblock_proxy_start_failed,
                t.message ?: t.javaClass.simpleName,
                source = AppLog.resolve(R.string.log_source_adblock),
            )
        }
    }

    private fun stopFilterProxyWatchdog() {
        filterProxyWatchdog?.stop()
        filterProxyWatchdog = null
    }

    /**
     * Called from the watchdog's coroutine when the proxy dies mid-session.
     * Turns the feature off (so it doesn't silently keep failing on every
     * future reconnect) and forces a lightweight in-place reload — the same
     * technique reloadKillSwitch uses — so openTun() re-runs, re-reads
     * BoxModule.filterProxyRunning (now false), and drops setHttpProxy from
     * the live Builder. The sing-box config itself doesn't change; this is
     * purely to force a fresh TUN handover.
     */
    private fun onFilterProxyUnhealthy() {
        BoxModule.filterProxyRunning = false
        SettingsRepository.setBrowserAdBlock(false)
        AppLog.warning(getString(R.string.log_browser_adblock_disabled))
        val active = ProfileRepository.state.value.active ?: return
        val cfg = BuildOptionsBuilder.buildConfig(active, filesDir.absolutePath) ?: return
        worker.execute { BoxModule.reload(cfg) }
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
        // Stop the proxy watchdog BEFORE tearing the engine down — otherwise it
        // can see the dying proxy as "unhealthy" mid-revoke and trigger an
        // in-place reload against a VPN the OS has already pulled.
        stopFilterProxyWatchdog()
        TrafficWatcher.stop()
        ConnectionWatcher.stop()
        closeTun()
        VpnState.set(VpnStatus.Idle)
        stopForeground(STOP_FOREGROUND_REMOVE)
        worker.execute {
            BoxModule.filterProxyRunning = false
            // Same order as ACTION_STOP: box first (drops the tun + its
            // setHttpProxy), then the MITM proxy.
            BoxModule.stop()
            mobile.Mobile.stopFilterProxy()
        }
        postReconnectPromptNotification()
        stopSelf()
    }

    override fun onDestroy() {
        reloadWatcher?.cancel(); reloadWatcher = null
        stopKillSwitchWatchdog()
        stopFilterProxyWatchdog()
        TrafficWatcher.stop()
        ConnectionWatcher.stop()
        scope.cancel()
        closeTun()
        if (!reloadInProgress) {
            VpnState.set(VpnStatus.Idle)
        }
        worker.execute {
            BoxModule.filterProxyRunning = false
            // Box first — see the ACTION_STOP comment: closing the box drops
            // the tun (and its setHttpProxy) before the MITM proxy goes away.
            BoxModule.stop()
            mobile.Mobile.stopFilterProxy()
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
            AppLog.error(
                getString(R.string.log_ks_reload_build_failed),
                getString(R.string.log_source_killswitch),
            )
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
        ProfileRepository.state.value.active ?: return
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
        ConnectionWatcher.stop()
        reloadWatcher?.cancel(); reloadWatcher = null
        stopKillSwitchWatchdog()

        // Keep UI/notification in Connecting through the gap so the user
        // doesn't see a flash of Idle.
        VpnState.set(VpnStatus.Connecting)
        renotify(buildNotification(VpnStatus.Connecting))

        val ctx = applicationContext
        // No config extra: the fresh service rebuilds it from the same persisted
        // state. Passing it here would hit the same Binder size limit as the
        // connect path did.
        val restartIntent = Intent(ctx, ResultVpnService::class.java).apply {
            action = ACTION_START
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
