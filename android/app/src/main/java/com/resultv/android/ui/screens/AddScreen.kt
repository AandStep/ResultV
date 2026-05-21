package com.resultv.android.ui.screens

import android.content.ClipboardManager
import android.content.Context
import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Check
import androidx.compose.material.icons.outlined.CloudDownload
import androidx.compose.material.icons.outlined.ContentPaste
import androidx.compose.material.icons.outlined.FileOpen
import androidx.compose.material.icons.outlined.Link
import androidx.compose.material.icons.outlined.QrCodeScanner
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ElevatedCard
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
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
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.resultv.android.R
import com.resultv.android.theme.Brand
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.codescanner.GmsBarcodeScannerOptions
import com.google.mlkit.vision.codescanner.GmsBarcodeScanning
import com.resultv.android.vpn.DeepLinkImporter
import com.resultv.android.vpn.Profile
import com.resultv.android.vpn.ProfileRepository
import com.resultv.android.vpn.Subscription
import com.resultv.android.vpn.SubscriptionRepository
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import mobile.Mobile
import org.json.JSONArray
import org.json.JSONObject

private enum class AddMode { Paste, Manual, Subscription }

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AddScreen(
    dataDir: String,
    onDone: () -> Unit,
) {
    val ctx = LocalContext.current
    val focusManager = LocalFocusManager.current
    val keyboard = LocalSoftwareKeyboardController.current
    var mode by remember { mutableStateOf(AddMode.Paste) }
    var importMessage by remember { mutableStateOf<String?>(null) }

    val defaultName = stringResource(R.string.add_paste_default_name)
    val msgFileEmpty = stringResource(R.string.add_msg_file_empty)
    val msgNoUrisFile = stringResource(R.string.add_msg_no_valid_uris_file)
    val msgClipboardEmpty = stringResource(R.string.add_msg_clipboard_empty)
    val msgNoUrisClipboard = stringResource(R.string.add_msg_no_valid_uris_clipboard)
    val msgQrEmpty = stringResource(R.string.add_msg_qr_empty)
    val msgQrUnsupported = stringResource(R.string.add_msg_qr_unsupported)
    val msgQrImported = stringResource(R.string.add_msg_qr_imported)
    val msgQrInvalid = stringResource(R.string.add_msg_qr_invalid)

    // SAF file picker — accepts any text/* and reads it as UTF-8.
    val filePicker = rememberLauncherForActivityResult(
        ActivityResultContracts.OpenDocument(),
    ) { uri ->
        if (uri != null) {
            val text = readTextFromUri(ctx, uri)
            if (text.isNullOrBlank()) {
                importMessage = msgFileEmpty
            } else {
                val added = importLines(text, defaultName)
                importMessage = if (added > 0)
                    ctx.getString(R.string.add_msg_imported_file, added)
                else msgNoUrisFile
            }
        }
    }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .pointerInput(Unit) {
                detectTapGestures(onTap = {
                    keyboard?.hide()
                    focusManager.clearFocus()
                })
            }
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        // Quick-import shortcuts (clipboard / file / QR).
        Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            QuickAddCard(
                icon = Icons.Outlined.ContentPaste,
                title = stringResource(R.string.add_quick_clipboard_title),
                subtitle = stringResource(R.string.add_quick_clipboard_subtitle),
                onClick = {
                    val cm = ctx.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                    val text = cm.primaryClip?.getItemAt(0)?.coerceToText(ctx)?.toString().orEmpty()
                    val added = importLines(text, defaultName)
                    importMessage = when {
                        text.isBlank() -> msgClipboardEmpty
                        added > 0 -> ctx.getString(R.string.add_msg_imported_clipboard, added)
                        else -> msgNoUrisClipboard
                    }
                },
                modifier = Modifier.weight(1f),
            )
            QuickAddCard(
                icon = Icons.Outlined.FileOpen,
                title = stringResource(R.string.add_quick_file_title),
                subtitle = stringResource(R.string.add_quick_file_subtitle),
                onClick = { filePicker.launch(arrayOf("*/*")) },
                modifier = Modifier.weight(1f),
            )
            QuickAddCard(
                icon = Icons.Outlined.QrCodeScanner,
                title = stringResource(R.string.add_quick_qr_title),
                subtitle = stringResource(R.string.add_quick_qr_subtitle),
                onClick = {
                    launchQrScan(
                        ctx = ctx,
                        defaultName = defaultName,
                        onResult = { msg -> importMessage = msg },
                        msgEmpty = msgQrEmpty,
                        msgUnsupported = msgQrUnsupported,
                        msgImported = msgQrImported,
                        msgInvalid = msgQrInvalid,
                    )
                },
                modifier = Modifier.weight(1f),
            )
        }

        importMessage?.let {
            Text(
                it,
                style = MaterialTheme.typography.bodySmall,
                color = Brand.SecondaryText,
            )
        }

        SingleChoiceSegmentedButtonRow(modifier = Modifier.fillMaxWidth()) {
            AddMode.entries.forEachIndexed { i, m ->
                SegmentedButton(
                    selected = mode == m,
                    onClick = { mode = m },
                    shape = SegmentedButtonDefaults.itemShape(i, AddMode.entries.size),
                ) {
                    Text(
                        text = stringResource(
                            when (m) {
                                AddMode.Paste -> R.string.add_mode_paste
                                AddMode.Manual -> R.string.add_mode_manual
                                AddMode.Subscription -> R.string.add_mode_subscription
                            },
                        ),
                    )
                }
            }
        }

        when (mode) {
            AddMode.Paste -> PastePane(onDone = onDone)
            AddMode.Manual -> ManualPane(onDone = onDone)
            AddMode.Subscription -> SubscriptionPane(dataDir = dataDir, onDone = onDone)
        }
    }
}

