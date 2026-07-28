package com.resultv.android.ui.screens

import android.content.Context
import android.content.pm.ApplicationInfo
import android.content.pm.PackageManager
import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.drawable.Drawable
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.AltRoute
import androidx.compose.material.icons.outlined.Apps
import androidx.compose.material.icons.outlined.Block
import androidx.compose.material.icons.outlined.CallSplit
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material.icons.outlined.Dns
import androidx.compose.material.icons.outlined.Hub
import androidx.compose.material.icons.outlined.Language
import androidx.compose.material.icons.outlined.Public
import androidx.compose.material.icons.outlined.VpnLock
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.resultv.android.R
import com.resultv.android.theme.Brand
import com.resultv.android.ui.components.SettingIcon
import com.resultv.android.vpn.AppInventory
import com.resultv.android.vpn.AppRoutingRepository
import com.resultv.android.vpn.AppTunnelMembership
import com.resultv.android.vpn.RoutingMode
import com.resultv.android.vpn.RoutingRulesRepository
import com.resultv.android.vpn.RuleAction
import com.resultv.android.vpn.SmartAppMembership
import com.resultv.android.vpn.SmartListRepository
import com.resultv.android.vpn.domainPatternShadows
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

private val QuickDomains = listOf("*.ru", "*.рф", "*.su", "*.by", "*.kz")

