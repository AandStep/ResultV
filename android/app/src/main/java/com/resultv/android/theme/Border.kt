package com.resultv.android.theme

import androidx.compose.foundation.border
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp

/**
 * Градиентная обводка интерактивных элементов, перенос `.rv-border` из
 * `ResultV-dev/frontend/src/design/borders.css`.
 *
 * Градиент идёт СТРОГО ПОПЕРЁК элемента слева направо: у левого края белый
 * 10 %, у правого 5 %. Горизонталь — не догадка: на ПК она снята с рендера
 * макета попиксельно (у сайдбара 74x740, тумблера 256x61 и карточки сервера
 * 778x118 верхняя и нижняя грани светятся одинаково, а левая и правая
 * отличаются вдвое). Вертикальной составляющей у градиента нет.
 *
 * Обводку получают элементы, с которыми можно взаимодействовать.
 */
private val BorderWidth = 1.dp

/**
 * Тот же градиент для любого цвета обводки, в том числе цветной (зелёной,
 * красной): у левого края цвет как есть, у правого — вдвое прозрачнее, как
 * белая 10 % -> 5 %.
 */
fun rvBorderBrush(color: Color): Brush =
    Brush.horizontalGradient(listOf(color, color.copy(alpha = color.alpha / 2)))

/**
 * Кисть обводки в покое — для мест, где рамку рисует не `rvBorder`.
 * На телефоне белая обводка тише макета ПК: 7 % -> 3,5 % вместо 10 -> 5.
 */
val RvBorderBrush: Brush
    @Composable get() {
        val colors = RvColor
        return remember(colors) {
            if (colors.isDark) rvBorderBrush(colors.whiteA07)
            else SolidColor(colors.White.copy(alpha = 0.10f))
        }
    }

@Composable
fun Modifier.rvBorder(shape: Shape, pressed: Boolean = false): Modifier =
    border(BorderWidth, if (pressed) pressedBorder(strong = false) else RvBorderBrush, shape)

/** Полная обводка макета — только у главной кнопки. */
@Composable
fun Modifier.rvBorderStrong(shape: Shape, pressed: Boolean = false): Modifier {
    val colors = RvColor
    val brush = if (pressed) pressedBorder(strong = true) else remember(colors) {
        if (colors.isDark) rvBorderBrush(colors.whiteA10)
        else SolidColor(colors.White.copy(alpha = 0.10f))
    }
    return border(BorderWidth, brush, shape)
}

@Composable
private fun pressedBorder(strong: Boolean): Brush {
    val colors = RvColor
    return remember(colors, strong) {
        val (left, right) = when {
            !colors.isDark -> 0.18f to 0.13f
            strong -> 0.20f to 0.15f
            else -> 0.14f to 0.105f
        }
        Brush.horizontalGradient(listOf(colors.White.copy(alpha = left), colors.White.copy(alpha = right)))
    }
}

/** Цветная обводка (состояние: выбрано, фокус, ошибка) тем же градиентом. */
fun Modifier.rvBorder(color: Color, shape: Shape, width: Dp = BorderWidth): Modifier =
    border(width, rvBorderBrush(color), shape)

/**
 * Нажатие: содержимое садится внутрь на 1 dp, габарит при этом не меняется —
 * соседи в ряду не разъезжаются и окно не дёргается по высоте. На ПК то же
 * самое делает `clip-path: inset(...)`, здесь — обрезка по форме.
 *
 * Padding идёт ПОСЛЕ clip: обрезается внешний контур, а содержимое отступает
 * внутрь от него.
 */
fun Modifier.rvPress(pressed: Boolean, shape: Shape): Modifier =
    clip(shape).then(if (pressed) Modifier.padding(BorderWidth) else Modifier)
