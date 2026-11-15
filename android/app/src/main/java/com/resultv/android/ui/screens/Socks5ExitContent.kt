package com.resultv.android.ui.screens

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Apps
import androidx.compose.material.icons.outlined.Dns
import androidx.compose.material.icons.outlined.ExpandLess
import androidx.compose.material.icons.outlined.ExpandMore
import androidx.compose.material.icons.outlined.Language
import androidx.compose.material.icons.outlined.Route
import androidx.compose.material.icons.outlined.Speed
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.resultv.android.R
import com.resultv.android.theme.RvCategory
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.RvSpace
import com.resultv.android.ui.components.RvButton
import com.resultv.android.ui.components.RvButtonColors
import com.resultv.android.ui.components.RvButtonLabel
import com.resultv.android.ui.components.TagField
import com.resultv.android.vpn.Socks5ExitRepository
import com.resultv.android.vpn.VpnState
import com.resultv.android.vpn.VpnStatus
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

@Composable
internal fun Socks5ExitContent() {
    val saved by Socks5ExitRepository.profile.collectAsStateWithLifecycle()
    val status by Socks5ExitRepository.check.collectAsStateWithLifecycle()
    val vpn by VpnState.status.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var enabled by remember(saved) { mutableStateOf(saved.enabled) }
    var host by remember(saved) { mutableStateOf(saved.host) }
    var port by remember(saved) { mutableStateOf(saved.port.toString()) }
    var username by remember(saved) { mutableStateOf(saved.username) }
    var password by remember(saved) { mutableStateOf(saved.password) }
    var sites by remember(saved) { mutableStateOf(saved.sites) }
    var apps by remember(saved) { mutableStateOf(saved.apps) }
    var siteDraft by remember { mutableStateOf("") }
    var saving by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf(false) }
    var showApps by remember { mutableStateOf(false) }
    val draft = saved.copy(
        enabled = enabled, host = host.trim(), port = port.toIntOrNull() ?: 0,
        username = username, password = password,
        sites = (sites + listOfNotNull(siteDraft.trim().takeIf { it.isNotEmpty() })).distinct(),
        apps = apps,
    )

    SheetGroup(stringResource(R.string.socks5_title), Icons.Outlined.Route) {
        SheetToggleRow(
            icon = Icons.Outlined.Route,
            tint = RvCategory.Main,
            title = stringResource(R.string.socks5_enable),
            subtitle = stringResource(R.string.socks5_description),
            checked = enabled,
            onCheckedChange = { enabled = it },
        )
        SheetDivider()
        SheetRow(
            icon = Icons.Outlined.Dns,
            tint = RvCategory.Blue,
            title = stringResource(R.string.socks5_connection),
            subtitle = stringResource(R.string.socks5_connection_desc),
            below = {
                SheetField(host, { host = it }, stringResource(R.string.socks5_server), isError = error)
                SheetField(
                    port, { port = it.filter(Char::isDigit).take(5) },
                    stringResource(R.string.socks5_port), isError = error, keyboardType = KeyboardType.Number,
                )
                SheetField(username, { username = it }, stringResource(R.string.socks5_login))
                SheetField(
                    password, { password = it }, stringResource(R.string.socks5_password),
                    keyboardType = KeyboardType.Password,
                    visualTransformation = PasswordVisualTransformation(),
                )
            },
        )
        SheetDivider()
        SheetRow(
            icon = Icons.Outlined.Language,
            tint = RvCategory.Violet,
            title = stringResource(R.string.socks5_sites),
            subtitle = stringResource(R.string.socks5_scope),
            below = {
                TagField(
                    values = sites,
                    draft = siteDraft,
                    onDraftChange = { siteDraft = it },
                    onCommit = { site -> if (site.isNotBlank()) sites = (sites + site.trim()).distinct() },
                    onRemove = { sites = sites - it },
                    placeholder = stringResource(R.string.rules_domain_placeholder),
                )
            },
        )
        SheetDivider()
        SheetRow(
            icon = Icons.Outlined.Apps,
            tint = RvCategory.Violet,
            title = stringResource(R.string.socks5_apps_title),
            subtitle = stringResource(R.string.rules_app_selected_count, apps.size),
            modifier = Modifier.clickable(role = Role.Button) { showApps = !showApps },
            trailing = {
                Icon(
                    if (showApps) Icons.Outlined.ExpandLess else Icons.Outlined.ExpandMore,
                    contentDescription = null, tint = RvColor.description,
                )
            },
            below = if (showApps) ({
                InstalledAppPicker(
                    selectedPackages = apps,
                    onToggle = { pkg, checked -> apps = if (checked) apps + pkg else apps - pkg },
                    showCount = false,
                )
            }) else null,
        )
        SheetDivider()
        SheetRow(
            icon = Icons.Outlined.Speed,
            tint = RvCategory.Cyan,
            title = stringResource(R.string.socks5_status),
            subtitle = when {
                status.inProgress -> stringResource(R.string.socks5_checking)
                status.failed -> stringResource(R.string.socks5_check_error)
                status.ip.isNotBlank() -> stringResource(
                    R.string.socks5_check_result,
                    status.ip,
                    status.country.ifBlank { "—" },
                    stringResource(if (status.udp) R.string.socks5_udp_exit else R.string.socks5_udp_vpn),
                )
                else -> stringResource(R.string.socks5_check_hint)
            },
            below = {
                if (error) Text(stringResource(R.string.socks5_save_error), style = SheetNoteStyle, color = RvColor.Errors)
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2)) {
                    RvButton(
                        enabled = !saving && draft != saved,
                        onClick = {
                            scope.launch {
                                saving = true
                                error = withContext(Dispatchers.IO) {
                                    runCatching { Socks5ExitRepository.save(draft) }.isFailure
                                }
                                if (!error) siteDraft = ""
                                saving = false
                            }
                        },
                        fill = RvButtonColors.greenFill,
                        outline = RvButtonColors.greenOutline,
                        modifier = Modifier.weight(1f),
                    ) {
                        Text(stringResource(R.string.socks5_save), style = RvButtonLabel, fontWeight = FontWeight.Bold, color = RvColor.successText)
                    }
                    RvButton(
                        enabled = !status.inProgress && saved.enabled && draft == saved && vpn is VpnStatus.Connected,
                        onClick = { scope.launch(Dispatchers.IO) { Socks5ExitRepository.checkExit() } },
                        fill = RvButtonColors.greyFill,
                        outline = RvButtonColors.greyOutline,
                        modifier = Modifier.weight(1f),
                    ) {
                        Text(stringResource(R.string.socks5_check), style = RvButtonLabel, fontWeight = FontWeight.Bold, color = RvColor.White)
                    }
                }
            },
        )
    }
}
