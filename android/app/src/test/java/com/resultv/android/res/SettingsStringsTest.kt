package com.resultv.android.res

import org.junit.Assert.assertEquals
import org.junit.Test
import java.io.File

/**
 * Тексты настроек проверяются по файлу ресурсов, а не через `R.string`:
 * юнит-тесты модуля идут на голой JVM без Robolectric, и разрешать
 * идентификаторы ресурсов здесь нечем. XML разбирается регуляркой — файл
 * плоский, настоящий парсер был бы тяжелее задачи.
 *
 * Оба теста сужены до префикса `settings_`. Вне его паритет локалей УЖЕ
 * нарушен: `awg_*` (имена полей AmneziaWG) и `log_source_*` лежат только в
 * английском файле. Это не следствие этой правки, и расширять охват здесь
 * значит красить чужой забор.
 */
class SettingsStringsTest {

    private companion object {
        /**
         * Одна строка подписи — примерно 32 символа при 13sp на экране
         * 360 dp. Две строки, то есть предел из спеки, — 64.
         */
        const val MAX_LEN = 64

        val SUFFIXES = listOf("_desc", "_subtitle", "_hint", "_items", "_warning")

        val EN_RU = listOf(
            "main/res/values/strings.xml" to "main/res/values-ru/strings.xml",
            "full/res/values/strings.xml" to "full/res/values-ru/strings.xml",
        )

        val STRING_RE = Regex(
            """<string name="([^"]+)"[^>]*>(.*?)</string>""",
            RegexOption.DOT_MATCHES_ALL,
        )
        val TAG_RE = Regex("<[^>]+>")
    }

    /**
     * Рабочий каталог юнит-теста задаёт Gradle, и у AGP это каталог модуля
     * (`android/app`). Запуск из IDE иногда стартует от корня проекта,
     * поэтому проверяются оба варианта.
     */
    private fun resFile(relative: String): File =
        listOf(File("src/$relative"), File("app/src/$relative"), File("android/app/src/$relative"))
            .firstOrNull { it.isFile }
            ?: error("не найден файл ресурсов: $relative")

    private fun strings(relative: String): Map<String, String> =
        STRING_RE.findAll(resFile(relative).readText())
            .associate { it.groupValues[1] to it.groupValues[2] }

    private fun settingsNames(relative: String): Set<String> =
        strings(relative).keys.filterTo(mutableSetOf()) { it.startsWith("settings_") }

    @Test fun settingsCopyFitsTwoLines() {
        val tooLong = EN_RU.flatMap { listOf(it.first, it.second) }.flatMap { file ->
            strings(file)
                .filterKeys { name ->
                    name.startsWith("settings_") && SUFFIXES.any { name.endsWith(it) }
                }
                .mapNotNull { (name, raw) ->
                    val text = TAG_RE.replace(raw, "")
                    if (text.length > MAX_LEN) "$file · $name · ${text.length}" else null
                }
        }
        assertEquals("строки длиннее $MAX_LEN символов", emptyList<String>(), tooLong)
    }

    @Test fun bothLocalesKnowTheSameSettingsStrings() {
        val gaps = EN_RU.flatMap { (en, ru) ->
            val onlyEn = settingsNames(en) - settingsNames(ru)
            val onlyRu = settingsNames(ru) - settingsNames(en)
            onlyEn.map { "$en · только en · $it" } + onlyRu.map { "$ru · только ru · $it" }
        }
        assertEquals("набор settings_* разошёлся между локалями", emptyList<String>(), gaps)
    }
}
