package com.resultv.android.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material3.BottomSheetDefaults
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.SheetState
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.resultv.android.R
import com.resultv.android.theme.CategoryTint
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.rvBorder
import com.resultv.android.theme.RvSpace
import com.resultv.android.theme.SegoeUi
import com.resultv.android.ui.components.ClearFocusOnImeHide
import com.resultv.android.ui.components.DarkSheetSystemBars
import com.resultv.android.ui.components.RvButton
import com.resultv.android.ui.components.RvButtonColors
import com.resultv.android.ui.components.RvButtonLabel

/*
 * Детали шторок настроек — по эталону «Сеть» мобильного макета (Figma
 * 6879:5047): шапка с круглой плиткой 48 и крестиком 32, группы «подпись
 * над карточкой», строки с плиткой 32, тумблер 46×26, чипы 52, поле 44 и
 * выпадающий выбор капсулой. Шторки собираются из них, а не рисуют каждая
 * своё.
 */

private val SheetTitle = TextStyle(fontFamily = SegoeUi, fontSize = 14.sp, lineHeight = 16.8.sp, fontWeight = FontWeight.Bold)
private val SheetDesc = TextStyle(fontFamily = SegoeUi, fontSize = 10.sp, lineHeight = 12.sp, fontWeight = FontWeight.SemiBold)
private val GroupLabelStyle = TextStyle(fontFamily = SegoeUi, fontSize = 12.sp, lineHeight = 14.4.sp, fontWeight = FontWeight.SemiBold)
private val RowTitle = TextStyle(fontFamily = SegoeUi, fontSize = 12.sp, lineHeight = 15.6.sp, fontWeight = FontWeight.Bold)
internal val SheetNoteStyle = TextStyle(fontFamily = SegoeUi, fontSize = 10.sp, lineHeight = 12.sp, fontWeight = FontWeight.SemiBold)
private val FieldStyle = TextStyle(fontFamily = SegoeUi, fontSize = 12.sp, lineHeight = 16.8.sp, fontWeight = FontWeight.SemiBold)

/** Шторка раздела: Black, шапка раздела, группы через 20. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun SettingsSheet(
    icon: ImageVector,
    tint: CategoryTint,
    title: String,
    description: String,
    sheetState: SheetState,
    onDismiss: () -> Unit,
    content: @Composable ColumnScope.() -> Unit,
) {
    ModalBottomSheet(
        onDismissRequest = onDismiss,
        modifier = Modifier.windowInsetsPadding(WindowInsets.statusBars),
        sheetState = sheetState,
        containerColor = RvColor.Black,
        dragHandle = { BottomSheetDefaults.DragHandle(width = 32.dp, height = 4.dp, color = RvColor.whiteA20) },
    ) {
        DarkSheetSystemBars()
        ClearFocusOnImeHide()
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .verticalScroll(rememberScrollState())
                // Не safe area — её лист держит сам; это поле, чтобы последняя
                // группа не упиралась в панель навигации.
                .padding(start = 12.dp, end = 12.dp, top = 4.dp, bottom = 24.dp),
            verticalArrangement = Arrangement.spacedBy(20.dp),
        ) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2),
            ) {
                Box(
                    modifier = Modifier.size(48.dp).clip(CircleShape).background(tint.tile),
                    contentAlignment = Alignment.Center,
                ) {
                    Icon(icon, contentDescription = null, tint = tint.glyph, modifier = Modifier.size(24.dp))
                }
                Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(RvSpace.xs)) {
                    Text(title, style = SheetTitle, color = RvColor.White)
                    Text(description, style = SheetDesc, color = RvColor.whiteA50)
                }
                Box(
                    modifier = Modifier
                        .size(32.dp)
                        .clip(CircleShape)
                        .background(RvColor.Grey)
                        .clickable(role = Role.Button, onClick = onDismiss),
                    contentAlignment = Alignment.Center,
                ) {
                    Icon(
                        Icons.Outlined.Close,
                        contentDescription = stringResource(R.string.action_close),
                        tint = RvColor.whiteA50,
                        modifier = Modifier.size(14.dp),
                    )
                }
            }
            content()
        }
    }
}

/** Подпись группы: значок 14 и текст 12 Semibold; цвет — белый 50 % или цвет действия. */
@Composable
internal fun SheetGroupLabel(label: String, icon: ImageVector, color: Color = RvColor.whiteA50) {
    Row(
        modifier = Modifier.padding(start = RvSpace.xs),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        Icon(icon, contentDescription = null, tint = color, modifier = Modifier.size(14.dp))
        Text(label, style = GroupLabelStyle, color = color)
    }
}

