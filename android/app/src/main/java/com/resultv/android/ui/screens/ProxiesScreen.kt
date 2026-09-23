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
import androidx.compose.foundation.lazy.itemsIndexed
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
import android.widget.Toast
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
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.RvIcon
import com.resultv.android.theme.RvRadius
import com.resultv.android.theme.RvSpace
import com.resultv.android.ui.components.HomeLook
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
import com.resultv.android.vpn.AppLog
import com.resultv.android.vpn.SubscriptionRefresher
import com.resultv.android.vpn.SubscriptionRepository
import com.resultv.android.vpn.SubscriptionRouting
import com.resultv.android.vpn.SubscriptionUsage
import com.resultv.android.vpn.serverDisplayName
import kotlinx.coroutines.launch
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.concurrent.TimeUnit

/**
 * Ключ-заглушка для «Мои серверы» в списке свёрнутых групп — той же
 * коллекции, что хранит свёрнутость подписок. Группа самостоятельных
 * профилей не имеет своего id подписки, а заводить для неё отдельное
 * состояние — значит заводить второй механизм ради одной группы. Реальные
 * id подписок — это UUID, так что со строковой константой они никогда не
 * совпадут.
 */
private const val STANDALONE_GROUP_ID = "standalone"

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
    // Удаление своих серверов сносит их все разом, поэтому идёт через
    // подтверждение — как и удаление подписки.
    var pendingDeleteStandalone by remember { mutableStateOf(false) }
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

    Column(modifier = Modifier.fillMaxSize().padding(horizontal = RvSpace.nest1, vertical = RvSpace.nest2)) {
        if (state.profiles.isEmpty() && subscriptions.subs.isEmpty()) {
            EmptyState(onAddPressed)
            return@Column
        }

        Row(
            modifier = Modifier.fillMaxWidth().padding(bottom = RvSpace.nest3),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                text = stringResource(R.string.proxies_count, state.profiles.count { !it.isSection }),
                style = MaterialTheme.typography.labelLarge,
                color = RvColor.whiteA50,
                modifier = Modifier.weight(1f),
            )
            IconButton(onClick = { PingRepository.refreshAll(state.profiles) }) {
                Icon(
                    imageVector = Icons.Outlined.Bolt,
                    contentDescription = stringResource(R.string.ping_refresh_cd),
                    tint = RvColor.whiteA50,
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
            modifier = Modifier.padding(bottom = RvSpace.nest3),
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

        // «Мои серверы» — теперь ПОСЛЕДНЯЯ группа списка (как на ПК,
        // ServersScreen.jsx), а не отдельная плоская пачка перед подписками:
        // самостоятельные профили и подписки равноправны, разница только в
        // источнике, а не в порядке на экране.
        val standaloneCollapsed = STANDALONE_GROUP_ID in collapsedSubs

        LazyColumn(
            state = listState,
            modifier = Modifier.fillMaxSize(),
        ) {
            subscriptionBuckets.forEachIndexed { idx, (sub, subProfiles) ->
                val collapsed = sub.id in collapsedSubs
                val needsTopGap = idx > 0
                val ordered = orderedBySub[sub.id].orEmpty()

                item("sub-${sub.id}-head", contentType = "sub-head") {
                    SubscriptionHeaderBlock(
                        modifier = if (needsTopGap) Modifier.padding(top = RvSpace.nest2) else Modifier,
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
                                val result = runCatching {
                                    SubscriptionRefresher.refreshOne(sub, dataDir)
                                }
                                refreshingSubId = null
                                // A failed manual refresh used to end here silently:
                                // the spinner stopped, nothing else changed, and the
                                // reason was logged only on the auto-refresh path
                                // (SubscriptionRefresher.refreshDue). From the screen
                                // the button looked dead - which is exactly how it was
                                // reported. Log it like the timer does, and say so on
                                // screen.
                                result.onFailure { t ->
                                    AppLog.warning(
                                        R.string.log_sub_refresh_failed,
                                        sub.name,
                                        t.message ?: t.javaClass.simpleName,
                                    )
                                    Toast.makeText(
                                        ctx,
                                        ctx.getString(R.string.sub_refresh_failed),
                                        Toast.LENGTH_LONG,
                                    ).show()
                                }
                            }
                        },
                        onEdit = { editingSubId = sub.id },
                        onDelete = { pendingDeleteSub = sub },
                    )
                }

                if (!collapsed) {
                    itemsIndexed(
                        ordered,
                        key = { _, it -> "sub-${sub.id}-${it.id}" },
                        contentType = { _, it -> if (it.isSection) "sub-sec" else "sub-row" },
                    ) { index, p ->
                        val isLast = index == ordered.lastIndex
                        if (p.isSection) {
                            SubscriptionSectionRowBlock(p.name, isLast = isLast)
                        } else {
                            GroupServerRowBlock(
                                profile = p,
                                activeId = state.activeId,
                                sample = pings[p.id],
                                country = p.country ?: countries[p.id],
                                isLoading = p.id in pingInflight,
                                isLast = isLast,
                                onClick = { ProfileRepository.setActive(p.id) },
                                onLongClick = { editingProfileId = p.id },
                            )
                        }
                    }
                }
            }

            if (sortedStandalone.isNotEmpty()) {
                item("standalone-head", contentType = "standalone-head") {
                    StandaloneHeaderBlock(
                        modifier = if (subscriptionBuckets.isNotEmpty())
                            Modifier.padding(top = RvSpace.nest2) else Modifier,
                        count = sortedStandalone.size,
                        collapsed = standaloneCollapsed,
                        refreshing = sortedStandalone.any { it.id in pingInflight },
                        onRefresh = { PingRepository.refreshAll(sortedStandalone) },
                        onDelete = { pendingDeleteStandalone = true },
                        onToggleCollapsed = {
                            collapsedSubsList = if (STANDALONE_GROUP_ID in collapsedSubs)
                                collapsedSubsList - STANDALONE_GROUP_ID
                            else
                                collapsedSubsList + STANDALONE_GROUP_ID
                        },
                    )
                }

                if (!standaloneCollapsed) {
                    itemsIndexed(
                        sortedStandalone,
                        key = { _, it -> it.id },
                        contentType = { _, _ -> "standalone-row" },
                    ) { index, p ->
                        GroupServerRowBlock(
                            profile = p,
                            activeId = state.activeId,
                            sample = pings[p.id],
                            country = p.country ?: countries[p.id],
                            isLoading = p.id in pingInflight,
                            isLast = index == sortedStandalone.lastIndex,
                            onClick = { ProfileRepository.setActive(p.id) },
                            onLongClick = { editingProfileId = p.id },
                        )
                    }
                }
            }
        }
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
                    color = RvColor.whiteA50,
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    // Профиль маршрутизации этой подписки уходит вместе с ней:
                    // держать его значило бы маршрутизировать по правилам
                    // провайдера, которого больше нет. Кэш правил сносится
                    // внутри forget, до записи конфига.
                    SubscriptionRouting.forget(target.id, ctx.filesDir.absolutePath)
                    SubscriptionRepository.delete(target.id)
                    pendingDeleteSub = null
                }) { Text(stringResource(R.string.action_delete), color = RvColor.Errors) }
            },
            dismissButton = {
                TextButton(onClick = { pendingDeleteSub = null }) { Text(stringResource(R.string.action_cancel)) }
            },
        )
    }

    if (pendingDeleteStandalone) {
        // Удаляются все свои серверы разом, поэтому в тексте стоит их число:
        // без него человек не видит, на что соглашается.
        val own = state.profiles.filter { it.subscriptionId.isBlank() && !it.isSection }
        AlertDialog(
            onDismissRequest = { pendingDeleteStandalone = false },
            title = { Text(stringResource(R.string.proxies_standalone_delete_title)) },
            text = {
                Text(
                    stringResource(R.string.proxies_standalone_delete_message, own.size),
                    color = RvColor.whiteA50,
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    own.forEach { ProfileRepository.remove(it.id) }
                    pendingDeleteStandalone = false
                }) { Text(stringResource(R.string.action_delete), color = RvColor.Errors) }
            },
            dismissButton = {
                TextButton(onClick = { pendingDeleteStandalone = false }) {
                    Text(stringResource(R.string.action_cancel))
                }
            },
        )
    }
}

