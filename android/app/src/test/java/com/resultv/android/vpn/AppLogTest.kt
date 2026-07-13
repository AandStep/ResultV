package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class AppLogTest {

    @Test fun resourceLoggingBeforeInitIsDroppedSilently() {
        AppLog.clear()
        // Не инициализирован (юнит-тест) — вызов не должен ни падать, ни писать.
        AppLog.info(42, "arg", source = "TEST")
        AppLog.error(43)
        assertTrue(AppLog.entries.value.isEmpty())
    }

    @Test fun resolveBeforeInitReturnsEmpty() {
        assertEquals("", AppLog.resolve(42))
    }

    @Test fun stringLoggingStillAddsNewestFirst() {
        AppLog.clear()
        AppLog.info("first")
        AppLog.error("second", source = "X")
        val e = AppLog.entries.value
        assertEquals(2, e.size)
        assertEquals("second", e[0].message)
        assertEquals(LogLevel.Error, e[0].level)
        assertEquals("X", e[0].source)
    }
}
