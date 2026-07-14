package com.resultv.android.vpn

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import android.util.Log
import java.io.IOException
import java.net.Inet6Address
import java.net.InetSocketAddress
import java.net.Socket
import java.util.concurrent.Executors

private const val TAG = "ResultV/NetProbe"

/**
 * Detects whether the *underlying* (non-VPN) network has WORKING IPv6.
 *
 * The IPv6 settings toggle makes the engine resolve AAAA and build a
 * dual-stack TUN. On a carrier that advertises an IPv6 address but blackholes
 * v6 transit, every `direct` IPv6 connection stalls ~5s on an i/o timeout —
 * and Smart mode routes all non-listed traffic to `direct`, so a search in the
 * browser hangs / returns 502. A presence check alone isn't enough (the
 * address exists but the route is dead), so we actually probe reachability and
 * only let IPv6 through when it truly works.
 *
 * The result is cached and read NON-BLOCKING from [BuildOptionsBuilder], which
 * runs on the main thread on the connect path. The blocking probe runs on a
 * dedicated background thread; callers kick [refreshAsync] at app start and on
 * each connect. The default before the first probe is `false` — the safe
 * direction (IPv4-only never hangs), so a broken-v6 network is fixed even on
 * the very first connect.
 */
object NetworkProbe {

    // Cloudflare + Google public-DNS v6 anycast: highly available and not in
    // our ad/route reject lists, so a failed probe means broken transit, not a
    // blocked host.
    private val probeHosts = listOf("2606:4700:4700::1111", "2001:4860:4860::8888")
    private const val PROBE_PORT = 443
    private const val PROBE_TIMEOUT_MS = 1500
    // Don't re-probe more often than this; v6 capability rarely flips mid-session.
    private const val TTL_MS = 30_000L

    @Volatile private var appContext: Context? = null
    @Volatile private var usable: Boolean = false
    @Volatile private var lastProbeAt: Long = 0L

    private val io = Executors.newSingleThreadExecutor { r ->
        Thread(r, "ResultV-NetProbe").apply { isDaemon = true }
    }

    fun init(ctx: Context) {
        if (appContext == null) appContext = ctx.applicationContext
    }

    /** Non-blocking: last known IPv6 reachability. `false` until the first probe. */
    fun usableIPv6(): Boolean = usable

    /** Kick a background refresh unless a recent probe already ran. */
    fun refreshAsync() {
        if (System.currentTimeMillis() - lastProbeAt < TTL_MS) return
        io.execute { refresh() }
    }

    /** Blocking reachability probe. MUST be called off the main thread. */
    @Synchronized
    fun refresh() {
        val ctx = appContext ?: return
        val result = runCatching { probe(ctx) }.getOrDefault(false)
        usable = result
        lastProbeAt = System.currentTimeMillis()
        Log.i(TAG, "underlying IPv6 usable=$result")
    }

    private fun probe(ctx: Context): Boolean {
        val cm = ctx.getSystemService(ConnectivityManager::class.java) ?: return false
        val net = underlyingNetwork(cm) ?: return false
        if (!hasGlobalV6Address(cm, net)) return false
        // Address present — verify the route actually carries traffic. Bind the
        // probe socket to the underlying network so it never rides our own TUN.
        val factory = net.socketFactory
        for (host in probeHosts) {
            var socket: Socket? = null
            try {
                socket = factory.createSocket()
                socket.connect(InetSocketAddress(host, PROBE_PORT), PROBE_TIMEOUT_MS)
                return true
            } catch (_: IOException) {
                // try the next host
            } finally {
                try { socket?.close() } catch (_: IOException) {}
            }
        }
        return false
    }

    /** First validated, internet-capable network that is NOT our VPN. */
    private fun underlyingNetwork(cm: ConnectivityManager): Network? {
        for (n in cm.allNetworks) {
            val caps = cm.getNetworkCapabilities(n) ?: continue
            if (caps.hasTransport(NetworkCapabilities.TRANSPORT_VPN)) continue
            if (!caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)) continue
            return n
        }
        return null
    }

    private fun hasGlobalV6Address(cm: ConnectivityManager, net: Network): Boolean {
        val lp = cm.getLinkProperties(net) ?: return false
        for (la in lp.linkAddresses) {
            val a = la.address
            if (a is Inet6Address &&
                !a.isLinkLocalAddress && !a.isLoopbackAddress &&
                !a.isAnyLocalAddress && !a.isMulticastAddress &&
                !isUniqueLocal(a)
            ) {
                return true
            }
        }
        return false
    }

    // fc00::/7 — unique-local addresses aren't globally routable.
    private fun isUniqueLocal(a: Inet6Address): Boolean {
        val b = a.address
        return b.isNotEmpty() && (b[0].toInt() and 0xfe) == 0xfc
    }
}
