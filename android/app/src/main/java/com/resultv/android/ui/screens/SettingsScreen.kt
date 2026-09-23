package com.resultv.android.ui.screens

import android.app.Activity
import android.content.Context
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import com.resultv.android.theme.SegoeUi
import androidx.compose.ui.unit.sp
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
    /** Короткое описание под заголовком шторки. */
    val descRes: Int,
    val icon: ImageVector,
    val tint: CategoryTint,
    /** Пункты раздела в списке настроек («• DNS  • IPv6 …»). */
    val itemsRes: Int,
) {
    Network(
        R.string.settings_group_network, R.string.settings_group_network_desc,
        Icons.Outlined.Public, RvCategory.Main,
        R.string.settings_group_network_items,
    ),
    Ping(
        R.string.settings_group_ping, R.string.settings_group_ping_desc,
        Icons.Outlined.NetworkPing, RvCategory.Cyan,
        R.string.settings_group_ping_items,
    ),
    Security(
        R.string.settings_group_security, R.string.settings_group_security_desc,
        Icons.Outlined.Security, RvCategory.Red,
        R.string.settings_group_security_items,
    ),
    AdBlock(
        AdBlockGroupRes.label, AdBlockGroupRes.desc,
        Icons.Outlined.Block, RvCategory.Red,
        AdBlockGroupRes.items,
    ),
    Experimental(
        R.string.settings_group_experimental, R.string.settings_group_experimental_desc,
        Icons.Outlined.Science, RvCategory.Emerald,
        R.string.settings_group_experimental_items,
    ),
    Subscriptions(
        R.string.settings_group_subscriptions, R.string.settings_group_subscriptions_desc,
        Icons.Outlined.RssFeed, RvCategory.Amber,
        R.string.settings_group_subscriptions_items,
    ),
    Appearance(
        R.string.settings_group_appearance, R.string.settings_group_appearance_desc,
        Icons.Outlined.Palette, RvCategory.Violet,
        R.string.settings_group_appearance_items,
    ),
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(onOpenLogs: () -> Unit = {}, onOpenCertWizard: () -> Unit = {}) {
    val settings by SettingsRepository.state.collectAsStateWithLifecycle()
    var activeSheet by rememberSaveable { mutableStateOf<SettingsSubcategory?>(null) }
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)

    // Макет Settings (Figma 6871:4916): ровный список карточек через 8,
    // без подзаголовков категорий — у каждой строки свой цвет и пункты.
    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(start = 12.dp, end = 12.dp, bottom = 12.dp),
        verticalArrangement = Arrangement.spacedBy(RvSpace.nest3),
    ) {
        listOfNotNull(
            SettingsSubcategory.Appearance,
            SettingsSubcategory.Subscriptions,
            SettingsSubcategory.Security,
            SettingsSubcategory.AdBlock.takeIf { com.resultv.android.BuildConfig.DNS_ADBLOCK },
            SettingsSubcategory.Network,
            SettingsSubcategory.Ping,
            SettingsSubcategory.Experimental,
        ).forEach { sub ->
            SettingsEntry(
                icon = sub.icon,
                tint = sub.tint,
                title = stringResource(sub.labelRes),
                description = stringResource(sub.itemsRes),
                onClick = { activeSheet = sub },
            )
        }
        SettingsEntry(
            icon = Icons.Outlined.Article,
            tint = RvCategory.Cyan,
            title = stringResource(R.string.settings_group_logs),
            description = stringResource(R.string.settings_group_logs_items),
            onClick = onOpenLogs,
        )
    }

    activeSheet?.let { sheet ->
        SettingsSheet(
            icon = sheet.icon,
            tint = sheet.tint,
            title = stringResource(sheet.labelRes),
            description = stringResource(sheet.descRes),
            sheetState = sheetState,
            onDismiss = { activeSheet = null },
        ) {
            when (sheet) {
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
                SettingsSubcategory.Ping -> PingGroup(settings)
                SettingsSubcategory.Experimental -> ExperimentalGroup(settings)
                SettingsSubcategory.Appearance -> AppearanceGroup(onBeforeRecreate = { activeSheet = null })
            }
        }
    }
}

