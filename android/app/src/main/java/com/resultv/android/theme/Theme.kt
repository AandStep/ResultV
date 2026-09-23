package com.resultv.android.theme

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.Density
import androidx.compose.ui.text.ExperimentalTextApi
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontVariation
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp
import com.resultv.android.R

/** Benzin Bold. Им набрано слово «ResultV» — и больше ничего. */
val BenzinBold = FontFamily(Font(R.font.benzin_bold, FontWeight.Bold))

/**
 * Segoe UI Variable — шрифт всего интерфейса. Файл один, вариативный
 * (ось wght 300–700), поэтому каждый вес — тот же ресурс со своей точкой
 * на оси: без явного variationSettings Android отрисовал бы всё весом 400.
 *
 * В файле правлены метрики OS/2 typo: они приравнены к hhea (2210 / −514,
 * межстрочный 0). В оригинале typo 1491 / −431, Android строил строку по
 * ним, и прописные (0.70 em) упирались в верх коробки: текст в кнопках и
 * бейджах сидел выше центра, а строки слипались. Figma считает по hhea —
 * после правки положение текста совпадает с макетом до пикселя.
 */
@OptIn(ExperimentalTextApi::class)
val SegoeUi = FontFamily(
    listOf(
        FontWeight.Light,
        FontWeight.Normal,
        FontWeight.Medium,
        FontWeight.SemiBold,
        FontWeight.Bold,
    ).map { weight ->
        Font(
            R.font.segoe_ui_variable,
            weight,
            variationSettings = FontVariation.Settings(FontVariation.weight(weight.weight)),
        )
    }
)

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
    headlineLarge = TextStyle(fontFamily = SegoeUi, fontSize = 30.sp, lineHeight = 38.sp),
    headlineMedium = TextStyle(fontFamily = SegoeUi, fontSize = 26.sp, lineHeight = 34.sp),
    headlineSmall = TextStyle(fontFamily = SegoeUi, fontSize = 22.sp, lineHeight = 30.sp),
    titleLarge = TextStyle(fontFamily = SegoeUi, fontSize = 20.sp, lineHeight = 26.sp),
    titleMedium = TextStyle(fontFamily = SegoeUi, fontSize = 14.sp, lineHeight = 22.sp),
    titleSmall = TextStyle(fontFamily = SegoeUi, fontSize = 12.sp, lineHeight = 18.sp),
    bodyLarge = TextStyle(fontFamily = SegoeUi, fontSize = 14.sp, lineHeight = 22.sp),
    bodyMedium = TextStyle(fontFamily = SegoeUi, fontSize = 12.sp, lineHeight = 18.sp),
    bodySmall = TextStyle(fontFamily = SegoeUi, fontSize = 11.sp, lineHeight = 14.sp),
    labelLarge = TextStyle(fontFamily = SegoeUi, fontSize = 12.sp, lineHeight = 18.sp),
    labelMedium = TextStyle(fontFamily = SegoeUi, fontSize = 11.sp, lineHeight = 14.sp),
    labelSmall = TextStyle(fontFamily = SegoeUi, fontSize = 10.sp, lineHeight = 14.sp),
)

private val ResultVShapes = Shapes(
    extraSmall = RoundedCornerShape(RvRadius.small),
    small = RoundedCornerShape(RvRadius.chip),
    medium = RoundedCornerShape(RvRadius.control),
    large = RoundedCornerShape(RvRadius.card),
    extraLarge = RoundedCornerShape(RvRadius.panel),
)

/** Ширина мобильных фреймов в Figma, dp. */
private const val DESIGN_WIDTH_DP = 360f

/**
 * Интерфейс масштабируется под ширину макета: на любом телефоне 360 dp
 * макета занимают всю ширину экрана, и пропорции — как в Figma. Без этого
 * на экране шире 360 (Redmi Note 7 — 393) всё выглядело на 9 % мельче.
 *
 * Берётся меньшая сторона экрана, чтобы поворот не раздувал интерфейс.
 * Системный масштаб шрифта сохраняется поверх — это настройка доступности.
 */
@Composable
fun ResultVTheme(content: @Composable () -> Unit) {
    val base = LocalDensity.current
    val scale = LocalConfiguration.current.smallestScreenWidthDp / DESIGN_WIDTH_DP
    CompositionLocalProvider(
        LocalDensity provides Density(base.density * scale, base.fontScale),
    ) {
        MaterialTheme(
            colorScheme = ResultVColors,
            typography = ResultVTypography,
            shapes = ResultVShapes,
            content = content,
        )
    }
}
