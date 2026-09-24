package com.resultv.android.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.clickable
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.painter.Painter
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.material3.Text
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.rvBorder
import com.resultv.android.theme.RvSpace

/**
 * Плитка BigBtn мобильного макета: 106 в высоту, значок 32 над подписью
 * 12 Bold, скругление 16. Обычная — Grey с белой обводкой 10 % и белым 50 %;
 * выбранная (режим на странице правил) — зелёная подложка 10 % и Main.
 * [selected] = null — плитка-действие без выбора.
 */
@Composable
fun RvBigButton(
    icon: Painter,
    label: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    selected: Boolean? = null,
) {
    val shape = RoundedCornerShape(16.dp)
    val on = selected == true
    val tint = if (on) RvColor.Main else RvColor.whiteA50
    Column(
        modifier = modifier
            .height(106.dp)
            .clip(shape)
            .background(if (on) RvColor.mainA10 else RvColor.Grey)
            .then(if (on) Modifier.rvBorder(RvColor.mainA10, shape) else Modifier.rvBorder(shape))
            .then(
                if (selected == null) Modifier.clickable(role = Role.Button, onClick = onClick)
                else Modifier.selectable(selected = on, role = Role.RadioButton, onClick = onClick)
            ),
        verticalArrangement = Arrangement.spacedBy(RvSpace.nest3, Alignment.CenterVertically),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Icon(painter = icon, contentDescription = null, tint = tint, modifier = Modifier.size(32.dp))
        Text(label, style = RvButtonLabel, fontWeight = FontWeight.Bold, color = tint)
    }
}
