package com.resultv.android.ui.screens

import android.content.Context
import android.content.pm.ApplicationInfo
import android.content.pm.PackageManager
import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.drawable.Drawable
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.offset
import androidx.compose.material.icons.automirrored.outlined.OpenInNew
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Public
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.IconButton
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.rememberVectorPainter
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.unit.sp
import com.resultv.android.theme.SegoeUi
import com.resultv.android.ui.components.RvBigButton
import com.resultv.android.ui.components.RvSearchField
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.AltRoute
import androidx.compose.material.icons.outlined.Apps
import androidx.compose.material.icons.outlined.Block
import androidx.compose.material.icons.outlined.Public
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.input.nestedscroll.NestedScrollConnection
import androidx.compose.ui.input.nestedscroll.NestedScrollSource
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.resultv.android.R
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.rvBorder
import com.resultv.android.theme.RvSpace
import com.resultv.android.ui.components.TagField
import com.resultv.android.vpn.AppInventory
import com.resultv.android.vpn.AppRoutingRepository
import com.resultv.android.vpn.RoutingMode
import com.resultv.android.vpn.RoutingRulesRepository
import com.resultv.android.vpn.RuleAction
import com.resultv.android.vpn.SmartAppMembership
import com.resultv.android.vpn.SmartListRepository
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

private val RulesSectionPadding = 14.dp
private val RulesSectionRadius = 20.dp
private val RulesListEndPadding = 12.dp

/**
 * Страница «Умные правила» — мобильный макет (Figma 6865:4881 Smart,
 * 6869:4923 Global). Плитки режимов, в Global под ними — профили
 * маршрутизации, дальше две секции-карточки: сайты и приложения.
 *
 * Оба направления редактируются независимо для каждого режима.
 */
@Composable
fun RulesScreen(viewportHeight: Dp, onOpenRoutingProfiles: () -> Unit = {}) {
    val rules by RoutingRulesRepository.state.collectAsStateWithLifecycle()
    val focusManager = LocalFocusManager.current
    val keyboard = LocalSoftwareKeyboardController.current
    val smart = rules.mode == RoutingMode.Smart
    var action by remember(rules.mode) { mutableStateOf(primaryAction(rules.mode)) }
    var domainInput by remember(rules.mode, action) { mutableStateOf("") }
    var confirmClearSites by remember(rules.mode, action) { mutableStateOf(false) }
    val intoVpn = action == RuleAction.IntoVpn
    val pageScroll = rememberScrollState()
    val revealAppList = remember(pageScroll) {
        object : NestedScrollConnection {
            override fun onPreScroll(available: Offset, source: NestedScrollSource): Offset {
                // Reveal the full list before its rows start scrolling.
                if (available.y >= 0f || !pageScroll.canScrollForward) return Offset.Zero
                return Offset(0f, -pageScroll.dispatchRawDelta(-available.y))
            }
        }
    }

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .verticalScroll(pageScroll)
            .nestedScroll(revealAppList)
            .pointerInput(Unit) {
                detectTapGestures(onTap = {
                    keyboard?.hide()
                    focusManager.clearFocus()
                })
            },
        verticalArrangement = Arrangement.spacedBy(RvSpace.nest3),
    ) {
        Row(horizontalArrangement = Arrangement.spacedBy(RvSpace.nest3)) {
            RvBigButton(
                icon = painterResource(R.drawable.ic_smart),
                label = stringResource(R.string.rules_mode_smart),
                selected = smart,
                onClick = {
                    RoutingRulesRepository.setMode(RoutingMode.Smart)
                    // Список блокировок качается сразу при выборе Smart —
                    // иначе первое подключение ждало бы загрузку.
                    SmartListRepository.refreshAsync()
                },
                modifier = Modifier.weight(1f),
            )
            RvBigButton(
                icon = rememberVectorPainter(Icons.Filled.Public),
                label = stringResource(R.string.rules_mode_global),
                selected = !smart,
                onClick = { RoutingRulesRepository.setMode(RoutingMode.Global) },
                modifier = Modifier.weight(1f),
            )
        }

        // Профиль маршрутизации действует только в Global: в Smart клиент
        // решает сам (так же на ПК, SmartRulesPage.jsx).
        if (!smart) {
            ProfilesCard(onClick = onOpenRoutingProfiles)
        }

        Row(horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2)) {
            FilterChip(
                selected = intoVpn,
                onClick = { action = RuleAction.IntoVpn },
                label = { Text(stringResource(R.string.routing_direction_vpn)) },
            )
            FilterChip(
                selected = !intoVpn,
                onClick = { action = RuleAction.OutOfVpn },
                label = { Text(stringResource(R.string.routing_direction_direct)) },
            )
        }

        RulesSection(
            title = stringResource(if (intoVpn) R.string.rules_sites_vpn else R.string.rules_sites_direct),
            description = stringResource(
                if (intoVpn) R.string.routing_sites_vpn_desc else R.string.rules_sites_direct_desc,
            ),
            onClear = { confirmClearSites = true },
        ) {
            val active = rules.domains.listFor(action)
            // Один ввод и для набора, и для уже добавленного: пробел и Enter
            // добавляют, подробности в TagField.
            TagField(
                values = active,
                draft = domainInput,
                onDraftChange = { domainInput = it },
                onCommit = { RoutingRulesRepository.addDomain(it, action) },
                onRemove = { RoutingRulesRepository.removeDomain(it, action) },
                placeholder = stringResource(R.string.rules_domain_placeholder),
            )
        }

        PerAppRoutingSection(
            mode = rules.mode,
            tab = action,
            listHeight = viewportHeight,
        )
    }

    if (confirmClearSites) {
        ConfirmClear(
            message = stringResource(R.string.rules_confirm_clear_sites),
            onConfirm = { RoutingRulesRepository.clearDomains(action) },
            onDismiss = { confirmClearSites = false },
        )
    }
}

