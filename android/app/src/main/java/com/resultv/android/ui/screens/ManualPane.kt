package com.resultv.android.ui.screens

import android.util.Base64
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Check
import androidx.compose.material3.AssistChip
import androidx.compose.material3.AssistChipDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.annotation.StringRes
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.foundation.text.KeyboardOptions
import com.resultv.android.R
import com.resultv.android.theme.Brand
import com.resultv.android.vpn.Profile
import com.resultv.android.vpn.ProfileRepository
import mobile.Mobile
import org.json.JSONObject
import java.net.URLEncoder

/**
 * Manual entry pane: pick a protocol, fill a form, build a share-URI string,
 * validate via Mobile.parseProxyURI, save the profile.
 *
 * URI builders mirror the schemes accepted by internal/proxy/uriparser.go.
 */
@Composable
fun ManualPane(onDone: () -> Unit) {
    var picked by remember { mutableStateOf<ProtocolSpec?>(null) }
    val current = picked
    if (current == null) {
        ProtocolGrid(onPick = { picked = it })
    } else {
        ProtocolForm(
            spec = current,
            onBack = { picked = null },
            onDone = onDone,
        )
    }
}

// ───────────────────────────── Grid ─────────────────────────────

@Composable
private fun ProtocolGrid(onPick: (ProtocolSpec) -> Unit) {
    Card(
        shape = RoundedCornerShape(20.dp),
        colors = CardDefaults.cardColors(containerColor = Brand.Surface),
    ) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Text(
                stringResource(R.string.manual_choose_protocol),
                style = MaterialTheme.typography.labelLarge,
                color = Brand.SecondaryText,
            )
            LazyVerticalGrid(
                columns = GridCells.Fixed(3),
                verticalArrangement = Arrangement.spacedBy(8.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                modifier = Modifier.height(280.dp),
            ) {
                items(Protocols, key = { it.id }) { spec ->
                    ProtocolCard(spec, onClick = { onPick(spec) })
                }
            }
        }
    }
}

@Composable
private fun ProtocolCard(spec: ProtocolSpec, onClick: () -> Unit) {
    Box(
        modifier = Modifier
            .fillMaxWidth()
            .height(80.dp)
            .clip(RoundedCornerShape(16.dp))
            .background(Brand.SurfaceHigh)
            .clickable(onClick = onClick),
        contentAlignment = Alignment.Center,
    ) {
        Column(
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(2.dp),
        ) {
            Text(
                spec.title,
                style = MaterialTheme.typography.titleSmall,
                fontWeight = FontWeight.SemiBold,
            )
            Text(
                spec.scheme,
                style = MaterialTheme.typography.bodySmall,
                color = Brand.MutedText,
            )
        }
    }
}

