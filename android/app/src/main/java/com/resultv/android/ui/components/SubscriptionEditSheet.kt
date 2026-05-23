package com.resultv.android.ui.components

import android.content.Intent
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.HelpOutline
import androidx.compose.material.icons.outlined.Check
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material.icons.outlined.NorthEast
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.core.net.toUri
import com.resultv.android.R
import com.resultv.android.theme.Brand
import com.resultv.android.vpn.Subscription

/**
 * Result captured by [SubscriptionEditSheet] when the user taps Save.
 */
data class SubscriptionEditResult(
    val customName: String,
    val hiddenOnHome: Boolean,
    val customRefreshIntervalMinutes: Int,
)

/**
 * Bottom-sheet settings panel for a single [Subscription]. Visual language
 * is taken verbatim from `SettingsScreen` group cards
 * (`SubscriptionsGroup`, `NetworkGroup`): one [Card] per logical group,
 * rows inside separated by [HorizontalDivider]s with `Brand.SurfaceHigh`,
 * `ListItem` for switch rows so padding matches what `SettingsScreen` ships.
 *
 * Sections:
 *   • Name (single row, OutlinedTextField inside the same card as below).
 *   • Refresh — auto-update toggle + chip flow.
 *   • Links — surfaced only when the panel response carried at least one
 *     known link header (currently `Support-Url`). No hardcoded fallbacks.
 *
 * The panel URL is intentionally NOT editable here — re-rotating the URL is
 * the import flow's job.
 */
@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
fun SubscriptionEditSheet(
    subscription: Subscription,
    onSave: (SubscriptionEditResult) -> Unit,
    onDismiss: () -> Unit,
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    val ctx = LocalContext.current

    val initialName = subscription.customName.ifBlank { subscription.displayName }
    var name by remember(subscription.id) { mutableStateOf(initialName) }
    var hiddenOnHome by remember(subscription.id) { mutableStateOf(subscription.hiddenOnHome) }
    var customIntervalOn by remember(subscription.id) {
        mutableStateOf(subscription.customRefreshIntervalMinutes > 0)
    }
    var customIntervalMinutes by remember(subscription.id) {
        mutableIntStateOf(
            subscription.customRefreshIntervalMinutes.takeIf { it > 0 }
                ?: INTERVAL_CHOICES.first().minutes
        )
    }
    val supportUrl = remember(subscription.id, subscription.supportUrl) {
        subscription.supportUrl.trim()
    }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        containerColor = Brand.Bg,
        contentColor = MaterialTheme.colorScheme.onSurface,
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp)
                .padding(bottom = 24.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            SheetHeader(displayName = subscription.displayName, onClose = onDismiss)

            // Card 1 — Name + Visibility, joined by a divider exactly like
            // SubscriptionsGroup pairs Auto-update toggle with Interval row.
            Card(
                shape = RoundedCornerShape(20.dp),
                colors = CardDefaults.cardColors(containerColor = Brand.Surface),
            ) {
                Column {
                    NameRow(
                        title = stringResource(R.string.sub_edit_section_name),
                        value = name,
                        onValueChange = { name = it },
                    )
                    HorizontalDivider(color = Brand.SurfaceHigh)
                    ToggleRow(
                        title = stringResource(R.string.sub_edit_visibility_toggle),
                        subtitle = stringResource(R.string.sub_edit_visibility_desc),
                        checked = !hiddenOnHome,
                        onCheckedChange = { hiddenOnHome = !it },
                    )
                }
            }

            // Card 2 — Refresh interval (toggle + chips). Same shape as
            // SettingsScreen.NetworkGroup's chip card.
            Card(
                shape = RoundedCornerShape(20.dp),
                colors = CardDefaults.cardColors(containerColor = Brand.Surface),
            ) {
                Column {
                    ToggleRow(
                        title = stringResource(R.string.sub_edit_custom_interval_title),
                        subtitle = stringResource(R.string.sub_edit_custom_interval_desc),
                        checked = customIntervalOn,
                        onCheckedChange = { customIntervalOn = it },
                    )
                    HorizontalDivider(color = Brand.SurfaceHigh)
                    FlowRow(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 16.dp, vertical = 14.dp),
                        horizontalArrangement = Arrangement.spacedBy(6.dp),
                        verticalArrangement = Arrangement.spacedBy(6.dp),
                    ) {
                        INTERVAL_CHOICES.forEach { choice ->
                            FilterChip(
                                selected = customIntervalOn && choice.minutes == customIntervalMinutes,
                                onClick = { customIntervalMinutes = choice.minutes },
                                enabled = customIntervalOn,
                                label = { Text(stringResource(choice.labelResId)) },
                                colors = FilterChipDefaults.filterChipColors(
                                    selectedContainerColor = Brand.Green.copy(alpha = 0.2f),
                                    selectedLabelColor = Brand.GreenLight,
                                ),
                            )
                        }
                    }
                }
            }

            // Card 3 — Links. Rendered ONLY when the panel response shipped a
            // `Support-Url` header (or anything else we plumb through later).
            if (supportUrl.isNotEmpty()) {
                Card(
                    shape = RoundedCornerShape(20.dp),
                    colors = CardDefaults.cardColors(containerColor = Brand.Surface),
                ) {
                    LinkRow(
                        title = stringResource(R.string.sub_edit_link_support),
                        onClick = {
                            runCatching {
                                ctx.startActivity(
                                    Intent(Intent.ACTION_VIEW, supportUrl.toUri())
                                        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
                                )
                            }
                        },
                    )
                }
            }

            FilledTonalButton(
                onClick = {
                    onSave(
                        SubscriptionEditResult(
                            customName = name.trim(),
                            hiddenOnHome = hiddenOnHome,
                            customRefreshIntervalMinutes = if (customIntervalOn) customIntervalMinutes else 0,
                        )
                    )
                },
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(top = 4.dp),
            ) {
                Icon(
                    imageVector = Icons.Outlined.Check,
                    contentDescription = null,
                    modifier = Modifier.size(18.dp),
                )
                Spacer(Modifier.width(8.dp))
                Text(stringResource(R.string.action_save))
            }
        }
    }
}