/**
 * Карточка раздела — SettingsItem мобильного макета (Figma 6871:4921): Grey
 * с обводкой белым 10 %, скругление 20, поле 14; плитка 40 с глифом 22,
 * заголовок 14 Bold и пункты раздела 10 Semibold.
 */
@Composable
private fun SettingsEntry(
    icon: ImageVector,
    tint: CategoryTint,
    title: String,
    description: String,
    onClick: () -> Unit,
) {
    val shape = RoundedCornerShape(20.dp)
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(shape)
            .background(RvColor.Grey)
            .border(1.dp, RvColor.whiteA10, shape)
            .clickable(onClick = onClick)
            .padding(14.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2),
    ) {
        SettingIcon(icon, tint, glyph = 22.dp)
        Column(verticalArrangement = Arrangement.spacedBy(RvSpace.xs)) {
            Text(
                title,
                fontFamily = SegoeUi,
                fontSize = 14.sp,
                lineHeight = 19.6.sp,
                fontWeight = FontWeight.Bold,
                color = RvColor.White,
            )
            Text(
                description,
                fontFamily = SegoeUi,
                fontSize = 10.sp,
                lineHeight = 11.sp,
                fontWeight = FontWeight.SemiBold,
                color = RvColor.whiteA50,
            )
        }
    }
}

@Composable
private fun SubscriptionsGroup(settings: com.resultv.android.vpn.SettingsState) {
    SheetGroup(stringResource(R.string.settings_label_update), Icons.Outlined.Sync) {
        SheetToggleRow(
            icon = Icons.Outlined.Sync,
            tint = RvCategory.Emerald,
            title = stringResource(R.string.settings_sub_auto_update_title),
            subtitle = stringResource(R.string.settings_sub_auto_update_desc),
            checked = settings.subscriptionAutoUpdate,
            onCheckedChange = { SettingsRepository.setSubscriptionAutoUpdate(it) },
        )
        SheetDivider()
        SheetRow(
            icon = Icons.Outlined.Timer,
            tint = RvCategory.Amber,
            title = stringResource(R.string.settings_sub_interval_title),
            subtitle = stringResource(R.string.settings_sub_interval_desc),
            trailing = {
                val hours = settings.subscriptionUpdateIntervalHours
                val options = (listOf(1, 2, 4, 6, 12, 24) + hours).distinct().sorted()
                SheetDropdown(
                    value = hours,
                    options = options.map { it to stringResource(R.string.settings_hours_short, it) },
                    onSelect = { SettingsRepository.setSubscriptionUpdateIntervalHours(it) },
                )
            },
        )
    }
    SheetGroup(stringResource(R.string.settings_label_provider_data), Icons.Outlined.Badge) {
        SheetToggleRow(
            icon = Icons.Outlined.Fingerprint,
            tint = RvCategory.Violet,
            title = stringResource(R.string.settings_sub_hwid_title),
            subtitle = stringResource(R.string.settings_sub_hwid_desc),
            checked = settings.subscriptionSendHwid,
            onCheckedChange = { SettingsRepository.setSubscriptionSendHwid(it) },
        )
        SheetDivider()
        var ua by remember(settings.subscriptionUserAgent) { mutableStateOf(settings.subscriptionUserAgent) }
        SheetRow(
            icon = Icons.Outlined.Badge,
            tint = RvCategory.Slate,
            title = stringResource(R.string.settings_sub_ua_title),
            subtitle = stringResource(R.string.settings_sub_ua_desc),
            below = {
                SheetField(
                    value = ua,
                    onValueChange = {
                        ua = it
                        SettingsRepository.setSubscriptionUserAgent(it)
                    },
                    placeholder = stringResource(R.string.settings_sub_ua_placeholder),
                    keyboardType = KeyboardType.Ascii,
                )
            },
        )
    }
}

