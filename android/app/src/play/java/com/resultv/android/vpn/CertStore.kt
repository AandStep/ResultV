/*
 * Copyright (C) 2026 ResultV
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 */

package com.resultv.android.vpn

/**
 * Заглушка для дистрибутива Play.
 *
 * Браузерный ad-block перехватывает TLS пользовательским корневым
 * сертификатом. Политика VpnService в Google Play запрещает VPN-приложениям
 * вмешиваться в рекламу, и ни одно приложение в магазине не поставляет
 * перехват TLS — именно поэтому полный AdGuard раздаётся мимо Play.
 *
 * Реализация живёт в src/full. Go-слой в этой сборке тоже без неё: тег
 * no_mitm выкидывает internal/filter из линковки, так что в .so нет ни
 * генерации CA, ни перехвата.
 */
object CertStore {
    fun isInstalled(dataDir: String): Boolean = false

    fun applySeed(dataDir: String, seed: String) = Unit

    fun staleEntryCount(dataDir: String): Int = 0

    fun isResultVCommonName(subjectDn: String?): Boolean = false
}
