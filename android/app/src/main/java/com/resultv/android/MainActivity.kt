package com.resultv.android

import android.content.Context
import android.content.Intent
import android.net.VpnService
import android.os.Bundle
import android.util.Log
import androidx.activity.ComponentActivity
import androidx.activity.compose.BackHandler
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.annotation.StringRes
import androidx.lifecycle.lifecycleScope
import androidx.core.net.toUri
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Apps
import androidx.compose.material.icons.outlined.Home
import androidx.compose.material.icons.outlined.Language
import androidx.compose.material.icons.outlined.List
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material3.CenterAlignedTopAppBar
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.saveable.rememberSaveableStateHolder
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.resultv.android.locale.LocaleManager
import com.resultv.android.theme.Brand
import com.resultv.android.theme.ResultVTheme
import com.resultv.android.ui.screens.AddScreen
import com.resultv.android.ui.screens.HomeScreen
import com.resultv.android.ui.screens.FaqScreen
import com.resultv.android.ui.screens.LogsScreen
import com.resultv.android.ui.screens.ProxiesScreen
import com.resultv.android.ui.screens.RulesScreen
import com.resultv.android.ui.screens.SettingsScreen
import com.resultv.android.vpn.ACTION_START
import com.resultv.android.vpn.ACTION_STOP
import com.resultv.android.vpn.AppRoutingRepository
import com.resultv.android.vpn.BuildOptionsBuilder
import com.resultv.android.vpn.DeepLinkImporter
import com.resultv.android.vpn.EXTRA_CONFIG_JSON
import com.resultv.android.vpn.ProfileRepository
import com.resultv.android.vpn.ResultVpnService
import com.resultv.android.vpn.VpnState
import com.resultv.android.vpn.VpnStatus

private const val TAG = "ResultV/UI"

private const val WEBSITE_URL = "https://result-proxy.ru/"
private const val TELEGRAM_URL = "https://t.me/resultvpn"

/** Open an external URL in the user's browser / Telegram app. */
private fun openUrl(ctx: Context, url: String) {
    runCatching {
        ctx.startActivity(
            Intent(Intent.ACTION_VIEW, url.toUri())
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
        )
    }
}

private enum class Tab(
    @StringRes val titleRes: Int,
    val icon: ImageVector,
) {
    Home(R.string.tab_home, Icons.Outlined.Home),
    Proxies(R.string.tab_proxies, Icons.Outlined.List),
    Add(R.string.tab_add, Icons.Outlined.Add),
    Settings(R.string.tab_settings, Icons.Outlined.Settings),
}

class MainActivity : ComponentActivity() {

    private var pendingConfig: String? = null

    /**
     * Apply the user-selected locale before any resource is resolved. The
     * Activity is recreated when the user picks a new language; on the
     * second pass attachBaseContext sees the new persisted code and wraps
     * the configuration so all stringResource() lookups pick the right
     * `values-<lang>/strings.xml`.
     */
    override fun attachBaseContext(newBase: Context) {
        super.attachBaseContext(LocaleManager.wrap(newBase))
    }

