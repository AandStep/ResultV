package com.resultv.android.ui.components

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.FlowRowOverflow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Bolt
import androidx.compose.material.icons.filled.Star
import androidx.compose.material.icons.outlined.Public
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.em
import com.resultv.android.R
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.RvRadius
import com.resultv.android.theme.RvSpace
import com.resultv.android.theme.rvBorder

/**
 * Бейдж протокола — перенос `Badge` (Figma 6503:3035) из кита ПК.
 *
 * Первый бейдж ярче остальных: в макете это два разных варианта, First и
 * Second. Разрядка 2 % — правило дизайнера поверх макета: имена протоколов
 * набраны латиницей в верхнем регистре и без неё слипаются.
 */
@Composable
fun ProtocolBadge(text: String, first: Boolean, accent: HomeLook) {
    val bg = when (accent) {
        HomeLook.Success -> RvColor.mainA10
        HomeLook.Processing -> RvColor.warningA10
        HomeLook.Error -> RvColor.errorsA10
        HomeLook.Idle -> if (first) RvColor.LightGray else RvColor.lightGrayA50
    }
    val fg = when (accent) {
        HomeLook.Success -> RvColor.Main
        HomeLook.Processing -> RvColor.Warning
        HomeLook.Error -> RvColor.Errors
        HomeLook.Idle -> RvColor.whiteA50
    }
    Box(
        modifier = Modifier
            .height(20.dp)
            .clip(RoundedCornerShape(percent = 50))
            .background(bg)
            .padding(horizontal = RvSpace.nest3),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            text = text,
            style = MaterialTheme.typography.labelSmall,
            fontWeight = FontWeight.SemiBold,
            letterSpacing = 0.02.em,
            color = fg,
            maxLines = 1,
        )
    }
}

/**
 * Строка сервера/профиля — используется селектором на главном экране и
 * списком «Прокси». Протокол показан бейджами над именем (перенос вида
 * ПК — Figma ServerItem); подключённая строка отмечена подложкой строки и
 * зелёным цветом плитки флага/бейджей, а не цветом имени
 * (ResultV-dev ServerItem.css:86-96).
 */
