package com.resultv.android.theme

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

import androidx.compose.material3.Typography
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.unit.sp

// All slots are explicitly set so Material 3 components (Switch, SegmentedButton,
// Checkbox, NavigationBar indicator, …) can't fall back to the default purple
// tonal palette for any unset slot.
private val ResultVColors = darkColorScheme(
    primary = Brand.Green,
    onPrimary = Color.White,
    primaryContainer = Brand.GreenDark,
    onPrimaryContainer = Color.White,
    inversePrimary = Brand.GreenLight,

    secondary = Brand.GreenLight,
    onSecondary = Color.Black,
    secondaryContainer = Color(0xFF003D1C),
    onSecondaryContainer = Brand.GreenLight,

    tertiary = Brand.Favorite,
    onTertiary = Color.Black,
    tertiaryContainer = Color(0xFF3D2E00),
    onTertiaryContainer = Brand.Favorite,

    error = Brand.Danger,
    onError = Color.White,
    errorContainer = Color(0x33F43F5E),
    onErrorContainer = Brand.Danger,

    background = Brand.Bg,
    onBackground = Color(0xFFEDEDEF),
    surface = Brand.Surface,
    onSurface = Color(0xFFEDEDEF),
    surfaceVariant = Brand.SurfaceHigh,
    onSurfaceVariant = Brand.SecondaryText,
    surfaceTint = Brand.Green,

    inverseSurface = Color(0xFFEDEDEF),
    inverseOnSurface = Brand.Bg,

    outline = Brand.SurfaceBorder,
    outlineVariant = Brand.SurfaceHigh,

    scrim = Color(0xCC000000),

    // Material 3 1.2+ surface tonal slots. All clamped to our brand
    // dark scale so containerised components (NavigationBar, SegmentedButton,
    // Card with elevation > 0) stay on-brand.
    surfaceBright = Color(0xFF2A2A2E),
    surfaceDim = Brand.Bg,
    surfaceContainerLowest = Color(0xFF050507),
    surfaceContainerLow = Color(0xFF0E0E11),
    surfaceContainer = Brand.Surface,
    surfaceContainerHigh = Color(0xFF1F1F22),
    surfaceContainerHighest = Brand.SurfaceHigh,
)

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

@Composable
fun ResultVTheme(content: @Composable () -> Unit) {
    MaterialTheme(
        colorScheme = ResultVColors,
        typography = ResultVTypography,
        content = content,
    )
}
