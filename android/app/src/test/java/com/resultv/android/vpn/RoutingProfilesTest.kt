package com.resultv.android.vpn

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class RoutingProfilesTest {

    // Имена ключей — единственное, что связывает Kotlin с config.RoutingProfile
    // в Go. Переименуй любое, и поле молча потеряется на круге через хранилище.
    @Test fun jsonRoundTripKeepsEveryField() {
        val p = RoutingProfile(
            id = "abc123",
            name = "Моя маршрутизация",
            directSites = listOf("direct.example"),
            directIp = listOf("10.0.0.0/8"),
            proxySites = listOf("proxy.example"),
            proxyIp = listOf("1.2.3.0/24"),
            blockSites = listOf("block.example"),
            blockIp = listOf("5.6.7.0/24"),
            routeOrder = "block-proxy-direct",
            domainStrategy = "IPIfNonMatch",
            geoipUrl = "https://panel.example/geoip.dat",
            geositeUrl = "https://panel.example/geosite.dat",
            listUrls = mapOf("proxy" to listOf("https://panel.example/l.txt")),
            allowInsecure = true,
            source = "deeplink",
            subscriptionId = "sub1",
            originName = "Panel A",
            updatedAt = 1788322632L,
            lastError = "что-то пошло не так",
        )
        val back = routingProfileFromJson(JSONObject(p.toJson().toString()))
        assertEquals(p, back)
    }

    @Test fun jsonUsesTheGoFieldNames() {
        val p = RoutingProfile(id = "a", name = "N", proxySites = listOf("x.example"))
        val raw = p.toJson().toString()
        for (key in listOf("id", "name", "proxySites", "directIp", "blockIp", "originName")) {
            assertTrue("нет ключа $key в $raw", raw.contains("\"$key\""))
        }
    }

    @Test fun missingFieldsDecodeToEmptyNotCrash() {
        val p = routingProfileFromJson(JSONObject("""{"id":"a","name":"N"}"""))
        assertEquals("a", p.id)
        assertTrue(p.directSites.isEmpty())
        assertTrue(p.listUrls.isEmpty())
        assertEquals("", p.routeOrder)
        assertEquals(0L, p.updatedAt)
    }

    // Счётчик для строки «• 12 direct • 3 block». Ссылка на список считается за
    // единицу: сколько правил за ней, неизвестно, пока её не скачали.
    @Test fun ruleCountCountsTokensAndLinks() {
        val p = RoutingProfile(
            id = "a", name = "N",
            directSites = listOf("a.example", "b.example"),
            directIp = listOf("10.0.0.0/8"),
            proxySites = listOf("c.example"),
            listUrls = mapOf("proxy" to listOf("https://panel.example/l.txt")),
        )
        assertEquals(3, p.ruleCount("direct"))
        assertEquals(2, p.ruleCount("proxy"))
        assertEquals(0, p.ruleCount("block"))
        assertEquals(0, p.ruleCount("чепуха"))
    }

    // Издателя показываем только когда он отличается от того, что видно:
    // иначе строка «Издатель: Х» под заголовком «Х» — просто шум.
    @Test fun publisherShownOnlyWhenRenamed() {
        assertEquals("", RoutingProfile(id = "a", name = "X", originName = "X").publisherName)
        assertEquals("", RoutingProfile(id = "a", name = "X").publisherName)
        assertEquals("Panel A", RoutingProfile(id = "a", name = "Моё", originName = "Panel A").publisherName)
    }

    @Test fun storeRoundTripKeepsActiveId() {
        val s = RoutingProfilesState(
            profiles = listOf(
                RoutingProfile(id = "a", name = "A", proxySites = listOf("a.example")),
                RoutingProfile(id = "b", name = "B", proxySites = listOf("b.example")),
            ),
            activeId = "b",
        )
        assertEquals(s, decodeRoutingProfiles(encodeRoutingProfiles(s)))
    }

    // Форма хранилища обязана совпадать с тем, что ждёт и отдаёт
    // Mobile.mergeRoutingProfile: {"profiles":[...],"activeId":"..."}.
    @Test fun storeJsonHasTheShapeGoExpects() {
        val raw = encodeRoutingProfiles(
            RoutingProfilesState(listOf(RoutingProfile(id = "a", name = "A")), "a")
        )
        val o = JSONObject(raw)
        assertTrue(o.has("profiles"))
        assertTrue(o.has("activeId"))
        assertEquals(1, o.getJSONArray("profiles").length())
    }

    @Test fun brokenStoreDecodesToEmptyInsteadOfThrowing() {
        assertEquals(RoutingProfilesState(), decodeRoutingProfiles("не json"))
        assertEquals(RoutingProfilesState(), decodeRoutingProfiles(""))
    }

    // Активным не может остаться профиль, которого нет: строка списка была бы
    // без выделения, а движок получил бы id, по которому нет ни одного файла.
    @Test fun activeIdIsDroppedWhenProfileIsGone() {
        val s = decodeRoutingProfiles(
            """{"profiles":[{"id":"a","name":"A"}],"activeId":"ghost"}"""
        )
        assertEquals("", s.activeId)
    }

    @Test fun activeReturnsTheProfileInForce() {
        val s = RoutingProfilesState(
            profiles = listOf(
                RoutingProfile(id = "a", name = "A"),
                RoutingProfile(id = "b", name = "B"),
            ),
            activeId = "b",
        )
        assertEquals("B", s.active?.name)
        assertEquals(null, s.copy(activeId = "").active)
    }
}
