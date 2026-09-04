/*
 * Copyright (C) 2026 ResultV
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 */

package com.resultv.android.vpn

import android.content.Context
import android.content.Intent

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
object CertInstaller {
    fun canInstallDirectly(): Boolean = false

    /**
     * Недостижимо в этой сборке: мастер сертификата живёт в src/full, а ведущий
     * к нему UI спрятан за BuildConfig.BROWSER_ADBLOCK. Бросаем, а не возвращаем
     * пустой Intent: молчаливая заглушка спрятала бы ошибку в разводке
     * флейворов вместо того, чтобы её показать.
     */
    fun buildInstallIntent(context: Context, dataDir: String): Intent =
        throw UnsupportedOperationException("certificate install is not available in the Play build")
}
