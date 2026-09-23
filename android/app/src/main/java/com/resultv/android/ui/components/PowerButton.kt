package com.resultv.android.ui.components

import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.requiredSize
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Icon
import androidx.compose.material3.Surface
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.draw.scale
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.resultv.android.R
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.RvMotion
import com.resultv.android.theme.rvBorder

/**
 * Кнопка питания — перенос `PowerButton` (Figma 6481:7) из кита ПК.
 *
 * Размер — из мобильного макета (Figma 6856:4994): круг 180, глиф 90,
 * ровно половина, как и на ПК.
 *
 * Кольца прогресса нет намеренно. На ПК признак работы в том, что кнопка
 * ОСТАЁТСЯ вдавленной, пока идёт подключение, и распрямляется в момент
 * успеха: вдавливание и распрямление сами по себе движение. Кольцо обещало
 * бы прогресс, которого никто не считает.
 */
private val CIRCLE = 180.dp
private val GLYPH = 90.dp
private val HALO = 216.dp

@Composable
fun PowerButton(
    look: HomeLook,
    enabled: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val interaction = remember { MutableInteractionSource() }
    val touched by interaction.collectIsPressedAsState()
    // Пока идёт подключение, кнопка вдавлена независимо от касания.
    val pressed = touched || look == HomeLook.Processing

    val motion = tween<Color>(RvMotion.durationMillis, easing = RvMotion.easing)

    val fill by animateColorAsState(
        targetValue = if (look == HomeLook.Success) RvColor.Main else RvColor.DarkGrey,
        animationSpec = motion, label = "powerFill",
    )
    val glyph by animateColorAsState(
        targetValue = when (look) {
            HomeLook.Idle -> RvColor.whiteA50
            HomeLook.Processing -> RvColor.Warning
            HomeLook.Success -> RvColor.Black
            HomeLook.Error -> RvColor.Errors
        },
        animationSpec = motion, label = "powerGlyph",
    )
    // В покое свечение у всех трёх состояний — 20 % (--rv-shadow-main/warning/
    // error на ПК). 50 % — это `--pb-glow-hover`, состояние наведения, которого
    // на телефоне не существует вовсе.
    val glow by animateColorAsState(
        targetValue = when (look) {
            HomeLook.Idle -> Color.Transparent
            HomeLook.Processing -> RvColor.warningA20
            HomeLook.Success -> RvColor.mainA20
            HomeLook.Error -> RvColor.errorsA20
        },
        animationSpec = motion, label = "powerGlow",
    )
    val scale by animateFloatAsState(
        targetValue = if (pressed) 0.9727f else 1f,
        animationSpec = tween(RvMotion.durationMillis, easing = RvMotion.easing),
        label = "powerScale",
    )
    // Внутренняя тень нажатия. На ПК это `inset -8px -8px 16px`; в Compose
    // inset-shadow нет, рисуется радиальным градиентом поверх заливки.
    val inset = if (look == HomeLook.Success) RvColor.blackA25 else RvColor.blackA80

    // Место в раскладке занимает только круг, как в мобильном макете:
    // ореол выходит за его границы и на отступы до соседей не влияет.
    Box(modifier = modifier.size(CIRCLE), contentAlignment = Alignment.Center) {
        // Свечение — отдельный диск позади кнопки, плавно уходящий в ноль,
        // чтобы у ореола не было видимого края.
        Box(
            modifier = Modifier
                .requiredSize(HALO)
                .background(
                    Brush.radialGradient(
                        colorStops = arrayOf(
                            0f to glow,
                            0.35f to glow.copy(alpha = glow.alpha * 0.55f),
                            0.7f to glow.copy(alpha = glow.alpha * 0.15f),
                            1f to Color.Transparent,
                        ),
                    ),
                    CircleShape,
                ),
        )
        Surface(
            onClick = onClick,
            enabled = enabled,
            interactionSource = interaction,
            shape = CircleShape,
            color = fill,
            contentColor = glyph,
            modifier = Modifier
                .size(CIRCLE)
                .scale(scale)
                .then(
                    when (look) {
                        // У нейтральной кнопки обводка общая, градиентная.
                        HomeLook.Idle -> Modifier.rvBorder(CircleShape, pressed = pressed)
                        // Жёлтая обводка в макете сплошная и непрозрачная.
                        HomeLook.Processing -> Modifier.border(1.dp, RvColor.Warning, CircleShape)
                        // У зелёной и красной обводки нет вовсе.
                        else -> Modifier
                    }
                )
                .drawWithContent {
                    drawContent()
                    if (pressed) {
                        drawCircle(
                            Brush.radialGradient(
                                colorStops = arrayOf(
                                    0.55f to Color.Transparent,
                                    1f to inset,
                                ),
                                radius = size.minDimension / 2f,
                            )
                        )
                    }
                },
        ) {
            Box(contentAlignment = Alignment.Center) {
                Icon(
                    painter = painterResource(R.drawable.ic_power),
                    contentDescription = stringResource(
                        if (look == HomeLook.Success) R.string.action_disconnect
                        else R.string.action_connect,
                    ),
                    modifier = Modifier.size(GLYPH),
                )
            }
        }
    }
}
