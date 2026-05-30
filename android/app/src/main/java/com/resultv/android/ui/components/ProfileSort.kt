package com.resultv.android.ui.components

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Sort
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.resultv.android.R
import com.resultv.android.theme.Brand
import com.resultv.android.vpn.PingRepository
import com.resultv.android.vpn.Profile

/**
 * Sort orderings shared by Home dropdown and Proxies list. "Default" keeps
 * whatever order the caller already arranged (favourites-first, source order
 * within a subscription, etc.); the others apply a stable sort over the
 * caller's pre-sorted list so favourites still bubble to the top.
 */
enum class ProfileSortMode(@StringRes val labelRes: Int) {
    Default(R.string.sort_default),
    Ping(R.string.sort_ping),
    Country(R.string.sort_country),
    Type(R.string.sort_type),
    Name(R.string.sort_name),
}

/**
 * Apply [mode] to [profiles]. SECTION rows are kept in place — they're
 * labels that anchor the row block beneath them. Favourites stay pinned
 * regardless of [mode] (matches the desktop behaviour and the existing
 * favourites-first sort on Proxies).
 */
fun sortProfiles(
    profiles: List<Profile>,
    mode: ProfileSortMode,
    pings: Map<String, PingRepository.Sample>,
): List<Profile> {
    // Favourites are pinned to the top regardless of [mode] — Default keeps
    // the caller's order otherwise, so a stable sort moves only the
    // favourites and leaves the rest of the list untouched.
    val favFirst = compareByDescending<Profile> { it.isFavorite }
    if (mode == ProfileSortMode.Default) {
        return profiles.sortedWith(favFirst)
    }
    val comparator: Comparator<Profile> = when (mode) {
        ProfileSortMode.Ping -> compareBy<Profile> {
            // Unreachable / unknown → push to the bottom.
            val s = pings[it.id]
            if (s == null || !s.reachable) Int.MAX_VALUE else s.latencyMs
        }
        ProfileSortMode.Country -> compareBy(nullsLast()) { it.country }
        ProfileSortMode.Type -> compareBy { it.rawType.ifBlank { "zzz" } }
        ProfileSortMode.Name -> compareBy(String.CASE_INSENSITIVE_ORDER) { it.name }
        else -> return profiles.sortedWith(favFirst)
    }
    return profiles.sortedWith(favFirst.then(comparator))
}

/** Compact sort icon-button + dropdown menu. */
@Composable
fun ProfileSortMenu(
    mode: ProfileSortMode,
    onModeChange: (ProfileSortMode) -> Unit,
    modifier: Modifier = Modifier,
) {
    var expanded by remember { mutableStateOf(false) }
    // Wrap in Box so the DropdownMenu anchors under the IconButton instead
    // of jumping to the top-left of the current popup window — `DropdownMenu`
    // attaches its popup to the nearest non-Box parent's bounds, which
    // when used as a direct child of a wide Row collapses to (0, 0).
    Box(modifier = modifier) {
        IconButton(
            onClick = { expanded = true },
            modifier = Modifier.size(36.dp),
        ) {
            Icon(
                imageVector = Icons.Outlined.Sort,
                contentDescription = stringResource(R.string.sort_cd),
                tint = Brand.SecondaryText,
                modifier = Modifier.size(20.dp),
            )
        }
        DropdownMenu(
            expanded = expanded,
            onDismissRequest = { expanded = false },
        ) {
            ProfileSortMode.entries.forEach { entry ->
                DropdownMenuItem(
                    text = {
                        Text(
                            text = stringResource(entry.labelRes),
                            color = if (entry == mode) Brand.GreenLight else MaterialTheme.colorScheme.onBackground,
                        )
                    },
                    onClick = {
                        onModeChange(entry)
                        expanded = false
                    },
                )
            }
        }
    }
}
