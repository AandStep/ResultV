package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class AppRulesTest {

    @Test fun blockEvictsFromBothRoutingLists() {
        val s = AppRulesState(outOfVpn = setOf("a"), intoVpn = setOf("a"))
            .withAction("a", RuleAction.Block)
        assertEquals(setOf("a"), s.blocked)
        assertTrue(s.outOfVpn.isEmpty())
        assertTrue(s.intoVpn.isEmpty())
    }

    // The asymmetric half of the invariant: Global's "out of VPN" and Smart's
    // "into VPN" are never active at the same time, so setting one must NOT
    // silently destroy the other mode's setting.
    @Test fun outOfVpnKeepsIntoVpnAndClearsBlock() {
        val s = AppRulesState(intoVpn = setOf("a"), blocked = setOf("a"))
            .withAction("a", RuleAction.OutOfVpn)
        assertEquals(setOf("a"), s.outOfVpn)
        assertEquals(setOf("a"), s.intoVpn)
        assertTrue(s.blocked.isEmpty())
    }

    @Test fun intoVpnKeepsOutOfVpnAndClearsBlock() {
        val s = AppRulesState(outOfVpn = setOf("a"), blocked = setOf("a"))
            .withAction("a", RuleAction.IntoVpn)
        assertEquals(setOf("a"), s.intoVpn)
        assertEquals(setOf("a"), s.outOfVpn)
        assertTrue(s.blocked.isEmpty())
    }

    @Test fun withoutActionOnlyClearsThatTabsList() {
        val s = AppRulesState(outOfVpn = setOf("a"), intoVpn = setOf("a"))
            .withoutAction("a", RuleAction.OutOfVpn)
        assertTrue(s.outOfVpn.isEmpty())
        assertEquals(setOf("a"), s.intoVpn)
    }

    @Test fun actionOfIsModeScopedAndBlockWins() {
        val s = AppRulesState(outOfVpn = setOf("a"), intoVpn = setOf("b"), blocked = setOf("c"))
        assertEquals(RuleAction.OutOfVpn, s.actionOf("a", RoutingMode.Global))
        assertNull(s.actionOf("a", RoutingMode.Smart))
        assertEquals(RuleAction.IntoVpn, s.actionOf("b", RoutingMode.Smart))
        assertNull(s.actionOf("b", RoutingMode.Global))
        assertEquals(RuleAction.Block, s.actionOf("c", RoutingMode.Global))
        assertEquals(RuleAction.Block, s.actionOf("c", RoutingMode.Smart))
        assertNull(s.actionOf("zz", RoutingMode.Global))
    }

    @Test fun codecRoundTrips() {
        val s = AppRulesState(outOfVpn = setOf("a"), intoVpn = setOf("b"), blocked = setOf("c"))
        assertEquals(s, decodeAppRules(encodeAppRules(s)))
    }

    @Test fun legacyDisallowListMigratesToOutOfVpn() {
        val s = decodeAppRules("""{"mode":"DisallowList","packages":["a","b"]}""")
        assertEquals(setOf("a", "b"), s.outOfVpn)
        assertTrue(s.intoVpn.isEmpty())
        assertTrue(s.blocked.isEmpty())
    }

    // AllowList has no equivalent: it meant "only these in the tunnel". The
    // intent ("I want these apps in the VPN") is preserved; tunnel coverage
    // grows. The opposite mapping would invert the user's wish.
    @Test fun legacyAllowListMigratesToIntoVpn() {
        val s = decodeAppRules("""{"mode":"AllowList","packages":["a"]}""")
        assertEquals(setOf("a"), s.intoVpn)
        assertTrue(s.outOfVpn.isEmpty())
    }

    @Test fun legacyAllModeMigratesToEmpty() {
        val s = decodeAppRules("""{"mode":"All","packages":["a"]}""")
        assertEquals(AppRulesState(), s)
    }

    @Test fun legacyMigrationIsReportedOnce() {
        assertTrue(decodeAppRules("""{"mode":"AllowList","packages":["a"]}""").migratedFromAllowList)
        assertTrue(!decodeAppRules("""{"mode":"DisallowList","packages":["a"]}""").migratedFromAllowList)
        assertTrue(!decodeAppRules(encodeAppRules(AppRulesState())).migratedFromAllowList)
    }
}