@OptIn(ExperimentalFoundationApi::class, ExperimentalLayoutApi::class)
@Composable
fun ServerRow(
    name: String,
    badges: List<String>,
    countryCode: String?,
    isAuto: Boolean,
    isActive: Boolean,
    isFavorite: Boolean,
    onClick: () -> Unit,
    /** Подсветка плитки флага и бейджей под состояние подключения. */
    accent: HomeLook = HomeLook.Idle,
    trailing: @Composable (() -> Unit)? = null,
    /** Latest ping in milliseconds when reachable, or null otherwise. */
    latencyMs: Int? = null,
    /**
     * Failure kind ("timeout", "connection_refused", …) when the server was
     * probed but is unreachable; null when it has a latency or hasn't been
     * probed yet. Renders as a text label ("Timeout"/"Refused"/…) instead of
     * leaving an endless spinner.
     */
    offlineReason: String? = null,
    /**
     * True while a user-triggered ping refresh for this row is in flight.
     * Forces the spinner even when [latencyMs] is non-null, so re-pinging
     * already-pinged servers gives visible feedback.
     */
    isLoading: Boolean = false,
    /** Long-press handler — used by Proxies to open the edit sheet. */
    onLongClick: (() -> Unit)? = null,
) {
    // Подключённый сервер выходит из прозрачности на ту же подложку, что и
    // остальные строки под касанием, — по ней его и находят глазами среди
    // прочих (ResultV-dev ServerItem.css:86-96). Зелёным его метят плитка
    // флага и бейдж, а не цвет имени.
    val bg = if (isActive) RvColor.DarkGrey else RvColor.Black.copy(alpha = 0.7f)
    val tile = when {
        accent == HomeLook.Success -> RvColor.mainA10
        accent == HomeLook.Processing -> RvColor.warningA10
        accent == HomeLook.Error -> RvColor.errorsA10
        else -> RvColor.LightGray
    }

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .height(64.dp)
            .background(bg)
            .let { base ->
                if (onLongClick != null)
                    base.combinedClickable(onClick = onClick, onLongClick = onLongClick)
                else
                    base.clickable(onClick = onClick)
            }
            .padding(horizontal = RvSpace.nest2),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2),
    ) {
        Box(
            modifier = Modifier
                .size(44.dp)
                .clip(RoundedCornerShape(RvRadius.chip))
                .background(tile)
                .rvBorder(RoundedCornerShape(RvRadius.chip)),
            contentAlignment = Alignment.Center,
        ) {
            when {
                isAuto -> Icon(
                    imageVector = Icons.Filled.Bolt,
                    contentDescription = null,
                    tint = if (accent == HomeLook.Idle) RvColor.Second else RvColor.Main,
                    modifier = Modifier.size(22.dp),
                )
                countryCode != null -> Text(
                    text = flagFromCountry(countryCode),
                    style = MaterialTheme.typography.titleLarge,
                )
                else -> Icon(
                    imageVector = Icons.Outlined.Public,
                    contentDescription = null,
                    tint = RvColor.whiteA50,
                    modifier = Modifier.size(22.dp),
                )
            }
        }

        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(RvSpace.xs),
        ) {
            val shown = if (isAuto) listOf(stringResource(R.string.badge_auto)) else badges
            if (shown.isNotEmpty()) {
                // Строка держит фиксированную высоту (64.dp) — переносить
                // бейджи на вторую строку некуда. На 360dp три бейджа
                // (обычная связка вроде VLESS + Reality + XHTTP) не
                // помещаются рядом с плиткой флага, звездой и задержкой —
                // без ограничения третий чип рисуется поверх соседей,
                // Row в Compose сам не обрезает переполнение. maxLines = 1
                // + Clip показывают столько целых бейджей, сколько влезает,
                // и обрубают по границе чипа, а не посреди него.
                FlowRow(
                    maxItemsInEachRow = Int.MAX_VALUE,
                    maxLines = 1,
                    overflow = FlowRowOverflow.Clip,
                    horizontalArrangement = Arrangement.spacedBy(RvSpace.xs),
                ) {
                    shown.forEachIndexed { i, b ->
                        ProtocolBadge(text = b, first = i == 0, accent = accent)
                    }
                }
            }
            Text(
                text = name,
                color = RvColor.White,
                style = MaterialTheme.typography.titleSmall,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }

        // Favourite marker — small star inline next to ping when set.
        // Toggle moved to the long-press sheet, so no IconButton here.
        if (isFavorite) {
            Icon(
                imageVector = Icons.Filled.Star,
                contentDescription = stringResource(R.string.action_unfavorite),
                tint = RvColor.Warning,
                modifier = Modifier.size(14.dp),
            )
        }

        // Latency reading. Precedence:
        //  1. in-flight refresh → spinner (visible feedback on re-ping)
        //  2. reachable → "N ms" (or "Online" for an RTT-less UDP probe)
        //  3. probed but unreachable → text reason ("Timeout"/"Refused"/…)
        //  4. not yet probed → spinner
        // Задержка набрана одним цветом, как на ПК: белым 50 %. Цветовая
        // шкала по порогам снята осознанно, см. P-4 спеки.
        when {
            isLoading -> androidx.compose.material3.CircularProgressIndicator(
                modifier = Modifier.size(14.dp),
                color = RvColor.whiteA50,
                strokeWidth = 2.dp,
            )
            latencyMs != null -> Text(
                text = if (latencyMs <= 0) stringResource(R.string.ping_online)
                else "$latencyMs ms",
                style = MaterialTheme.typography.labelMedium,
                color = RvColor.whiteA50,
            )
            offlineReason != null -> Text(
                text = offlineLabel(offlineReason),
                style = MaterialTheme.typography.labelMedium,
                color = RvColor.Errors,
            )
            else -> androidx.compose.material3.CircularProgressIndicator(
                modifier = Modifier.size(14.dp),
                color = RvColor.whiteA50,
                strokeWidth = 2.dp,
            )
        }

        if (trailing != null) trailing()
    }
}

/**
 * Map a probe failure [reason] to a short localized label. Mirrors the
 * desktop's pingResultToLabel vocabulary so both clients read the same.
 */
@Composable
private fun offlineLabel(reason: String): String = stringResource(
    when (reason) {
        "timeout" -> R.string.ping_timeout
        "connection_refused" -> R.string.ping_refused
        "network_unreachable", "no_route_to_host" -> R.string.ping_unreachable
        "connection_closed" -> R.string.ping_closed
        "error", "probe_error" -> R.string.ping_error
        // Причины новых типов пробы. «Нет ICMP» — это про сокет, а не про
        // узел: путать их значит отправить человека чинить не то.
        "icmp_unavailable" -> R.string.ping_no_icmp
        "unsupported_for_protocol" -> R.string.ping_not_applicable
        "bad_test_url" -> R.string.ping_bad_url
        "proxy_auth_required" -> R.string.ping_auth
        "engine_start_failed", "engine_config_failed" -> R.string.ping_engine_failed
        else -> R.string.ping_unavailable
    }
)

/** Convert a 2-letter ISO country code to the corresponding flag emoji. */
fun flagFromCountry(code: String): String {
    if (code.length != 2) return "🌐"
    val upper = code.uppercase()
    val a = 0x1F1E6 - 'A'.code + upper[0].code
    val b = 0x1F1E6 - 'A'.code + upper[1].code
    return runCatching {
        String(Character.toChars(a)) + String(Character.toChars(b))
    }.getOrDefault("🌐")
}
