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
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import android.content.ContextWrapper
import com.resultv.android.R
import com.resultv.android.locale.LocaleManager
import android.widget.Toast
import androidx.compose.runtime.rememberCoroutineScope
import com.resultv.android.vpn.RoutingProfile
import com.resultv.android.vpn.RoutingProfileCompiler
import com.resultv.android.vpn.RoutingProfileRepository
import com.resultv.android.vpn.parseRoutingMergeResult
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import mobile.Mobile
import com.resultv.android.theme.CategoryTint
import com.resultv.android.theme.RvCategory
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.RvRadius
import com.resultv.android.theme.RvSpace
import com.resultv.android.theme.rvBorder
import com.resultv.android.ui.components.DarkSheetSystemBars
import com.resultv.android.ui.components.SettingIcon
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
    val itemsRes: Int,
    val icon: ImageVector,
    val tint: CategoryTint,
) {
    Network(
        R.string.settings_group_network, R.string.settings_group_network_desc,
        R.string.settings_group_network_items, Icons.Outlined.Public, RvCategory.Main,
    ),
    // Описание своё, а не `rules_section_smart_subtitle`: тот же текст стоит
    // заголовком первого раздела ВНУТРИ шторки, и одна и та же фраза читалась
    // дважды подряд — в шапке и строкой ниже.
    Routing(
        R.string.tab_rules, R.string.settings_group_routing_desc,
        R.string.settings_group_routing_items, Icons.Outlined.AltRoute, RvCategory.Blue,
    ),
    Security(
        R.string.settings_group_security, R.string.settings_group_security_desc,
        R.string.settings_group_security_items, Icons.Outlined.Security, RvCategory.Red,
    ),
    AdBlock(
        AdBlockGroupRes.label, AdBlockGroupRes.desc,
        AdBlockGroupRes.items, Icons.Outlined.Block, RvCategory.Red,
    ),
    Subscriptions(
        R.string.settings_group_subscriptions, R.string.settings_group_subscriptions_desc,
        R.string.settings_group_subscriptions_items, Icons.Outlined.RssFeed, RvCategory.Amber,
    ),
    Appearance(
        R.string.settings_group_appearance, R.string.settings_group_appearance_desc,
        R.string.settings_group_appearance_items, Icons.Outlined.Palette, RvCategory.Violet,
    ),
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(onOpenLogs: () -> Unit = {}, onOpenCertWizard: () -> Unit = {}) {
    val settings by SettingsRepository.state.collectAsStateWithLifecycle()
    var activeSheet by rememberSaveable { mutableStateOf<SettingsSubcategory?>(null) }
    // Профили маршрутизации — ВЛОЖЕННАЯ шторка поверх «Правил», а не
    // отдельный экран. Прошлая версия гасила лист и открывала
    // полноэкранный маршрут, из-за чего закрытие возвращало в список
    // настроек, а не туда, откуда её открыли.
    var routingProfilesOpen by rememberSaveable { mutableStateOf(false) }
    // Редактор — ТРЕТЬЯ шторка, поверх профилей: закрылась, и ты в списке,
    // откуда её открыл. Тот же приём, что уровнем выше.
    var editorOpen by remember { mutableStateOf(false) }
    var editorProfile by remember { mutableStateOf<RoutingProfile?>(null) }
    var editorBusy by remember { mutableStateOf(false) }
    val editorSheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    val editorScope = rememberCoroutineScope()
    val routingSheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = RvSpace.page, vertical = RvSpace.nest2),
        verticalArrangement = Arrangement.spacedBy(RvSpace.nest1),
    ) {
        CategoryHeader(stringResource(R.string.settings_cat_connection))
        SettingsCard {
            SubcategoryRow(SettingsSubcategory.Network) { activeSheet = SettingsSubcategory.Network }
            HorizontalDivider(color = RvColor.whiteA10)
            SubcategoryRow(SettingsSubcategory.Routing) { activeSheet = SettingsSubcategory.Routing }
        }

        CategoryHeader(stringResource(R.string.settings_cat_security))
        SettingsCard {
            SubcategoryRow(SettingsSubcategory.Security) { activeSheet = SettingsSubcategory.Security }
            if (com.resultv.android.BuildConfig.DNS_ADBLOCK) {
                HorizontalDivider(color = RvColor.whiteA10)
                SubcategoryRow(SettingsSubcategory.AdBlock) { activeSheet = SettingsSubcategory.AdBlock }
            }
        }

        CategoryHeader(stringResource(R.string.settings_cat_app))
        SettingsCard {
            SubcategoryRow(SettingsSubcategory.Appearance) { activeSheet = SettingsSubcategory.Appearance }
            HorizontalDivider(color = RvColor.whiteA10)
            SubcategoryRow(SettingsSubcategory.Subscriptions) { activeSheet = SettingsSubcategory.Subscriptions }
            HorizontalDivider(color = RvColor.whiteA10)
            NavRow(
                label = stringResource(R.string.settings_group_logs),
                icon = Icons.Outlined.Article,
                tint = RvCategory.Cyan,
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
            containerColor = RvColor.Grey,
            dragHandle = { BottomSheetDefaults.DragHandle() },
        ) {
            DarkSheetSystemBars()
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .verticalScroll(rememberScrollState())
                    .padding(horizontal = 16.dp, vertical = 8.dp)
                    // Не safe area: её лист уже держит сам —
                    // ModalBottomSheet кладёт на содержимое
                    // BottomSheetDefaults.windowInsets (safeDrawing снизу). Это
                    // просто поле, чтобы последняя строка не упиралась в панель.
                    .padding(bottom = 24.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                // Sheet Header
                activeSheet?.let { sheet ->
                    Row(
                        modifier = Modifier.padding(bottom = RvSpace.nest1),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2)
                    ) {
                        SettingIcon(sheet.icon, sheet.tint)
                        Column {
                            Text(stringResource(sheet.labelRes), style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold)
                            Text(stringResource(sheet.descRes), style = MaterialTheme.typography.bodyMedium, color = RvColor.whiteA50)
                        }
                    }
                }

                when (activeSheet) {
                    SettingsSubcategory.Subscriptions -> SubscriptionsGroup(settings)
                    SettingsSubcategory.Security -> SecurityGroup(settings)
                    SettingsSubcategory.AdBlock -> AdBlockGroupContent(
                        settings,
                        // Dismiss the sheet first, or the wizard opens beneath
                        // it — this sheet is a sub-window layered above the
                        // Scaffold, so a full-screen route can't cover it.
                        onOpenCertWizard = {
                            activeSheet = null
                            onOpenCertWizard()
                        },
                    )
                    SettingsSubcategory.Network -> NetworkGroup(settings)
                    SettingsSubcategory.Appearance -> AppearanceGroup(onBeforeRecreate = { activeSheet = null })
                    SettingsSubcategory.Routing -> RulesScreen(
                        // Родительская шторка НЕ гасится: вложенная встаёт
                        // поверх, и закрытие возвращает сюда же.
                        onOpenRoutingProfiles = { routingProfilesOpen = true },
                    )
                    null -> {}
                }
            }
        }
    }

    if (routingProfilesOpen) {
        ModalBottomSheet(
            onDismissRequest = { routingProfilesOpen = false },
            modifier = Modifier.windowInsetsPadding(WindowInsets.statusBars),
            sheetState = routingSheetState,
            containerColor = RvColor.Grey,
            dragHandle = { BottomSheetDefaults.DragHandle() },
        ) {
            DarkSheetSystemBars()
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .verticalScroll(rememberScrollState())
                    .padding(horizontal = 16.dp, vertical = 8.dp)
                    // Не safe area: её шторка держит сама. Это поле, чтобы
                    // последняя строка не упиралась в панель навигации.
                    .padding(bottom = 24.dp),
            ) {
                RoutingProfilesSheetContent(
                    dataDir = LocalContext.current.filesDir.absolutePath,
                    onEdit = { p ->
                        editorProfile = p
                        editorOpen = true
                    },
                )
            }
        }
    }

    if (editorOpen) {
        val ctx = LocalContext.current
        val dataDir = ctx.filesDir.absolutePath
        ModalBottomSheet(
            onDismissRequest = { if (!editorBusy) editorOpen = false },
            modifier = Modifier.windowInsetsPadding(WindowInsets.statusBars),
            sheetState = editorSheetState,
            containerColor = RvColor.Grey,
            dragHandle = { BottomSheetDefaults.DragHandle() },
        ) {
            DarkSheetSystemBars()
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .verticalScroll(rememberScrollState())
                    .padding(horizontal = 16.dp, vertical = 8.dp)
                    .padding(bottom = 24.dp),
            ) {
                RoutingProfileEditorContent(
                    profile = editorProfile,
                    busy = editorBusy,
                    onSave = { edited ->
                        editorBusy = true
                        editorScope.launch {
                            val merged = withContext(Dispatchers.IO) {
                                runCatching {
                                    Mobile.mergeRoutingProfile(
                                        RoutingProfileRepository.storeJson(),
                                        edited.toJson().toString(),
                                        false,
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
                                // Ищем по имени и происхождению, а не по id: у
                                // нового профиля id назначает Go, и до слияния
                                // его здесь неоткуда взять.
                                val saved = state.profiles.firstOrNull {
                                    it.name == edited.name && it.source == edited.source
                                }
                                if (saved != null) {
                                    RoutingProfileCompiler.compile(saved, dataDir)
                                }
                                editorOpen = false
                            }
                            editorBusy = false
                        }
                    },
                )
            }
        }
    }
}

