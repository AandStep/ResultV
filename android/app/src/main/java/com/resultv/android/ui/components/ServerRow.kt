package com.resultv.android.ui.components

import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.tween
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
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.resultv.android.theme.SegoeUi
import androidx.compose.ui.unit.sp
import androidx.compose.ui.unit.em
import com.resultv.android.R
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.RvMotion
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
    // Бейдж — часть плитки флага по волне: обе меняют цвет с одной и той же
    // задержкой WaveStep.Card, иначе подсветка карточки распадалась бы на
    // два разновременных пятна.
    val waveEnabled = rememberWaveEnabled()
    val delay = if (waveEnabled) {
        waveDelayMillis(WaveStep.Card, connected = accent == HomeLook.Success)
    } else 0
    val spec = tween<Color>(RvMotion.durationMillis, delay, RvMotion.easing)

    // Первый бейдж и остальные различаются В КАЖДОМ состоянии, а не только в
    // покое — так задан компонент кита на ПК (`Badge.css`, варианты First и
    // Second). Второй вариант всюду глуше первого: у покоя заливка вполовину
    // прозрачнее, у жёлтого и красного — вдвое плотнее при том же тексте, а у
    // зелёного меняется сам тон, с Main-color на Second-color.
    val bg by animateColorAsState(
        targetValue = when (accent) {
            HomeLook.Idle -> if (first) RvColor.LightGray else RvColor.lightGrayA50
            HomeLook.Processing -> if (first) RvColor.warningA10 else RvColor.warningA20
            HomeLook.Success -> if (first) RvColor.mainA10 else RvColor.secondA10
            HomeLook.Error -> if (first) RvColor.errorsA10 else RvColor.errorsA20
        },
        animationSpec = spec, label = "badgeBg",
    )
    val fg by animateColorAsState(
        targetValue = when (accent) {
            HomeLook.Idle -> RvColor.whiteA50
            HomeLook.Processing -> RvColor.Warning
            HomeLook.Success -> if (first) RvColor.Main else RvColor.Second
            HomeLook.Error -> RvColor.Errors
        },
        animationSpec = spec, label = "badgeFg",
    )
    Box(
        modifier = Modifier
            .height(20.dp)
            .clip(RoundedCornerShape(percent = 50))
            .background(bg)
            .padding(horizontal = 6.dp),
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
 * Плитка флага/молнии/глобуса — общий блок шапки карточки (`ActiveProfileRow`
 * в `HomeScreen.kt`) и строки списка ([ServerRow]). На ПК это один компонент
 * кита, `Flag` (`frontend/src/components/kit/Flag.jsx`), который принимает
 * `size` («md»/«sm») и `status` и которым пользуются и шапка, и строка
 * (`ServerItem.jsx`); здесь то же самое, но параметром — [size]/[glyph]
 * задают разницу в масштабе, а не превращают компонент в две копии. Задача 9
 * начала их расходиться по пикселям (48/44, 24/22) именно потому, что копии
 * были раздельные — общий composable убирает саму возможность разъехаться.
 *
 * Случай «профиль не выбран» отдельной ветки не требует: он совпадает со
 * «страны нет и это не авто» — просто передайте `isAuto = false,
 * countryCode = null`.
 */
@Composable
fun ProfileTile(
    accent: HomeLook,
    isAuto: Boolean,
    countryCode: String?,
    size: Dp,
    glyph: Dp,
    flagStyle: TextStyle,
) {
    val tile = when (accent) {
        HomeLook.Success -> RvColor.mainA10
        HomeLook.Processing -> RvColor.warningA10
        HomeLook.Error -> RvColor.errorsA10
        HomeLook.Idle -> RvColor.LightGray
    }
    val connected = accent == HomeLook.Success
    val waveEnabled = rememberWaveEnabled()
    val tileColor by animateColorAsState(
        targetValue = tile,
        animationSpec = tween(
            durationMillis = RvMotion.durationMillis,
            delayMillis = if (waveEnabled) waveDelayMillis(WaveStep.Card, connected) else 0,
            easing = RvMotion.easing,
        ),
        label = "rowTile",
    )
    Box(
        modifier = Modifier
            .size(size)
            .clip(RoundedCornerShape(RvRadius.chip))
            .background(tileColor),
        contentAlignment = Alignment.Center,
    ) {
        when {
            isAuto -> Icon(
                imageVector = Icons.Filled.Bolt,
                contentDescription = null,
                tint = if (accent == HomeLook.Idle) RvColor.Second else RvColor.Main,
                modifier = Modifier.size(glyph),
            )
            countryCode != null -> Text(
                text = flagFromCountry(countryCode),
                style = flagStyle,
            )
            else -> Icon(
                imageVector = Icons.Outlined.Public,
                contentDescription = null,
                tint = RvColor.whiteA50,
                modifier = Modifier.size(glyph),
            )
        }
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
    /**
     * Заливка строки в покое и заливка активной строки — параметром, а не
     * зашитым цветом. На главной строки лежат прямо на чёрной карточке
     * (RvColor.Black), и собственная подложка там уместна; на «Прокси» те же
     * строки лежат внутри уже закрашенных серых блоков подписок, и та же
     * подложка читается лишним тёмным прямоугольником поверх блока. Один и
     * тот же цвет не может быть верным в обоих местах, поэтому решает
     * вызывающий: главная не передаёт ничего и получает старое поведение,
     * «Прокси» передаёт прозрачный `surface` и лёгкий `activeSurface`.
     */
    surface: Color = RvColor.Black.copy(alpha = 0.7f),
    activeSurface: Color = RvColor.DarkGrey,
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
    val bg = if (isActive) activeSurface else surface

    Row(
        // Метрики — ServerItem мобильного макета (Figma 6864:4978).
        modifier = Modifier
            .fillMaxWidth()
            .background(bg)
            .let { base ->
                if (onLongClick != null)
                    base.combinedClickable(onClick = onClick, onLongClick = onLongClick)
                else
                    base.clickable(onClick = onClick)
            }
            .padding(start = 15.dp, end = 14.dp, top = 14.dp, bottom = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RvSpace.nest3),
    ) {
        ProfileTile(
            accent = accent,
            isAuto = isAuto,
            countryCode = countryCode,
            size = 46.dp,
            glyph = 23.dp,
            flagStyle = MaterialTheme.typography.titleLarge,
        )

        Column(modifier = Modifier.weight(1f)) {
            val shown = if (isAuto) listOf(stringResource(R.string.badge_auto)) else badges
            if (shown.isNotEmpty()) {
                // Бейджи в одну строку — переносить их на вторую нельзя,
                // строка тогда выросла бы. На 360dp три бейджа
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
                style = MaterialTheme.typography.titleMedium,
                lineHeight = 19.6.sp,
                fontWeight = FontWeight.Bold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                // Вровень с текстом бейджа, а не с краем капсулы — как в макете.
                modifier = Modifier.padding(start = 5.dp),
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
                else stringResource(R.string.ping_ms, latencyMs),
                style = PingStyle,
                color = RvColor.whiteA50,
            )
            offlineReason != null -> Text(
                text = offlineLabel(offlineReason),
                style = PingStyle,
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

/** Задержка в строке: 10 Medium, межстрочный 1.1 — как в макете. */
private val PingStyle = TextStyle(
    fontFamily = SegoeUi,
    fontSize = 10.sp,
    lineHeight = 11.sp,
    fontWeight = FontWeight.Medium,
)

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
