package com.resultv.android.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.AltRoute
import androidx.compose.material.icons.outlined.Close
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
import androidx.compose.material3.Scaffold
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
 * Экран «Профили маршрутизации». Повторяет окно ПК
 * (frontend/src/views/redesign/RoutingProfilesDialog.jsx, Figma 6600:3530):
 * шапка с иконкой, подписью и крестиком, затем три раздела с зазором 16 —
 * «Активный профиль», «Все профили», «Действия». Подписи разделов белым 50 %.
 *
 * Метрики строки взяты из kit/ProfileItem.css: отступы 16 слева и по
 * вертикали, 24 справа; скругление 24, зазор 16; плашка значка — отступ 16,
 * скругление 14, значок 28. Название 18/700, счётчики 14/500.
 *
 * Отличие от ПК одно и намеренное: окно там модальное, здесь полноэкранный
 * маршрут. Лист настроек на Android — подокно над Scaffold, и накрыть его
 * модальным окном нечем; тот же приём использует мастер сертификата.
 */

private val RowShape = RoundedCornerShape(24.dp)
private val BadgeShape = RoundedCornerShape(14.dp)

/** Заливка активной строки и её плашки: Main 10 %, как в макете. */
private val ActiveFill = Brand.Green.copy(alpha = 0.10f)
private val IdleBorder = Color.White.copy(alpha = 0.10f)
private val LabelColor = Color.White.copy(alpha = 0.50f)

/**
 * Цвет счётчика — цвет действия на половине непрозрачности. Правило ПК; proxy
 * взят вторым фирменным, как он набран в редакторе профиля.
 */
private fun countColor(action: String): Color = when (action) {
    "direct" -> Brand.Green.copy(alpha = 0.50f)
    "proxy" -> Brand.GreenLight.copy(alpha = 0.50f)
    else -> Brand.Danger.copy(alpha = 0.50f)
}

@Composable
fun RoutingProfilesScreen(dataDir: String, onClose: () -> Unit) {
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

    Scaffold(containerColor = Brand.Bg) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp, vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Header(onClose = onClose)

            if (active != null) {
                Section(stringResource(R.string.routing_profiles_active)) {
                    ProfileRow(
                        profile = active,
                        isActive = true,
                        ready = ready[active.id].orEmpty(),
                        busy = busyId == active.id,
                        onSelect = {},
                        onRebuild = {
                            busyId = active.id
                            scope.launch {
                                RoutingProfileCompiler.compile(active, dataDir, refreshGeo = true)
                                busyId = ""
                            }
                        },
                        onDelete = { confirmDelete = active },
                    )
                    // Выключить профиль, ничего не удаляя. Отдельной кнопки на
                    // ПК нет — там профиль снимается выбором другого, а снять
                    // его совсем нечем; здесь это единственный способ вернуться
                    // к маршрутизации без профиля.
                    TextButton(
                        onClick = { RoutingProfileRepository.setActive("") },
                        contentPadding = androidx.compose.foundation.layout.PaddingValues(
                            horizontal = 8.dp, vertical = 0.dp,
                        ),
                    ) {
                        // Приглушённо: это выход из состояния, а не главное
                        // действие экрана — акцентный зелёный тянул бы на себя
                        // внимание сильнее, чем импорт.
                        Text(
                            stringResource(R.string.routing_profiles_off),
                            fontSize = 14.sp,
                            color = LabelColor,
                        )
                    }
                }
            }

            if (rest.isNotEmpty()) {
                Section(stringResource(R.string.routing_profiles_all)) {
                    rest.forEach { profile ->
                        ProfileRow(
                            profile = profile,
                            isActive = false,
                            ready = ready[profile.id].orEmpty(),
                            busy = busyId == profile.id,
                            onSelect = { RoutingProfileRepository.setActive(profile.id) },
                            onRebuild = {
                                busyId = profile.id
                                scope.launch {
                                    RoutingProfileCompiler.compile(profile, dataDir, refreshGeo = true)
                                    busyId = ""
                                }
                            },
                            onDelete = { confirmDelete = profile },
                        )
                    }
                }
            }

            if (state.profiles.isEmpty()) {
                Text(
                    stringResource(R.string.routing_profiles_empty),
                    style = MaterialTheme.typography.bodyMedium,
                    color = LabelColor,
                )
            }

            // «Создать профиль» из макета появится вместе с редактором
            // (этап C): кнопка без него обещала бы то, чего нет.
            Section(stringResource(R.string.routing_profiles_actions)) {
                Button(
                    onClick = { showImport = true },
                    modifier = Modifier.fillMaxWidth().height(56.dp),
                    shape = RowShape,
                    colors = ButtonDefaults.buttonColors(
                        containerColor = ActiveFill,
                        contentColor = Brand.Green,
                    ),
                ) {
                    Text(
                        stringResource(R.string.routing_profiles_import),
                        fontSize = 16.sp,
                        fontWeight = FontWeight.Bold,
                    )
                }
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
                singleLine = false,
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

/** Шапка окна ПК: значок, заголовок с подписью, крестик справа. */
@Composable
private fun Header(onClose: () -> Unit) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        SettingIcon(
            icon = Icons.Outlined.AltRoute,
            bg = ActiveFill,
            tint = Brand.Green,
        )
        Column(modifier = Modifier.weight(1f)) {
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
        IconButton(onClick = onClose) {
            Icon(
                Icons.Outlined.Close,
                contentDescription = stringResource(R.string.action_close),
                tint = LabelColor,
            )
        }
    }
}

/** Раздел: подпись белым 50 % и содержимое под ней с зазором 8. */
@Composable
private fun Section(label: String, content: @Composable () -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(label, fontSize = 14.sp, color = LabelColor)
        content()
    }
}

