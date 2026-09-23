package com.resultv.android.ui.screens

import android.widget.Toast
import kotlin.math.roundToInt
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.width
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Star
import androidx.compose.material.icons.outlined.Bolt
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.resultv.android.R
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.RvIcon
import com.resultv.android.theme.RvRadius
import com.resultv.android.theme.RvSpace
import com.resultv.android.theme.rvBorder
import com.resultv.android.ui.components.HomeLook
import com.resultv.android.ui.components.PowerButton
import com.resultv.android.ui.components.ProfileEditSheet
import com.resultv.android.ui.components.ProfileSortMenu
import com.resultv.android.ui.components.ProfileSortMode
import com.resultv.android.ui.components.ProfileTile
import com.resultv.android.ui.components.ProtocolBadge
import com.resultv.android.ui.components.RvButton
import com.resultv.android.ui.components.RvButtonColors
import com.resultv.android.ui.components.RvButtonLabel
import com.resultv.android.ui.components.ServerRow
import com.resultv.android.ui.components.SpeedTile
import com.resultv.android.ui.components.UptimeChip
import com.resultv.android.ui.components.homeLook
import com.resultv.android.ui.components.sortProfiles
import com.resultv.android.vpn.CountryRepository
import com.resultv.android.vpn.PingRepository
import com.resultv.android.vpn.Profile
import com.resultv.android.vpn.ProfileRepository
import com.resultv.android.vpn.Subscription
import com.resultv.android.vpn.SubscriptionRepository
import com.resultv.android.vpn.VpnState
import com.resultv.android.vpn.VpnStatus
import com.resultv.android.vpn.serverDisplayName

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HomeScreen(
    onPowerPressed: () -> Unit,
    onOpenProxies: () -> Unit,
    onOpenAdd: () -> Unit,
) {
    val status by VpnState.status.collectAsStateWithLifecycle()
    val profilesState by ProfileRepository.state.collectAsStateWithLifecycle()
    val subsState by SubscriptionRepository.state.collectAsStateWithLifecycle()
    val pings by PingRepository.results.collectAsStateWithLifecycle()
    val pingInflight by PingRepository.inflight.collectAsStateWithLifecycle()
    val countries by CountryRepository.results.collectAsStateWithLifecycle()
    val ctx = LocalContext.current
    val scope = rememberCoroutineScope()
    val dataDir = remember(ctx) { ctx.filesDir.absolutePath }
    LaunchedEffect(profilesState.profiles) {
        CountryRepository.resolve(profilesState.profiles, dataDir)
        // The auto-ping sweep for unprobed profiles lives in AppShell, not
        // here: only one tab is composed at a time, so a screen-local effect
        // missed every profile added while another tab was on top.
    }
    // Persisted across tab switches so reopening Home doesn't snap shut.
    var dropdownOpen by rememberSaveable { mutableStateOf(false) }
    var sortMode by rememberSaveable { mutableStateOf(ProfileSortMode.Default) }

    // Subscriptions can be hidden from Home individually. Build a quick
    // lookup so the dropdown filter is O(1) per profile.
    val hiddenSubIds = remember(subsState.subs) {
        subsState.subs.asSequence().filter { it.hiddenOnHome }.map { it.id }.toSet()
    }
    val visibleHomeProfiles = remember(profilesState.profiles, hiddenSubIds) {
        if (hiddenSubIds.isEmpty()) profilesState.profiles
        else profilesState.profiles.filter { it.subscriptionId !in hiddenSubIds }
    }
    var editingProfileId by remember { mutableStateOf<String?>(null) }
    var fullEditProfileId by remember { mutableStateOf<String?>(null) }
    var pendingDeleteProfile by remember { mutableStateOf<Profile?>(null) }

    val active = profilesState.active
    val canConnect = active != null && (status is VpnStatus.Idle || status is VpnStatus.Error)
    val canDisconnect = status is VpnStatus.Connecting || status is VpnStatus.Connected

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            // Ритм мобильного макета (Figma 6856:4885): поле 12, между
            // крупными блоками 24, панель над карточкой — 16, карточка и
            // плитки скорости — 8, до кнопок снова 16.
            .padding(start = HomeGap.page, end = HomeGap.page, top = HomeGap.block, bottom = HomeGap.page),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        PowerButton(
            look = homeLook(status),
            enabled = canConnect || canDisconnect,
            onClick = onPowerPressed,
        )
        Spacer(Modifier.height(HomeGap.block))

        // Панель над карточкой: слева таймер соединения, справа замер задержки
        // и сортировка. На ПК эти кнопки живут в шапке карточки, но здесь
        // решено иначе — им место над списком, а не внутри строки выбора.
        // Высота фиксирована — по плашке времени, 30: иначе стандартный
        // IconButton забирает 48 dp и панель отрывается от карточки, а без
        // плашки (она видна только при соединении) ряд прыгал бы по высоте.
        // Кнопки по 36 ради пальца: между глифами 20, как в макете, и край
        // последнего глифа сдвигом ложится на поле страницы.
        Row(
            modifier = Modifier.fillMaxWidth().height(30.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            UptimeChip(status = status)
            Spacer(Modifier.weight(1f))
            Row(
                modifier = Modifier.offset(x = 8.dp),
                horizontalArrangement = Arrangement.spacedBy(4.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                IconButton(
                    onClick = { PingRepository.refreshAll(profilesState.profiles) },
                    modifier = Modifier.size(36.dp),
                ) {
                    Icon(
                        painter = painterResource(R.drawable.ic_ping),
                        contentDescription = stringResource(R.string.ping_refresh_cd),
                        tint = RvColor.whiteA50,
                        modifier = Modifier.size(RvIcon.glyph),
                    )
                }
                ProfileSortMenu(mode = sortMode, onModeChange = { sortMode = it })
            }
        }
        Spacer(Modifier.height(HomeGap.panel))

        // Active profile selector + expandable picker — one Card.
        val listShape = RoundedCornerShape(RvRadius.panel)
        Card(
            shape = listShape,
            colors = CardDefaults.cardColors(containerColor = RvColor.Black),
            modifier = Modifier.fillMaxWidth().rvBorder(listShape),
        ) {
            // Обрезка живёт внутри, а не на карточке: обрезка по внешнему
            // краю съела бы собственную обводку вместе со сглаживанием.
            Column(modifier = Modifier.clip(listShape)) {
                ActiveProfileRow(
                    active = active,
                    activeCountry = active?.let { it.country ?: countries[it.id] },
                    accent = homeLook(status),
                    expanded = dropdownOpen,
                    onToggle = { dropdownOpen = !dropdownOpen },
                )
                AnimatedVisibility(visible = dropdownOpen) {
                    ProfileDropdown(
                        profiles = visibleHomeProfiles,
                        subscriptions = subsState.subs,
                        activeId = profilesState.activeId,
                        accent = homeLook(status),
                        pings = pings,
                        pingInflight = pingInflight,
                        countries = countries,
                        sortMode = sortMode,
                        onSelect = {
                            ProfileRepository.setActive(it.id)
                            dropdownOpen = false
                        },
                        onLongPress = { editingProfileId = it.id },
                    )
                }
            }
        }

        Spacer(Modifier.height(HomeGap.tight))
        TrafficStatsRow(active = status is VpnStatus.Connected)
        Spacer(Modifier.height(HomeGap.panel))

        // Кнопки добавления видны в любом состоянии — сервер часто
        // добавляют посреди сессии, не отключаясь.
        HomeActions(
            onAdd = onOpenAdd,
            onPaste = {
                pasteFromClipboard(ctx, scope, dataDir) { msg ->
                    msg?.let { Toast.makeText(ctx, it, Toast.LENGTH_LONG).show() }
                }
            },
            onScan = {
                scanQr(ctx) { msg -> Toast.makeText(ctx, msg, Toast.LENGTH_LONG).show() }
            },
        )
    }

    editingProfileId?.let { id ->
        val target = profilesState.profiles.firstOrNull { it.id == id }
        if (target == null || target.isSection) {
            editingProfileId = null
            return@let
        }
        val canEditFull = target.subscriptionId.isBlank() && canFullEdit(target.uri)
        ProfileEditSheet(
            profile = target,
            onProbeLatency = { PingRepository.refresh(target) },
            onRename = { ProfileRepository.rename(id, it) },
            onToggleFavorite = { ProfileRepository.toggleFavorite(id) },
            onDelete = { pendingDeleteProfile = target },
            onDismiss = { editingProfileId = null },
            onEditFull = if (canEditFull) {
                { editingProfileId = null; fullEditProfileId = id }
            } else null,
        )
    }

    fullEditProfileId?.let { id ->
        val target = profilesState.profiles.firstOrNull { it.id == id }
        if (target == null || target.isSection) {
            fullEditProfileId = null
            return@let
        }
        ProfileFullEditSheet(
            profile = target,
            onDismiss = { fullEditProfileId = null },
        )
    }

    pendingDeleteProfile?.let { target ->
        AlertDialog(
            onDismissRequest = { pendingDeleteProfile = null },
            title = { Text(stringResource(R.string.proxies_delete_title)) },
            text = { Text(stringResource(R.string.proxies_delete_message, target.name), color = RvColor.whiteA50) },
            confirmButton = {
                TextButton(onClick = {
                    ProfileRepository.remove(target.id)
                    pendingDeleteProfile = null
                }) { Text(stringResource(R.string.action_delete), color = RvColor.Errors) }
            },
            dismissButton = {
                TextButton(onClick = { pendingDeleteProfile = null }) { Text(stringResource(R.string.action_cancel)) }
            },
        )
    }
}

@Composable
private fun TrafficStatsRow(active: Boolean) {
    val stats by com.resultv.android.vpn.TrafficStats.snapshot.collectAsStateWithLifecycle()
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(HomeGap.tight),
    ) {
        SpeedTile(
            label = stringResource(R.string.home_stat_download),
            rate = rateText(stats.downloadBps),
            total = trafficText(stats.downloadBytes),
            history = stats.downloadHistory.map { it.toFloat() },
            color = RvColor.Main,
            active = active,
            modifier = Modifier.weight(1f),
        )
        SpeedTile(
            label = stringResource(R.string.home_stat_upload),
            rate = rateText(stats.uploadBps),
            total = trafficText(stats.uploadBytes),
            history = stats.uploadHistory.map { it.toFloat() },
            color = RvColor.Second,
            active = active,
            modifier = Modifier.weight(1f),
        )
    }
}

