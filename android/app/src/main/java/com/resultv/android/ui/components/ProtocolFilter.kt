package com.resultv.android.ui.components

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import com.resultv.android.theme.Brand

/**
 * Canonical protocol codes the filter chip row understands. The order is
 * the desktop's display order. Each label maps 1:1 to the value returned
 * by `profileProtocol` in HomeScreen.kt.
 */
val PROTOCOL_FILTER_OPTIONS: List<Pair<String, String>> = listOf(
    "VLESS" to "VLESS",
    "VMESS" to "VMess",
    "TROJAN" to "Trojan",
    "SHADOWSOCKS" to "SS",
    "HYSTERIA2" to "Hysteria2",
    "WIREGUARD" to "WG",
    "AMNEZIAWG" to "AWG",
)

/**
 * Multi-select chip row that filters a profile list by protocol type. Empty
 * selection = "no filter". Chips for protocols not present in [available]
 * are hidden so the row never advertises an empty bucket.
 */
@Composable
fun ProtocolFilterChips(
    selected: Set<String>,
    available: Set<String>,
    onToggle: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val visible = PROTOCOL_FILTER_OPTIONS.filter { it.first in available }
    if (visible.size < 2) return
    Row(
        modifier = modifier.horizontalScroll(rememberScrollState()),
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        visible.forEach { (code, label) ->
            val isSelected = code in selected
            FilterChip(
                selected = isSelected,
                onClick = { onToggle(code) },
                label = {
                    Text(
                        text = label,
                        style = MaterialTheme.typography.labelMedium,
                    )
                },
                colors = FilterChipDefaults.filterChipColors(
                    containerColor = Color.White.copy(alpha = 0.04f),
                    labelColor = Brand.SecondaryText,
                    selectedContainerColor = Brand.Green.copy(alpha = 0.20f),
                    selectedLabelColor = Brand.GreenLight,
                ),
                modifier = Modifier.padding(vertical = 2.dp),
            )
        }
    }
}
