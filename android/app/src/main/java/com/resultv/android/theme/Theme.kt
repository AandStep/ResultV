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
import androidx.compose.ui.unit.sp
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
 * Типографика НЕ переносится с ПК — единственная часть дизайн-системы,
 * оставленная как была.
 *
 * Шкала макета (32/20/18/16/14, Bold у верхних трёх) даже со сдвигом на
 * ступень вниз читается на телефоне слишком крупно и тяжело: на ПК её
 * держит окно 1000x740 и воздух вокруг, а на 360 dp тот же ритм превращает
 * список настроек в череду заголовков. Проверено на устройстве.
 *
 * Поэтому здесь прежняя мобильная шкала, и вес намеренно НЕ задаётся ни в
 * одном слоте: Material сам ставит Normal телу и заголовкам и Medium
 * названиям и меткам. Как только вес проставлен руками, весь интерфейс
 * уходит в полужирный — ровно это и пришлось откатывать.
 *
 * Цвета, отступы, скругления, обводка и движение перенесены с ПК как есть;
 * расхождение по типографике записано в спеку, раздел «Пробелы».
 */
private val ResultVTypography = Typography(
    headlineLarge = TextStyle(fontSize = 30.sp, lineHeight = 38.sp),
    headlineMedium = TextStyle(fontSize = 26.sp, lineHeight = 34.sp),
    headlineSmall = TextStyle(fontSize = 22.sp, lineHeight = 30.sp),
    titleLarge = TextStyle(fontSize = 20.sp, lineHeight = 26.sp),
    titleMedium = TextStyle(fontSize = 14.sp, lineHeight = 22.sp),
    titleSmall = TextStyle(fontSize = 12.sp, lineHeight = 18.sp),
    bodyLarge = TextStyle(fontSize = 14.sp, lineHeight = 22.sp),
    bodyMedium = TextStyle(fontSize = 12.sp, lineHeight = 18.sp),
    bodySmall = TextStyle(fontSize = 11.sp, lineHeight = 14.sp),
    labelLarge = TextStyle(fontSize = 12.sp, lineHeight = 18.sp),
    labelMedium = TextStyle(fontSize = 11.sp, lineHeight = 14.sp),
    labelSmall = TextStyle(fontSize = 10.sp, lineHeight = 14.sp),
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
