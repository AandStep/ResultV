package com.resultv.android.theme

import androidx.compose.animation.core.FastOutSlowInEasing
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp

/**
 * Токены дизайн-системы, зеркало `ResultV-dev/frontend/src/design/tokens.css`.
 * Источник — Figma "ResultV" -> страница App UI-kit (node 0:1).
 *
 * Правило то же, что на ПК: сюда не попадает ни одно значение, которого нет в
 * макете. Всё, что решено без макета, выписано в разделе «Пробелы» спеки
 * docs/superpowers/specs/2026-09-16-android-design-system-design.md, а не
 * додумывается здесь.
 *
 * Светлые токены сняты из :root[data-theme="light"] того же файла.
 */
@Immutable
class RvPalette internal constructor(val isDark: Boolean) {
    val White = if (isDark) Color.White else Color(0xFF161A17)
    val Black = if (isDark) Color(0xFF141414) else Color(0xFFE6E6E6)
    val DarkGrey = if (isDark) Color(0xFF171717) else Color(0xFFEEF0EF)
    val Grey = if (isDark) Color(0xFF1A1A1A) else Color.White
    val LightGray = if (isDark) Color(0xFF1F1F1F) else DarkGrey
    val ServerList = if (isDark) Black else Color(0xFFF2F2F2)
    val ActiveServer = if (isDark) DarkGrey else Color(0xFFE5E5E5)
    val OnAccent = Color.White
    val OnAccentGlyph = if (isDark) Black else OnAccent

    val Main = Color(0xFF007E3A)
    val Second = Color(0xFF00A819)
    val Warning = if (isDark) Color(0xFFF2CC0D) else Color(0xFFB58900)
    val Errors = Color(0xFFF20D46)

    // Прозрачные ступени. В Figma это не отдельные переменные, а базовый цвет
    // с непрозрачностью слоя; значения сняты с компонентов кита.
    val whiteA05 = White.copy(alpha = if (isDark) 0.05f else 0.045f)
    val whiteA06 = White.copy(alpha = if (isDark) 0.06f else 0.065f)   // обводка карточек групп в шторках
    val whiteA07 = White.copy(alpha = 0.07f)   // белая обводка элементов (кроме главной кнопки)
    val whiteA10 = White.copy(alpha = if (isDark) 0.10f else 0.09f)
    val whiteA15 = White.copy(alpha = if (isDark) 0.15f else 0.14f)
    val whiteA20 = White.copy(alpha = if (isDark) 0.20f else 0.24f)
    val whiteA50 = White.copy(alpha = if (isDark) 0.50f else 0.56f)
    val whiteA80 = White.copy(alpha = if (isDark) 0.80f else 0.84f)
    val description = if (isDark) whiteA50 else whiteA80
    val successText = if (isDark) Second else Main
    val warningText = if (isDark) Warning else Color(0xFF785A00)
    internal val badgeSuccessText = if (isDark) Second else Color(0xFF00672F)
    internal val badgeWarningText = if (isDark) Warning else Color(0xFF6B5000)
    internal val badgeErrorText = if (isDark) Errors else Color(0xFFA00730)
    internal val chipOutline = if (isDark) Color.Transparent else whiteA20
    internal val tagChipFill = if (isDark) LightGray else Grey
    internal val tagPlaceholder = if (isDark) whiteA20 else description

    val blackA20 = if (isDark) Black.copy(alpha = 0.20f) else White.copy(alpha = 0.08f)
    val blackA25 = if (isDark) Black.copy(alpha = 0.25f) else White.copy(alpha = 0.10f)
    val blackA80 = if (isDark) Black.copy(alpha = 0.80f) else White.copy(alpha = 0.16f)

    val mainA08 = Main.copy(alpha = 0.08f)
    val mainA10 = Main.copy(alpha = 0.10f)
    val mainA20 = Main.copy(alpha = 0.20f)
    val mainA50 = Main.copy(alpha = 0.50f)

    val warningA08 = Warning.copy(alpha = 0.08f)
    val warningA10 = Warning.copy(alpha = 0.10f)
    val warningA20 = Warning.copy(alpha = 0.20f)
    val warningA50 = Warning.copy(alpha = 0.50f)