/** Список, который правит страница в этом режиме: Smart — «через ВПН», Global — «напрямую». */
private fun primaryAction(mode: RoutingMode): RuleAction = when (mode) {
    RoutingMode.Smart -> RuleAction.IntoVpn
    RoutingMode.Global -> RuleAction.OutOfVpn
}

/**
 * Карточка «Профили маршрутизации» (SettingsItem, Figma 6869:5072): плитка
 * 40, заголовок 14 Bold, описание 10 Semibold, значок перехода справа.
 */
@Composable
private fun ProfilesCard(onClick: () -> Unit) {
    val shape = RoundedCornerShape(20.dp)
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(shape)
            .background(RvColor.Grey)
            .rvBorder(shape)
            .clickable(role = Role.Button, onClick = onClick)
            .padding(15.dp),
        horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2),
    ) {
        Box(
            modifier = Modifier
                .size(40.dp)
                .clip(RoundedCornerShape(10.dp))
                .background(RvColor.mainA10),
            contentAlignment = Alignment.Center,
        ) {
            Icon(
                imageVector = Icons.Outlined.AltRoute,
                contentDescription = null,
                tint = RvColor.Main,
                modifier = Modifier.size(22.dp),
            )
        }
        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(RvSpace.xs),
        ) {
            Text(stringResource(R.string.routing_profiles_row), style = SectionTitle, color = RvColor.White)
            Text(stringResource(R.string.rules_profiles_desc), style = SectionDesc, color = RvColor.description)
        }
        Icon(
            imageVector = Icons.AutoMirrored.Outlined.OpenInNew,
            contentDescription = null,
            tint = RvColor.whiteA50,
            modifier = Modifier.size(20.dp),
        )
    }
}

private val SectionTitle = TextStyle(
    fontFamily = SegoeUi,
    fontSize = 14.sp,
    lineHeight = 19.6.sp,
    fontWeight = FontWeight.Bold,
)

private val SectionDesc = TextStyle(
    fontFamily = SegoeUi,
    fontSize = 10.sp,
    lineHeight = 11.sp,
    fontWeight = FontWeight.SemiBold,
)

/**
 * Секция-карточка макета (Section, Figma 6869:4935): Grey, скругление 20,
 * поле 14; сверху заголовок с описанием и корзина, ниже содержимое через 12.
 */
