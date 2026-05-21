package com.resultv.android.ui.screens

import android.app.Activity
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Check
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.resultv.android.R
import com.resultv.android.locale.LocaleManager
import com.resultv.android.theme.Brand
import com.resultv.android.vpn.SettingsRepository

private data class DnsPreset(val key: String, val label: String, val servers: String)

@Composable
private fun dnsPresets(): List<DnsPreset> = listOf(
    DnsPreset("Auto", stringResource(R.string.settings_dns_preset_auto), ""),
    DnsPreset("Google", "Google", "8.8.8.8, 8.8.4.4"),
    DnsPreset("Cloudflare", "Cloudflare", "1.1.1.1, 1.0.0.1"),
    DnsPreset("Quad9", "Quad9", "9.9.9.9, 149.112.112.112"),
)

private data class Lang(val code: String, val title: String)
private val Languages = listOf(
    Lang("EN", "English"),
    Lang("RU", "Русский"),
)

@OptIn(
    ExperimentalMaterial3Api::class,
    androidx.compose.foundation.layout.ExperimentalLayoutApi::class,
)
@Composable
fun SettingsScreen() {
    val ctx = LocalContext.current
    val activity = ctx as? Activity
    val settings by SettingsRepository.state.collectAsStateWithLifecycle()

    var langOpen by remember { mutableStateOf(false) }
    val currentLang = remember(activity) {
        LocaleManager.currentLocale(ctx) ?: "EN"
    }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
        ) {
            Text(
                stringResource(R.string.tab_settings),
                style = MaterialTheme.typography.headlineSmall,
                fontWeight = FontWeight.SemiBold,
            )
            androidx.compose.foundation.layout.Box {
                OutlinedButton(onClick = { langOpen = true }) {
                    Text("🌐  $currentLang")
                }
                DropdownMenu(expanded = langOpen, onDismissRequest = { langOpen = false }) {
                    Languages.forEach { l ->
                        DropdownMenuItem(
                            text = { Text(l.title) },
                            onClick = {
                                langOpen = false
                                if (l.code != currentLang && activity != null) {
                                    LocaleManager.setLocale(activity, l.code)
                                }
                            },
                            trailingIcon = {
                                if (l.code == currentLang) {
                                    Icon(
                                        Icons.Outlined.Check,
                                        contentDescription = null,
                                        tint = Brand.GreenLight,
                                    )
                                }
                            },
                        )
                    }
                }
            }
        }

        SectionLabel(stringResource(R.string.settings_section_general))
        Card(
            shape = RoundedCornerShape(20.dp),
            colors = CardDefaults.cardColors(containerColor = Brand.Surface),
        ) {
            Column {
                ToggleRow(
                    title = stringResource(R.string.settings_kill_switch),
                    subtitle = stringResource(R.string.settings_kill_switch_subtitle),
                    checked = settings.killSwitch,
                    onCheckedChange = { SettingsRepository.setKillSwitch(it) },
                    enabled = false,
                )
                HorizontalDivider(color = Brand.SurfaceHigh)
                ToggleRow(
                    title = stringResource(R.string.settings_adblock),
                    subtitle = stringResource(R.string.settings_adblock_subtitle),
                    checked = settings.adblock,
                    onCheckedChange = { SettingsRepository.setAdblock(it) },
                    enabled = false,
                )
                HorizontalDivider(color = Brand.SurfaceHigh)
                ToggleRow(
                    title = stringResource(R.string.settings_ipv6),
                    subtitle = stringResource(R.string.settings_ipv6_subtitle),
                    checked = settings.ipv6,
                    onCheckedChange = { SettingsRepository.setIpv6(it) },
                )
                HorizontalDivider(color = Brand.SurfaceHigh)
                ToggleRow(
                    title = stringResource(R.string.settings_bypass_lan),
                    subtitle = stringResource(R.string.settings_bypass_lan_subtitle),
                    checked = settings.bypassLan,
                    onCheckedChange = { SettingsRepository.setBypassLan(it) },
                )
            }
        }

        SectionLabel(stringResource(R.string.settings_section_dns))
        Card(
            shape = RoundedCornerShape(20.dp),
            colors = CardDefaults.cardColors(containerColor = Brand.Surface),
        ) {
            Column(
                modifier = Modifier.padding(14.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp),
            ) {
                Text(
                    stringResource(R.string.settings_dns_hint),
                    style = MaterialTheme.typography.bodySmall,
                    color = Brand.SecondaryText,
                )
                androidx.compose.foundation.layout.FlowRow(
                    horizontalArrangement = Arrangement.spacedBy(6.dp),
                    verticalArrangement = Arrangement.spacedBy(6.dp),
                ) {
                    dnsPresets().forEach { p ->
                        FilterChip(
                            selected = settings.dnsPreset == p.key,
                            onClick = { SettingsRepository.setDnsPreset(p.key, "") },
                            label = { Text(p.label) },
                            colors = FilterChipDefaults.filterChipColors(
                                selectedContainerColor = Brand.Green.copy(alpha = 0.2f),
                                selectedLabelColor = Brand.GreenLight,
                            ),
                        )
                    }
                }
                OutlinedTextField(
                    value = if (settings.dnsPreset == "Custom") settings.dnsCustom else "",
                    onValueChange = { SettingsRepository.setDnsPreset("Custom", it) },
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true,
                    placeholder = { Text(stringResource(R.string.settings_dns_custom_placeholder)) },
                )
            }
        }

        SectionLabel(stringResource(R.string.settings_section_about))
        Card(
            shape = RoundedCornerShape(20.dp),
            colors = CardDefaults.cardColors(containerColor = Brand.Surface),
        ) {
            ListItem(
                headlineContent = { Text(stringResource(R.string.app_name)) },
                supportingContent = {
                    Text(
                        stringResource(R.string.settings_about_subtitle),
                        color = Brand.SecondaryText,
                    )
                },
                colors = ListItemDefaults.colors(containerColor = Brand.Surface),
            )
        }
    }
}

@Composable
private fun SectionLabel(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.labelMedium,
        color = Brand.MutedText,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ToggleRow(
    title: String,
    subtitle: String,
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
    enabled: Boolean = true,
) {
    ListItem(
        headlineContent = { Text(title) },
        supportingContent = { Text(subtitle, color = Brand.SecondaryText) },
        trailingContent = {
            Switch(
                checked = checked,
                onCheckedChange = onCheckedChange,
                enabled = enabled,
            )
        },
        colors = ListItemDefaults.colors(containerColor = Brand.Surface),
    )
}
