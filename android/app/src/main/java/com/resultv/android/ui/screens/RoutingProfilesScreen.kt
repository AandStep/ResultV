package com.resultv.android.ui.screens

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.DeleteOutline
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.resultv.android.R
import com.resultv.android.theme.Brand
import com.resultv.android.vpn.ROUTING_ACTIONS
import com.resultv.android.vpn.RoutingProfile
import com.resultv.android.vpn.RoutingProfileCompiler
import com.resultv.android.vpn.RoutingProfileRepository
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * Список профилей маршрутизации: какой в силе, что в каждом и собран ли он.
 *
 * Открывается только из «Правил» и только в Global — там профиль и действует.
 * Редактора здесь нет, он этапа C: пока профили приходят ссылкой.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun RoutingProfilesScreen(dataDir: String, onClose: () -> Unit) {
    val state by RoutingProfileRepository.state.collectAsStateWithLifecycle()
    val generation by RoutingProfileRepository.compileGeneration.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var busyId by remember { mutableStateOf("") }
    var confirmDelete by remember { mutableStateOf<RoutingProfile?>(null) }

    // Готовность читается с диска — три stat на профиль. Раз на изменение
    // списка или на удачную сборку, а не на каждую рекомпозицию.
    var ready by remember { mutableStateOf<Map<String, Map<String, Boolean>>>(emptyMap()) }
    LaunchedEffect(state.profiles, generation) {
        ready = withContext(Dispatchers.IO) {
            state.profiles.associate { it.id to RoutingProfileCompiler.statusOf(dataDir, it.id) }
        }
    }

    Scaffold(
        containerColor = Brand.Bg,
        topBar = {
            TopAppBar(
                title = {
                    Text(
                        stringResource(R.string.routing_profiles_title),
                        fontWeight = FontWeight.Bold,
                    )
                },
                navigationIcon = {
                    IconButton(onClick = onClose) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = null)
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = Brand.Bg),
            )
        },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            if (state.profiles.isEmpty()) {
                Text(
                    stringResource(R.string.routing_profiles_empty),
                    style = MaterialTheme.typography.bodyMedium,
                    color = Brand.SecondaryText,
                    modifier = Modifier.padding(vertical = 24.dp),
                )
                return@Column
            }

            Card(
                shape = RoundedCornerShape(20.dp),
                colors = CardDefaults.cardColors(containerColor = Brand.Surface),
                modifier = Modifier.fillMaxWidth(),
            ) {
                Column {
                    // «Без профиля» — это выбор, а не его отсутствие: выключить
                    // маршрутизацию по профилю, ничего не удаляя.
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clickable { RoutingProfileRepository.setActive("") }
                            .padding(horizontal = 16.dp, vertical = 14.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Text(
                            stringResource(R.string.routing_profiles_off),
                            style = MaterialTheme.typography.bodyLarge,
                            modifier = Modifier.weight(1f),
                        )
                        RadioButton(
                            selected = state.activeId.isEmpty(),
                            onClick = { RoutingProfileRepository.setActive("") },
                        )
                    }
                    state.profiles.forEach { profile ->
                        HorizontalDivider(color = Brand.SurfaceHigh)
                        RoutingProfileRow(
                            profile = profile,
                            isActive = profile.id == state.activeId,
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
}

@Composable
private fun RoutingProfileRow(
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
            .clickable(enabled = !busy, onClick = onSelect)
            .padding(start = 16.dp, end = 8.dp, top = 12.dp, bottom = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(profile.name, style = MaterialTheme.typography.bodyLarge)
            if (profile.publisherName.isNotEmpty()) {
                Text(
                    stringResource(R.string.routing_sheet_publisher, profile.publisherName),
                    style = MaterialTheme.typography.bodySmall,
                    color = Brand.MutedText,
                )
            }
            Text(
                stringResource(
                    R.string.routing_sheet_counts,
                    profile.ruleCount("direct"),
                    profile.ruleCount("proxy"),
                    profile.ruleCount("block"),
                ),
                style = MaterialTheme.typography.bodySmall,
                color = Brand.SecondaryText,
            )
            when {
                profile.lastError.isNotEmpty() -> {
                    Text(
                        profile.lastError,
                        style = MaterialTheme.typography.bodySmall,
                        color = Color(0xFFf87171),
                    )
                    TextButton(onClick = onRebuild, enabled = !busy) {
                        Text(stringResource(R.string.routing_profiles_rebuild))
                    }
                }
                // «Собран» означает, что на диске есть хоть один пригодный
                // rule-set: у профиля из одних proxy-правил два других действия
                // пусты законно.
                ROUTING_ACTIONS.any { ready[it] == true } -> Text(
                    stringResource(R.string.routing_profiles_built),
                    style = MaterialTheme.typography.bodySmall,
                    color = Brand.SecondaryText,
                )
                else -> {
                    Text(
                        stringResource(R.string.routing_profiles_not_built),
                        style = MaterialTheme.typography.bodySmall,
                        color = Brand.MutedText,
                    )
                    TextButton(onClick = onRebuild, enabled = !busy) {
                        Text(stringResource(R.string.routing_profiles_rebuild))
                    }
                }
            }
        }
        IconButton(onClick = onDelete, enabled = !busy) {
            Icon(
                Icons.Outlined.DeleteOutline,
                contentDescription = stringResource(R.string.routing_profiles_delete),
                tint = Brand.MutedText,
            )
        }
        RadioButton(selected = isActive, onClick = onSelect, enabled = !busy)
    }
}
