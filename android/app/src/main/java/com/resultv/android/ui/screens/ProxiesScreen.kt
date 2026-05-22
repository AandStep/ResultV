package com.resultv.android.ui.screens

import androidx.compose.foundation.Image
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
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Bolt
import androidx.compose.material.icons.outlined.CloudDownload
import androidx.compose.material.icons.outlined.DeleteOutline
import androidx.compose.material.icons.outlined.Dns
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.ExpandLess
import androidx.compose.material.icons.outlined.ExpandMore
import androidx.compose.material.icons.outlined.ListAlt
import androidx.compose.material.icons.outlined.NetworkPing
import androidx.compose.material.icons.outlined.Refresh
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.resultv.android.R
import com.resultv.android.theme.Brand
import com.resultv.android.ui.components.ProfileEditSheet
import com.resultv.android.ui.components.ProfileSortMenu
import com.resultv.android.ui.components.ProfileSortMode
import com.resultv.android.ui.components.ProtocolFilterChips
import com.resultv.android.ui.components.ServerRow
import com.resultv.android.ui.components.sortProfiles
import com.resultv.android.vpn.PingRepository
import com.resultv.android.vpn.Profile
import com.resultv.android.vpn.ProfileRepository
import com.resultv.android.vpn.Subscription
import com.resultv.android.vpn.SubscriptionRepository
import com.resultv.android.vpn.SubscriptionUsage
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import mobile.Mobile
import org.json.JSONArray
import org.json.JSONObject
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.concurrent.TimeUnit

