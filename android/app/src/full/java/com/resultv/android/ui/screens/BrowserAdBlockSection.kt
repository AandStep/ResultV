package com.resultv.android.ui.screens

import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Shield
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.resultv.android.R
import com.resultv.android.theme.Brand
import com.resultv.android.vpn.CertStore
import com.resultv.android.vpn.CertTrustState
import com.resultv.android.vpn.SettingsRepository
import com.resultv.android.vpn.SettingsState
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * Переключатель браузерного ad-block и строка состояния сертификата.
 *
 * Живёт в src/full, а не в общем `SettingsScreen`, потому что в play-сборке
 * этой функции нет ни в Kotlin, ни в .so (тег `no_mitm`) — и её строки не
 * должны попадать в play-APK. Ресурсы лежат рядом, в `src/full/res`; из
 * общего кода их нельзя было бы даже упомянуть.
 *
 * Требует API 29+ (`VpnService.Builder.setHttpProxy`), поэтому проверка
 * версии тоже здесь: снаружи о ней знать незачем.
 */
@Composable
internal fun BrowserAdBlockSection(settings: SettingsState, onOpenCertWizard: () -> Unit) {
    if (android.os.Build.VERSION.SDK_INT < android.os.Build.VERSION_CODES.Q) return
    HorizontalDivider(color = Brand.SurfaceHigh)

    val context = LocalContext.current
    val scope = rememberCoroutineScope()

    // The persisted trust state only refreshes on a VPN connect, so it goes
    // stale the moment the user installs or removes the cert outside the app.
    // Re-reading the trust store when this section opens keeps the status line
    // and the "Install certificate" row honest.
    var certInstalled by remember { mutableStateOf(settings.certTrustState == CertTrustState.TRUSTED) }
    LaunchedEffect(Unit) {
        certInstalled = withContext(Dispatchers.IO) {
            CertStore.isInstalled(context.filesDir.absolutePath)
        }
        SettingsRepository.setCertTrustState(
            if (certInstalled) CertTrustState.TRUSTED else CertTrustState.UNTRUSTED,
        )
    }

    ToggleRow(
        title = stringResource(R.string.settings_browser_adblock),
        subtitle = stringResource(R.string.settings_browser_adblock_subtitle),
        icon = Icons.Outlined.Shield,
        iconBg = Color(0xFF22c55e).copy(alpha = 0.18f),
        iconTint = Color(0xFF4ade80),
        checked = settings.browserAdBlock,
        onCheckedChange = { enabled ->
            if (enabled) {
                scope.launch(Dispatchers.IO) {
                    mobile.Mobile.fetchFilterLists(context.filesDir.absolutePath)
                }
                SettingsRepository.setBrowserAdBlock(true)
                // Turning the feature on without a trusted cert does nothing
                // useful, so the toggle doubles as the wizard's entry point.
                if (!certInstalled) onOpenCertWizard()
            } else {
                SettingsRepository.setBrowserAdBlock(false)
            }
        },
    )
    Text(
        text = stringResource(
            if (certInstalled) R.string.cert_status_trusted else R.string.cert_status_untrusted,
        ),
        style = MaterialTheme.typography.labelSmall,
        color = if (certInstalled) Brand.GreenLight else Brand.MutedText,
        modifier = Modifier.padding(start = 62.dp, top = 2.dp),
    )
    if (!certInstalled) {
        HorizontalDivider(color = Brand.SurfaceHigh)
        NavRow(
            label = stringResource(R.string.settings_browser_adblock_install),
            icon = Icons.Outlined.Shield,
            iconBg = Color(0xFF22c55e).copy(alpha = 0.18f),
            iconTint = Color(0xFF4ade80),
            onClick = onOpenCertWizard,
        )
    }
}
