package com.resultv.android.ui.screens

import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.DeleteOutline
import androidx.compose.material.icons.outlined.Route
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import com.resultv.android.R
import com.resultv.android.theme.RvCategory
import com.resultv.android.theme.RvColor
import com.resultv.android.ui.components.RemovableTagList
import com.resultv.android.vpn.NativeCore
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import mobile.Mobile
import org.json.JSONObject

private data class LearnedRoutes(val hosts: List<String>, val hidden: Int)

/** Keep the learned rules accessible even while Adaptive Smart is off. */
@Composable
internal fun LearnedSmartRoutesRow() {
    val dataDir = LocalContext.current.filesDir.absolutePath
    val scope = rememberCoroutineScope()
    val requests = remember { Mutex() }
    var routes by remember { mutableStateOf<LearnedRoutes?>(null) }
    var failed by remember { mutableStateOf(false) }
    var editing by remember { mutableStateOf(false) }
    var confirmReset by remember { mutableStateOf(false) }

    suspend fun update(request: () -> String) = requests.withLock {
        try {
            routes = withContext(Dispatchers.IO) {
                val json = JSONObject(request())
                val items = json.getJSONArray("items")
                LearnedRoutes(List(items.length()) { items.getString(it) }, json.getInt("hidden"))
            }
            failed = false
        } catch (error: CancellationException) {
            throw error
        } catch (_: LinkageError) {
            failed = true
        } catch (_: Exception) {
            failed = true
        }
    }

    fun edit(request: () -> String) {
        if (editing) return
        editing = true
        scope.launch {
            try {
                update(request)
            } finally {
                editing = false
            }
        }
    }

    LaunchedEffect(dataDir) {
        if (!NativeCore.available) {
            failed = true
            return@LaunchedEffect
        }
        while (true) {
            update { Mobile.getAdaptiveSmartLearnedRoutes(dataDir) }
            delay(5_000)
        }
    }

    val snapshot = routes
    SheetRow(
        icon = Icons.Outlined.Route,
        tint = RvCategory.Emerald,
        title = stringResource(R.string.learned_routes_title),
        subtitle = when {
            failed -> stringResource(R.string.learned_routes_error)
            snapshot != null && snapshot.hosts.isEmpty() && snapshot.hidden == 0 ->
                stringResource(R.string.learned_routes_empty)
            else -> stringResource(R.string.learned_routes_desc)
        },
        trailing = {
            if (snapshot != null && (snapshot.hosts.isNotEmpty() || snapshot.hidden > 0)) {
                IconButton(enabled = !editing, onClick = { confirmReset = true }) {
                    Icon(Icons.Outlined.DeleteOutline, stringResource(R.string.learned_routes_reset), tint = RvColor.description)
                }
            }
        },
        below = {
            if (snapshot != null) {
                RemovableTagList(
                    values = snapshot.hosts,
                    enabled = !editing,
                    onRemove = { host -> edit { Mobile.forgetAdaptiveSmartLearnedRoute(dataDir, host) } },
                )
                if (snapshot.hidden > 0) {
                    Text(
                        pluralStringResource(R.plurals.learned_routes_hidden, snapshot.hidden, snapshot.hidden),
                        style = SheetNoteStyle,
                        color = RvColor.description,
                    )
                }
            }
        },
    )

    if (confirmReset) {
        AlertDialog(
            onDismissRequest = { confirmReset = false },
            title = { Text(stringResource(R.string.learned_routes_reset)) },
            text = { Text(stringResource(R.string.learned_routes_reset_desc)) },
            confirmButton = {
                TextButton(onClick = {
                    confirmReset = false
                    edit { Mobile.clearAdaptiveSmartLearnedRoutes(dataDir) }
                }) { Text(stringResource(R.string.learned_routes_reset_action)) }
            },
            dismissButton = {
                TextButton(onClick = { confirmReset = false }) { Text(stringResource(android.R.string.cancel)) }
            },
        )
    }
}
