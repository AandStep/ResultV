package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import java.util.UUID

class SubscriptionProfileRefreshTest {
    @Test
    fun preservesDistinctIdsAndFavoritesWhenSimilarServersChangeOrder() {
        val subscriptionId = UUID.randomUUID().toString()
        try {
            val first = server("First", subscriptionId)
            val second = server("Second", subscriptionId).copy(isFavorite = true)
            ProfileRepository.replaceForSubscription(subscriptionId, listOf(first, second))
            ProfileRepository.setActive(second.id)

            ProfileRepository.replaceForSubscription(
                subscriptionId,
                listOf(server("Second", subscriptionId), server("First", subscriptionId)),
            )

            val state = ProfileRepository.state.value
            val refreshed = state.profiles.filter { it.subscriptionId == subscriptionId }
            assertEquals(listOf(second.id, first.id), refreshed.map { it.id })
            assertEquals(listOf(true, false), refreshed.map { it.isFavorite })
            assertEquals(second.id, state.activeId)
        } finally {
            ProfileRepository.removeBySubscriptionId(subscriptionId)
        }
    }

    @Test
    fun clearsSelectionWhenTheActiveServerDisappears() {
        val subscriptionId = UUID.randomUUID().toString()
        try {
            val first = server("First", subscriptionId)
            val second = server("Second", subscriptionId, port = 8443)
            ProfileRepository.replaceForSubscription(subscriptionId, listOf(first, second))
            ProfileRepository.setActive(first.id)

            ProfileRepository.replaceForSubscription(
                subscriptionId,
                listOf(Profile.section("Other servers", subscriptionId), second),
            )

            assertNull(ProfileRepository.state.value.activeId)
        } finally {
            ProfileRepository.removeBySubscriptionId(subscriptionId)
        }
    }

    @Test
    fun keepsIdentityEvenWhenTwoServersHaveTheSameNameAndEndpoint() {
        val subscriptionId = UUID.randomUUID().toString()
        try {
            val first = server("Germany", subscriptionId, uuid = "first")
            val second = server("Germany", subscriptionId, uuid = "second").copy(isFavorite = true)
            ProfileRepository.replaceForSubscription(subscriptionId, listOf(first, second))
            ProfileRepository.setActive(second.id)

            val refreshed = ProfileRepository.replaceForSubscription(
                subscriptionId,
                listOf(server("Germany", subscriptionId, uuid = "second"), server("Germany", subscriptionId, uuid = "first")),
            )
            assertEquals(listOf(second.id, first.id), refreshed.map { it.id })
            assertEquals(listOf(true, false), refreshed.map { it.isFavorite })

            ProfileRepository.replaceForSubscription(
                subscriptionId,
                listOf(server("Germany", subscriptionId, uuid = "first")),
            )
            assertNull(ProfileRepository.state.value.activeId)
        } finally {
            ProfileRepository.removeBySubscriptionId(subscriptionId)
        }
    }

    private fun server(name: String, subscriptionId: String, port: Int = 443, uuid: String = ""): Profile =
        Profile.fromEntryJson(
            name,
            """{"name":"$name","type":"VLESS","ip":"example.org","port":$port,"extra":{"uuid":"$uuid"}}""",
            subscriptionId,
        )
}
