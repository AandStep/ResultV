package com.resultv.android.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.resultv.android.R
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.SegoeUi

/**
 * Поле поиска — Search мобильного макета (Figma 6864:4933): высота 44,
 * Grey с обводкой белой 10 %, скругление 16, лупа справа. Тот же вид у
 * поиска приложений на странице правил.
 */
@Composable
fun RvSearchField(
    value: String,
    onValueChange: (String) -> Unit,
    placeholder: String,
    modifier: Modifier = Modifier,
) {
    val shape = RoundedCornerShape(16.dp)
    val style = TextStyle(
        fontFamily = SegoeUi,
        fontSize = 12.sp,
        lineHeight = 16.8.sp,
        fontWeight = FontWeight.SemiBold,
    )
    BasicTextField(
        value = value,
        onValueChange = onValueChange,
        singleLine = true,
        textStyle = style.copy(color = RvColor.White),
        cursorBrush = SolidColor(RvColor.whiteA50),
        modifier = modifier.fillMaxWidth().height(44.dp),
        decorationBox = { inner ->
            Row(
                modifier = Modifier
                    .fillMaxSize()
                    .clip(shape)
                    .background(RvColor.Grey)
                    .border(1.dp, RvColor.whiteA10, shape)
                    .padding(horizontal = 14.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Box(modifier = Modifier.weight(1f)) {
                    if (value.isEmpty()) {
                        Text(placeholder, style = style, color = RvColor.whiteA20)
                    }
                    inner()
                }
                Icon(
                    painter = painterResource(R.drawable.ic_search),
                    contentDescription = null,
                    tint = RvColor.whiteA50,
                    modifier = Modifier.size(20.dp),
                )
            }
        },
    )
}
