package com.resultv.android.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.BottomSheetDefaults
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.resultv.android.R
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.RvSpace
import com.resultv.android.vpn.ROUTING_ACTIONS
import com.resultv.android.vpn.RoutingProfile

/**
 * Что принесла ссылка — до того, как что-нибудь применится.
 *
 * Сеть здесь не трогается: `Mobile.previewRoutingDeepLink` только разбирает
 * payload. Правила скачиваются после согласия, и про это сказано прямым
 * текстом — иначе перечисленные geo-базы выглядели бы уже загруженными.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun RoutingDeepLinkSheet(
    profile: RoutingProfile,
    busy: Boolean,
    onAccept: () -> Unit,
    onDismiss: () -> Unit,
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    ModalBottomSheet(
        onDismissRequest = { if (!busy) onDismiss() },
        sheetState = sheetState,
        containerColor = RvColor.Grey,
        dragHandle = { BottomSheetDefaults.DragHandle() },
    ) {
        DarkSheetSystemBars()
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = RvSpace.nest1, vertical = RvSpace.nest3)
                // Не safe area: её лист держит сам. Это просто поле, чтобы
                // кнопки не упирались в панель навигации.
                .padding(bottom = RvSpace.page),
            verticalArrangement = Arrangement.spacedBy(RvSpace.nest2),
        ) {
            Text(
                stringResource(R.string.routing_sheet_title),
                style = MaterialTheme.typography.titleLarge,
                fontWeight = FontWeight.Bold,
            )
            Text(profile.name, style = MaterialTheme.typography.titleMedium)
            if (profile.publisherName.isNotEmpty()) {
                Text(
                    stringResource(R.string.routing_sheet_publisher, profile.publisherName),
                    style = MaterialTheme.typography.bodySmall,
                    color = RvColor.whiteA50,
                )
            }
            // Ноль не показывается — то же правило, что у карточки в списке
            // профилей. Иначе один и тот же профиль выглядит по-разному в двух
            // соседних окнах: «0 direct · 1 proxy · 0 block» здесь и «1 proxy»
            // строкой ниже.
            val parts = ROUTING_ACTIONS.mapNotNull { action ->
                val n = profile.ruleCount(action)
                if (n > 0) "$n $action" else null
            }
            if (parts.isNotEmpty()) {
                Text(
                    parts.joinToString(" · "),
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
            if (profile.geositeUrl.isNotEmpty() || profile.geoipUrl.isNotEmpty()) {
                Text(
                    stringResource(R.string.routing_sheet_geo),
                    style = MaterialTheme.typography.bodySmall,
                    color = RvColor.whiteA50,
                )
            }
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(RvSpace.nest3, Alignment.End),
            ) {
                TextButton(onClick = onDismiss, enabled = !busy) {
                    Text(stringResource(R.string.routing_sheet_decline))
                }
                Button(onClick = onAccept, enabled = !busy) {
                    Text(stringResource(R.string.routing_sheet_accept))
                }
            }
        }
    }
}
