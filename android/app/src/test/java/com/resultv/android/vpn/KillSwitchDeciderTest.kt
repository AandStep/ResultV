package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Test

class KillSwitchDeciderTest {

    @Test fun healthyTicksDoNothing() {
        val d = KillSwitchDecider(failuresBeforeDead = 3)
        repeat(5) { assertEquals(KsAction.NONE, d.onTick(delayMs = 42, trafficDelta = 0)) }
    }

    @Test fun engagesAfterConsecutiveFailures() {
        val d = KillSwitchDecider(failuresBeforeDead = 3)
        assertEquals(KsAction.NONE, d.onTick(0, 0))
        assertEquals(KsAction.NONE, d.onTick(0, 0))
        assertEquals(KsAction.ENGAGE, d.onTick(0, 0))
    }

    @Test fun trafficVetoHoldsOffEngage() {
        val d = KillSwitchDecider(failuresBeforeDead = 3, trafficAliveBytes = 1024)
        d.onTick(0, 0); d.onTick(0, 0)
        // Probe failed but real traffic moved → veto, no engage this tick.
        assertEquals(KsAction.NONE, d.onTick(0, trafficDelta = 4096))
        // Still dead, no traffic now → engages.
        assertEquals(KsAction.ENGAGE, d.onTick(0, 0))
    }

    @Test fun healthyResetsCounter() {
        val d = KillSwitchDecider(failuresBeforeDead = 3)
        d.onTick(0, 0); d.onTick(0, 0)
        assertEquals(KsAction.NONE, d.onTick(50, 0)) // recovered before dead
        assertEquals(KsAction.NONE, d.onTick(0, 0))  // counter reset → 1 fail
    }

    @Test fun disengagesOnRecoveryAfterEngage() {
        val d = KillSwitchDecider(failuresBeforeDead = 2)
        d.onTick(0, 0)
        assertEquals(KsAction.ENGAGE, d.onTick(0, 0))
        assertEquals(KsAction.NONE, d.onTick(0, 0))      // still dead, stay engaged
        assertEquals(KsAction.DISENGAGE, d.onTick(33, 0)) // proxy back
    }
}
