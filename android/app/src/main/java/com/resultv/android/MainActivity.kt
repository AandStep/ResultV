package com.resultv.android

import android.content.Context
import android.content.Intent
import android.net.VpnService
import android.os.Bundle
import android.util.Log
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Apps
import androidx.compose.material.icons.outlined.Home
import androidx.compose.material.icons.outlined.List
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material3.CenterAlignedTopAppBar
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import com.resultv.android.locale.LocaleManager
import com.resultv.android.theme.Brand
import com.resultv.android.theme.ResultVTheme
import com.resultv.android.ui.screens.AddScreen
import com.resultv.android.ui.screens.HomeScreen
import com.resultv.android.ui.screens.ProxiesScreen
import com.resultv.android.ui.screens.RulesScreen
import com.resultv.android.ui.screens.SettingsScreen
import com.resultv.android.vpn.ACTION_START
import com.resultv.android.vpn.ACTION_STOP
import com.resultv.android.vpn.AppRoutingRepository
import com.resultv.android.vpn.EXTRA_CONFIG_JSON
import com.resultv.android.vpn.Profile
import com.resultv.android.vpn.ProfileRepository
import com.resultv.android.vpn.ResultVpnService
import com.resultv.android.vpn.VpnState
import com.resultv.android.vpn.VpnStatus
import mobile.Mobile
import org.json.JSONObject

private const val TAG = "ResultV/UI"

private enum class Tab(
    @StringRes val titleRes: Int,
    val icon: ImageVector,
) {
    Home(R.string.tab_home, Icons.Outlined.Home),
    Proxies(R.string.tab_proxies, Icons.Outlined.List),
    Add(R.string.tab_add, Icons.Outlined.Add),
    Rules(R.string.tab_rules, Icons.Outlined.Apps),
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
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        // Swap from the white splash theme (manifest) to the real app theme
        // BEFORE super.onCreate, so the system's first frame after the
        // splash window matches Compose's dark background instead of
        // flashing white.
        setTheme(R.style.Theme_ResultV)
        super.onCreate(savedInstanceState)
        // Edge-to-edge: app draws behind status + nav bars; Scaffold's TopAppBar
        // and NavigationBar consume the window-inset paddings so content above
        // the gesture nav stays tappable.
        enableEdgeToEdge()
        ProfileRepository.init(applicationContext)
        com.resultv.android.vpn.SubscriptionRepository.init(applicationContext)
        AppRoutingRepository.init(applicationContext)
        com.resultv.android.vpn.RoutingRulesRepository.init(applicationContext)
        com.resultv.android.vpn.SettingsRepository.init(applicationContext)
        seedFromBuildConfigIfEmpty()
        setContent {
            ResultVTheme {
                AppShell(
                    dataDir = filesDir.absolutePath,
                    onPower = ::onPowerPressed,
                )
            }
        }
    }

    /** First-run convenience: import the URI from `local.properties` once. */
    private fun seedFromBuildConfigIfEmpty() {
        val seed = BuildConfig.VLESS_URI
        if (seed.isBlank()) return
        if (ProfileRepository.state.value.profiles.isNotEmpty()) return
        val name = nameFromUri(seed) ?: "PoC profile"
        ProfileRepository.add(Profile.fromUri(name, seed))
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
            return
        }
        val dataDir = filesDir.absolutePath
        val excludedDomains = com.resultv.android.vpn.RoutingRulesRepository
            .state.value.domainExclusions.joinToString(",")
        val configJson = try {
            when {
                active.entryJson.isNotBlank() ->
                    Mobile.buildSingBoxConfigFromEntry(
                        active.entryJson, dataDir, "8.8.8.8,1.1.1.1", excludedDomains,
                    )
                active.uri.isNotBlank() ->
                    Mobile.buildSingBoxConfig(
                        active.uri, dataDir, "8.8.8.8,1.1.1.1", excludedDomains,
                    )
                else -> {
                    Log.e(TAG, "active profile has neither URI nor entry JSON")
                    return
                }
            }
        } catch (t: Throwable) {
            Log.e(TAG, "buildSingBoxConfig failed", t)
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

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun AppShell(
    dataDir: String,
    onPower: () -> Unit,
) {
    var tab by remember { mutableStateOf(Tab.Home) }

    Scaffold(
        topBar = {
            CenterAlignedTopAppBar(
                title = {
                    Text(
                        text = if (tab == Tab.Home) stringResource(R.string.app_name)
                        else stringResource(tab.titleRes),
                    )
                },
                colors = TopAppBarDefaults.centerAlignedTopAppBarColors(
                    containerColor = Brand.Bg,
                ),
            )
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
                Tab.Rules -> RulesScreen()
                Tab.Settings -> SettingsScreen()
            }
        }
    }
}

private fun nameFromUri(uri: String): String? = runCatching {
    val parsed = JSONObject(Mobile.parseProxyURI(uri))
    parsed.optString("name").ifBlank { parsed.optString("ip").ifBlank { null } }
}.getOrNull()
