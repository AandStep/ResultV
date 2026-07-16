package com.resultv.android.ui.screens

import android.app.Activity
import android.content.Context
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.KeyboardArrowRight
import androidx.compose.material.icons.outlined.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import android.content.ContextWrapper
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import com.resultv.android.R
import com.resultv.android.locale.LocaleManager
import com.resultv.android.theme.Brand
import com.resultv.android.ui.components.SettingIcon
import com.resultv.android.vpn.AdBlockRepository
import com.resultv.android.vpn.SettingsRepository

private data class DnsPreset(val key: String, val label: String, val servers: String)

@Composable
private fun dnsPresets(): List<DnsPreset> = listOf(
    DnsPreset("Auto", stringResource(R.string.settings_dns_preset_auto), ""),
    DnsPreset("Google", "Google", "8.8.8.8, 8.8.4.4"),
    DnsPreset("Cloudflare", "Cloudflare", "1.1.1.1, 1.0.0.1"),
    DnsPreset("Quad9", "Quad9", "9.9.9.9, 149.112.112.112"),
)

private data class Lang(val code: String, val title: String)
private val Languages = listOf(
    Lang("EN", "English"),
    Lang("RU", "Русский"),
)

private enum class SettingsSubcategory(
    val labelRes: Int,
    val descRes: Int,
    val icon: ImageVector,
    val iconBg: Color,
    val iconTint: Color
) {
    Network(R.string.settings_group_network, R.string.settings_group_network_desc, Icons.Outlined.Public, Brand.Green.copy(alpha = 0.18f), Brand.GreenLight),
    Routing(R.string.tab_rules, R.string.rules_section_smart_subtitle, Icons.Outlined.AltRoute, Color(0xFF3b82f6).copy(alpha = 0.18f), Color(0xFF60a5fa)),
    Security(R.string.settings_group_security, R.string.settings_group_security_desc, Icons.Outlined.Security, Color(0xFFef4444).copy(alpha=0.18f), Color(0xFFf87171)),
    AdBlock(R.string.settings_group_adblock, R.string.settings_group_adblock_desc, Icons.Outlined.Block, Color(0xFFef4444).copy(alpha=0.18f), Color(0xFFf87171)),
    Subscriptions(R.string.settings_group_subscriptions, R.string.settings_group_subscriptions_desc, Icons.Outlined.RssFeed, Color(0xFFf59e0b).copy(alpha=0.18f), Color(0xFFfbbf24)),
    Appearance(R.string.settings_group_appearance, R.string.settings_group_appearance_desc, Icons.Outlined.Palette, Color(0xFF8b5cf6).copy(alpha=0.18f), Color(0xFFa78bfa)),
    Advanced(R.string.settings_group_advanced, R.string.settings_group_advanced_desc, Icons.Outlined.Tune, Color(0xFF64748b).copy(alpha=0.18f), Color(0xFF94a3b8)),
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(onOpenLogs: () -> Unit = {}, onOpenCertWizard: () -> Unit = {}) {
    val settings by SettingsRepository.state.collectAsStateWithLifecycle()
    var activeSheet by rememberSaveable { mutableStateOf<SettingsSubcategory?>(null) }
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        Text(
            stringResource(R.string.settings_title),
            style = MaterialTheme.typography.headlineLarge,
            fontWeight = FontWeight.Bold,
            modifier = Modifier.padding(bottom = 8.dp)
        )

        CategoryHeader("Соединение и Маршрутизация")
        SettingsCard {
            SubcategoryRow(SettingsSubcategory.Network) { activeSheet = SettingsSubcategory.Network }
            HorizontalDivider(color = Brand.SurfaceHigh)
            SubcategoryRow(SettingsSubcategory.Routing) { activeSheet = SettingsSubcategory.Routing }
        }

        CategoryHeader("Безопасность")
        SettingsCard {
            SubcategoryRow(SettingsSubcategory.Security) { activeSheet = SettingsSubcategory.Security }
            HorizontalDivider(color = Brand.SurfaceHigh)
            SubcategoryRow(SettingsSubcategory.AdBlock) { activeSheet = SettingsSubcategory.AdBlock }
        }

        CategoryHeader("Приложение")
        SettingsCard {
            SubcategoryRow(SettingsSubcategory.Appearance) { activeSheet = SettingsSubcategory.Appearance }
            HorizontalDivider(color = Brand.SurfaceHigh)
            SubcategoryRow(SettingsSubcategory.Subscriptions) { activeSheet = SettingsSubcategory.Subscriptions }
            HorizontalDivider(color = Brand.SurfaceHigh)
            SubcategoryRow(SettingsSubcategory.Advanced) { activeSheet = SettingsSubcategory.Advanced }
            HorizontalDivider(color = Brand.SurfaceHigh)
            NavRow(
                label = stringResource(R.string.settings_group_logs),
                icon = Icons.Outlined.Article,
                iconBg = Color(0xFF06b6d4).copy(alpha = 0.18f),
                iconTint = Color(0xFF22d3ee),
                onClick = onOpenLogs,
            )
        }

        AppInfoCard()

        Spacer(modifier = Modifier.height(32.dp))
    }

    if (activeSheet != null) {
        ModalBottomSheet(
            onDismissRequest = { activeSheet = null },
            modifier = Modifier.windowInsetsPadding(WindowInsets.statusBars),
            sheetState = sheetState,
            containerColor = Brand.Surface,
            dragHandle = { BottomSheetDefaults.DragHandle() },
        ) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .verticalScroll(rememberScrollState())
                    .padding(horizontal = 16.dp, vertical = 8.dp)
                    .padding(bottom = 48.dp), // Safe area
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                // Sheet Header
                activeSheet?.let { sheet ->
                    Row(
                        modifier = Modifier.padding(bottom = 16.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(14.dp)
                    ) {
                        SettingIcon(sheet.icon, sheet.iconBg, sheet.iconTint)
                        Column {
                            Text(stringResource(sheet.labelRes), style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold)
                            Text(stringResource(sheet.descRes), style = MaterialTheme.typography.bodyMedium, color = Brand.SecondaryText)
                        }
                    }
                }

                when (activeSheet) {
                    SettingsSubcategory.Advanced -> AdvancedGroup(settings)
                    SettingsSubcategory.Subscriptions -> SubscriptionsGroup(settings)
                    SettingsSubcategory.Security -> SecurityGroup(settings)
                    SettingsSubcategory.AdBlock -> AdBlockGroup(settings, onOpenCertWizard = onOpenCertWizard)
                    SettingsSubcategory.Network -> NetworkGroup(settings)
                    SettingsSubcategory.Appearance -> AppearanceGroup(onBeforeRecreate = { activeSheet = null })
                    SettingsSubcategory.Routing -> RulesScreen()
                    null -> {}
                }
            }
        }
    }
}