@Composable
private fun RulesSection(
    title: String,
    description: String,
    onClear: () -> Unit,
    content: @Composable ColumnScope.() -> Unit,
) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(RulesSectionRadius))
            .background(RvColor.Grey)
            .padding(RulesSectionPadding),
        verticalArrangement = Arrangement.spacedBy(RvSpace.nest2),
    ) {
        RulesSectionHeader(title, description, onClear)
        content()
    }
}

@Composable
private fun RulesSectionHeader(title: String, description: String, onClear: () -> Unit) {
    Row {
        Column(
            modifier = Modifier.weight(1f).padding(end = RvSpace.nest2),
            verticalArrangement = Arrangement.spacedBy(RvSpace.xs),
        ) {
            Text(title, style = SectionTitle, color = RvColor.White)
            Text(description, style = SectionDesc, color = RvColor.description)
        }
        // Глиф 20, цель пальца 36: лишнее уходит сдвигом за край глифа.
        IconButton(
            onClick = onClear,
            modifier = Modifier.size(36.dp).offset(x = 8.dp, y = (-8).dp),
        ) {
            Icon(
                imageVector = Icons.Filled.Delete,
                contentDescription = stringResource(R.string.rules_clear_list),
                tint = RvColor.whiteA50,
                modifier = Modifier.size(20.dp),
            )
        }
    }
}

@Composable
private fun ConfirmClear(message: String, onConfirm: () -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        text = { Text(message) },
        confirmButton = {
            TextButton(onClick = { onConfirm(); onDismiss() }) {
                Text(stringResource(R.string.action_delete), color = RvColor.Errors)
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        },
    )
}

// ────────────────────────── Per-app routing section ─────────────────────────

private data class InstalledApp(
    val packageName: String,
    val label: String,
    val icon: androidx.compose.ui.graphics.ImageBitmap?,
    val isSystem: Boolean,
)

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun PerAppRoutingSection(mode: RoutingMode, tab: RuleAction, listHeight: Dp) {
    // package_name rules can't match without ConnectivityManager
    // .getConnectionOwnerUid — see BoxPlatform.findConnectionOwner.
    val canResolveOwner = android.os.Build.VERSION.SDK_INT >= android.os.Build.VERSION_CODES.Q
    val intoVpn = tab == RuleAction.IntoVpn
    var confirmClear by remember(mode, tab) { mutableStateOf(false) }
    val title = stringResource(if (intoVpn) R.string.rules_apps_vpn else R.string.rules_apps_direct)
    val description = stringResource(if (intoVpn) R.string.rules_apps_vpn_desc else R.string.rules_apps_direct_desc)
    val onClear = { confirmClear = true }

    // Package rules need connection-owner lookup, which was added in Android 10.
    if (tab != RuleAction.OutOfVpn && !canResolveOwner) {
        RulesSection(title, description, onClear) {
            Text(
                stringResource(R.string.rules_apps_needs_q),
                style = SectionDesc,
                color = RvColor.description,
            )
        }
    } else {
        AppList(mode = mode, tab = tab, listHeight = listHeight) {
            RulesSectionHeader(title, description, onClear)
        }
    }

    if (confirmClear) {
        ConfirmClear(
            message = stringResource(R.string.rules_confirm_clear_apps),
            onConfirm = { AppRoutingRepository.clearList(mode, tab) },
            onDismiss = { confirmClear = false },
        )
    }
}

@Composable
private fun AppList(mode: RoutingMode, tab: RuleAction, listHeight: Dp, cardHeader: @Composable () -> Unit) {
    val modeRules by AppRoutingRepository.state.collectAsStateWithLifecycle()
    val rules = modeRules.forMode(mode)
    val selected = when (tab) {
        RuleAction.OutOfVpn -> rules.outOfVpn
        RuleAction.IntoVpn -> rules.intoVpn
        RuleAction.Block -> rules.blocked
    }
    InstalledAppPicker(
        selectedPackages = selected,
        listModifier = Modifier.heightIn(max = listHeight),
        cardHeader = cardHeader,
        onToggleAutomatic = if (mode == RoutingMode.Smart && tab == RuleAction.IntoVpn) {
            AppRoutingRepository::setAutomaticSmartMembership
        } else null,
        directPackages = rules.outOfVpn,
        blockedPackages = rules.blocked,
        onToggle = { packageName, checked ->
            if (checked) AppRoutingRepository.setAction(mode, packageName, tab)
            else AppRoutingRepository.clearAction(mode, packageName, tab)
        },
    )
}

