package com.resultv.android.theme

enum class AppearanceTheme(val preference: String) {
    Dark("dark"),
    Light("light"),
    System("system");

    fun isDark(systemDark: Boolean): Boolean = when (this) {
        Dark -> true
        Light -> false
        System -> systemDark
    }

    companion object {
        fun fromPreference(preference: String?): AppearanceTheme =
            entries.firstOrNull { it.preference == preference } ?: Dark
    }
}
