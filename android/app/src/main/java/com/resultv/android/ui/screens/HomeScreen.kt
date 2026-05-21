package com.resultv.android.ui.screens

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Bolt
import androidx.compose.material.icons.filled.Star
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.ExpandMore
import androidx.compose.material.icons.outlined.NetworkPing
import androidx.compose.material.icons.outlined.Public
import androidx.compose.material.icons.outlined.Schedule
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
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.resultv.android.R
import com.resultv.android.theme.Brand
import com.resultv.android.ui.components.PowerButton
import com.resultv.android.ui.components.ProfileEditSheet
import com.resultv.android.ui.components.ProfileSortMenu
import com.resultv.android.ui.components.ProfileSortMode
import com.resultv.android.ui.components.ServerRow
import com.resultv.android.ui.components.Sparkline
import com.resultv.android.ui.components.StatusHeader
import com.resultv.android.ui.components.flagFromCountry
import com.resultv.android.ui.components.sortProfiles
import com.resultv.android.vpn.PingRepository
import com.resultv.android.vpn.Profile
import com.resultv.android.vpn.ProfileRepository
import com.resultv.android.vpn.VpnState
import com.resultv.android.vpn.VpnStatus
import org.json.JSONObject

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HomeScreen(
    onPowerPressed: () -> Unit,
    onOpenProxies: () -> Unit,
    onOpenAdd: () -> Unit,
) {
    val status by VpnState.status.collectAsStateWithLifecycle()
    val profilesState by ProfileRepository.state.collectAsStateWithLifecycle()
    val pings by PingRepository.results.collectAsStateWithLifecycle()
    var dropdownOpen by remember { mutableStateOf(false) }
    var sortMode by remember { mutableStateOf(ProfileSortMode.Default) }
    var editingProfileId by remember { mutableStateOf<String?>(null) }
    var pendingDeleteProfile by remember { mutableStateOf<Profile?>(null) }

    val active = profilesState.active
    val canConnect = active != null && (status is VpnStatus.Idle || status is VpnStatus.Error)
    val canDisconnect = status is VpnStatus.Connecting || status is VpnStatus.Connected

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 16.dp, vertical = 8.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        // Standardised gap between every block on Home — the toolbar row
        // sits the same distance above the current-server card as the
        // speed cards sit above "Add server".
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        StatusHeader(status = status, activeProfileName = active?.name)

        PowerButton(
            status = status,
            enabled = canConnect || canDisconnect,
            onClick = onPowerPressed,
        )

        // Toolbar row: left = uptime chip (only when Connected), right =
        // refresh-ping + sort. Uptime supersedes the previous standalone
        // 3-cell banner — down/up speeds already live in the cards below.
        // Fixed row height ≈ 36dp keeps the gap to the next card consistent
        // with the rest of the Column spacing (default IconButton claims
        // 48dp which made the toolbar look detached from the card below).
        Row(
            modifier = Modifier.fillMaxWidth().height(36.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            (status as? VpnStatus.Connected)?.let { connected ->
                UptimeChip(connectedAt = connected.connectedAt)
            }
            Spacer(Modifier.weight(1f))
            IconButton(
                onClick = { PingRepository.refreshAll(profilesState.profiles) },
                modifier = Modifier.size(36.dp),
            ) {
                Icon(
                    imageVector = Icons.Outlined.NetworkPing,
                    contentDescription = stringResource(R.string.ping_refresh_cd),
                    tint = Brand.SecondaryText,
                    modifier = Modifier.size(20.dp),
                )
            }
            ProfileSortMenu(mode = sortMode, onModeChange = { sortMode = it })
        }

        // Active profile selector + expandable picker — one Card. Container
        // stays neutral regardless of connection state; the active row in
        // the list below highlights itself via [ServerRow.isActive] so the
        // green tint reads as "this is the connected server", not "the whole
        // picker is the connection".
        Card(
            shape = RoundedCornerShape(20.dp),
            colors = CardDefaults.cardColors(containerColor = Brand.Surface),
            modifier = Modifier
                .fillMaxWidth()
                .border(
                    1.dp,
                    Color.White.copy(alpha = 0.09f),
                    RoundedCornerShape(20.dp),
                ),
        ) {
            ActiveProfileRow(
                active = active,
                connected = status is VpnStatus.Connected,
                expanded = dropdownOpen,
                onToggle = { dropdownOpen = !dropdownOpen },
            )

            AnimatedVisibility(visible = dropdownOpen) {
                ProfileDropdown(
                    profiles = profilesState.profiles,
                    activeId = profilesState.activeId,
                    pings = pings,
                    sortMode = sortMode,
                    onSelect = {
                        ProfileRepository.setActive(it.id)
                        dropdownOpen = false
                    },
                    onLongPress = { editingProfileId = it.id },
                )
            }
        }

        if (status is VpnStatus.Connected) {
            TrafficStatsRow()
        }

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
        ProfileEditSheet(
            profile = target,
            onProbeLatency = { PingRepository.refresh(target) },
            onRename = { ProfileRepository.rename(id, it) },
            onToggleFavorite = { ProfileRepository.toggleFavorite(id) },
            onDelete = { pendingDeleteProfile = target },
            onDismiss = { editingProfileId = null },
        )
    }

    pendingDeleteProfile?.let { target ->
        AlertDialog(
            onDismissRequest = { pendingDeleteProfile = null },
            title = { Text(stringResource(R.string.proxies_delete_title)) },
            text = { Text(stringResource(R.string.proxies_delete_message, target.name), color = Brand.SecondaryText) },
            confirmButton = {
                TextButton(onClick = {
                    ProfileRepository.remove(target.id)
                    pendingDeleteProfile = null
                }) { Text(stringResource(R.string.action_delete), color = Brand.Danger) }
            },
            dismissButton = {
                TextButton(onClick = { pendingDeleteProfile = null }) { Text(stringResource(R.string.action_cancel)) }
            },
        )
    }
}