private const val KB = 1024.0
private const val MB = KB * 1024
private const val GB = MB * 1024

/** Накопленный объём, как на ПК (`formatTraffic`): «0 Мб», «312 Мб», «1.5 Гб». */
@Composable
private fun trafficText(bytes: Long): String =
    if (bytes >= GB) stringResource(R.string.unit_gb, bytes / GB)
    else stringResource(R.string.unit_mb, (bytes / MB).roundToInt())

/** Текущая скорость, как на ПК (`formatRate`): «0 кб/с», «312 кб/с», «1.5 Мб/с». */
@Composable
private fun rateText(bytesPerSec: Long): String =
    if (bytesPerSec >= MB) stringResource(R.string.unit_mbps, bytesPerSec / MB)
    else stringResource(R.string.unit_kbps, (bytesPerSec / KB).roundToInt())

@Composable
private fun ActiveProfileRow(
    active: Profile?,
    activeCountry: String?,
    accent: HomeLook,
    expanded: Boolean,
    onToggle: () -> Unit,
) {
    // Метрики — ServerItem мобильного макета (Figma 6859:5086).
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .background(RvColor.Grey)
            .clickable(onClick = onToggle)
            .padding(start = 15.dp, end = 14.dp, top = 14.dp, bottom = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RvSpace.nest3),
    ) {
        // Общая с ServerRow плитка (см. её KDoc в ServerRow.kt) — здесь
        // масштаб шапки: 46dp/23dp против 44dp/22dp у строки списка.
        // activeCountry уже null всякий раз, когда active == null (см.
        // вычисление в вызывающем коде), так что отдельная ветка не нужна.
        ProfileTile(
            accent = accent,
            isAuto = active?.isAuto ?: false,
            countryCode = activeCountry,
            size = 46.dp,
            glyph = 23.dp,
            flagStyle = MaterialTheme.typography.headlineSmall,
        )

        Column(modifier = Modifier.weight(1f)) {
            val shown = when {
                active == null -> emptyList()
                active.isAuto -> listOf(stringResource(R.string.badge_auto))
                else -> active.badges
            }
            if (shown.isNotEmpty()) {
                Row(horizontalArrangement = Arrangement.spacedBy(RvSpace.xs)) {
                    shown.forEachIndexed { i, b ->
                        ProtocolBadge(text = b, first = i == 0, accent = accent)
                    }
                }
            }
            Text(
                text = active?.let { serverDisplayName(it.name, activeCountry) }
                    ?: stringResource(R.string.home_no_profile_selected),
                style = MaterialTheme.typography.titleMedium,
                lineHeight = 19.6.sp,
                fontWeight = FontWeight.Bold,
                color = RvColor.White,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                // В макете имя сдвинуто на 5 — вровень с текстом бейджа, а
                // не с краем его капсулы.
                modifier = Modifier.padding(start = 5.dp),
            )
        }

        // Замер задержки и сортировка показываются только в раскрытом виде —
        // на ПК это тоже кнопки шапки, а не отдельная панель над карточкой.
        // Поворот, а не подмена иконки — то же движение, которым шеврон и
        // открывает карточку (парность с ПК).
        // Глиф в макете смотрит вверх; свёрнутая карточка показывает его
        // перевёрнутым, раскрытая — как есть.
        Icon(
            painter = painterResource(R.drawable.ic_menu_arrow),
            contentDescription = stringResource(
                if (expanded) R.string.action_collapse else R.string.action_expand,
            ),
            tint = RvColor.whiteA50,
            modifier = Modifier
                .size(32.dp)
                .graphicsLayer { rotationZ = if (expanded) 0f else 180f },
        )
    }
}

