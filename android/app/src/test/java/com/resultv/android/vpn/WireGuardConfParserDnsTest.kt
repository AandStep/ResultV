package com.resultv.android.vpn

import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Строка DNS из `[Interface]` доезжает до ссылки параметром `dns`: оттуда Go
 * кладёт её в extra.dns_servers, и она перекрывает пресет из настроек — как на
 * ПК. Раньше импорт её молча выбрасывал.
 */
class WireGuardConfParserDnsTest {

    private fun conf(dnsLine: String, amnezia: Boolean) = buildString {
        appendLine("[Interface]")
        appendLine("PrivateKey = aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=")
        appendLine("Address = 10.8.1.2/24")
        if (dnsLine.isNotEmpty()) appendLine(dnsLine)
        if (amnezia) appendLine("Jc = 8")
        appendLine()
        appendLine("[Peer]")
        appendLine("PublicKey = WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=")
        appendLine("Endpoint = 203.0.113.7:51820")
    }

    @Test fun dnsSurvivesIntoTheUri() {
        for (amnezia in listOf(false, true)) {
            val uri = WireGuardConfParser.toUri(conf("DNS = 10.8.0.1, 1.1.1.1", amnezia))
            assertNotNull(uri)
            assertTrue(uri!!, uri.contains("dns=10.8.0.1%2C+1.1.1.1"))
        }
    }

    @Test fun noDnsLineMeansNoParam() {
        val uri = WireGuardConfParser.toUri(conf("", amnezia = true))
        assertNotNull(uri)
        assertFalse(uri!!, uri.contains("dns="))
    }
}
