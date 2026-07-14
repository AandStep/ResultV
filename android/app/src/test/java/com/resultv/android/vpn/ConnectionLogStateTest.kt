package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class ConnectionLogStateTest {

    @Test fun firstSeenDomainFormatsDesktopStyle() {
        val s = ConnectionLogState()
        assertEquals(
            "2ip.ru -> 188.40.167.82:443 | via proxy | status: connected",
            s.onConnection("2ip.ru", "188.40.167.82:443", "proxy"),
        )
    }

    @Test fun blankDomainSkipped() {
        val s = ConnectionLogState()
        assertNull(s.onConnection("", "1.2.3.4:443", "proxy"))
        assertNull(s.onConnection("   ", "1.2.3.4:443", "proxy"))
    }

    @Test fun duplicateDomainSkippedWithinSession() {
        val s = ConnectionLogState()
        assertEquals(
            "example.com -> 93.184.216.34:443 | via proxy | status: connected",
            s.onConnection("example.com", "93.184.216.34:443", "proxy"),
        )
        // Same host reconnecting → logged once per session, like the desktop.
        assertNull(s.onConnection("example.com", "93.184.216.34:443", "proxy"))
        assertNull(s.onConnection("example.com", "10.0.0.1:80", "proxy"))
    }

    @Test fun directOrBlankOutboundOmitsVia() {
        val s = ConnectionLogState()
        assertEquals(
            "a.com -> 1.1.1.1:443 | status: connected",
            s.onConnection("a.com", "1.1.1.1:443", "direct"),
        )
        assertEquals(
            "b.com -> 1.1.1.1:443 | status: connected",
            s.onConnection("b.com", "1.1.1.1:443", ""),
        )
    }

    @Test fun blankDestinationOmitted() {
        val s = ConnectionLogState()
        assertEquals(
            "c.com | via proxy | status: connected",
            s.onConnection("c.com", "", "proxy"),
        )
    }

    @Test fun resetAllowsRelog() {
        val s = ConnectionLogState()
        assertEquals(
            "d.com -> 1.1.1.1:443 | via proxy | status: connected",
            s.onConnection("d.com", "1.1.1.1:443", "proxy"),
        )
        assertNull(s.onConnection("d.com", "1.1.1.1:443", "proxy"))
        s.reset()
        assertEquals(
            "d.com -> 1.1.1.1:443 | via proxy | status: connected",
            s.onConnection("d.com", "1.1.1.1:443", "proxy"),
        )
    }

    @Test fun capStopsLoggingBeyondLimit() {
        val s = ConnectionLogState(cap = 3)
        assertEquals("h1 | via proxy | status: connected", s.onConnection("h1", "", "proxy"))
        assertEquals("h2 | via proxy | status: connected", s.onConnection("h2", "", "proxy"))
        assertEquals("h3 | via proxy | status: connected", s.onConnection("h3", "", "proxy"))
        // 4th unique host exceeds the cap → dropped to bound memory/log volume.
        assertNull(s.onConnection("h4", "", "proxy"))
    }
}