@Composable
private fun SecurityGroup(settings: com.resultv.android.vpn.SettingsState) {
    SheetGroup(stringResource(R.string.settings_label_connection), Icons.Outlined.Security) {
        SheetToggleRow(
            icon = Icons.Outlined.GppBad,
            tint = RvCategory.Red,
            title = stringResource(R.string.settings_killswitch_title),
            subtitle = stringResource(R.string.settings_killswitch_desc),
            checked = settings.killSwitch,
            onCheckedChange = { SettingsRepository.setKillSwitch(it) },
        )
    }
}

@Composable
private fun NetworkGroup(settings: com.resultv.android.vpn.SettingsState) {
    SheetGroup("DNS", Icons.Outlined.Dns) {
        SheetRow(
            icon = Icons.Outlined.Dns,
            tint = RvCategory.Blue,
            title = stringResource(R.string.settings_dns_title),
            subtitle = stringResource(R.string.settings_dns_hint),
            below = {
                SheetChips(
                    options = dnsPresets().map { it.key to it.label },
                    selected = settings.dnsPreset,
                    onSelect = { SettingsRepository.setDnsPreset(it, "") },
                )
                SheetField(
                    value = if (settings.dnsPreset == "Custom") settings.dnsCustom else "",
                    onValueChange = { SettingsRepository.setDnsPreset("Custom", it) },
                    placeholder = stringResource(R.string.settings_dns_custom_placeholder),
                )
                Text(
                    stringResource(R.string.settings_dns_private_warning),
                    style = SheetNoteStyle,
                    color = RvColor.whiteA50,
                )
            },
        )
    }
    SheetGroup(stringResource(R.string.settings_label_lan), Icons.Outlined.Lan) {
        SheetToggleRow(
            icon = Icons.Outlined.Lan,
            tint = RvCategory.Violet,
            title = stringResource(R.string.settings_bypass_lan),
            subtitle = stringResource(R.string.settings_bypass_lan_subtitle),
            checked = settings.bypassLan,
            onCheckedChange = { SettingsRepository.setBypassLan(it) },
        )
    }
    SheetGroup(stringResource(R.string.settings_label_tunnel), Icons.Outlined.Language) {
        SheetToggleRow(
            icon = Icons.Outlined.Language,
            tint = RvCategory.Blue,
            title = stringResource(R.string.settings_ipv6),
            subtitle = stringResource(R.string.settings_ipv6_subtitle),
            checked = settings.ipv6,
            onCheckedChange = { SettingsRepository.setIpv6(it) },
        )
    }
}

/**
 * Недоделанное и необкатанное.
 *
 * Отдельный раздел, а не строка в «Сети», по одной причине: в «Сети» эти
 * тумблеры стояли рядом с IPv6 и обходом LAN — среди вещей, которые работают и
 * на которые можно положиться. Адаптивный Smart к таким пока не относится, и
 * соседство обещало человеку не то.
 */