@Composable
fun ProxiesScreen(onAddPressed: () -> Unit) {
    val state by ProfileRepository.state.collectAsStateWithLifecycle()
    val subscriptions by SubscriptionRepository.state.collectAsStateWithLifecycle()
    val pings by PingRepository.results.collectAsStateWithLifecycle()
    var pendingDeleteProfile by remember { mutableStateOf<Profile?>(null) }
    var pendingDeleteSub by remember { mutableStateOf<Subscription?>(null) }
    var refreshingSubId by remember { mutableStateOf<String?>(null) }
    var sortMode by remember { mutableStateOf(ProfileSortMode.Default) }
    var editingProfileId by remember { mutableStateOf<String?>(null) }
    var editingSubId by remember { mutableStateOf<String?>(null) }
    var protocolFilter by remember { mutableStateOf(emptySet<String>()) }
    val ctx = LocalContext.current
    val scope = rememberCoroutineScope()
    val dataDir = remember(ctx) { ctx.filesDir.absolutePath }

    Column(modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp, vertical = 12.dp)) {
        if (state.profiles.isEmpty() && subscriptions.subs.isEmpty()) {
            EmptyState(onAddPressed)
            return@Column
        }

        Row(
            modifier = Modifier.fillMaxWidth().padding(bottom = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                text = stringResource(R.string.proxies_count, state.profiles.count { !it.isSection }),
                style = MaterialTheme.typography.labelLarge,
                color = Brand.SecondaryText,
                modifier = Modifier.weight(1f),
            )
            IconButton(onClick = { PingRepository.refreshAll(state.profiles) }) {
                Icon(
                    imageVector = Icons.Outlined.NetworkPing,
                    contentDescription = stringResource(R.string.ping_refresh_cd),
                    tint = Brand.SecondaryText,
                )
            }
            ProfileSortMenu(mode = sortMode, onModeChange = { sortMode = it })
        }

        // Protocol filter chips. Hidden when the underlying data only
        // contains a single protocol — ProtocolFilterChips itself bails
        // out below the 2-entry threshold.
        val availableProtocols = remember(state.profiles) {
            state.profiles.asSequence()
                .filterNot { it.isSection }
                .map { profileProtocol(it) }
                .filter { it.isNotEmpty() }
                .toSet()
        }
        ProtocolFilterChips(
            selected = protocolFilter,
            available = availableProtocols,
            onToggle = { code ->
                protocolFilter = if (code in protocolFilter)
                    protocolFilter - code else protocolFilter + code
            },
            modifier = Modifier.padding(bottom = 8.dp),
        )

        // Group profiles by subscription. Unaffiliated ("My proxies") go
        // in their own bucket; SECTION rows stay with their subscription
        // and keep their original order so impVPN's "👇 выберите конфиг
        // ниже" labels land between the right blocks.
        val standaloneRaw = state.profiles.filter { it.subscriptionId.isBlank() && !it.isSection }
        val standalone = if (protocolFilter.isEmpty()) standaloneRaw
            else standaloneRaw.filter { profileProtocol(it) in protocolFilter }

        LazyColumn(
            modifier = Modifier.fillMaxSize(),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            if (standalone.isNotEmpty()) {
                item("standalone-header") {
                    StandaloneHeader(standalone.size)
                }
                items(sortProfiles(standalone, sortMode, pings), key = { it.id }) { p ->
                    ProfileCard(
                        profile = p,
                        activeId = state.activeId,
                        sample = pings[p.id],
                        onClick = { ProfileRepository.setActive(p.id) },
                        onLongClick = { editingProfileId = p.id },
                    )
                }
            }

            subscriptions.subs.forEach { sub ->
                val raw = state.profiles.filter { it.subscriptionId == sub.id }
                // When a protocol filter is on, drop sections (they're labels
                // for surrounding rows) and any row whose protocol isn't picked.
                val subProfiles = if (protocolFilter.isEmpty()) raw
                    else raw.filterNot { it.isSection }
                        .filter { profileProtocol(it) in protocolFilter }
                if (protocolFilter.isNotEmpty() && subProfiles.isEmpty()) return@forEach
                item("sub-${sub.id}") {
                    SubscriptionGroup(
                        subscription = sub,
                        profiles = subProfiles,
                        activeId = state.activeId,
                        pings = pings,
                        sortMode = sortMode,
                        refreshing = refreshingSubId == sub.id,
                        onPickProfile = { p -> ProfileRepository.setActive(p.id) },
                        onLongPressProfile = { p -> editingProfileId = p.id },
                        onRefresh = {
                            if (refreshingSubId != null) return@SubscriptionGroup
                            refreshingSubId = sub.id
                            scope.launch {
                                refreshSubscription(sub, dataDir)
                                refreshingSubId = null
                            }
                        },
                        onEdit = { editingSubId = sub.id },
                        onDelete = { pendingDeleteSub = sub },
                    )
                }
            }
        }
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

    editingProfileId?.let { id ->
        val target = state.profiles.firstOrNull { it.id == id }
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

    editingSubId?.let { id ->
        val target = subscriptions.subs.firstOrNull { it.id == id }
        if (target == null) {
            editingSubId = null
            return@let
        }
        SubscriptionUrlEditDialog(
            initialUrl = target.url,
            onDismiss = { editingSubId = null },
            onSave = { newUrl ->
                editingSubId = null
                SubscriptionRepository.update(id) { it.copy(url = newUrl) }
                if (refreshingSubId != null) return@SubscriptionUrlEditDialog
                refreshingSubId = id
                scope.launch {
                    SubscriptionRepository.byId(id)?.let { refreshSubscription(it, dataDir) }
                    refreshingSubId = null
                }
            },
        )
    }

    pendingDeleteSub?.let { target ->
        val children = state.profiles.count { it.subscriptionId == target.id && !it.isSection }
        AlertDialog(
            onDismissRequest = { pendingDeleteSub = null },
            title = { Text(stringResource(R.string.sub_delete_title)) },
            text = {
                Text(
                    stringResource(R.string.sub_delete_message, target.displayName, children),
                    color = Brand.SecondaryText,
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    SubscriptionRepository.delete(target.id)
                    pendingDeleteSub = null
                }) { Text(stringResource(R.string.action_delete), color = Brand.Danger) }
            },
            dismissButton = {
                TextButton(onClick = { pendingDeleteSub = null }) { Text(stringResource(R.string.action_cancel)) }
            },
        )
    }
}