/**
 * Общий каркас шапки группы — заливка, форма со скруглением, шеврон,
 * ведущая иконка/лого, название и счётчик. У подписки и у «Моих серверов»
 * на ПК это один и тот же вид строки (`subitem`/`myitem` в ServerItem.jsx,
 * различаются только ведущим значком и набором кнопок справа), так что и
 * здесь это один composable, а не две почти одинаковые копии — [leading] и
 * [trailing] параметризуют ровно то немногое, чем группы отличаются.
 *
 * Рендерится как отдельный LazyColumn item, чтобы не тянуть за собой
 * композицию всех строк тела. Форма — полное скругление, когда группа
 * свёрнута (шапка — единственный кусок), и только сверху, когда ниже идут
 * строки тела.
 */
@Composable
private fun GroupHeaderBlock(
    modifier: Modifier = Modifier,
    collapsed: Boolean,
    onToggleCollapsed: () -> Unit,
    leading: @Composable () -> Unit,
    title: String,
    trailing: @Composable () -> Unit = {},
    footer: @Composable () -> Unit = {},
) {
    val shape = if (collapsed) RoundedCornerShape(RvRadius.card)
    else RoundedCornerShape(topStart = RvRadius.card, topEnd = RvRadius.card)

    Column(
        modifier = modifier
            .fillMaxWidth()
            .clip(shape)
            .background(RvColor.Grey)
            .clickable(onClick = onToggleCollapsed)
            .padding(horizontal = RvSpace.nest2, vertical = RvSpace.nest2),
        verticalArrangement = Arrangement.spacedBy(RvSpace.nest3),
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(RvSpace.nest3),
        ) {
            ChevronChip(collapsed = collapsed, onClick = onToggleCollapsed)
            leading()
            Text(
                text = title,
                style = MaterialTheme.typography.titleSmall,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f),
            )
            trailing()
        }

        footer()
    }
}

