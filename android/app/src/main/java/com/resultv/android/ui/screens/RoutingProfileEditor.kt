package com.resultv.android.ui.screens

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.ExpandLess
import androidx.compose.material.icons.outlined.ExpandMore
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.resultv.android.R
import com.resultv.android.theme.Brand
import com.resultv.android.theme.CategoryTint
import com.resultv.android.ui.components.SettingIcon
import com.resultv.android.vpn.ROUTING_ACTIONS
import com.resultv.android.vpn.RoutingProfile
import com.resultv.android.vpn.normalizeRouteOrder
import com.resultv.android.vpn.routingLinesOf
import com.resultv.android.vpn.routingTokensOf

/*
 * Правка профиля. Раскладка с ПК (RoutingProfileEditor.jsx, Figma 6648:4105
 * «Добавление профиля» и 6636:4310 «Изменение профиля»): одна форма в двух
 * ролях, отличаются заголовок, подпись и значок. Внутри — название, три
 * раздела действий со складными панелями «Домены» и «IP адреса», «Стратегия»,
 * «Geo данные», «Действия». Раскрыта первая панель, как в макете.
 *
 * Оболочка своя, телефонная: третья шторка поверх профилей, а не отдельный
 * экран. Закрылась — и ты в списке профилей, откуда её открыл.
 *
 * Правила правятся многострочным текстом, одно правило на строку. Чипы здесь
 * не годятся: профиль может нести до 20 000 токенов, и это 20 000 элементов
 * вместо одного.
 */

private val PanelShape = RoundedCornerShape(16.dp)

/**
 * Поле правил скруглено сильнее стандартного: оно лежит ВНУТРИ панели на
 * 16 dp, и прямые углы Material внутри скруглённой коробки читались как
 * чужая деталь.
 */
private val FieldShape = RoundedCornerShape(14.dp)
private val EditorBorder = Color.White.copy(alpha = 0.06f)
private val EditorMuted = Color.White.copy(alpha = 0.50f)

private fun actionLabelRes(action: String): Int = when (action) {
    "direct" -> R.string.routing_editor_direct
    "proxy" -> R.string.routing_editor_proxy
    else -> R.string.routing_editor_block
}

private fun actionColor(action: String): Color = when (action) {
    "direct" -> Brand.Green
    "proxy" -> Brand.GreenLight
    else -> Brand.Danger
}