@Composable
private fun SheetHeader(displayName: String, onClose: () -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(top = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = stringResource(R.string.sub_edit_title),
                style = MaterialTheme.typography.labelMedium,
                color = Brand.SecondaryText,
            )
            Text(
                text = displayName,
                style = MaterialTheme.typography.titleLarge,
                fontWeight = FontWeight.SemiBold,
                maxLines = 2,
            )
        }
        IconButton(onClick = onClose) {
            Icon(
                imageVector = Icons.Outlined.Close,
                contentDescription = stringResource(R.string.sub_edit_close_cd),
                tint = Brand.SecondaryText,
            )
        }
    }
}

/**
 * One-line OutlinedTextField row. Padding mirrors `IntervalRow` /
 * `TextFieldRow` from SettingsScreen so the input lines up against the
 * same edges as the [ToggleRow]s that share the card.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun NameRow(
    title: String,
    value: String,
    onValueChange: (String) -> Unit,
) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Text(title, style = MaterialTheme.typography.bodyLarge)
        OutlinedTextField(
            value = value,
            onValueChange = onValueChange,
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
    }
}

/**
 * Same shape as SettingsScreen's ToggleRow — drives uniform horizontal
 * padding via `ListItem` defaults. Subtitle styling mirrors what
 * SettingsScreen passes (no explicit style → bodyMedium).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ToggleRow(
    title: String,
    subtitle: String,
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
) {
    ListItem(
        headlineContent = { Text(title) },
        supportingContent = { Text(subtitle, color = Brand.SecondaryText) },
        trailingContent = {
            Switch(checked = checked, onCheckedChange = onCheckedChange)
        },
        colors = ListItemDefaults.colors(containerColor = Color.Transparent),
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun LinkRow(title: String, onClick: () -> Unit) {
    ListItem(
        headlineContent = { Text(title) },
        leadingContent = {
            Icon(
                imageVector = Icons.AutoMirrored.Outlined.HelpOutline,
                contentDescription = null,
                tint = Brand.GreenLight,
            )
        },
        trailingContent = {
            Icon(
                imageVector = Icons.Outlined.NorthEast,
                contentDescription = stringResource(R.string.sub_edit_link_open_cd),
                tint = Brand.SecondaryText,
            )
        },
        colors = ListItemDefaults.colors(containerColor = Color.Transparent),
        modifier = Modifier.clickable(onClick = onClick),
    )
}

/** Refresh-interval presets in display order. Minutes is the source of truth. */
internal data class IntervalChoice(val minutes: Int, val labelResId: Int)
internal val INTERVAL_CHOICES: List<IntervalChoice> = listOf(
    IntervalChoice(30, R.string.sub_edit_interval_30m),
    IntervalChoice(60, R.string.sub_edit_interval_1h),
    IntervalChoice(120, R.string.sub_edit_interval_2h),
    IntervalChoice(360, R.string.sub_edit_interval_6h),
    IntervalChoice(720, R.string.sub_edit_interval_12h),
    IntervalChoice(1440, R.string.sub_edit_interval_24h),
)
