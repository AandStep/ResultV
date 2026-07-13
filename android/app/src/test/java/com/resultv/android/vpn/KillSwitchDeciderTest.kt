package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class KillSwitchDeciderTest {

    @Test fun healthyTicksDoNothing() {
        val d = KillSwitchDecider(failuresBeforeDead = 3)
        repeat(5) { assertEquals(KsAction.NONE, d.onTick(delayMs = 42, trafficDelta = 0).action) }
    }

    @Test fun engagesAfterConsecutiveFailures() {
        val d = KillSwitchDecider(failuresBeforeDead = 3)
        assertEquals(KsAction.NONE, d.onTick(0, 0).action)
        assertEquals(KsAction.NONE, d.onTick(0, 0).action)
        assertEquals(KsAction.ENGAGE, d.onTick(0, 0).action)
    }

    @Test fun tickReportsFailCountForProbeLogging() {
        val d = KillSwitchDecider(failuresBeforeDead = 3)
        assertEquals(1, d.onTick(0, 0).failCount)
        assertEquals(2, d.onTick(0, 0).failCount)
        assertEquals(3, d.failureThreshold)
    }

    @Test fun trafficVetoHoldsOffEngageAndIsReported() {
        val d = KillSwitchDecider(failuresBeforeDead = 3, trafficAliveBytes = 1024)
        d.onTick(0, 0); d.onTick(0, 0)
        // Probe failed but real traffic moved → veto, no engage this tick.
        val veto = d.onTick(0, trafficDelta = 4096)
        assertEquals(KsAction.NONE, veto.action)
        assertTrue(veto.deferredByTraffic)
        // Still dead, no traffic now → engages, not deferred.
        val engage = d.onTick(0, 0)
        assertEquals(KsAction.ENGAGE, engage.action)
        assertFalse(engage.deferredByTraffic)
    }

    @Test fun healthyResetsCounter() {
        val d = KillSwitchDecider(failuresBeforeDead = 3)
        d.onTick(0, 0); d.onTick(0, 0)
        assertEquals(KsAction.NONE, d.onTick(50, 0).action) // recovered before dead
        val next = d.onTick(0, 0) // counter reset → 1 fail
        assertEquals(KsAction.NONE, next.action)
        assertEquals(1, next.failCount)
    }

    @Test fun disengagesOnRecoveryAfterEngage() {
        val d = KillSwitchDecider(failuresBeforeDead = 2)
        d.onTick(0, 0)
        assertEquals(KsAction.ENGAGE, d.onTick(0, 0).action)
        assertEquals(KsAction.NONE, d.onTick(0, 0).action)      // still dead, stay engaged
        assertEquals(KsAction.DISENGAGE, d.onTick(33, 0).action) // proxy back
    }
}
