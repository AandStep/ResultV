package com.resultv.android.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.border
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.res.painterResource
import com.resultv.android.ui.components.PageHeader
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.Sync
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Bolt
import androidx.compose.material.icons.outlined.ListAlt
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
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
import androidx.compose.ui.graphics.vector.ImageVector
import android.widget.Toast
import kotlin.math.roundToInt
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.resultv.android.R
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.RvSpace
import com.resultv.android.theme.SegoeUi
import com.resultv.android.ui.components.HomeLook
import com.resultv.android.ui.components.ProfileEditSheet
import com.resultv.android.ui.components.ProfileSortMenu
import com.resultv.android.ui.components.ProfileSortMode
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
    var search by rememberSaveable { mutableStateOf("") }
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

    Column(modifier = Modifier.fillMaxSize()) {
        PageHeader(title = stringResource(R.string.servers_title)) {
            IconButton(
                onClick = { PingRepository.refreshAll(state.profiles) },
                modifier = Modifier.size(36.dp),
            ) {
                Icon(
                    painter = painterResource(R.drawable.ic_ping),
                    contentDescription = stringResource(R.string.ping_refresh_cd),
                    tint = RvColor.whiteA50,
                    modifier = Modifier.size(20.dp),
                )
            }
            ProfileSortMenu(mode = sortMode, onModeChange = { sortMode = it })
        }
        Column(modifier = Modifier.fillMaxSize().padding(horizontal = GroupLook.pagePadding)) {
            if (state.profiles.isEmpty() && subscriptions.subs.isEmpty()) {
                EmptyState(onAddPressed)
                return@Column
            }

            SearchField(
                value = search,
                onValueChange = { search = it },
                modifier = Modifier.padding(bottom = GroupLook.groupGap),
            )

            // Поиск как на ПК (ServersScreen.jsx `matches`): по имени, без учёта
            // регистра. Пока он идёт, группы раскрыты, а пустые скрыты.
            val query = search.trim().lowercase()
            val searching = query.isNotEmpty()
            val matches: (Profile) -> Boolean = { query.isEmpty() || it.name.lowercase().contains(query) }

            // Group profiles by subscription. Unaffiliated ("My proxies") go
            // in their own bucket; SECTION rows stay with their subscription
            // and keep their original order so impVPN's "👇 выберите конфиг
            // ниже" labels land between the right blocks.
            //
            // Memoised on (profiles, query) so a ping-only update doesn't
            // re-walk every profile to rebuild the bucket lists.
            val standalone = remember(state.profiles, query) {
                state.profiles.filter { it.subscriptionId.isBlank() && !it.isSection && matches(it) }
            }
            val sortedStandalone = remember(standalone, sortMode, pings) {
                sortProfiles(standalone, sortMode, pings)
            }
            // Pre-bucket subscription profiles once per (profiles, subs, query)
            // change. Each bucket's rows are rendered as individual LazyColumn
            // items below, so opening Proxies only composes rows actually on
            // screen instead of all N at once.
            val subscriptionBuckets = remember(state.profiles, subscriptions.subs, query) {
                subscriptions.subs.mapNotNull { sub ->
                    val raw = state.profiles.filter { it.subscriptionId == sub.id }
                    val subProfiles = if (!searching) raw
                        else raw.filter { !it.isSection && matches(it) }
                    if (searching && subProfiles.isEmpty()) null
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
            val standaloneCollapsed = !searching && STANDALONE_GROUP_ID in collapsedSubs

            LazyColumn(
                state = listState,
                modifier = Modifier.fillMaxSize(),
            ) {
                subscriptionBuckets.forEachIndexed { idx, (sub, subProfiles) ->
                    val collapsed = !searching && sub.id in collapsedSubs
                    val needsTopGap = idx > 0
                    val ordered = orderedBySub[sub.id].orEmpty()

                    item("sub-${sub.id}-head", contentType = "sub-head") {
                        SubscriptionHeaderBlock(
                            modifier = if (needsTopGap) Modifier.padding(top = GroupLook.groupGap) else Modifier,
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
                                Modifier.padding(top = GroupLook.groupGap) else Modifier,
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
 * Поле поиска — Search мобильного макета (Figma 6864:4933): высота 44,
 * Grey с обводкой белой 10 %, скругление 16, лупа справа.
 */
@Composable
private fun SearchField(
    value: String,
    onValueChange: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val shape = RoundedCornerShape(16.dp)
    val style = TextStyle(
        fontFamily = SegoeUi,
        fontSize = 12.sp,
        lineHeight = 16.8.sp,
        fontWeight = FontWeight.SemiBold,
    )
    BasicTextField(
        value = value,
        onValueChange = onValueChange,
        singleLine = true,
        textStyle = style.copy(color = RvColor.White),
        cursorBrush = SolidColor(RvColor.whiteA50),
        modifier = modifier.fillMaxWidth().height(44.dp),
        decorationBox = { inner ->
            Row(
                modifier = Modifier
                    .fillMaxSize()
                    .clip(shape)
                    .background(RvColor.Grey)
                    .border(1.dp, RvColor.whiteA10, shape)
                    .padding(horizontal = 14.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Box(modifier = Modifier.weight(1f)) {
                    if (value.isEmpty()) {
                        Text(stringResource(R.string.servers_search), style = style, color = RvColor.whiteA20)
                    }
                    inner()
                }
                Icon(
                    painter = painterResource(R.drawable.ic_search),
                    contentDescription = null,
                    tint = RvColor.whiteA50,
                    modifier = Modifier.size(20.dp),
                )
            }
        },
    )
}

/**
 * Метрики шапки группы — сняты с макета Figma (ResultV, узел 6853:4805) один
 * в один, поэтому живут здесь, а не в общих токенах: радиус 20 и отступ 14 —
 * собственные размеры этой карточки, а не шаг общей шкалы.
 */
private object GroupLook {
    val pagePadding = 12.dp
    val groupGap = 8.dp
    val radius = 20.dp
    val padding = 14.dp
    val logoGap = 12.dp
    val titleGap = 4.dp
    val footerGap = 20.dp
    val countGap = 8.dp
    val chip = 30.dp
    val chipIcon = 14.dp
    val chipGap = 4.dp
    val countIcon = 12.dp
    val iconTint = RvColor.whiteA50
    val metaColor = RvColor.whiteA20
    val titleStyle = TextStyle(
        fontFamily = SegoeUi,
        fontWeight = FontWeight.Bold,
        fontSize = 14.sp,
        lineHeight = 19.6.sp,
    )
    val metaStyle = TextStyle(
        fontFamily = SegoeUi,
        fontWeight = FontWeight.SemiBold,
        fontSize = 12.sp,
        lineHeight = 13.2.sp,
    )
}

/**
 * Общий каркас шапки группы — заливка, форма со скруглением и тап по всей
 * карточке, сворачивающий группу. Содержимое у подписки и у «Моих серверов»
 * разное по составу (у подписки лого, срок и трафик, у своих серверов одна
 * строка), поэтому каркас отдаёт его целиком в [content], а не пытается
 * параметризовать каждый кусок.
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
    content: @Composable ColumnScope.() -> Unit,
) {
    val shape = if (collapsed) RoundedCornerShape(GroupLook.radius)
    else RoundedCornerShape(topStart = GroupLook.radius, topEnd = GroupLook.radius)

    Column(
        modifier = modifier
            .fillMaxWidth()
            .clip(shape)
            .background(RvColor.Grey)
            .clickable(
                onClickLabel = stringResource(
                    if (collapsed) R.string.action_expand else R.string.action_collapse,
                ),
                onClick = onToggleCollapsed,
            )
            .padding(GroupLook.padding),
        verticalArrangement = Arrangement.spacedBy(GroupLook.footerGap),
        content = content,
    )
}

/**
 * Шапка подписки по макету: лого, название со сроком действия под ним и
 * кнопки справа; ниже — строка «трафик слева, число серверов справа».
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

    GroupHeaderBlock(
        modifier = modifier,
        collapsed = collapsed,
        onToggleCollapsed = onToggleCollapsed,
    ) {
        // Кнопки по макету прижаты к верху, вровень с лого, а не по центру.
        Row(horizontalArrangement = Arrangement.spacedBy(GroupLook.logoGap)) {
            SubscriptionLogo(usesImpLogo = usesImpLogo)
            Column(
                modifier = Modifier.weight(1f),
                verticalArrangement = Arrangement.spacedBy(GroupLook.titleGap),
            ) {
                GroupTitle(subscription.displayName)
                if (usage.hasExpiry) ExpiryLine(usage)
            }
            Row(horizontalArrangement = Arrangement.spacedBy(GroupLook.chipGap)) {
                CircleActionChip(
                    onClick = onEdit,
                    contentDescription = stringResource(R.string.sub_edit_cd),
                ) { ChipIcon(Icons.Filled.Edit) }
                RefreshChip(
                    refreshing = refreshing,
                    onClick = onRefresh,
                    contentDescription = stringResource(R.string.sub_refresh_cd),
                )
                CircleActionChip(
                    onClick = onDelete,
                    contentDescription = stringResource(R.string.sub_delete_cd),
                ) { ChipIcon(Icons.Filled.Delete) }
            }
        }

        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                text = trafficText(usage),
                style = GroupLook.metaStyle,
                color = GroupLook.metaColor,
                maxLines = 1,
                modifier = Modifier.weight(1f),
            )
            ServerCount(profileCount)
        }
    }
}

/**
 * Шапка группы «Мои серверы» — одна строка: название, число серверов рядом с
 * ним и две кнопки. Правки нет: у группы своих серверов нет ни адреса, ни
 * расписания. Обновление здесь означает не выкачать список заново, а
 * перемерить задержку до своих узлов — так же решено и на ПК
 * (ServersScreen.jsx, группа `myitem`).
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
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            // Счётчик стоит сразу за названием, а не у кнопок: вес у общей
            // пары, а не у названия, иначе свободное место делится между
            // названием и распоркой и кнопки отъезжают от края.
            Row(
                modifier = Modifier.weight(1f),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(GroupLook.countGap),
            ) {
                GroupTitle(
                    text = stringResource(R.string.proxies_standalone_header),
                    modifier = Modifier.weight(1f, fill = false),
                )
                ServerCount(count)
            }
            Row(horizontalArrangement = Arrangement.spacedBy(GroupLook.chipGap)) {
                RefreshChip(
                    refreshing = refreshing,
                    onClick = onRefresh,
                    contentDescription = stringResource(R.string.proxies_standalone_refresh_cd),
                )
                CircleActionChip(
                    onClick = onDelete,
                    contentDescription = stringResource(R.string.proxies_standalone_delete_cd),
                ) { ChipIcon(Icons.Filled.Delete) }
            }
        }
    }
}

@Composable
private fun GroupTitle(text: String, modifier: Modifier = Modifier) {
    Text(
        text = text,
        style = GroupLook.titleStyle,
        maxLines = 1,
        overflow = TextOverflow.Ellipsis,
        modifier = modifier,
    )
}

/**
 * «До 16.08.26 в 21:32» под названием подписки. Цвет предупреждает о сроке:
 * жёлтый за неделю до конца, красный с надписью «Истекла» — после.
 */
@Composable
private fun ExpiryLine(usage: SubscriptionUsage) {
    val expireMs = usage.expireUnix * 1000L
    val text = if (usage.expired) {
        stringResource(R.string.sub_expired)
    } else {
        val (date, time) = remember(usage.expireUnix) {
            val d = Date(expireMs)
            SimpleDateFormat("dd.MM.yy", Locale.getDefault()).format(d) to
                SimpleDateFormat("HH:mm", Locale.getDefault()).format(d)
        }
        stringResource(R.string.sub_valid_until, date, time)
    }
    val daysLeft = TimeUnit.MILLISECONDS.toDays(expireMs - System.currentTimeMillis())
    Text(
        text = text,
        style = GroupLook.metaStyle,
        color = when {
            usage.expired -> RvColor.Errors
            daysLeft <= 7 -> RvColor.Warning
            else -> GroupLook.metaColor
        },
        maxLines = 1,
    )
}

/** «634.5 GB / ∞» или «18.4 / 50 GB»; пусто, если провайдер трафик не отдаёт. */
@Composable
private fun trafficText(usage: SubscriptionUsage): String = when {
    usage.hasQuota -> formatBytesPair(usage.used, usage.total)
    usage.used > 0 -> stringResource(R.string.sub_traffic_used_unlimited, formatBytesShort(usage.used))
    else -> ""
}

/** «39 [dns]» — число серверов группы. */
@Composable
private fun ServerCount(count: Int) {
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RvSpace.xs),
    ) {
        Text(
            text = count.toString(),
            style = GroupLook.metaStyle,
            color = GroupLook.metaColor,
        )
        Icon(
            imageVector = Icons.Filled.Dns,
            contentDescription = null,
            tint = GroupLook.metaColor,
            modifier = Modifier.size(GroupLook.countIcon),
        )
    }
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
    if (isLast) clip(RoundedCornerShape(bottomStart = GroupLook.radius, bottomEnd = GroupLook.radius))
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

/**
 * Round 30dp action chip — refresh/delete in the header use this.
 *
 * Plain Box + clickable instead of IconButton: IconButton enforces a 48dp
 * minimum tap target which made the chips overflow into the title text
 * even when sized to 30dp. The role+contentDescription semantics keep
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
            .size(GroupLook.chip)
            .clip(RoundedCornerShape(50))
            .background(RvColor.LightGray)
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
private fun ChipIcon(imageVector: ImageVector) {
    Icon(
        imageVector = imageVector,
        contentDescription = null,
        tint = GroupLook.iconTint,
        modifier = Modifier.size(GroupLook.chipIcon),
    )
}

/** Кнопка обновления: пока идёт работа, на месте значка крутится индикатор. */
@Composable
private fun RefreshChip(refreshing: Boolean, onClick: () -> Unit, contentDescription: String) {
    CircleActionChip(
        onClick = onClick,
        enabled = !refreshing,
        contentDescription = contentDescription,
    ) {
        if (refreshing) {
            CircularProgressIndicator(
                modifier = Modifier.size(GroupLook.chipIcon),
                strokeWidth = 2.dp,
                color = RvColor.Second,
            )
        } else {
            ChipIcon(Icons.Filled.Sync)
        }
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

/** Объём как на ПК (`formatTraffic`): «312 Мб», «634.5 Гб». */
@Composable
private fun formatBytesShort(bytes: Long): String {
    val gb = 1024.0 * 1024 * 1024
    val mb = 1024.0 * 1024
    return if (bytes >= gb) stringResource(R.string.unit_gb, bytes / gb)
    else stringResource(R.string.unit_mb, (bytes / mb).roundToInt())
}

/** «Использовано / всего» — каждая величина со своей единицей, как `subMeta` на ПК. */
@Composable
private fun formatBytesPair(used: Long, total: Long): String =
    "${formatBytesShort(used)} / ${formatBytesShort(total)}"
