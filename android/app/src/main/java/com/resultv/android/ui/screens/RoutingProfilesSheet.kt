package com.resultv.android.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.AltRoute
import androidx.compose.material.icons.outlined.DeleteOutline
import androidx.compose.material.icons.outlined.Public
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.resultv.android.R
import com.resultv.android.theme.Brand
import com.resultv.android.ui.components.SettingIcon
import com.resultv.android.vpn.DeepLinkImporter
import com.resultv.android.vpn.ROUTING_ACTIONS
import com.resultv.android.vpn.RoutingProfile
import com.resultv.android.vpn.RoutingProfileCompiler
import com.resultv.android.vpn.RoutingProfileRepository
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/*
 * Содержимое шторки «Профили маршрутизации».
 *
 * Раскладка взята с ПК (RoutingProfilesDialog.jsx): разделы «Активный профиль»,
 * «Все профили», «Действия», строка профиля со значком, названием, цветными
 * счётчиками и корзиной. Оттуда же правило «ноль не показывается».
 *
 * А вот ОБОЛОЧКА — своя, телефонная. Прошлая версия была переносом окна ПК
 * один в один, и это вылезло тремя способами сразу: полноэкранный маршрут
 * вместо шторки, крестик вместо ручки и свайпа, и — хуже всего — закрытие
 * возвращало в список настроек, а не в «Правила», откуда экран и открывали.
 * Теперь это вложенная шторка поверх «Правил»: закрылась — и ты там же, где
 * был.
 *
 * Цвета тоже успокоены. Заливка Main 10 % с ПК на почти чёрном фоне телефона
 * читалась зелёной плашкой во всю ширину, а вместе с зелёной плашкой значка,
 * зелёной ссылкой и зелёной кнопкой экран становился зелёным целиком. Активный
 * профиль теперь помечен так же, как помечает себя всё остальное в шторках:
 * подъём белым 4 % и тонкая фирменная обводка; зелёным остаются значок и
 * счётчик direct.
 */

private val CardShape = RoundedCornerShape(20.dp)
private val BadgeShape = RoundedCornerShape(12.dp)

/** Подъём над заливкой шторки — тот же, что у поля-тегов в «Правилах». */
private val CardFill = Color.White.copy(alpha = 0.04f)
private val CardBorder = Color.White.copy(alpha = 0.06f)
private val ActiveBorder = Brand.Green.copy(alpha = 0.45f)
private val Muted = Color.White.copy(alpha = 0.50f)

/**
 * Цвет счётчика — цвет действия. Правило ПК, но приглушённое: там оно на
 * половине непрозрачности, здесь на 0.8. Половина на почти чёрном фоне
 * телефона уже не читается в 13sp, а полная яркость делает вторую строку
 * карточки громче названия.
 */
private fun countColor(action: String): Color = when (action) {
    "direct" -> Brand.Green.copy(alpha = 0.8f)
    "proxy" -> Brand.GreenLight.copy(alpha = 0.8f)
    else -> Brand.Danger.copy(alpha = 0.8f)
}