    private val vpnPermissionLauncher = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult(),
    ) { result ->
        if (result.resultCode == RESULT_OK) {
            val cfg = pendingConfig
            pendingConfig = null
            if (cfg != null) startService(cfg)
        } else {
            Log.w(TAG, "VPN permission denied (resultCode=${result.resultCode})")
            com.resultv.android.vpn.AppLog.warning(
                getString(R.string.log_permission_denied),
                getString(R.string.log_source_system),
            )
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        // Swap from the white splash theme (manifest) to the real app theme
        // BEFORE super.onCreate, so the system's first frame after the
        // splash window matches Compose's dark background instead of
        // flashing white.
        setTheme(R.style.Theme_ResultV)
        super.onCreate(savedInstanceState)
        com.resultv.android.vpn.AppLog.init(this)
        // Edge-to-edge: app draws behind status + nav bars; Scaffold's TopAppBar
        // and NavigationBar consume the window-inset paddings so content above
        // the gesture nav stays tappable.
        enableEdgeToEdge()
        ProfileRepository.init(applicationContext)
        com.resultv.android.vpn.SubscriptionRepository.init(applicationContext)
        AppRoutingRepository.init(applicationContext)
        com.resultv.android.vpn.RoutingRulesRepository.init(applicationContext)
        com.resultv.android.vpn.SettingsRepository.init(applicationContext)
        // Probe underlying IPv6 reachability early so the effective IPv6 flag is
        // ready by the time the user taps connect (see NetworkProbe / BuildOptions).
        com.resultv.android.vpn.NetworkProbe.init(applicationContext)
        com.resultv.android.vpn.NetworkProbe.refreshAsync()
        com.resultv.android.vpn.AdBlockRepository.init(applicationContext)
        // Keep the ad-block SRS cache warm (24h TTL) when blocking is on, so
        // connects reference local lists instead of waiting on a remote fetch.
        if (com.resultv.android.vpn.SettingsRepository.state.value.adblock) {
            com.resultv.android.vpn.AdBlockRepository.ensureLoadedAsync()
        }
        // Load + log the Smart routing lists at startup, unconditionally, like
        // the desktop's initSmartBlockedDomains (app.go): it surfaces "[SMART]
        // Источник списков …" at launch regardless of routing mode, and the
        // 24h TTL means a warm cache just logs without a network fetch.
        com.resultv.android.vpn.SmartListRepository.init(applicationContext)
        com.resultv.android.vpn.SmartListRepository.ensureLoadedAsync()
        com.resultv.android.vpn.DataUsageRepository.init(applicationContext)
        // Periodic subscription auto-refresh — runs while the Activity is
        // alive (matches the desktop's React hook). The refresher self-
        // arms based on SettingsRepository.subscriptionAutoUpdate / interval.
        com.resultv.android.vpn.SubscriptionRefresher.start(lifecycleScope, applicationContext)


        
        setContent {
            ResultVTheme {
                AppShell(
                    dataDir = filesDir.absolutePath,
                    onPower = ::onPowerPressed,
                )
            }
        }

        // resultv:// VIEW intent from a browser or another app — singleTask
        // means the cold-start case lands here. Hand the URL to the importer
        // which already knows how to decode rvsub ciphertext, fetch a
        // subscription, or parse a bare share-link.
        //
        // Only on a genuine cold start (savedInstanceState == null). When the
        // Activity is recreated for a configuration change — notably the
        // language switch, which calls recreate() — getIntent() still returns
        // the original launch intent, so re-running this would re-import the
        // same subscription. The warm-start path is handled by onNewIntent.
        if (savedInstanceState == null) {
            // Cold-start boot marker, mirroring the desktop's "ResultV
            // запускается" line. Guarded by the same cold-start check so an
            // Activity recreate (e.g. language switch) doesn't re-log it.
            com.resultv.android.vpn.AppLog.info(getString(R.string.log_app_started))
            handleDeepLinkIntent(intent)
        }
    }

    /**
     * Warm-start path: when the activity is already alive (singleTask)
     * Android delivers the resultv:// URL via onNewIntent instead of
     * onCreate, so we have to mirror the cold-start handling here.
     */
    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        // Replace the held intent so getIntent() reflects what brought us
        // back to the foreground — matches the Android contract for
        // singleTask activities.
        setIntent(intent)
        handleDeepLinkIntent(intent)
    }

    private fun handleDeepLinkIntent(intent: Intent?) {
        if (intent == null) return
        if (intent.action != Intent.ACTION_VIEW) return
        val data = intent.data ?: return
        if (!data.scheme.equals("resultv", ignoreCase = true)) return
        DeepLinkImporter.import(this, data.toString())
    }

    private fun onPowerPressed() {
        val status = VpnState.status.value
        when (status) {
            is VpnStatus.Idle, is VpnStatus.Error -> connect()
            is VpnStatus.Connecting, is VpnStatus.Connected -> disconnect()
        }
    }

    private fun connect() {
        val active = ProfileRepository.state.value.active ?: run {
            Log.w(TAG, "no active profile to connect to")
            com.resultv.android.vpn.AppLog.warning(getString(R.string.log_no_server))
            return
        }
        val configJson = BuildOptionsBuilder.buildConfig(active, filesDir.absolutePath)
            ?: run {
                Log.e(TAG, "buildConfig failed or profile empty")
                com.resultv.android.vpn.AppLog.error(getString(R.string.log_build_failed))
                return
            }

        val prepareIntent = VpnService.prepare(this)
        if (prepareIntent != null) {
            pendingConfig = configJson
            vpnPermissionLauncher.launch(prepareIntent)
        } else {
            startService(configJson)
        }
    }

    private fun disconnect() {
        // Optimistic UI: state flips immediately so the button reacts on
        // the same frame; the service repeats the same transition idempotently.
        VpnState.set(VpnStatus.Idle)
        val intent = Intent(this, ResultVpnService::class.java).apply { action = ACTION_STOP }
        startService(intent)
    }

    private fun startService(configJson: String) {
        val intent = Intent(this, ResultVpnService::class.java).apply {
            action = ACTION_START
            putExtra(EXTRA_CONFIG_JSON, configJson)
        }
        startForegroundService(intent)
    }
}