/**
 * Compact uptime pill — clock icon + HH:MM:SS / MM:SS — that lives in the
 * toolbar row when the tunnel is up. Ticks once per second via a
 * [LaunchedEffect] keyed on [connectedAt] so the rest of HomeScreen
 * doesn't recompose with the timer.
 */
@Composable
private fun UptimeChip(connectedAt: Long) {
    var now by remember { mutableLongStateOf(System.currentTimeMillis()) }
    LaunchedEffect(connectedAt) {
        while (true) {
            now = System.currentTimeMillis()
            kotlinx.coroutines.delay(1000L)
        }
    }
    val elapsedSec = ((now - connectedAt).coerceAtLeast(0L) / 1000L)

    Row(
        modifier = Modifier
            .clip(RoundedCornerShape(14.dp))
            .background(Color.White.copy(alpha = 0.04f))
            .padding(horizontal = 12.dp, vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        Icon(
            imageVector = Icons.Outlined.Schedule,
            contentDescription = null,
            tint = Brand.SecondaryText,
            modifier = Modifier.size(14.dp),
        )
        Text(
            text = formatDuration(elapsedSec),
            style = MaterialTheme.typography.labelMedium,
            color = Brand.SecondaryText,
        )
    }
}

private fun formatDuration(totalSec: Long): String {
    val h = totalSec / 3600
    val m = (totalSec % 3600) / 60
    val s = totalSec % 60
    return if (h > 0) String.format("%d:%02d:%02d", h, m, s)
    else String.format("%02d:%02d", m, s)
}

@Composable
private fun TrafficStatsRow() {
    val stats by com.resultv.android.vpn.TrafficStats.snapshot.collectAsStateWithLifecycle()
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        StatCard(
            label = stringResource(R.string.home_stat_download),
            total = formatBytes(stats.downloadBytes),
            speed = formatBps(stats.downloadBps),
            history = stats.downloadHistory.map { it.toFloat() },
            color = Brand.Green,
            modifier = Modifier.weight(1f),
        )
        StatCard(
            label = stringResource(R.string.home_stat_upload),
            total = formatBytes(stats.uploadBytes),
            speed = formatBps(stats.uploadBps),
            history = stats.uploadHistory.map { it.toFloat() },
            color = Brand.GreenLight,
            modifier = Modifier.weight(1f),
        )
    }
}

