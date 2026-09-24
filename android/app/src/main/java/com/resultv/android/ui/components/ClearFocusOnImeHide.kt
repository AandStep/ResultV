package com.resultv.android.ui.components

import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.isImeVisible
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalFocusManager

/**
 * Снимает фокус с поля, когда клавиатуру убрали — жестом «назад» или
 * кнопкой системы. Без этого поле оставалось в фокусе: мигал курсор,
 * горела рамка фокуса, и следующий тап по нему не поднимал клавиатуру.
 *
 * Фокус у каждого окна свой, поэтому вызов нужен в корне приложения и в
 * каждой шторке (ModalBottomSheet — отдельное окно со своим менеджером).
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun ClearFocusOnImeHide() {
    val focusManager = LocalFocusManager.current
    val imeVisible = WindowInsets.isImeVisible
    var wasVisible by remember { mutableStateOf(false) }
    LaunchedEffect(imeVisible) {
        if (wasVisible && !imeVisible) focusManager.clearFocus()
        wasVisible = imeVisible
    }
}
