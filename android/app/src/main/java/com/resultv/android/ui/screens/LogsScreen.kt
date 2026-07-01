package com.resultv.android.ui.screens

import android.widget.Toast
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.DeleteOutline
import androidx.compose.material.icons.outlined.SaveAlt
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.resultv.android.R
import com.resultv.android.theme.Brand
import com.resultv.android.vpn.AppLog
import com.resultv.android.vpn.LogEntry
import com.resultv.android.vpn.LogLevel
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

private val timeFormat = SimpleDateFormat("HH:mm:ss", Locale.US)

private fun LogLevel.color(): Color = when (this) {
    LogLevel.Info -> Brand.SecondaryText
    LogLevel.Success -> Brand.GreenLight
    LogLevel.Warning -> Brand.Warning
    LogLevel.Error -> Brand.Danger
}

/** Render the full buffer oldest-first as `[time] SOURCE message` per line. */
private fun exportText(entries: List<LogEntry>): String =
    entries.asReversed().joinToString("\n") { e ->
        val time = "[${timeFormat.format(Date(e.timestamp))}]"
        val source = if (e.source.isNotEmpty()) " ${e.source}" else ""
        "$time$source ${e.message}"
    }

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun LogsScreen(onBack: () -> Unit) {
    val entries by AppLog.entries.collectAsStateWithLifecycle()
    val ctx = LocalContext.current

    val saveLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.CreateDocument("text/plain"),
    ) { uri ->
        if (uri == null) return@rememberLauncherForActivityResult
        val ok = runCatching {
            ctx.contentResolver.openOutputStream(uri)?.use { out ->
                out.write(exportText(AppLog.entries.value).toByteArray())
            }
        }.isSuccess
        Toast.makeText(
            ctx,
            ctx.getString(if (ok) R.string.logs_saved else R.string.logs_save_failed),
            Toast.LENGTH_SHORT,
        ).show()
    }

    Scaffold(
        containerColor = Brand.Bg,
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.logs_title), fontWeight = FontWeight.Bold) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(
                            Icons.AutoMirrored.Outlined.ArrowBack,
                            contentDescription = stringResource(R.string.action_back),
                        )
                    }
                },
                actions = {
                    IconButton(
                        onClick = {
                            val now = SimpleDateFormat("yyyy-MM-dd_HH-mm-ss", Locale.US)
                                .format(Date())
                            saveLauncher.launch("resultv-logs-$now.txt")
                        },
                    ) {
                        Icon(
                            Icons.Outlined.SaveAlt,
                            contentDescription = stringResource(R.string.logs_save_cd),
                            tint = Brand.SecondaryText,
                        )
                    }
                    IconButton(onClick = { AppLog.clear() }) {
                        Icon(
                            Icons.Outlined.DeleteOutline,
                            contentDescription = stringResource(R.string.logs_clear_cd),
                            tint = Brand.Danger,
                        )
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = Brand.Bg),
            )
        },
    ) { padding ->
        if (entries.isEmpty()) {
            Box(
                modifier = Modifier.fillMaxSize().padding(padding),
                contentAlignment = Alignment.Center,
            ) {
                Text(
                    stringResource(R.string.logs_empty),
                    style = MaterialTheme.typography.bodyMedium,
                    color = Brand.MutedText,
                )
            }
        } else {
            LazyColumn(
                modifier = Modifier.fillMaxSize(),
                contentPadding = PaddingValues(
                    start = 16.dp,
                    end = 16.dp,
                    top = padding.calculateTopPadding() + 4.dp,
                    bottom = padding.calculateBottomPadding() + 24.dp,
                ),
                verticalArrangement = Arrangement.spacedBy(2.dp),
            ) {
                items(entries, key = { it.timestamp.toString() + it.message.hashCode() }) { entry ->
                    LogRow(entry)
                }
            }
        }
    }
}

@Composable
private fun LogRow(entry: LogEntry) {
    Row(
        modifier = Modifier.padding(vertical = 6.dp),
        horizontalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Text(
            text = "[${timeFormat.format(Date(entry.timestamp))}]",
            style = MaterialTheme.typography.bodySmall,
            fontFamily = FontFamily.Monospace,
            color = Brand.MutedText,
        )
        Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
            if (entry.source.isNotEmpty()) {
                Text(
                    text = entry.source,
                    style = MaterialTheme.typography.labelSmall,
                    fontWeight = FontWeight.SemiBold,
                    color = Brand.MutedText,
                )
            }
            Text(
                text = entry.message,
                style = MaterialTheme.typography.bodyMedium,
                color = entry.level.color(),
            )
        }
    }
}
