package com.resultv.android.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.vector.ImageVector
import com.resultv.android.theme.CategoryTint
import com.resultv.android.theme.RvIcon
import com.resultv.android.theme.RvRadius

/**
 * Цветная плитка со значком — ведущий элемент строк настроек и шапок шторок.
 * Единственный источник правды, чтобы настройки, правила и шторка правки
 * подписки рисовали один и тот же квадрат.
 *
 * Пара цветов приходит одним значением: раздельные bg и tint разъезжались —
 * подложка одной категории вставала под глиф другой.
 */
@Composable
fun SettingIcon(icon: ImageVector, tint: CategoryTint) {
    Box(
        modifier = Modifier
            .size(RvIcon.tile)
            .clip(RoundedCornerShape(RvRadius.chip))
            .background(tint.tile),
        contentAlignment = Alignment.Center,
    ) {
        Icon(
            imageVector = icon,
            contentDescription = null,
            tint = tint.glyph,
            modifier = Modifier.size(RvIcon.glyph),
        )
    }
}
