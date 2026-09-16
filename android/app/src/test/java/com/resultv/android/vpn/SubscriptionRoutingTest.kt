package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class SubscriptionRoutingTest {

    // Go отдаёт профиль без subscriptionId: его знает только Kotlin. Без
    // простановки следующая синхронизация не узнает свой профиль и заведёт
    // второй.
    @Test fun tagsProfileWithSubscriptionId() {
        val p = tagSubscriptionProfile(
            """{"name":"impVPN","source":"subscription","proxySites":["a.example"]}""",
            subId = "sub1",
            subName = "impVPN",
        )!!
        assertEquals("sub1", p.subscriptionId)
        assertEquals("subscription", p.source)
    }

    // Имя подписки пользователь мог поменять, а OriginName — опознавательный
    // знак, и он должен остаться тем, что прислал провайдер.
    @Test fun keepsOriginNameFromGo() {
        val p = tagSubscriptionProfile(
            """{"name":"impVPN","originName":"impVPN","source":"subscription","proxySites":["a.example"]}""",
            subId = "sub1",
            subName = "Моя подписка",
        )!!
        assertEquals("impVPN", p.originName)
    }

    // Профиль без опознавательного знака дополняется именем подписки: иначе
    // SameRoutingProfile не с чем сравнивать, и любой безымянный профиль
    // совпал бы с любым другим безымянным.
    @Test fun fillsOriginNameWhenGoLeftItEmpty() {
        val p = tagSubscriptionProfile(
            """{"name":"","source":"subscription","proxySites":["a.example"]}""",
            subId = "sub1",
            subName = "impVPN",
        )!!
        assertEquals("impVPN", p.originName)
        assertEquals("impVPN", p.name)
    }

    @Test fun emptyOrBrokenJsonIsNull() {
        assertNull(tagSubscriptionProfile("", "sub1", "S"))
        assertNull(tagSubscriptionProfile("   ", "sub1", "S"))
        assertNull(tagSubscriptionProfile("не json", "sub1", "S"))
    }

    // Профиль без единого правила принимать нельзя: он занял бы строку в
    // списке, предложил себя включить и ничего бы не маршрутизировал.
    @Test fun profileWithoutRulesIsNull() {
        assertNull(tagSubscriptionProfile("""{"name":"S","source":"subscription"}""", "sub1", "S"))
    }

    // Ссылка на список — тоже правило: качать её будет сборка, но профиль с
    // одной только ссылкой уже осмысленный.
    @Test fun profileWithOnlyListUrlsIsAccepted() {
        val p = tagSubscriptionProfile(
            """{"name":"S","source":"subscription",
                "listUrls":{"proxy":["https://panel.example/l.txt"]}}""",
            subId = "sub1",
            subName = "S",
        )!!
        assertEquals(1, p.ruleCount("proxy"))
    }
}
