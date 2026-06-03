package com.resultv.android.ui.components

import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.CloudDownload
import androidx.compose.material3.Icon
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.resultv.android.R
import com.resultv.android.theme.Brand
import com.resultv.android.vpn.Subscription
import com.resultv.android.vpn.decodePanelTitle

/**
 * Square subscription avatar shared by the Proxies list and the Home group
 * headers. Shows the impVPN brand artwork when [usesImpLogo], otherwise a
 * neutral cloud-download glyph.
 */
@Composable
fun SubscriptionLogo(usesImpLogo: Boolean, size: Dp = 40.dp) {
    Box(
        modifier = Modifier
            .size(size)
            .clip(RoundedCornerShape(12.dp))
            .background(
                if (usesImpLogo) Brand.Green.copy(alpha = 0.18f)
                else Color.White.copy(alpha = 0.07f)
            ),
        contentAlignment = Alignment.Center,
    ) {
        if (usesImpLogo) {
            Image(
                painter = painterResource(R.drawable.imp_logo),
                contentDescription = null,
                contentScale = ContentScale.Fit,
                modifier = Modifier.size(size * 0.8f),
            )
        } else {
            Icon(
                imageVector = Icons.Outlined.CloudDownload,
                contentDescription = null,
                tint = Brand.SecondaryText,
                modifier = Modifier.size(size * 0.55f),
            )
        }
    }
}

/**
 * impVPN logo override: pulled when the subscription came from a
 * `resultv://rvsub/…` deep link, or when the (decoded) name / title / URL
 * mentions impVPN. Mirrors `subscriptionUsesImpLogo` on the PC side.
 * `displayName` already runs `base64:` decoding, so panels that wrap their
 * Profile-Title in base64 still light up the logo.
 */
fun subscriptionUsesImpLogo(s: Subscription): Boolean {
    if (s.source == "rvsub") return true
    val haystack = buildString {
        append(s.displayName).append(' ')
        append(decodePanelTitle(s.name)).append(' ')
        append(decodePanelTitle(s.title)).append(' ')
        append(s.url)
    }.lowercase()
    return "impvpn" in haystack || "imp vpn" in haystack
}