@Composable
private fun CategoryHeader(title: String) {
    Text(
        text = title,
        style = MaterialTheme.typography.labelMedium,
        color = RvColor.whiteA50,
        modifier = Modifier.padding(start = RvSpace.nest1, top = RvSpace.nest3, bottom = 0.dp)
    )
}

@Composable
private fun SettingsCard(content: @Composable ColumnScope.() -> Unit) {
    val shape = RoundedCornerShape(RvRadius.card)
    Card(
        shape = shape,
        colors = CardDefaults.cardColors(containerColor = RvColor.Grey),
        modifier = Modifier
            .fillMaxWidth()
            .rvBorder(shape),
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
            .padding(horizontal = RvSpace.nest1, vertical = RvSpace.nest2),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2),
    ) {
        SettingIcon(subcategory.icon, subcategory.tint)
        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(RvSpace.xs),
        ) {
            Text(
                stringResource(subcategory.labelRes),
                style = MaterialTheme.typography.titleMedium,
            )
            // Одна строка с обрезкой: состав раздела виден без захода внутрь,
            // а длинный список не разгоняет строку по высоте.
            Text(
                stringResource(subcategory.itemsRes),
                style = MaterialTheme.typography.bodyMedium,
                color = RvColor.whiteA50,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
        Icon(
            imageVector = Icons.AutoMirrored.Outlined.KeyboardArrowRight,
            contentDescription = null,
            tint = RvColor.iconDefault,
        )
    }
}

