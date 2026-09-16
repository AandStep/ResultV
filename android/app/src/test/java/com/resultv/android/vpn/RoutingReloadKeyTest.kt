package com.resultv.android.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Test

class RoutingReloadKeyTest {

    @Test fun switchingActiveProfileChangesTheKey() {
        assertNotEquals(
            routingReloadKey(RoutingMode.Global, "a", 1),
            routingReloadKey(RoutingMode.Global, "b", 1),
        )
    }

    @Test fun rebuildingTheActiveProfileChangesTheKey() {
        assertNotEquals(
            routingReloadKey(RoutingMode.Global, "a", 1),
            routingReloadKey(RoutingMode.Global, "a", 2),
        )
    }

    // Главное, ради чего этот ключ существует: правка НЕактивного профиля не
    // должна рвать живое соединение. Она не двигает ни activeId, ни счётчик
    // сборок активного — значит и ключ не двигает.
    @Test fun keyIsStableWhenNothingRelevantMoved() {
        assertEquals(
            routingReloadKey(RoutingMode.Global, "a", 1),
            routingReloadKey(RoutingMode.Global, "a", 1),
        )
    }

    // В Smart профиль не действует, поэтому его смена — не повод
    // перезапускаться и рвать соединение.
    @Test fun profileIsInvisibleInSmart() {
        assertEquals(
            routingReloadKey(RoutingMode.Smart, "a", 1),
            routingReloadKey(RoutingMode.Smart, "b", 7),
        )
    }

    // Но смена режима — повод: в Global правила появляются, в Smart исчезают.
    @Test fun changingModeChangesTheKey() {
        assertNotEquals(
            routingReloadKey(RoutingMode.Global, "a", 1),
            routingReloadKey(RoutingMode.Smart, "a", 1),
        )
    }

    // Снятие профиля («без профиля») — тоже смена маршрутизации.
    @Test fun clearingTheActiveProfileChangesTheKey() {
        assertNotEquals(
            routingReloadKey(RoutingMode.Global, "a", 1),
            routingReloadKey(RoutingMode.Global, "", 1),
        )
    }
}
