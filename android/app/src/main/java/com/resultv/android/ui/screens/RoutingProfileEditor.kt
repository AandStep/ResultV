package com.resultv.android.ui.screens

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowForward
import androidx.compose.material.icons.automirrored.outlined.KeyboardArrowRight
import androidx.compose.material.icons.outlined.AltRoute
import androidx.compose.material.icons.outlined.HighlightOff
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material.icons.outlined.Public
import androidx.compose.material.icons.outlined.Tune
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.resultv.android.R
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.RvSpace
import com.resultv.android.theme.SegoeUi
import com.resultv.android.ui.components.RvButton
import com.resultv.android.ui.components.RvButtonColors
import com.resultv.android.ui.components.RvButtonLabel
import com.resultv.android.vpn.ROUTING_ACTIONS
import com.resultv.android.vpn.RoutingProfile
import com.resultv.android.vpn.normalizeRouteOrder
import com.resultv.android.vpn.routingLinesOf
import com.resultv.android.vpn.routingTokensOf

/*
 * Правка профиля. Одна форма в двух ролях — «Добавление» и «Изменение»,
 * отличаются заголовок, подпись и значок шапки (её рисует SettingsSheet).
 *
 * Оболочка своя, телефонная: третья шторка поверх профилей, а не отдельный
 * экран. Закрылась — и ты в списке профилей, откуда её открыл.
 */

private fun actionLabelRes(action: String): Int = when (action) {
    "direct" -> R.string.routing_editor_direct
    "proxy" -> R.string.routing_editor_proxy
    else -> R.string.routing_editor_block
}

private fun actionColor(action: String): Color = when (action) {
    "direct" -> RvColor.Main
    "proxy" -> RvColor.Second
    else -> RvColor.Errors
}

private fun actionIcon(action: String): ImageVector = when (action) {
    "direct" -> Icons.AutoMirrored.Outlined.ArrowForward
    "proxy" -> Icons.Outlined.AltRoute
    else -> Icons.Outlined.HighlightOff
}

private val RowLabel = TextStyle(fontFamily = SegoeUi, fontSize = 12.sp, lineHeight = 15.6.sp, fontWeight = FontWeight.Bold)
private val RowValue = TextStyle(fontFamily = SegoeUi, fontSize = 12.sp, lineHeight = 15.6.sp, fontWeight = FontWeight.SemiBold)
private val FieldText = TextStyle(fontFamily = SegoeUi, fontSize = 12.sp, lineHeight = 16.8.sp, fontWeight = FontWeight.SemiBold)

/**
 * Содержимое редактора профиля — мобильный макет (Figma 6884:5039): группы
 * «Основное», Direct / Proxy / Block (подпись цветом действия, строки
 * «Домены» и «IP адреса» со счётчиком), «Стратегия», «Geo данные» и
 * «Действия» — «Сбросить» и «Сохранить». Шапку рисует SettingsSheet.
 */
