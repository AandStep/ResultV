package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class AppTunnelMembershipTest {

    private val own = "com.resultv.android"

    @Test fun allowlist_unionMinusExclusionsMinusOwn() {
        val allow = AppTunnelMembership.smartAllowlist(
            matched = setOf("a", "b"),
            browsers = setOf("chrome", own),   // own must be dropped even if a "browser"
            intoVpn = setOf("c"),
            outOfVpn = setOf("b"),               // exclusion wins over match
            ownPackage = own,
        )
        assertEquals(setOf("a", "chrome", "c"), allow)
    }

    @Test fun include_manualApp_clearsExclusionAndAddsIntoVpn() {
        val before = AppRulesState(outOfVpn = setOf("m"))
        val after = AppTunnelMembership.setSmartMembership(before, "m", wantIn = true, isAuto = false)
        assertTrue("m" in after.intoVpn)
        assertFalse("m" in after.outOfVpn)
    }

    @Test fun include_autoApp_onlyClearsExclusion() {
        val before = AppRulesState(outOfVpn = setOf("auto"))
        val after = AppTunnelMembership.setSmartMembership(before, "auto", wantIn = true, isAuto = true)
        assertFalse("auto" in after.outOfVpn)
        assertFalse("auto" in after.intoVpn) // stays auto, no manual entry
    }

    @Test fun exclude_autoApp_addsToOutOfVpn() {
        val before = AppRulesState()
        val after = AppTunnelMembership.setSmartMembership(before, "auto", wantIn = false, isAuto = true)
        assertTrue("auto" in after.outOfVpn)
    }

    @Test fun exclude_manualApp_removesIntoVpn() {
        val before = AppRulesState(intoVpn = setOf("m"))
        val after = AppTunnelMembership.setSmartMembership(before, "m", wantIn = false, isAuto = false)
        assertFalse("m" in after.intoVpn)
        assertFalse("m" in after.outOfVpn)
    }

    @Test fun isInSmart_reflectsAutoUnionManualMinusExclusion() {
        val rules = AppRulesState(intoVpn = setOf("m"), outOfVpn = setOf("x"))
        assertTrue(AppTunnelMembership.isInSmart("m", emptySet(), emptySet(), rules))
        assertTrue(AppTunnelMembership.isInSmart("auto", setOf("auto"), emptySet(), rules))
        assertFalse(AppTunnelMembership.isInSmart("x", setOf("x"), emptySet(), rules)) // excluded wins
        assertFalse(AppTunnelMembership.isInSmart("none", emptySet(), emptySet(), rules))
    }
}
