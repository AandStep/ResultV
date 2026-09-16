package com.resultv.android.theme

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import com.resultv.android.R

/** Benzin Bold. Им набрано слово «ResultV» — и больше ничего. */
val BenzinBold = FontFamily(Font(R.font.benzin_bold, FontWeight.Bold))

/**
 * Все слоты выставлены явно: любой незаданный уезжает в фиолетовую тональную
 * палитру Material по умолчанию, и тогда Switch, SegmentedButton или индикатор
 * NavigationBar оказываются не нашего цвета.
 */
private val ResultVColors = darkColorScheme(
    primary = RvColor.Main,
    onPrimary = RvColor.White,
    primaryContainer = RvColor.mainA20,
    onPrimaryContainer = RvColor.Main,
    inversePrimary = RvColor.Second,

    secondary = RvColor.Second,
    onSecondary = RvColor.Black,
    secondaryContainer = RvColor.secondA10,
    onSecondaryContainer = RvColor.Second,

    tertiary = RvColor.Warning,
    onTertiary = RvColor.Black,
    tertiaryContainer = RvColor.warningA10,
    onTertiaryContainer = RvColor.Warning,

    error = RvColor.Errors,
    onError = RvColor.White,
    errorContainer = RvColor.errorsA10,
    onErrorContainer = RvColor.Errors,

    background = RvColor.Black,
    onBackground = RvColor.White,
    surface = RvColor.Grey,
    onSurface = RvColor.White,
    surfaceVariant = RvColor.LightGray,
    onSurfaceVariant = RvColor.whiteA50,
    surfaceTint = RvColor.Main,

    inverseSurface = RvColor.White,
    inverseOnSurface = RvColor.Black,

    outline = RvColor.whiteA10,
    outlineVariant = RvColor.whiteA05,

    scrim = RvColor.overlay,

    surfaceBright = RvColor.LightGray,
    surfaceDim = RvColor.Black,
    surfaceContainerLowest = RvColor.Black,
    surfaceContainerLow = RvColor.DarkGrey,
    surfaceContainer = RvColor.Grey,
    surfaceContainerHigh = RvColor.LightGray,
    surfaceContainerHighest = RvColor.LightGray,
)

/**
 * Пять стилей макета разложены по слотам Material, а не живут рядом с ними:
 * компоненты M3 читают MaterialTheme.typography сами. Слоты, которым в макете
 * ничего не соответствует, получают ближайший стиль — а не выдуманное значение.
 */
private fun style(size: androidx.compose.ui.unit.TextUnit, line: androidx.compose.ui.unit.TextUnit, weight: FontWeight) =
    TextStyle(fontSize = size, lineHeight = line, fontWeight = weight)

private val H1 = style(RvType.h1Size, RvType.h1Line, FontWeight.Bold)
private val Title = style(RvType.titleSize, RvType.titleLine, FontWeight.Bold)
private val Btn = style(RvType.btnSize, RvType.btnLine, FontWeight.Bold)
private val Regular = style(RvType.regularSize, RvType.regularLine, FontWeight.Medium)
private val Chip = style(RvType.chipSize, RvType.chipLine, FontWeight.Medium)

private val ResultVTypography = Typography(
    displayLarge = H1, displayMedium = H1, displaySmall = H1,
    headlineLarge = H1, headlineMedium = Title, headlineSmall = Title,
    titleLarge = Title, titleMedium = Btn, titleSmall = Btn,
    bodyLarge = Regular, bodyMedium = Chip, bodySmall = Chip,
    labelLarge = Btn, labelMedium = Chip, labelSmall = Chip,
)

private val ResultVShapes = Shapes(
    extraSmall = RoundedCornerShape(RvRadius.small),
    small = RoundedCornerShape(RvRadius.chip),
    medium = RoundedCornerShape(RvRadius.control),
    large = RoundedCornerShape(RvRadius.card),
    extraLarge = RoundedCornerShape(RvRadius.panel),
)

@Composable
fun ResultVTheme(content: @Composable () -> Unit) {
    MaterialTheme(
        colorScheme = ResultVColors,
        typography = ResultVTypography,
        shapes = ResultVShapes,
        content = content,
    )
}
