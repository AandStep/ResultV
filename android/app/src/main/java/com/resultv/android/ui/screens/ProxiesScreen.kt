package com.resultv.android.ui.screens

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
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Bolt
import androidx.compose.material.icons.outlined.DeleteOutline
import androidx.compose.material.icons.outlined.Dns
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.ExpandLess
import androidx.compose.material.icons.outlined.ExpandMore
import androidx.compose.material.icons.outlined.ListAlt
import androidx.compose.material.icons.outlined.Refresh
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
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
import com.resultv.android.ui.components.SubscriptionEditSheet
import com.resultv.android.ui.components.SubscriptionLogo
import com.resultv.android.ui.components.sortProfiles
import com.resultv.android.ui.components.subscriptionUsesImpLogo
import com.resultv.android.vpn.CountryRepository
import com.resultv.android.vpn.PingRepository
import com.resultv.android.vpn.Profile
import com.resultv.android.vpn.ProfileRepository
import com.resultv.android.vpn.Subscription
import com.resultv.android.vpn.SubscriptionRefresher
import com.resultv.android.vpn.SubscriptionRepository
import com.resultv.android.vpn.SubscriptionUsage
import kotlinx.coroutines.launch
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.concurrent.TimeUnit

@Composable
fun ProxiesScreen(onAddPressed: () -> Unit) {
    val state by ProfileRepository.state.collectAsStateWithLifecycle()
    val subscriptions by SubscriptionRepository.state.collectAsStateWithLifecycle()
    val pings by PingRepository.results.collectAsStateWithLifecycle()
    val pingInflight by PingRepository.inflight.collectAsStateWithLifecycle()
    val countries by CountryRepository.results.collectAsStateWithLifecycle()
    // Transient — these are dialogs/sheets that should always start dismissed.
    var pendingDeleteProfile by remember { mutableStateOf<Profile?>(null) }
    var pendingDeleteSub by remember { mutableStateOf<Subscription?>(null) }
    var refreshingSubId by remember { mutableStateOf<String?>(null) }
    var editingProfileId by remember { mutableStateOf<String?>(null) }
    var fullEditProfileId by remember { mutableStateOf<String?>(null) }
    var editingSubId by remember { mutableStateOf<String?>(null) }
    // Persisted across tab switches via the SaveableStateHolder in AppShell.
    var sortMode by rememberSaveable { mutableStateOf(ProfileSortMode.Default) }
    var protocolFilter by rememberSaveable { mutableStateOf<List<String>>(emptyList()) }
    // Subscription collapse state hoisted here so each row can live as its
    // own LazyColumn item (true virtualisation). List-of-ids form is what
    // rememberSaveable can persist; the Set is the runtime lookup form.
    var collapsedSubsList by rememberSaveable { mutableStateOf<List<String>>(emptyList()) }
    val collapsedSubs = remember(collapsedSubsList) { collapsedSubsList.toSet() }
    val listState = rememberLazyListState()
    val ctx = LocalContext.current
    val scope = rememberCoroutineScope()
    val dataDir = remember(ctx) { ctx.filesDir.absolutePath }

    // Resolve flags by GeoIP for any server whose name carries no country
    // hint. Re-runs when the profile set changes (import / refresh); the
    // repository skips already-resolved and in-flight entries.
    LaunchedEffect(state.profiles) {
        CountryRepository.resolve(state.profiles, dataDir)
    }

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
                    imageVector = Icons.Outlined.Bolt,
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
                .map { it.protocol }
                .filter { it.isNotEmpty() }
                .toSet()
        }
        // Membership tests inside the chip use Set semantics — the list
        // form is just for [rememberSaveable] friendliness.
        val protocolFilterSet = remember(protocolFilter) { protocolFilter.toSet() }
        ProtocolFilterChips(
            selected = protocolFilterSet,
            available = availableProtocols,
            onToggle = { code ->
                protocolFilter = if (code in protocolFilterSet)
                    protocolFilter - code else protocolFilter + code
            },
            modifier = Modifier.padding(bottom = 8.dp),
        )

        // Group profiles by subscription. Unaffiliated ("My proxies") go
        // in their own bucket; SECTION rows stay with their subscription
        // and keep their original order so impVPN's "👇 выберите конфиг
        // ниже" labels land between the right blocks.
        //
        // Memoised on (profiles, filter) so a ping-only update doesn't
        // re-walk every profile to rebuild the bucket lists.
        val standalone = remember(state.profiles, protocolFilterSet) {
            val raw = state.profiles.filter { it.subscriptionId.isBlank() && !it.isSection }
            if (protocolFilterSet.isEmpty()) raw
            else raw.filter { it.protocol in protocolFilterSet }
        }
        val sortedStandalone = remember(standalone, sortMode, pings) {
            sortProfiles(standalone, sortMode, pings)
        }
        // Pre-bucket subscription profiles once per (profiles, subs, filter)
        // change. Each bucket's rows are rendered as individual LazyColumn
        // items below, so opening Proxies only composes rows actually on
        // screen instead of all N at once.
        val subscriptionBuckets = remember(state.profiles, subscriptions.subs, protocolFilterSet) {
            subscriptions.subs.mapNotNull { sub ->
                val raw = state.profiles.filter { it.subscriptionId == sub.id }
                val subProfiles = if (protocolFilterSet.isEmpty()) raw
                    else raw.filterNot { it.isSection }
                        .filter { it.protocol in protocolFilterSet }
                if (protocolFilterSet.isNotEmpty() && subProfiles.isEmpty()) null
                else sub to subProfiles
            }
        }
        // Sort each bucket once per (buckets, mode, pings) change — the
        // LazyColumn body just walks the pre-sorted lists.
        val orderedBySub = remember(subscriptionBuckets, sortMode, pings) {
            subscriptionBuckets.associate { (sub, profiles) ->
                sub.id to reorderForDisplay(profiles, sortMode, pings)
            }
        }

        LazyColumn(
            state = listState,
            modifier = Modifier.fillMaxSize(),
        ) {
            if (sortedStandalone.isNotEmpty()) {
                item("standalone-header", contentType = "standalone-header") {
                    StandaloneHeader(sortedStandalone.size)
                }
                items(sortedStandalone, key = { it.id }, contentType = { "standalone-row" }) { p ->
                    Box(modifier = Modifier.padding(top = 12.dp)) {
                        ProfileCard(
                            profile = p,
                            activeId = state.activeId,
                            sample = pings[p.id],
                            country = p.country ?: countries[p.id],
                            isLoading = p.id in pingInflight,
                            onClick = { ProfileRepository.setActive(p.id) },
                            onLongClick = { editingProfileId = p.id },
                        )
                    }
                }
            }

            subscriptionBuckets.forEachIndexed { idx, (sub, subProfiles) ->
                val collapsed = sub.id in collapsedSubs
                val needsTopGap = idx > 0 || sortedStandalone.isNotEmpty()
                val ordered = orderedBySub[sub.id].orEmpty()

                item("sub-${sub.id}-head", contentType = "sub-head") {
                    SubscriptionHeaderBlock(
                        modifier = if (needsTopGap) Modifier.padding(top = 12.dp) else Modifier,
                        subscription = sub,
                        profileCount = subProfiles.count { !it.isSection },
                        collapsed = collapsed,
                        refreshing = refreshingSubId == sub.id,
                        onToggleCollapsed = {
                            collapsedSubsList = if (sub.id in collapsedSubs)
                                collapsedSubsList - sub.id
                            else
                                collapsedSubsList + sub.id
                        },
                        onRefresh = onRefresh@{
                            if (refreshingSubId != null) return@onRefresh
                            refreshingSubId = sub.id
                            scope.launch {
                                runCatching { SubscriptionRefresher.refreshOne(sub, dataDir) }
                                refreshingSubId = null
                            }
                        },
                        onEdit = { editingSubId = sub.id },
                        onDelete = { pendingDeleteSub = sub },
                    )
                }

                if (!collapsed) {
                    items(
                        ordered,
                        key = { "sub-${sub.id}-${it.id}" },
                        contentType = { if (it.isSection) "sub-sec" else "sub-row" },
                    ) { p ->
                        if (p.isSection) {
                            SubscriptionSectionRowBlock(p.name)
                        } else {
                            SubscriptionServerRowBlock(
                                profile = p,
                                activeId = state.activeId,
                                sample = pings[p.id],
                                country = p.country ?: countries[p.id],
                                isLoading = p.id in pingInflight,
                                onClick = { ProfileRepository.setActive(p.id) },
                                onLongClick = { editingProfileId = p.id },
                            )
                        }
                    }
                    item("sub-${sub.id}-foot", contentType = "sub-foot") {
                        SubscriptionTrailingCap()
                    }
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
        val target = state.profiles.firstOrNull { it.id == id }
        if (target == null || target.isSection) {
            fullEditProfileId = null
            return@let
        }
        ProfileFullEditSheet(
            profile = target,
            onDismiss = { fullEditProfileId = null },
        )
    }

    editingSubId?.let { id ->
        val target = subscriptions.subs.firstOrNull { it.id == id }
        if (target == null) {
            editingSubId = null
            return@let
        }
        SubscriptionEditSheet(
            subscription = target,
            onDismiss = { editingSubId = null },
            onSave = { result ->
                editingSubId = null
                // Treat a custom name that matches the panel display name
                // as "no override" so we don't freeze a stale title across
                // future refreshes. Trim handles the visibility-only edit
                // case where the user never touched the field.
                val nameOverride = result.customName
                    .takeIf { it.isNotBlank() && it != target.displayName }
                    .orEmpty()
                SubscriptionRepository.update(id) { sub ->
                    sub.copy(
                        customName = nameOverride,
                        hiddenOnHome = result.hiddenOnHome,
                        customRefreshIntervalMinutes = result.customRefreshIntervalMinutes,
                    )
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
    country: String?,
    isLoading: Boolean,
    onClick: () -> Unit,
    onLongClick: () -> Unit,
) {
    ServerRow(
        name = profile.name,
        subtitle = profile.subtitle,
        countryCode = country,
        isAuto = profile.isAuto,
        isActive = profile.id == activeId,
        isFavorite = profile.isFavorite,
        onClick = onClick,
        onLongClick = onLongClick,
        latencyMs = sample?.takeIf { it.reachable }?.latencyMs,
        offlineReason = sample?.takeUnless { it.reachable }?.reason,
        isLoading = isLoading,
    )
}

/**
 * Top portion of a subscription "card", but rendered as its own LazyColumn
 * item so it doesn't drag the body's row composition along with it. Shape
 * is fully-rounded when collapsed (header is the only chunk) and only
 * top-rounded when the body items follow below.
 */
@Composable
private fun SubscriptionHeaderBlock(
    modifier: Modifier = Modifier,
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
    val shape = if (collapsed) RoundedCornerShape(20.dp)
    else RoundedCornerShape(topStart = 20.dp, topEnd = 20.dp)

    Column(
        modifier = modifier
            .fillMaxWidth()
            .clip(shape)
            .background(Brand.Surface)
            .clickable(onClick = onToggleCollapsed)
            .padding(horizontal = 14.dp, vertical = 12.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
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

        if (usage.hasQuota || usage.hasExpiry || usage.used > 0) {
            UsageInlineStrip(usage)
        }

        SubscriptionFooter(subscription.lastFetchedAt, profileCount)
    }
}

/** Body row in a subscription, sharing the surface background of the header. */
@Composable
private fun SubscriptionServerRowBlock(
    profile: Profile,
    activeId: String?,
    sample: PingRepository.Sample?,
    country: String?,
    isLoading: Boolean,
    onClick: () -> Unit,
    onLongClick: () -> Unit,
) {
    Box(
        modifier = Modifier
            .fillMaxWidth()
            .background(Brand.Surface)
            .padding(horizontal = 8.dp, vertical = 2.dp),
    ) {
        ServerRow(
            name = profile.name,
            subtitle = profile.subtitle,
            countryCode = country,
            isAuto = profile.isAuto,
            isActive = profile.id == activeId,
            isFavorite = profile.isFavorite,
            onClick = onClick,
            onLongClick = onLongClick,
            latencyMs = sample?.takeIf { it.reachable }?.latencyMs,
            offlineReason = sample?.takeUnless { it.reachable }?.reason,
            isLoading = isLoading,
        )
    }
}

/** SECTION label inside a subscription (impVPN "выберите конфиг ниже" etc.). */
@Composable
private fun SubscriptionSectionRowBlock(name: String) {
    Box(
        modifier = Modifier
            .fillMaxWidth()
            .background(Brand.Surface),
    ) {
        SectionLabel(name)
    }
}

/**
 * Visual "cap" at the bottom of an expanded subscription — gives the
 * grouped rows their rounded bottom edge without needing a wrapping Card
 * (the rows would otherwise have to compose all at once to fit inside one).
 */
@Composable
private fun SubscriptionTrailingCap() {
    Box(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(bottomStart = 20.dp, bottomEnd = 20.dp))
            .background(Brand.Surface)
            .height(8.dp),
    )
}

internal fun reorderForDisplay(
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
internal fun SectionLabel(text: String) {
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