/** Группа: подпись (значок 14 + текст 12) над карточкой Grey с обводкой белым 6 %. */
@Composable
internal fun SheetGroup(
    label: String,
    icon: ImageVector,
    labelColor: Color = RvColor.whiteA50,
    content: @Composable ColumnScope.() -> Unit,
) {
    Column(verticalArrangement = Arrangement.spacedBy(RvSpace.nest3)) {
        SheetGroupLabel(label, icon, labelColor)
        val shape = RoundedCornerShape(20.dp)
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .clip(shape)
                .background(RvColor.Grey)
                .rvBorder(shape),
            content = content,
        )
    }
}

/** Разделитель строк внутри одной карточки группы. */
@Composable
internal fun SheetDivider() {
    HorizontalDivider(color = RvColor.whiteA06)
}

/**
 * Строка группы: плитка 32 (глиф 18), заголовок 12 Bold и подпись 10
 * Semibold, справа — [trailing]; всё, что под строкой (чипы, поле,
 * заметка), — в [below], через 12.
 */
@Composable
internal fun SheetRow(
    icon: ImageVector,
    tint: CategoryTint,
    title: String,
    subtitle: String,
    modifier: Modifier = Modifier,
    trailing: (@Composable () -> Unit)? = null,
    below: (@Composable ColumnScope.() -> Unit)? = null,
) {
    Column(
        modifier = modifier.fillMaxWidth().padding(14.dp),
        verticalArrangement = Arrangement.spacedBy(RvSpace.nest2),
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2),
        ) {
            Box(
                modifier = Modifier.size(32.dp).clip(RoundedCornerShape(8.dp)).background(tint.tile),
                contentAlignment = Alignment.Center,
            ) {
                Icon(icon, contentDescription = null, tint = tint.glyph, modifier = Modifier.size(18.dp))
            }
            Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(title, style = RowTitle, color = RvColor.White)
                Text(subtitle, style = SheetDesc, color = RvColor.whiteA50)
            }
            trailing?.invoke()
        }
        below?.invoke(this)
    }
}

/** Строка с тумблером; нажимается вся строка, а не только тумблер. */
@Composable
internal fun SheetToggleRow(
    icon: ImageVector,
    tint: CategoryTint,
    title: String,
    subtitle: String,
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
    enabled: Boolean = true,
    below: (@Composable ColumnScope.() -> Unit)? = null,
) {
    SheetRow(
        icon = icon,
        tint = tint,
        title = title,
        subtitle = subtitle,
        modifier = Modifier.toggleable(
            value = checked,
            enabled = enabled,
            role = Role.Switch,
            onValueChange = onCheckedChange,
        ),
        trailing = { RvSwitch(checked = checked) },
        below = below,
    )
}

/**
 * Тумблер макета (Tumbler, Figma 6862:5637): 46×26, Grey с обводкой белым
 * 10 %, бегунок 16 — справа и Main во включённом, слева и серый в выключенном.
 * Сам не нажимается: жест держит строка.
 */
@Composable
internal fun RvSwitch(checked: Boolean) {
    Box(
        modifier = Modifier
            .width(46.dp)
            .height(26.dp)
            .clip(CircleShape)
            .background(RvColor.Grey)
            .rvBorder(CircleShape)
            .padding(4.dp),
        contentAlignment = if (checked) Alignment.CenterEnd else Alignment.CenterStart,
    ) {
        Box(
            modifier = Modifier
                .size(16.dp)
                .clip(CircleShape)
                .background(if (checked) RvColor.Main else RvColor.whiteA20),
        )
    }
}

/**
 * Ряд чипов выбора (пресеты DNS, тип замера): чипы 52, прокрутка вбок,
 * затухание 48 справа, пока есть что листать. Выбранный — Main 50 %,
 * остальные — прозрачные с белой обводкой 10 %.
 */