@Composable
fun RoutingProfileEditorContent(
    profile: RoutingProfile?,
    busy: Boolean,
    onSave: (RoutingProfile) -> Unit,
) {
    // «Сбросить» возвращает поля к тому, с чем редактор открыли: всё
    // состояние ниже держится на этом ключе.
    var resetKey by remember { mutableIntStateOf(0) }
    var name by remember(profile, resetKey) { mutableStateOf(profile?.name.orEmpty()) }
    var geoip by remember(profile, resetKey) { mutableStateOf(profile?.geoipUrl.orEmpty()) }
    var geosite by remember(profile, resetKey) { mutableStateOf(profile?.geositeUrl.orEmpty()) }

    // Ключи полей: "<действие>-sites" и "<действие>-ips", как на ПК.
    val fields = remember(profile, resetKey) {
        mutableStateMapOf(
            "direct-sites" to routingLinesOf(profile?.directSites.orEmpty()),
            "direct-ips" to routingLinesOf(profile?.directIp.orEmpty()),
            "proxy-sites" to routingLinesOf(profile?.proxySites.orEmpty()),
            "proxy-ips" to routingLinesOf(profile?.proxyIp.orEmpty()),
            "block-sites" to routingLinesOf(profile?.blockSites.orEmpty()),
            "block-ips" to routingLinesOf(profile?.blockIp.orEmpty()),
        )
    }
    val opened = remember(profile) { mutableStateMapOf<String, Boolean>() }
    val order = remember(profile, resetKey) {
        mutableStateListOf<String>().apply {
            val stored = profile?.routeOrder.orEmpty().split("-").filter { it.isNotBlank() }
            addAll(
                if (stored.toSet() == ROUTING_ACTIONS.toSet()) stored
                else listOf("block", "proxy", "direct")
            )
        }
    }

    val empty = fields.values.all { routingTokensOf(it).isEmpty() }

    SheetGroup(stringResource(R.string.routing_editor_basic), Icons.Outlined.Info) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(14.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2),
        ) {
            Text(stringResource(R.string.routing_editor_name), style = RowLabel, color = RvColor.White)
            BasicTextField(
                value = name,
                onValueChange = { name = it },
                singleLine = true,
                textStyle = RowValue.copy(color = RvColor.White, textAlign = TextAlign.End),
                cursorBrush = SolidColor(RvColor.whiteA50),
                modifier = Modifier.weight(1f),
                decorationBox = { inner ->
                    Box(contentAlignment = Alignment.CenterEnd) {
                        if (name.isEmpty()) {
                            Text(
                                stringResource(R.string.routing_editor_name_hint),
                                style = RowValue,
                                color = RvColor.whiteA20,
                            )
                        }
                        inner()
                    }
                },
            )
        }
    }

    ROUTING_ACTIONS.forEach { action ->
        SheetGroup(stringResource(actionLabelRes(action)), actionIcon(action), labelColor = actionColor(action)) {
            listOf(
                Triple("$action-sites", R.string.routing_editor_domains, R.string.routing_editor_domains_hint),
                Triple("$action-ips", R.string.routing_editor_ips, R.string.routing_editor_ips_hint),
            ).forEachIndexed { index, (key, labelRes, hintRes) ->
                if (index > 0) SheetDivider()
                RuleRow(
                    label = stringResource(labelRes),
                    hint = stringResource(hintRes),
                    color = actionColor(action),
                    value = fields[key].orEmpty(),
                    onChange = { fields[key] = it },
                    open = opened[key] == true,
                    onToggle = { opened[key] = opened[key] != true },
                )
            }
        }
    }

    SheetGroup(stringResource(R.string.routing_editor_strategy), Icons.Outlined.Tune) {
        Column(
            modifier = Modifier.fillMaxWidth().padding(14.dp),
            verticalArrangement = Arrangement.spacedBy(RvSpace.nest2),
        ) {
            Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(stringResource(R.string.routing_editor_order), style = RowLabel, color = RvColor.White)
                Text(stringResource(R.string.routing_editor_strategy_hint), style = SheetNoteStyle, color = RvColor.whiteA50)
            }
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(RvSpace.nest3)) {
                order.forEachIndexed { index, action ->
                    if (index > 0) {
                        Icon(
                            Icons.AutoMirrored.Outlined.ArrowForward,
                            contentDescription = null,
                            tint = RvColor.whiteA50,
                            modifier = Modifier.size(14.dp),
                        )
                    }
                    // На ПК порядок меняют перетаскиванием. Ради трёх меток на
                    // телефоне это лишняя механика: нажатие отправляет метку в
                    // конец, порядок собирается теми же тремя движениями.
                    Text(
                        action.replaceFirstChar { it.uppercase() },
                        style = RowLabel,
                        color = actionColor(action),
                        modifier = Modifier
                            .clip(CircleShape)
                            .background(actionColor(action).copy(alpha = 0.1f))
                            .clickable(enabled = !busy) {
                                order.remove(action)
                                order.add(action)
                            }
                            .padding(horizontal = RvSpace.nest2, vertical = 6.dp),
                    )
                }
            }
        }
    }

    SheetGroup(stringResource(R.string.routing_editor_geo), Icons.Outlined.Public) {
        GeoRow(stringResource(R.string.routing_editor_geoip), geoip) { geoip = it }
        SheetDivider()
        GeoRow(stringResource(R.string.routing_editor_geosite), geosite) { geosite = it }
    }

    Column(verticalArrangement = Arrangement.spacedBy(RvSpace.nest3)) {
        Text(
            stringResource(R.string.routing_profiles_actions),
            style = RowValue,
            color = RvColor.whiteA50,
            modifier = Modifier.padding(start = RvSpace.xs),
        )
        Row(horizontalArrangement = Arrangement.spacedBy(RvSpace.nest3)) {
            RvButton(
                onClick = { resetKey++ },
                fill = RvColor.Black,
                outline = RvColor.whiteA10,
                enabled = !busy,
                modifier = Modifier.weight(1f),
            ) {
                Text(stringResource(R.string.routing_editor_reset), style = RvButtonLabel, fontWeight = FontWeight.Bold, color = RvColor.White)
            }
            RvButton(
                onClick = {
                    val base = profile ?: RoutingProfile(id = "", name = "", source = "manual")
                    onSave(
                        base.copy(
                            name = name.trim(),
                            directSites = routingTokensOf(fields["direct-sites"].orEmpty()),
                            directIp = routingTokensOf(fields["direct-ips"].orEmpty()),
                            proxySites = routingTokensOf(fields["proxy-sites"].orEmpty()),
                            proxyIp = routingTokensOf(fields["proxy-ips"].orEmpty()),
                            blockSites = routingTokensOf(fields["block-sites"].orEmpty()),
                            blockIp = routingTokensOf(fields["block-ips"].orEmpty()),
                            routeOrder = normalizeRouteOrder(order.toList()),
                            geoipUrl = geoip.trim(),
                            geositeUrl = geosite.trim(),
                            // Время правки — как SaveRoutingProfile на ПК.
                            // Профиль из диплинка несёт сюда штамп издателя, у
                            // правки руками его взять неоткуда.
                            updatedAt = System.currentTimeMillis() / 1000,
                            lastError = "",
                        )
                    )
                },
                fill = RvButtonColors.greenFill,
                outline = RvButtonColors.greenOutline,
                // Профиль без имени или без единого правила сохранять нечего —
                // Go его всё равно отклонит, и лучше это видно до нажатия.
                enabled = !busy && name.isNotBlank() && !empty,
                modifier = Modifier.weight(1f),
            ) {
                Text(stringResource(R.string.action_save), style = RvButtonLabel, fontWeight = FontWeight.Bold, color = RvColor.Main)
            }
        }
    }
}