// ───────────────────────────── Form ─────────────────────────────

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ProtocolForm(
    spec: ProtocolSpec,
    onBack: () -> Unit,
    onDone: () -> Unit,
) {
    val ctx = LocalContext.current
    val values = remember(spec.id) {
        mutableStateMapOf<String, String>().apply {
            spec.fields.forEach { put(it.key, it.default) }
        }
    }
    var error by remember(spec.id) { mutableStateOf<String?>(null) }
    val errBuild = stringResource(R.string.manual_err_build)
    val errInvalid = stringResource(R.string.manual_err_invalid)

    val submit = submit@{
        val missing = spec.fields.firstOrNull {
            it.required && values[it.key].orEmpty().isBlank()
        }
        if (missing != null) {
            error = ctx.getString(R.string.manual_err_required, ctx.getString(missing.labelRes))
            return@submit
        }
        val uri = try {
            spec.build(values)
        } catch (t: Throwable) {
            error = t.message ?: errBuild
            return@submit
        }
        try {
            Mobile.parseProxyURI(uri)
        } catch (t: Throwable) {
            error = t.message ?: errInvalid
            return@submit
        }
        val name = values["name"].orEmpty().ifBlank { spec.title }
        ProfileRepository.add(Profile.fromUri(name, uri))
        onDone()
    }

    Card(
        shape = RoundedCornerShape(20.dp),
        colors = CardDefaults.cardColors(containerColor = Brand.Surface),
    ) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                IconButton(onClick = onBack) {
                    Icon(
                        Icons.AutoMirrored.Outlined.ArrowBack,
                        contentDescription = stringResource(R.string.action_back),
                    )
                }
                Spacer(Modifier.width(4.dp))
                Text(spec.title, style = MaterialTheme.typography.titleMedium)
            }

            spec.fields.forEach { f ->
                FieldRow(
                    field = f,
                    value = values[f.key].orEmpty(),
                    onValue = { values[f.key] = it; error = null },
                )
            }

            error?.let {
                Text(
                    it,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.error,
                )
            }

            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                FilledTonalButton(onClick = submit) {
                    Icon(Icons.Outlined.Check, contentDescription = null)
                    Spacer(Modifier.width(8.dp))
                    Text(stringResource(R.string.action_save))
                }
                TextButton(onClick = onBack) { Text(stringResource(R.string.action_cancel)) }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun FieldRow(
    field: Field,
    value: String,
    onValue: (String) -> Unit,
) {
    val label = stringResource(field.labelRes)
    val placeholder = when {
        field.placeholderRes != null -> stringResource(field.placeholderRes)
        field.placeholderLiteral != null -> field.placeholderLiteral
        else -> null
    }
    when (field.kind) {
        FieldKind.Text, FieldKind.Number, FieldKind.Password -> {
            OutlinedTextField(
                value = value,
                onValueChange = onValue,
                modifier = Modifier.fillMaxWidth(),
                label = {
                    Text(if (field.required) "$label *" else label)
                },
                placeholder = placeholder?.let { { Text(it) } },
                singleLine = true,
                visualTransformation = if (field.kind == FieldKind.Password)
                    PasswordVisualTransformation() else androidx.compose.ui.text.input.VisualTransformation.None,
                keyboardOptions = when (field.kind) {
                    FieldKind.Number -> KeyboardOptions(keyboardType = KeyboardType.Number)
                    FieldKind.Password -> KeyboardOptions(keyboardType = KeyboardType.Password)
                    else -> KeyboardOptions.Default
                },
            )
        }
        FieldKind.Choice -> {
            ChoiceField(
                label = label,
                value = value,
                options = field.options,
                onValue = onValue,
            )
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ChoiceField(
    label: String,
    value: String,
    options: List<String>,
    onValue: (String) -> Unit,
) {
    var expanded by remember { mutableStateOf(false) }
    Column {
        Text(label, style = MaterialTheme.typography.labelSmall, color = Brand.SecondaryText)
        Spacer(Modifier.height(4.dp))
        Box {
            AssistChip(
                onClick = { expanded = true },
                label = { Text(value.ifBlank { "—" }) },
                colors = AssistChipDefaults.assistChipColors(containerColor = Brand.SurfaceHigh),
            )
            DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
                options.forEach { opt ->
                    val none = stringResource(R.string.manual_choice_none)
                    DropdownMenuItem(
                        text = { Text(opt.ifBlank { none }) },
                        onClick = { onValue(opt); expanded = false },
                    )
                }
            }
        }
    }
}

// ───────────────────────────── Spec model ─────────────────────────────

private enum class FieldKind { Text, Number, Password, Choice }

private data class Field(
    val key: String,
    @StringRes val labelRes: Int,
    val kind: FieldKind = FieldKind.Text,
    val required: Boolean = false,
    val default: String = "",
    @StringRes val placeholderRes: Int? = null,
    val placeholderLiteral: String? = null,
    val options: List<String> = emptyList(),
)

private data class ProtocolSpec(
    val id: String,
    val title: String,
    val scheme: String,
    val fields: List<Field>,
    val build: (Map<String, String>) -> String,
)

// ───────────────────────────── Builders ─────────────────────────────

private val NameField = Field("name", R.string.manual_field_name, placeholderRes = R.string.manual_field_name_placeholder)
private val HostField = Field("host", R.string.manual_field_host, required = true, placeholderRes = R.string.manual_field_host_placeholder)
private val PortField = Field("port", R.string.manual_field_port, kind = FieldKind.Number, required = true, default = "443")

private fun enc(s: String): String = URLEncoder.encode(s, "UTF-8")

private fun frag(name: String): String =
    if (name.isBlank()) "" else "#" + enc(name)

private fun query(pairs: List<Pair<String, String>>): String {
    val nonEmpty = pairs.filter { it.second.isNotBlank() }
    if (nonEmpty.isEmpty()) return ""
    return "?" + nonEmpty.joinToString("&") { (k, v) -> "$k=${enc(v)}" }
}

private val Protocols: List<ProtocolSpec> = listOf(
    ProtocolSpec(
        id = "vless",
        title = "VLESS",
        scheme = "vless://",
        fields = listOf(
            NameField,
            HostField,
            PortField,
            Field("uuid", R.string.manual_field_uuid, required = true),
            Field("type", R.string.manual_field_network, kind = FieldKind.Choice, default = "tcp",
                options = listOf("tcp", "ws", "grpc", "xhttp", "httpupgrade", "h2")),
            Field("security", R.string.manual_field_security, kind = FieldKind.Choice, default = "none",
                options = listOf("none", "tls", "reality")),
            Field("flow", R.string.manual_field_flow, placeholderLiteral = "xtls-rprx-vision"),
            Field("sni", R.string.manual_field_sni),
            Field("fp", R.string.manual_field_fingerprint, kind = FieldKind.Choice, default = "",
                options = listOf("", "chrome", "firefox", "safari", "ios", "android", "edge", "random")),
            Field("pbk", R.string.manual_field_pbk),
            Field("sid", R.string.manual_field_sid),
            Field("path", R.string.manual_field_path),
            Field("host_header", R.string.manual_field_host_header),
            Field("alpn", R.string.manual_field_alpn, placeholderRes = R.string.manual_field_alpn_placeholder),
        ),
        build = { v ->
            val name = v["name"].orEmpty()
            "vless://${enc(v["uuid"]!!)}@${v["host"]}:${v["port"]}" +
                query(listOf(
                    "type" to v["type"].orEmpty(),
                    "security" to v["security"].orEmpty(),
                    "flow" to v["flow"].orEmpty(),
                    "sni" to v["sni"].orEmpty(),
                    "fp" to v["fp"].orEmpty(),
                    "pbk" to v["pbk"].orEmpty(),
                    "sid" to v["sid"].orEmpty(),
                    "path" to v["path"].orEmpty(),
                    "host" to v["host_header"].orEmpty(),
                    "alpn" to v["alpn"].orEmpty(),
                )) + frag(name)
        },
    ),

    ProtocolSpec(
        id = "vmess",
        title = "VMess",
        scheme = "vmess://",
        fields = listOf(
            NameField,
            HostField,
            PortField,
            Field("uuid", R.string.manual_field_uuid, required = true),
            Field("aid", R.string.manual_field_alterid, kind = FieldKind.Number, default = "0"),
            Field("net", R.string.manual_field_network, kind = FieldKind.Choice, default = "tcp",
                options = listOf("tcp", "ws", "grpc", "h2")),
            Field("path", R.string.manual_field_path),
            Field("host_header", R.string.manual_field_host_header),
            Field("tls", R.string.manual_field_tls, kind = FieldKind.Choice, default = "",
                options = listOf("", "tls")),
            Field("sni", R.string.manual_field_sni),
        ),
        build = { v ->
            val obj = JSONObject()
                .put("v", "2")
                .put("ps", v["name"].orEmpty().ifBlank { "VMess" })
                .put("add", v["host"])
                .put("port", v["port"])
                .put("id", v["uuid"])
                .put("aid", v["aid"].orEmpty().ifBlank { "0" })
                .put("net", v["net"].orEmpty().ifBlank { "tcp" })
                .put("path", v["path"].orEmpty())
                .put("host", v["host_header"].orEmpty())
                .put("tls", v["tls"].orEmpty())
            if (!v["sni"].isNullOrBlank()) obj.put("sni", v["sni"])
            val b64 = Base64.encodeToString(
                obj.toString().toByteArray(Charsets.UTF_8),
                Base64.NO_WRAP or Base64.NO_PADDING,
            )
            "vmess://$b64"
        },
    ),

    ProtocolSpec(
        id = "trojan",
        title = "Trojan",
        scheme = "trojan://",
        fields = listOf(
            NameField,
            HostField,
            PortField,
            Field("password", R.string.manual_field_password, kind = FieldKind.Password, required = true),
            Field("type", R.string.manual_field_network, kind = FieldKind.Choice, default = "tcp",
                options = listOf("tcp", "ws", "grpc")),
            Field("security", R.string.manual_field_security, kind = FieldKind.Choice, default = "tls",
                options = listOf("tls", "reality", "none")),
            Field("sni", R.string.manual_field_sni),
            Field("fp", R.string.manual_field_fingerprint, kind = FieldKind.Choice, default = "",
                options = listOf("", "chrome", "firefox", "safari", "ios", "android", "edge", "random")),
            Field("path", R.string.manual_field_path),
            Field("alpn", R.string.manual_field_alpn),
        ),
        build = { v ->
            "trojan://${enc(v["password"]!!)}@${v["host"]}:${v["port"]}" +
                query(listOf(
                    "type" to v["type"].orEmpty(),
                    "security" to v["security"].orEmpty(),
                    "sni" to v["sni"].orEmpty(),
                    "fp" to v["fp"].orEmpty(),
                    "path" to v["path"].orEmpty(),
                    "alpn" to v["alpn"].orEmpty(),
                )) + frag(v["name"].orEmpty())
        },
    ),

    ProtocolSpec(
        id = "ss",
        title = "Shadowsocks",
        scheme = "ss://",
        fields = listOf(
            NameField,
            HostField,
            PortField,
            Field("method", R.string.manual_field_method, kind = FieldKind.Choice, default = "aes-256-gcm",
                options = listOf(
                    "aes-256-gcm", "aes-128-gcm", "chacha20-ietf-poly1305",
                    "2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm",
                    "2022-blake3-chacha20-poly1305", "none",
                )),
            Field("password", R.string.manual_field_password, kind = FieldKind.Password, required = true),
        ),
        build = { v ->
            val auth = "${v["method"].orEmpty()}:${v["password"]}"
            val b64 = Base64.encodeToString(
                auth.toByteArray(Charsets.UTF_8),
                Base64.NO_WRAP or Base64.NO_PADDING or Base64.URL_SAFE,
            )
            "ss://$b64@${v["host"]}:${v["port"]}" + frag(v["name"].orEmpty())
        },
    ),

    ProtocolSpec(
        id = "hy2",
        title = "Hysteria2",
        scheme = "hy2://",
        fields = listOf(
            NameField,
            HostField,
            PortField,
            Field("password", R.string.manual_field_password, kind = FieldKind.Password, required = true),
            Field("sni", R.string.manual_field_sni),
            Field("alpn", R.string.manual_field_alpn, default = "h3"),
            Field("insecure", R.string.manual_field_allow_insecure, kind = FieldKind.Choice, default = "0",
                options = listOf("0", "1")),
            Field("obfs", R.string.manual_field_obfuscation, kind = FieldKind.Choice, default = "",
                options = listOf("", "salamander")),
            Field("obfs-password", R.string.manual_field_obfs_password, kind = FieldKind.Password),
        ),
        build = { v ->
            "hy2://${enc(v["password"]!!)}@${v["host"]}:${v["port"]}" +
                query(listOf(
                    "sni" to v["sni"].orEmpty(),
                    "alpn" to v["alpn"].orEmpty(),
                    "insecure" to v["insecure"].orEmpty(),
                    "obfs" to v["obfs"].orEmpty(),
                    "obfs-password" to v["obfs-password"].orEmpty(),
                )) + frag(v["name"].orEmpty())
        },
    ),

    ProtocolSpec(
        id = "wg",
        title = "WireGuard",
        scheme = "wg://",
        fields = listOf(
            NameField,
            HostField,
            PortField,
            Field("private_key", R.string.manual_field_private_key, kind = FieldKind.Password, required = true),
            Field("public_key", R.string.manual_field_peer_public_key, required = true),
            Field("address", R.string.manual_field_address, placeholderRes = R.string.manual_field_address_placeholder,
                required = true),
            Field("allowed_ips", R.string.manual_field_allowed_ips, default = "0.0.0.0/0, ::/0"),
            Field("pre_shared_key", R.string.manual_field_psk, kind = FieldKind.Password),
            Field("mtu", R.string.manual_field_mtu, kind = FieldKind.Number, default = "1408"),
        ),
        build = { v ->
            "wg://${enc(v["private_key"]!!)}@${v["host"]}:${v["port"]}" +
                query(listOf(
                    "public_key" to v["public_key"].orEmpty(),
                    "address" to v["address"].orEmpty(),
                    "allowed_ips" to v["allowed_ips"].orEmpty(),
                    "pre_shared_key" to v["pre_shared_key"].orEmpty(),
                    "mtu" to v["mtu"].orEmpty(),
                )) + frag(v["name"].orEmpty())
        },
    ),

    ProtocolSpec(
        id = "awg",
        title = "AmneziaWG",
        scheme = "awg://",
        fields = listOf(
            NameField,
            HostField,
            PortField,
            Field("private_key", R.string.manual_field_private_key, kind = FieldKind.Password, required = true),
            Field("public_key", R.string.manual_field_peer_public_key, required = true),
            Field("address", R.string.manual_field_address, required = true,
                placeholderRes = R.string.manual_field_address_placeholder),
            Field("allowed_ips", R.string.manual_field_allowed_ips, default = "0.0.0.0/0, ::/0"),
            Field("pre_shared_key", R.string.manual_field_psk, kind = FieldKind.Password),
            Field("mtu", R.string.manual_field_mtu, kind = FieldKind.Number, default = "1408"),
            // AmneziaWG obfuscation params — protocol-defined, not localised.
            Field("jc", R.string.awg_jc, kind = FieldKind.Number),
            Field("jmin", R.string.awg_jmin, kind = FieldKind.Number),
            Field("jmax", R.string.awg_jmax, kind = FieldKind.Number),
            Field("s1", R.string.awg_s1, kind = FieldKind.Number),
            Field("s2", R.string.awg_s2, kind = FieldKind.Number),
            Field("s3", R.string.awg_s3, kind = FieldKind.Number),
            Field("s4", R.string.awg_s4, kind = FieldKind.Number),
            Field("h1", R.string.awg_h1),
            Field("h2", R.string.awg_h2),
            Field("h3", R.string.awg_h3),
            Field("h4", R.string.awg_h4),
            Field("i1", R.string.awg_i1),
            Field("i2", R.string.awg_i2),
            Field("i3", R.string.awg_i3),
            Field("i4", R.string.awg_i4),
            Field("i5", R.string.awg_i5),
            Field("j1", R.string.awg_j1),
            Field("j2", R.string.awg_j2),
            Field("j3", R.string.awg_j3),
        ),
        build = { v ->
            val pairs = mutableListOf(
                "public_key" to v["public_key"].orEmpty(),
                "address" to v["address"].orEmpty(),
                "allowed_ips" to v["allowed_ips"].orEmpty(),
                "pre_shared_key" to v["pre_shared_key"].orEmpty(),
                "mtu" to v["mtu"].orEmpty(),
            )
            listOf("jc", "jmin", "jmax", "s1", "s2", "s3", "s4",
                "h1", "h2", "h3", "h4",
                "i1", "i2", "i3", "i4", "i5", "j1", "j2", "j3").forEach { k ->
                v[k]?.takeIf { it.isNotBlank() }?.let { pairs.add(k to it) }
            }
            "awg://${enc(v["private_key"]!!)}@${v["host"]}:${v["port"]}" +
                query(pairs) + frag(v["name"].orEmpty())
        },
    ),

    ProtocolSpec(
        id = "naive",
        title = "Naive",
        scheme = "naive+https://",
        fields = listOf(
            NameField,
            HostField,
            PortField,
            Field("username", R.string.manual_field_username, required = true),
            Field("password", R.string.manual_field_password, kind = FieldKind.Password, required = true),
            Field("sni", R.string.manual_field_sni),
        ),
        build = { v ->
            "naive+https://${enc(v["username"]!!)}:${enc(v["password"]!!)}@${v["host"]}:${v["port"]}" +
                query(listOf(
                    "sni" to v["sni"].orEmpty(),
                )) + frag(v["name"].orEmpty())
        },
    ),
)
