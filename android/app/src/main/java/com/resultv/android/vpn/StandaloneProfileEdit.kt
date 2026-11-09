package com.resultv.android.vpn

import org.json.JSONObject
import java.net.URLEncoder

internal val standardProxyTypes = listOf("HTTP", "HTTPS", "SOCKS4", "SOCKS4A", "SOCKS5")
private val supportedEntryTypes = standardProxyTypes + listOf(
    "VLESS", "VMESS", "TROJAN", "SS", "HYSTERIA2", "WIREGUARD", "AMNEZIAWG", "NAIVEPROXY",
)

internal fun Profile.canEditConnection(): Boolean =
    subscriptionId.isBlank() && !isSection && !isAuto && (uri.isNotBlank() || entryJson.isNotBlank())

internal fun Profile.standardProxyType(): String? {
    val kind = rawType.ifBlank { uri.substringBefore("://", "") }.uppercase()
    return when (kind) {
        "SOCKS", "SOCKS5H" -> "SOCKS5"
        else -> kind.takeIf { it in standardProxyTypes }
    }
}

internal fun standardProxyEditUri(
    type: String,
    address: String,
    portText: String,
    username: String,
    password: String,
): String {
    require(type in standardProxyTypes)
    val host = address.trim()
    require(host.isNotEmpty() && host.none { it.isWhitespace() || it in "@/#?" })
    val port = portText.trim().toInt()
    require(port in 1..65535)
    val authority = if (':' in host && !host.startsWith('[')) "[$host]" else host
    val userInfo = if (username.isEmpty() && password.isEmpty()) "" else {
        val user = encodeProxyCredential(username)
        val secret = encodeProxyCredential(password)
        "$user:$secret@"
    }
    return "${type.lowercase()}://$userInfo$authority:$port"
}

private fun encodeProxyCredential(value: String): String =
    URLEncoder.encode(value, "UTF-8").replace("+", "%20")

internal fun applyStandardProxyEdit(original: JSONObject, parsed: JSONObject, name: String): JSONObject {
    val result = JSONObject(original.toString())
    for (field in listOf("type", "ip", "port", "username", "password")) {
        result.put(field, parsed.get(field))
    }
    if (!original.optString("ip").equals(parsed.getString("ip"), ignoreCase = true)) {
        result.put("country", parsed.optString("country"))
    }
    return result.put("name", name)
}

internal fun applyRawProxyEntryEdit(original: JSONObject?, raw: String, name: String): JSONObject {
    val entry = JSONObject(raw)
    val host = entry.get("ip") as? String
    val port = entry.get("port") as? Int
    require(!host.isNullOrBlank() && port != null && port in 1..65535)
    val type = entry.getString("type").uppercase()
    require(type in supportedEntryTypes)
    entry.put("type", type)
    require(entry.isNull("extra") || entry.optJSONObject("extra") != null)
    if (original != null && !original.optString("ip").equals(entry.getString("ip"), ignoreCase = true) &&
        original.optString("country").equals(entry.optString("country"), ignoreCase = true)
    ) {
        entry.remove("country")
    }
    return entry.put("name", name)
}