@Composable
fun RoutingProfilesSheetContent(dataDir: String) {
    val state by RoutingProfileRepository.state.collectAsStateWithLifecycle()
    val generation by RoutingProfileRepository.compileGeneration.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var busyId by remember { mutableStateOf("") }
    var confirmDelete by remember { mutableStateOf<RoutingProfile?>(null) }
    var showImport by remember { mutableStateOf(false) }

    // Готовность читается с диска — три stat на профиль. Раз на изменение
    // списка или на удачную сборку, а не на каждую рекомпозицию.
    var ready by remember { mutableStateOf<Map<String, Map<String, Boolean>>>(emptyMap()) }
    LaunchedEffect(state.profiles, generation) {
        ready = withContext(Dispatchers.IO) {
            state.profiles.associate { it.id to RoutingProfileCompiler.statusOf(dataDir, it.id) }
        }
    }

    val active = state.active
    // «Все профили» — это остальные: активный уже показан своим разделом выше,
    // и повторять его строкой ниже значило бы показать один профиль дважды.
    val rest = state.profiles.filter { it.id != state.activeId }

    fun rebuild(profile: RoutingProfile) {
        busyId = profile.id
        scope.launch {
            RoutingProfileCompiler.compile(profile, dataDir, refreshGeo = true)
            busyId = ""
        }
    }

    Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
        // Шапка — та же, что у всех шторок настроек: значок 36 dp, заголовок,
        // подпись. Крестика нет: у шторки есть ручка, свайп и «назад».
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(14.dp),
        ) {
            SettingIcon(
                icon = Icons.Outlined.AltRoute,
                bg = Color(0xFF3b82f6).copy(alpha = 0.18f),
                tint = Color(0xFF60a5fa),
            )
            Column {
                Text(
                    stringResource(R.string.routing_profiles_title),
                    style = MaterialTheme.typography.titleLarge,
                    fontWeight = FontWeight.Bold,
                )
                Text(
                    stringResource(R.string.routing_profiles_subtitle),
                    style = MaterialTheme.typography.bodyMedium,
                    color = Brand.SecondaryText,
                )
            }
        }

        if (active != null) {
            Section(stringResource(R.string.routing_profiles_active)) {
                ProfileCard(
                    profile = active,
                    isActive = true,
                    ready = ready[active.id].orEmpty(),
                    busy = busyId == active.id,
                    onSelect = {},
                    onRebuild = { rebuild(active) },
                    onDelete = { confirmDelete = active },
                )
                TextButton(
                    onClick = { RoutingProfileRepository.setActive("") },
                    contentPadding = androidx.compose.foundation.layout.PaddingValues(
                        horizontal = 8.dp, vertical = 0.dp,
                    ),
                ) {
                    // Приглушённо: это выход из состояния, а не действие,
                    // ради которого сюда пришли.
                    Text(
                        stringResource(R.string.routing_profiles_off),
                        fontSize = 14.sp,
                        color = Muted,
                    )
                }
            }
        }

        if (rest.isNotEmpty()) {
            Section(stringResource(R.string.routing_profiles_all)) {
                rest.forEach { profile ->
                    ProfileCard(
                        profile = profile,
                        isActive = false,
                        ready = ready[profile.id].orEmpty(),
                        busy = busyId == profile.id,
                        onSelect = { RoutingProfileRepository.setActive(profile.id) },
                        onRebuild = { rebuild(profile) },
                        onDelete = { confirmDelete = profile },
                    )
                }
            }
        }

        if (state.profiles.isEmpty()) {
            Text(
                stringResource(R.string.routing_profiles_empty),
                style = MaterialTheme.typography.bodyMedium,
                color = Muted,
            )
        }

        // «Создать профиль» из макета появится вместе с редактором: кнопка без
        // него обещала бы то, чего нет.
        Section(stringResource(R.string.routing_profiles_actions)) {
            Button(
                onClick = { showImport = true },
                modifier = Modifier.fillMaxWidth().height(52.dp),
                shape = CardShape,
                colors = ButtonDefaults.buttonColors(
                    containerColor = Brand.Green.copy(alpha = 0.14f),
                    contentColor = Brand.Green,
                ),
            ) {
                Text(
                    stringResource(R.string.routing_profiles_import),
                    fontSize = 15.sp,
                    fontWeight = FontWeight.Bold,
                )
            }
        }
    }

    confirmDelete?.let { victim ->
        AlertDialog(
            onDismissRequest = { confirmDelete = null },
            title = { Text(stringResource(R.string.routing_profiles_delete_confirm, victim.name)) },
            confirmButton = {
                TextButton(onClick = {
                    confirmDelete = null
                    scope.launch {
                        // Кэш сносится ДО записи конфига: иначе он остаётся без
                        // владельца и продолжает маршрутизировать.
                        withContext(Dispatchers.IO) {
                            RoutingProfileCompiler.forget(dataDir, victim.id)
                        }
                        RoutingProfileRepository.delete(victim.id)
                    }
                }) { Text(stringResource(R.string.routing_profiles_delete)) }
            },
            dismissButton = {
                TextButton(onClick = { confirmDelete = null }) {
                    Text(stringResource(R.string.routing_sheet_decline))
                }
            },
            containerColor = Brand.Surface,
        )
    }

    if (showImport) {
        ImportLinkDialog(onDismiss = { showImport = false })
    }
}

/** Раздел: подпись белым 50 % и содержимое под ней. */
@Composable
private fun Section(label: String, content: @Composable () -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(label, fontSize = 14.sp, color = Muted)
        content()
    }
}

