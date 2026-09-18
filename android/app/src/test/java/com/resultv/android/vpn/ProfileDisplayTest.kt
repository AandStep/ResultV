package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Имя и бейджи строки сервера — перенос `formatProxyDisplayName` и
 * `getProtocolLabel` с ПК (`ResultVPC/frontend/src/utils/proxyParser.js`).
 * Обе функции чистые, поэтому проверяются на голой JVM.
 */
class ProfileDisplayTest {

    // ---- имя ----

    @Test fun stripsLeadingFlagEmoji() {
        assertEquals("Netherlands 01", serverDisplayName("🇳🇱 Netherlands 01", "NL"))
    }

    @Test fun leavesNameWithoutFlagAlone() {
        assertEquals("Netherlands 01", serverDisplayName("Netherlands 01", "NL"))
    }

    @Test fun stripsLeadingCountryCodeWhenItMatches() {
        assertEquals("Amsterdam", serverDisplayName("🇳🇱 NL Amsterdam", "nl"))
    }

    // Код страны снимается только целым словом: «NLD» — начало имени, а не код.
    @Test fun keepsCountryCodeGluedToAWord() {
        assertEquals("NLD Server", serverDisplayName("NLD Server", "NL"))
    }

    // Если после чистки не осталось ничего — возвращаем исходное имя, иначе
    // строка сервера стала бы пустой.
    @Test fun keepsOriginalWhenNothingIsLeft() {
        val onlyFlag = "🇳🇱"
        assertEquals(onlyFlag, serverDisplayName(onlyFlag, "NL"))
    }

    @Test fun unknownCountryStripsFlagOnly() {
        assertEquals("NL Amsterdam", serverDisplayName("🇳🇱 NL Amsterdam", null))
    }

    // Имя из одних пробелов возвращается как есть: чистить нечего, а пустая
    // строка на его месте выглядела бы поломкой списка.
    @Test fun blankNameIsReturnedUnchanged() {
        assertEquals("   ", serverDisplayName("   ", "NL"))
    }

    // ---- бейджи ----

    private fun profile(entry: String) = Profile.fromEntryJson("srv", entry)

    @Test fun bareTypeWithoutExtraGivesOneBadge() {
        val p = profile("""{"type":"VLESS","ip":"1.2.3.4","port":443}""")
        assertEquals(listOf("VLESS"), p.badges)
    }

    @Test fun realitySecurityAddsSecondBadge() {
        val p = profile("""{"type":"VLESS","extra":{"security":"reality"}}""")
        assertEquals(listOf("VLESS", "Reality"), p.badges)
    }

    @Test fun tlsAndGrpcGiveThreeBadges() {
        val p = profile("""{"type":"VLESS","extra":{"security":"tls","network":"grpc"}}""")
        assertEquals(listOf("VLESS", "TLS", "gRPC"), p.badges)
    }

    // Голова приводится к написанию макета, хвост уже в нужном виде.
    @Test fun headTakesTheDesignSpelling() {
        val p = profile("""{"type":"SS"}""")
        assertEquals(listOf("Shadowsocks"), p.badges)
    }

    // Подписки иногда отдают extra строкой, а не объектом — как на ПК.
    @Test fun extraAsAStringIsParsedToo() {
        val p = profile("""{"type":"VMESS","extra":"{\"network\":\"ws\"}"}""")
        assertEquals(listOf("VMess", "WS"), p.badges)
    }

    // У авто-группы бейдж рисует строка ресурсов, а не профиль.
    @Test fun autoGroupHasNoOwnBadges() {
        val p = profile("""{"type":"AUTO"}""")
        assertEquals(emptyList<String>(), p.badges)
    }

    @Test fun sectionHasNoBadges() {
        assertEquals(emptyList<String>(), Profile.section("Выберите ниже", "sub1").badges)
    }

    // tcp — это «ничего особенного», отдельного бейджа у него нет.
    @Test fun plainTcpAddsNothing() {
        val p = profile("""{"type":"TROJAN","extra":{"security":"none","network":"tcp"}}""")
        assertEquals(listOf("Trojan"), p.badges)
    }
}
