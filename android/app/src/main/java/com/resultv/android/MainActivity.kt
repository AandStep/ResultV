package com.resultv.android

import android.content.Context
import android.content.Intent
import android.graphics.Color
import android.net.VpnService
import android.os.Bundle
import android.util.Log
import androidx.activity.ComponentActivity
import androidx.activity.SystemBarStyle
import androidx.activity.compose.BackHandler
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.annotation.StringRes
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.lifecycleScope
import androidx.core.net.toUri
import androidx.annotation.DrawableRes
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.ui.Alignment
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.resultv.android.ui.screens.RoutingProfilesSheets
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.outlined.Apps
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.saveable.rememberSaveableStateHolder
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import com.resultv.android.locale.LocaleManager
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.ResultVTheme
import com.resultv.android.ui.screens.AddScreen
import com.resultv.android.ui.screens.HomeScreen
import com.resultv.android.ui.screens.CertWizardScreen
import com.resultv.android.ui.screens.LogsScreen
import com.resultv.android.ui.screens.ProxiesScreen
import com.resultv.android.ui.components.ClearFocusOnImeHide
import com.resultv.android.ui.components.HomeHeader
import com.resultv.android.ui.components.PageHeader
import com.resultv.android.ui.components.RoutingDeepLinkSheet
import com.resultv.android.ui.screens.RulesScreen
import com.resultv.android.ui.screens.SettingsScreen
import android.widget.Toast
import androidx.compose.runtime.rememberCoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import mobile.Mobile
import com.resultv.android.vpn.ACTION_START
import com.resultv.android.vpn.ACTION_STOP
import com.resultv.android.vpn.AppInventory
import com.resultv.android.vpn.AppRoutingRepository
import com.resultv.android.vpn.DeepLinkImporter
import com.resultv.android.vpn.PendingRoutingImport
import com.resultv.android.vpn.PingRepository
import com.resultv.android.vpn.ProfileRepository
import com.resultv.android.vpn.RoutingProfileCompiler
import com.resultv.android.vpn.RoutingProfileRepository
import com.resultv.android.vpn.parseRoutingMergeResult
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

/** Порядок и значки — нижняя панель мобильного макета (Figma 6859:5157). */
private enum class Tab(
    @StringRes val titleRes: Int,
    @DrawableRes val icon: Int,
    /** Заголовок страницы в шапке; у главной своя шапка. */
    @StringRes val headerRes: Int = titleRes,
    /** Экран рисует шапку сам — ей нужны его кнопки и состояние. */
    val ownHeader: Boolean = false,
) {
    Home(R.string.tab_home, R.drawable.ic_nav_home),
    Add(R.string.tab_add, R.drawable.ic_nav_add, headerRes = R.string.home_add_server),
    Proxies(R.string.tab_proxies, R.drawable.ic_nav_servers, ownHeader = true),
    Rules(R.string.tab_rules, R.drawable.ic_nav_rules, headerRes = R.string.rules_title),
    Settings(R.string.tab_settings, R.drawable.ic_nav_settings),
}

class MainActivity : ComponentActivity() {