@Composable
private fun StatCard(
    label: String,
    total: String,
    speed: String,
    history: List<Float>,
    color: Color,
    modifier: Modifier = Modifier,
) {
    Card(
        modifier = modifier,
        shape = RoundedCornerShape(22.dp),
        colors = CardDefaults.cardColors(containerColor = Brand.Surface),
    ) {
        Column(modifier = Modifier.padding(18.dp)) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(label, style = MaterialTheme.typography.labelMedium, color = Brand.MutedText)
                Text(speed, style = MaterialTheme.typography.labelMedium, color = color)
            }
            Spacer(Modifier.height(6.dp))
            Text(
                total,
                style = MaterialTheme.typography.headlineSmall,
                color = color,
            )
            Spacer(Modifier.height(10.dp))
            Sparkline(
                values = history,
                color = color,
                modifier = Modifier
                    .fillMaxWidth()
                    .height(36.dp),
            )
        }
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
    connected: Boolean,
    expanded: Boolean,
    onToggle: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onToggle)
            .padding(horizontal = 18.dp, vertical = 18.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        Box(
            modifier = Modifier
                .size(54.dp)
                .clip(RoundedCornerShape(14.dp))
                .background(
                    if (connected) Brand.Green.copy(alpha = 0.18f)
                    else Color.White.copy(alpha = 0.07f)
                ),
            contentAlignment = Alignment.Center,
        ) {
            val country = active?.let { profileCountry(it) }
            val isAuto = active?.let { profileIsAuto(it) } ?: false
            when {
                active == null -> Icon(
                    imageVector = Icons.Outlined.Public,
                    contentDescription = null,
                    tint = Brand.SecondaryText,
                )
                isAuto -> Icon(
                    imageVector = Icons.Filled.Bolt,
                    contentDescription = null,
                    tint = Brand.GreenLight,
                )
                country != null -> Text(text = flagFromCountry(country), style = MaterialTheme.typography.headlineSmall)
                else -> Icon(
                    imageVector = Icons.Outlined.Public,
                    contentDescription = null,
                    tint = Brand.SecondaryText,
                )
            }
        }

        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = stringResource(R.string.home_current_server),
                style = MaterialTheme.typography.labelSmall,
                color = Brand.MutedText,
            )
            Text(
                text = active?.name ?: stringResource(R.string.home_no_profile_selected),
                style = MaterialTheme.typography.titleMedium,
                color = if (connected) Brand.GreenLight else MaterialTheme.colorScheme.onBackground,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }

        Icon(
            imageVector = Icons.Outlined.ExpandMore,
            contentDescription = stringResource(
                if (expanded) R.string.action_collapse else R.string.action_expand,
            ),
            tint = Brand.SecondaryText,
        )
    }
}

