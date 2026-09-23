package com.resultv.android.ui.screens

import android.widget.Toast
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.BottomSheetDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.rememberModalBottomSheetState
import com.resultv.android.ui.components.DarkSheetSystemBars
import com.resultv.android.vpn.parseRoutingMergeResult
import mobile.Mobile
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
import androidx.compose.material.icons.outlined.Edit
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
import com.resultv.android.theme.rvBorder
import com.resultv.android.theme.RvCategory
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.RvRadius
import com.resultv.android.theme.RvSpace
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

private val CardShape = RoundedCornerShape(RvRadius.card)
private val BadgeShape = RoundedCornerShape(RvRadius.chip)

/** Подъём над заливкой шторки — тот же, что у поля-тегов в «Правилах». */
private val CardFill = Color.White.copy(alpha = 0.04f)
private val ActiveBorder = RvColor.Main.copy(alpha = 0.45f)
private val Muted = Color.White.copy(alpha = 0.50f)

/**
 * Цвет счётчика — цвет действия. Правило ПК, но приглушённое: там оно на
 * половине непрозрачности, здесь на 0.8. Половина на почти чёрном фоне
 * телефона уже не читается в 13sp, а полная яркость делает вторую строку
 * карточки громче названия.
 */
private fun countColor(action: String): Color = when (action) {
    "direct" -> RvColor.Main.copy(alpha = 0.8f)
    "proxy" -> RvColor.Second.copy(alpha = 0.8f)
    else -> RvColor.Errors.copy(alpha = 0.8f)
}

@Composable
fun RoutingProfilesSheetContent(
    dataDir: String,
    onEdit: (RoutingProfile?) -> Unit = {},
) {
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

    Column(verticalArrangement = Arrangement.spacedBy(RvSpace.nest1)) {
        // Шапка — та же, что у всех шторок настроек: значок 36 dp, заголовок,
        // подпись. Крестика нет: у шторки есть ручка, свайп и «назад».
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2),
        ) {
            SettingIcon(
                icon = Icons.Outlined.AltRoute,
                tint = RvCategory.Blue,
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
                    color = RvColor.whiteA50,
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
                    // Тап по активному профилю выключает его. Отдельная
                    // строка «Маршрутизация без профиля» под карточкой была
                    // вторым способом сделать то же самое — и единственной
                    // ссылкой в шторке, где всё остальное нажимается
                    // карточками.
                    onSelect = { RoutingProfileRepository.setActive("") },
                    onRebuild = { rebuild(active) },
                    onEdit = { onEdit(active) },
                    onDelete = { confirmDelete = active },
                )
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
                        onEdit = { onEdit(profile) },
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
            Row(horizontalArrangement = Arrangement.spacedBy(RvSpace.nest3)) {
                Button(
                    onClick = { onEdit(null) },
                    modifier = Modifier.weight(1f).height(52.dp),
                    shape = CardShape,
                    colors = ButtonDefaults.buttonColors(
                        containerColor = CardFill,
                        contentColor = Color.White,
                    ),
                ) {
                    Text(
                        stringResource(R.string.routing_profiles_create),
                        fontSize = 15.sp,
                        fontWeight = FontWeight.Bold,
                    )
                }
                Button(
                    onClick = { showImport = true },
                    modifier = Modifier.weight(1f).height(52.dp),
                    shape = CardShape,
                    colors = ButtonDefaults.buttonColors(
                        containerColor = RvColor.Main.copy(alpha = 0.14f),
                        contentColor = RvColor.Main,
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
            containerColor = RvColor.Grey,
        )
    }

    if (showImport) {
        ImportLinkDialog(onDismiss = { showImport = false })
    }
}

/** Раздел: подпись белым 50 % и содержимое под ней. */
@Composable
private fun Section(label: String, content: @Composable () -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(RvSpace.nest3)) {
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
    onEdit: () -> Unit,
    onDelete: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(CardShape)
            .background(CardFill)
            // Активный профиль держит свой зелёный контур: это признак
            // выбора, а не обводка интерактивного элемента, и градиент его
            // стёр бы.
            .then(
                if (isActive) Modifier.border(1.dp, ActiveBorder, CardShape)
                else Modifier.rvBorder(CardShape)
            )
            .clickable(enabled = !busy, onClick = onSelect)
            .padding(start = RvSpace.nest2, end = RvSpace.xs, top = RvSpace.nest2, bottom = RvSpace.nest2),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2),
    ) {
        Box(
            modifier = Modifier
                .size(44.dp)
                .clip(BadgeShape)
                .background(
                    if (isActive) RvColor.Main.copy(alpha = 0.16f) else RvColor.LightGray
                ),
            contentAlignment = Alignment.Center,
        ) {
            if (busy) {
                CircularProgressIndicator(
                    modifier = Modifier.size(20.dp),
                    strokeWidth = 2.dp,
                    color = RvColor.Main,
                )
            } else {
                Icon(
                    Icons.Outlined.Public,
                    contentDescription = null,
                    tint = if (isActive) RvColor.Main else Muted,
                    modifier = Modifier.size(22.dp),
                )
            }
        }
        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(RvSpace.xs),
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
                    color = RvColor.whiteA50,
                    maxLines = 1,
                )
            }
            Counts(profile)
            when {
                profile.lastError.isNotEmpty() -> {
                    Text(profile.lastError, fontSize = 13.sp, color = RvColor.Errors)
                    RebuildLink(onRebuild, busy)
                }
                // «Не собран» показывается только когда правил нет НИ У ОДНОГО
                // действия: у профиля из одних proxy-правил два других пусты
                // законно, и жаловаться там не на что.
                ROUTING_ACTIONS.none { ready[it] == true } -> {
                    Text(
                        stringResource(R.string.routing_profiles_not_built),
                        fontSize = 13.sp,
                        color = RvColor.whiteA50,
                    )
                    RebuildLink(onRebuild, busy)
                }
            }
        }
        // Две кнопки — одной группой, иначе между ними встаёт шаг строки
        // (14 dp) поверх собственных полей IconButton, и корзина отъезжает от
        // карандаша дальше, чем карандаш от текста.
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(0.dp),
        ) {
            // Карандаш у каждой строки, включая профиль подписки: на ПК onEdit
            // тоже передаётся безусловно (RoutingProfilesDialog.jsx:75). Правка
            // профиля подписки осмысленна — происхождение и ссылки на списки
            // переживают её (см. UpsertRoutingProfile), — хотя следующая
            // синхронизация правила перепишет.
            IconButton(onClick = onEdit, enabled = !busy, modifier = Modifier.size(40.dp)) {
                Icon(
                    Icons.Outlined.Edit,
                    contentDescription = stringResource(R.string.routing_editor_edit_title),
                    tint = Muted,
                    modifier = Modifier.size(20.dp),
                )
            }
            IconButton(onClick = onDelete, enabled = !busy, modifier = Modifier.size(40.dp)) {
                Icon(
                    Icons.Outlined.DeleteOutline,
                    contentDescription = stringResource(R.string.routing_profiles_delete),
                    tint = Muted,
                    modifier = Modifier.size(20.dp),
                )
            }
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
    Row(horizontalArrangement = Arrangement.spacedBy(RvSpace.xs)) {
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
        containerColor = RvColor.Grey,
    )
}

