package com.resultv.android.theme

import androidx.compose.animation.core.FastOutSlowInEasing
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
 * Объект плоский, а не CompositionLocal: тема одна и тёмная, светлой в макете
 * нет вовсе. CompositionLocal платил бы сложностью за возможность, которой не
 * пользуются; надстроить его сверху, если светлая тема появится, ничто не
 * мешает.
 *
 * Это единственный файл модуля с литералами Color(0x…). Проверяется гревом,
 * см. спеку, раздел «Проверка».
 */
object RvColor {
    val White = Color(0xFFFFFFFF)
    val Black = Color(0xFF141414)      // фон экрана
    val DarkGrey = Color(0xFF171717)
    val Grey = Color(0xFF1A1A1A)       // карточка
    val LightGray = Color(0xFF1F1F1F)

    val Main = Color(0xFF007E3A)
    val Second = Color(0xFF00A819)
    val Warning = Color(0xFFF2CC0D)
    val Errors = Color(0xFFF20D46)

    // Прозрачные ступени. В Figma это не отдельные переменные, а базовый цвет
    // с непрозрачностью слоя; значения сняты с компонентов кита.
    val whiteA05 = White.copy(alpha = 0.05f)
    val whiteA06 = White.copy(alpha = 0.06f)   // обводка карточек групп в шторках
    val whiteA10 = White.copy(alpha = 0.10f)
    val whiteA15 = White.copy(alpha = 0.15f)
    val whiteA20 = White.copy(alpha = 0.20f)
    val whiteA50 = White.copy(alpha = 0.50f)
    val whiteA80 = White.copy(alpha = 0.80f)

    val blackA20 = Black.copy(alpha = 0.20f)
    val blackA25 = Black.copy(alpha = 0.25f)
    val blackA80 = Black.copy(alpha = 0.80f)

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

    val lightGrayA50 = LightGray.copy(alpha = 0.50f)

    // Сняты с компонентов, переменными Figma не являются.
    val iconDefault = whiteA50
    val overlay = Color(0x80000000)
}

/**
 * Цвета плиток категорий. В палитре Figma их нет — это решение для телефона,
 * где список длиннее и цвет помогает найти строку глазом (пробел G-4 спеки).
 * Подложка у всех 18 %, глиф — светлый парный тон.
 */
data class CategoryTint(val tile: Color, val glyph: Color)

object RvCategory {
    val Main = CategoryTint(RvColor.Main.copy(alpha = 0.18f), RvColor.Second)
    val Blue = CategoryTint(Color(0xFF3B82F6).copy(alpha = 0.18f), Color(0xFF60A5FA))
    val Red = CategoryTint(Color(0xFFEF4444).copy(alpha = 0.18f), Color(0xFFF87171))
    val Amber = CategoryTint(Color(0xFFF59E0B).copy(alpha = 0.18f), Color(0xFFFBBF24))
    val Violet = CategoryTint(Color(0xFF8B5CF6).copy(alpha = 0.18f), Color(0xFFA78BFA))
    val Slate = CategoryTint(Color(0xFF64748B).copy(alpha = 0.18f), Color(0xFF94A3B8))
    val Cyan = CategoryTint(Color(0xFF06B6D4).copy(alpha = 0.18f), Color(0xFF22D3EE))
    val Emerald = CategoryTint(Color(0xFF10B981).copy(alpha = 0.18f), Color(0xFF34D399))
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