// ───────────────────────────── Quick-add card ────────────────────────────

@Composable
private fun QuickAddCard(
    icon: androidx.compose.ui.graphics.vector.ImageVector,
    title: String,
    @Suppress("UNUSED_PARAMETER") subtitle: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    // Square card — equal height for clipboard/file/QR. Subtitle dropped at
    // user request; tooltip-style hints live in i18n only.
    ElevatedCard(
        onClick = onClick,
        modifier = modifier.aspectRatio(1f),
        shape = RoundedCornerShape(20.dp),
        colors = CardDefaults.elevatedCardColors(containerColor = Brand.Surface),
    ) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(12.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp, Alignment.CenterVertically),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Box(
                modifier = Modifier
                    .size(40.dp)
                    .clip(RoundedCornerShape(12.dp))
                    .background(Brand.SurfaceHigh),
                contentAlignment = Alignment.Center,
            ) {
                Icon(icon, contentDescription = null, tint = Brand.GreenLight)
            }
            Text(title, style = MaterialTheme.typography.titleSmall)
        }
    }
}

// ──────────────────────────── Paste pane ────────────────────────────

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun PastePane(onDone: () -> Unit) {
    var uri by remember { mutableStateOf("") }
    var error by remember { mutableStateOf<String?>(null) }
    val focusManager = LocalFocusManager.current
    val keyboard = LocalSoftwareKeyboardController.current
    val errEmpty = stringResource(R.string.add_paste_err_empty)
    val errInvalid = stringResource(R.string.add_paste_err_invalid)
    val defaultName = stringResource(R.string.add_paste_default_name)

    val tryAdd = tryAdd@{
        val trimmed = uri.trim()
        if (trimmed.isEmpty()) { error = errEmpty; return@tryAdd }
        val name = try {
            Mobile.parseProxyURI(trimmed)
            nameFromUri(trimmed) ?: defaultName
        } catch (t: Throwable) {
            error = t.message ?: errInvalid
            return@tryAdd
        }
        ProfileRepository.add(Profile.fromUri(name, trimmed))
        keyboard?.hide()
        focusManager.clearFocus()
        onDone()
    }

    Card(
        shape = RoundedCornerShape(20.dp),
        colors = CardDefaults.cardColors(containerColor = Brand.Surface),
    ) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Text(
                stringResource(R.string.add_paste_label),
                style = MaterialTheme.typography.labelLarge,
                color = Brand.SecondaryText,
            )
            OutlinedTextField(
                value = uri,
                onValueChange = { uri = it; error = null },
                modifier = Modifier.fillMaxWidth(),
                placeholder = { Text(stringResource(R.string.add_paste_placeholder)) },
                isError = error != null,
                supportingText = error?.let { { Text(it) } },
                singleLine = true,
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
                keyboardActions = KeyboardActions(onDone = { tryAdd() }),
            )
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                FilledTonalButton(onClick = tryAdd) {
                    Icon(Icons.Outlined.Add, contentDescription = null)
                    Spacer(Modifier.width(8.dp))
                    Text(stringResource(R.string.action_add))
                }
                TextButton(onClick = {
                    uri = ""; error = null
                    keyboard?.hide(); focusManager.clearFocus()
                }) { Text(stringResource(R.string.action_clear)) }
            }
        }
    }
}

