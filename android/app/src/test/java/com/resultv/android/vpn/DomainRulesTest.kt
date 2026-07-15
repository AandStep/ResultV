package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class DomainRulesTest {

    @Test fun blockEvictsFromBothRoutingLists() {
        val s = DomainRulesState(outOfVpn = listOf("a.ru"), intoVpn = listOf("a.ru"))
            .withAction("a.ru", RuleAction.Block)
        assertEquals(listOf("a.ru"), s.blocked)
        assertTrue(s.outOfVpn.isEmpty())
        assertTrue(s.intoVpn.isEmpty())
    }

    @Test fun outOfVpnKeepsIntoVpnAndClearsBlock() {
        val s = DomainRulesState(intoVpn = listOf("a.ru"), blocked = listOf("a.ru"))
            .withAction("a.ru", RuleAction.OutOfVpn)
        assertEquals(listOf("a.ru"), s.outOfVpn)
        assertEquals(listOf("a.ru"), s.intoVpn)
        assertTrue(s.blocked.isEmpty())
    }

    @Test fun addIsIdempotentAndTrims() {
        val s = DomainRulesState().withAction("  A.RU ", RuleAction.OutOfVpn)
            .withAction("a.ru", RuleAction.OutOfVpn)
        assertEquals(listOf("a.ru"), s.outOfVpn)
    }

    @Test fun addRecordsHistoryNewestFirstAndCaps() {
        var s = DomainRulesState()
        repeat(DOMAIN_HISTORY_MAX + 5) { s = s.withAction("d$it.ru", RuleAction.OutOfVpn) }
        assertEquals(DOMAIN_HISTORY_MAX, s.history.size)
        assertEquals("d${DOMAIN_HISTORY_MAX + 4}.ru", s.history.first())
    }

    @Test fun otherListHoldingFindsCrossTabDomain() {
        val s = DomainRulesState(blocked = listOf("a.ru"))
        assertEquals(RuleAction.Block, s.otherListHolding("a.ru", RuleAction.IntoVpn))
        assertNull(s.otherListHolding("b.ru", RuleAction.IntoVpn))
        // Same tab is not "other".
        assertNull(s.otherListHolding("a.ru", RuleAction.Block))
    }

    @Test fun withoutActionOnlyClearsThatTabsList() {
        val s = DomainRulesState(outOfVpn = listOf("a.ru"), intoVpn = listOf("a.ru"))
            .withoutAction("a.ru", RuleAction.OutOfVpn)
        assertTrue(s.outOfVpn.isEmpty())
        assertEquals(listOf("a.ru"), s.intoVpn)
    }

    @Test fun codecRoundTrips() {
        val s = DomainRulesState(
            outOfVpn = listOf("a.ru"), intoVpn = listOf("b.ru"),
            blocked = listOf("c.ru"), history = listOf("a.ru"),
        )
        assertEquals(s, decodeDomainRules(encodeDomainRules(s)))
    }

    @Test fun legacyDomainExclusionsMigrateToOutOfVpn() {
        val s = decodeDomainRules(
            """{"mode":"Global","domainExclusions":["*.ru","localhost"],"domainHistory":["*.ru"]}"""
        )
        assertEquals(listOf("*.ru", "localhost"), s.outOfVpn)
        assertEquals(listOf("*.ru"), s.history)
        assertTrue(s.intoVpn.isEmpty())
        assertTrue(s.blocked.isEmpty())
    }
}