/**
 * Строка правил «Домены» / «IP адреса»: счётчик капсулой цвета действия и
 * шеврон; по нажатию раскрывается поле, одно правило на строку. Чипы здесь не
 * годятся: профиль может нести до 20 000 токенов.
 */
@Composable
private fun RuleRow(
    label: String,
    hint: String,
    color: Color,
    value: String,
    onChange: (String) -> Unit,
    open: Boolean,
    onToggle: () -> Unit,
) {
    Column(modifier = Modifier.fillMaxWidth()) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .clickable(onClick = onToggle)
                .padding(14.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(RvSpace.nest3),
        ) {
            Text(label, style = RowLabel, color = RvColor.whiteA80, modifier = Modifier.weight(1f))
            // Сколько правил внутри — видно, не раскрывая строку.
            val count = routingTokensOf(value).size
            if (count > 0) {
                Box(
                    modifier = Modifier
                        .clip(CircleShape)
                        .background(color.copy(alpha = 0.1f))
                        .padding(horizontal = 7.dp, vertical = 2.dp),
                ) {
                    Text("$count", style = SheetNoteStyle, color = color)
                }
            }
            Icon(
                Icons.AutoMirrored.Outlined.KeyboardArrowRight,
                contentDescription = null,
                tint = RvColor.whiteA50,
                modifier = Modifier.size(20.dp).rotate(if (open) 90f else 0f),
            )
        }
        AnimatedVisibility(visible = open) {
            val shape = RoundedCornerShape(16.dp)
            BasicTextField(
                value = value,
                onValueChange = onChange,
                textStyle = FieldText.copy(color = RvColor.White),
                cursorBrush = SolidColor(RvColor.whiteA50),
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(start = 14.dp, end = 14.dp, bottom = 14.dp)
                    // Потолок высоты, а не рост по содержимому: у профиля
                    // подписки 115 доменов, и раскрытая строка занимала бы
                    // несколько экранов. Выше потолка текст листается внутри.
                    .heightIn(min = 110.dp, max = 220.dp),
                decorationBox = { inner ->
                    Box(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(shape)
                            .background(RvColor.DarkGrey)
                            .padding(12.dp),
                    ) {
                        if (value.isEmpty()) Text(hint, style = FieldText, color = RvColor.whiteA20)
                        inner()
                    }
                },
            )
        }
    }
}

/** Строка Geo: подпись 12 Bold и поле адреса под ней. */
@Composable
private fun GeoRow(label: String, value: String, onChange: (String) -> Unit) {
    Column(
        modifier = Modifier.fillMaxWidth().padding(14.dp),
        verticalArrangement = Arrangement.spacedBy(RvSpace.nest2),
    ) {
        Text(label, style = RowLabel, color = RvColor.whiteA80)
        SheetField(
            value = value,
            onValueChange = onChange,
            placeholder = "https://example.com",
            keyboardType = KeyboardType.Uri,
        )
    }
}