    val errorsA08 = Errors.copy(alpha = 0.08f)
    val errorsA10 = Errors.copy(alpha = 0.10f)
    val errorsA20 = Errors.copy(alpha = 0.20f)
    val errorsA50 = Errors.copy(alpha = 0.50f)

    val secondA10 = Second.copy(alpha = 0.10f)
    val secondA50 = Second.copy(alpha = 0.50f)

    val lightGrayA50 = if (isDark) LightGray.copy(alpha = 0.50f) else White.copy(alpha = 0.05f)

    // Сняты с компонентов, переменными Figma не являются.
    val iconDefault = whiteA50
    val overlay = if (isDark) Color(0x80000000) else White.copy(alpha = 0.30f)
}

private val DarkPalette = RvPalette(isDark = true)
private val LightPalette = RvPalette(isDark = false)

internal fun paletteFor(dark: Boolean): RvPalette = if (dark) DarkPalette else LightPalette

internal val LocalRvPalette = staticCompositionLocalOf { DarkPalette }

val RvColor: RvPalette
    @Composable @ReadOnlyComposable get() = LocalRvPalette.current

/**
 * Цвета плиток категорий. В палитре Figma их нет — это решение для телефона,
 * где список длиннее и цвет помогает найти строку глазом (пробел G-4 спеки).
 * Подложка у всех 18 %, глиф — светлый парный тон.
 */
@Immutable
data class CategoryTint(val tile: Color, private val darkGlyph: Color, private val lightGlyph: Color = darkGlyph) {
    val glyph: Color
        @Composable @ReadOnlyComposable get() = glyphFor(RvColor.isDark)

    internal fun glyphFor(dark: Boolean): Color = if (dark) darkGlyph else lightGlyph
}

object RvCategory {
    val Main = DarkPalette.Main.let { CategoryTint(it.copy(alpha = 0.18f), DarkPalette.Second, it) }
    val Blue = CategoryTint(Color(0xFF3B82F6).copy(alpha = 0.18f), Color(0xFF60A5FA), Color(0xFF1D4ED8))
    val Red = CategoryTint(Color(0xFFEF4444).copy(alpha = 0.18f), Color(0xFFF87171), Color(0xFFB91C1C))
    val Amber = CategoryTint(Color(0xFFF59E0B).copy(alpha = 0.18f), Color(0xFFFBBF24), Color(0xFF92400E))
    val Violet = CategoryTint(Color(0xFF8B5CF6).copy(alpha = 0.18f), Color(0xFFA78BFA), Color(0xFF6D28D9))
    val Slate = CategoryTint(Color(0xFF64748B).copy(alpha = 0.18f), Color(0xFF94A3B8), Color(0xFF475569))
    val Cyan = CategoryTint(Color(0xFF06B6D4).copy(alpha = 0.18f), Color(0xFF22D3EE), Color(0xFF0E7490))
    val Emerald = CategoryTint(Color(0xFF10B981).copy(alpha = 0.18f), Color(0xFF34D399), Color(0xFF047857))
}

/**
 * Отступы. Шкала ПК (32 / 18 / 16 / 8 / 4), сдвинутая на одну ступень вниз:
 * экран телефона уже, а числа при этом остаются со своей же шкалы, а не
 * выдумываются.
 */
object RvSpace {
    val page = 24.dp
    val nest1 = 16.dp
    val nest2 = 12.dp
    val nest3 = 8.dp
    val xs = 4.dp
}

/** Скругления. Шкала ПК (32 / 24 / 16 / 14 / 8 / 2) ступенью ниже. */
object RvRadius {
    val panel = 20.dp
    val card = 16.dp
    val control = 12.dp
    val chip = 10.dp
    val small = 6.dp
    val hairline = 2.dp
}

/** Плитка значка: на ПК 68 с глифом 36, здесь ступенью ниже. */
object RvIcon {
    val tile = 40.dp
    val glyph = 20.dp
}

/**
 * Движение. Одна кривая на весь интерфейс: быстрый старт и мягкое торможение.
 * FastOutSlowInEasing — это ровно cubic-bezier(0.4, 0, 0.2, 1) из макета.
 */
object RvMotion {
    const val durationMillis = 300
    val easing = FastOutSlowInEasing
}

