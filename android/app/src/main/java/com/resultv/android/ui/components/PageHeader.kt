package com.resultv.android.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.resultv.android.theme.RvColor

/**
 * Шапка страницы мобильного макета (AddPage 6863:4864, ServersPage 6864:4927):
 * заголовок 24 Bold слева, поле страницы 12, до содержимого 24. Справа —
 * необязательные кнопки; их край сдвигом ложится на поле страницы, хотя
 * сами кнопки шире глифа ради пальца.
 *
 * Строку состояния шапка не обходит — это дело того, кто её ставит.
 */
@Composable
fun PageHeader(
    title: String,
    modifier: Modifier = Modifier,
    actions: (@Composable RowScope.() -> Unit)? = null,
) {
    Row(
        modifier = modifier
            .fillMaxWidth()
            .padding(start = 12.dp, end = 12.dp, top = 12.dp, bottom = 24.dp)
            .height(29.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = title,
            fontSize = 24.sp,
            lineHeight = 28.8.sp,
            fontWeight = FontWeight.Bold,
            color = RvColor.White,
            maxLines = 1,
            modifier = Modifier.weight(1f),
        )
        if (actions != null) {
            // Кнопки по 36, глифы 20 с шагом 20, как в макете; край последнего
            // глифа — на поле страницы.
            Row(
                modifier = Modifier.offset(x = 8.dp),
                horizontalArrangement = Arrangement.spacedBy(4.dp),
                verticalAlignment = Alignment.CenterVertically,
                content = actions,
            )
        }
    }
}
