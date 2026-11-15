package com.resultv.android.ui.components

import android.os.Build
import android.view.View
import android.view.Window
import androidx.compose.runtime.Composable
import androidx.compose.runtime.SideEffect
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.window.DialogWindowProvider
import androidx.core.view.WindowCompat
import com.resultv.android.theme.RvColor

/** Applies the active palette's system bar contrast to the sheet's own window. */
@Suppress("DEPRECATION")
@Composable
fun SheetSystemBars() {
    val light = !RvColor.isDark
    val view = LocalView.current
    val window = view.dialogWindow() ?: return
    SideEffect {
        WindowCompat.getInsetsController(window, view).apply {
            isAppearanceLightStatusBars = light
            isAppearanceLightNavigationBars = light
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            window.isStatusBarContrastEnforced = false
            window.isNavigationBarContrastEnforced = false
        }
    }
}

/**
 * Окно диалога, в котором нарисован этот View, или null для обычного
 * содержимого активити. Composable-содержимое листа сидит на несколько
 * уровней ниже [DialogWindowProvider], поэтому цепочка родителей обходится
 * целиком, а не только первый.
 */
private fun View.dialogWindow(): Window? {
    var p = parent
    while (p != null) {
        if (p is DialogWindowProvider) return p.window
        p = p.parent
    }
    return null
}