@Composable
private fun ProfileDropdown(
    profiles: List<Profile>,
    subscriptions: List<Subscription>,
    activeId: String?,
    /** Состояние подключения — им подсвечивается строка выбранного сервера. */
    accent: HomeLook,
    pings: Map<String, PingRepository.Sample>,
    pingInflight: Set<String>,
    countries: Map<String, String>,
    sortMode: ProfileSortMode,
    onSelect: (Profile) -> Unit,
    onLongPress: (Profile) -> Unit,
) {
    // Build the grouped layout the same way the desktop does: a Favourites
    // bucket pulled to the top, then one bucket per subscription (in the
    // order subscriptions were added), then unaffiliated "My proxies".
    // Headers are suppressed when there's effectively a single flat list —
    // i.e. no favourites and at most one group — so a user with one
    // subscription still sees a plain list.
    val groups = remember(profiles, subscriptions, sortMode, pings) {
        buildHomeGroups(profiles, subscriptions, sortMode, pings)
    }

    Column(
        modifier = Modifier.fillMaxWidth(),
    ) {
        if (groups.isEmpty()) {
            Text(
                text = stringResource(R.string.home_no_profiles_yet),
                style = MaterialTheme.typography.bodySmall,
                color = RvColor.whiteA50,
                modifier = Modifier.padding(RvSpace.nest3),
            )
            return@Column
        }

        val showHeaders = groups.size > 1 || groups.any { it.kind == HomeGroupKind.Favorites }
        groups.forEach { group ->
            if (showHeaders) {
                HomeGroupHeader(group)
            }
            group.profiles.forEach { p ->
                key(p.id) {
                    if (p.isSection) {
                        SectionLabel(p.name)
                    } else {
                        ServerRow(
                            name = serverDisplayName(p.name, p.country ?: countries[p.id]),
                            badges = p.badges,
                            countryCode = p.country ?: countries[p.id],
                            isAuto = p.isAuto,
                            isActive = p.id == activeId,
                            isFavorite = p.isFavorite,
                            accent = if (p.id == activeId) accent else HomeLook.Idle,
                            onClick = { onSelect(p) },
                            onLongClick = { onLongPress(p) },
                            latencyMs = pings[p.id]?.takeIf { it.reachable }?.latencyMs,
                            offlineReason = pings[p.id]?.takeUnless { it.reachable }?.reason,
                            isLoading = p.id in pingInflight,
                        )
                    }
                }
            }
        }
    }
}