@Composable
private fun ProfileRow(
    profile: RoutingProfile,
    isActive: Boolean,
    ready: Map<String, Boolean>,
    busy: Boolean,
    onSelect: () -> Unit,
    onRebuild: () -> Unit,
    onDelete: () -> Unit,
) {
    val fill = if (isActive) ActiveFill else Brand.Surface
    val border = if (isActive) ActiveFill else IdleBorder
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RowShape)
            .background(fill)
            .border(1.dp, border, RowShape)
            .clickable(enabled = !busy && !isActive, onClick = onSelect)
            .padding(start = 16.dp, end = 24.dp, top = 16.dp, bottom = 16.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Box(
            modifier = Modifier
                .clip(BadgeShape)
                .background(if (isActive) ActiveFill else Brand.SurfaceHigh)
                .padding(16.dp),
            contentAlignment = Alignment.Center,
        ) {
            if (busy) {
                CircularProgressIndicator(
                    modifier = Modifier.size(28.dp),
                    strokeWidth = 2.dp,
                    color = Brand.Green,
                )
            } else {
                Icon(
                    Icons.Outlined.Public,
                    contentDescription = null,
                    tint = if (isActive) Brand.Green else LabelColor,
                    modifier = Modifier.size(28.dp),
                )
            }
        }
        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Text(
                profile.name,
                fontSize = 18.sp,
                lineHeight = 25.sp,
                fontWeight = FontWeight.Bold,
                maxLines = 1,
            )
            if (profile.publisherName.isNotEmpty()) {
                Text(
                    stringResource(R.string.routing_sheet_publisher, profile.publisherName),
                    fontSize = 14.sp,
                    lineHeight = 16.sp,
                    color = Brand.MutedText,
                    maxLines = 1,
                )
            }
            Counts(profile)
            when {
                profile.lastError.isNotEmpty() -> {
                    Text(
                        profile.lastError,
                        fontSize = 14.sp,
                        lineHeight = 16.sp,
                        color = Brand.Danger,
                    )
                    TextButton(
                        onClick = onRebuild,
                        enabled = !busy,
                        contentPadding = androidx.compose.foundation.layout.PaddingValues(0.dp),
                    ) {
                        Text(stringResource(R.string.routing_profiles_rebuild), fontSize = 14.sp)
                    }
                }
                // «Не собран» показывается только когда правил нет НИ У ОДНОГО
                // действия: у профиля из одних proxy-правил два других пусты
                // законно, и жаловаться там не на что.
                ROUTING_ACTIONS.none { ready[it] == true } -> {
                    Text(
                        stringResource(R.string.routing_profiles_not_built),
                        fontSize = 14.sp,
                        lineHeight = 16.sp,
                        color = Brand.MutedText,
                    )
                    TextButton(
                        onClick = onRebuild,
                        enabled = !busy,
                        contentPadding = androidx.compose.foundation.layout.PaddingValues(0.dp),
                    ) {
                        Text(stringResource(R.string.routing_profiles_rebuild), fontSize = 14.sp)
                    }
                }
            }
        }
        // Карандаша здесь пока нет: редактор — этап C, а кнопка обещала бы то,
        // чего нет. На ПК то же правило — правка есть не у всякой строки.
        IconButton(onClick = onDelete, enabled = !busy) {
            Icon(
                Icons.Outlined.DeleteOutline,
                contentDescription = stringResource(R.string.routing_profiles_delete),
                tint = LabelColor,
            )
        }
    }
}

/**
 * Счётчики одной строкой, каждое действие своим цветом. Ноль не показывается —
 * в макете у профиля с 117 direct и 1 block нарисованы ровно две пары, а не
 * три с нулём.
 */
@Composable
private fun Counts(profile: RoutingProfile) {
    val parts = ROUTING_ACTIONS.mapNotNull { action ->
        val n = profile.ruleCount(action)
        if (n > 0) action to n else null
    }
    if (parts.isEmpty()) return
    Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
        parts.forEach { (action, n) ->
            Text(
                "• $n $action",
                fontSize = 14.sp,
                lineHeight = 16.sp,
                fontWeight = FontWeight.Medium,
                color = countColor(action),
            )
        }
    }
}
