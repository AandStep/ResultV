package com.resultv.android.ui.screens

import androidx.compose.runtime.Composable
import com.resultv.android.R
import com.resultv.android.vpn.SettingsState

/**
 * Заглушка для дистрибутива Play — по образцу `CertSelfTest`, `CertStore`,
 * `AdBlockRepository` и `BrowserAdBlockAttachment`.
 *
 * Ни DNS-фильтрации рекламы (тег `no_adblock`), ни браузерного ad-block (тег
 * `no_mitm`) в этой сборке нет. Секция вынесена из общего `SettingsScreen`
 * ради ресурсов: её строки лежат в `src/full/res` и в play-APK не попадают.
 */
internal object AdBlockGroupRes {
    // Подкатегория «Блокировка рекламы» в play не рендерится: и ряд в списке,
    // и ветка листа стоят за `BuildConfig.DNS_ADBLOCK`, который здесь false.
    // Метки нужны лишь для того, чтобы общий enum SettingsSubcategory
    // скомпилировался, поэтому указывают на первую попавшуюся живую строку.
    val label = R.string.settings_group_security
    val desc = R.string.settings_group_security_desc
}

@Composable
@Suppress("UNUSED_PARAMETER")
internal fun AdBlockGroupContent(settings: SettingsState, onOpenCertWizard: () -> Unit) {
}
