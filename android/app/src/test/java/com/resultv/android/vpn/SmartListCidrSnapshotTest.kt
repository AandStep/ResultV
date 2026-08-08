package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The IP-subnet half of the Smart block-list is tracked separately from the
 * domains because it answers a different question: it is the only thing that
 * routes Telegram's native client, which dials MTProto by IP and therefore
 * never appears in the domain count at all.
 */
class SmartListCidrSnapshotTest {

    @Test fun `cidr fields default to empty`() {
        val s = SmartListRepository.Snapshot()
        assertEquals(0, s.cidrCount)
        assertEquals("", s.cidrSource)
    }

    /**
     * The readiness of Smart routing is still decided by the compiled rule-set.
     * A large subnet list must not make an otherwise-unusable snapshot look
     * ready, or Smart mode would engage with no domain routing at all.
     */
    @Test fun `subnets alone do not make a snapshot usable`() {
        val s = SmartListRepository.Snapshot(cidrCount = 2300, cidrSource = "remote")
        assertTrue(s.isEmpty)
    }

    @Test fun `domains decide readiness regardless of subnets`() {
        val s = SmartListRepository.Snapshot(count = 150_000, ready = true, cidrCount = 0)
        assertFalse(s.isEmpty)
    }

    /**
     * "builtin" means the remote subnet list never arrived: the static Telegram
     * ranges still work, Discord voice does not. That distinction is the whole
     * reason the source is carried alongside the count rather than the count
     * being reported on its own.
     */
    @Test fun `builtin subnet source is distinguishable from a real one`() {
        val fallback = SmartListRepository.Snapshot(cidrCount = 14, cidrSource = "builtin")
        val real = SmartListRepository.Snapshot(cidrCount = 2300, cidrSource = "remote")

        assertEquals("builtin", fallback.cidrSource)
        assertTrue(
            "a real list should be substantially larger than the builtin fallback",
            real.cidrCount > fallback.cidrCount,
        )
    }

    /**
     * The subnet list has its own cache and its own fallback, so a builtin
     * subnet result says nothing about whether the domain list is good — the
     * replace rule keys off the domain source only.
     */
    @Test fun `subnet source does not affect the domain replace rule`() {
        assertTrue(shouldReplaceSmartSnapshot(curSource = "cache", curEmpty = false, nextSource = "remote"))
        assertFalse(shouldReplaceSmartSnapshot(curSource = "remote", curEmpty = false, nextSource = "builtin"))
    }
}
