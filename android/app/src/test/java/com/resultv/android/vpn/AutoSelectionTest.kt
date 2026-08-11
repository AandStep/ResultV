package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class AutoSelectionTest {

    private val payload = """
        {"candidates":[
          {"key":"k1","name":"NL-1","rttMs":41,"entry":{"ip":"203.0.113.1","port":443,"type":"VLESS"}},
          {"key":"k2","name":"NL-2","rttMs":58,"entry":{"ip":"203.0.113.2","port":443,"type":"VLESS"}}
        ]}
    """.trimIndent()

    @Test
    fun `parses candidates in rank order`() {
        val got = AutoSelection.parseCandidates(payload)
        assertEquals(listOf("k1", "k2"), got.map { it.key })
        assertEquals(41, got[0].rttMs)
        // entry must survive as raw JSON the Go builder can consume
        assertTrue(got[0].entryJson.contains("\"203.0.113.1\""))
    }

    @Test
    fun `malformed payload yields an empty list rather than throwing`() {
        assertTrue(AutoSelection.parseCandidates("not json").isEmpty())
        assertTrue(AutoSelection.parseCandidates("""{"candidates":null}""").isEmpty())
    }

    @Test
    fun `advance walks the ranked list and stops at the end`() {
        AutoSelection.installForTest("p1", AutoSelection.parseCandidates(payload))
        assertEquals("k1", AutoSelection.current()?.key)
        assertEquals("k2", AutoSelection.advance()?.key)
        assertNull(AutoSelection.advance())
    }

    @Test
    fun `advance stops at MAX_ATTEMPTS even when more ranked candidates remain`() {
        // Six candidates, one more than MAX_ATTEMPTS (5) — installed directly
        // via installForTest so no JSON parsing is involved, only the cap.
        val sixCandidates = (1..6).map {
            AutoSelection.Candidate(key = "k$it", name = "N$it", rttMs = it * 10, entryJson = "{}")
        }
        AutoSelection.installForTest("p1", sixCandidates)
        assertEquals("k1", AutoSelection.current()?.key)
        assertEquals("k2", AutoSelection.advance()?.key)
        assertEquals("k3", AutoSelection.advance()?.key)
        assertEquals("k4", AutoSelection.advance()?.key)
        assertEquals("k5", AutoSelection.advance()?.key)
        // Cap reached at the 5th candidate: k6 is still in the ranked list but
        // must never be handed out — this is the bound that keeps a group of
        // dead nodes from turning one connect into a long retry storm.
        assertNull(AutoSelection.advance())
        assertEquals("k5", AutoSelection.current()?.key)
    }

    @Test
    fun `reset clears the selection`() {
        AutoSelection.installForTest("p1", AutoSelection.parseCandidates(payload))
        AutoSelection.reset()
        assertNull(AutoSelection.current())
    }

    @Test
    fun `a different profile invalidates the cached selection`() {
        AutoSelection.installForTest("p1", AutoSelection.parseCandidates(payload))
        assertNull(AutoSelection.currentFor("p2"))
        assertEquals("k1", AutoSelection.currentFor("p1")?.key)
    }
}
