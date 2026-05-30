package com.resultv.android.ui.components

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.Stroke
import kotlin.math.max

/**
 * Sparkline — visual parity with desktop SpeedChart (SpeedChart.jsx).
 *
 * Renders [values] as a thin polyline scaled to max(values, [baseline]).
 * No fill, 0.9 alpha, 2.25 px stroke — matches the desktop look so a brief
 * 1 s burst draws as a smooth dip-and-return rather than a tall narrow spike.
 *
 * Expects [values] to already be pre-filled (e.g. with zeros) at the buffer
 * size — TrafficStats does this — so the line always has a baseline to draw
 * against and we never fall into a degenerate single-point case.
 */
@Composable
fun Sparkline(
    values: List<Float>,
    color: Color,
    modifier: Modifier = Modifier,
    strokeWidthPx: Float = 2.25f,
    baseline: Float = 1024f,
) {
    Canvas(modifier = modifier.fillMaxSize()) {
        val w = size.width
        val h = size.height
        if (w <= 0f || h <= 0f || values.size < 2) return@Canvas

        val peak = max(values.max(), baseline).coerceAtLeast(1f)
        val stepX = w / (values.size - 1).toFloat()
        val pad = strokeWidthPx
        val drawH = h - pad * 2

        val stroke = Path().apply {
            for (i in values.indices) {
                val x = i * stepX
                val norm = (values[i].coerceAtLeast(0f) / peak).coerceIn(0f, 1f)
                val y = pad + (1f - norm) * drawH
                if (i == 0) moveTo(x, y) else lineTo(x, y)
            }
        }

        drawPath(
            path = stroke,
            color = color.copy(alpha = 0.9f),
            style = Stroke(
                width = strokeWidthPx,
                cap = StrokeCap.Round,
                join = StrokeJoin.Round,
            ),
        )
    }
}