// ──────────────────────────── Subscription pane ─────────────────────

private data class FetchedEntry(
    val key: String,
    val name: String,
    val uri: String,
    val entryJson: String,
    val preview: String,
    val type: String,
    val isSection: Boolean,
)

/** What FetchSubscriptionV2 returns — entries + sub-level metadata. */
private data class FetchedSubscription(
    val entries: List<FetchedEntry>,
    val title: String,
    val userInfo: String,
)

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun SubscriptionPane(dataDir: String, onDone: () -> Unit) {
    var url by remember { mutableStateOf("") }
    var loading by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    var fetched by remember { mutableStateOf<FetchedSubscription?>(null) }
    val selected = remember { mutableStateOf(setOf<String>()) }
    val scope = rememberCoroutineScope()
    val focusManager = LocalFocusManager.current
    val keyboard = LocalSoftwareKeyboardController.current

    // Auto-tick everything real when a new fetch lands (SECTION rows are
    // imported alongside the selection but not toggleable).
    LaunchedEffect(fetched) {
        selected.value = fetched?.entries.orEmpty()
            .filter { !it.isSection }
            .map { it.key }
            .toSet()
    }

    val triggerFetch: () -> Unit = {
        if (!loading && url.isNotBlank()) {
            keyboard?.hide(); focusManager.clearFocus()
            doFetch(scope, url, dataDir,
                onLoad = { loading = it },
                onError = { error = it; fetched = null },
                onResult = { fetched = it; error = null },
            )
        }
    }

    Card(
        shape = RoundedCornerShape(20.dp),
        colors = CardDefaults.cardColors(containerColor = Brand.Surface),
    ) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Text(
                stringResource(R.string.add_sub_label),
                style = MaterialTheme.typography.labelLarge,
                color = Brand.SecondaryText,
            )
            OutlinedTextField(
                value = url,
                onValueChange = { url = it; error = null },
                modifier = Modifier.fillMaxWidth(),
                placeholder = { Text(stringResource(R.string.add_sub_placeholder)) },
                singleLine = true,
                isError = error != null,
                supportingText = error?.let { { Text(it) } },
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
                keyboardActions = KeyboardActions(onDone = { triggerFetch() }),
                leadingIcon = { Icon(Icons.Outlined.Link, contentDescription = null) },
            )
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                FilledTonalButton(
                    onClick = triggerFetch,
                    enabled = !loading && url.isNotBlank(),
                ) {
                    Icon(Icons.Outlined.CloudDownload, contentDescription = null)
                    Spacer(Modifier.width(8.dp))
                    Text(stringResource(if (loading) R.string.add_sub_fetching else R.string.add_sub_fetch))
                }
                if (loading) {
                    CircularProgressIndicator(
                        modifier = Modifier.heightIn(max = 18.dp).width(18.dp),
                        strokeWidth = 2.dp,
                    )
                }
            }

            val sub = fetched
            if (sub != null && sub.entries.isNotEmpty()) {
                val realCount = sub.entries.count { !it.isSection }
                Text(
                    text = stringResource(R.string.add_sub_selected, selected.value.size, realCount),
                    style = MaterialTheme.typography.bodySmall,
                    color = Brand.SecondaryText,
                )
                LazyColumn(
                    modifier = Modifier.heightIn(max = 360.dp),
                    verticalArrangement = Arrangement.spacedBy(2.dp),
                ) {
                    items(sub.entries, key = { it.key }) { e ->
                        if (e.isSection) {
                            // Render SECTION rows as inline subtitles, not
                            // checkboxes — they always import alongside the
                            // selection.
                            Text(
                                text = e.name,
                                style = MaterialTheme.typography.labelLarge,
                                color = Brand.SecondaryText,
                                modifier = Modifier.padding(start = 4.dp, top = 8.dp, bottom = 4.dp),
                            )
                            return@items
                        }
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            val checked = e.key in selected.value
                            Checkbox(
                                checked = checked,
                                onCheckedChange = { now ->
                                    selected.value = if (now) selected.value + e.key
                                    else selected.value - e.key
                                },
                            )
                            Column(modifier = Modifier.padding(start = 4.dp)) {
                                Text(
                                    e.name,
                                    style = MaterialTheme.typography.bodyMedium,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis,
                                )
                                Text(
                                    e.preview,
                                    style = MaterialTheme.typography.bodySmall,
                                    color = Brand.MutedText,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis,
                                )
                            }
                        }
                    }
                }

                FilledTonalButton(
                    enabled = selected.value.isNotEmpty(),
                    onClick = {
                        importSubscription(
                            url = url.trim(),
                            sub = sub,
                            selectedKeys = selected.value,
                            sourceTag = "",
                        )
                        onDone()
                    },
                ) {
                    Icon(Icons.Outlined.Check, contentDescription = null)
                    Spacer(Modifier.width(8.dp))
                    Text(stringResource(R.string.add_sub_import, selected.value.size))
                }
            }
        }
    }
}

