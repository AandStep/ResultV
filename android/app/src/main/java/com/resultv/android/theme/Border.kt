package com.resultv.android.theme

import androidx.compose.foundation.border
import androidx.compose.foundation.layout.padding
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Shape
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
val RvBorderBrush = rvBorderBrush(RvColor.whiteA07)

/**
 * В макете второй слой (20 % -> 15 %) включает НАВЕДЕНИЕ. На телефоне
 * наведения нет, поэтому он отдан нажатию: повод другой, рисунок тот же.
 * См. G-3 спеки. Приглушён в той же пропорции, что и покой.
 */
private val PressedBrush = Brush.horizontalGradient(
    listOf(Color.White.copy(alpha = 0.14f), Color.White.copy(alpha = 0.105f)),
)

fun Modifier.rvBorder(shape: Shape, pressed: Boolean = false): Modifier =
    border(BorderWidth, if (pressed) PressedBrush else RvBorderBrush, shape)

/** Полная обводка макета (10 % -> 5 %, нажатие 20 -> 15) — только у главной кнопки. */
private val StrongBrush = rvBorderBrush(RvColor.whiteA10)
private val StrongPressedBrush = Brush.horizontalGradient(
    listOf(RvColor.whiteA20, RvColor.whiteA15),
)

fun Modifier.rvBorderStrong(shape: Shape, pressed: Boolean = false): Modifier =
    border(BorderWidth, if (pressed) StrongPressedBrush else StrongBrush, shape)

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