/**
 * Шторка «Профили маршрутизации» и поверх неё — редактор профиля.
 *
 * Профили — ВЛОЖЕННАЯ шторка поверх страницы правил, а не отдельный экран:
 * закрытие возвращает туда, откуда её открыли. Редактор — следующая шторка
 * поверх профилей, тот же приём уровнем выше.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun RoutingProfilesSheets(open: Boolean, onDismiss: () -> Unit) {
    var editorOpen by remember { mutableStateOf(false) }
    var editorProfile by remember { mutableStateOf<RoutingProfile?>(null) }
    var editorBusy by remember { mutableStateOf(false) }
    val editorSheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    val editorScope = rememberCoroutineScope()
    val routingSheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)

    if (open) {
        ModalBottomSheet(
            onDismissRequest = onDismiss,
            modifier = Modifier.windowInsetsPadding(WindowInsets.statusBars),
            sheetState = routingSheetState,
            containerColor = RvColor.Grey,
            dragHandle = { BottomSheetDefaults.DragHandle() },
        ) {
            DarkSheetSystemBars()
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .verticalScroll(rememberScrollState())
                    .padding(horizontal = 16.dp, vertical = 8.dp)
                    // Не safe area: её шторка держит сама. Это поле, чтобы
                    // последняя строка не упиралась в панель навигации.
                    .padding(bottom = 24.dp),
            ) {
                RoutingProfilesSheetContent(
                    dataDir = LocalContext.current.filesDir.absolutePath,
                    onEdit = { p ->
                        editorProfile = p
                        editorOpen = true
                    },
                )
            }
        }
    }

    if (editorOpen) {
        val ctx = LocalContext.current
        val dataDir = ctx.filesDir.absolutePath
        ModalBottomSheet(
            onDismissRequest = { if (!editorBusy) editorOpen = false },
            modifier = Modifier.windowInsetsPadding(WindowInsets.statusBars),
            sheetState = editorSheetState,
            containerColor = RvColor.Grey,
            dragHandle = { BottomSheetDefaults.DragHandle() },
        ) {
            DarkSheetSystemBars()
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .verticalScroll(rememberScrollState())
                    .padding(horizontal = 16.dp, vertical = 8.dp)
                    .padding(bottom = 24.dp),
            ) {
                RoutingProfileEditorContent(
                    profile = editorProfile,
                    busy = editorBusy,
                    onSave = { edited ->
                        editorBusy = true
                        editorScope.launch {
                            val merged = withContext(Dispatchers.IO) {
                                runCatching {
                                    Mobile.mergeRoutingProfile(
                                        RoutingProfileRepository.storeJson(),
                                        edited.toJson().toString(),
                                        false,
                                    )
                                }.getOrNull()
                            }
                            val state = merged?.let { parseRoutingMergeResult(it) }
                            if (state == null) {
                                Toast.makeText(
                                    ctx,
                                    ctx.getString(R.string.routing_import_failed, "merge failed"),
                                    Toast.LENGTH_LONG,
                                ).show()
                            } else {
                                RoutingProfileRepository.replaceAll(state.profiles, state.activeId)
                                // Ищем по имени и происхождению, а не по id: у
                                // нового профиля id назначает Go, и до слияния
                                // его здесь неоткуда взять.
                                val saved = state.profiles.firstOrNull {
                                    it.name == edited.name && it.source == edited.source
                                }
                                if (saved != null) {
                                    RoutingProfileCompiler.compile(saved, dataDir)
                                }
                                editorOpen = false
                            }
                            editorBusy = false
                        }
                    },
                )
            }
        }
    }
}
