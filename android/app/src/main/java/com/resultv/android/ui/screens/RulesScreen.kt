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
import androidx.compose.material.icons.outlined.Apps
import androidx.compose.material.icons.outlined.Block
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material.icons.outlined.PlaylistAddCheck
import androidx.compose.material.icons.outlined.Public
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalButton
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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.resultv.android.R
import com.resultv.android.theme.Brand
import com.resultv.android.vpn.AppRoutingMode
import com.resultv.android.vpn.AppRoutingRepository
import com.resultv.android.vpn.RoutingMode
import com.resultv.android.vpn.RoutingRulesRepository
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

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .pointerInput(Unit) {
                detectTapGestures(onTap = {
                    keyboard?.hide()
                    focusManager.clearFocus()
                })
            }
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalArrangement = Arrangement.spacedBy(20.dp),
    ) {
        Section {
            SectionHeader(
                title = stringResource(R.string.rules_section_smart_title),
                subtitle = stringResource(R.string.rules_section_smart_subtitle),
            )
            ModeCard(
                title = stringResource(R.string.rules_mode_global),
                subtitle = stringResource(R.string.rules_mode_global_subtitle),
                selected = rules.mode == RoutingMode.Global,
                onClick = { RoutingRulesRepository.setMode(RoutingMode.Global) },
            )
            ModeCard(
                title = stringResource(R.string.rules_mode_smart),
                subtitle = stringResource(R.string.rules_mode_smart_subtitle),
                selected = rules.mode == RoutingMode.Smart,
                onClick = {
                    RoutingRulesRepository.setMode(RoutingMode.Smart)
                    // Kick off the blocked-list download as soon as the
                    // user opts in — first connect after toggling Smart
                    // would otherwise have to block on the fetch.
                    SmartListRepository.refreshAsync()
                },
            )
        }

        Section {
            SectionHeader(
                title = stringResource(R.string.rules_section_domains_title),
                subtitle = stringResource(R.string.rules_section_domains_subtitle),
            )
            Card(
                shape = CardShape,
                colors = CardDefaults.cardColors(containerColor = Brand.Surface),
            ) {
                Column(
                    modifier = Modifier.padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(14.dp),
                ) {
                    val trimmedInput = domainInput.trim()
                    val coveredBy = remember(trimmedInput, rules.domainExclusions) {
                        if (trimmedInput.isEmpty()) null
                        else rules.domainExclusions.firstOrNull {
                            domainPatternShadows(it, trimmedInput)
                        }
                    }
                    val willShadow = remember(trimmedInput, rules.domainExclusions) {
                        if (trimmedInput.isEmpty()) emptyList()
                        else rules.domainExclusions.filter {
                            domainPatternShadows(trimmedInput, it)
                        }
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
                                RoutingRulesRepository.addDomain(domainInput)
                                domainInput = ""
                                keyboard?.hide(); focusManager.clearFocus()
                            }),
                        )
                        FilledTonalButton(
                            onClick = {
                                RoutingRulesRepository.addDomain(domainInput)
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

                    if (coveredBy != null) {
                        Text(
                            stringResource(R.string.rules_shadow_covered, coveredBy),
                            style = MaterialTheme.typography.bodySmall,
                            color = Brand.Warning,
                        )
                    } else if (willShadow.isNotEmpty()) {
                        Text(
                            stringResource(
                                R.string.rules_shadow_overrides,
                                willShadow.joinToString(", "),
                            ),
                            style = MaterialTheme.typography.bodySmall,
                            color = Brand.Warning,
                        )
                    }

                    if (rules.domainExclusions.isNotEmpty()) {
                        FlowRow(
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                            verticalArrangement = Arrangement.spacedBy(8.dp),
                        ) {
                            rules.domainExclusions.forEach { domain ->
                                DomainChip(
                                    label = domain,
                                    onRemove = { RoutingRulesRepository.removeDomain(domain) },
                                )
                            }
                        }
                    }

                    // "Recently used" suggestions — persisted history of every
                    // domain the user has added at least once, filtered to
                    // those not currently in the active list. Helps re-adding
                    // a previously removed pattern without retyping.
                    val recentSuggestions = remember(rules.domainHistory, rules.domainExclusions) {
                        rules.domainHistory.asSequence()
                            .filter { it !in rules.domainExclusions }
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
                                        onAdd = { RoutingRulesRepository.addDomain(d) },
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
                                    already = d in rules.domainExclusions,
                                    onAdd = { RoutingRulesRepository.addDomain(d) },
                                )
                            }
                        }
                    }
                }
            }
        }

        Section {
            SectionHeader(
                title = stringResource(R.string.rules_section_perapp_title),
                subtitle = stringResource(R.string.rules_section_perapp_subtitle),
            )
            PerAppRoutingSection()
        }
    }
}

