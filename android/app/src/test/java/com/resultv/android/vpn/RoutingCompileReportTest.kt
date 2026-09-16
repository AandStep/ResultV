package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class RoutingCompileReportTest {

    @Test fun parsesCountsAndUnresolved() {
        val r = parseRoutingCompileReport(
            """{"counts":{"direct":12,"proxy":3,"block":0},
                "unresolved":{"regexp:.*":"regular expressions are not supported"}}"""
        )
        assertEquals(12, r.counts["direct"])
        assertEquals(3, r.counts["proxy"])
        assertEquals(0, r.counts["block"])
        assertEquals(1, r.unresolved.size)
        assertTrue(r.error.isEmpty())
        assertTrue(r.ok)
    }

    @Test fun emptyReportIsNotAnError() {
        val r = parseRoutingCompileReport("""{"counts":{},"unresolved":{}}""")
        assertTrue(r.counts.isEmpty())
        assertTrue(r.unresolved.isEmpty())
        assertTrue(r.ok)
    }

    // Go отдаёт отказ исключением, а не полем отчёта. Но нечитаемый отчёт —
    // тоже отказ, а не «ноль правил»: второе выглядело бы как успешная сборка
    // пустого профиля.
    @Test fun brokenReportBecomesAnError() {
        val r = parseRoutingCompileReport("не json")
        assertFalse(r.ok)
        assertTrue(r.error.isNotEmpty())
        assertTrue(r.counts.isEmpty())
    }

    @Test fun totalCountsEveryAction() {
        val r = parseRoutingCompileReport("""{"counts":{"direct":2,"proxy":3,"block":5}}""")
        assertEquals(10, r.total)
    }

    // Действие, которого нет в отчёте, — это не ноль правил, а отсутствие
    // ключа: ноль пишется явно, когда профиль действие использует, но из него
    // ничего не собралось.
    @Test fun absentActionIsNotZero() {
        val r = parseRoutingCompileReport("""{"counts":{"proxy":4}}""")
        assertEquals(4, r.counts["proxy"])
        assertEquals(null, r.counts["direct"])
        assertEquals(1, r.counts.size)
    }

    // Ключи непринятых токенов — сами токены, они произвольные; разбор не
    // должен на них спотыкаться.
    @Test fun unresolvedKeysSurviveOddTokens() {
        val r = parseRoutingCompileReport(
            """{"counts":{},"unresolved":{"geosite:cat@attr":"не поддерживается",
                "https://panel.example/l.txt":"веб-страница вместо списка"}}"""
        )
        assertEquals(2, r.unresolved.size)
        assertEquals("не поддерживается", r.unresolved["geosite:cat@attr"])
    }
}