/** A settings row that navigates elsewhere (full screen) instead of opening a sheet. */
@Composable
internal fun NavRow(
    label: String,
    icon: ImageVector,
    tint: CategoryTint,
    onClick: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .padding(horizontal = RvSpace.nest1, vertical = RvSpace.nest1),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2),
    ) {
        SettingIcon(icon, tint)
        Text(
            label,
            style = MaterialTheme.typography.titleMedium,
            fontWeight = FontWeight.Medium,
            modifier = Modifier.weight(1f),
        )
        Icon(
            imageVector = Icons.AutoMirrored.Outlined.KeyboardArrowRight,
            contentDescription = null,
            tint = RvColor.iconDefault,
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
        tint = RvCategory.Emerald,
        checked = settings.subscriptionAutoUpdate,
        onCheckedChange = { SettingsRepository.setSubscriptionAutoUpdate(it) },
    )
    HorizontalDivider(color = RvColor.whiteA10)
    IntervalRow(
        title = stringResource(R.string.settings_sub_interval_title),
        subtitle = stringResource(R.string.settings_sub_interval_desc),
        icon = Icons.Outlined.Timer,
        tint = RvCategory.Amber,
        hours = settings.subscriptionUpdateIntervalHours,
        onChange = { SettingsRepository.setSubscriptionUpdateIntervalHours(it) },
    )
    HorizontalDivider(color = RvColor.whiteA10)
    ToggleRow(
        title = stringResource(R.string.settings_sub_hwid_title),
        subtitle = stringResource(R.string.settings_sub_hwid_desc),
        icon = Icons.Outlined.Fingerprint,
        tint = RvCategory.Violet,
        checked = settings.subscriptionSendHwid,
        onCheckedChange = { SettingsRepository.setSubscriptionSendHwid(it) },
    )
    HorizontalDivider(color = RvColor.whiteA10)
    TextFieldRow(
        title = stringResource(R.string.settings_sub_ua_title),
        subtitle = stringResource(R.string.settings_sub_ua_desc),
        icon = Icons.Outlined.Badge,
        tint = RvCategory.Slate,
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
        tint = RvCategory.Red,
        checked = settings.killSwitch,
        onCheckedChange = { SettingsRepository.setKillSwitch(it) },
    )
}