@Composable
private fun ProfileDropdown(
    profiles: List<Profile>,
    activeId: String?,
    pings: Map<String, PingRepository.Sample>,
    sortMode: ProfileSortMode,
    onSelect: (Profile) -> Unit,
    onLongPress: (Profile) -> Unit,
) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(8.dp),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        if (profiles.isEmpty()) {
            Text(
                text = stringResource(R.string.home_no_profiles_yet),
                style = MaterialTheme.typography.bodySmall,
                color = Brand.MutedText,
                modifier = Modifier.padding(8.dp),
            )
        } else {
            // SECTION rows are list labels, never selectable. Favourites
            // bubble to the top via sortProfiles regardless of [sortMode].
            val real = profiles.filterNot { it.isSection }
            val ordered = sortProfiles(real, sortMode, pings)

            // Cap the dropdown's *vertical* size so it doesn't push the
            // rest of Home off-screen, but render every profile (no count
            // truncation) — the user wants to pick from the full list
            // without leaving Home.
            LazyColumn(
                modifier = Modifier.heightIn(max = 420.dp),
                verticalArrangement = Arrangement.spacedBy(4.dp),
            ) {
                items(ordered, key = { it.id }) { p ->
                    ServerRow(
                        name = p.name,
                        subtitle = profileSubtitle(p),
                        countryCode = profileCountry(p),
                        isAuto = profileIsAuto(p),
                        isActive = p.id == activeId,
                        isFavorite = p.isFavorite,
                        onClick = { onSelect(p) },
                        onLongClick = { onLongPress(p) },
                        latencyMs = pings[p.id]?.takeIf { it.reachable }?.latencyMs,
                    )
                }
            }
        }
    }
}

@Composable
private fun AddProfileShortcut(onClick: () -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(20.dp))
            .clickable(onClick = onClick)
            .border(
                1.dp,
                Color.White.copy(alpha = 0.12f),
                RoundedCornerShape(20.dp),
            )
            .background(Color.White.copy(alpha = 0.02f))
            .padding(horizontal = 14.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Box(
            modifier = Modifier
                .size(38.dp)
                .clip(RoundedCornerShape(11.dp))
                .background(Color.White.copy(alpha = 0.07f)),
            contentAlignment = Alignment.Center,
        ) {
            Icon(
                imageVector = Icons.Outlined.Add,
                contentDescription = null,
                tint = Brand.SecondaryText,
            )
        }
        Column {
            Text(stringResource(R.string.home_add_server), style = MaterialTheme.typography.titleSmall)
            Text(
                stringResource(R.string.home_add_server_subtitle),
                style = MaterialTheme.typography.bodySmall,
                color = Brand.MutedText,
            )
        }
    }
}

// ───────────────────────── Profile field helpers ──────────────────────────

/** Best-effort country code — proxies to [Profile.country]. */
internal fun profileCountry(p: Profile): String? = p.country()

internal fun profileIsAuto(p: Profile): Boolean {
    val src = p.entryJson.ifBlank { return false }
    return runCatching {
        JSONObject(src).optString("type").equals("AUTO", ignoreCase = true)
    }.getOrDefault(false)
}

internal fun profileSubtitle(p: Profile): String {
    if (p.isSection) return ""
    // Subscription profiles: hide raw host/IP — the provider's panel owns
    // those (and they're usually meaningless to the user). Show only the
    // protocol tag.
    val src = p.entryJson
    val protocolFromEntry = runCatching {
        if (src.isNotBlank()) JSONObject(src).optString("type") else ""
    }.getOrDefault("")
    if (p.subscriptionId.isNotBlank()) {
        return protocolFromEntry.ifBlank { protocolFromUri(p.uri).orEmpty() }
    }
    if (p.uri.isNotBlank()) {
        // Trim long URIs into "vless://… host"
        val short = p.uri.substringBefore("?").take(64)
        return short
    }
    if (src.isBlank()) return ""
    return runCatching {
        val o = JSONObject(src)
        val type = o.optString("type")
        val ip = o.optString("ip")
        val port = o.optInt("port")
        listOfNotNull(
            type.takeIf { it.isNotBlank() },
            "$ip:$port".takeIf { ip.isNotBlank() }
        ).joinToString("  ·  ")
    }.getOrDefault("")
}

/** Extract "vless" / "vmess" / "trojan" etc. from a `<proto>://...` URI. */
private fun protocolFromUri(uri: String): String? {
    val schemeEnd = uri.indexOf("://")
    if (schemeEnd <= 0) return null
    return uri.substring(0, schemeEnd).uppercase()
}