@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
fun RulesScreen() {
    val rules by RoutingRulesRepository.state.collectAsStateWithLifecycle()
    val focusManager = LocalFocusManager.current
    val keyboard = LocalSoftwareKeyboardController.current
    var domainInput by remember { mutableStateOf("") }
    var domainTab by remember { mutableStateOf(RuleAction.OutOfVpn) }
    // Mode switch changes which tabs exist — snap to the first valid one.
    LaunchedEffect(rules.mode) {
        if (domainTab !in tabsFor(rules.mode)) domainTab = tabsFor(rules.mode).first()
    }

    Column(
        modifier = Modifier
            .fillMaxWidth()
            // Match the top breathing room other settings sheets get from their
            // group wrappers, so the sheet header → content gap reads the same.
            .padding(top = 8.dp)
            .pointerInput(Unit) {
                detectTapGestures(onTap = {
                    keyboard?.hide()
                    focusManager.clearFocus()
                })
            },
        verticalArrangement = Arrangement.spacedBy(20.dp),
    ) {
        Section {
            SectionHeader(
                // Distinct from the sheet's "Rules" AltRoute glyph so the two
                // routing-related headers don't read as duplicates.
                icon = Icons.Outlined.Hub,
                iconBg = Color(0xFF3b82f6).copy(alpha = 0.18f),
                iconTint = Color(0xFF60a5fa),
                title = stringResource(R.string.rules_section_smart_title),
                subtitle = stringResource(R.string.rules_section_smart_subtitle),
            )
            SectionBody {
                RoutingModeSelector(
                    mode = rules.mode,
                    onSelect = { mode ->
                        RoutingRulesRepository.setMode(mode)
                        // Kick off the blocked-list download as soon as the user
                        // opts into Smart — first connect after toggling would
                        // otherwise have to block on the fetch.
                        if (mode == RoutingMode.Smart) SmartListRepository.refreshAsync()
                    },
                )
            }
        }

        HorizontalDivider(color = Brand.SurfaceHigh)

        Section {
            SectionHeader(
                icon = Icons.Outlined.Dns,
                iconBg = Color(0xFF10b981).copy(alpha = 0.18f),
                iconTint = Color(0xFF34d399),
                title = stringResource(R.string.rules_section_domains_title),
                subtitle = stringResource(
                    when (rules.mode) {
                        RoutingMode.Global -> R.string.rules_section_domains_subtitle_global
                        RoutingMode.Smart -> R.string.rules_section_domains_subtitle_smart
                    }
                ),
            )
            SectionBody {
                RuleTabs(mode = rules.mode, selected = domainTab, onSelect = { domainTab = it })
                Text(
                    stringResource(hintOf(domainTab)),
                    style = MaterialTheme.typography.bodySmall,
                    color = Brand.SecondaryText,
                )

                val active = rules.domains.listFor(domainTab)
                val trimmedInput = domainInput.trim()
                val coveredBy = remember(trimmedInput, active) {
                    if (trimmedInput.isEmpty()) null
                    else active.firstOrNull { domainPatternShadows(it, trimmedInput) }
                }
                val willShadow = remember(trimmedInput, active) {
                    if (trimmedInput.isEmpty()) emptyList()
                    else active.filter { domainPatternShadows(trimmedInput, it) }
                }
                // Domains have no shared catalogue to badge, so the cross-list
                // invariant surfaces here instead: adding a domain another tab
                // holds moves it, and we say so.
                val movedFrom = remember(trimmedInput, rules.domains, domainTab) {
                    if (trimmedInput.isEmpty()) null
                    else rules.domains.otherListHolding(trimmedInput, domainTab)
                }
                if (movedFrom != null) {
                    Text(
                        stringResource(R.string.rules_domain_moved_from, stringResource(labelOf(movedFrom))),
                        style = MaterialTheme.typography.bodySmall,
                        color = Brand.SecondaryText,
                    )
                }

                Row(
                    horizontalArrangement = Arrangement.spacedBy(10.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    OutlinedTextField(
                        value = domainInput,
                        onValueChange = { domainInput = it },
                        modifier = Modifier.weight(1f),
                        singleLine = true,
                        shape = RoundedCornerShape(16.dp),
                        placeholder = { Text(stringResource(R.string.rules_domain_placeholder)) },
                        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
                        keyboardActions = KeyboardActions(onDone = {
                            RoutingRulesRepository.addDomain(domainInput, domainTab)
                            domainInput = ""
                            keyboard?.hide(); focusManager.clearFocus()
                        }),
                    )
                    FilledTonalButton(
                        onClick = {
                            RoutingRulesRepository.addDomain(domainInput, domainTab)
                            domainInput = ""
                            keyboard?.hide(); focusManager.clearFocus()
                        },
                        shape = RoundedCornerShape(16.dp),
                        contentPadding = PaddingValues(horizontal = 18.dp, vertical = 14.dp),
                    ) {
                        Icon(Icons.Outlined.Add, contentDescription = null)
                        Spacer(Modifier.width(6.dp))
                        Text(stringResource(R.string.action_add))
                    }
                }


                if (active.isNotEmpty()) {
                    FlowRow(
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                        verticalArrangement = Arrangement.spacedBy(8.dp),
                    ) {
                        active.forEach { domain ->
                            DomainChip(
                                label = domain,
                                onRemove = { RoutingRulesRepository.removeDomain(domain, domainTab) },
                            )
                        }
                    }
                }

                val recentSuggestions = remember(rules.domains.history, active) {
                    rules.domains.history.asSequence()
                        .filter { it !in active }
                        .filter { it !in QuickDomains }
                        .take(6)
                        .toList()
                }
                if (recentSuggestions.isNotEmpty()) {
                    Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                        Text(
                            stringResource(R.string.rules_recent_title),
                            style = MaterialTheme.typography.labelMedium,
                            color = Brand.MutedText,
                        )
                        FlowRow(
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                            verticalArrangement = Arrangement.spacedBy(8.dp),
                        ) {
                            recentSuggestions.forEach { d ->
                                QuickAddChip(
                                    label = d,
                                    already = false,
                                    onAdd = { RoutingRulesRepository.addDomain(d, domainTab) },
                                )
                            }
                        }
                    }
                }

                Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    Text(
                        stringResource(R.string.rules_quick_add),
                        style = MaterialTheme.typography.labelMedium,
                        color = Brand.MutedText,
                    )
                    FlowRow(
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                        verticalArrangement = Arrangement.spacedBy(8.dp),
                    ) {
                        QuickDomains.forEach { d ->
                            QuickAddChip(
                                label = d,
                                already = d in active,
                                onAdd = { RoutingRulesRepository.addDomain(d, domainTab) },
                            )
                        }
                    }
                }
            }
        }

        HorizontalDivider(color = Brand.SurfaceHigh)

        Section {
            SectionHeader(
                icon = Icons.Outlined.Apps,
                iconBg = Color(0xFF8b5cf6).copy(alpha = 0.18f),
                iconTint = Color(0xFFa78bfa),
                title = stringResource(R.string.rules_section_perapp_title),
                subtitle = stringResource(R.string.rules_section_perapp_subtitle),
            )
            SectionBody {
                PerAppRoutingSection(mode = rules.mode)
            }
        }
    }
}