@Composable
fun RoutingProfileEditorContent(
    profile: RoutingProfile?,
    busy: Boolean,
    onSave: (RoutingProfile) -> Unit,
) {
    val isEdit = profile != null && profile.id.isNotEmpty()
    var name by remember(profile) { mutableStateOf(profile?.name.orEmpty()) }
    var geoip by remember(profile) { mutableStateOf(profile?.geoipUrl.orEmpty()) }
    var geosite by remember(profile) { mutableStateOf(profile?.geositeUrl.orEmpty()) }

    // Ключи полей: "<действие>-sites" и "<действие>-ips", как на ПК.
    val fields = remember(profile) {
        mutableStateMapOf(
            "direct-sites" to routingLinesOf(profile?.directSites.orEmpty()),
            "direct-ips" to routingLinesOf(profile?.directIp.orEmpty()),
            "proxy-sites" to routingLinesOf(profile?.proxySites.orEmpty()),
            "proxy-ips" to routingLinesOf(profile?.proxyIp.orEmpty()),
            "block-sites" to routingLinesOf(profile?.blockSites.orEmpty()),
            "block-ips" to routingLinesOf(profile?.blockIp.orEmpty()),
        )
    }
    // В макете раскрыта первая панель Direct, остальные свёрнуты.
    val opened = remember(profile) { mutableStateMapOf("direct-sites" to true) }
    val order = remember(profile) {
        mutableStateListOf<String>().apply {
            val stored = profile?.routeOrder.orEmpty().split("-").filter { it.isNotBlank() }
            addAll(
                if (stored.toSet() == ROUTING_ACTIONS.toSet()) stored
                else listOf("block", "proxy", "direct")
            )
        }
    }

    val empty = fields.values.all { routingTokensOf(it).isEmpty() }

    Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(14.dp),
        ) {
            SettingIcon(
                icon = if (isEdit) Icons.Outlined.Edit else Icons.Outlined.Add,
                tint = CategoryTint(Brand.Green.copy(alpha = 0.16f), Brand.Green),
            )
            Column {
                Text(
                    stringResource(
                        if (isEdit) R.string.routing_editor_edit_title
                        else R.string.routing_editor_create_title
                    ),
                    style = MaterialTheme.typography.titleLarge,
                    fontWeight = FontWeight.Bold,
                )
                Text(
                    stringResource(
                        if (isEdit) R.string.routing_editor_edit_subtitle
                        else R.string.routing_editor_create_subtitle
                    ),
                    style = MaterialTheme.typography.bodyMedium,
                    color = Brand.SecondaryText,
                )
            }
        }

        EditorSection(stringResource(R.string.routing_editor_name)) {
            OutlinedTextField(
                value = name,
                onValueChange = { name = it },
                placeholder = { Text(stringResource(R.string.routing_editor_name_hint)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
        }

        ROUTING_ACTIONS.forEach { action ->
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(
                    stringResource(actionLabelRes(action)),
                    fontSize = 14.sp,
                    fontWeight = FontWeight.Medium,
                    color = actionColor(action),
                )
                RulePanel(
                    label = stringResource(R.string.routing_editor_domains),
                    hint = stringResource(R.string.routing_editor_domains_hint),
                    value = fields["$action-sites"].orEmpty(),
                    onChange = { fields["$action-sites"] = it },
                    open = opened["$action-sites"] == true,
                    onToggle = { opened["$action-sites"] = opened["$action-sites"] != true },
                )
                RulePanel(
                    label = stringResource(R.string.routing_editor_ips),
                    hint = stringResource(R.string.routing_editor_ips_hint),
                    value = fields["$action-ips"].orEmpty(),
                    onChange = { fields["$action-ips"] = it },
                    open = opened["$action-ips"] == true,
                    onToggle = { opened["$action-ips"] = opened["$action-ips"] != true },
                )
            }
        }

        EditorSection(stringResource(R.string.routing_editor_strategy)) {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                order.forEach { action ->
                    Text(
                        action,
                        fontSize = 14.sp,
                        fontWeight = FontWeight.Medium,
                        color = actionColor(action),
                        modifier = Modifier
                            .clip(RoundedCornerShape(100.dp))
                            .border(1.dp, EditorBorder, RoundedCornerShape(100.dp))
                            // На ПК порядок меняют перетаскиванием. Ради трёх
                            // меток на телефоне это лишняя механика, и попасть
                            // пальцем труднее: нажатие отправляет метку в
                            // конец, порядок собирается теми же тремя
                            // движениями.
                            .clickable(enabled = !busy) {
                                order.remove(action)
                                order.add(action)
                            }
                            .padding(horizontal = 14.dp, vertical = 8.dp),
                    )
                }
            }
            Text(
                stringResource(R.string.routing_editor_strategy_hint),
                fontSize = 13.sp,
                color = EditorMuted,
            )
        }

        EditorSection(stringResource(R.string.routing_editor_geo)) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(PanelShape)
                    .border(1.dp, EditorBorder, PanelShape)
                    .padding(14.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Text(
                    stringResource(R.string.routing_editor_geoip),
                    fontSize = 13.sp,
                    color = EditorMuted,
                )
                OutlinedTextField(
                    value = geoip,
                    onValueChange = { geoip = it },
                    placeholder = { Text("https://example.com") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                Text(
                    stringResource(R.string.routing_editor_geosite),
                    fontSize = 13.sp,
                    color = EditorMuted,
                )
                OutlinedTextField(
                    value = geosite,
                    onValueChange = { geosite = it },
                    placeholder = { Text("https://example.com") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
            }
        }

        EditorSection(stringResource(R.string.routing_profiles_actions)) {
            Button(
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
                // Профиль без имени или без единого правила сохранять нечего —
                // Go его всё равно отклонит, и лучше это видно до нажатия.
                enabled = !busy && name.isNotBlank() && !empty,
                modifier = Modifier.fillMaxWidth().height(52.dp),
                shape = PanelShape,
                colors = ButtonDefaults.buttonColors(
                    containerColor = Brand.Green.copy(alpha = 0.14f),
                    contentColor = Brand.Green,
                ),
            ) {
                Text(
                    stringResource(R.string.action_save),
                    fontSize = 15.sp,
                    fontWeight = FontWeight.Bold,
                )
            }
        }
    }
}

@Composable
private fun EditorSection(label: String, content: @Composable () -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(label, fontSize = 14.sp, color = EditorMuted)
        content()
    }
}

/** Складная панель со списком правил (Figma 6648:4159). */
@Composable
private fun RulePanel(
    label: String,
    hint: String,
    value: String,
    onChange: (String) -> Unit,
    open: Boolean,
    onToggle: () -> Unit,
) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(PanelShape)
            .border(1.dp, EditorBorder, PanelShape),
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .clickable(onClick = onToggle)
                .padding(horizontal = 14.dp, vertical = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(label, fontSize = 14.sp, modifier = Modifier.weight(1f))
            // Сколько правил внутри — видно, не раскрывая панель.
            val count = routingTokensOf(value).size
            if (count > 0) {
                Text("$count", fontSize = 13.sp, color = EditorMuted)
            }
            Icon(
                if (open) Icons.Outlined.ExpandLess else Icons.Outlined.ExpandMore,
                contentDescription = null,
                tint = EditorMuted,
                modifier = Modifier.padding(start = 8.dp),
            )
        }
        AnimatedVisibility(visible = open) {
            OutlinedTextField(
                value = value,
                onValueChange = onChange,
                placeholder = { Text(hint, fontSize = 13.sp) },
                shape = FieldShape,
                modifier = Modifier
                    .fillMaxWidth()
                    // Потолок высоты, а не рост по содержимому: у профиля
                    // подписки 115 доменов, и раскрытая панель занимала
                    // несколько экранов — до «Стратегии» приходилось листать
                    // мимо всего списка. Выше потолка текст прокручивается
                    // внутри поля.
                    .heightIn(min = 110.dp, max = 220.dp)
                    .padding(horizontal = 14.dp)
                    .padding(bottom = 14.dp),
            )
        }
    }
}
