package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class RoutingEditorFormTest {

    // Правила правятся текстом, одно правило на строку — как на ПК
    // (linesOf / tokensOf в RoutingProfileEditor.jsx).
    @Test fun linesRoundTrip() {
        val list = listOf("geosite:private", "example.com", "domain:nalog.ru")
        assertEquals(list, routingTokensOf(routingLinesOf(list)))
    }

    @Test fun tokensIgnoreBlankAndWhitespace() {
        val text = "  example.com  \n\n\t\n  10.0.0.0/8\n"
        assertEquals(listOf("example.com", "10.0.0.0/8"), routingTokensOf(text))
    }

    // Вставка списка с CRLF — обычное дело, если его скопировали из письма или
    // с сайта. Хвостовой \r превратил бы каждый токен в мусор.
    @Test fun tokensSurviveCarriageReturns() {
        assertEquals(
            listOf("example.com", "other.example"),
            routingTokensOf("example.com\r\nother.example\r\n"),
        )
    }

    @Test fun emptyTextGivesEmptyList() {
        assertTrue(routingTokensOf("").isEmpty())
        assertTrue(routingTokensOf("   \n  \n").isEmpty())
    }

    @Test fun linesOfEmptyListIsEmptyText() {
        assertEquals("", routingLinesOf(emptyList()))
    }

    // Порядок правил — три метки через дефис, ровно как ждёт Go
    // (NormalizeRoutingOrder).
    @Test fun orderJoinsWithDashes() {
        assertEquals("block-proxy-direct", normalizeRouteOrder(listOf("block", "proxy", "direct")))
        assertEquals("direct-proxy-block", normalizeRouteOrder(listOf("direct", "proxy", "block")))
    }

    // Неполный или повторяющийся набор — не порядок. Пустая строка означает
    // «умолчание», и Go подставит своё, а не будет гадать: порядок решает,
    // какое правило выигрывает при нескольких совпадениях.
    @Test fun brokenOrderBecomesEmpty() {
        assertEquals("", normalizeRouteOrder(listOf("block", "block", "direct")))
        assertEquals("", normalizeRouteOrder(listOf("block", "proxy")))
        assertEquals("", normalizeRouteOrder(emptyList()))
        assertEquals("", normalizeRouteOrder(listOf("block", "proxy", "чепуха")))
    }
}