@Composable
private fun StandaloneHeader(count: Int) {
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        modifier = Modifier.padding(start = 4.dp, bottom = 2.dp, top = 2.dp),
    ) {
        Icon(
            imageVector = Icons.Outlined.Bolt,
            contentDescription = null,
            tint = Brand.SecondaryText,
            modifier = Modifier.size(16.dp),
        )
        Text(
            text = stringResource(R.string.proxies_standalone_header, count),
            style = MaterialTheme.typography.labelMedium,
            color = Brand.MutedText,
        )
    }
}

@Composable
private fun ProfileCard(
    profile: Profile,
    activeId: String?,
    sample: PingRepository.Sample?,
    onClick: () -> Unit,
    onLongClick: () -> Unit,
) {
    ServerRow(
        name = profile.name,
        subtitle = profileSubtitle(profile),
        countryCode = profile.country(),
        isAuto = profileIsAuto(profile),
        isActive = profile.id == activeId,
        isFavorite = profile.isFavorite,
        onClick = onClick,
        onLongClick = onLongClick,
        latencyMs = sample?.takeIf { it.reachable }?.latencyMs,
    )
}

@Composable
private fun SubscriptionGroup(
    subscription: Subscription,
    profiles: List<Profile>,
    activeId: String?,
    pings: Map<String, PingRepository.Sample>,
    sortMode: ProfileSortMode,
    refreshing: Boolean,
    onPickProfile: (Profile) -> Unit,
    onLongPressProfile: (Profile) -> Unit,
    onRefresh: () -> Unit,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
) {
    var collapsed by remember(subscription.id) { mutableStateOf(false) }

    Card(
        shape = RoundedCornerShape(20.dp),
        colors = CardDefaults.cardColors(containerColor = Brand.Surface),
        border = null,
        modifier = Modifier
            .fillMaxWidth()
            .border(
                1.dp,
                Color.White.copy(alpha = 0.06f),
                RoundedCornerShape(20.dp),
            ),
    ) {
        Column {
            SubscriptionHeader(
                subscription = subscription,
                profileCount = profiles.count { !it.isSection },
                collapsed = collapsed,
                refreshing = refreshing,
                onToggleCollapsed = { collapsed = !collapsed },
                onRefresh = onRefresh,
                onEdit = onEdit,
                onDelete = onDelete,
            )

            if (!collapsed) {
                // Sort: favourites first + user-chosen sort mode applied
                // within each SECTION-bounded block (sections are labels
                // for the rows that follow, so reorder must not cross them).
                val ordered = reorderForDisplay(profiles, sortMode, pings)
                Column(
                    modifier = Modifier.padding(horizontal = 8.dp, vertical = 4.dp),
                    verticalArrangement = Arrangement.spacedBy(4.dp),
                ) {
                    ordered.forEach { p ->
                        if (p.isSection) {
                            SectionLabel(p.name)
                        } else {
                            ServerRow(
                                name = p.name,
                                subtitle = profileSubtitle(p),
                                countryCode = p.country(),
                                isAuto = profileIsAuto(p),
                                isActive = p.id == activeId,
                                isFavorite = p.isFavorite,
                                onClick = { onPickProfile(p) },
                                onLongClick = { onLongPressProfile(p) },
                                latencyMs = pings[p.id]?.takeIf { it.reachable }?.latencyMs,
                            )
                        }
                    }
                }
            }
        }
    }
}

private fun reorderForDisplay(
    profiles: List<Profile>,
    sortMode: ProfileSortMode,
    pings: Map<String, PingRepository.Sample>,
): List<Profile> {
    if (profiles.isEmpty()) return profiles
    val result = ArrayList<Profile>(profiles.size)
    val current = ArrayList<Profile>()
    fun flush() {
        result += sortProfiles(current, sortMode, pings)
        current.clear()
    }
    for (p in profiles) {
        if (p.isSection) {
            flush()
            result += p
        } else {
            current += p
        }
    }
    flush()
    return result
}