/** Common section wrapper — keeps header + body together with consistent spacing. */
@Composable
private fun Section(content: @Composable () -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) { content() }
}

/**
 * Section content, indented so it lines up under the header title rather than
 * the screen edge — mirrors the DNS block in Settings. The 50dp inset is the
 * 36dp [SettingIcon] plus the 14dp header gap.
 */
@Composable
private fun SectionBody(content: @Composable ColumnScope.() -> Unit) {
    Column(
        modifier = Modifier.padding(start = 50.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp),
        content = content,
    )
}

@Composable
private fun SectionHeader(
    icon: ImageVector,
    iconBg: Color,
    iconTint: Color,
    title: String,
    subtitle: String,
) {
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        SettingIcon(icon, iconBg, iconTint)
        Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(title, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
            Text(subtitle, style = MaterialTheme.typography.bodyMedium, color = Brand.SecondaryText)
        }
    }
}

/**
 * Compact segmented Global / Smart picker — replaces the two stacked
 * mode cards. The description of the active mode is shown below so the
 * trade-off stays visible without the bulky cards.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun RoutingModeSelector(
    mode: RoutingMode,
    onSelect: (RoutingMode) -> Unit,
) {
    val modes = listOf(RoutingMode.Global, RoutingMode.Smart)
    Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
        SingleChoiceSegmentedButtonRow(modifier = Modifier.fillMaxWidth()) {
            modes.forEachIndexed { i, m ->
                SegmentedButton(
                    selected = mode == m,
                    onClick = { onSelect(m) },
                    shape = SegmentedButtonDefaults.itemShape(i, modes.size),
                    icon = {
                        Icon(
                            imageVector = when (m) {
                                RoutingMode.Global -> Icons.Outlined.Language
                                RoutingMode.Smart -> Icons.Outlined.AltRoute
                            },
                            contentDescription = null,
                            modifier = Modifier.size(16.dp),
                        )
                    },
                ) {
                    Text(
                        stringResource(
                            when (m) {
                                RoutingMode.Global -> R.string.rules_mode_global
                                RoutingMode.Smart -> R.string.rules_mode_smart
                            },
                        ),
                    )
                }
            }
        }
        Text(
            text = stringResource(
                when (mode) {
                    RoutingMode.Global -> R.string.rules_mode_global_subtitle
                    RoutingMode.Smart -> R.string.rules_mode_smart_subtitle
                },
            ),
            style = MaterialTheme.typography.bodyMedium,
            color = Brand.SecondaryText,
        )
    }
}

/**
 * The tab pair for the current mode. Global edits the "out of VPN" list, Smart
 * the "into VPN" one; "Block" is offered in both because blocking applies to
 * both. The hidden list is not cleared — it's the other mode's setting.
 */
private fun tabsFor(mode: RoutingMode): List<RuleAction> = when (mode) {
    RoutingMode.Global -> listOf(RuleAction.OutOfVpn, RuleAction.Block)
    RoutingMode.Smart -> listOf(RuleAction.IntoVpn, RuleAction.Block)
}

private fun labelOf(action: RuleAction): Int = when (action) {
    RuleAction.OutOfVpn -> R.string.rules_tab_out_of_vpn
    RuleAction.IntoVpn -> R.string.rules_tab_into_vpn
    RuleAction.Block -> R.string.rules_tab_block
}

private fun hintOf(action: RuleAction): Int = when (action) {
    RuleAction.OutOfVpn -> R.string.rules_tab_out_of_vpn_hint
    RuleAction.IntoVpn -> R.string.rules_tab_into_vpn_hint
    RuleAction.Block -> R.string.rules_tab_block_hint
}

