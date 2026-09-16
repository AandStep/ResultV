package com.resultv.android.theme

import androidx.compose.foundation.border
import androidx.compose.foundation.layout.padding
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Shape
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

private val BaseBrush = Brush.horizontalGradient(
    listOf(RvColor.whiteA10, RvColor.whiteA05),
)

/**
 * В макете второй слой (20 % -> 15 %) включает НАВЕДЕНИЕ. На телефоне
 * наведения нет, поэтому он отдан нажатию: повод другой, рисунок тот же.
 * См. G-3 спеки.
 */
private val PressedBrush = Brush.horizontalGradient(
    listOf(RvColor.whiteA20, RvColor.whiteA15),
)

fun Modifier.rvBorder(shape: Shape, pressed: Boolean = false): Modifier =
    border(BorderWidth, if (pressed) PressedBrush else BaseBrush, shape)

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
