package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

class BrowserAdBlockRetryTest {

    // Первая попытка — самая дешёвая: короткая заминка сети лечится сама.
    @Test fun firstRetryComesQuickly() {
        assertEquals(5_000L, certSelfTestRetryDelayMs(1))
    }

    // Вторая ждёт дольше: если и через 5 с не вышло, скорее всего туннель
    // занят загрузкой фильтр-листов, а она измеряется десятками секунд.
    @Test fun secondRetryWaitsLonger() {
        val first = certSelfTestRetryDelayMs(1)!!
        val second = certSelfTestRetryDelayMs(2)!!
        assertEquals(15_000L, second)
        assertEquals(true, second > first)
    }

    // Каждая попытка занимает worker сервиса, поэтому их число ограничено.
    @Test fun givesUpAfterMaxAttempts() {
        assertNotNull(certSelfTestRetryDelayMs(CERT_SELF_TEST_MAX_ATTEMPTS - 1))
        assertNull(certSelfTestRetryDelayMs(CERT_SELF_TEST_MAX_ATTEMPTS))
        assertNull(certSelfTestRetryDelayMs(CERT_SELF_TEST_MAX_ATTEMPTS + 10))
    }
}