@OptIn(ExperimentalMaterial3Api::class, androidx.compose.foundation.layout.ExperimentalLayoutApi::class)
@Composable
private fun NetworkGroup(settings: com.resultv.android.vpn.SettingsState) {
    Column(
        modifier = Modifier.padding(vertical = RvSpace.nest3),
        verticalArrangement = Arrangement.spacedBy(RvSpace.nest3),
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2)
        ) {
            SettingIcon(Icons.Outlined.Dns, RvCategory.Blue)
            Column {
                Text("DNS", style = MaterialTheme.typography.bodyLarge)
                Text(
                    stringResource(R.string.settings_dns_hint),
                    style = MaterialTheme.typography.bodySmall,
                    color = RvColor.whiteA50,
                )
            }
        }

        androidx.compose.foundation.layout.FlowRow(
            modifier = Modifier.padding(start = 50.dp),
            horizontalArrangement = Arrangement.spacedBy(RvSpace.xs),
            verticalArrangement = Arrangement.spacedBy(RvSpace.xs),
        ) {
            dnsPresets().forEach { p ->
                FilterChip(
                    selected = settings.dnsPreset == p.key,
                    onClick = { SettingsRepository.setDnsPreset(p.key, "") },
                    label = { Text(p.label) },
                    colors = FilterChipDefaults.filterChipColors(
                        selectedContainerColor = RvColor.Main.copy(alpha = 0.2f),
                        selectedLabelColor = RvColor.Second,
                    ),
                )
            }
        }
        OutlinedTextField(
            value = if (settings.dnsPreset == "Custom") settings.dnsCustom else "",
            onValueChange = { SettingsRepository.setDnsPreset("Custom", it) },
            modifier = Modifier.fillMaxWidth().padding(start = 50.dp, top = RvSpace.nest3),
            singleLine = true,
            placeholder = { Text(stringResource(R.string.settings_dns_custom_placeholder)) },
        )
        Text(
            stringResource(R.string.settings_dns_private_warning),
            style = MaterialTheme.typography.bodyMedium,
            color = RvColor.whiteA50,
            modifier = Modifier.padding(start = 50.dp, top = RvSpace.nest3),
        )
    }
    HorizontalDivider(color = RvColor.whiteA10, modifier = Modifier.padding(vertical = RvSpace.nest3))
    ToggleRow(
        title = stringResource(R.string.settings_bypass_lan),
        subtitle = stringResource(R.string.settings_bypass_lan_subtitle),
        icon = Icons.Outlined.Lan,
        tint = RvCategory.Violet,
        checked = settings.bypassLan,
        onCheckedChange = { SettingsRepository.setBypassLan(it) },
    )
    HorizontalDivider(color = RvColor.whiteA10)
    ToggleRow(
        title = stringResource(R.string.settings_ipv6),
        subtitle = stringResource(R.string.settings_ipv6_subtitle),
        icon = Icons.Outlined.Language,
        tint = RvCategory.Blue,
        checked = settings.ipv6,
        onCheckedChange = { SettingsRepository.setIpv6(it) },
    )
    PingGroup(settings)
}

