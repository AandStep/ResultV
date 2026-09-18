package com.resultv.android.ui.components

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.RvMotion
import com.resultv.android.theme.RvRadius
import com.resultv.android.theme.RvSpace

/**
 * Плитка скорости — перенос `Speed` (Figma 6528:714) из кита ПК.
 *
 * Плитка видна всегда, как на ПК: без соединения блок цифр приглушён до
 * 50 %, а вместо кривой стоит плоская линия. Иначе страница меняла бы
 * высоту в момент подключения — ровно тогда, когда на неё смотрят.
 *
 * Высота фиксированная: пропорция ПК (158 из 423) дала бы здесь 56 dp, в
 * которые содержимое не влезает.
 */
private val TILE_HEIGHT = 112.dp
private val CHART_HEIGHT = 28.dp

@Composable
fun SpeedTile(
    label: String,
    rate: String,
    total: String,
    history: List<Float>,
    color: Color,
    active: Boolean,
    modifier: Modifier = Modifier,
) {
    val dataAlpha by animateFloatAsState(
        targetValue = if (active) 1f else 0.5f,
        animationSpec = tween(RvMotion.durationMillis, easing = RvMotion.easing),
        label = "speedData",
    )

    Card(
        modifier = modifier.height(TILE_HEIGHT),
        shape = RoundedCornerShape(RvRadius.card),
        colors = CardDefaults.cardColors(containerColor = RvColor.Grey),
    ) {
        Column(
            modifier = Modifier.fillMaxWidth().padding(RvSpace.nest1),
            verticalArrangement = Arrangement.SpaceBetween,
        ) {
            Column(modifier = Modifier.alpha(dataAlpha)) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween,
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(label, style = MaterialTheme.typography.labelMedium, color = RvColor.whiteA50)
                    Text(rate, style = MaterialTheme.typography.labelMedium, color = RvColor.whiteA50)
                }
                Text(
                    total,
                    style = MaterialTheme.typography.titleLarge,
                    fontWeight = FontWeight.Bold,
                    color = color,
                )
            }

            Box(
                modifier = Modifier.fillMaxWidth().height(CHART_HEIGHT),
                contentAlignment = Alignment.BottomStart,
            ) {
                if (active) {
                    Sparkline(
                        values = history,
                        color = color,
                        modifier = Modifier.fillMaxWidth().height(CHART_HEIGHT),
                    )
                } else {
                    // «Трафика не было» — отрезок в цвет плитки на 50 %,
                    // ровно там, где пошла бы кривая.
                    Box(
                        modifier = Modifier
                            .fillMaxWidth()
                            .height(2.dp)
                            .background(color.copy(alpha = 0.5f)),
                    )
                }
            }
        }
    }
}
