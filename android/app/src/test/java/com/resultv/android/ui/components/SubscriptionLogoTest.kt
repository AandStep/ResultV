package com.resultv.android.ui.components

import com.resultv.android.vpn.Subscription
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SubscriptionLogoTest {
    @Test
    fun recognizesOurSubscriptionServiceAndExistingOfficialDomain() {
        for (url in listOf(
            "https://result-service.ru/s/example",
            "https://RESULT-SERVICE.RU/s/example",
            "https://result-proxy.ru/s/example",
            "https://sub.result-proxy.ru/s/example",
        )) {
            assertTrue(url, isOfficialSubscription(Subscription("saved", url, "Subscription")))
        }
    }

    @Test
    fun rejectsSimilarHostsAndCleartextLinks() {
        for (url in listOf(
            "https://result-service.ru.example.net/s/example",
            "https://result-service.ru@example.net/s/example",
            "https://other-result-service.ru/s/example",
            "http://result-service.ru/s/example",
        )) {
            assertFalse(url, isOfficialSubscription(Subscription("saved", url, "Subscription")))
        }
    }
}