private enum class HomeGroupKind { Favorites, Subscription, Standalone }

/**
 * One rendered section in the Home picker. [subscription] is set only for
 * [HomeGroupKind.Subscription] groups so the header can show its logo/name.
 */
private data class HomeGroup(
    val kind: HomeGroupKind,
    val subscription: Subscription?,
    val profiles: List<Profile>,
)

/**
 * Partition the visible Home profiles into Favourites → per-subscription →
 * standalone, sorting within each bucket via [sortProfiles]. Favourites are
 * pulled out of their groups so they aren't shown twice. Empty buckets are
 * omitted, so callers can decide whether to draw headers based on the
 * resulting group count.
 *
 * Subscription buckets keep their SECTION rows (e.g. impVPN's "Когда
 * глушат" dividers) in place, chunk-sorted around them via
 * [reorderForDisplay] — same treatment the Proxies screen gives them —
 * so the picker on Home shows the same dividers. Favourites/standalone
 * never contain SECTION rows since those buckets don't preserve
 * subscription order.
 */
private fun buildHomeGroups(
    profiles: List<Profile>,
    subscriptions: List<Subscription>,
    sortMode: ProfileSortMode,
    pings: Map<String, PingRepository.Sample>,
): List<HomeGroup> {
    val selectable = profiles.filterNot { it.isSection }
    if (selectable.isEmpty()) return emptyList()

    val favorites = selectable.filter { it.isFavorite }
    val favoriteIds = favorites.mapTo(mutableSetOf()) { it.id }

    val groups = mutableListOf<HomeGroup>()
    if (favorites.isNotEmpty()) {
        groups += HomeGroup(HomeGroupKind.Favorites, null, sortProfiles(favorites, sortMode, pings))
    }
    subscriptions.forEach { sub ->
        val bucket = profiles.filter {
            it.subscriptionId == sub.id && (it.isSection || it.id !in favoriteIds)
        }
        if (bucket.any { !it.isSection }) {
            groups += HomeGroup(HomeGroupKind.Subscription, sub, reorderForDisplay(bucket, sortMode, pings))
        }
    }
    val standalone = selectable.filterNot { it.isFavorite }.filter { it.subscriptionId.isBlank() }
    if (standalone.isNotEmpty()) {
        groups += HomeGroup(HomeGroupKind.Standalone, null, sortProfiles(standalone, sortMode, pings))
    }
    return groups
}

