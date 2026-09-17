package com.resultv.android.vpn

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Настройки пинга нормализуются и на стороне Kotlin, хотя настоящая проверка
 * живёт в Go. Причина не в недоверии: поле ввода здесь, и краснеть оно должно
 * до нажатия, а не после того, как весь список показал «URL».
 *
 * Границы и правило «только https» те же, что в `internal/config`: один и тот
 * же параметр не может значить в двух местах разное.
 */
class PingOptionsTest {

    @Test
    fun `timeout outside the range falls back to the default`() {
        assertEquals(0, SettingsRepository.normalizePingTimeoutSec(""))
        assertEquals(0, SettingsRepository.normalizePingTimeoutSec("   "))
        assertEquals(0, SettingsRepository.normalizePingTimeoutSec("секунда"))
        assertEquals(0, SettingsRepository.normalizePingTimeoutSec("0"))
        assertEquals(0, SettingsRepository.normalizePingTimeoutSec("11"))
        assertEquals(1, SettingsRepository.normalizePingTimeoutSec("1"))
        assertEquals(10, SettingsRepository.normalizePingTimeoutSec("10"))
        assertEquals(4, SettingsRepository.normalizePingTimeoutSec(" 4 "))
    }

    @Test
    fun `only https urls are accepted`() {
        assertTrue(SettingsRepository.isValidPingTestUrl("https://www.gstatic.com/generate_204"))
        assertTrue(SettingsRepository.isValidPingTestUrl(" https://example.org/ping "))
        assertFalse(SettingsRepository.isValidPingTestUrl(""))
        assertFalse(SettingsRepository.isValidPingTestUrl("http://example.org/ping"))
        assertFalse(SettingsRepository.isValidPingTestUrl("https://"))
        assertFalse(SettingsRepository.isValidPingTestUrl("example.org"))
    }

    /**
     * Ключи совпадают с именами в общем конфиге — по ним же биндинг и читает.
     * Пустые значения не отправляются вовсе: отсутствие ключа означает «как по
     * умолчанию», и это ровно то, что нужно сказать.
     */
    @Test
    fun `options json carries the settings under the shared names`() {
        val json = JSONObject(SettingsRepository.pingOptionsJson("http_head", "https://example.org/p", 7))
        assertEquals("http_head", json.getString("pingType"))
        assertEquals("https://example.org/p", json.getString("pingTestUrl"))
        assertEquals(7, json.getInt("pingTimeoutSec"))

        val empty = JSONObject(SettingsRepository.pingOptionsJson("auto", "", 0))
        assertFalse(empty.has("pingTestUrl"))
        assertFalse(empty.has("pingTimeoutSec"))
        assertEquals("auto", empty.getString("pingType"))
    }
}