@Composable
private fun SubscriptionHeader(
    subscription: Subscription,
    profileCount: Int,
    collapsed: Boolean,
    refreshing: Boolean,
    onToggleCollapsed: () -> Unit,
    onRefresh: () -> Unit,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
) {
    val usage = remember(subscription.userInfo) { SubscriptionUsage.parse(subscription.userInfo) }
    val usesImpLogo = remember(subscription.id, subscription.name, subscription.source) {
        subscriptionUsesImpLogo(subscription)
    }

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onToggleCollapsed)
            .padding(horizontal = 14.dp, vertical = 12.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        // Row 1 — [chevron] [logo] [title] [refresh] [delete]
        // Chevron is leading per the desktop-parity mock; refresh and
        // delete are independent circular chips (no merged pill).
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            ChevronChip(collapsed = collapsed, onClick = onToggleCollapsed)
            SubscriptionLogo(usesImpLogo = usesImpLogo)
            Text(
                text = subscription.displayName,
                style = MaterialTheme.typography.titleSmall,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f),
            )
            CircleActionChip(
                onClick = onRefresh,
                enabled = !refreshing,
                contentDescription = stringResource(R.string.sub_refresh_cd),
            ) {
                if (refreshing) {
                    CircularProgressIndicator(
                        modifier = Modifier.size(14.dp),
                        strokeWidth = 2.dp,
                        color = Brand.GreenLight,
                    )
                } else {
                    Icon(
                        imageVector = Icons.Outlined.Refresh,
                        contentDescription = null,
                        tint = Brand.SecondaryText,
                        modifier = Modifier.size(16.dp),
                    )
                }
            }
            CircleActionChip(
                onClick = onEdit,
                contentDescription = stringResource(R.string.sub_edit_cd),
            ) {
                Icon(
                    imageVector = Icons.Outlined.Edit,
                    contentDescription = null,
                    tint = Brand.SecondaryText,
                    modifier = Modifier.size(16.dp),
                )
            }
            CircleActionChip(
                onClick = onDelete,
                contentDescription = stringResource(R.string.sub_delete_cd),
            ) {
                Icon(
                    imageVector = Icons.Outlined.DeleteOutline,
                    contentDescription = null,
                    tint = Brand.MutedText,
                    modifier = Modifier.size(16.dp),
                )
            }
        }

        // Row 2 — "Осталось N дней | [progress] used / total" packed into
        // one line. Renders whenever we have *any* signal: quota, expiry,
        // or just used bytes (panels often omit `total=` from the
        // Subscription-Userinfo header but still report `download=`).
        if (usage.hasQuota || usage.hasExpiry || usage.used > 0) {
            UsageInlineStrip(usage)
        }

        // Row 3 — small footer: last refresh + server count.
        SubscriptionFooter(subscription.lastFetchedAt, profileCount)
    }
}

/** Leading chevron — flips Up/Down based on [collapsed]. */
@Composable
private fun ChevronChip(collapsed: Boolean, onClick: () -> Unit) {
    Box(
        modifier = Modifier
            .size(32.dp)
            .clip(RoundedCornerShape(50))
            .clickable(onClick = onClick),
        contentAlignment = Alignment.Center,
    ) {
        Icon(
            imageVector = if (collapsed) Icons.Outlined.ExpandMore else Icons.Outlined.ExpandLess,
            contentDescription = stringResource(
                if (collapsed) R.string.action_expand else R.string.action_collapse,
            ),
            tint = Brand.SecondaryText,
            modifier = Modifier.size(20.dp),
        )
    }
}

/**
 * Round 32dp action chip — refresh/delete in the header use this.
 *
 * Plain Box + clickable instead of IconButton: IconButton enforces a 48dp
 * minimum tap target which made the chips overflow into the title text
 * even when sized to 32dp. The role+contentDescription semantics keep
 * accessibility behaviour.
 */
@Composable
private fun CircleActionChip(
    onClick: () -> Unit,
    contentDescription: String,
    enabled: Boolean = true,
    content: @Composable () -> Unit,
) {
    val cdState = contentDescription
    Box(
        modifier = Modifier
            .size(32.dp)
            .clip(RoundedCornerShape(50))
            .background(Color.White.copy(alpha = 0.06f))
            .clickable(enabled = enabled, onClick = onClick)
            .semantics {
                this.contentDescription = cdState
                this.role = Role.Button
            },
        contentAlignment = Alignment.Center,
    ) {
        content()
    }
}

