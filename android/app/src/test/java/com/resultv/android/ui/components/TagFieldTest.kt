package com.resultv.android.ui.components

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class TagFieldTest {

    @Test fun typingLeavesTheDraftAlone() {
        val edit = tagDraftEdit("exa")
        assertEquals("exa", edit.draft)
        assertTrue(edit.commits.isEmpty())
    }

    // Пробел коммитит — это и есть отличие от ПК, где коммитит только Enter.
    @Test fun spaceCommitsTheDraft() {
        val edit = tagDraftEdit("example.com ")
        assertEquals("", edit.draft)
        assertEquals(listOf("example.com"), edit.commits)
    }

    // Вставка списка приходит одним onValueChange: всё завершённое коммитится,
    // хвост без разделителя остаётся черновиком.
    @Test fun pasteCommitsEveryCompleteValueAndKeepsTheTail() {
        val edit = tagDraftEdit("a.ru b.ru c.r")
        assertEquals("c.r", edit.draft)
        assertEquals(listOf("a.ru", "b.ru"), edit.commits)
    }

    @Test fun anyWhitespaceSeparates() {
        val edit = tagDraftEdit("a.ru\nb.ru\tc.ru ")
        assertEquals("", edit.draft)
        assertEquals(listOf("a.ru", "b.ru", "c.ru"), edit.commits)
    }

    @Test fun runsOfWhitespaceDoNotCommitEmptyValues() {
        val edit = tagDraftEdit("   a.ru    ")
        assertEquals("", edit.draft)
        assertEquals(listOf("a.ru"), edit.commits)
    }

    @Test fun whitespaceOnlyCommitsNothing() {
        val edit = tagDraftEdit("   ")
        assertEquals("", edit.draft)
        assertTrue(edit.commits.isEmpty())
    }

    @Test fun clearingTheFieldIsNotACommit() {
        val edit = tagDraftEdit("")
        assertEquals("", edit.draft)
        assertTrue(edit.commits.isEmpty())
    }
}