@Composable
private fun HomeGroupHeader(group: HomeGroup) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .background(RvColor.Black)
            .padding(start = RvSpace.nest1, end = RvSpace.nest1, top = RvSpace.nest2, bottom = RvSpace.xs),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RvSpace.nest3),
    ) {
        when (group.kind) {
            HomeGroupKind.Favorites -> {
                Icon(
                    imageVector = Icons.Filled.Star,
                    contentDescription = null,
                    tint = RvColor.Warning,
                    modifier = Modifier.size(16.dp),
                )
                Text(
                    text = stringResource(R.string.home_favorites),
                    style = MaterialTheme.typography.labelMedium,
                    color = RvColor.whiteA20,
                )
            }
            HomeGroupKind.Subscription -> {
                // Логотипа провайдера здесь нет: на главной подпись группы —
                // это подпись, а не строка провайдера, и значок рядом с ней
                // спорил с плитками флагов, которые начинаются строкой ниже.
                val sub = group.subscription
                Text(
                    text = sub?.displayName.orEmpty().uppercase(),
                    style = MaterialTheme.typography.labelMedium,
                    color = RvColor.whiteA20,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
            HomeGroupKind.Standalone -> {
                Icon(
                    imageVector = Icons.Outlined.Bolt,
                    contentDescription = null,
                    tint = RvColor.whiteA50,
                    modifier = Modifier.size(16.dp),
                )
                Text(
                    text = stringResource(R.string.home_group_standalone),
                    style = MaterialTheme.typography.labelMedium,
                    color = RvColor.whiteA20,
                )
            }
        }
    }
}

/** Шаг мобильного макета главной. */
private object HomeGap {
    val page = 12.dp
    val block = 24.dp
    val panel = 16.dp
    val tight = 8.dp
}

/**
 * Ряд «Добавить / Вставить / QR» — мобильный макет (Figma 6862:5757).
 * Все три высотой 52 со скруглением 16; «Добавить» — основное действие,
 * на зелёной подложке 10 %.
 */
@Composable
private fun HomeActions(onAdd: () -> Unit, onPaste: () -> Unit, onScan: () -> Unit) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(HomeGap.tight),
    ) {
        RvButton(
            onClick = onAdd,
            fill = RvButtonColors.greenFill,
            outline = RvButtonColors.greenOutline,
            modifier = Modifier.weight(1f),
        ) {
            Icon(
                painter = painterResource(R.drawable.ic_nav_add),
                contentDescription = null,
                tint = RvColor.Main,
                modifier = Modifier.size(RvIcon.glyph),
            )
            Text(
                text = stringResource(R.string.tab_add),
                style = RvButtonLabel,
                fontWeight = FontWeight.Bold,
                color = RvColor.Main,
            )
        }
        RvButton(
            onClick = onPaste,
            fill = RvButtonColors.greyFill,
            outline = RvButtonColors.greyOutline,
            modifier = Modifier.weight(1f),
        ) {
            Icon(
                painter = painterResource(R.drawable.ic_paste),
                contentDescription = null,
                tint = RvColor.whiteA50,
                modifier = Modifier.size(RvIcon.glyph),
            )
            Text(
                text = stringResource(R.string.home_paste),
                style = RvButtonLabel,
                fontWeight = FontWeight.SemiBold,
                color = RvColor.whiteA50,
            )
        }
        val scanLabel = stringResource(R.string.add_quick_qr_title)
        RvButton(
            onClick = onScan,
            fill = RvButtonColors.greyFill,
            outline = RvButtonColors.greyOutline,
            modifier = Modifier
                .width(52.dp)
                .semantics { contentDescription = scanLabel },
        ) {
            Icon(
                painter = painterResource(R.drawable.ic_qr_scan),
                contentDescription = null,
                tint = RvColor.whiteA50,
                modifier = Modifier.size(RvIcon.glyph),
            )
        }
    }
}

// ───────────────────────── Profile field helpers ──────────────────────────
// Thin aliases over the cached properties on [Profile]. They exist so call
// sites read uniformly across screens — and to keep the existing function
// shape that consumers were already using.

internal fun profileProtocol(p: Profile): String = p.protocol