@Composable
private fun SubscriptionLogo(usesImpLogo: Boolean) {
    Box(
        modifier = Modifier
            .size(40.dp)
            .clip(RoundedCornerShape(12.dp))
            .background(
                if (usesImpLogo) Brand.Green.copy(alpha = 0.18f)
                else Color.White.copy(alpha = 0.07f)
            ),
        contentAlignment = Alignment.Center,
    ) {
        if (usesImpLogo) {
            // Real impVPN brand artwork (PNG copied from the PC frontend
            // assets) — replaces the rocket-emoji placeholder.
            Image(
                painter = painterResource(R.drawable.imp_logo),
                contentDescription = null,
                contentScale = ContentScale.Fit,
                modifier = Modifier.size(32.dp),
            )
        } else {
            Icon(
                imageVector = Icons.Outlined.CloudDownload,
                contentDescription = null,
                tint = Brand.SecondaryText,
                modifier = Modifier.size(22.dp),
            )
        }
    }
}

/**
 * One-line strip packing days-left + traffic progress + used/total. Three
 * cells separated by a thin divider, mirroring the desktop mock:
 *
 *   "Осталось 25 дней │ ▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬ │ 203.84 ГБ / ∞"
 *
 * Renders only the cells we have data for; the progress bar fills the
 * available middle space.
 */
@Composable
private fun UsageInlineStrip(usage: SubscriptionUsage) {
    val daysLeft = if (usage.hasExpiry) {
        TimeUnit.MILLISECONDS.toDays(usage.expireUnix * 1000L - System.currentTimeMillis())
            .coerceAtLeast(0L)
    } else 0L
    val daysLeftInt = daysLeft.coerceAtMost(Int.MAX_VALUE.toLong()).toInt()
    val daysLeftText = when {
        !usage.hasExpiry -> ""
        usage.expired -> stringResource(R.string.sub_expired)
        else -> pluralStringResource(R.plurals.sub_days_left, daysLeftInt, daysLeftInt)
    }
    val expireOnText = if (usage.hasExpiry && !usage.expired) {
        val formatted = remember(usage.expireUnix) {
            SimpleDateFormat("dd.MM.yy HH:mm", Locale.getDefault())
                .format(Date(usage.expireUnix * 1000L))
        }
        stringResource(R.string.sub_expires_on, formatted)
    } else ""
    val daysColour = when {
        !usage.hasExpiry -> Brand.MutedText
        usage.expired -> Brand.Danger
        daysLeft <= 7 -> Brand.Warning
        else -> Brand.SecondaryText
    }
    val ratio = if (usage.hasQuota && usage.total > 0)
        (usage.used.toFloat() / usage.total.toFloat()).coerceIn(0f, 1f)
    else 0f

    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        if (usage.hasExpiry) {
            Column(verticalArrangement = Arrangement.spacedBy(1.dp)) {
                Text(
                    text = daysLeftText,
                    style = MaterialTheme.typography.labelSmall,
                    color = daysColour,
                    maxLines = 1,
                )
                if (expireOnText.isNotEmpty()) {
                    Text(
                        text = expireOnText,
                        style = MaterialTheme.typography.labelSmall,
                        color = Brand.MutedText,
                        maxLines = 1,
                    )
                }
            }
            ThinVerticalDivider()
        }
        when {
            usage.hasQuota -> {
                LinearProgressIndicator(
                    progress = { ratio },
                    modifier = Modifier
                        .weight(1f)
                        .height(4.dp),
                    color = if (ratio > 0.9f) Brand.Danger else Brand.GreenLight,
                    trackColor = Color.White.copy(alpha = 0.08f),
                )
                Text(
                    text = formatBytesPair(usage.used, usage.total),
                    style = MaterialTheme.typography.labelSmall,
                    color = Brand.SecondaryText,
                    maxLines = 1,
                )
            }
            usage.used > 0 -> {
                // No quota declared — show only "USED / ∞" so the user
                // still sees how much they've spent. Skip the progress bar
                // (there's no denominator to fill it against).
                Spacer(Modifier.weight(1f))
                Text(
                    text = stringResource(
                        R.string.sub_traffic_used_unlimited,
                        formatBytesShort(usage.used),
                    ),
                    style = MaterialTheme.typography.labelSmall,
                    color = Brand.SecondaryText,
                    maxLines = 1,
                )
            }
            else -> {
                // Filler so the days-left line still spans the row when
                // there's nothing else to display.
                Spacer(Modifier.weight(1f))
            }
        }
    }
}

