package com.resultv.android.ui.screens

import android.content.Context
import android.content.pm.ApplicationInfo
import android.content.pm.PackageManager
import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.drawable.Drawable
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.height
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
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.AltRoute
import androidx.compose.material.icons.outlined.Apps
import androidx.compose.material.icons.outlined.Block
import androidx.compose.material.icons.outlined.Public
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.resultv.android.R
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.rvBorder
import com.resultv.android.theme.RvSpace
import com.resultv.android.ui.components.TagField
import com.resultv.android.vpn.AppInventory
import com.resultv.android.vpn.AppRoutingRepository
import com.resultv.android.vpn.AppTunnelMembership
import com.resultv.android.vpn.RoutingMode
import com.resultv.android.vpn.RoutingRulesRepository
import com.resultv.android.vpn.RuleAction
import com.resultv.android.vpn.SmartAppMembership
import com.resultv.android.vpn.SmartListRepository
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/**
 * Страница «Умные правила» — мобильный макет (Figma 6865:4881 Smart,
 * 6869:4923 Global). Плитки режимов, в Global под ними — профили
 * маршрутизации, дальше две секции-карточки: сайты и приложения.
 *
 * Переключателя «В ВПН / Запретить» в макете нет: у каждой секции один
 * список, и какой — решает режим. Smart правит «через ВПН», Global —
 * «напрямую». Список блокировки из интерфейса ушёл, но то, что в нём уже
 * было, движок по-прежнему применяет.
 */
