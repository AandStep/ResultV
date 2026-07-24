package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SmartAppMatcherTest {

    private val blocked = setOf(
        "instagram.com", "youtube.com", "tiktok.com", "x.com", "t.me",
    )

    @Test fun brandsFrom_takesSecondLevelLabelMinLen3() {
        val brands = SmartAppMatcher.brandsFrom(
            listOf("youtube.com", "www.instagram.com", "t.me", ".x.com", "")
        )
        assertTrue("youtube" in brands)
        assertTrue("instagram" in brands)
        assertFalse("t" in brands)   // too short → alias territory
        assertFalse("x" in brands)   // too short → alias territory
        assertFalse("com" in brands)
    }

    @Test fun matches_byPackageSegment() {
        val out = SmartAppMatcher.matchedPackages(
            listOf(AppMeta("com.google.android.youtube", "YouTube")), blocked
        )
        assertEquals(setOf("com.google.android.youtube"), out)
    }

    @Test fun matches_byLabel_whenPackageObfuscated() {
        // TikTok's package carries no brand token; its label does.
        val out = SmartAppMatcher.matchedPackages(
            listOf(AppMeta("com.zhiliaoapp.musically", "TikTok")), blocked
        )
        assertTrue("com.zhiliaoapp.musically" in out)
    }

    @Test fun matches_byAlias_forRebrandsAndShortNames() {
        // com.twitter.android ↔ x.com, org.telegram.messenger ↔ t.me:
        // neither package nor label yields a ≥3-char brand in the list.
        val apps = listOf(
            AppMeta("com.twitter.android", "X"),
            AppMeta("org.telegram.messenger", "Telegram"),
        )
        val out = SmartAppMatcher.matchedPackages(apps, blocked)
        assertTrue("com.twitter.android" in out)
        assertTrue("org.telegram.messenger" in out)
    }

    @Test fun noMatch_forUnrelatedApp() {
        val out = SmartAppMatcher.matchedPackages(
            listOf(AppMeta("ru.gosuslugi.app", "Госуслуги")), blocked
        )
        assertTrue(out.isEmpty())
    }

    @Test fun structuralSegmentsDoNotMatch() {
        // "android"/"com" must never be treated as brands even if a blocked
        // domain happened to be e.g. "android.com".
        val out = SmartAppMatcher.matchedPackages(
            listOf(AppMeta("com.example.weather", "Weather")),
            setOf("com.com", "android.com"),
        )
        assertTrue(out.isEmpty())
    }
}