@Composable
internal fun InstalledAppPicker(
    selectedPackages: Set<String>,
    onToggle: (String, Boolean) -> Unit,
    listModifier: Modifier = Modifier.height(280.dp),
    cardHeader: (@Composable () -> Unit)? = null,
    onToggleAutomatic: ((String, Boolean) -> Unit)? = null,
    showCount: Boolean = true,
    directPackages: Set<String> = emptySet(),
    blockedPackages: Set<String> = emptySet(),
) {
    val ctx = LocalContext.current
    val showAutomatic = onToggleAutomatic != null
    var apps by remember { mutableStateOf<List<InstalledApp>>(emptyList()) }
    var loading by remember { mutableStateOf(false) }
    var query by remember { mutableStateOf("") }

    // The catalogue is the same for every tab, so load it once the section is
    // on screen rather than per tab.
    LaunchedEffect(Unit) {
        if (apps.isEmpty()) {
            loading = true
            apps = withContext(Dispatchers.IO) { loadInstalledApps(ctx) }
            loading = false
        }
    }

    // Apps the engine puts in the tunnel on its own in Smart: matched against
    // the blocklist brands, plus browsers. Recomputed when the list refreshes
    // so the badges follow a freshly downloaded blocklist.
    val smartSnapshot by SmartListRepository.state.collectAsStateWithLifecycle()
    var autoIn by remember { mutableStateOf<Set<String>>(emptySet()) }
    // Для какого каталога и режима autoIn уже посчитан. Отмеченные стоят
    // наверху, а авто-отмеченные известны только после этого подсчёта: покажи
    // список раньше — он откроется по алфавиту и тут же пересортируется, и
    // отмеченные уедут за верхний край. Поэтому до подсчёта — индикатор загрузки.
    var autoInFor by remember { mutableStateOf<Pair<List<InstalledApp>, Boolean>?>(null) }
    LaunchedEffect(showAutomatic, apps, smartSnapshot.count, smartSnapshot.ready) {
        autoIn = if (!showAutomatic || apps.isEmpty()) {
            emptySet()
        } else withContext(Dispatchers.IO) {
            val matched = SmartAppMembership.matchedPackages(
                ctx.filesDir.absolutePath, apps.map { it.packageName },
            )
            val browsers = AppInventory.browserPackages(ctx)
            matched + browsers
        }
        autoInFor = apps to showAutomatic
    }
    val orderReady = autoInFor?.let { (forApps, forMode) -> forApps === apps && forMode == showAutomatic } == true

    val filtered = remember(apps, query) {
        if (query.isBlank()) apps
        else apps.filter {
            it.label.contains(query, ignoreCase = true) ||
                it.packageName.contains(query, ignoreCase = true)
        }
    }

    val checkedPackages = remember(selectedPackages, showAutomatic, autoIn, directPackages, blockedPackages) {
        if (showAutomatic) (selectedPackages + autoIn) - directPackages - blockedPackages else selectedPackages
    }
    // Отмеченные всегда наверху. Сортировка устойчивая, так что внутри обеих
    // частей остаётся порядок каталога: пользовательские, затем системные, по имени.
    // Не rememberSaveable: каталог грузится заново при каждом показе, и список
    // открывается с начала — с отмеченных.
    val listState = remember { LazyListState() }
    val atTop by remember {
        derivedStateOf { listState.firstVisibleItemIndex == 0 && listState.firstVisibleItemScrollOffset == 0 }
    }
    val ordered = remember(filtered, checkedPackages) {
        // Отметка, сделанная в начале списка, поднимает строку на место выше
        // первой видимой, а ленивый список держит позицию за первой видимой —
        // и только что отмеченное ушло бы за верхний край. requestScrollToItem
        // не прокручивает, а говорит перемеру держаться за индекс, а не за
        // ключ. На открытии списка он ничего не делает: порядок к первому кадру
        // уже окончательный (orderReady).
        if (atTop) listState.requestScrollToItem(0)
        filtered.sortedByDescending { it.packageName in checkedPackages }
    }

    val waiting = loading || !orderReady
    val rowsVisible = !waiting && ordered.isNotEmpty()
    val headerModifier = if (cardHeader != null) {
        val shape = if (rowsVisible) RoundedCornerShape(topStart = RulesSectionRadius, topEnd = RulesSectionRadius)
        else RoundedCornerShape(RulesSectionRadius)
        Modifier.fillMaxWidth().clip(shape).background(RvColor.Grey).padding(RulesSectionPadding)
    } else Modifier

    Column(verticalArrangement = Arrangement.spacedBy(if (cardHeader == null) RvSpace.nest2 else 0.dp)) {
        Column(modifier = headerModifier, verticalArrangement = Arrangement.spacedBy(RvSpace.nest2)) {
            cardHeader?.invoke()
            RvSearchField(
                value = query,
                onValueChange = { query = it },
                placeholder = stringResource(R.string.rules_app_search),
            )

            if (showCount) {
                Text(
                    stringResource(R.string.rules_app_selected_count, selectedPackages.size),
                    style = SectionDesc.copy(fontSize = 12.sp, lineHeight = 13.2.sp),
                    color = RvColor.description,
                )
            }
            if (waiting) {
                Box(modifier = Modifier.fillMaxWidth().padding(RvSpace.nest1), contentAlignment = Alignment.Center) {
                    CircularProgressIndicator()
                }
            }
        }

        if (!waiting && (rowsVisible || cardHeader == null)) {
            Box(modifier = Modifier.fillMaxWidth().then(listModifier)) {
                LazyColumn(
                    state = listState,
                    modifier = Modifier.fillMaxWidth(),
                    contentPadding = PaddingValues(bottom = if (cardHeader != null) RulesListEndPadding else 0.dp),
                ) {
                    itemsIndexed(ordered, key = { _, app -> app.packageName }) { index, app ->
                        val checked = app.packageName in checkedPackages
                        val automaticToggle = onToggleAutomatic?.takeIf { app.packageName in autoIn }
                        val rowModifier = if (cardHeader != null) {
                            val last = index == ordered.lastIndex
                            val shape = if (last) {
                                Modifier.clip(RoundedCornerShape(bottomStart = RulesSectionRadius, bottomEnd = RulesSectionRadius))
                            } else Modifier
                            shape.background(RvColor.Grey)
                                .padding(
                                    start = RulesSectionPadding,
                                    end = RulesSectionPadding,
                                    bottom = if (last) RulesSectionPadding else 0.dp,
                                )
                        } else Modifier
                        AppRow(
                            app = app,
                            checked = checked,
                            blockedElsewhere = app.packageName in blockedPackages,
                            autoBadge = automaticToggle != null && checked,
                            onToggle = { (automaticToggle ?: onToggle)(app.packageName, !checked) },
                            modifier = Modifier.animateItem().then(rowModifier),
                        )
                    }
                }
                if (listState.canScrollForward) {
                    Box(
                        modifier = Modifier
                            .align(Alignment.BottomCenter)
                            .fillMaxWidth()
                            .height(40.dp)
                            .background(Brush.verticalGradient(listOf(Color.Transparent, RvColor.Grey))),
                    )
                }
            }
        } else if (cardHeader != null) {
            Spacer(Modifier.height(RulesListEndPadding))
        }
    }
}

