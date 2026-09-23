package com.resultv.android.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.RvSpace
import com.resultv.android.theme.SegoeUi

/** Подпись кнопок и чипов мобильного макета: 12, межстрочный 1.3. Вес — у вызова. */
val RvButtonLabel = TextStyle(fontFamily = SegoeUi, fontSize = 12.sp, lineHeight = 15.6.sp)

/**
 * Кнопка мобильного макета (компонент `Button` кита): высота 52, скругление
 * 16, заливка и обводка задаются вызовом — зелёная (Main 10 %), серая (Grey с
 * белой 10 %) или выбранная (Main 50 %). Выключенная — та же кнопка на 50 %.
 */
@Composable
fun RvButton(
    onClick: () -> Unit,
    fill: Color,
    outline: Color,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    content: @Composable RowScope.() -> Unit,
) {
    val shape = RoundedCornerShape(16.dp)
    Row(
        modifier = modifier
            .height(52.dp)
            .alpha(if (enabled) 1f else 0.5f)
            .clip(shape)
            .background(fill)
            .border(1.dp, outline, shape)
            .clickable(enabled = enabled, role = Role.Button, onClick = onClick),
        horizontalArrangement = Arrangement.spacedBy(RvSpace.xs, Alignment.CenterHorizontally),
        verticalAlignment = Alignment.CenterVertically,
        content = content,
    )
}

/** Цвета кнопок макета: зелёная — основное действие, серая — второстепенное. */
object RvButtonColors {
    val greenFill = RvColor.mainA10
    val greenOutline = RvColor.mainA10
    val greenSelected = RvColor.mainA50
    val greyFill = RvColor.Grey
    val greyOutline = RvColor.whiteA10
}