@Composable
private fun ExperimentalGroup(settings: com.resultv.android.vpn.SettingsState) {
    SheetGroup(stringResource(R.string.settings_adaptive_smart), Icons.Outlined.AutoAwesome) {
        SheetToggleRow(
            icon = Icons.Outlined.AutoAwesome,
            tint = RvCategory.Emerald,
            title = stringResource(R.string.settings_adaptive_smart),
            subtitle = stringResource(R.string.settings_adaptive_smart_subtitle),
            checked = settings.adaptiveSmart,
            onCheckedChange = { SettingsRepository.setAdaptiveSmart(it) },
        )
        // Подтумблер показывается только при включённом основном: висящая в
        // интерфейсе настройка того, чего нет, — это вопрос, на который человеку
        // приходится отвечать зря.
        if (settings.adaptiveSmart) {
            SheetDivider()
            SheetToggleRow(
                icon = Icons.Outlined.Memory,
                tint = RvCategory.Violet,
                title = stringResource(R.string.settings_adaptive_smart_memory),
                subtitle = stringResource(R.string.settings_adaptive_smart_memory_subtitle),
                checked = settings.adaptiveSmartMemoryOnly,
                onCheckedChange = { SettingsRepository.setAdaptiveSmartMemoryOnly(it) },
            )
        }
    }
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
@Composable
private fun PingGroup(settings: com.resultv.android.vpn.SettingsState) {
    SheetGroup(stringResource(R.string.settings_label_probe), Icons.Outlined.Speed) {
        SheetRow(
            icon = Icons.Outlined.Speed,
            tint = RvCategory.Cyan,
            title = stringResource(R.string.settings_ping_type_title),
            subtitle = stringResource(R.string.settings_ping_hint),
            below = {
                SheetChips(
                    options = listOf(
                        "auto" to stringResource(R.string.settings_ping_type_auto),
                        "icmp" to stringResource(R.string.settings_ping_type_icmp),
                        "http_head" to stringResource(R.string.settings_ping_type_http_head),
                        "http_get" to stringResource(R.string.settings_ping_type_http_get),
                    ),
                    selected = settings.pingType,
                    onSelect = { SettingsRepository.setPingType(it) },
                )
            },
        )
    }
    // Адрес и бюджет показываются всегда, а не только под http-типами:
    // бюджет действует и на ICMP, и пряча поле, мы бы прятали причину, по
    // которой проба вернулась именно так.
    SheetGroup(stringResource(R.string.settings_label_params), Icons.Outlined.Timer) {
        var urlDraft by rememberSaveable(settings.pingTestUrl) { mutableStateOf(settings.pingTestUrl) }
        val urlInvalid = urlDraft.isNotBlank() && !SettingsRepository.isValidPingTestUrl(urlDraft)
        SheetRow(
            icon = Icons.Outlined.Link,
            tint = RvCategory.Blue,
            title = stringResource(R.string.settings_ping_url),
            subtitle = stringResource(
                if (urlInvalid) R.string.settings_ping_url_invalid else R.string.settings_ping_url_subtitle,
            ),
            below = {
                SheetField(
                    value = urlDraft,
                    onValueChange = {
                        urlDraft = it
                        if (it.isBlank() || SettingsRepository.isValidPingTestUrl(it)) {
                            SettingsRepository.setPingTestUrl(it)
                        }
                    },
                    placeholder = "https://www.gstatic.com/generate_204",
                    isError = urlInvalid,
                    keyboardType = KeyboardType.Uri,
                )
            },
        )
        SheetDivider()
        SheetRow(
            icon = Icons.Outlined.Timer,
            tint = RvCategory.Amber,
            title = stringResource(R.string.settings_ping_timeout),
            subtitle = stringResource(R.string.settings_ping_timeout_subtitle),
            trailing = {
                // 0 — «по умолчанию», это три секунды.
                val sec = settings.pingTimeoutSec.takeIf { it > 0 } ?: 3
                SheetDropdown(
                    value = sec,
                    options = (1..10).map { it to stringResource(R.string.settings_seconds_short, it) },
                    onSelect = { SettingsRepository.setPingTimeoutSec(it) },
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

@Composable
private fun AppearanceGroup(onBeforeRecreate: () -> Unit) {
    val ctx = LocalContext.current
    val activity = remember(ctx) { ctx.findActivity() }
    val currentLang = remember(activity) { LocaleManager.currentLocale(ctx) ?: "EN" }
    SheetGroup(stringResource(R.string.settings_label_interface), Icons.Outlined.Translate) {
        SheetRow(
            icon = Icons.Outlined.Translate,
            tint = RvCategory.Violet,
            title = stringResource(R.string.settings_appearance_language_title),
            subtitle = stringResource(R.string.settings_appearance_language_desc),
            trailing = {
                SheetDropdown(
                    value = currentLang,
                    options = Languages.map { it.code to it.title },
                    onSelect = { code ->
                        if (code != currentLang && activity != null) {
                            // Dismiss the sheet before recreate() so the
                            // sub-window doesn't outlive the Activity.
                            onBeforeRecreate()
                            LocaleManager.setLocale(activity, code)
                        }
                    },
                )
            },
        )
    }
}

