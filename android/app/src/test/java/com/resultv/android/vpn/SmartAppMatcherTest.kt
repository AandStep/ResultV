package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class SmartAppMatcherTest {

    // Real registrable domains that ARE blocked in the RU list (verified against
    // the merged Re-filter/Antizapret sources). Vendor domains like google.com,
    // yandex.ru, ozon.ru, tinkoff.ru are deliberately NOT here — they are not
    // blocked, so their apps must never be auto-matched.
    private val blocked = setOf(
        "instagram.com", "youtube.com", "tiktok.com", "x.com", "twitter.com",
        "facebook.com", "telegram.org", "t.me",
    )

    @Test fun registrableDomain_reversesFirstTwoLabels() {
        assertEquals("instagram.com", SmartAppMatcher.registrableDomain("com.instagram.android"))
        assertEquals("ozon.ru", SmartAppMatcher.registrableDomain("ru.ozon.app.android"))
        assertNull(SmartAppMatcher.registrableDomain("singlelabel"))
    }

    @Test fun matches_targetApps_byRegistrableDomain() {
        val out = SmartAppMatcher.matchedPackages(
            listOf(
                "com.instagram.android",   // instagram.com
                "com.twitter.android",     // twitter.com (blocked)
                "com.facebook.katana",     // facebook.com
                "org.telegram.messenger",  // telegram.org
            ),
            blocked,
        )
        assertEquals(
            setOf("com.instagram.android", "com.twitter.android",
                "com.facebook.katana", "org.telegram.messenger"),
            out,
        )
    }

    @Test fun matches_youtubeAndTiktok_viaAlias() {
        // Their package's registrable domain (google.com / zhiliaoapp.com) is not
        // blocked; the curated alias maps them to youtube.com / tiktok.com.
        val out = SmartAppMatcher.matchedPackages(
            listOf("com.google.android.youtube", "com.zhiliaoapp.musically"),
            blocked,
        )
        assertTrue("com.google.android.youtube" in out)
        assertTrue("com.zhiliaoapp.musically" in out)
    }

    @Test fun doesNotMatch_reportedFalsePositives() {
        // Regression for the on-device over-matching: none of these vendors'
        // registrable domains are blocked, so none may be auto-tunnelled.
        val apps = listOf(
            "ru.ozon.app.android",              // ozon.ru
            "com.idamob.tinkoff.android",       // idamob.com
            "ru.yandex.mail",                   // yandex.ru
            "ru.yandex.yandexnavi",             // yandex.ru
            "ru.mts.mymts",                     // mts.ru
            "com.google.android.apps.photos",   // google.com (NOT blocked)
            "com.google.android.apps.docs",     // google.com
            "com.wildberries.ru.app",           // wildberries.ru
        )
        val out = SmartAppMatcher.matchedPackages(apps, blocked)
        assertTrue("expected no matches, got $out", out.isEmpty())
    }

    @Test fun aliasRespectsBlocklist_noMatchWhenDomainNotBlocked() {
        // If the region's list does not block tiktok.com, the aliased app must
        // not be forced into the tunnel.
        val out = SmartAppMatcher.matchedPackages(
            listOf("com.zhiliaoapp.musically"),
            setOf("instagram.com"),  // tiktok.com absent
        )
        assertFalse("com.zhiliaoapp.musically" in out)
    }
}
