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
            outOfVpn = listOf("a.ru"), intoVpn = listOf("b.ru"), blocked = listOf("c.ru"),
        )
        assertEquals(s, decodeDomainRules(encodeDomainRules(s)))
    }

    // Записи старого формата несут ещё и мёртвый `domainHistory` — он просто
    // не читается, и следующее сохранение его роняет.
    @Test fun legacyDomainExclusionsMigrateToOutOfVpn() {
        val s = decodeDomainRules(
            """{"mode":"Global","domainExclusions":["*.ru","localhost"],"domainHistory":["*.ru"]}"""
        )
        assertEquals(listOf("*.ru", "localhost"), s.outOfVpn)
        assertTrue(s.intoVpn.isEmpty())
        assertTrue(s.blocked.isEmpty())
        assertTrue("domainHistory" !in encodeDomainRules(s))
    }

    // Смена дефолта на Smart копирует ПК (config.go DefaultConfig).
    @Test fun freshInstallDefaultsToSmart() {
        assertEquals(RoutingMode.Smart, RoutingRulesState().mode)
    }

    // Тот же список, что Whitelist в DefaultConfig() на ПК: без *.ru / *.рф.
    // В Smart активна вкладка «в туннель», и доменные исключения там ни на что
    // не влияют — держать их в дефолте значило бы показывать свежему
    // пользователю правила, которые не работают.
    @Test fun freshInstallExclusionsMatchDesktop() {
        assertEquals(listOf("localhost", "127.0.0.1"), RoutingRulesState().domains.outOfVpn)
        assertTrue(RoutingRulesState().domains.intoVpn.isEmpty())
    }

    // Миграция ПК (ensureDefaults): режим доставляется только тому конфигу,
    // который его не записывал. Явно выбранный режим — решение пользователя.
    @Test fun storedModeSurvivesTheNewDefault() {
        assertEquals(RoutingMode.Global, decodeRoutingMode("""{"mode":"Global"}"""))
        assertEquals(RoutingMode.Smart, decodeRoutingMode("""{"mode":"Smart"}"""))
    }

    @Test fun configWithoutModeMigratesToSmart() {
        assertEquals(RoutingMode.Smart, decodeRoutingMode("""{"domainHistory":[]}"""))
        assertEquals(RoutingMode.Smart, decodeRoutingMode("""{"mode":"Whatever"}"""))
    }
}
