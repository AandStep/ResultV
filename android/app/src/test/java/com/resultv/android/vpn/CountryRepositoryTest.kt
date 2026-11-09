package com.resultv.android.vpn

import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class CountryRepositoryTest {
    @Test
    fun retriesTransientFailuresAndSharesLookupAcrossProfilesOnSameHost() = runBlocking {
        var attempts = 0
        lateinit var lookups: CountryLookups
        coroutineScope {
            lookups = CountryLookups(this, detectCountry = { _, _ ->
                when (++attempts) {
                    1 -> throw Exception("country api request: i/o timeout")
                    2 -> throw Exception("country api http 503")
                    else -> "NL"
                }
            })
            lookups.resolve(listOf(profile("one", "192.0.2.1"), profile("two", "192.0.2.1")), "unused")
        }
        assertEquals(3, attempts)
        assertEquals(mapOf("one" to "nl", "two" to "nl"), lookups.results.value)
    }

    @Test
    fun doesNotRetryPermanentErrors() = runBlocking {
        var attempts = 0
        lateinit var lookups: CountryLookups
        coroutineScope {
            lookups = CountryLookups(this, detectCountry = { _, _ ->
                attempts++
                throw Exception("country api http 404")
            })
            lookups.resolve(listOf(profile("one", "192.0.2.1")), "unused")
        }
        assertEquals(1, attempts)
        assertTrue(lookups.results.value.isEmpty())
    }

    @Test
    fun stopsAfterThreeTransientFailures() = runBlocking {
        var attempts = 0
        lateinit var lookups: CountryLookups
        coroutineScope {
            lookups = CountryLookups(this, detectCountry = { _, _ ->
                attempts++
                throw Exception("country api http 503")
            })
            lookups.resolve(listOf(profile("one", "192.0.2.1")), "unused")
        }
        assertEquals(3, attempts)
        assertTrue(lookups.results.value.isEmpty())
    }

    @Test
    fun oldLookupCannotOverwriteNewHostCountryAfterEdit() = runBlocking {
        val oldStarted = CompletableDeferred<Unit>()
        val oldResult = CompletableDeferred<String>()
        lateinit var lookups: CountryLookups
        withTimeout(5_000) {
            lookups = CountryLookups(this, detectCountry = { host, _ ->
                if (host == "192.0.2.1") {
                    oldStarted.complete(Unit)
                    oldResult.await()
                } else {
                    "NL"
                }
            })
            lookups.resolve(listOf(profile("one", "192.0.2.1")), "unused")
            oldStarted.await()
            lookups.resolve(listOf(profile("one", "192.0.2.2")), "unused")
            lookups.results.first { it["one"] == "nl" }
            oldResult.complete("DE")
        }
        assertEquals(mapOf("one" to "nl"), lookups.results.value)
    }

    @Test
    fun clearsResolvedFlagWhenProfileChangesHost() = runBlocking {
        val newResult = CompletableDeferred<String>()
        withTimeout(5_000) {
            val lookups = CountryLookups(this, detectCountry = { host, _ ->
                if (host == "192.0.2.1") "DE" else newResult.await()
            })
            lookups.resolve(listOf(profile("one", "192.0.2.1")), "unused")
            lookups.results.first { it["one"] == "de" }
            lookups.resolve(listOf(profile("one", "192.0.2.2")), "unused")
            assertTrue(lookups.results.value.isEmpty())
            newResult.complete("NL")
            assertEquals(mapOf("one" to "nl"), lookups.results.first { it["one"] == "nl" })
        }
    }

    private fun profile(id: String, host: String) = Profile(
        id = id,
        name = id,
        entryJson = JSONObject().put("type", "SOCKS5").put("ip", host).toString(),
    )
}
