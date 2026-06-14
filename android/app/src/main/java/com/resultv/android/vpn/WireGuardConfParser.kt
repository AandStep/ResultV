package com.resultv.android.vpn

import java.net.URLEncoder

/**
 * Parses a WireGuard / AmneziaWG `.conf` file (INI format) and builds a
 * share-URI compatible with [mobile.Mobile.parseProxyURI].
 *
 * The file format is a standard INI file with [Interface] and [Peer] sections:
 *
 *   [Interface]
 *   PrivateKey = <base64>
 *   Address    = 10.0.0.1/24, ::/0
 *   DNS        = 1.1.1.1
 *   MTU        = 1420
 *   # AmneziaWG extras:
 *   Jc = 4  Jmin = 40  Jmax = 70  S1 = 0  S2 = 0  H1 = 1  H2 = 2  H3 = 3  H4 = 4
 *
 *   [Peer]
 *   PublicKey    = <base64>
 *   PresharedKey = <base64>
 *   Endpoint     = 1.2.3.4:51820
 *   AllowedIPs   = 0.0.0.0/0, ::/0
 */
object WireGuardConfParser {

    /** Returns true if [text] looks like a WireGuard/AmneziaWG .conf file. */
    fun isWireGuardConf(text: String): Boolean =
        text.lineSequence().any { it.trim().equals("[Interface]", ignoreCase = true) }

    /**
     * Parse [text] and return a URI string (`wg://…` or `awg://…`) ready to
     * be passed to [mobile.Mobile.parseProxyURI], or null on failure.
     *
     * [defaultName] is used when no better name can be derived from the file.
     */
    fun toUri(text: String, defaultName: String = "WireGuard"): String? {
        val sections = parseSections(text)
        val iface = sections["interface"] ?: return null
        val peer = sections["peer"] ?: return null

        val privateKey = iface["privatekey"] ?: return null
        val publicKey = peer["publickey"] ?: return null
        val endpoint = peer["endpoint"] ?: return null

        val address = iface["address"].orEmpty()
        val allowedIps = peer["allowedips"].orEmpty().ifBlank { "0.0.0.0/0, ::/0" }
        val preSharedKey = peer["presharedkey"].orEmpty()
        val mtu = iface["mtu"].orEmpty()

        // Amnezia-specific obfuscation fields — presence of any of these
        // switches the scheme to awg://.
        val jc = iface["jc"].orEmpty()
        val jmin = iface["jmin"].orEmpty()
        val jmax = iface["jmax"].orEmpty()
        val s1 = iface["s1"].orEmpty()
        val s2 = iface["s2"].orEmpty()
        val s3 = iface["s3"].orEmpty()
        val s4 = iface["s4"].orEmpty()
        val h1 = iface["h1"].orEmpty()
        val h2 = iface["h2"].orEmpty()
        val h3 = iface["h3"].orEmpty()
        val h4 = iface["h4"].orEmpty()
        val i1 = iface["i1"].orEmpty()
        val i2 = iface["i2"].orEmpty()
        val i3 = iface["i3"].orEmpty()
        val i4 = iface["i4"].orEmpty()
        val i5 = iface["i5"].orEmpty()
        val j1 = iface["j1"].orEmpty()
        val j2 = iface["j2"].orEmpty()
        val j3 = iface["j3"].orEmpty()

        val isAmnezia = listOf(jc, jmin, jmax, s1, s2, s3, s4, h1, h2, h3, h4).any { it.isNotBlank() }
        val scheme = if (isAmnezia) "awg" else "wg"
        val name = if (isAmnezia) "AmneziaWG" else defaultName

        val (host, port) = splitEndpoint(endpoint)

        val pairs = mutableListOf(
            "public_key" to publicKey,
            "address" to address,
            "allowed_ips" to allowedIps,
            "pre_shared_key" to preSharedKey,
            "mtu" to mtu,
        )
        if (isAmnezia) {
            listOf(
                "jc" to jc, "jmin" to jmin, "jmax" to jmax,
                "s1" to s1, "s2" to s2, "s3" to s3, "s4" to s4,
                "h1" to h1, "h2" to h2, "h3" to h3, "h4" to h4,
                "i1" to i1, "i2" to i2, "i3" to i3, "i4" to i4, "i5" to i5,
                "j1" to j1, "j2" to j2, "j3" to j3,
            ).forEach { pairs.add(it) }
        }

        val query = buildQuery(pairs)
        val fragment = if (name.isBlank()) "" else "#${enc(name)}"
        return "$scheme://${enc(privateKey)}@$host:$port$query$fragment"
    }

    // ── INI parser ────────────────────────────────────────────────────────

    private fun parseSections(text: String): Map<String, Map<String, String>> {
        val sections = linkedMapOf<String, MutableMap<String, String>>()
        var current: String? = null
        for (line in text.lineSequence()) {
            val trimmed = line.trim()
            if (trimmed.isEmpty() || trimmed.startsWith("#")) continue
            if (trimmed.startsWith("[") && trimmed.endsWith("]")) {
                current = trimmed.substring(1, trimmed.length - 1).lowercase()
                // Keep first occurrence of each section (ignore duplicate [Peer]s)
                sections.getOrPut(current) { mutableMapOf() }
            } else if (current != null) {
                val eq = trimmed.indexOf('=')
                if (eq < 1) continue
                val key = trimmed.substring(0, eq).trim().lowercase()
                val value = trimmed.substring(eq + 1).substringBefore('#').trim()
                sections[current]!!.putIfAbsent(key, value)
            }
        }
        return sections
    }

    // ── URI helpers (same pattern as ManualPane.kt) ───────────────────────

    private fun enc(s: String): String = URLEncoder.encode(s, "UTF-8")

    private fun buildQuery(pairs: List<Pair<String, String>>): String {
        val nonEmpty = pairs.filter { it.second.isNotBlank() }
        if (nonEmpty.isEmpty()) return ""
        return "?" + nonEmpty.joinToString("&") { (k, v) -> "$k=${enc(v)}" }
    }

    private fun splitEndpoint(endpoint: String): Pair<String, String> {
        val lastColon = endpoint.lastIndexOf(':')
        return if (lastColon > 0) {
            endpoint.substring(0, lastColon) to endpoint.substring(lastColon + 1)
        } else {
            endpoint to "51820"
        }
    }
}