/** Top portion of a subscription "card" — subscription-specific dressing over [GroupHeaderBlock]. */
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

    GroupHeaderBlock(
        modifier = modifier,
        collapsed = collapsed,
        onToggleCollapsed = onToggleCollapsed,
        leading = { SubscriptionLogo(usesImpLogo = usesImpLogo) },
        title = subscription.displayName,
        trailing = {
            CircleActionChip(
                onClick = onRefresh,
                enabled = !refreshing,
                contentDescription = stringResource(R.string.sub_refresh_cd),
            ) {
                if (refreshing) {
                    CircularProgressIndicator(
                        modifier = Modifier.size(14.dp),
                        strokeWidth = 2.dp,
                        color = RvColor.Second,
                    )
                } else {
                    Icon(
                        imageVector = Icons.Outlined.Refresh,
                        contentDescription = null,
                        tint = RvColor.whiteA50,
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
                    tint = RvColor.whiteA50,
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
                    tint = RvColor.whiteA50,
                    modifier = Modifier.size(16.dp),
                )
            }
        },
        footer = {
            if (usage.hasQuota || usage.hasExpiry || usage.used > 0) {
                UsageInlineStrip(usage)
            }
            SubscriptionFooter(subscription.lastFetchedAt, profileCount)
        },
    )
}

/**
 * Шапка группы «Мои серверы» — тот же [GroupHeaderBlock], что и у подписки,
 * но без логотипа провайдера (сервера ничьи — значок вместо него) и без
 * кнопок обновления/удаления: обновлять у самостоятельных профилей нечего
 * (это не подписка, синку неоткуда взяться), а удаление профиля уже живёт в
 * длинном нажатии по строке — заводить для группы второй путь удаления
 * не нужно. Счётчик показан тем же [SubscriptionFooter], что и у подписки —
 * `lastFetchedAt = 0` прячет в нём метку времени, оставляя только «N серверов».
 */
