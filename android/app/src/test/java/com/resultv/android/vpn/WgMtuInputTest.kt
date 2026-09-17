package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Поле MTU — свободный ввод, а значение уезжает в движок. Границы те же, что
 * в Go (`internal/proxy/endpoints.go`, `wireguardMTU`): ниже 576 путь IPv4 не
 * обязан нести ничего, выше 1500 переопределение само создаст ту проблему,
 * ради проверки которой заведено. Пустое и негодное читаются как «из
 * профиля», а не как ноль-значение MTU.
 */
class WgMtuInputTest {

    @Test
    fun `empty means take it from the profile`() {
        assertEquals(0, SettingsRepository.normalizeWgMtu(""))
        assertEquals(0, SettingsRepository.normalizeWgMtu("   "))
    }

    @Test
    fun `garbage is not a number`() {
        assertEquals(0, SettingsRepository.normalizeWgMtu("тысяча"))
        assertEquals(0, SettingsRepository.normalizeWgMtu("12a8"))
    }

    @Test
    fun `out of range is ignored`() {
        assertEquals(0, SettingsRepository.normalizeWgMtu("500"))
        assertEquals(0, SettingsRepository.normalizeWgMtu("9000"))
    }

    @Test
    fun `boundaries are accepted`() {
        assertEquals(576, SettingsRepository.normalizeWgMtu("576"))
        assertEquals(1500, SettingsRepository.normalizeWgMtu("1500"))
        assertEquals(1280, SettingsRepository.normalizeWgMtu(" 1280 "))
    }
}