// ──────────────────────────── Helpers ──────────────────────────

/**
 * Parse a chunk of text (clipboard or file) as a list of share-links, one
 * per line, importing each as a profile. Returns the count of successful
 * imports.
 */
private fun importLines(text: String, defaultName: String): Int {
    var added = 0
    text.lineSequence().forEach { raw ->
        val trimmed = raw.trim()
        if (trimmed.isEmpty()) return@forEach
        runCatching {
            Mobile.parseProxyURI(trimmed)
            val name = nameFromUri(trimmed) ?: defaultName
            ProfileRepository.add(Profile.fromUri(name, trimmed))
            added++
        }
    }
    return added
}

private fun readTextFromUri(ctx: Context, uri: Uri): String? = runCatching {
    ctx.contentResolver.openInputStream(uri)?.use { it.bufferedReader().readText() }
}.getOrNull()

private fun doFetch(
    scope: CoroutineScope,
    url: String,
    dataDir: String,
    onLoad: (Boolean) -> Unit,
    onError: (String) -> Unit,
    onResult: (FetchedSubscription) -> Unit,
) {
    scope.launch {
        onLoad(true)
        try {
            val responseJson = withContext(Dispatchers.IO) {
                Mobile.fetchSubscriptionV2(url.trim(), dataDir)
            }
            val response = JSONObject(responseJson)
            val arr = response.optJSONArray("entries") ?: JSONArray()
            val list = (0 until arr.length()).map { i -> parseEntry(arr.getJSONObject(i), i) }
            onResult(
                FetchedSubscription(
                    entries = list,
                    title = response.optString("title"),
                    userInfo = response.optString("userInfo"),
                )
            )
        } catch (t: Throwable) {
            onError(t.message ?: t.javaClass.simpleName)
        } finally {
            onLoad(false)
        }
    }
}

