package com.resultv.android.ui.components

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Bolt
import androidx.compose.material.icons.filled.Star
import androidx.compose.material.icons.outlined.Public
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.resultv.android.R
import com.resultv.android.theme.Brand

/**
 * Server / profile row used by Home selector and Proxies list. Highlights
 * when active and shows a leading flag (or AUTO bolt) + name + optional
 * favorite star.
 */
@OptIn(ExperimentalFoundationApi::class)
@Composable
fun ServerRow(
    name: String,
    subtitle: String,
    countryCode: String?,
    isAuto: Boolean,
    isActive: Boolean,
    isFavorite: Boolean,
    onClick: () -> Unit,
    trailing: @Composable (() -> Unit)? = null,
    /** Latest ping in milliseconds, or null if not yet probed. */
    latencyMs: Int? = null,
    /**
     * True while a user-triggered ping refresh for this row is in flight.
     * Forces the spinner even when [latencyMs] is non-null, so re-pinging
     * already-pinged servers gives visible feedback.
     */
    isLoading: Boolean = false,
    /** Long-press handler — used by Proxies to open the edit sheet. */
    onLongClick: (() -> Unit)? = null,
) {
    val border = if (isActive) Brand.Green.copy(alpha = 0.45f) else Color.White.copy(alpha = 0.06f)
    val bg = if (isActive) Brand.Green.copy(alpha = 0.10f) else Color.White.copy(alpha = 0.03f)
    val titleColor = if (isActive) Brand.GreenLight else MaterialTheme.colorScheme.onBackground
    // Latency is colour-coded: green <80ms, amber 80–200ms, rose >200ms.
    val latencyColor = when {
        latencyMs == null -> Brand.MutedText
        latencyMs <= 200 -> Brand.GreenLight
        latencyMs <= 499 -> Brand.Warning
        else -> Brand.Danger
    }

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(18.dp))
            .background(bg)
            .border(1.dp, border, RoundedCornerShape(18.dp))
            .let { base ->
                if (onLongClick != null)
                    base.combinedClickable(onClick = onClick, onLongClick = onLongClick)
                else
                    base.clickable(onClick = onClick)
            }
            .padding(horizontal = 14.dp, vertical = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        // Leading icon — flag emoji, AUTO bolt, or globe fallback.
        Box(
            modifier = Modifier
                .size(48.dp)
                .clip(RoundedCornerShape(12.dp))
                .background(
                    if (isActive) Brand.Green.copy(alpha = 0.18f)
                    else Color.White.copy(alpha = 0.07f)
                )
                .border(
                    1.dp,
                    if (isActive) Brand.Green.copy(alpha = 0.28f)
                    else Color.White.copy(alpha = 0.09f),
                    RoundedCornerShape(12.dp)
                ),
            contentAlignment = Alignment.Center,
        ) {
            when {
                isAuto -> Icon(
                    imageVector = Icons.Filled.Bolt,
                    contentDescription = null,
                    tint = Brand.GreenLight,
                    modifier = Modifier.size(24.dp),
                )
                countryCode != null -> Text(
                    text = flagFromCountry(countryCode),
                    style = MaterialTheme.typography.titleLarge,
                )
                else -> Icon(
                    imageVector = Icons.Outlined.Public,
                    contentDescription = null,
                    tint = Brand.SecondaryText,
                    modifier = Modifier.size(24.dp),
                )
            }
        }

        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = name,
                color = titleColor,
                style = MaterialTheme.typography.titleSmall,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Text(
                text = subtitle,
                color = Brand.MutedText,
                style = MaterialTheme.typography.bodySmall,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }

        // Favourite marker — small star inline next to ping when set.
        // Toggle moved to the long-press sheet, so no IconButton here.
        if (isFavorite) {
            Icon(
                imageVector = Icons.Filled.Star,
                contentDescription = stringResource(R.string.action_unfavorite),
                tint = Brand.Favorite,
                modifier = Modifier.size(14.dp),
            )
        }

        // Latency reading — single number, colour reflects health, or loading spinner.
        // Spinner wins while a user-triggered refresh is in flight, so re-pinging
        // already-pinged servers still shows the loading state.
        if (isLoading || latencyMs == null) {
            androidx.compose.material3.CircularProgressIndicator(
                modifier = Modifier.size(14.dp),
                color = Brand.MutedText,
                strokeWidth = 2.dp
            )
        } else {
            Text(
                text = "$latencyMs ms",
                style = MaterialTheme.typography.labelMedium,
                color = latencyColor,
            )
        }

        if (trailing != null) trailing()
    }
}

/** Convert a 2-letter ISO country code to the corresponding flag emoji. */
fun flagFromCountry(code: String): String {
    if (code.length != 2) return "🌐"
    val upper = code.uppercase()
    val a = 0x1F1E6 - 'A'.code + upper[0].code
    val b = 0x1F1E6 - 'A'.code + upper[1].code
    return runCatching {
        String(Character.toChars(a)) + String(Character.toChars(b))
    }.getOrDefault("🌐")
}