@Composable
private fun CategoryHeader(title: String) {
    Text(
        text = title,
        style = MaterialTheme.typography.labelMedium,
        color = Brand.SecondaryText,
        modifier = Modifier.padding(start = 16.dp, top = 8.dp, bottom = 0.dp)
    )
}

@Composable
private fun SettingsCard(content: @Composable ColumnScope.() -> Unit) {
    Card(
        shape = RoundedCornerShape(20.dp),
        colors = CardDefaults.cardColors(containerColor = Brand.Surface),
        modifier = Modifier.fillMaxWidth()
    ) {
        Column {
            content()
        }
    }
}

@Composable
private fun SubcategoryRow(subcategory: SettingsSubcategory, onClick: () -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .padding(horizontal = 18.dp, vertical = 16.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        SettingIcon(subcategory.icon, subcategory.iconBg, subcategory.iconTint)
        Text(
            stringResource(subcategory.labelRes),
            style = MaterialTheme.typography.titleMedium,
            fontWeight = FontWeight.Medium,
            modifier = Modifier.weight(1f),
        )
        Icon(
            imageVector = Icons.AutoMirrored.Outlined.KeyboardArrowRight,
            contentDescription = null,
            tint = Brand.MutedText,
        )
    }
}

/** A settings row that navigates elsewhere (full screen) instead of opening a sheet. */
@Composable
private fun NavRow(
    label: String,
    icon: ImageVector,
    iconBg: Color,
    iconTint: Color,
    onClick: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .padding(horizontal = 18.dp, vertical = 16.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        SettingIcon(icon, iconBg, iconTint)
        Text(
            label,
            style = MaterialTheme.typography.titleMedium,
            fontWeight = FontWeight.Medium,
            modifier = Modifier.weight(1f),
        )
        Icon(
            imageVector = Icons.AutoMirrored.Outlined.KeyboardArrowRight,
            contentDescription = null,
            tint = Brand.MutedText,
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun AdvancedGroup(settings: com.resultv.android.vpn.SettingsState) {
    ToggleRow(
        title = stringResource(R.string.settings_ipv6),
        subtitle = stringResource(R.string.settings_ipv6_subtitle),
        icon = Icons.Outlined.Language,
        iconBg = Color(0xFF3b82f6).copy(alpha = 0.18f),
        iconTint = Color(0xFF60a5fa),
        checked = settings.ipv6,
        onCheckedChange = { SettingsRepository.setIpv6(it) },
    )
}

@Composable
private fun AdBlockGroup(settings: com.resultv.android.vpn.SettingsState, onOpenCertWizard: () -> Unit) {
    ToggleRow(
        title = stringResource(R.string.settings_adblock),
        subtitle = stringResource(R.string.settings_adblock_subtitle),
        icon = Icons.Outlined.Block,
        iconBg = Color(0xFFef4444).copy(alpha = 0.18f),
        iconTint = Color(0xFFf87171),
        checked = settings.adblock,
        onCheckedChange = {
            SettingsRepository.setAdblock(it)
            // Warm the SRS cache so the next connect references local lists
            // instead of waiting on sing-box's remote fetch. Safe no-op when
            // already fresh (24h TTL).
            if (it) AdBlockRepository.refreshAsync()
        },
    )
    if (android.os.Build.VERSION.SDK_INT >= android.os.Build.VERSION_CODES.Q) {
        HorizontalDivider(color = Brand.SurfaceHigh)
        BrowserAdBlockRow(settings, onOpenCertWizard)
    }
}

@Composable
private fun BrowserAdBlockRow(
    settings: com.resultv.android.vpn.SettingsState,
    onOpenCertWizard: () -> Unit,
) {
    val context = androidx.compose.ui.platform.LocalContext.current
    val scope = rememberCoroutineScope()

    // The persisted trust state only refreshes on a VPN connect, so it goes
    // stale the moment the user installs or removes the cert outside the app.
    // Re-reading the trust store when this section opens keeps the status line
    // and the "Install certificate" row honest.
    var certInstalled by remember { mutableStateOf(settings.certTrustState == com.resultv.android.vpn.CertTrustState.TRUSTED) }
    LaunchedEffect(Unit) {
        certInstalled = withContext(Dispatchers.IO) {
            com.resultv.android.vpn.CertStore.isInstalled(context.filesDir.absolutePath)
        }
        SettingsRepository.setCertTrustState(
            if (certInstalled) com.resultv.android.vpn.CertTrustState.TRUSTED
            else com.resultv.android.vpn.CertTrustState.UNTRUSTED,
        )
    }

    ToggleRow(
        title = stringResource(R.string.settings_browser_adblock),
        subtitle = stringResource(R.string.settings_browser_adblock_subtitle),
        icon = Icons.Outlined.Shield,
        iconBg = Color(0xFF22c55e).copy(alpha = 0.18f),
        iconTint = Color(0xFF4ade80),
        checked = settings.browserAdBlock,
        onCheckedChange = { enabled ->
            if (enabled) {
                scope.launch(Dispatchers.IO) {
                    mobile.Mobile.fetchFilterLists(context.filesDir.absolutePath)
                }
                SettingsRepository.setBrowserAdBlock(true)
                // Turning the feature on without a trusted cert does nothing
                // useful, so the toggle doubles as the wizard's entry point.
                if (!certInstalled) onOpenCertWizard()
            } else {
                SettingsRepository.setBrowserAdBlock(false)
            }
        },
    )
    Text(
        text = stringResource(
            if (certInstalled) R.string.cert_status_trusted else R.string.cert_status_untrusted,
        ),
        style = MaterialTheme.typography.labelSmall,
        color = if (certInstalled) Brand.GreenLight else Brand.MutedText,
        modifier = Modifier.padding(start = 62.dp, top = 2.dp),
    )
    if (!certInstalled) {
        HorizontalDivider(color = Brand.SurfaceHigh)
        NavRow(
            label = stringResource(R.string.settings_browser_adblock_install),
            icon = Icons.Outlined.Shield,
            iconBg = Color(0xFF22c55e).copy(alpha = 0.18f),
            iconTint = Color(0xFF4ade80),
            onClick = onOpenCertWizard,
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun SubscriptionsGroup(settings: com.resultv.android.vpn.SettingsState) {
    ToggleRow(
        title = stringResource(R.string.settings_sub_auto_update_title),
        subtitle = stringResource(R.string.settings_sub_auto_update_desc),
        icon = Icons.Outlined.Sync,
        iconBg = Color(0xFF10b981).copy(alpha = 0.18f),
        iconTint = Color(0xFF34d399),
        checked = settings.subscriptionAutoUpdate,
        onCheckedChange = { SettingsRepository.setSubscriptionAutoUpdate(it) },
    )
    HorizontalDivider(color = Brand.SurfaceHigh)
    IntervalRow(
        title = stringResource(R.string.settings_sub_interval_title),
        subtitle = stringResource(R.string.settings_sub_interval_desc),
        icon = Icons.Outlined.Timer,
        iconBg = Color(0xFFf59e0b).copy(alpha = 0.18f),
        iconTint = Color(0xFFfbbf24),
        hours = settings.subscriptionUpdateIntervalHours,
        onChange = { SettingsRepository.setSubscriptionUpdateIntervalHours(it) },
    )
    HorizontalDivider(color = Brand.SurfaceHigh)
    ToggleRow(
        title = stringResource(R.string.settings_sub_hwid_title),
        subtitle = stringResource(R.string.settings_sub_hwid_desc),
        icon = Icons.Outlined.Fingerprint,
        iconBg = Color(0xFF8b5cf6).copy(alpha = 0.18f),
        iconTint = Color(0xFFa78bfa),
        checked = settings.subscriptionSendHwid,
        onCheckedChange = { SettingsRepository.setSubscriptionSendHwid(it) },
    )
    HorizontalDivider(color = Brand.SurfaceHigh)
    TextFieldRow(
        title = stringResource(R.string.settings_sub_ua_title),
        subtitle = stringResource(R.string.settings_sub_ua_desc),
        icon = Icons.Outlined.Badge,
        iconBg = Color(0xFF64748b).copy(alpha = 0.18f),
        iconTint = Color(0xFF94a3b8),
        initialValue = settings.subscriptionUserAgent,
        keyboardType = KeyboardType.Ascii,
        onCommit = { SettingsRepository.setSubscriptionUserAgent(it) },
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun SecurityGroup(settings: com.resultv.android.vpn.SettingsState) {
    ToggleRow(
        title = stringResource(R.string.settings_killswitch_title),
        subtitle = stringResource(R.string.settings_killswitch_desc),
        icon = Icons.Outlined.GppBad,
        iconBg = Color(0xFFef4444).copy(alpha = 0.18f),
        iconTint = Color(0xFFf87171),
        checked = settings.killSwitch,
        onCheckedChange = { SettingsRepository.setKillSwitch(it) },
    )
}

@OptIn(ExperimentalMaterial3Api::class, androidx.compose.foundation.layout.ExperimentalLayoutApi::class)
@Composable
private fun NetworkGroup(settings: com.resultv.android.vpn.SettingsState) {
    Column(
        modifier = Modifier.padding(vertical = 8.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(14.dp)
        ) {
            SettingIcon(Icons.Outlined.Dns, Color(0xFF3b82f6).copy(alpha = 0.18f), Color(0xFF60a5fa))
            Column {
                Text("DNS", style = MaterialTheme.typography.bodyLarge)
                Text(
                    stringResource(R.string.settings_dns_hint),
                    style = MaterialTheme.typography.bodySmall,
                    color = Brand.SecondaryText,
                )
            }
        }
        
        androidx.compose.foundation.layout.FlowRow(
            modifier = Modifier.padding(start = 50.dp),
            horizontalArrangement = Arrangement.spacedBy(6.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            dnsPresets().forEach { p ->
                FilterChip(
                    selected = settings.dnsPreset == p.key,
                    onClick = { SettingsRepository.setDnsPreset(p.key, "") },
                    label = { Text(p.label) },
                    colors = FilterChipDefaults.filterChipColors(
                        selectedContainerColor = Brand.Green.copy(alpha = 0.2f),
                        selectedLabelColor = Brand.GreenLight,
                    ),
                )
            }
        }
        OutlinedTextField(
            value = if (settings.dnsPreset == "Custom") settings.dnsCustom else "",
            onValueChange = { SettingsRepository.setDnsPreset("Custom", it) },
            modifier = Modifier.fillMaxWidth().padding(start = 50.dp, top = 8.dp),
            singleLine = true,
            placeholder = { Text(stringResource(R.string.settings_dns_custom_placeholder)) },
        )
    }
    HorizontalDivider(color = Brand.SurfaceHigh, modifier = Modifier.padding(vertical = 8.dp))
    ToggleRow(
        title = stringResource(R.string.settings_bypass_lan),
        subtitle = stringResource(R.string.settings_bypass_lan_subtitle),
        icon = Icons.Outlined.Lan,
        iconBg = Color(0xFF8b5cf6).copy(alpha = 0.18f),
        iconTint = Color(0xFFa78bfa),
        checked = settings.bypassLan,
        onCheckedChange = { SettingsRepository.setBypassLan(it) },
    )
}

/**
 * Walk the context-wrapper chain to the hosting Activity. Required because
 * `ModalBottomSheet` hosts its content in a sub-window whose
 * `LocalContext.current` is a `ContextThemeWrapper`, not the Activity — so a
 * plain `ctx as? Activity` returns null and the language tap did nothing.
 */
private tailrec fun Context.findActivity(): Activity? = when (this) {
    is Activity -> this
    is ContextWrapper -> baseContext.findActivity()
    else -> null
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun AppearanceGroup(onBeforeRecreate: () -> Unit) {
    val ctx = LocalContext.current
    val activity = remember(ctx) { ctx.findActivity() }
    val currentLang = remember(activity) { LocaleManager.currentLocale(ctx) ?: "EN" }
    var menuOpen by remember { mutableStateOf(false) }

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable { menuOpen = true }
            .padding(vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        SettingIcon(Icons.Outlined.Translate, Color(0xFF8b5cf6).copy(alpha = 0.18f), Color(0xFFa78bfa))
        Spacer(modifier = Modifier.width(14.dp))
        Column(modifier = Modifier.weight(1f)) {
            Text(
                stringResource(R.string.settings_appearance_language_title),
                style = MaterialTheme.typography.bodyLarge,
            )
            Text(
                stringResource(R.string.settings_appearance_language_desc),
                style = MaterialTheme.typography.bodySmall,
                color = Brand.SecondaryText,
            )
        }
        Box {
            TextButton(onClick = { menuOpen = true }) {
                Text(currentLang, color = Brand.GreenLight)
            }
            DropdownMenu(expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                Languages.forEach { l ->
                    DropdownMenuItem(
                        text = { Text(l.title) },
                        onClick = {
                            menuOpen = false
                            if (l.code != currentLang && activity != null) {
                                // Dismiss the sheet before recreate() so the
                                // sub-window doesn't outlive the Activity.
                                onBeforeRecreate()
                                LocaleManager.setLocale(activity, l.code)
                            }
                        },
                        trailingIcon = {
                            if (l.code == currentLang) {
                                Icon(
                                    Icons.Outlined.Check,
                                    contentDescription = null,
                                    tint = Brand.GreenLight,
                                )
                            }
                        },
                    )
                }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ToggleRow(
    title: String,
    subtitle: String,
    icon: ImageVector,
    iconBg: Color,
    iconTint: Color,
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
    enabled: Boolean = true,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        SettingIcon(icon, iconBg, iconTint)
        Column(modifier = Modifier.weight(1f)) {
            Text(title, style = MaterialTheme.typography.bodyLarge)
            Text(subtitle, style = MaterialTheme.typography.bodySmall, color = Brand.SecondaryText)
        }
        Switch(
            checked = checked,
            onCheckedChange = onCheckedChange,
            enabled = enabled,
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun IntervalRow(
    title: String,
    subtitle: String,
    icon: ImageVector,
    iconBg: Color,
    iconTint: Color,
    hours: Int,
    onChange: (Int) -> Unit,
) {
    var raw by remember(hours) { mutableStateOf(hours.toString()) }
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        SettingIcon(icon, iconBg, iconTint)
        Column(modifier = Modifier.weight(1f)) {
            Text(title, style = MaterialTheme.typography.bodyLarge)
            Text(subtitle, style = MaterialTheme.typography.bodySmall, color = Brand.SecondaryText)
        }
        OutlinedTextField(
            value = raw,
            onValueChange = { next ->
                if (next.isEmpty() || next.all { it.isDigit() }) {
                    raw = next
                    next.toIntOrNull()?.takeIf { it >= 1 }?.let(onChange)
                }
            },
            singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
            modifier = Modifier.size(width = 96.dp, height = 56.dp),
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun TextFieldRow(
    title: String,
    subtitle: String,
    icon: ImageVector,
    iconBg: Color,
    iconTint: Color,
    initialValue: String,
    keyboardType: KeyboardType,
    onCommit: (String) -> Unit,
) {
    var value by remember(initialValue) { mutableStateOf(initialValue) }
    LaunchedEffect(value) {
        if (value != initialValue) onCommit(value)
    }
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = 8.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(14.dp)
        ) {
            SettingIcon(icon, iconBg, iconTint)
            Text(title, style = MaterialTheme.typography.bodyLarge)
        }
        OutlinedTextField(
            value = value,
            onValueChange = { value = it },
            modifier = Modifier.fillMaxWidth().padding(start = 50.dp),
            singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = keyboardType),
        )
        Text(subtitle, style = MaterialTheme.typography.bodySmall, color = Brand.SecondaryText, modifier = Modifier.padding(start = 50.dp))
    }
}

// ─────────────────────────── App Info card ──────────────────────────────────

@Composable
private fun AppInfoCard() {
    val ctx = LocalContext.current
    val versionName = remember(ctx) {
        runCatching { ctx.packageManager.getPackageInfo(ctx.packageName, 0).versionName }.getOrDefault("—")
    }

    SettingsCard {
        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 18.dp, vertical = 16.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(14.dp),
        ) {
            SettingIcon(
                icon = Icons.Outlined.Info,
                bg = Color(0xFF8b5cf6).copy(alpha = 0.18f),
                tint = Color(0xFFa78bfa),
            )
            Column(modifier = Modifier.weight(1f)) {
                Text(
                    stringResource(R.string.app_name),
                    style = MaterialTheme.typography.bodyLarge,
                    fontWeight = FontWeight.Medium,
                )
                Text(
                    "v$versionName",
                    style = MaterialTheme.typography.bodySmall,
                    color = Brand.SecondaryText,
                )
            }
        }
    }
}