/**
 * Строка приложения — AppItem мобильного макета (Figma 6869:4959): флажок
 * 20, значок 30, имя 14 Bold и пакет 10 Semibold, тег «система» справа.
 */
@Composable
private fun AppRow(
    app: InstalledApp,
    checked: Boolean,
    blockedElsewhere: Boolean,
    autoBadge: Boolean,
    onToggle: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Row(
        // Весь ряд — одна цель нажатия: попасть в 20-dp квадрат флажка пальцем
        // трудно, а сама строка выглядит нажимаемой. Роль Checkbox отдаёт ряду
        // семантику флажка — TalkBack объявляет одну цель, а не две.
        modifier = modifier
            .fillMaxWidth()
            .toggleable(value = checked, role = Role.Checkbox, onValueChange = { onToggle() })
            .padding(vertical = RvSpace.nest3),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2),
    ) {
        RulesCheckbox(checked)
        if (app.icon != null) {
            Image(
                bitmap = app.icon,
                contentDescription = null,
                modifier = Modifier.size(30.dp).clip(RoundedCornerShape(8.dp)),
            )
        } else {
            Box(
                modifier = Modifier
                    .size(30.dp)
                    .clip(RoundedCornerShape(8.dp))
                    .background(RvColor.LightGray),
                contentAlignment = Alignment.Center,
            ) {
                Icon(Icons.Outlined.Public, contentDescription = null, tint = RvColor.whiteA50, modifier = Modifier.size(18.dp))
            }
        }
        Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(app.label, style = SectionTitle, color = RvColor.White, maxLines = 1, overflow = TextOverflow.Ellipsis)
            if (autoBadge) {
                Text("✓ " + stringResource(R.string.rules_badge_auto_vpn), style = PackageStyle, color = RvColor.successText)
            }
            if (blockedElsewhere) {
                Text("⛔ " + stringResource(R.string.rules_badge_blocked), style = PackageStyle, color = RvColor.description)
            }
            Text(app.packageName, style = PackageStyle, color = if (RvColor.isDark) RvColor.whiteA20 else RvColor.description, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
        if (app.isSystem) {
            Box(
                modifier = Modifier
                    .height(20.dp)
                    .clip(RoundedCornerShape(percent = 50))
                    .background(RvColor.LightGray)
                    .padding(horizontal = 6.dp),
                contentAlignment = Alignment.Center,
            ) {
                Text(stringResource(R.string.rules_app_system_tag), style = PackageStyle, color = RvColor.description)
            }
        }
    }
}

