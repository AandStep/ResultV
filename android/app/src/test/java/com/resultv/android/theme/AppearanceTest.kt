package com.resultv.android.theme

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.compositeOver
import androidx.compose.ui.graphics.luminance
import com.resultv.android.ui.components.HomeLook
import com.resultv.android.ui.components.protocolBadgeColors
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class AppearanceTest {
    private fun contrast(first: Color, second: Color): Float {
        val a = first.luminance() + 0.05f
        val b = second.luminance() + 0.05f
        return maxOf(a, b) / minOf(a, b)
    }

    @Test fun darkRemainsDefaultForExistingInstalls() {
        assertEquals(AppearanceTheme.Dark, AppearanceTheme.fromPreference(null))
        assertEquals(AppearanceTheme.Dark, AppearanceTheme.fromPreference("unexpected"))
    }

    @Test fun preferenceRoundTripsAndSystemThemeTracksPlatform() {
        AppearanceTheme.entries.forEach { theme ->
            assertEquals(theme, AppearanceTheme.fromPreference(theme.preference))
        }
        assertTrue(AppearanceTheme.Dark.isDark(systemDark = false))
        assertFalse(AppearanceTheme.Light.isDark(systemDark = true))
        assertTrue(AppearanceTheme.System.isDark(systemDark = true))
        assertFalse(AppearanceTheme.System.isDark(systemDark = false))
    }

    @Test fun lightPaletteUsesDesktopSurfacesAndReadableForeground() {
        val light = paletteFor(dark = false)
        assertEquals(Color(0xFFE6E6E6), light.Black)
        assertEquals(Color(0xFFFFFFFF), light.Grey)
        assertEquals(Color(0xFFF2F2F2), light.ServerList)
        assertEquals(Color(0xFFE5E5E5), light.ActiveServer)
        assertEquals(Color(0xFF161A17), light.White)
        val secondaryText = light.description.compositeOver(light.Grey)
        val contrast = (light.Grey.luminance() + 0.05f) / (secondaryText.luminance() + 0.05f)
        assertTrue("secondary text contrast: $contrast", contrast >= 4.5f)
        assertEquals(Color.White, light.OnAccent)
    }

    @Test fun lightSettingsIconsHaveContrastOverTheirTiles() {
        val light = paletteFor(dark = false)
        listOf(RvCategory.Main, RvCategory.Blue, RvCategory.Red, RvCategory.Amber,
            RvCategory.Violet, RvCategory.Slate, RvCategory.Cyan, RvCategory.Emerald).forEach { tint ->
            val tile = tint.tile.compositeOver(light.Grey)
            val glyph = tint.glyphFor(dark = false)
            val contrast = (tile.luminance() + 0.05f) / (glyph.luminance() + 0.05f)
            assertTrue("settings icon contrast: $contrast", contrast >= 3f)
        }
    }

    @Test fun lightProtocolBadgesStayReadableAndDefinedOnServerSurfaces() {
        val light = paletteFor(dark = false)
        val backgrounds = listOf(light.Grey, light.ServerList, light.ActiveServer, light.Black, light.DarkGrey)
        for (row in backgrounds) {
            for (first in listOf(true, false)) {
                for (look in HomeLook.entries) {
                    val badge = protocolBadgeColors(light, first, look)
                    val fill = badge.fill.compositeOver(row)
                    val foreground = badge.text.compositeOver(fill)
                    val textRatio = contrast(foreground, fill)
                    assertTrue("$look, first=$first, row=$row text contrast: $textRatio", textRatio >= 4.5f)
                    if (look == HomeLook.Idle) {
                        val edge = light.chipOutline.compositeOver(fill)
                        assertTrue("idle badge edge disappears on $row", contrast(edge, row) >= 1.3f)
                    }
                }
            }
        }
    }

    @Test fun lightTagsHaveTheirOwnSurfaceOnTheTagField() {
        val light = paletteFor(dark = false)
        val field = light.DarkGrey
        assertEquals(light.Grey, light.tagChipFill)
        assertTrue(contrast(light.chipOutline.compositeOver(light.tagChipFill), field) >= 1.3f)
        assertTrue(contrast(light.description.compositeOver(light.tagChipFill), light.tagChipFill) >= 4.5f)
        assertTrue(contrast(light.tagPlaceholder.compositeOver(field), field) >= 4.5f)
    }
}