@Composable
private fun ThinVerticalDivider() {
    Box(
        modifier = Modifier
            .size(width = 1.dp, height = 14.dp)
            .background(Color.White.copy(alpha = 0.10f)),
    )
}

/**
 * "DD.MM.YY, HH:MM · N серверов 📋" — small right-aligned footer under
 * the header. Hidden when [lastFetchedAt] is 0 (never refreshed, e.g.
 * just-imported via deep-link).
 */
@Composable
private fun SubscriptionFooter(lastFetchedAt: Long, profileCount: Int) {
    val timestamp = remember(lastFetchedAt) {
        if (lastFetchedAt <= 0L) ""
        else SimpleDateFormat("dd.MM.yy, HH:mm", Locale.getDefault())
            .format(Date(lastFetchedAt))
    }
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.End,
    ) {
        if (timestamp.isNotEmpty()) {
            Text(
                text = timestamp,
                style = MaterialTheme.typography.labelSmall,
                color = Brand.MutedText,
            )
            Spacer(Modifier.width(8.dp))
            Box(
                modifier = Modifier
                    .size(width = 1.dp, height = 10.dp)
                    .background(Color.White.copy(alpha = 0.10f)),
            )
            Spacer(Modifier.width(8.dp))
        }
        Text(
            text = pluralStringResource(R.plurals.sub_footer_servers, profileCount, profileCount),
            style = MaterialTheme.typography.labelSmall,
            color = Brand.MutedText,
        )
        Spacer(Modifier.width(4.dp))
        Icon(
            imageVector = Icons.Outlined.Dns,
            contentDescription = null,
            tint = Brand.MutedText,
            modifier = Modifier.size(12.dp),
        )
    }
}

@Composable
private fun SectionLabel(text: String) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 8.dp, vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Icon(
            imageVector = Icons.Outlined.ListAlt,
            contentDescription = null,
            tint = Brand.Favorite,
            modifier = Modifier.size(14.dp),
        )
        Text(
            text = text,
            style = MaterialTheme.typography.labelMedium,
            color = Brand.SecondaryText,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
        )
    }
}

@Composable
private fun EmptyState(onAddPressed: () -> Unit) {
    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Column(
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Text(
                stringResource(R.string.proxies_empty_title),
                style = MaterialTheme.typography.titleLarge,
                fontWeight = FontWeight.SemiBold,
            )
            Text(
                stringResource(R.string.proxies_empty_subtitle),
                style = MaterialTheme.typography.bodyMedium,
                color = Brand.SecondaryText,
            )
            Spacer(Modifier.height(4.dp))
            FilledTonalButton(onClick = onAddPressed) {
                Icon(Icons.Outlined.Add, contentDescription = null)
                Spacer(Modifier.fillMaxWidth(0.05f))
                Text(stringResource(R.string.home_add_server))
            }
        }
    }
}

/**
 * impVPN logo override: pulled when the subscription came from a
 * `resultv://rvsub/…` deep link, or when the (decoded) name / title /
 * URL mentions impVPN. Mirrors `subscriptionUsesImpLogo` on the PC side.
 * `displayName` already runs `base64:` decoding, so panels that wrap their
 * Profile-Title in base64 still light up the logo.
 */
private fun subscriptionUsesImpLogo(s: Subscription): Boolean {
    if (s.source == "rvsub") return true
    val haystack = buildString {
        append(s.displayName).append(' ')
        append(com.resultv.android.vpn.decodePanelTitle(s.name)).append(' ')
        append(com.resultv.android.vpn.decodePanelTitle(s.title)).append(' ')
        append(s.url)
    }.lowercase()
    return "impvpn" in haystack || "imp vpn" in haystack
}

