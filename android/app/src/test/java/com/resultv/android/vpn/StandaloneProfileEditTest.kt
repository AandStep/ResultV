package com.resultv.android.vpn

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assert.assertThrows
import org.junit.Test

class StandaloneProfileEditTest {
    @Test
    fun savedEntryCanBeEditedOnlyAsPersonalServer() {
        val profile = Profile.fromEntryJson("Proxy", """{"type":"SOCKS5","ip":"192.0.2.5","port":1080}""")
        assertTrue(profile.canEditConnection())
        assertFalse(profile.copy(subscriptionId = "subscription").canEditConnection())
        assertFalse(Profile.section("Section", "").canEditConnection())
    }

    @Test
    fun proxyEditUriPreservesIpv6AndEscapesCredentials() {
        assertEquals(
            "socks5://my%3Auser:p%40ss%20word@[2001:db8::5]:8000",
            standardProxyEditUri("SOCKS5", "2001:db8::5", "8000", "my:user", "p@ss word"),
        )
    }

    @Test
    fun changingConnectionKeepsOtherSettingsAndClearsOldCountry() {
        val saved = JSONObject("""{"type":"HTTP","ip":"192.0.2.5","port":80,"country":"nl","extra":{"key":42}}""")
        val parsed = JSONObject("""{"type":"HTTPS","ip":"192.0.2.6","port":443,"username":"user","password":"pass","country":""}""")
        val edited = applyStandardProxyEdit(saved, parsed, "Edited")
        assertEquals("Edited", edited.getString("name"))
        assertEquals("HTTPS", edited.getString("type"))
        assertEquals(443, edited.getInt("port"))
        assertEquals("pass", edited.getString("password"))
        assertEquals(42, edited.getJSONObject("extra").getInt("key"))
        assertEquals("", edited.getString("country"))
        assertEquals("nl", saved.getString("country"))
    }

    @Test
    fun rawConfigRetainsProtocolSettingsAndRejectsUnknownProtocol() {
        val raw = """{"type":"vless","ip":"example.test","port":443,"extra":{"flow":"xtls-rprx-vision"}}"""
        val result = applyRawProxyEntryEdit(null, raw, "Server")
        assertEquals("VLESS", result.getString("type"))
        assertEquals("xtls-rprx-vision", result.getJSONObject("extra").getString("flow"))
        assertThrows(IllegalArgumentException::class.java) {
            applyRawProxyEntryEdit(null, raw.replace("vless", "invalid"), "Server")
        }
        assertThrows(IllegalArgumentException::class.java) {
            applyRawProxyEntryEdit(null, raw.replace("443", "443.5"), "Server")
        }
    }
}
