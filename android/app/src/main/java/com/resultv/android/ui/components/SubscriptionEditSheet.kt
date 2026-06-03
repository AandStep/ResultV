package com.resultv.android.ui.components

import android.content.Intent
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.HelpOutline
import androidx.compose.material.icons.outlined.Check
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material.icons.outlined.DriveFileRenameOutline
import androidx.compose.material.icons.outlined.Home
import androidx.compose.material.icons.outlined.NorthEast
import androidx.compose.material.icons.outlined.RssFeed
import androidx.compose.material.icons.outlined.Timer
import androidx.compose.material3.BottomSheetDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
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
import androidx.compose.ui.graphics.vector.ImageVector
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
 * Bottom-sheet settings panel for a single [Subscription]. Visual language is
 * taken verbatim from `SettingsScreen`'s sub-category sheets: `Brand.Surface`
 * container, a drag handle, NO inner cards — every option is a flat row laid
 * out on the sheet background, separated by `Brand.SurfaceHigh`
 * [HorizontalDivider]s, with the same `vertical = 8.dp` row rhythm and
 * `spacedBy(12.dp)` column spacing the settings groups ship.
 *
 * Sections:
 *   • Name (single labelled OutlinedTextField row).
 *   • Visibility — show-on-home toggle.
 *   • Refresh — custom-interval toggle + chip flow.
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
        modifier = Modifier.windowInsetsPadding(WindowInsets.statusBars),
        sheetState = sheetState,
        containerColor = Brand.Surface,
        contentColor = MaterialTheme.colorScheme.onSurface,
        dragHandle = { BottomSheetDefaults.DragHandle() },
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp, vertical = 8.dp)
                .padding(bottom = 48.dp), // Safe area
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            SheetHeader(displayName = subscription.displayName, onClose = onDismiss)

            NameRow(
                title = stringResource(R.string.sub_edit_section_name),
                value = name,
                onValueChange = { name = it },
            )

            HorizontalDivider(color = Brand.SurfaceHigh)

            ToggleRow(
                title = stringResource(R.string.sub_edit_visibility_toggle),
                subtitle = stringResource(R.string.sub_edit_visibility_desc),
                icon = Icons.Outlined.Home,
                iconBg = Color(0xFF10b981).copy(alpha = 0.18f),
                iconTint = Color(0xFF34d399),
                checked = !hiddenOnHome,
                onCheckedChange = { hiddenOnHome = !it },
            )

            HorizontalDivider(color = Brand.SurfaceHigh)

            ToggleRow(
                title = stringResource(R.string.sub_edit_custom_interval_title),
                subtitle = stringResource(R.string.sub_edit_custom_interval_desc),
                icon = Icons.Outlined.Timer,
                iconBg = Color(0xFFf59e0b).copy(alpha = 0.18f),
                iconTint = Color(0xFFfbbf24),
                checked = customIntervalOn,
                onCheckedChange = { customIntervalOn = it },
            )
            FlowRow(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(start = 50.dp),
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

            // Links — rendered ONLY when the panel response shipped a
            // `Support-Url` header (or anything else we plumb through later).
            if (supportUrl.isNotEmpty()) {
                HorizontalDivider(color = Brand.SurfaceHigh)
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
                    .padding(top = 8.dp),
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

/**
 * Header mirrors `SettingsScreen`'s sheet header (icon chip + title/desc
 * column) with a trailing close affordance the settings sheets don't need.
 */
@Composable
private fun SheetHeader(displayName: String, onClose: () -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(bottom = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        SettingIcon(
            icon = Icons.Outlined.RssFeed,
            bg = Color(0xFFf59e0b).copy(alpha = 0.18f),
            tint = Color(0xFFfbbf24),
        )
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = displayName,
                style = MaterialTheme.typography.titleLarge,
                fontWeight = FontWeight.Bold,
                maxLines = 2,
            )
            Text(
                text = stringResource(R.string.sub_edit_title),
                style = MaterialTheme.typography.bodyMedium,
                color = Brand.SecondaryText,
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
 * Labelled OutlinedTextField row matching SettingsScreen's `TextFieldRow`:
 * icon + title on one line, the field below indented to clear the icon chip.
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
            .padding(vertical = 8.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(14.dp),
        ) {
            SettingIcon(
                icon = Icons.Outlined.DriveFileRenameOutline,
                bg = Color(0xFF3b82f6).copy(alpha = 0.18f),
                tint = Color(0xFF60a5fa),
            )
            Text(title, style = MaterialTheme.typography.bodyLarge)
        }
        OutlinedTextField(
            value = value,
            onValueChange = onValueChange,
            singleLine = true,
            modifier = Modifier
                .fillMaxWidth()
                .padding(start = 50.dp),
        )
    }
}

/**
 * Flat toggle row — identical layout to SettingsScreen's `ToggleRow`
 * (icon chip + title/subtitle column + Switch, `vertical = 8.dp`).
 */
@Composable
private fun ToggleRow(
    title: String,
    subtitle: String,
    icon: ImageVector,
    iconBg: Color,
    iconTint: Color,
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        SettingIcon(icon, iconBg, iconTint)
        Column(modifier = Modifier.weight(1f)) {
            Text(title, style = MaterialTheme.typography.bodyLarge)
            Text(subtitle, style = MaterialTheme.typography.bodySmall, color = Brand.SecondaryText)
        }
        Switch(checked = checked, onCheckedChange = onCheckedChange)
    }
}

@Composable
private fun LinkRow(title: String, onClick: () -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .padding(vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        SettingIcon(
            icon = Icons.AutoMirrored.Outlined.HelpOutline,
            bg = Brand.Green.copy(alpha = 0.18f),
            tint = Brand.GreenLight,
        )
        Text(title, style = MaterialTheme.typography.bodyLarge, modifier = Modifier.weight(1f))
        Icon(
            imageVector = Icons.Outlined.NorthEast,
            contentDescription = stringResource(R.string.sub_edit_link_open_cd),
            tint = Brand.SecondaryText,
        )
    }
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
