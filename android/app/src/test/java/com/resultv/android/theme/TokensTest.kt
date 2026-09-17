package com.resultv.android.theme

import androidx.compose.ui.unit.dp
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Шкалы токенов — двадцать с лишним чисел, набранных руками с таблицы. Тест
 * ловит не «неправильное значение» (это было бы повторением кода другими
 * словами), а сломанную ЛОГИКУ шкалы: ступень, которая не убывает, или две
 * совпавшие ступени. Совпадение особенно коварно: если фон карточки случайно
 * станет равен фону экрана, разница исчезнет молча, без единой ошибки.
 */
class TokensTest {

    private fun <T : Comparable<T>> assertDescendingAndDistinct(name: String, ladder: List<T>) {
        assertEquals("$name: ступени должны убывать", ladder.sortedDescending(), ladder)
        assertEquals("$name: ступени должны различаться", ladder.distinct(), ladder)
    }

    @Test fun spacingLadderDescends() {
        assertDescendingAndDistinct(
            "RvSpace",
            listOf(RvSpace.page, RvSpace.nest1, RvSpace.nest2, RvSpace.nest3, RvSpace.xs),
        )
    }

    @Test fun radiusLadderDescends() {
        assertDescendingAndDistinct(
            "RvRadius",
            listOf(
                RvRadius.panel, RvRadius.card, RvRadius.control,
                RvRadius.chip, RvRadius.small, RvRadius.hairline,
            ),
        )
    }

    // Шкалы для текста здесь нет намеренно: типографика единственная осталась
    // прежней, мобильной, и живёт прямо в Theme.kt слотами Material. Причина
    // записана там же и в спеке, раздел «Пробелы».

    /**
     * Фоновая лестница: экран темнее карточки, карточка темнее ступени над
     * ней. Сравниваются сырые значения цвета — Color по яркости не
     * упорядочивается, а здесь все четыре оттенка серые и растут ровно.
     */
    @Test fun backgroundLadderHasFourDistinctSteps() {
        val ladder = listOf(RvColor.Black, RvColor.DarkGrey, RvColor.Grey, RvColor.LightGray)
        assertEquals("фоновые ступени должны различаться", ladder.distinct(), ladder)
    }

    @Test fun iconTileIsBiggerThanItsGlyph() {
        assertEquals(true, RvIcon.tile > RvIcon.glyph)
    }

    @Test fun motionMatchesTheDesktopCurve() {
        assertEquals(300, RvMotion.durationMillis)
    }

    @Test fun categoryTilesAreAllDistinct() {
        val tints = listOf(
            RvCategory.Blue, RvCategory.Red, RvCategory.Amber,
            RvCategory.Violet, RvCategory.Slate, RvCategory.Cyan, RvCategory.Emerald,
        ).map { it.glyph }
        assertEquals("цвета плиток должны различаться", tints.distinct(), tints)
    }

    @Test fun spacingMatchesTheSpecLadder() {
        assertEquals(
            listOf(24.dp, 16.dp, 12.dp, 8.dp, 4.dp),
            listOf(RvSpace.page, RvSpace.nest1, RvSpace.nest2, RvSpace.nest3, RvSpace.xs),
        )
    }
}