/**
 * Что меряет пинг в списке серверов.
 *
 * «Авто» — сегодняшняя проба по протоколу. ICMP меряет путь до адреса узла и
 * потому применим к любому протоколу. Два http-типа отвечают на другой вопрос
 * — «узел действительно возит трафик», а не «порт открыт», — и стоят
 * одноразового движка на узел, поэтому их выбирают осознанно.
 *
 * Автоподбор и кил-свитч эту настройку не читают: они работают без человека
 * на каждом подключении, и движок на узел сделал бы подключение медленнее.
 */
@OptIn(ExperimentalMaterial3Api::class, androidx.compose.foundation.layout.ExperimentalLayoutApi::class)
@Composable
private fun PingGroup(settings: com.resultv.android.vpn.SettingsState) {
    HorizontalDivider(color = RvColor.whiteA10, modifier = Modifier.padding(vertical = RvSpace.nest3))
    Column(
        modifier = Modifier.padding(vertical = RvSpace.nest3),
        verticalArrangement = Arrangement.spacedBy(RvSpace.nest3),
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2),
        ) {
            SettingIcon(Icons.Outlined.NetworkPing, RvCategory.Cyan)
            Column {
                Text(stringResource(R.string.settings_ping), style = MaterialTheme.typography.bodyLarge)
                Text(
                    stringResource(R.string.settings_ping_hint),
                    style = MaterialTheme.typography.bodySmall,
                    color = RvColor.whiteA50,
                )
            }
        }
        androidx.compose.foundation.layout.FlowRow(
            modifier = Modifier.padding(start = 50.dp),
            horizontalArrangement = Arrangement.spacedBy(RvSpace.xs),
            verticalArrangement = Arrangement.spacedBy(RvSpace.xs),
        ) {
            listOf(
                "auto" to stringResource(R.string.settings_ping_type_auto),
                "icmp" to stringResource(R.string.settings_ping_type_icmp),
                "http_get" to stringResource(R.string.settings_ping_type_http_get),
                "http_head" to stringResource(R.string.settings_ping_type_http_head),
            ).forEach { (key, label) ->
                FilterChip(
                    selected = settings.pingType == key,
                    onClick = { SettingsRepository.setPingType(key) },
                    label = { Text(label) },
                    colors = FilterChipDefaults.filterChipColors(
                        selectedContainerColor = RvColor.Main.copy(alpha = 0.2f),
                        selectedLabelColor = RvColor.Second,
                    ),
                )
            }
        }

        // Адрес и бюджет показываются всегда, а не только под http-типами:
        // бюджет действует и на ICMP, и пряча поле, мы бы прятали причину,
        // по которой проба вернулась именно так.
        var urlDraft by rememberSaveable(settings.pingTestUrl) { mutableStateOf(settings.pingTestUrl) }
        val urlInvalid = urlDraft.isNotBlank() && !SettingsRepository.isValidPingTestUrl(urlDraft)
        OutlinedTextField(
            value = urlDraft,
            onValueChange = {
                urlDraft = it
                if (it.isBlank() || SettingsRepository.isValidPingTestUrl(it)) {
                    SettingsRepository.setPingTestUrl(it)
                }
            },
            modifier = Modifier.fillMaxWidth().padding(start = 50.dp),
            singleLine = true,
            isError = urlInvalid,
            label = { Text(stringResource(R.string.settings_ping_url)) },
            supportingText = {
                Text(
                    stringResource(
                        if (urlInvalid) R.string.settings_ping_url_invalid
                        else R.string.settings_ping_url_subtitle
                    ),
                    style = MaterialTheme.typography.bodySmall,
                    color = if (urlInvalid) RvColor.Errors else RvColor.whiteA50,
                )
            },
        )

        var timeoutDraft by rememberSaveable(settings.pingTimeoutSec) {
            mutableStateOf(if (settings.pingTimeoutSec == 0) "" else settings.pingTimeoutSec.toString())
        }
        OutlinedTextField(
            value = timeoutDraft,
            onValueChange = { raw ->
                timeoutDraft = raw.filter { ch -> ch.isDigit() }.take(2)
                SettingsRepository.setPingTimeoutSec(SettingsRepository.normalizePingTimeoutSec(timeoutDraft))
            },
            modifier = Modifier.fillMaxWidth().padding(start = 50.dp),
            singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
            label = { Text(stringResource(R.string.settings_ping_timeout)) },
            supportingText = {
                Text(
                    stringResource(R.string.settings_ping_timeout_subtitle),
                    style = MaterialTheme.typography.bodySmall,
                    color = RvColor.whiteA50,
                )
            },
        )
    }
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
            .padding(vertical = RvSpace.nest2),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        SettingIcon(Icons.Outlined.Translate, RvCategory.Violet)
        Spacer(modifier = Modifier.width(RvSpace.nest2))
        Column(modifier = Modifier.weight(1f)) {
            // Название «Язык» и текущее значение справа говорят всё сами —
            // отдельное описание под ним было лишним.
            Text(
                stringResource(R.string.settings_appearance_language_title),
                style = MaterialTheme.typography.bodyLarge,
            )
        }
        Box {
            TextButton(onClick = { menuOpen = true }) {
                Text(currentLang, color = RvColor.Second)
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
                                    tint = RvColor.Second,
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
internal fun ToggleRow(
    title: String,
    subtitle: String,
    icon: ImageVector,
    tint: CategoryTint,
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
    enabled: Boolean = true,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = RvSpace.nest3),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2),
    ) {
        SettingIcon(icon, tint)
        Column(modifier = Modifier.weight(1f)) {
            Text(title, style = MaterialTheme.typography.bodyLarge)
            Text(subtitle, style = MaterialTheme.typography.bodySmall, color = RvColor.whiteA50)
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
    tint: CategoryTint,
    hours: Int,
    onChange: (Int) -> Unit,
) {
    var raw by remember(hours) { mutableStateOf(hours.toString()) }
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = RvSpace.nest3),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2),
    ) {
        SettingIcon(icon, tint)
        Column(modifier = Modifier.weight(1f)) {
            Text(title, style = MaterialTheme.typography.bodyLarge)
            Text(subtitle, style = MaterialTheme.typography.bodySmall, color = RvColor.whiteA50)
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
    tint: CategoryTint,
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
            .padding(vertical = RvSpace.nest3),
        verticalArrangement = Arrangement.spacedBy(RvSpace.nest3),
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2)
        ) {
            SettingIcon(icon, tint)
            Text(title, style = MaterialTheme.typography.bodyLarge)
        }
        OutlinedTextField(
            value = value,
            onValueChange = { value = it },
            modifier = Modifier.fillMaxWidth().padding(start = 50.dp),
            singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = keyboardType),
        )
        Text(subtitle, style = MaterialTheme.typography.bodySmall, color = RvColor.whiteA50, modifier = Modifier.padding(start = 50.dp))
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
            modifier = Modifier.fillMaxWidth().padding(horizontal = RvSpace.nest1, vertical = RvSpace.nest1),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2),
        ) {
            SettingIcon(
                icon = Icons.Outlined.Info,
                tint = RvCategory.Violet,
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
                    color = RvColor.whiteA50,
                )
            }
        }
    }
}
