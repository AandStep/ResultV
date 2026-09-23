package com.resultv.android.ui.screens

import androidx.compose.material.icons.outlined.OpenInBrowser
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.ui.text.font.FontWeight
import com.resultv.android.ui.components.RvButton
import com.resultv.android.ui.components.RvButtonColors
import com.resultv.android.ui.components.RvButtonLabel
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Block
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
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.resultv.android.R
import com.resultv.android.theme.RvCategory
import com.resultv.android.theme.RvColor
import com.resultv.android.vpn.AdBlockRepository
import com.resultv.android.vpn.CertStore
import com.resultv.android.vpn.CertTrustState
import com.resultv.android.vpn.SettingsRepository
import com.resultv.android.vpn.SettingsState
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * Метки подкатегории «Блокировка рекламы» в списке настроек.
 *
 * Лежат здесь, а не прямо в `SettingsSubcategory`: enum общий, а эти две
 * строки описывают функцию, которой в play нет, и потому живут в
 * `src/full/res`.
 */
internal object AdBlockGroupRes {
    val label = R.string.settings_group_adblock
    val desc = R.string.settings_group_adblock_desc
    val items = R.string.settings_group_adblock_items
}

/**
 * Содержимое листа «Блокировка рекламы»: DNS-фильтрация и браузерный
 * ad-block (MITM).
 *
 * Вся секция живёт в `src/full`, потому что в play-сборке нет ни одной из
 * двух функций — ни в Kotlin, ни в .so (теги `no_adblock` и `no_mitm`), — и
 * её строки не должны попадать в play-APK. Ресурсы лежат в `src/full/res`;
 * из общего кода их нельзя было бы даже упомянуть.
 */
@Composable
internal fun AdBlockGroupContent(settings: SettingsState, onOpenCertWizard: () -> Unit) {
    SheetGroup(stringResource(R.string.settings_label_dns_filter), Icons.Outlined.Block) {
        SheetToggleRow(
            icon = Icons.Outlined.Block,
            tint = RvCategory.Red,
            title = stringResource(R.string.settings_adblock),
            subtitle = stringResource(R.string.settings_adblock_subtitle),
            checked = settings.adblock,
            onCheckedChange = {
                SettingsRepository.setAdblock(it)
                // Warm the SRS cache so the next connect references local lists
                // instead of waiting on sing-box's remote fetch. Safe no-op when
                // already fresh (24h TTL).
                if (it) AdBlockRepository.refreshAsync()
            },
        )
    }
    BrowserAdBlockSection(settings, onOpenCertWizard)
}

/**
 * Переключатель браузерного ad-block и строка состояния сертификата.
 *
 * Требует API 29+ (`VpnService.Builder.setHttpProxy`), поэтому проверка
 * версии здесь: снаружи о ней знать незачем.
 */
@Composable
private fun BrowserAdBlockSection(settings: SettingsState, onOpenCertWizard: () -> Unit) {
    if (android.os.Build.VERSION.SDK_INT < android.os.Build.VERSION_CODES.Q) return

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

    SheetGroup(stringResource(R.string.settings_label_browser), Icons.Outlined.OpenInBrowser) {
        SheetToggleRow(
            icon = Icons.Outlined.OpenInBrowser,
            tint = RvCategory.Amber,
            title = stringResource(R.string.settings_browser_adblock),
            subtitle = stringResource(R.string.settings_browser_adblock_subtitle),
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
            below = {
                Text(
                    text = stringResource(
                        if (certInstalled) R.string.cert_status_trusted else R.string.cert_status_untrusted,
                    ),
                    style = SheetNoteStyle,
                    color = if (certInstalled) RvColor.Second else RvColor.Warning,
                )
                if (!certInstalled) {
                    RvButton(
                        onClick = onOpenCertWizard,
                        fill = RvButtonColors.greenFill,
                        outline = RvButtonColors.greenOutline,
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text(
                            stringResource(R.string.settings_browser_adblock_install),
                            style = RvButtonLabel,
                            fontWeight = FontWeight.Bold,
                            color = RvColor.Main,
                        )
                    }
                }
            },
        )
    }
}
