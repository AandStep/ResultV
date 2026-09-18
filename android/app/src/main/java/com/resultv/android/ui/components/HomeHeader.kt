package com.resultv.android.ui.components

import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Language
import androidx.compose.material.icons.outlined.Schedule
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.resultv.android.R
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.RvMotion
import com.resultv.android.theme.RvRadius
import com.resultv.android.theme.RvSpace
import com.resultv.android.vpn.VpnState
import com.resultv.android.vpn.VpnStatus

/**
 * Шапка главной — перенос `Header` (Figma 6521:263) из кита ПК.
 *
 * На ПК это один ряд: слева плашка времени, по центру заголовок стилем H1,
 * справа сайт и телеграм. На 360 dp «Что-то пошло не так» между плашкой и
 * двумя иконками не помещается, поэтому здесь два ряда: служебный сверху,
 * заголовок под ним во всю ширину. Заголовок при этом центрируется честно,
 * а не «по остатку», как на ПК.
 *
 * Логотип остаётся, слова «ResultV» больше нет: на ПК его в шапке не было
 * вовсе, оно жило в сайдбаре, которого на телефоне нет.
 *
 * Подзаголовка нет ни в одном состоянии.
 */
@Composable
fun HomeHeader(
    onOpenWebsite: () -> Unit,
    onOpenTelegram: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val status by VpnState.status.collectAsStateWithLifecycle()
    val look = homeLook(status)

    val titleColor by animateColorAsState(
        targetValue = when (look) {
            HomeLook.Idle -> RvColor.whiteA50
            HomeLook.Processing -> RvColor.Warning
            HomeLook.Success -> RvColor.Main
            HomeLook.Error -> RvColor.Errors
        },
        animationSpec = tween(RvMotion.durationMillis, easing = RvMotion.easing),
        label = "headerTitle",
    )

    Column(
        modifier = modifier
            .fillMaxWidth()
            .background(RvColor.Black)
            .windowInsetsPadding(WindowInsets.statusBars)
            .padding(horizontal = RvSpace.nest1, vertical = RvSpace.nest3),
    ) {
        Row(
            modifier = Modifier.fillMaxWidth().height(40.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(RvSpace.nest3),
        ) {
            androidx.compose.foundation.Image(
                painter = painterResource(R.drawable.resultv_logo),
                contentDescription = null,
                modifier = Modifier.size(24.dp),
            )
            (status as? VpnStatus.Connected)?.let { UptimeChip(connectedAt = it.connectedAt) }
            Spacer(Modifier.weight(1f))
            IconButton(onClick = onOpenWebsite, modifier = Modifier.size(36.dp)) {
                Icon(
                    imageVector = Icons.Outlined.Language,
                    contentDescription = stringResource(R.string.header_open_website),
                    tint = RvColor.whiteA50,
                    modifier = Modifier.size(20.dp),
                )
            }
            IconButton(onClick = onOpenTelegram, modifier = Modifier.size(36.dp)) {
                Icon(
                    painter = painterResource(R.drawable.ic_telegram),
                    contentDescription = stringResource(R.string.header_open_telegram),
                    tint = RvColor.whiteA50,
                    modifier = Modifier.size(20.dp),
                )
            }
        }

        Text(
            text = stringResource(
                when (look) {
                    HomeLook.Idle -> R.string.status_unprotected
                    HomeLook.Processing -> R.string.status_connecting
                    HomeLook.Success -> R.string.status_protected
                    HomeLook.Error -> R.string.status_error
                }
            ),
            style = MaterialTheme.typography.headlineSmall,
            fontWeight = FontWeight.Bold,
            color = titleColor,
            textAlign = TextAlign.Center,
            modifier = Modifier.fillMaxWidth().padding(top = RvSpace.nest3),
        )
    }
}

/**
 * Плашка времени соединения — `rv-header__time` с ПК.
 *
 * Отступы несимметричны по горизонтали, как в макете (9/18): слева стоит
 * значок часов со своим воздухом внутри рисунка, и равные отступы читались
 * бы как сдвиг текста влево.
 *
 * Тикает раз в секунду своим `LaunchedEffect`, чтобы остальная шапка не
 * пересобиралась вместе с таймером.
 */
@Composable
private fun UptimeChip(connectedAt: Long) {
    var now by remember { mutableLongStateOf(System.currentTimeMillis()) }
    LaunchedEffect(connectedAt) {
        while (true) {
            now = System.currentTimeMillis()
            kotlinx.coroutines.delay(1000L)
        }
    }
    val elapsedSec = ((now - connectedAt).coerceAtLeast(0L) / 1000L)

    Row(
        modifier = Modifier
            .clip(RoundedCornerShape(RvRadius.chip))
            .background(RvColor.Grey)
            .padding(start = 8.dp, top = 8.dp, bottom = 8.dp, end = 16.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        Icon(
            imageVector = Icons.Outlined.Schedule,
            contentDescription = null,
            tint = RvColor.whiteA50,
            modifier = Modifier.size(14.dp),
        )
        Text(
            text = formatUptime(elapsedSec),
            style = MaterialTheme.typography.labelMedium,
            color = RvColor.whiteA50,
        )
    }
}

internal fun formatUptime(totalSec: Long): String {
    val h = totalSec / 3600
    val m = (totalSec % 3600) / 60
    val s = totalSec % 60
    return if (h > 0) String.format("%d:%02d:%02d", h, m, s)
    else String.format("%02d:%02d", m, s)
}