@Composable
private fun StandaloneHeaderBlock(
    modifier: Modifier = Modifier,
    count: Int,
    collapsed: Boolean,
    refreshing: Boolean,
    onToggleCollapsed: () -> Unit,
    onRefresh: () -> Unit,
    onDelete: () -> Unit,
) {
    GroupHeaderBlock(
        modifier = modifier,
        collapsed = collapsed,
        onToggleCollapsed = onToggleCollapsed,
        leading = {
            Icon(
                imageVector = Icons.Outlined.Bolt,
                contentDescription = null,
                tint = RvColor.whiteA50,
                modifier = Modifier.size(RvIcon.glyph),
            )
        },
        title = stringResource(R.string.proxies_standalone_header),
        // Кнопок две, а не три, как у подписки: правку у группы своих
        // серверов править нечего — ни адреса, ни расписания у неё нет.
        // Обновление здесь означает не выкачать список заново, а перемерить
        // задержку до своих узлов: значок тот же, работа по смыслу та же,
        // так же решено и на ПК (ServersScreen.jsx, группа `myitem`).
        trailing = {
            CircleActionChip(
                onClick = onRefresh,
                enabled = !refreshing,
                contentDescription = stringResource(R.string.proxies_standalone_refresh_cd),
            ) {
                if (refreshing) {
                    CircularProgressIndicator(
                        modifier = Modifier.size(14.dp),
                        strokeWidth = 2.dp,
                        color = RvColor.Second,
                    )
                } else {
                    Icon(
                        imageVector = Icons.Outlined.Refresh,
                        contentDescription = null,
                        tint = RvColor.whiteA50,
                        modifier = Modifier.size(16.dp),
                    )
                }
            }
            CircleActionChip(
                onClick = onDelete,
                contentDescription = stringResource(R.string.proxies_standalone_delete_cd),
            ) {
                Icon(
                    imageVector = Icons.Outlined.DeleteOutline,
                    contentDescription = null,
                    tint = RvColor.whiteA50,
                    modifier = Modifier.size(16.dp),
                )
            }
        },
        footer = { SubscriptionFooter(lastFetchedAt = 0L, profileCount = count) },
    )
}

/**
 * Body row inside a group — subscription or standalone, they share the same
 * look. Заливка/подсветка строки берётся не по умолчанию из [ServerRow]:
 * строка уже лежит на закрашенном сером блоке группы, и заливка ServerRow
 * "для чёрного фона" здесь читалась бы лишним прямоугольником поверх блока
 * (см. комментарий у параметров surface/activeSurface в ServerRow.kt).
 */
@Composable
private fun GroupServerRowBlock(
    profile: Profile,
    activeId: String?,
    sample: PingRepository.Sample?,
    country: String?,
    isLoading: Boolean,
    isLast: Boolean,
    onClick: () -> Unit,
    onLongClick: () -> Unit,
) {
    // Ни бокового, ни вертикального отступа у обёртки нет намеренно. На ПК
    // строки внутри группы идут вплотную и во всю её ширину: отступ по бокам
    // оставлял бы подсветку активной строки не доходящей до краёв блока — по
    // краям светился бы другой фон, — а вертикальный превращался бы в щель
    // между строками и в лишнюю полосу под последней. Воздух вокруг
    // содержимого держит сама ServerRow своим внутренним отступом.
    Box(
        modifier = Modifier
            .fillMaxWidth()
            .groupBottom(isLast)
            .background(RvColor.Grey),
    ) {
        ServerRow(
            name = serverDisplayName(profile.name, country),
            badges = profile.badges,
            countryCode = country,
            isAuto = profile.isAuto,
            isActive = profile.id == activeId,
            isFavorite = profile.isFavorite,
            accent = if (profile.id == activeId) HomeLook.Success else HomeLook.Idle,
            onClick = onClick,
            onLongClick = onLongClick,
            surface = Color.Transparent,
            activeSurface = RvColor.whiteA05,
            latencyMs = sample?.takeIf { it.reachable }?.latencyMs,
            offlineReason = sample?.takeUnless { it.reachable }?.reason,
            isLoading = isLoading,
        )
    }
}

