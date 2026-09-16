package com.resultv.android.ui.screens

import androidx.compose.runtime.Composable
import com.resultv.android.vpn.SettingsState

/**
 * Заглушка для дистрибутива Play — по образцу `CertSelfTest`, `CertStore` и
 * прочих пар из `src/full` / `src/play`.
 *
 * Браузерного ad-block в этой сборке нет ни в .so (тег `no_mitm`), ни в
 * Kotlin. Секция вынесена из общего `SettingsScreen` ради ресурсов: строки,
 * описывающие перехват TLS, лежат в `src/full/res` и в play-APK не
 * попадают, так что общий код не может на них даже сослаться.
 */
@Composable
@Suppress("UNUSED_PARAMETER")
internal fun BrowserAdBlockSection(settings: SettingsState, onOpenCertWizard: () -> Unit) {
}