@Composable
private fun ProfileCard(
    profile: RoutingProfile,
    isActive: Boolean,
    ready: Map<String, Boolean>,
    busy: Boolean,
    onSelect: () -> Unit,
    onRebuild: () -> Unit,
    onDelete: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(CardShape)
            .background(CardFill)
            .border(1.dp, if (isActive) ActiveBorder else CardBorder, CardShape)
            .clickable(enabled = !busy && !isActive, onClick = onSelect)
            .padding(start = 14.dp, end = 6.dp, top = 12.dp, bottom = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        Box(
            modifier = Modifier
                .size(44.dp)
                .clip(BadgeShape)
                .background(
                    if (isActive) Brand.Green.copy(alpha = 0.16f) else Brand.SurfaceHigh
                ),
            contentAlignment = Alignment.Center,
        ) {
            if (busy) {
                CircularProgressIndicator(
                    modifier = Modifier.size(20.dp),
                    strokeWidth = 2.dp,
                    color = Brand.Green,
                )
            } else {
                Icon(
                    Icons.Outlined.Public,
                    contentDescription = null,
                    tint = if (isActive) Brand.Green else Muted,
                    modifier = Modifier.size(22.dp),
                )
            }
        }
        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(3.dp),
        ) {
            Text(
                profile.name,
                fontSize = 16.sp,
                fontWeight = FontWeight.Bold,
                maxLines = 1,
            )
            if (profile.publisherName.isNotEmpty()) {
                Text(
                    stringResource(R.string.routing_sheet_publisher, profile.publisherName),
                    fontSize = 13.sp,
                    color = Brand.MutedText,
                    maxLines = 1,
                )
            }
            Counts(profile)
            when {
                profile.lastError.isNotEmpty() -> {
                    Text(profile.lastError, fontSize = 13.sp, color = Brand.Danger)
                    RebuildLink(onRebuild, busy)
                }
                // «Не собран» показывается только когда правил нет НИ У ОДНОГО
                // действия: у профиля из одних proxy-правил два других пусты
                // законно, и жаловаться там не на что.
                ROUTING_ACTIONS.none { ready[it] == true } -> {
                    Text(
                        stringResource(R.string.routing_profiles_not_built),
                        fontSize = 13.sp,
                        color = Brand.MutedText,
                    )
                    RebuildLink(onRebuild, busy)
                }
            }
        }
        // Карандаша пока нет: редактор — этап C, а кнопка обещала бы то, чего
        // нет. Это и правило ПК: правка есть не у всякой строки.
        IconButton(onClick = onDelete, enabled = !busy) {
            Icon(
                Icons.Outlined.DeleteOutline,
                contentDescription = stringResource(R.string.routing_profiles_delete),
                tint = Muted,
            )
        }
    }
}

@Composable
private fun RebuildLink(onClick: () -> Unit, busy: Boolean) {
    TextButton(
        onClick = onClick,
        enabled = !busy,
        contentPadding = androidx.compose.foundation.layout.PaddingValues(0.dp),
    ) {
        Text(stringResource(R.string.routing_profiles_rebuild), fontSize = 13.sp)
    }
}

/**
 * Счётчики одной строкой, каждое действие своим цветом. Ноль не показывается —
 * в макете у профиля со 117 direct и 1 block нарисованы ровно две пары, а не
 * три с нулём.
 */
@Composable
private fun Counts(profile: RoutingProfile) {
    val parts = ROUTING_ACTIONS.mapNotNull { action ->
        val n = profile.ruleCount(action)
        if (n > 0) action to n else null
    }
    if (parts.isEmpty()) return
    Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
        parts.forEach { (action, n) ->
            Text(
                "• $n $action",
                fontSize = 13.sp,
                fontWeight = FontWeight.Medium,
                color = countColor(action),
            )
        }
    }
}

/**
 * Вставить ссылку маршрутизации руками — когда она пришла не тапом по ссылке,
 * а текстом. Разбор тот же, что у диплинка, и тот же лист превью: применяется
 * только после согласия.
 */
@Composable
private fun ImportLinkDialog(onDismiss: () -> Unit) {
    var link by remember { mutableStateOf("") }
    val ctx = LocalContext.current
    val trimmed = link.trim()
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.routing_profiles_import)) },
        text = {
            OutlinedTextField(
                value = link,
                onValueChange = { link = it },
                placeholder = { Text("resultv://routing/...") },
                maxLines = 4,
                modifier = Modifier.fillMaxWidth(),
            )
        },
        confirmButton = {
            TextButton(
                enabled = trimmed.isNotEmpty(),
                onClick = {
                    onDismiss()
                    // Тот же путь, что у полученного intent: развилка и превью
                    // живут в DeepLinkImporter, второй копии разбора здесь нет.
                    DeepLinkImporter.import(ctx, trimmed)
                },
            ) { Text(stringResource(R.string.action_add)) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) {
                Text(stringResource(R.string.routing_sheet_decline))
            }
        },
        containerColor = Brand.Surface,
    )
}
