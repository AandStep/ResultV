package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class RoutingPreviewTest {

    // Что отдаёт Mobile.mergeRoutingProfile: весь новый список, активный и
    // сохранённый профиль отдельно.
    @Test fun parsesMergeResult() {
        val s = parseRoutingMergeResult(
            """{"profiles":[{"id":"a","name":"A"},{"id":"b","name":"B"}],
                "activeId":"b",
                "saved":{"id":"b","name":"B"}}"""
        )!!
        assertEquals(2, s.profiles.size)
        assertEquals("b", s.activeId)
        assertEquals("B", s.active?.name)
    }

    // null, а не пустое хранилище: второе выбросило бы все профили
    // пользователя из-за одной нечитаемой строки.
    @Test fun brokenMergeResultIsNullNotEmpty() {
        assertNull(parseRoutingMergeResult("не json"))
        assertNull(parseRoutingMergeResult(""))
    }

    @Test fun mergeResultWithoutProfilesIsRejected() {
        assertNull(parseRoutingMergeResult("""{"activeId":"a"}"""))
    }

    // Пустой список — законный результат (удалили последний профиль), и он не
    // должен читаться как ошибка.
    @Test fun emptyProfileListIsValid() {
        val s = parseRoutingMergeResult("""{"profiles":[],"activeId":""}""")!!
        assertEquals(0, s.profiles.size)
        assertEquals("", s.activeId)
    }

    // Go не должен присылать активный id без профиля, но если пришлёт —
    // здесь он не закрепится: движок получил бы id, по которому нет файлов.
    @Test fun activeIdPointingNowhereIsDropped() {
        val s = parseRoutingMergeResult(
            """{"profiles":[{"id":"a","name":"A"}],"activeId":"ghost"}"""
        )!!
        assertEquals("", s.activeId)
    }
}
