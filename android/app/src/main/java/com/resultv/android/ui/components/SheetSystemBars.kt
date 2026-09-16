package com.resultv.android.ui.components

import android.os.Build
import android.view.View
import android.view.Window
import androidx.compose.runtime.Composable
import androidx.compose.runtime.SideEffect
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.window.DialogWindowProvider
import androidx.core.view.WindowCompat

/**
 * Приводит системные панели окна `ModalBottomSheet` к тому же виду, что
 * `MainActivity` ставит активити через
 * `enableEdgeToEdge(SystemBarStyle.dark(TRANSPARENT))`.
 *
 * Лист живёт в собственном окне-диалоге, и стиль панелей активити на него не
 * распространяется. Своя тема у листа — `EdgeToEdgeFloatingDialogTheme`
 * (material3), а её родитель `android:Theme.DeviceDefault.Dialog` следует
 * системной светлой/тёмной теме. На устройстве в светлой теме она поднимает
 * `windowLightNavigationBar`, и под прозрачной панелью система рисует светлый
 * контраст-скрим — белую полосу под всегда тёмным листом. Цвета самих панелей
 * в теме уже `transparent`, поэтому чинится именно appearance.
 *
 * Вызывать первым в содержимом листа.
 */
// isStatusBarContrastEnforced / isNavigationBarContrastEnforced помечены
// deprecated вместе со всем до-edge-to-edge API в API 35; замены у них нет,
// и AndroidX сам дёргает их внутри enableEdgeToEdge(). На API 35+ они уже
// no-op, но сборка ездит с minSdk 26.
@Suppress("DEPRECATION")
@Composable
fun DarkSheetSystemBars() {
    val view = LocalView.current
    val window = view.dialogWindow() ?: return
    SideEffect {
        WindowCompat.getInsetsController(window, view).apply {
            isAppearanceLightStatusBars = false
            isAppearanceLightNavigationBars = false
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            // enableEdgeToEdge(SystemBarStyle.dark) снимает то же самое с окна
            // активити: скрим не нужен, панели и так лежат на тёмном листе.
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