@Composable
private fun RuleTabs(
    mode: RoutingMode,
    selected: RuleAction,
    onSelect: (RuleAction) -> Unit,
    enabled: (RuleAction) -> Boolean = { true },
) {
    val tabs = tabsFor(mode)
    SingleChoiceSegmentedButtonRow(modifier = Modifier.fillMaxWidth()) {
        tabs.forEachIndexed { i, action ->
            SegmentedButton(
                selected = selected == action,
                enabled = enabled(action),
                onClick = { onSelect(action) },
                shape = SegmentedButtonDefaults.itemShape(i, tabs.size),
                icon = {
                    Icon(
                        imageVector = when (action) {
                            RuleAction.OutOfVpn -> Icons.Outlined.CallSplit
                            RuleAction.IntoVpn -> Icons.Outlined.VpnLock
                            RuleAction.Block -> Icons.Outlined.Block
                        },
                        contentDescription = null,
                        modifier = Modifier.size(16.dp),
                    )
                },
            ) { Text(stringResource(labelOf(action))) }
        }
    }
}

private val ChipShape = RoundedCornerShape(50)

@Composable
private fun DomainChip(label: String, onRemove: () -> Unit) {
    Row(
        modifier = Modifier
            .clip(ChipShape)
            .background(Color.White.copy(alpha = 0.07f))
            .border(1.dp, Color.White.copy(alpha = 0.09f), ChipShape)
            .padding(start = 14.dp, end = 6.dp, top = 6.dp, bottom = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        Text(label, style = MaterialTheme.typography.bodyMedium)
        IconButton(onClick = onRemove, modifier = Modifier.size(28.dp)) {
            Icon(
                Icons.Outlined.Close,
                contentDescription = stringResource(R.string.action_remove),
                tint = Brand.MutedText,
                modifier = Modifier.size(16.dp),
            )
        }
    }
}

@Composable
private fun QuickAddChip(label: String, already: Boolean, onAdd: () -> Unit) {
    val bg = if (already) Brand.Green.copy(alpha = 0.18f) else Color.White.copy(alpha = 0.06f)
    val fg = if (already) Brand.GreenLight else Brand.SecondaryText
    val border = if (already) Brand.Green.copy(alpha = 0.35f) else Color.White.copy(alpha = 0.09f)
    Row(
        modifier = Modifier
            .clip(ChipShape)
            .background(bg)
            .border(1.dp, border, ChipShape)
            .clickable(enabled = !already, onClick = onAdd)
            .padding(horizontal = 16.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            (if (already) "✓ " else "+ ") + label,
            style = MaterialTheme.typography.bodyMedium,
            color = fg,
        )
    }
}

// ────────────────────────── Per-app routing section ─────────────────────────

private data class InstalledApp(
    val packageName: String,
    val label: String,
    val icon: androidx.compose.ui.graphics.ImageBitmap?,
    val isSystem: Boolean,
)

@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
private fun PerAppRoutingSection(mode: RoutingMode) {
    // package_name rules can't match without ConnectivityManager
    // .getConnectionOwnerUid — see BoxPlatform.findConnectionOwner.
    val canResolveOwner = android.os.Build.VERSION.SDK_INT >= android.os.Build.VERSION_CODES.Q
    val tabEnabled: (RuleAction) -> Boolean = { it == RuleAction.OutOfVpn || canResolveOwner }
    val validTabs = tabsFor(mode).filter(tabEnabled)

    if (validTabs.isEmpty()) {
        // Smart mode's tabs (IntoVpn, Block) both need package_name rules,
        // which can't match below API 29 — there's nothing this section can
        // offer on such a device, so show the explanation and stop here.
        Text(
            stringResource(R.string.rules_apps_needs_q),
            style = MaterialTheme.typography.bodySmall,
            color = Brand.SecondaryText,
        )
        return
    }

    val ctx = LocalContext.current
    val appRules by AppRoutingRepository.state.collectAsStateWithLifecycle()
    var apps by remember { mutableStateOf<List<InstalledApp>>(emptyList()) }
    var loading by remember { mutableStateOf(false) }
    var query by remember { mutableStateOf("") }
    var tab by remember { mutableStateOf(RuleAction.OutOfVpn) }
    val focusManager = LocalFocusManager.current
    val keyboard = LocalSoftwareKeyboardController.current

    LaunchedEffect(mode, canResolveOwner) {
        if (tab !in validTabs) tab = validTabs.first()
    }

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

    Column(verticalArrangement = Arrangement.spacedBy(14.dp)) {
        RuleTabs(mode = mode, selected = tab, onSelect = { tab = it }, enabled = tabEnabled)

        if (!canResolveOwner) {
            Text(
                stringResource(R.string.rules_apps_needs_q),
                style = MaterialTheme.typography.bodySmall,
                color = Brand.SecondaryText,
            )
        }
        Text(
            stringResource(hintOf(tab)),
            style = MaterialTheme.typography.bodySmall,
            color = Brand.SecondaryText,
        )

        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedTextField(
                value = query,
                onValueChange = { query = it },
                modifier = Modifier.weight(1f),
                singleLine = true,
                placeholder = { Text(stringResource(R.string.rules_app_search)) },
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
                keyboardActions = KeyboardActions(onDone = {
                    keyboard?.hide(); focusManager.clearFocus()
                }),
            )
            TextButton(onClick = { AppRoutingRepository.clearList(tab) }) {
                Text(stringResource(R.string.action_clear))
            }
        }

        val selectedCount = when (tab) {
            RuleAction.OutOfVpn -> appRules.outOfVpn.size
            RuleAction.IntoVpn -> appRules.intoVpn.size
            RuleAction.Block -> appRules.blocked.size
        }
        Text(
            stringResource(R.string.rules_app_selected_count, selectedCount),
            style = MaterialTheme.typography.bodySmall,
            color = Brand.SecondaryText,
        )

        if (loading) {
            Box(modifier = Modifier.fillMaxWidth().padding(16.dp), contentAlignment = Alignment.Center) {
                CircularProgressIndicator()
            }
        } else {
            LazyColumn(
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(max = 480.dp),
                verticalArrangement = Arrangement.spacedBy(2.dp),
            ) {
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
        }
    }
}

@Composable
private fun AppRow(
    app: InstalledApp,
    checked: Boolean,
    blockedElsewhere: Boolean,
    autoBadge: Boolean,
    onToggle: () -> Unit,
) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(vertical = 4.dp, horizontal = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Checkbox(checked = checked, onCheckedChange = { onToggle() })
        if (app.icon != null) {
            Image(bitmap = app.icon, contentDescription = null, modifier = Modifier.size(32.dp))
        } else {
            Box(
                modifier = Modifier
                    .size(32.dp)
                    .clip(RoundedCornerShape(8.dp))
                    .background(Brand.SurfaceHigh),
                contentAlignment = Alignment.Center,
            ) {
                Icon(Icons.Outlined.Public, contentDescription = null, tint = Brand.SecondaryText)
            }
        }
        Column(modifier = Modifier.padding(start = 12.dp).weight(1f)) {
            Text(app.label, style = MaterialTheme.typography.bodyLarge, maxLines = 1, overflow = TextOverflow.Ellipsis)
            if (autoBadge) {
                Text(
                    "✓ " + stringResource(R.string.rules_badge_auto_vpn),
                    style = MaterialTheme.typography.bodySmall,
                    color = Brand.GreenLight,
                )
            }
            if (blockedElsewhere) {
                Text(
                    "⛔ " + stringResource(R.string.rules_badge_blocked),
                    style = MaterialTheme.typography.bodySmall,
                    color = Brand.SecondaryText,
                )
            }
            Text(
                app.packageName,
                style = MaterialTheme.typography.bodySmall,
                color = Brand.MutedText,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
        if (app.isSystem) {
            Text(stringResource(R.string.rules_app_system_tag), style = MaterialTheme.typography.labelSmall, color = Brand.MutedText)
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
