package com.resultv.android.ui.screens

import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.automirrored.outlined.List
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.Public
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Bolt
import androidx.compose.material.icons.outlined.Shield
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextStyle
import com.resultv.android.theme.SegoeUi
import com.resultv.android.ui.components.RvButton
import com.resultv.android.ui.components.RvButtonColors
import com.resultv.android.ui.components.RvButtonLabel
import android.widget.Toast
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.rememberModalBottomSheetState
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
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.AltRoute
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.Public
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
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
import com.resultv.android.theme.RvCategory
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.RvSpace
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
private val Muted = RvColor.whiteA50

/** Цвет счётчика — цвет действия, как в макете (Figma 6887:5064). */
private fun countColor(action: String): Color = when (action) {
    "direct" -> RvColor.Main
    "proxy" -> RvColor.Second
    else -> RvColor.Errors
}

private val CardTitle = TextStyle(fontFamily = SegoeUi, fontSize = 12.sp, lineHeight = 15.6.sp, fontWeight = FontWeight.Bold)
private val CardMeta = TextStyle(fontFamily = SegoeUi, fontSize = 10.sp, lineHeight = 12.sp, fontWeight = FontWeight.SemiBold)

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

    Column(verticalArrangement = Arrangement.spacedBy(20.dp)) {
        if (active != null) {
            Section(stringResource(R.string.routing_profiles_active), Icons.Outlined.Shield) {
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
            Section(stringResource(R.string.routing_profiles_all), Icons.AutoMirrored.Outlined.List) {
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

        Section(stringResource(R.string.routing_profiles_actions), Icons.Outlined.Bolt) {
            Row(horizontalArrangement = Arrangement.spacedBy(RvSpace.nest3)) {
                RvButton(
                    onClick = { onEdit(null) },
                    fill = RvButtonColors.greenFill,
                    outline = RvButtonColors.greenOutline,
                    modifier = Modifier.weight(1f),
                ) {
                    Text(
                        stringResource(R.string.routing_profiles_create),
                        style = RvButtonLabel,
                        fontWeight = FontWeight.Bold,
                        color = RvColor.Main,
                    )
                }
                RvButton(
                    onClick = { showImport = true },
                    fill = RvButtonColors.greyFill,
                    outline = RvButtonColors.greyOutline,
                    modifier = Modifier.weight(1f),
                ) {
                    Text(
                        stringResource(R.string.routing_profiles_import),
                        style = RvButtonLabel,
                        fontWeight = FontWeight.SemiBold,
                        color = RvColor.whiteA50,
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

/** Раздел: подпись группы и карточки под ней через 8. */
@Composable
private fun Section(label: String, icon: ImageVector, content: @Composable () -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(RvSpace.nest3)) {
        SheetGroupLabel(label, icon)
        content()
    }
}

/**
 * Карточка профиля — ProfileItem макета (Figma 6887:5058 / 5082): поле 14,
 * скругление 20, плитка 40 с глобусом 20, название 12 Bold и счётчики 10.
 * Активная — Main 10 % с зелёной обводкой 30 %, остальные — Grey с белой 6 %
 * и кнопками правки и удаления (круги 32).
 */
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
            .background(if (isActive) RvColor.mainA10 else RvColor.Grey)
            .border(1.dp, if (isActive) RvColor.Main.copy(alpha = 0.3f) else RvColor.whiteA06, CardShape)
            .clickable(enabled = !busy, onClick = onSelect)
            .padding(14.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RvSpace.nest2),
    ) {
        Box(
            modifier = Modifier
                .size(40.dp)
                .clip(RoundedCornerShape(10.dp))
                .background(if (isActive) RvCategory.Main.tile else RvColor.LightGray),
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
                    Icons.Filled.Public,
                    contentDescription = null,
                    tint = if (isActive) RvCategory.Main.glyph else Muted,
                    modifier = Modifier.size(20.dp),
                )
            }
        }
        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(RvSpace.xs),
        ) {
            Text(profile.name, style = CardTitle, color = RvColor.White, maxLines = 1)
            if (profile.publisherName.isNotEmpty()) {
                Text(
                    stringResource(R.string.routing_sheet_publisher, profile.publisherName),
                    style = CardMeta,
                    color = Muted,
                    maxLines = 1,
                )
            }
            Counts(profile)
            when {
                profile.lastError.isNotEmpty() -> {
                    Text(profile.lastError, style = CardMeta, color = RvColor.Errors)
                    RebuildLink(onRebuild, busy)
                }
                // «Не собран» показывается только когда правил нет НИ У ОДНОГО
                // действия: у профиля из одних proxy-правил два других пусты
                // законно, и жаловаться там не на что.
                ROUTING_ACTIONS.none { ready[it] == true } -> {
                    Text(stringResource(R.string.routing_profiles_not_built), style = CardMeta, color = Muted)
                    RebuildLink(onRebuild, busy)
                }
            }
        }
        // У активного кнопок в макете нет: тап по нему выключает профиль, а
        // правят и удаляют его из общего списка, куда он вернётся.
        if (!isActive) {
            Row(horizontalArrangement = Arrangement.spacedBy(RvSpace.nest3)) {
                CircleAction(
                    icon = Icons.Filled.Edit,
                    label = stringResource(R.string.routing_editor_edit_title),
                    fill = RvColor.LightGray,
                    tint = Muted,
                    enabled = !busy,
                    onClick = onEdit,
                )
                CircleAction(
                    icon = Icons.Filled.Delete,
                    label = stringResource(R.string.routing_profiles_delete),
                    fill = RvColor.errorsA10,
                    tint = RvColor.Errors,
                    enabled = !busy,
                    onClick = onDelete,
                )
            }
        }
    }
}

@Composable
private fun CircleAction(
    icon: ImageVector,
    label: String,
    fill: Color,
    tint: Color,
    enabled: Boolean,
    onClick: () -> Unit,
) {
    Box(
        modifier = Modifier
            .size(32.dp)
            .clip(CircleShape)
            .background(fill)
            .clickable(enabled = enabled, role = Role.Button, onClick = onClick)
            .semantics { contentDescription = label },
        contentAlignment = Alignment.Center,
    ) {
        Icon(icon, contentDescription = null, tint = tint, modifier = Modifier.size(16.dp))
    }
}

@Composable
private fun RebuildLink(onClick: () -> Unit, busy: Boolean) {
    TextButton(
        onClick = onClick,
        enabled = !busy,
        contentPadding = androidx.compose.foundation.layout.PaddingValues(0.dp),
    ) {
        Text(stringResource(R.string.routing_profiles_rebuild), style = CardMeta, color = RvColor.Main)
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
    Row(horizontalArrangement = Arrangement.spacedBy(RvSpace.nest3)) {
        parts.forEach { (action, n) ->
            Text("• $n $action", style = CardMeta, color = countColor(action))
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
        SettingsSheet(
            icon = Icons.Outlined.AltRoute,
            tint = RvCategory.Main,
            title = stringResource(R.string.routing_profiles_title),
            description = stringResource(R.string.routing_profiles_subtitle),
            sheetState = routingSheetState,
            onDismiss = onDismiss,
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

    if (editorOpen) {
        val ctx = LocalContext.current
        val dataDir = ctx.filesDir.absolutePath
        val isEdit = editorProfile?.id?.isNotEmpty() == true
        SettingsSheet(
            icon = if (isEdit) Icons.Filled.Edit else Icons.Outlined.Add,
            tint = RvCategory.Main,
            title = stringResource(
                if (isEdit) R.string.routing_editor_edit_title else R.string.routing_editor_create_title,
            ),
            description = if (isEdit) {
                stringResource(R.string.routing_editor_edit_subtitle_named, editorProfile?.name.orEmpty())
            } else {
                stringResource(R.string.routing_editor_create_subtitle)
            },
            sheetState = editorSheetState,
            onDismiss = { if (!editorBusy) editorOpen = false },
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