private fun parseEntry(o: JSONObject, index: Int): FetchedEntry {
    val uri = o.optString("uri")
    val ip = o.optString("ip")
    val port = o.optInt("port")
    val type = o.optString("type")
    val isSection = type.equals("SECTION", ignoreCase = true)
    val name = o.optString("name")
        .ifBlank { if (isSection) "—" else ip }
        .ifBlank { "Profile ${index + 1}" }
    val preview = when {
        isSection -> ""
        uri.isNotBlank() -> uri
        else -> listOf(type, "$ip:$port").filter { it.isNotBlank() }.joinToString("  ·  ")
    }
    val key = when {
        isSection -> "section|$index|${o.optString("name")}"
        uri.isNotBlank() -> uri
        else -> "$type|$ip|$port|$index"
    }
    return FetchedEntry(key, name, uri, o.toString(), preview, type, isSection)
}

/**
 * Materialise the picked entries: persist one [Subscription] record and a
 * batch of [Profile]s tagged with its id. SECTION rows from the response
 * are imported alongside the selection — they keep the visual structure
 * intact ("👇 выберите конфиг ниже") without being selectable.
 */
private fun importSubscription(
    url: String,
    sub: FetchedSubscription,
    selectedKeys: Set<String>,
    sourceTag: String,
) {
    val subscription = Subscription.new(
        url = url,
        name = sub.title.ifBlank { defaultSubscriptionName(url) },
        title = sub.title,
        userInfo = sub.userInfo,
        source = sourceTag,
    )
    SubscriptionRepository.add(subscription)
    // Preserve the response order so SECTION rows land between the right
    // group of real entries.
    val profiles = sub.entries.mapNotNull { e ->
        when {
            e.isSection -> Profile.section(name = e.name, subscriptionId = subscription.id)
            e.key !in selectedKeys -> null
            e.uri.isNotBlank() -> Profile.fromUri(e.name, e.uri, subscriptionId = subscription.id)
            else -> Profile.fromEntryJson(e.name, e.entryJson, subscriptionId = subscription.id)
        }
    }
    profiles.forEach { ProfileRepository.add(it) }
}

private fun defaultSubscriptionName(url: String): String {
    return runCatching {
        java.net.URI(url).host?.takeIf { it.isNotBlank() } ?: "Subscription"
    }.getOrDefault("Subscription")
}

private fun nameFromUri(uri: String): String? = runCatching {
    val parsed = JSONObject(Mobile.parseProxyURI(uri))
    parsed.optString("name").ifBlank { parsed.optString("ip").ifBlank { null } }
}.getOrNull()

/**
 * Open Google Play Services' bundled barcode scanner UI. Restricted to
 * QR codes (the only format we ever encode). The decoded value is routed
 * via [DeepLinkImporter.importPlain] so resultv://, http(s):// (subscription)
 * and bare share-links all import through one path. Single-URI QRs that
 * fail parsing fall back to an "invalid" message.
 */
private fun launchQrScan(
    ctx: Context,
    defaultName: String,
    onResult: (String) -> Unit,
    msgEmpty: String,
    msgUnsupported: String,
    msgImported: String,
    msgInvalid: String,
) {
    val options = GmsBarcodeScannerOptions.Builder()
        .setBarcodeFormats(Barcode.FORMAT_QR_CODE)
        .build()
    val scanner = GmsBarcodeScanning.getClient(ctx, options)
    scanner.startScan()
        .addOnSuccessListener { barcode ->
            val raw = barcode.rawValue.orEmpty().trim()
            if (raw.isEmpty()) {
                onResult(msgEmpty)
                return@addOnSuccessListener
            }
            val lower = raw.lowercase()
            when {
                lower.startsWith("resultv:") || lower.startsWith("http://") ||
                    lower.startsWith("https://") -> {
                    // DeepLinkImporter handles all three: rvsub deep-link,
                    // subscription URL, and rejects garbage with its own toast.
                    DeepLinkImporter.importPlain(ctx, raw)
                    onResult(msgImported)
                }
                else -> {
                    val added = importLines(raw, defaultName)
                    onResult(if (added > 0) msgImported else msgInvalid)
                }
            }
        }
        .addOnCanceledListener { onResult(msgEmpty) }
        .addOnFailureListener { onResult(msgUnsupported) }
}