/** Common section wrapper — keeps header + body together with consistent spacing. */
@Composable
private fun Section(content: @Composable () -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(10.dp)) { content() }
}

@Composable
private fun SectionHeader(title: String, subtitle: String) {
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(title, style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.SemiBold)
        Text(subtitle, style = MaterialTheme.typography.bodyMedium, color = Brand.SecondaryText)
    }
}

private val ChipShape = RoundedCornerShape(50)
// Same radius as Home/Add main cards (20dp) so the standard reads as one
// design language across screens.
private val CardShape = RoundedCornerShape(20.dp)

@Composable
private fun ModeCard(
    title: String,
    subtitle: String,
    selected: Boolean,
    onClick: () -> Unit,
) {
    val borderColor = if (selected) Brand.Green.copy(alpha = 0.55f) else Color.White.copy(alpha = 0.07f)
    val containerColor = if (selected) Brand.Green.copy(alpha = 0.10f) else Brand.Surface
    Card(
        shape = CardShape,
        colors = CardDefaults.cardColors(containerColor = containerColor),
        modifier = Modifier
            .fillMaxWidth()
            .border(1.dp, borderColor, CardShape)
            .clickable(onClick = onClick),
    ) {
        Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(
                title,
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.SemiBold,
                color = if (selected) Brand.GreenLight else MaterialTheme.colorScheme.onBackground,
            )
            Text(
                subtitle,
                style = MaterialTheme.typography.bodyMedium,
                color = Brand.SecondaryText,
            )
        }
    }
}

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
private fun PerAppRoutingSection() {
    val ctx = LocalContext.current
    val routing by AppRoutingRepository.state.collectAsStateWithLifecycle()
    var apps by remember { mutableStateOf<List<InstalledApp>>(emptyList()) }
    var loading by remember { mutableStateOf(false) }
    var query by remember { mutableStateOf("") }
    val focusManager = LocalFocusManager.current
    val keyboard = LocalSoftwareKeyboardController.current

    // Load apps lazily — only when user picks a non-default mode.
    LaunchedEffect(routing.mode) {
        if (routing.mode != AppRoutingMode.All && apps.isEmpty()) {
            loading = true
            apps = withContext(Dispatchers.IO) { loadInstalledApps(ctx) }
            loading = false
        }
    }

    val filtered = remember(apps, query) {
        if (query.isBlank()) apps
        else apps.filter {
            it.label.contains(query, ignoreCase = true) ||
                it.packageName.contains(query, ignoreCase = true)
        }
    }

    Card(
        shape = CardShape,
        colors = CardDefaults.cardColors(containerColor = Brand.Surface),
    ) {
        Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
            SingleChoiceSegmentedButtonRow(modifier = Modifier.fillMaxWidth()) {
                val modes = AppRoutingMode.entries
                modes.forEachIndexed { i, m ->
                    SegmentedButton(
                        selected = routing.mode == m,
                        onClick = { AppRoutingRepository.setMode(m) },
                        shape = SegmentedButtonDefaults.itemShape(i, modes.size),
                        icon = {
                            Icon(
                                imageVector = when (m) {
                                    AppRoutingMode.All -> Icons.Outlined.Apps
                                    AppRoutingMode.AllowList -> Icons.Outlined.PlaylistAddCheck
                                    AppRoutingMode.DisallowList -> Icons.Outlined.Block
                                },
                                contentDescription = null,
                                modifier = Modifier.size(16.dp),
                            )
                        },
                    ) {
                        Text(
                            text = stringResource(
                                when (m) {
                                    AppRoutingMode.All -> R.string.rules_app_mode_all
                                    AppRoutingMode.AllowList -> R.string.rules_app_mode_allow
                                    AppRoutingMode.DisallowList -> R.string.rules_app_mode_block
                                },
                            ),
                        )
                    }
                }
            }

            if (routing.mode == AppRoutingMode.All) {
                Text(
                    stringResource(R.string.rules_app_mode_all_hint),
                    style = MaterialTheme.typography.bodyMedium,
                    color = Brand.SecondaryText,
                )
                return@Column
            }

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
                TextButton(onClick = { AppRoutingRepository.clearSelection() }) { Text(stringResource(R.string.action_clear)) }
            }

            Text(
                stringResource(R.string.rules_app_selected_count, routing.selectedPackages.size),
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
                        AppRow(
                            app = app,
                            checked = app.packageName in routing.selectedPackages,
                            onToggle = { AppRoutingRepository.toggle(app.packageName) },
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