/** Re-fetch a subscription, replace its profiles (preserving favourites by name). */
private suspend fun refreshSubscription(sub: Subscription, dataDir: String) {
    val responseJson = try {
        withContext(Dispatchers.IO) { Mobile.fetchSubscriptionV2(sub.url, dataDir) }
    } catch (_: Throwable) {
        return
    }
    val response = JSONObject(responseJson)
    val arr = response.optJSONArray("entries") ?: JSONArray()

    val existingFavouriteNames = ProfileRepository.state.value.profiles
        .asSequence()
        .filter { it.subscriptionId == sub.id && it.isFavorite }
        .map { it.name }
        .toSet()

    val fresh = (0 until arr.length()).mapNotNull { i ->
        val o = arr.getJSONObject(i)
        val type = o.optString("type")
        val name = o.optString("name").ifBlank { "Profile ${i + 1}" }
        val isSection = type.equals("SECTION", ignoreCase = true)
        when {
            isSection -> Profile.section(name = name, subscriptionId = sub.id)
            else -> {
                val uri = o.optString("uri")
                val base = if (uri.isNotBlank())
                    Profile.fromUri(name, uri, subscriptionId = sub.id)
                else
                    Profile.fromEntryJson(name, o.toString(), subscriptionId = sub.id)
                base.copy(isFavorite = base.name in existingFavouriteNames)
            }
        }
    }
    ProfileRepository.replaceForSubscription(sub.id, fresh)
    SubscriptionRepository.update(sub.id) {
        it.copy(
            title = response.optString("title").ifBlank { it.title },
            userInfo = response.optString("userInfo"),
            lastFetchedAt = System.currentTimeMillis(),
        )
    }
}

private val BYTE_UNITS = arrayOf("B", "KB", "MB", "GB", "TB")

private fun scaleBytes(bytes: Long): Pair<Double, Int> {
    if (bytes <= 0) return 0.0 to 0
    var v = bytes.toDouble()
    var i = 0
    while (v >= 1024 && i < BYTE_UNITS.size - 1) {
        v /= 1024.0
        i++
    }
    return v to i
}

private fun formatScaled(v: Double): String =
    if (v >= 100) String.format(Locale.US, "%.0f", v)
    else String.format(Locale.US, "%.1f", v)

private fun formatBytesShort(bytes: Long): String {
    val (v, i) = scaleBytes(bytes)
    return "${formatScaled(v)} ${BYTE_UNITS[i]}"
}

@Composable
private fun SubscriptionUrlEditDialog(
    initialUrl: String,
    onDismiss: () -> Unit,
    onSave: (String) -> Unit,
) {
    var url by remember(initialUrl) { mutableStateOf(initialUrl) }
    val trimmed = url.trim()
    val changed = trimmed != initialUrl.trim()
    val valid = trimmed.startsWith("http://", ignoreCase = true) ||
        trimmed.startsWith("https://", ignoreCase = true)
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.sub_edit_title)) },
        text = {
            OutlinedTextField(
                value = url,
                onValueChange = { url = it },
                label = { Text(stringResource(R.string.sub_edit_label)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
        },
        confirmButton = {
            TextButton(
                enabled = valid && changed,
                onClick = { onSave(trimmed) },
            ) {
                Text(stringResource(R.string.sub_edit_save))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) {
                Text(stringResource(R.string.action_cancel))
            }
        },
    )
}

/**
 * Pair-formatter mirroring desktop's `formatTrafficBytes` usage in
 * `ProxyListView.jsx`: when the two values share a unit suffix, emit
 * "18.4 / 50 GB" (single suffix). Otherwise fall back to "1.2 MB / 50 GB".
 */
private fun formatBytesPair(used: Long, total: Long): String {
    val (uV, uI) = scaleBytes(used)
    val (tV, tI) = scaleBytes(total)
    return if (uI == tI) {
        "${formatScaled(uV)} / ${formatScaled(tV)} ${BYTE_UNITS[uI]}"
    } else {
        "${formatBytesShort(used)} / ${formatBytesShort(total)}"
    }
}