/**
 * Home top bar — brand logo + wordmark pinned left, website + Telegram
 * shortcuts pinned right. Mirrors the desktop header layout.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun HomeTopBar(
    onOpenWebsite: () -> Unit,
    onOpenTelegram: () -> Unit,
) {
    TopAppBar(
        title = {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Image(
                    painter = painterResource(R.drawable.resultv_logo),
                    contentDescription = null,
                    modifier = Modifier.size(28.dp),
                )
                Spacer(Modifier.width(8.dp))
                Text(
                    text = stringResource(R.string.app_name),
                    fontWeight = FontWeight.Bold,
                )
            }
        },
        actions = {
            IconButton(onClick = onOpenWebsite) {
                Icon(
                    imageVector = Icons.Outlined.Language,
                    contentDescription = stringResource(R.string.header_open_website),
                    tint = Brand.SecondaryText,
                )
            }
            IconButton(onClick = onOpenTelegram) {
                Icon(
                    painter = painterResource(R.drawable.ic_telegram),
                    contentDescription = stringResource(R.string.header_open_telegram),
                    tint = Brand.SecondaryText,
                )
            }
        },
        colors = TopAppBarDefaults.topAppBarColors(containerColor = Brand.Bg),
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun AppShell(
    dataDir: String,
    onPower: () -> Unit,
) {
    var tab by rememberSaveable { mutableStateOf(Tab.Home) }
    // Full-screen Logs route layered above the tab Scaffold (covers the bottom
    // nav). There's no NavController in this app, so a simple overlay flag is
    // the lightest way to push a detail screen from a settings row.
    var showLogs by rememberSaveable { mutableStateOf(false) }
    var showFaq by rememberSaveable { mutableStateOf(false) }
    // SaveableStateHolder retains each tab's `rememberSaveable` state across
    // tab switches, so returning to Proxies keeps the user's scroll position,
    // expanded subscriptions, sort mode and protocol filter instead of
    // resetting them — the previous `when` switch threw all that away every
    // time the user touched the bottom nav.
    val tabStateHolder = rememberSaveableStateHolder()

    val ctx = LocalContext.current
    Scaffold(
        topBar = {
            if (tab == Tab.Home) {
                HomeTopBar(
                    onOpenWebsite = { openUrl(ctx, WEBSITE_URL) },
                    onOpenTelegram = { openUrl(ctx, TELEGRAM_URL) },
                )
            } else {
                CenterAlignedTopAppBar(
                    title = { Text(text = stringResource(tab.titleRes)) },
                    colors = TopAppBarDefaults.centerAlignedTopAppBarColors(
                        containerColor = Brand.Bg,
                    ),
                )
            }
        },
        bottomBar = {
            NavigationBar(containerColor = Brand.Surface) {
                Tab.entries.forEach { entry ->
                    val title = stringResource(entry.titleRes)
                    NavigationBarItem(
                        selected = tab == entry,
                        onClick = { tab = entry },
                        icon = { Icon(entry.icon, contentDescription = title) },
                        label = { Text(title) },
                    )
                }
            }
        },
    ) { padding ->
        Box(modifier = Modifier.padding(padding)) {
            tabStateHolder.SaveableStateProvider(tab.name) {
                when (tab) {
                    Tab.Home -> HomeScreen(
                        onPowerPressed = onPower,
                        onOpenProxies = { tab = Tab.Proxies },
                        onOpenAdd = { tab = Tab.Add },
                    )
                    Tab.Proxies -> ProxiesScreen(onAddPressed = { tab = Tab.Add })
                    Tab.Add -> AddScreen(
                        dataDir = dataDir,
                        onDone = { tab = Tab.Proxies },
                    )
                    Tab.Settings -> SettingsScreen(
                        onOpenLogs = { showLogs = true },
                        onOpenFaq = { showFaq = true },
                    )
                }
            }
        }
    }

    if (showLogs) {
        BackHandler { showLogs = false }
        LogsScreen(onBack = { showLogs = false })
    }

    if (showFaq) {
        BackHandler { showFaq = false }
        FaqScreen(onBack = { showFaq = false })
    }
}
