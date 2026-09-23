package com.resultv.android.ui.screens

import android.content.ClipboardManager
import android.content.Context
import android.net.Uri
import android.widget.Toast
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.annotation.DrawableRes
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp
import com.resultv.android.theme.SegoeUi
import com.resultv.android.ui.components.RvButton
import com.resultv.android.ui.components.RvButtonColors
import com.resultv.android.ui.components.RvButtonLabel
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
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
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.setValue
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.resultv.android.R
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.RvSpace
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.codescanner.GmsBarcodeScannerOptions
import com.google.mlkit.vision.codescanner.GmsBarcodeScanning
import com.resultv.android.vpn.DeepLinkImporter
import com.resultv.android.vpn.Profile
import com.resultv.android.vpn.ProfileRepository
import com.resultv.android.vpn.Subscription
import com.resultv.android.vpn.SubscriptionRepository
import com.resultv.android.vpn.SubscriptionRouting
import com.resultv.android.vpn.WireGuardConfParser
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import mobile.Mobile
import org.json.JSONArray
import org.json.JSONObject

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AddScreen(
    dataDir: String,
    onDone: () -> Unit,
) {
    val ctx = LocalContext.current
    val focusManager = LocalFocusManager.current
    val keyboard = LocalSoftwareKeyboardController.current
    val scope = rememberCoroutineScope()
    var importMessage by remember { mutableStateOf<String?>(null) }

    val defaultName = stringResource(R.string.add_paste_default_name)
    val msgFileEmpty = stringResource(R.string.add_msg_file_empty)
    val msgNoUrisFile = stringResource(R.string.add_msg_no_valid_uris_file)

    // SAF file picker — accepts any text/* and reads it as UTF-8.
    val filePicker = rememberLauncherForActivityResult(
        ActivityResultContracts.OpenDocument(),
    ) { uri ->
        if (uri != null) {
            val text = readTextFromUri(ctx, uri)
            // Success surfaces as a toast (like subscription imports); errors
            // stay as the persistent inline message so the user can read them.
            when {
                text.isNullOrBlank() -> importMessage = msgFileEmpty
                WireGuardConfParser.isWireGuardConf(text) -> {
                    val msg = importWireGuardConf(text, defaultName)
                    if (msg != null) {
                        importMessage = null
                        Toast.makeText(ctx, msg, Toast.LENGTH_LONG).show()
                    } else {
                        importMessage = msgNoUrisFile
                    }
                }
                else -> {
                    val added = importBlob(text)
                    if (added > 0) {
                        importMessage = null
                        Toast.makeText(
                            ctx,
                            ctx.getString(R.string.add_msg_imported_file, added),
                            Toast.LENGTH_LONG,
                        ).show()
                    } else {
                        importMessage = msgNoUrisFile
                    }
                }
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
            // Макет AddPage (Figma 6863:4833): поле 12, между блоками 8.
            .padding(start = 12.dp, end = 12.dp, bottom = 12.dp),
        verticalArrangement = Arrangement.spacedBy(RvSpace.nest3),
    ) {
        // Быстрые источники: файл, буфер, QR — плитки BigBtn макета.
        Row(horizontalArrangement = Arrangement.spacedBy(RvSpace.nest3)) {
            BigBtn(
                icon = R.drawable.ic_upload_file,
                label = stringResource(R.string.add_quick_file_title),
                onClick = { filePicker.launch(arrayOf("*/*")) },
                modifier = Modifier.weight(1f),
            )
            BigBtn(
                icon = R.drawable.ic_paste,
                label = stringResource(R.string.add_quick_clipboard_title),
                onClick = { pasteFromClipboard(ctx, scope, dataDir) { importMessage = it } },
                modifier = Modifier.weight(1f),
            )
            BigBtn(
                icon = R.drawable.ic_qr_scan,
                label = stringResource(R.string.add_quick_qr_title),
                onClick = { scanQr(ctx) { importMessage = it } },
                modifier = Modifier.weight(1f),
            )
        }

        importMessage?.let {
            Text(
                it,
                style = MaterialTheme.typography.bodySmall,
                color = RvColor.whiteA50,
            )
        }

        LinkPane(dataDir = dataDir, onDone = onDone)
    }
}

// ───────────────────────────── BigBtn ─────────────────────────────

/** Плитка быстрого источника — компонент BigBtn макета: 106 в высоту, значок 32. */
@Composable
private fun BigBtn(
    @DrawableRes icon: Int,
    label: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val shape = RoundedCornerShape(16.dp)
    Column(
        modifier = modifier
            .height(106.dp)
            .clip(shape)
            .background(RvColor.Grey)
            .border(1.dp, RvColor.whiteA10, shape)
            .clickable(role = Role.Button, onClick = onClick),
        verticalArrangement = Arrangement.spacedBy(RvSpace.nest3, Alignment.CenterVertically),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Icon(
            painter = painterResource(icon),
            contentDescription = null,
            tint = RvColor.whiteA50,
            modifier = Modifier.size(32.dp),
        )
        Text(label, style = RvButtonLabel, fontWeight = FontWeight.Bold, color = RvColor.whiteA50)
    }
}

// ───────────────────────────── Protocols ─────────────────────────────

/**
 * Протоколы ряда «Выберите протокол» — порядок и написание с макета, как на
 * ПК (ADD_PAGE_PROTOCOLS). Выбор меняет только подпись и подсказку поля:
 * разбор вставленного всё равно определяет формат сам.
 */
private class AddProtocol(
    val key: String,
    val label: String,
    /** Схема ссылки для подсказки; null — протокол вставляется конфигом. */
    val scheme: String?,
    val configHint: String? = null,
)

/** У обычного WireGuard нет джиттера, поэтому строки `Jc` в его конфиге нет. */
private const val WG_HINT = "[Interface]\nPrivateKey = \nAddress = \nDNS = "

private val AddProtocols = listOf(
    AddProtocol("vless", "VLESS", "vless"),
    AddProtocol("hysteria2", "Hysteria2", "hysteria2"),
    AddProtocol("amneziawg", "AmneziaWG", null, "$WG_HINT\nJc = "),
    AddProtocol("wireguard", "Wireguard", null, WG_HINT),
    AddProtocol("trojan", "Trojan", "trojan"),
    AddProtocol("vmess", "VMESS", "vmess"),
    AddProtocol("ss", "SS", "ss"),
    AddProtocol("naive", "NaiveProxy", "naive+https"),
    AddProtocol("http", "HTTP(S)", "http"),
    AddProtocol("socks5", "Socks5", "socks5"),
)

/**
 * Ряд протоколов шире экрана и прокручивается вбок; справа затухание 96,
 * пока есть что листать.
 */
@Composable
private fun ProtocolRow(selected: String, onSelect: (String) -> Unit) {
    val scroll = rememberScrollState()
    Box(modifier = Modifier.fillMaxWidth()) {
        Row(
            modifier = Modifier.horizontalScroll(scroll),
            horizontalArrangement = Arrangement.spacedBy(RvSpace.nest3),
        ) {
            AddProtocols.forEach { p ->
                val on = p.key == selected
                RvButton(
                    onClick = { onSelect(p.key) },
                    fill = if (on) RvButtonColors.greenSelected else RvButtonColors.greenFill,
                    outline = RvButtonColors.greenOutline,
                    modifier = Modifier.semantics { this.selected = on },
                ) {
                    Text(
                        text = p.label,
                        style = RvButtonLabel,
                        fontWeight = FontWeight.Bold,
                        color = RvColor.Main,
                        modifier = Modifier.padding(horizontal = 32.dp),
                    )
                }
            }
        }
        if (scroll.canScrollForward) {
            Box(
                modifier = Modifier
                    .align(Alignment.CenterEnd)
                    .width(96.dp)
                    .height(52.dp)
                    .background(Brush.horizontalGradient(listOf(Color.Transparent, RvColor.Grey))),
            )
        }
    }
}

/** Подписи полей и текст в поле: 12 Semibold, межстрочный 1.4. */
private val FieldTextStyle = TextStyle(
    fontFamily = SegoeUi,
    fontSize = 12.sp,
    lineHeight = 16.8.sp,
    fontWeight = FontWeight.SemiBold,
)

/**
 * Поле ссылки — Textarea макета: 140 в высоту, Dark Grey, скругление 16.
 * В фокусе рамка Main 20 %, с ошибкой — красная.
 */
@Composable
private fun LinkField(
    value: String,
    onValueChange: (String) -> Unit,
    hint: String,
    isError: Boolean,
) {
    var focused by remember { mutableStateOf(false) }
    val shape = RoundedCornerShape(16.dp)
    BasicTextField(
        value = value,
        onValueChange = onValueChange,
        modifier = Modifier
            .fillMaxWidth()
            .height(140.dp)
            .onFocusChanged { focused = it.isFocused },
        textStyle = FieldTextStyle.copy(color = RvColor.White),
        cursorBrush = SolidColor(RvColor.whiteA50),
        decorationBox = { inner ->
            Box(
                modifier = Modifier
                    .fillMaxSize()
                    .clip(shape)
                    .background(RvColor.DarkGrey)
                    .border(
                        1.dp,
                        when {
                            isError -> RvColor.errorsA50
                            focused -> RvColor.mainA20
                            else -> Color.Transparent
                        },
                        shape,
                    )
                    .padding(12.dp),
            ) {
                if (value.isEmpty()) {
                    Text(hint, style = FieldTextStyle, color = RvColor.whiteA20)
                }
                inner()
            }
        },
    )
}

// ──────────────────────────── Unified Link pane ────────────────────────────
//
// One textfield handles everything a user might paste:
//   - `resultv://…`         → DeepLinkImporter (RVSUB1 ciphertext, etc.)
//   - `http(s)://…`         → subscription fetch + selection list
//   - `vless://`, `vmess://`, `trojan://`, `hy2://`, …  → single share-link
//
// The previous split between "Paste link" and "Subscription" tabs was
// forcing users to know in advance which format they had — the unified
// pane just dispatches by prefix.

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun LinkPane(dataDir: String, onDone: () -> Unit) {
    val ctx = LocalContext.current
    val focusManager = LocalFocusManager.current
    val keyboard = LocalSoftwareKeyboardController.current
    val scope = rememberCoroutineScope()

    var input by remember { mutableStateOf("") }
    var protocolKey by rememberSaveable { mutableStateOf(AddProtocols.first().key) }
    val protocol = AddProtocols.first { it.key == protocolKey }
    var error by remember { mutableStateOf<String?>(null) }
    var loading by remember { mutableStateOf(false) }
    // Non-null once an http(s):// fetch succeeds — switches the pane from
    // "paste field" mode to "pick entries" mode for that subscription.
    var fetched by remember { mutableStateOf<FetchedSubscription?>(null) }
    var fetchedUrl by remember { mutableStateOf("") }
    val selected = remember { mutableStateOf(setOf<String>()) }

    val errEmpty = stringResource(R.string.add_link_err_empty)
    val errInvalid = stringResource(R.string.add_link_err_invalid)
    val defaultName = stringResource(R.string.add_paste_default_name)

    // Auto-tick everything real when a new fetch lands (SECTION rows are
    // imported alongside the selection but not toggleable).
    LaunchedEffect(fetched) {
        selected.value = fetched?.entries.orEmpty()
            .filter { !it.isSection }
            .map { it.key }
            .toSet()
    }

    val submit = submit@{
        val trimmed = input.trim()
        if (trimmed.isEmpty()) { error = errEmpty; return@submit }
        keyboard?.hide(); focusManager.clearFocus()
        val lower = trimmed.lowercase()
        // A subscription URL is one line. Several lines that merely start with
        // http:// are a pasted list of links, not a URL to fetch.
        val singleLine = trimmed.lineSequence().count { it.isNotBlank() } == 1

        when {
            // resultv://… — RVSUB1 ciphertext or opaque deep-link. The
            // importer parses, fetches if needed, and emits its own toast;
            // we close the Add screen so the user lands on Proxies where
            // the new entries show up.
            lower.startsWith("resultv:") -> {
                DeepLinkImporter.import(ctx, trimmed)
                input = ""; error = null
                onDone()
            }
            // http(s):// — subscription URL. Fetch and surface the selection
            // list so the user can untick anything they don't want.
            singleLine && (lower.startsWith("http://") || lower.startsWith("https://")) -> {
                error = null
                doFetch(
                    scope = scope,
                    url = trimmed,
                    dataDir = dataDir,
                    onLoad = { loading = it },
                    onError = { error = it; fetched = null; fetchedUrl = "" },
                    onResult = { fetched = it; fetchedUrl = trimmed; error = null },
                )
            }
            // WireGuard / AmneziaWG .conf — an INI file, so it has to be
            // recognised before the parser, which speaks links and JSON.
            WireGuardConfParser.isWireGuardConf(trimmed) -> {
                if (importWireGuardConf(trimmed, defaultName) == null) {
                    error = errInvalid
                    return@submit
                }
                input = ""; error = null
                onDone()
            }
            // Everything else goes to the same parser a subscription body
            // does: share-links one per line, base64, an xray or sing-box
            // outbound, a whole config with an `outbounds` array.
            else -> {
                if (importBlob(trimmed) == 0) {
                    error = errInvalid
                    return@submit
                }
                input = ""; error = null
                onDone()
            }
        }
    }

    // Карточка формы макета: Grey, скругление 24, поле 14, между блоками 12.
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(24.dp))
            .background(RvColor.Grey)
            .padding(14.dp),
        verticalArrangement = Arrangement.spacedBy(RvSpace.nest2),
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(RvSpace.nest3)) {
            Text(
                stringResource(R.string.manual_choose_protocol),
                style = FieldTextStyle,
                color = RvColor.whiteA50,
            )
            ProtocolRow(selected = protocolKey, onSelect = { protocolKey = it })
        }

        Column(verticalArrangement = Arrangement.spacedBy(RvSpace.nest3)) {
            Text(
                stringResource(
                    if (protocol.scheme == null) R.string.add_config_label else R.string.add_link_label,
                ),
                style = FieldTextStyle,
                color = RvColor.whiteA50,
            )
            // Многострочное: .conf или JSON вставляют целиком, список ссылок —
            // по нескольку строк. Enter переносит строку, добавляет кнопка ниже.
            LinkField(
                value = input,
                onValueChange = {
                    input = it
                    error = null
                    // Editing the input invalidates the previous fetch
                    // preview — user has switched to a new URL.
                    if (fetched != null && it.trim() != fetchedUrl) {
                        fetched = null; fetchedUrl = ""
                    }
                },
                hint = protocol.configHint
                    ?: stringResource(R.string.add_link_hint, protocol.scheme.orEmpty()),
                isError = error != null,
            )
            error?.let { Text(it, style = FieldTextStyle, color = RvColor.Errors) }
        }

        RvButton(
            onClick = submit,
            fill = RvButtonColors.greenFill,
            outline = RvButtonColors.greenOutline,
            enabled = !loading && input.isNotBlank(),
            modifier = Modifier.fillMaxWidth(),
        ) {
            if (loading) {
                CircularProgressIndicator(
                    modifier = Modifier.size(14.dp),
                    strokeWidth = 2.dp,
                    color = RvColor.Main,
                )
            }
            Text(
                stringResource(if (loading) R.string.add_sub_fetching else R.string.home_add_server),
                style = RvButtonLabel,
                fontWeight = FontWeight.Bold,
                color = RvColor.Main,
            )
        }

        // Subscription preview — only rendered after a successful
        // http(s):// fetch. SECTION rows are inlined as labels and ride
        // along with whatever's selected.
        val sub = fetched
        if (sub != null && sub.entries.isNotEmpty()) {
            val realKeys = remember(sub) {
                sub.entries.filterNot { it.isSection }.map { it.key }.toSet()
            }
            val realCount = realKeys.size
            val allSelected = selected.value.size >= realCount && realCount > 0

            // Selection summary + select-all toggle, kept directly above
            // the import action so neither is buried under the (possibly
            // long) server list.
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    text = stringResource(R.string.add_sub_selected, selected.value.size, realCount),
                    style = MaterialTheme.typography.bodySmall,
                    color = RvColor.whiteA50,
                    modifier = Modifier.weight(1f),
                )
                TextButton(onClick = {
                    selected.value = if (allSelected) emptySet() else realKeys
                }) {
                    Text(
                        stringResource(
                            if (allSelected) R.string.add_sub_clear_all
                            else R.string.add_sub_select_all,
                        ),
                    )
                }
            }

            // Import button pinned above the list — the user no longer
            // has to scroll to the bottom of all entries to commit.
            RvButton(
                fill = RvButtonColors.greenFill,
                outline = RvButtonColors.greenOutline,
                modifier = Modifier.fillMaxWidth(),
                enabled = selected.value.isNotEmpty(),
                onClick = {
                    importSubscription(
                        url = fetchedUrl,
                        sub = sub,
                        selectedKeys = selected.value,
                        sourceTag = "",
                        dataDir = dataDir,
                    )
                    onDone()
                },
            ) {
                Text(
                    stringResource(R.string.add_sub_import, selected.value.size),
                    style = RvButtonLabel,
                    fontWeight = FontWeight.Bold,
                    color = RvColor.Main,
                )
            }

            LazyColumn(
                modifier = Modifier.heightIn(max = 360.dp),
                verticalArrangement = Arrangement.spacedBy(2.dp),
            ) {
                items(sub.entries, key = { it.key }) { e ->
                    if (e.isSection) {
                        Text(
                            text = e.name,
                            style = MaterialTheme.typography.labelLarge,
                            color = RvColor.whiteA50,
                            modifier = Modifier.padding(start = RvSpace.xs, top = RvSpace.nest3, bottom = RvSpace.xs),
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
                        Column(modifier = Modifier.padding(start = RvSpace.xs)) {
                            Text(
                                e.name,
                                style = MaterialTheme.typography.bodyMedium,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                            )
                            if (e.preview.isNotBlank()) {
                                Text(
                                    e.preview,
                                    style = MaterialTheme.typography.bodySmall,
                                    color = RvColor.whiteA50,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis,
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}

// ──────────────────────────── Fetched-subscription model ─────────────────

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
    val supportUrl: String,
    /** Маршрутизация провайдера, уже свёрнутая Go в профиль. Пусто — её нет. */
    val routingJson: String = "",
)

// ──────────────────────────── Helpers ──────────────────────────

/**
 * Parse a WireGuard/AmneziaWG .conf file, add one Profile, and return a
 * human-readable success message, or null on parse failure.
 */
private fun importWireGuardConf(text: String, defaultName: String): String? {
    val uri = WireGuardConfParser.toUri(text, defaultName) ?: return null
    return runCatching {
        val parsed = org.json.JSONObject(Mobile.parseProxyURI(uri))
        val name = parsed.optString("name").ifBlank { parsed.optString("type").ifBlank { defaultName } }
        val protocol = parsed.optString("type").uppercase()
        ProfileRepository.add(Profile.fromUri(name, uri))
        "Imported $protocol: $name"
    }.getOrNull()
}

/**
 * Parse a pasted chunk of text and add a profile per entry it yields.
 * Returns how many landed.
 *
 * The format is decided by the content, not by the user: share-links one per
 * line, a base64 subscription body, an RVSUB1 ciphertext, an xray or sing-box
 * outbound, a whole config with an `outbounds` array — the same parser a
 * downloaded subscription goes through.
 *
 * This replaces a per-line loop over ParseProxyURI, which is why a pasted JSON
 * config used to import nothing at all: the parser's JSON branch was reachable
 * only behind a network fetch.
 */
private fun importBlob(text: String): Int {
    val json = runCatching { Mobile.parseProxyBlob(text) }.getOrNull() ?: return 0
    val arr = runCatching { JSONArray(json) }.getOrNull() ?: return 0
    var added = 0
    for (i in 0 until arr.length()) {
        val o = arr.optJSONObject(i) ?: continue
        val entry = parseEntry(o, i)
        // SECTION rows are a subscription's visual structure; pasted text has
        // no list for them to structure.
        if (entry.isSection) continue
        ProfileRepository.add(
            if (entry.uri.isNotBlank()) Profile.fromUri(entry.name, entry.uri)
            else Profile.fromEntryJson(entry.name, entry.entryJson)
        )
        added++
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
                Mobile.fetchSubscriptionV3(
                    url.trim(),
                    dataDir,
                    com.resultv.android.vpn.BuildOptionsBuilder.currentSubscriptionFetchOptionsJson(),
                )
            }
            val response = JSONObject(responseJson)
            val arr = response.optJSONArray("entries") ?: JSONArray()
            val list = (0 until arr.length()).map { i -> parseEntry(arr.getJSONObject(i), i) }
            onResult(
                FetchedSubscription(
                    entries = list,
                    title = response.optString("title"),
                    userInfo = response.optString("userInfo"),
                    supportUrl = response.optString("supportUrl"),
                    routingJson = response.optString("routing"),
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
        .ifBlank { if (isSection) "—" else "Profile ${index + 1}" }
    // Preview intentionally omits host/ip:port — the server address is
    // sensitive and shouldn't be shown while the user is still deciding
    // what to import. Only the protocol/type is surfaced as a hint.
    val preview = if (isSection) "" else type.uppercase()
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
    dataDir: String,
) {
    // Re-importing the same URL updates the existing record in place rather
    // than spawning a duplicate; a fresh URL inserts a new one.
    val subId = SubscriptionRepository.upsert(
        url = url,
        name = sub.title.ifBlank { defaultSubscriptionName(url) },
        title = sub.title,
        userInfo = sub.userInfo,
        supportUrl = sub.supportUrl,
        source = sourceTag,
    )
    val favouriteNames = ProfileRepository.favouriteNamesFor(subId)
    // Preserve the response order so SECTION rows land between the right
    // group of real entries.
    val profiles = sub.entries.mapNotNull { e ->
        when {
            e.isSection -> Profile.section(name = e.name, subscriptionId = subId)
            e.key !in selectedKeys -> null
            e.uri.isNotBlank() -> Profile.fromUri(e.name, e.uri, subscriptionId = subId)
            else -> Profile.fromEntryJson(e.name, e.entryJson, subscriptionId = subId)
        }?.let { p -> if (!p.isSection && p.name in favouriteNames) p.copy(isFavorite = true) else p }
    }
    ProfileRepository.replaceForSubscription(subId, profiles)
    // Маршрутизация провайдера приехала тем же ответом, что и серверы.
    // activate=true: пользователь только что сам согласился на эту подписку.
    SubscriptionRouting.acceptAsync(
        routingJson = sub.routingJson,
        subId = subId,
        subName = sub.title.ifBlank { url },
        dataDir = dataDir,
        activate = true,
    )
}

private fun defaultSubscriptionName(url: String): String {
    return runCatching {
        java.net.URI(url).host?.takeIf { it.isNotBlank() } ?: "Subscription"
    }.getOrDefault("Subscription")
}

/**
 * «Из буфера»: берёт текст из буфера обмена и отдаёт его [smartClipboardImport].
 * Общая точка для быстрой карточки здесь и кнопки «Вставить» на главной.
 */
internal fun pasteFromClipboard(
    ctx: Context,
    scope: CoroutineScope,
    dataDir: String,
    onMessage: (String?) -> Unit,
) {
    val cm = ctx.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
    val text = cm.primaryClip?.getItemAt(0)?.coerceToText(ctx)?.toString().orEmpty()
    if (text.isBlank()) {
        onMessage(ctx.getString(R.string.add_msg_clipboard_empty))
        return
    }
    // Smart dispatch: deep-link → DeepLinkImporter (own toast, no inline
    // message); single http(s):// URL → fetch + auto-import all entries;
    // everything else → line-by-line share-link import.
    smartClipboardImport(
        ctx = ctx,
        scope = scope,
        text = text,
        dataDir = dataDir,
        defaultName = ctx.getString(R.string.add_paste_default_name),
        msgNoUrisClipboard = ctx.getString(R.string.add_msg_no_valid_uris_clipboard),
        msgImportedClipboard = { n -> ctx.getString(R.string.add_msg_imported_clipboard, n) },
        msgFetchFailed = ctx.getString(R.string.deeplink_err_fetch),
        msgImportedSubscription = { n -> ctx.getString(R.string.deeplink_imported_subscription, n) },
        onMessage = onMessage,
    )
}

/** «Сканер QR» — общая точка для карточки здесь и кнопки на главной. */
internal fun scanQr(ctx: Context, onMessage: (String) -> Unit) {
    launchQrScan(
        ctx = ctx,
        defaultName = ctx.getString(R.string.add_paste_default_name),
        onResult = onMessage,
        msgEmpty = ctx.getString(R.string.add_msg_qr_empty),
        msgUnsupported = ctx.getString(R.string.add_msg_qr_unsupported),
        msgImported = ctx.getString(R.string.add_msg_qr_imported),
        msgInvalid = ctx.getString(R.string.add_msg_qr_invalid),
    )
}

/**
 * Clipboard quick-add dispatcher. Detects deep-links / subscription URLs /
 * WireGuard .conf / everything else the parser reads (links, base64, xray and
 * sing-box configs) and runs the right import path. For http(s):// URLs we
 * auto-import every entry rather than surfacing the selection UI — the
 * selection flow lives in the Link tab itself for users who want it.
 *
 * Same ladder as the Link pane's own submit, in the same order.
 */
private fun smartClipboardImport(
    ctx: Context,
    scope: CoroutineScope,
    text: String,
    dataDir: String,
    defaultName: String,
    msgNoUrisClipboard: String,
    msgImportedClipboard: (Int) -> String,
    msgFetchFailed: String,
    msgImportedSubscription: (Int) -> String,
    onMessage: (String?) -> Unit,
) {
    val trimmed = text.trim()
    val lower = trimmed.lowercase()

    if (lower.startsWith("resultv:")) {
        // DeepLinkImporter emits its own toast on success or failure;
        // suppress the inline message so we don't double up.
        DeepLinkImporter.import(ctx, trimmed)
        onMessage(null)
        return
    }

    val nonBlankLines = trimmed.lineSequence().filter { it.isNotBlank() }.toList()
    val isSingleHttpUrl = nonBlankLines.size == 1 &&
        (lower.startsWith("http://") || lower.startsWith("https://"))
    if (isSingleHttpUrl) {
        scope.launch {
            try {
                val responseJson = withContext(Dispatchers.IO) {
                    Mobile.fetchSubscriptionV3(
                        trimmed,
                        dataDir,
                        com.resultv.android.vpn.BuildOptionsBuilder.currentSubscriptionFetchOptionsJson(),
                    )
                }
                val response = JSONObject(responseJson)
                val arr = response.optJSONArray("entries") ?: JSONArray()
                val list = (0 until arr.length()).map { i -> parseEntry(arr.getJSONObject(i), i) }
                val sub = FetchedSubscription(
                    entries = list,
                    title = response.optString("title"),
                    userInfo = response.optString("userInfo"),
                    supportUrl = response.optString("supportUrl"),
                    routingJson = response.optString("routing"),
                )
                val allKeys = list.filter { !it.isSection }.map { it.key }.toSet()
                importSubscription(trimmed, sub, allKeys, "", dataDir)
                onMessage(msgImportedSubscription(allKeys.size))
            } catch (_: Throwable) {
                onMessage(msgFetchFailed)
            }
        }
        return
    }

    if (WireGuardConfParser.isWireGuardConf(trimmed)) {
        val msg = importWireGuardConf(trimmed, defaultName)
        onMessage(msg ?: msgNoUrisClipboard)
        return
    }

    val added = importBlob(trimmed)
    onMessage(if (added > 0) msgImportedClipboard(added) else msgNoUrisClipboard)
}

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
                    val added = importBlob(raw)
                    onResult(if (added > 0) msgImported else msgInvalid)
                }
            }
        }
        .addOnCanceledListener { onResult(msgEmpty) }
        .addOnFailureListener { onResult(msgUnsupported) }
}