@Composable
fun RulesScreen(onOpenRoutingProfiles: () -> Unit = {}) {
    val rules by RoutingRulesRepository.state.collectAsStateWithLifecycle()
    val focusManager = LocalFocusManager.current
    val keyboard = LocalSoftwareKeyboardController.current
    var domainInput by remember { mutableStateOf("") }
    var confirmClearSites by remember { mutableStateOf(false) }
    val smart = rules.mode == RoutingMode.Smart
    val action = primaryAction(rules.mode)

    Column(
        modifier = Modifier
            .fillMaxWidth()
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

        RulesSection(
            title = stringResource(if (smart) R.string.rules_sites_vpn else R.string.rules_sites_direct),
            description = stringResource(
                if (smart) R.string.rules_sites_vpn_desc else R.string.rules_sites_direct_desc,
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

        PerAppRoutingSection(mode = rules.mode)
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
            Text(stringResource(R.string.rules_profiles_desc), style = SectionDesc, color = RvColor.whiteA50)
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
            .clip(RoundedCornerShape(20.dp))
            .background(RvColor.Grey)
            .padding(14.dp),
        verticalArrangement = Arrangement.spacedBy(RvSpace.nest2),
    ) {
        Row {
            Column(
                modifier = Modifier.weight(1f).padding(end = RvSpace.nest2),
                verticalArrangement = Arrangement.spacedBy(RvSpace.xs),
            ) {
                Text(title, style = SectionTitle, color = RvColor.White)
                Text(description, style = SectionDesc, color = RvColor.whiteA50)
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
        content()
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
private fun PerAppRoutingSection(mode: RoutingMode) {
    // package_name rules can't match without ConnectivityManager
    // .getConnectionOwnerUid — see BoxPlatform.findConnectionOwner.
    val canResolveOwner = android.os.Build.VERSION.SDK_INT >= android.os.Build.VERSION_CODES.Q
    val tab = primaryAction(mode)
    val smart = mode == RoutingMode.Smart
    var confirmClear by remember { mutableStateOf(false) }

    RulesSection(
        title = stringResource(if (smart) R.string.rules_apps_vpn else R.string.rules_apps_direct),
        description = stringResource(if (smart) R.string.rules_apps_vpn_desc else R.string.rules_apps_direct_desc),
        onClear = { confirmClear = true },
    ) {
        // «Через ВПН» держится на package_name-правилах, а их ниже API 29 не
        // сопоставить — объяснение вместо списка.
        if (tab != RuleAction.OutOfVpn && !canResolveOwner) {
            Text(
                stringResource(R.string.rules_apps_needs_q),
                style = SectionDesc,
                color = RvColor.whiteA50,
            )
        } else {
            AppList(mode = mode, tab = tab)
        }
    }

    if (confirmClear) {
        ConfirmClear(
            message = stringResource(R.string.rules_confirm_clear_apps),
            onConfirm = { AppRoutingRepository.clearList(tab) },
            onDismiss = { confirmClear = false },
        )
    }
}

@Composable
private fun AppList(mode: RoutingMode, tab: RuleAction) {

    val ctx = LocalContext.current
    val appRules by AppRoutingRepository.state.collectAsStateWithLifecycle()
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
    LaunchedEffect(mode, apps, smartSnapshot.count, smartSnapshot.ready) {
        autoIn = if (mode != RoutingMode.Smart || apps.isEmpty()) {
            emptySet()
        } else withContext(Dispatchers.IO) {
            val matched = SmartAppMembership.matchedPackages(
                ctx.filesDir.absolutePath, apps.map { it.packageName },
            )
            val browsers = AppInventory.browserPackages(ctx)
            matched + browsers
        }
    }

    val filtered = remember(apps, query) {
        if (query.isBlank()) apps
        else apps.filter {
            it.label.contains(query, ignoreCase = true) ||
                it.packageName.contains(query, ignoreCase = true)
        }
    }

    Column(verticalArrangement = Arrangement.spacedBy(RvSpace.nest2)) {
        RvSearchField(
            value = query,
            onValueChange = { query = it },
            placeholder = stringResource(R.string.rules_app_search),
        )

        val selectedCount = when (tab) {
            RuleAction.OutOfVpn -> appRules.outOfVpn.size
            RuleAction.IntoVpn -> appRules.intoVpn.size
            RuleAction.Block -> appRules.blocked.size
        }
        Text(
            stringResource(R.string.rules_app_selected_count, selectedCount),
            style = SectionDesc.copy(fontSize = 12.sp, lineHeight = 13.2.sp),
            color = RvColor.whiteA50,
        )

        if (loading) {
            Box(modifier = Modifier.fillMaxWidth().padding(RvSpace.nest1), contentAlignment = Alignment.Center) {
                CircularProgressIndicator()
            }
        } else {
            // Список фиксированной высоты со своей прокруткой и затуханием
            // снизу — страница из-за сотни приложений не растёт.
            Box(modifier = Modifier.fillMaxWidth().height(280.dp)) {
            LazyColumn(modifier = Modifier.fillMaxSize()) {
                items(filtered, key = { it.packageName }) { app ->
                    val smartMembershipTab = mode == RoutingMode.Smart && tab == RuleAction.IntoVpn
                    if (smartMembershipTab) {
                        val auto = app.packageName in autoIn
                        val inVpn = AppTunnelMembership.isInSmart(
                            app.packageName, autoIn, emptySet(), appRules
                        )
                        val blocked = app.packageName in appRules.blocked
                        AppRow(
                            app = app,
                            checked = inVpn,
                            blockedElsewhere = blocked,
                            autoBadge = auto && inVpn,
                            onToggle = {
                                AppRoutingRepository.setSmartMembership(
                                    app.packageName, wantIn = !inVpn, isAuto = auto
                                )
                            },
                        )
                    } else {
                        val effective = appRules.actionOf(app.packageName, mode)
                        AppRow(
                            app = app,
                            checked = effective == tab,
                            // The user's ask: an app blocked in the Block tab shows
                            // as blocked in the routing tab too. Tapping it there
                            // moves it silently — withAction clears the block.
                            blockedElsewhere = effective == RuleAction.Block && tab != RuleAction.Block,
                            autoBadge = false,
                            onToggle = {
                                if (effective == tab) AppRoutingRepository.clearAction(app.packageName, tab)
                                else AppRoutingRepository.setAction(app.packageName, tab)
                            },
                        )
                    }
                }
            }
                Box(
                    modifier = Modifier
                        .align(Alignment.BottomCenter)
                        .fillMaxWidth()
                        .height(40.dp)
                        .background(Brush.verticalGradient(listOf(Color.Transparent, RvColor.Grey))),
                )
            }
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
) {
    Row(
        // Весь ряд — одна цель нажатия: попасть в 20-dp квадрат флажка пальцем
        // трудно, а сама строка выглядит нажимаемой. Роль Checkbox отдаёт ряду
        // семантику флажка — TalkBack объявляет одну цель, а не две.
        modifier = Modifier
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
                Text("✓ " + stringResource(R.string.rules_badge_auto_vpn), style = PackageStyle, color = RvColor.Second)
            }
            if (blockedElsewhere) {
                Text("⛔ " + stringResource(R.string.rules_badge_blocked), style = PackageStyle, color = RvColor.whiteA50)
            }
            Text(app.packageName, style = PackageStyle, color = RvColor.whiteA20, maxLines = 1, overflow = TextOverflow.Ellipsis)
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
                Text(stringResource(R.string.rules_app_system_tag), style = PackageStyle, color = RvColor.whiteA50)
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
            Icon(Icons.Filled.Check, contentDescription = null, tint = RvColor.Black, modifier = Modifier.size(14.dp))
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
