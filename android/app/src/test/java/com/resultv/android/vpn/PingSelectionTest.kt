package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Covers which profiles an automatic ping sweep should pick up. The sweep used
 * to live only in HomeScreen's LaunchedEffect, so a server added from the Add
 * tab (which lands on Proxies) never got probed and its row span an endless
 * spinner until the user hit the manual refresh.
 */
class PingSelectionTest {

    private fun profile(id: String, name: String = "s", isSection: Boolean = false) =
        Profile(
            id = id,
            name = name,
            uri = "vless://x@1.2.3.4:443",
            entryJson = "",
            isFavorite = false,
            subscriptionId = "",
            isSection = isSection,
        )

    @Test
    fun `picks profiles that have no sample yet`() {
        val profiles = listOf(profile("a"), profile("b"), profile("c"))
        val known = setOf("b")

        val out = PingRepository.profilesNeedingPing(profiles, known)

        assertEquals(listOf("a", "c"), out.map { it.id })
    }

    @Test
    fun `skips section headers - they are not real servers`() {
        val profiles = listOf(profile("hdr", isSection = true), profile("a"))

        val out = PingRepository.profilesNeedingPing(profiles, emptySet())

        assertEquals(listOf("a"), out.map { it.id })
    }

    @Test
    fun `returns empty when everything already has a reading`() {
        val profiles = listOf(profile("a"), profile("b"))

        val out = PingRepository.profilesNeedingPing(profiles, setOf("a", "b"))

        assertEquals(emptyList<Profile>(), out)
    }

    @Test
    fun `a section header never counts as needing a ping even when unknown`() {
        val profiles = listOf(profile("hdr", isSection = true))

        val out = PingRepository.profilesNeedingPing(profiles, emptySet())

        assertEquals(emptyList<Profile>(), out)
    }
}
