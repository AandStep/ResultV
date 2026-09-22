package com.resultv.android.ui.screens

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
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
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Bolt
import androidx.compose.material.icons.outlined.ExpandMore
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
import com.resultv.android.ui.components.ServerRow
import com.resultv.android.ui.components.SpeedTile
import com.resultv.android.ui.components.SubscriptionLogo
import com.resultv.android.ui.components.homeLook
import com.resultv.android.ui.components.sortProfiles
import com.resultv.android.ui.components.subscriptionUsesImpLogo
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
            .padding(horizontal = RvSpace.nest1, vertical = RvSpace.nest3),
        horizontalAlignment = Alignment.CenterHorizontally,
        // Standardised gap between every block on Home — the current-server
        // card sits the same distance below the power button as the speed
        // cards sit above "Add server".
        verticalArrangement = Arrangement.spacedBy(RvSpace.nest2),
    ) {
        PowerButton(
            look = homeLook(status),
            enabled = canConnect || canDisconnect,
            onClick = onPowerPressed,
        )

        // Active profile selector + expandable picker — one Card. Ping-probe
        // and sort controls live in the card's own header (desktop parity —
        // no separate toolbar row above it) and only draw once expanded.
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
                    onPing = { PingRepository.refreshAll(profilesState.profiles) },
                    sortMode = sortMode,
                    onSortModeChange = { sortMode = it },
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

        TrafficStatsRow(active = status is VpnStatus.Connected)

        // Add-server shortcut stays visible in every state — the user
        // commonly wants to add another profile mid-session without
        // disconnecting first.
        AddProfileShortcut(onClick = onOpenAdd)
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
        horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2),
    ) {
        SpeedTile(
            label = stringResource(R.string.home_stat_download),
            rate = formatBps(stats.downloadBps),
            total = formatBytes(stats.downloadBytes),
            history = stats.downloadHistory.map { it.toFloat() },
            color = RvColor.Main,
            active = active,
            modifier = Modifier.weight(1f),
        )
        SpeedTile(
            label = stringResource(R.string.home_stat_upload),
            rate = formatBps(stats.uploadBps),
            total = formatBytes(stats.uploadBytes),
            history = stats.uploadHistory.map { it.toFloat() },
            color = RvColor.Second,
            active = active,
            modifier = Modifier.weight(1f),
        )
    }
}

private fun formatBytes(bytes: Long): String {
    if (bytes < 1024) return "$bytes B"
    val units = arrayOf("KB", "MB", "GB", "TB")
    var v = bytes.toDouble() / 1024.0
    var i = 0
    while (v >= 1024 && i < units.size - 1) { v /= 1024.0; i++ }
    return String.format("%.2f %s", v, units[i])
}

private fun formatBps(bps: Long): String {
    if (bps == 0L) return "0 B/s"
    return formatBytes(bps) + "/s"
}

@Composable
private fun ActiveProfileRow(
    active: Profile?,
    activeCountry: String?,
    accent: HomeLook,
    expanded: Boolean,
    onToggle: () -> Unit,
    onPing: () -> Unit,
    sortMode: ProfileSortMode,
    onSortModeChange: (ProfileSortMode) -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .height(72.dp)
            .background(RvColor.Grey)
            .clickable(onClick = onToggle)
            .padding(horizontal = RvSpace.nest1),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2),
    ) {
        // Общая с ServerRow плитка (см. её KDoc в ServerRow.kt) — здесь
        // масштаб шапки: 48dp/24dp против 44dp/22dp у строки списка.
        // activeCountry уже null всякий раз, когда active == null (см.
        // вычисление в вызывающем коде), так что отдельная ветка не нужна.
        ProfileTile(
            accent = accent,
            isAuto = active?.isAuto ?: false,
            countryCode = activeCountry,
            size = 48.dp,
            glyph = 24.dp,
            flagStyle = MaterialTheme.typography.headlineSmall,
        )

        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(RvSpace.xs),
        ) {
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
                color = RvColor.White,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }

        // Замер задержки и сортировка показываются только в раскрытом виде —
        // на ПК это тоже кнопки шапки, а не отдельная панель над карточкой.
        if (expanded) {
            IconButton(onClick = onPing, modifier = Modifier.size(36.dp)) {
                Icon(
                    imageVector = Icons.Outlined.Bolt,
                    contentDescription = stringResource(R.string.ping_refresh_cd),
                    tint = RvColor.whiteA50,
                    modifier = Modifier.size(RvIcon.glyph),
                )
            }
            ProfileSortMenu(mode = sortMode, onModeChange = onSortModeChange)
        }

        // Поворот, а не подмена иконки — то же движение, которым шеврон и
        // открывает карточку (парность с ПК).
        Icon(
            imageVector = Icons.Outlined.ExpandMore,
            contentDescription = stringResource(
                if (expanded) R.string.action_collapse else R.string.action_expand,
            ),
            tint = RvColor.whiteA50,
            modifier = Modifier.graphicsLayer { rotationZ = if (expanded) 180f else 0f },
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
                val sub = group.subscription
                val usesImp = remember(sub?.id, sub?.name, sub?.source) {
                    sub?.let { subscriptionUsesImpLogo(it) } ?: false
                }
                SubscriptionLogo(usesImpLogo = usesImp, size = 22.dp)
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

@Composable
private fun AddProfileShortcut(onClick: () -> Unit) {
    val shape = RoundedCornerShape(RvRadius.card)
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(shape)
            .clickable(onClick = onClick)
            .rvBorder(shape)
            .background(Color.White.copy(alpha = 0.02f))
            .padding(horizontal = RvSpace.nest2, vertical = RvSpace.nest2),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2),
    ) {
        Box(
            modifier = Modifier
                .size(38.dp)
                .clip(RoundedCornerShape(RvRadius.chip))
                .background(Color.White.copy(alpha = 0.07f)),
            contentAlignment = Alignment.Center,
        ) {
            Icon(
                imageVector = Icons.Outlined.Add,
                contentDescription = null,
                tint = RvColor.whiteA50,
            )
        }
        Column {
            Text(stringResource(R.string.home_add_server), style = MaterialTheme.typography.titleSmall)
            Text(
                stringResource(R.string.home_add_server_subtitle),
                style = MaterialTheme.typography.bodySmall,
                color = RvColor.whiteA50,
            )
        }
    }
}

// ───────────────────────── Profile field helpers ──────────────────────────
// Thin aliases over the cached properties on [Profile]. They exist so call
// sites read uniformly across screens — and to keep the existing function
// shape that consumers were already using.

internal fun profileProtocol(p: Profile): String = p.protocol