    private var pendingStart: Boolean = false

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
            if (pendingStart) {
                pendingStart = false
                startService()
            }
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
        //
        // Both bars are pinned to the dark style instead of the default auto(),
        // which follows the *system* light/dark setting. This UI is dark at all
        // times (RvColor.Black), so on a phone in light mode auto() picked dark icons
        // — an unreadable clock over our near-black background — and painted the
        // three-button nav bar white. Until targetSdk 35 the splash theme's
        // statusBarColor/navigationBarColor hid that; API 35 ignores both, so
        // the styles have to say it here. TRANSPARENT scrims keep the bars
        // showing app content rather than a tinted band.
        enableEdgeToEdge(
            statusBarStyle = SystemBarStyle.dark(Color.TRANSPARENT),
            navigationBarStyle = SystemBarStyle.dark(Color.TRANSPARENT),
        )
        ProfileRepository.init(applicationContext)
        com.resultv.android.vpn.SubscriptionRepository.init(applicationContext)
        AppRoutingRepository.init(applicationContext)
        com.resultv.android.vpn.RoutingRulesRepository.init(applicationContext)
        com.resultv.android.vpn.RoutingProfileRepository.init(applicationContext)
        com.resultv.android.vpn.SettingsRepository.init(applicationContext)
        AppInventory.init(applicationContext)
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
        ProfileRepository.state.value.active ?: run {
            Log.w(TAG, "no active profile to connect to")
            com.resultv.android.vpn.AppLog.warning(getString(R.string.log_no_server))
            return
        }
        // The config is deliberately NOT built here and NOT put in the Intent.
        // With the expanded Smart list it reaches ~4.6 MB, which blows the ~1 MB
        // Binder transaction limit — startForegroundService then throws
        // TransactionTooLargeException and takes the app down. Building it on the
        // UI thread would also risk an ANR at that size. The service rebuilds it
        // from the same persisted state (see buildConfigFromActiveProfile), which
        // is the path always-on VPN has always used.
        val prepareIntent = VpnService.prepare(this)
        if (prepareIntent != null) {
            pendingStart = true
            vpnPermissionLauncher.launch(prepareIntent)
        } else {
            startService()
        }
    }

    private fun disconnect() {
        // Optimistic UI: state flips immediately so the button reacts on
        // the same frame; the service repeats the same transition idempotently.
        VpnState.set(VpnStatus.Idle)
        val intent = Intent(this, ResultVpnService::class.java).apply { action = ACTION_STOP }
        startService(intent)
    }

    private fun startService() {
        val intent = Intent(this, ResultVpnService::class.java).apply { action = ACTION_START }
        startForegroundService(intent)
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun AppShell(
    dataDir: String,
    onPower: () -> Unit,
) {
    ClearFocusOnImeHide()
    var tab by rememberSaveable { mutableStateOf(Tab.Home) }
    // Full-screen Logs route layered above the tab Scaffold (covers the bottom
    // nav). There's no NavController in this app, so a simple overlay flag is
    // the lightest way to push a detail screen from a settings row.
    var showLogs by rememberSaveable { mutableStateOf(false) }
    var showCertWizard by rememberSaveable { mutableStateOf(false) }
    // SaveableStateHolder retains each tab's `rememberSaveable` state across
    // tab switches, so returning to Proxies keeps the user's scroll position,
    // expanded subscriptions, sort mode and protocol filter instead of
    // resetting them — the previous `when` switch threw all that away every
    // time the user touched the bottom nav.
    val tabStateHolder = rememberSaveableStateHolder()

    // Auto-ping newly-appeared profiles, from here rather than from a screen.
    //
    // Only one tab is composed at a time (see the `when (tab)` below), and this
    // sweep used to live in HomeScreen's LaunchedEffect. Adding a server or a
    // subscription ends with `onDone` switching to the Proxies tab, which has no
    // such effect — so the new rows never got probed and each one span an
    // endless spinner (ServerRow renders "no sample yet" as a spinner) until the
    // user pressed the manual refresh. Hoisting it to the shell covers every
    // entry path and every tab.
    //
    // results.value is read directly instead of being collected as state: this
    // effect must re-run when the profile list changes, not every time a probe
    // publishes a sample (which would restart the sweep on its own output).
    val profilesForPing by ProfileRepository.state.collectAsStateWithLifecycle()
    LaunchedEffect(profilesForPing.profiles) {
        PingRepository.refreshMissing(profilesForPing.profiles)
    }

    val ctx = LocalContext.current
    Scaffold(
        topBar = {
            if (tab == Tab.Home) {
                HomeHeader(
                    onOpenWebsite = { openUrl(ctx, WEBSITE_URL) },
                    onOpenTelegram = { openUrl(ctx, TELEGRAM_URL) },
                )
            } else if (!tab.ownHeader) {
                PageHeader(
                    title = stringResource(tab.headerRes),
                    modifier = Modifier
                        .background(RvColor.Black)
                        .windowInsetsPadding(WindowInsets.statusBars),
                )
            }
        },
        bottomBar = { BottomBar(selected = tab, onSelect = { tab = it }) },
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
                    Tab.Rules -> {
                        var profilesOpen by rememberSaveable { mutableStateOf(false) }
                        Column(
                            modifier = Modifier
                                .fillMaxSize()
                                .verticalScroll(rememberScrollState())
                                .padding(horizontal = 12.dp),
                        ) {
                            RulesScreen(onOpenRoutingProfiles = { profilesOpen = true })
                        }
                        RoutingProfilesSheets(open = profilesOpen, onDismiss = { profilesOpen = false })
                    }
                    Tab.Settings -> SettingsScreen(
                        onOpenLogs = { showLogs = true },
                        onOpenCertWizard = { showCertWizard = true },
                    )
                }
            }
        }
    }

    if (showLogs) {
        BackHandler { showLogs = false }
        LogsScreen(onBack = { showLogs = false })
    }

    // Мастер сертификата — часть браузерного ad-block, которого в Play-сборке
    // нет. Флаг здесь, а не только на кнопке, чтобы восстановленное состояние
    // (rememberSaveable) не открыло экран-заглушку после смены дистрибутива.
    if (com.resultv.android.BuildConfig.BROWSER_ADBLOCK && showCertWizard) {
        BackHandler { showCertWizard = false }
        CertWizardScreen(dataDir = dataDir, onClose = { showCertWizard = false })
    }

    RoutingImportSheet(dataDir)
}

