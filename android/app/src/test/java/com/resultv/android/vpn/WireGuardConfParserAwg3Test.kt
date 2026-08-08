package com.resultv.android.vpn

import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * AmneziaWG 3.0 fields in an imported `.conf`.
 *
 * These knobs cannot reach the tunnel yet — sing-box-extended 2.6.0 bumped
 * wireguard-go to extended-1.5.0 but left `option.WireGuardAmnezia` at the
 * AWG 2.0 set — but the handshake probe builds its own UAPI and does honour
 * them. Dropping them at import time would break ping against an AWG 3.0
 * server, so the parser has to carry them through to the `awg://` URI.
 */
class WireGuardConfParserAwg3Test {

    private val awg3Conf = """
        [Interface]
        PrivateKey = aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=
        Address = 10.8.1.2/24
        Jc = 8
        S1 = 15
        S2 = 15
        HeaderProtectionKey = AwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwM=
        ContentPaddingAddition = 10-50
        RekeyAfterTime = 120
        RekeyTimeout = 5
        RejectAfterTime = 180
        KeepaliveTimeout = 10
        MaxHandshakeAttempts = 18

        [Peer]
        PublicKey = WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=
        Endpoint = 203.0.113.7:51820
        AllowedIPs = 0.0.0.0/0
    """.trimIndent()

    @Test fun awg3FieldsSurviveIntoTheUri() {
        val uri = WireGuardConfParser.toUri(awg3Conf)
        assertNotNull(uri)
        uri!!

        // Go's parser looks these up snake_cased (see awg3DeviceKnobs).
        listOf(
            "header_protection_key=",
            "content_padding_addition=",
            "rekey_after_time=",
            "rekey_timeout=",
            "reject_after_time=",
            "keepalive_timeout=",
            "max_handshake_attempts=",
        ).forEach { key ->
            assertTrue("missing $key in $uri", uri.contains(key))
        }
        // The range form must not be mangled by URL encoding into something
        // UintRange.FromString rejects.
        assertTrue(uri, uri.contains("content_padding_addition=10-50"))
    }

    /** A conf carrying only AWG 3.0 knobs is still AmneziaWG, not plain WireGuard. */
    @Test fun awg3OnlyConfIsDetectedAsAmnezia() {
        val conf = """
            [Interface]
            PrivateKey = aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=
            Address = 10.8.1.2/24
            HeaderProtectionKey = AwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwM=

            [Peer]
            PublicKey = WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=
            Endpoint = 203.0.113.7:51820
            AllowedIPs = 0.0.0.0/0
        """.trimIndent()

        val uri = WireGuardConfParser.toUri(conf)
        assertNotNull(uri)
        assertTrue(uri!!, uri.startsWith("awg://"))
    }

    /** A plain WireGuard conf must stay plain — no empty AWG 3.0 params bolted on. */
    @Test fun plainWireGuardConfIsUnaffected() {
        val conf = """
            [Interface]
            PrivateKey = aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=
            Address = 10.8.1.2/24

            [Peer]
            PublicKey = WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=
            Endpoint = 203.0.113.7:51820
            AllowedIPs = 0.0.0.0/0
        """.trimIndent()

        val uri = WireGuardConfParser.toUri(conf)
        assertNotNull(uri)
        uri!!
        assertTrue(uri, uri.startsWith("wg://"))
        assertFalse(uri, uri.contains("header_protection_key"))
        assertFalse(uri, uri.contains("rekey_after_time"))
    }
}