private val PackageStyle = TextStyle(
    fontFamily = SegoeUi,
    fontSize = 10.sp,
    lineHeight = 13.sp,
    fontWeight = FontWeight.SemiBold,
)

/** Флажок макета: 20, скругление 4; включённый — Main с тёмной галочкой. */
@Composable
private fun RulesCheckbox(checked: Boolean) {
    val shape = RoundedCornerShape(4.dp)
    Box(
        modifier = Modifier
            .size(20.dp)
            .clip(shape)
            .background(if (checked) RvColor.Main else Color.Transparent)
            .rvBorder(if (checked) RvColor.Main else RvColor.whiteA50, shape, width = 1.5.dp),
        contentAlignment = Alignment.Center,
    ) {
        if (checked) {
            Icon(Icons.Filled.Check, contentDescription = null, tint = RvColor.OnAccentGlyph, modifier = Modifier.size(14.dp))
        }
    }
}

private fun loadInstalledApps(ctx: Context): List<InstalledApp> {
    val pm = ctx.packageManager
    val apps = pm.getInstalledApplications(PackageManager.GET_META_DATA)
    return apps.mapNotNull { info ->
        if (info.packageName == ctx.packageName) return@mapNotNull null
        val isSystem = (info.flags and ApplicationInfo.FLAG_SYSTEM) != 0
        val label = pm.getApplicationLabel(info).toString()
        val icon = try { pm.getApplicationIcon(info) } catch (_: Throwable) { null }
        InstalledApp(
            packageName = info.packageName,
            label = label,
            icon = icon?.let { drawableToBitmap(it).asImageBitmap() },
            isSystem = isSystem,
        )
    }.sortedWith(compareBy({ it.isSystem }, { it.label.lowercase() }))
}

private fun drawableToBitmap(d: Drawable): Bitmap {
    val w = d.intrinsicWidth.coerceAtLeast(1)
    val h = d.intrinsicHeight.coerceAtLeast(1)
    val bmp = Bitmap.createBitmap(w, h, Bitmap.Config.ARGB_8888)
    val c = Canvas(bmp)
    d.setBounds(0, 0, w, h)
    d.draw(c)
    return bmp
}