/**
 * Нижняя панель макета (Figma 6859:5157): пять кнопок 44 dp одними значками
 * 20 dp, поля панели 24 по бокам и 12 сверху-снизу, выбранная — на зелёной
 * подложке 10 %. Подпись уходит в contentDescription, а не на экран.
 */
@Composable
private fun BottomBar(selected: Tab, onSelect: (Tab) -> Unit) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .background(RvColor.Grey)
            .drawBehind {
                drawLine(
                    color = RvColor.whiteA20,
                    start = Offset(0f, 0f),
                    end = Offset(size.width, 0f),
                    strokeWidth = 1.dp.toPx(),
                )
            }
            .windowInsetsPadding(WindowInsets.navigationBars)
            .padding(horizontal = 24.dp, vertical = 12.dp),
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
        ) {
            Tab.entries.forEach { entry ->
                val isSelected = entry == selected
                val title = stringResource(entry.titleRes)
                Box(
                    modifier = Modifier
                        .size(44.dp)
                        .clip(RoundedCornerShape(8.dp))
                        .background(if (isSelected) RvColor.mainA10 else androidx.compose.ui.graphics.Color.Transparent)
                        .selectable(
                            selected = isSelected,
                            role = Role.Tab,
                            onClick = { onSelect(entry) },
                        )
                        .semantics { contentDescription = title },
                    contentAlignment = Alignment.Center,
                ) {
                    Icon(
                        painter = painterResource(entry.icon),
                        contentDescription = null,
                        tint = if (isSelected) RvColor.Main else RvColor.whiteA50,
                        modifier = Modifier.size(20.dp),
                    )
                }
            }
        }
    }
}

/**
 * Лист «что принесла ссылка маршрутизации».
 *
 * Живёт в корне, поверх чего угодно: ссылка могла прийти, пока открыт любой
 * экран, а intent доходит раньше, чем что-либо отрисовано.
 *
 * Согласие делает три вещи по порядку: сливает профиль с хранилищем (правило
 * замены — в Go, один экземпляр с тестами), сохраняет и собирает правила.
 * Сборка ходит в сеть, поэтому до её конца кнопки заблокированы: иначе лист
 * закрылся бы раньше, чем что-то произошло.
 */
@Composable
private fun RoutingImportSheet(dataDir: String) {
    val pending by PendingRoutingImport.pending.collectAsStateWithLifecycle()
    var busy by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    val ctx = LocalContext.current

    val profile = pending ?: return
    RoutingDeepLinkSheet(
        profile = profile,
        busy = busy,
        onDismiss = { if (!busy) PendingRoutingImport.clear() },
        onAccept = {
            busy = true
            scope.launch {
                val merged = withContext(Dispatchers.IO) {
                    runCatching {
                        Mobile.mergeRoutingProfile(
                            RoutingProfileRepository.storeJson(),
                            profile.toJson().toString(),
                            true,
                        )
                    }.getOrNull()
                }
                val state = merged?.let { parseRoutingMergeResult(it) }
                if (state == null) {
                    Toast.makeText(
                        ctx,
                        ctx.getString(R.string.routing_import_failed, "merge failed"),
                        Toast.LENGTH_LONG,
                    ).show()
                } else {
                    RoutingProfileRepository.replaceAll(state.profiles, state.activeId)
                    val saved = state.active
                    if (saved != null) {
                        val outcome = RoutingProfileCompiler.compile(saved, dataDir)
                        val msg = when {
                            !outcome.ok ->
                                ctx.getString(R.string.routing_import_failed, outcome.error)
                            outcome.unresolved.isNotEmpty() ->
                                ctx.getString(
                                    R.string.routing_import_built_partly,
                                    outcome.unresolved.size,
                                )
                            else -> ctx.getString(R.string.routing_import_done, saved.name)
                        }
                        Toast.makeText(ctx, msg, Toast.LENGTH_LONG).show()
                    }
                }
                busy = false
                PendingRoutingImport.clear()
            }
        },
    )
}
