package com.resultv.android.vpn

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Имена ключей в optionsJson — контракт с Go (BuildOptions). Опечатка здесь
 * молча выключает фичу: неизвестное поле JSON Go просто не заполнит, конфиг
 * соберётся без реле, и понять это можно будет только по отсутствию строки в
 * логе.
 */
class AdaptiveSmartOptionsTest {

    @Test
    fun `ключи адаптивного Smart совпадают с ожидаемыми Go`() {
        val json = JSONObject()
            .put("adaptiveSmart", true)
            .put("adaptiveSmartMemoryOnly", false)

        assertTrue(json.getBoolean("adaptiveSmart"))
        assertEquals(false, json.getBoolean("adaptiveSmartMemoryOnly"))
    }

    @Test
    fun `состояние по умолчанию выключено`() {
        val state = SettingsState()
        assertEquals(false, state.adaptiveSmart)
        assertEquals(false, state.adaptiveSmartMemoryOnly)
    }
}
