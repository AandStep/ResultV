package com.resultv.android.ui.components

import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.animateDpAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.PowerSettingsNew
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.compose.ui.draw.shadow
import com.resultv.android.R
import com.resultv.android.theme.Brand
import com.resultv.android.vpn.VpnStatus

/**
 * Big circular Connect/Disconnect button with status-driven glow.
 *
 * Compose has no direct M3 primitive for an oversized circular action with
 * an animated halo, so this is custom — but everything inside (Icon, Surface,
 * progress) uses M3.
 */
@Composable
fun PowerButton(
    status: VpnStatus,
    enabled: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val connected = status is VpnStatus.Connected
    val connecting = status is VpnStatus.Connecting
    val errored = status is VpnStatus.Error

    // Desktop reference (HomeView.jsx, lines 197–224):
    //
    //   connected → bg-[#007E3A] text-zinc-950, NO border, shadow-[#007E3A]/50
    //   connecting → from-zinc-800 to-zinc-900, border-amber-500, text-amber-400, shadow-amber-500/30
    //   error → bg-zinc-900, border-rose-500/50, text-rose-500, shadow-rose-500/20
    //   idle → from-zinc-800 to-zinc-900, border-zinc-800, text-zinc-400, shadow-2xl
    //
    // The halo is a separate blurred disc behind the button:
    //   connected → bg-[#007E3A]/40, error → bg-rose-500/20 (pulsing), else → transparent.

    val fillColor by animateColorAsState(
        targetValue = when {
            connected -> Brand.Green
            errored -> Brand.Surface
            else -> Brand.SurfaceHigh   // connecting + idle
        },
        animationSpec = tween(350),
        label = "fillColor",
    )
    // Border: connected has no visible border (collapses to fill), the rest
    // get a 4dp ring in their state colour.
    val borderColor by animateColorAsState(
        targetValue = when {
            connected -> Brand.Green
            connecting -> Brand.Warning
            errored -> Brand.Danger.copy(alpha = 0.50f)
            else -> Brand.SurfaceHigh
        },
        animationSpec = tween(350),
        label = "borderColor",
    )
    val borderWidth = if (connected) 0.dp else 4.dp
    val iconTint by animateColorAsState(
        targetValue = when {
            connected -> Color.Black.copy(alpha = 0.78f)  // text-zinc-950 over green
            connecting -> Brand.Favorite                  // amber-400
            errored -> Brand.Danger
            else -> Brand.SecondaryText                   // text-zinc-400
        },
        animationSpec = tween(350),
        label = "iconTint",
    )
    // Glow uses a radial-gradient brush (NOT a blurred opaque disc): the
    // gradient fades smoothly to Color.Transparent so there's no visible
    // halo edge. Matches desktop's `shadow-2xl shadow-[#007E3A]/50`, which
    // is a feathered drop shadow without a discrete inner ring.
    val glowCenter = when {
        connected -> Brand.Green.copy(alpha = 0.55f)
        errored -> Brand.Danger.copy(alpha = 0.30f)
        connecting -> Brand.Warning.copy(alpha = 0.40f)
        else -> Color.Black.copy(alpha = 0.20f)
    }
    val glowSize by animateDpAsState(
        // Container is 240dp now; cap the glow to match so the halo doesn't
        // pad the parent Column with invisible dead space.
        targetValue = if (connected || errored || connecting) 240.dp else 220.dp,
        animationSpec = tween(600),
        label = "glow",
    )

    Box(
        modifier = modifier.size(240.dp),
        contentAlignment = Alignment.Center,
    ) {
        // Halo behind the button.
        Box(
            modifier = Modifier
                .size(glowSize)
                .background(
                    Brush.radialGradient(
                        // Strong at the centre, fully transparent at the edge.
                        // Stops shape the falloff curve — most of the glow
                        // sits in the inner ~60% so the disc looks dense
                        // near the button and dissolves into background.
                        colorStops = arrayOf(
                            0f to glowCenter,
                            0.35f to glowCenter.copy(alpha = glowCenter.alpha * 0.55f),
                            0.7f to glowCenter.copy(alpha = glowCenter.alpha * 0.15f),
                            1f to Color.Transparent,
                        ),
                    ),
                    CircleShape,
                ),
        )
        // Main button surface.
        Surface(
            onClick = onClick,
            enabled = enabled,
            shape = CircleShape,
            color = fillColor,
            contentColor = iconTint,
            modifier = Modifier
                .size(200.dp)
                .shadow(
                    elevation = if (connected) 0.dp else 24.dp,
                    shape = CircleShape,
                    spotColor = glowCenter,
                    ambientColor = glowCenter,
                )
                .then(
                    if (borderWidth > 0.dp)
                        Modifier.border(borderWidth, borderColor, CircleShape)
                    else Modifier
                ),
        ) {
            Box(contentAlignment = Alignment.Center) {
                Icon(
                    imageVector = Icons.Filled.PowerSettingsNew,
                    contentDescription = stringResource(
                        if (connected) R.string.action_disconnect else R.string.action_connect,
                    ),
                    modifier = Modifier.size(86.dp),
                )
                if (connecting) {
                    CircularProgressIndicator(
                        modifier = Modifier.size(192.dp),
                        color = Brand.Warning,
                        strokeWidth = 3.dp,
                        trackColor = Brand.Warning.copy(alpha = 0.30f),
                    )
                }
            }
        }
    }
}

/** Status text shown above the power button. */
@Composable
fun StatusHeader(status: VpnStatus, activeProfileName: String?) {
    val color = when (status) {
        is VpnStatus.Connected -> Brand.Green
        is VpnStatus.Connecting -> Brand.Warning
        is VpnStatus.Error -> Brand.Danger
        is VpnStatus.Idle -> Brand.SecondaryText
    }
    val title = when (status) {
        is VpnStatus.Connected -> stringResource(R.string.status_protected)
        is VpnStatus.Connecting -> stringResource(R.string.status_connecting)
        is VpnStatus.Error -> stringResource(R.string.status_error)
        is VpnStatus.Idle -> stringResource(R.string.status_unprotected)
    }
    val subtitle = when (status) {
        is VpnStatus.Connected -> activeProfileName?.let {
            stringResource(R.string.status_traffic_routed_via, it)
        }
        is VpnStatus.Error -> status.message
        else -> stringResource(R.string.status_unprotected_subtitle)
    }
    androidx.compose.foundation.layout.Column(
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        androidx.compose.material3.Text(
            text = title,
            style = MaterialTheme.typography.headlineMedium,
            color = color,
        )
        if (subtitle != null) {
            androidx.compose.material3.Text(
                text = subtitle,
                style = MaterialTheme.typography.bodySmall,
                color = Brand.SecondaryText,
            )
        }
    }
}
