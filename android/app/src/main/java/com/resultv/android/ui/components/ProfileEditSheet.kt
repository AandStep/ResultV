package com.resultv.android.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Star
import androidx.compose.material.icons.outlined.Bolt
import androidx.compose.material.icons.outlined.Check
import androidx.compose.material.icons.outlined.DeleteOutline
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.StarBorder
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import com.resultv.android.R
import com.resultv.android.theme.Brand
import com.resultv.android.vpn.Profile

/**
 * Long-press menu for a [Profile] row. Actions, top → bottom:
 * Probe latency · Rename · Toggle favourite · (divider) · Delete.
 * Rename uses inline editing — tap "Переименовать" to flip the row into a
 * text field, Save commits via [onRename]. The other rows act
 * immediately and dismiss the sheet so the list reflects state without
 * an extra confirm tap.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ProfileEditSheet(
    profile: Profile,
    onProbeLatency: () -> Unit,
    onRename: (String) -> Unit,
    onToggleFavorite: () -> Unit,
    onDelete: () -> Unit,
    onDismiss: () -> Unit,
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    var renameOpen by remember(profile.id) { mutableStateOf(false) }
    var name by remember(profile.id) { mutableStateOf(profile.name) }
    val saveEnabled = name.trim().isNotEmpty() && name.trim() != profile.name

    val save: () -> Unit = {
        if (saveEnabled) {
            onRename(name.trim())
            onDismiss()
        }
    }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        containerColor = Brand.Surface,
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 8.dp, vertical = 4.dp),
        ) {
            // Header row — profile name in bold so the user knows which
            // server they're editing without scrolling back to the list.
            Text(
                text = profile.name,
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.SemiBold,
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 12.dp),
            )

            ActionRow(
                icon = Icons.Outlined.Bolt,
                label = stringResource(R.string.proxies_action_probe_latency),
                onClick = { onProbeLatency(); onDismiss() },
            )
            ActionRow(
                icon = Icons.Outlined.Edit,
                label = stringResource(R.string.proxies_action_rename),
                onClick = { renameOpen = true },
            )
            if (renameOpen) {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp, vertical = 8.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    OutlinedTextField(
                        value = name,
                        onValueChange = { name = it },
                        label = { Text(stringResource(R.string.proxies_edit_name_label)) },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth(),
                        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
                        keyboardActions = KeyboardActions(onDone = { save() }),
                    )
                    Row(horizontalArrangement = Arrangement.End, modifier = Modifier.fillMaxWidth()) {
                        FilledTonalButton(onClick = save, enabled = saveEnabled) {
                            Icon(Icons.Outlined.Check, contentDescription = null)
                            Spacer(Modifier.width(6.dp))
                            Text(stringResource(R.string.action_save))
                        }
                    }
                }
            }
            ActionRow(
                icon = if (profile.isFavorite) Icons.Filled.Star else Icons.Outlined.StarBorder,
                iconTint = if (profile.isFavorite) Brand.Favorite else Brand.SecondaryText,
                label = stringResource(
                    if (profile.isFavorite) R.string.proxies_action_unfavorite
                    else R.string.proxies_action_favorite,
                ),
                onClick = { onToggleFavorite(); onDismiss() },
            )
            HorizontalDivider(
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp),
                color = Color.White.copy(alpha = 0.08f),
            )
            ActionRow(
                icon = Icons.Outlined.DeleteOutline,
                iconTint = Brand.Danger,
                labelColor = Brand.Danger,
                label = stringResource(R.string.action_delete),
                onClick = { onDelete(); onDismiss() },
            )
            Spacer(Modifier.height(8.dp))
        }
    }
}

@Composable
private fun ActionRow(
    icon: ImageVector,
    label: String,
    onClick: () -> Unit,
    iconTint: Color = Brand.SecondaryText,
    labelColor: Color = Color.Unspecified,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .background(Color.Transparent)
            .padding(0.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        TextButton(
            onClick = onClick,
            modifier = Modifier.fillMaxWidth(),
        ) {
            Box(modifier = Modifier.size(28.dp), contentAlignment = Alignment.Center) {
                Icon(
                    imageVector = icon,
                    contentDescription = null,
                    tint = iconTint,
                )
            }
            Spacer(Modifier.width(12.dp))
            Text(
                text = label,
                style = MaterialTheme.typography.bodyLarge,
                color = if (labelColor == Color.Unspecified) MaterialTheme.colorScheme.onSurface
                else labelColor,
                modifier = Modifier.fillMaxWidth(),
            )
        }
    }
}