@Composable
internal fun <T> SheetChips(options: List<Pair<T, String>>, selected: T, onSelect: (T) -> Unit) {
    val scroll = rememberScrollState()
    Box(modifier = Modifier.fillMaxWidth()) {
        Row(
            modifier = Modifier.horizontalScroll(scroll),
            horizontalArrangement = Arrangement.spacedBy(RvSpace.nest3),
        ) {
            options.forEach { (key, label) ->
                val on = key == selected
                RvButton(
                    onClick = { onSelect(key) },
                    fill = if (on) RvButtonColors.greenSelected else Color.Transparent,
                    outline = if (on) RvButtonColors.greenOutline else RvButtonColors.greyOutline,
                ) {
                    Text(
                        label,
                        style = RvButtonLabel,
                        fontWeight = FontWeight.Bold,
                        color = if (on) RvColor.Main else RvColor.White,
                        modifier = Modifier.padding(horizontal = 28.dp),
                    )
                }
            }
        }
        if (scroll.canScrollForward) {
            Box(
                modifier = Modifier
                    .align(Alignment.CenterEnd)
                    .width(48.dp)
                    .height(52.dp)
                    .background(Brush.horizontalGradient(listOf(Color.Transparent, RvColor.Grey))),
            )
        }
    }
}

/** Поле ввода макета: Dark Grey, 44, скругление 16, текст 12 Semibold. */
@Composable
internal fun SheetField(
    value: String,
    onValueChange: (String) -> Unit,
    placeholder: String,
    isError: Boolean = false,
    keyboardType: KeyboardType = KeyboardType.Text,
) {
    val shape = RoundedCornerShape(16.dp)
    BasicTextField(
        value = value,
        onValueChange = onValueChange,
        singleLine = true,
        textStyle = FieldStyle.copy(color = RvColor.White),
        cursorBrush = SolidColor(RvColor.whiteA50),
        keyboardOptions = KeyboardOptions(keyboardType = keyboardType),
        modifier = Modifier.fillMaxWidth().height(44.dp),
        decorationBox = { inner ->
            Box(
                modifier = Modifier
                    .fillMaxSize()
                    .clip(shape)
                    .background(RvColor.DarkGrey)
                    .rvBorder(if (isError) RvColor.errorsA50 else Color.Transparent, shape)
                    .padding(horizontal = 14.dp),
                contentAlignment = Alignment.CenterStart,
            ) {
                if (value.isEmpty()) Text(placeholder, style = FieldStyle, color = RvColor.whiteA20)
                inner()
            }
        },
    )
}

/**
 * Выпадающий выбор («2ч ▾», «Русский ▾») — Figma 6882:5272: плашка Grey
 * высотой 36, скругление 12, общая белая обводка, поля 12/8, текст 12 Bold
 * белым, стрелка вниз 20 dp белым 50 %.
 */
@Composable
internal fun <T> SheetDropdown(
    value: T,
    options: List<Pair<T, String>>,
    onSelect: (T) -> Unit,
) {
    var open by remember { mutableStateOf(false) }
    val current = options.firstOrNull { it.first == value }?.second ?: value.toString()
    val shape = RoundedCornerShape(12.dp)
    Box {
        Row(
            modifier = Modifier
                .height(36.dp)
                .clip(shape)
                .background(RvColor.Grey)
                .rvBorder(shape)
                .clickable(role = Role.DropdownList) { open = true }
                .padding(start = 12.dp, end = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            Text(current, style = RvButtonLabel, fontWeight = FontWeight.Bold, color = RvColor.White)
            Icon(
                painter = painterResource(R.drawable.ic_menu_arrow),
                contentDescription = null,
                tint = RvColor.whiteA50,
                // Глиф экспорта смотрит вверх, закрытый список — вниз.
                modifier = Modifier.size(20.dp).rotate(180f),
            )
        }
        DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            options.forEach { (key, label) ->
                DropdownMenuItem(
                    text = { Text(label, color = if (key == value) RvColor.Second else RvColor.White) },
                    onClick = { open = false; onSelect(key) },
                )
            }
        }
    }
}