/** SECTION label inside a subscription (impVPN "выберите конфиг ниже" etc.). */
@Composable
private fun SubscriptionSectionRowBlock(name: String, isLast: Boolean) {
    Box(
        modifier = Modifier
            .fillMaxWidth()
            .groupBottom(isLast)
            .background(RvColor.Grey),
    ) {
        SectionLabel(name)
    }
}

/**
 * Скругление низа раскрытой группы. Висит на самой последней строке, а не на
 * отдельной полосе под ней: полоса, чтобы угол в ней не ужимался, должна быть
 * не ниже радиуса, и тогда под последним сервером появляется пустая щель — на
 * ПК её нет, там последняя строка упирается прямо в скруглённый низ карточки.
 * Строка в 64 dp выше двух радиусов, поэтому угол в ней рисуется полностью и
 * совпадает с углом свёрнутой карточки.
 */
private fun Modifier.groupBottom(isLast: Boolean): Modifier =
    if (isLast) clip(RoundedCornerShape(bottomStart = RvRadius.card, bottomEnd = RvRadius.card))
    else this

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
            tint = RvColor.whiteA50,
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
        !usage.hasExpiry -> RvColor.whiteA50
        usage.expired -> RvColor.Errors
        daysLeft <= 7 -> RvColor.Warning
        else -> RvColor.whiteA50
    }
    val ratio = if (usage.hasQuota && usage.total > 0)
        (usage.used.toFloat() / usage.total.toFloat()).coerceIn(0f, 1f)
    else 0f

    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RvSpace.nest3),
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
                        color = RvColor.whiteA50,
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
                    color = if (ratio > 0.9f) RvColor.Errors else RvColor.Second,
                    trackColor = Color.White.copy(alpha = 0.08f),
                )
                Text(
                    text = formatBytesPair(usage.used, usage.total),
                    style = MaterialTheme.typography.labelSmall,
                    color = RvColor.whiteA50,
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
                    color = RvColor.whiteA50,
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
                color = RvColor.whiteA50,
            )
            Spacer(Modifier.width(RvSpace.nest3))
            Box(
                modifier = Modifier
                    .size(width = 1.dp, height = 10.dp)
                    .background(Color.White.copy(alpha = 0.10f)),
            )
            Spacer(Modifier.width(RvSpace.nest3))
        }
        Text(
            text = pluralStringResource(R.plurals.sub_footer_servers, profileCount, profileCount),
            style = MaterialTheme.typography.labelSmall,
            color = RvColor.whiteA50,
        )
        Spacer(Modifier.width(RvSpace.xs))
        Icon(
            imageVector = Icons.Outlined.Dns,
            contentDescription = null,
            tint = RvColor.whiteA50,
            modifier = Modifier.size(12.dp),
        )
    }
}

@Composable
internal fun SectionLabel(text: String) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = RvSpace.nest3, vertical = RvSpace.xs),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RvSpace.nest3),
    ) {
        Icon(
            imageVector = Icons.Outlined.ListAlt,
            contentDescription = null,
            tint = RvColor.Warning,
            modifier = Modifier.size(14.dp),
        )
        Text(
            text = text,
            style = MaterialTheme.typography.labelMedium,
            color = RvColor.whiteA50,
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
            verticalArrangement = Arrangement.spacedBy(RvSpace.nest2),
        ) {
            Text(
                stringResource(R.string.proxies_empty_title),
                style = MaterialTheme.typography.titleLarge,
                fontWeight = FontWeight.SemiBold,
            )
            Text(
                stringResource(R.string.proxies_empty_subtitle),
                style = MaterialTheme.typography.bodyMedium,
                color = RvColor.whiteA50,
            )
            Spacer(Modifier.height(RvSpace.xs))
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
