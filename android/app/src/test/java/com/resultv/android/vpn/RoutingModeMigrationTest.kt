package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Test

class RoutingModeMigrationTest {
    @Test
    fun legacyDomainsKeepTheirDirectionsAndNestedVpnException() {
        val migrated = decodeModeRoutingRules(
            """{"mode":"Global","outOfVpnDomains":["example.org","vpn.example.org"],"intoVpnDomains":["smart.example.net"],"blockedDomains":["blocked.example"]}""",
        )
        assertEquals(listOf("example.org"), migrated.globalDomains.outOfVpn)
        assertEquals(listOf("vpn.example.org"), migrated.globalDomains.intoVpn)
        assertEquals(listOf("smart.example.net"), migrated.smartDomains.intoVpn)
        assertEquals(listOf("blocked.example"), migrated.smartDomains.blocked)
        assertEquals(migrated, decodeModeRoutingRules(encodeModeRoutingRules(migrated)))
    }

    @Test
    fun legacyAppExclusionsAndBlocksKeepTheirEffectInSmart() {
        val migrated = decodeAppModeRules(
            """{"outOfVpnApps":["direct.app"],"intoVpnApps":["tunnel.app"],"blockedApps":["blocked.app"]}""",
        )
        assertEquals(setOf("direct.app"), migrated.smart.outOfVpn)
        assertEquals(setOf("direct.app"), migrated.global.outOfVpn)
        assertEquals(setOf("tunnel.app"), migrated.smart.intoVpn)
        assertEquals(emptySet<String>(), migrated.global.intoVpn)
        assertEquals(setOf("blocked.app"), migrated.smart.blocked)
        val edited = migrated.edit(RoutingMode.Global) { it.withAction("tunnel.app", RuleAction.OutOfVpn) }
        assertEquals(migrated.smart, edited.smart)
        assertEquals(edited, decodeAppModeRules(encodeAppModeRules(edited)))
    }
}
