package com.resultv.android.vpn

import android.content.Context
import android.net.ConnectivityManager
import android.net.VpnService
import android.os.ParcelFileDescriptor
import android.os.Process
import android.util.Log
import com.resultv.android.R
import libbox.CommandServer
import libbox.CommandServerHandler
import libbox.ConnectionOwner
import libbox.InterfaceUpdateListener
import libbox.Libbox
import libbox.LocalDNSTransport
import libbox.NetworkInterfaceIterator
import libbox.Notification
import libbox.OverrideOptions
import libbox.PlatformInterface
import libbox.SetupOptions
import libbox.StringIterator
import libbox.SystemProxyStatus
import libbox.TunOptions
import libbox.WIFIState
import java.io.File
import java.net.InetAddress
import java.net.InetSocketAddress

private const val TAG = "ResultV/Box"
const val BROWSER_ADBLOCK_PORT = 8130

// gomobile names Java packages by the Go package name only — for
// `package libbox` the Java imports live under the unqualified `libbox.*`
// namespace. The same applies to our wrapper `package mobile` -> `mobile.*`.

/**
 * Singleton that owns the libbox CommandServer for the lifetime of the
 * VPN service. Setup is one-shot (idempotent); start/stop can cycle.
 */
object BoxModule {
    private var setupDone = false
    private var commandServer: CommandServer? = null

    val isRunning: Boolean
        @Synchronized get() = commandServer != null

    /**
     * Set by ResultVpnService BEFORE calling BoxModule.start(), only when
     * Mobile.startFilterProxy() has already returned successfully. Read by
     * BoxPlatform.openTun() to decide whether to apply setHttpProxy — see
     * applyBrowserAdBlockProxy() for why this can't just read the settings
     * toggle directly.
     */
    @Volatile var filterProxyRunning: Boolean = false

    // One warning per session: protect() is called per-socket and a broken
    // state would otherwise flood the user log.
    @Volatile var protectWarned: Boolean = false

    /**
     * Configure libbox global paths. Must be called before any Service
     * construction. Safe to call multiple times.
     */
    @Synchronized
    fun ensureSetup(ctx: Context) {
        if (setupDone) return
        val base = ctx.filesDir
        val work = File(base, "work").apply { mkdirs() }
        val tmp = ctx.cacheDir
        val opts = SetupOptions().apply {
            basePath = base.absolutePath
            workingPath = work.absolutePath
            tempPath = tmp.absolutePath
            // grpc command socket — picking 0 forces a unix-domain socket
            // under basePath/command.sock, which is what we want on Android
            commandServerListenPort = 0
            // Workaround for golang.org/issues/68760 on Android.
            fixAndroidStack = true
            // Routes sing-box's log entries through our CommandServerHandler.
            // writeDebugMessage so they land in logcat. Without this flag the
            // log subscriber stays silent.
            debug = true
        }
        Libbox.setup(opts)
        setupDone = true
        Log.i(TAG, "libbox.Setup done; basePath=${base.absolutePath}")
    }

    @Synchronized
    fun start(service: ResultVpnService, configJson: String) {
        if (commandServer != null) {
            // A previous instance is still up — typically the last disconnect's
            // teardown hadn't finished when the user reconnected. Silently
            // returning here left the UI in Connecting forever (start "succeeds"
            // but nothing ever flips the state to Connected). Stop the stale
            // engine and start fresh instead.
            Log.w(TAG, "start() called while already running; stopping stale engine first")
            stop()
        }
        ensureSetup(service)

        protectWarned = false

        // Dump config in chunks (logcat caps lines around 4 KB).
        Log.i(TAG, "── config begin ──")
        configJson.chunked(3500).forEach { Log.i(TAG, it) }
        Log.i(TAG, "── config end ──")

        val platform = BoxPlatform(service)
        val handler = StubCommandHandler()
        val server = Libbox.newCommandServer(handler, platform)
        server.start()
        // OverrideOptions is empty — no per-app routing yet.
        server.startOrReloadService(configJson, OverrideOptions())
        commandServer = server
        Log.i(TAG, "BoxModule started")
    }

    @Synchronized
    fun reload(configJson: String) {
        val server = commandServer
        if (server == null) {
            Log.w(TAG, "reload() called while not running")
            return
        }
        
        Log.i(TAG, "── config reload begin ──")
        configJson.chunked(3500).forEach { Log.i(TAG, it) }
        Log.i(TAG, "── config reload end ──")
        
        server.startOrReloadService(configJson, OverrideOptions())
        Log.i(TAG, "BoxModule reloaded")
        AppLog.info(R.string.log_engine_reloaded, source = EngineLog.ENGINE)
    }

    @Synchronized
    fun stop() {
        val server = commandServer ?: return
        commandServer = null
        try {
            server.closeService()
        } catch (t: Throwable) {
            Log.w(TAG, "closeService threw", t)
            AppLog.warning(R.string.log_engine_stop_error,
                t.message ?: t.javaClass.simpleName, source = EngineLog.ENGINE)
        }
        try {
            server.close()
        } catch (t: Throwable) {
            Log.w(TAG, "close threw", t)
        }
        Log.i(TAG, "BoxModule stopped")
        AppLog.info(R.string.log_engine_stopped, source = EngineLog.ENGINE)
    }
}

/**
 * Implementation of libbox.PlatformInterface. The interesting method is
 * [openTun]; the rest are stubs that return safe defaults so libbox
 * stops asking us about features we do not support in the PoC.
 */
private class BoxPlatform(private val service: ResultVpnService) : PlatformInterface {

    override fun openTun(options: TunOptions): Int {
        // Use explicit getters to side-step Kotlin's ambiguous bean-name
        // mapping for all-caps Go names like getMTU / getDNSServerAddress.
        val builder = service.Builder()
        builder.setSession("ResultV")
        applyBrowserAdBlockProxy(builder)
        // Kotlin's bean-mapping is unreliable for all-caps acronyms
        // (getMTU / getDNSServerAddress) — call the explicit getters.
        val mtu = options.getMTU()
        builder.setMtu(if (mtu > 0) mtu else 9000)

        // RoutePrefix uses Go-style methods (address(), prefix()), not bean getters.
        val inet4 = options.inet4Address
        while (inet4.hasNext()) {
            val p = inet4.next()
            builder.addAddress(p.address(), p.prefix())
        }
        val inet6 = options.inet6Address
        while (inet6.hasNext()) {
            val p = inet6.next()
            builder.addAddress(p.address(), p.prefix())
        }

        // Auto-route: libbox passes 0.0.0.0/0 (and ::/0) via RouteRange.
        // If autoRoute is off we still respect the explicit route list.
        val ipv4Routes = if (options.autoRoute) options.inet4RouteRange else options.inet4RouteAddress
        while (ipv4Routes.hasNext()) {
            val p = ipv4Routes.next()
            builder.addRoute(p.address(), p.prefix())
        }
        val ipv6Routes = if (options.autoRoute) options.inet6RouteRange else options.inet6RouteAddress
        while (ipv6Routes.hasNext()) {
            val p = ipv6Routes.next()
            builder.addRoute(p.address(), p.prefix())
        }

        // DNS — libbox builds a synthetic in-tunnel DNS server it
        // intercepts itself. Throws if the IPv4 prefix is too narrow.
        try {
            val dns = options.getDNSServerAddress()
            if (dns != null && dns.value.isNotEmpty()) {
                builder.addDnsServer(dns.value)
            }
        } catch (t: Throwable) {
            Log.w(TAG, "no DNS hijack address from libbox; falling back to 8.8.8.8", t)
            AppLog.warning(R.string.log_dns_fallback, source = EngineLog.ENGINE)
            builder.addDnsServer("8.8.8.8")
        }

        applyAppRouting(builder)

        builder.setBlocking(false)

        // Service owns the PFD so it can close it on Disconnect/destroy.
        // During a reload, libbox asks for a new TUN. To perform a seamless handover
        // without dropping the VPN network (which causes connections to fail), we MUST
        // call builder.establish() BEFORE closing the old interface.
        val oldPfd = service.tunPfd
        service.tunPfd = null

        val pfd: ParcelFileDescriptor = builder.establish()
            ?: error("VpnService.Builder.establish() returned null — VPN permission revoked?")
        
        val newFd = pfd.fd
        
        if (oldPfd != null) {
            if (oldPfd.fd == newFd) {
                // FD was recycled! libbox already closed the raw FD in native code,
                // freeing the integer, which Binder immediately reused for the new TUN.
                // We MUST detach the old PFD to prevent its finalizer from closing the new TUN.
                oldPfd.detachFd()
            } else {
                // Safe to close the old interface now that the new one is established.
                try { oldPfd.close() } catch (_: Throwable) {}
            }
        }

        service.tunPfd = pfd
        Log.i(TAG, "openTun → fd=$newFd, mtu=$mtu, autoRoute=${options.autoRoute}")
        AppLog.info(R.string.log_tun_up, if (mtu > 0) mtu else 9000, source = EngineLog.ENGINE)
        return newFd
    }

    /**
     * Apply the "out of VPN" app list to the VpnService.Builder.
     *
     * Only the disallow half of the OS API is used now. The allow half is
     * mutually exclusive with it and modelled nothing we still offer: "into
     * VPN" and "block" are sing-box package_name rules instead, which is what
     * lets all three lists coexist.
     *
     * Blocked apps are deliberately NOT disallowed here — they must ENTER the
     * tunnel for the reject rule to have something to cut. Same for into-VPN
     * apps, obviously.
     *
     * Our own package is always disallowed: sing-box's outbound dial to the
     * proxy server must not recurse into our own tunnel.
     */
    private fun applyAppRouting(builder: VpnService.Builder) {
        val ownPkg = service.packageName
        tryDisallow(builder, ownPkg)
        val mode = RoutingRulesRepository.state.value.mode
        for (pkg in AppRoutingRepository.disallowedPackages(mode)) {
            if (pkg == ownPkg) continue
            tryDisallow(builder, pkg)
        }
    }

    /**
     * Points Android's system HTTP proxy at our local MITM (Task 3/4) so
     * Chrome/WebView/system-proxy-aware apps route through it. Apps with
     * their own TLS stack (YouTube, most native apps) ignore this entirely
     * and keep going straight through the TUN as before — this call has no
     * effect on them, by design.
     *
     * Deliberately checks `BoxModule.filterProxyRunning`, NOT the settings
     * toggle directly: openTun() runs synchronously inside BoxModule.start(),
     * so if we applied setHttpProxy whenever the toggle is on (regardless of
     * whether the MITM proxy actually came up), a failed/slow MITM start
     * would leave Chrome pointed at a port nothing is listening on — every
     * HTTPS request in the browser would fail, which is strictly worse than
     * not having this feature at all. ResultVpnService (Task 6) sets this
     * flag ONLY after Mobile.startFilterProxy() returns successfully, and
     * always BEFORE calling BoxModule.start() (which triggers this openTun).
     */
    private fun applyBrowserAdBlockProxy(builder: VpnService.Builder) {
        if (android.os.Build.VERSION.SDK_INT < android.os.Build.VERSION_CODES.Q) return
        if (!BoxModule.filterProxyRunning) return
        builder.setHttpProxy(
            android.net.ProxyInfo.buildDirectProxy("127.0.0.1", BROWSER_ADBLOCK_PORT)
        )
    }

    private fun tryDisallow(builder: VpnService.Builder, pkg: String) {
        try {
            builder.addDisallowedApplication(pkg)
        } catch (t: Throwable) {
            Log.w(TAG, "addDisallowedApplication($pkg) failed", t)
            AppLog.warning(R.string.log_app_route_failed, pkg)
        }
    }

    override fun usePlatformAutoDetectInterfaceControl(): Boolean = true

    override fun autoDetectInterfaceControl(fd: Int) {
        // VpnService.protect: keep this socket OUT of the tunnel.
        // sing-box uses this for its outbound connection to the proxy
        // server — without it we'd recurse into the tunnel.
        val ok = service.protect(fd)
        if (!ok) {
            Log.w(TAG, "protect($fd) returned false")
            if (!BoxModule.protectWarned) {
                BoxModule.protectWarned = true
                AppLog.warning(R.string.log_protect_failed, source = EngineLog.ENGINE)
            }
        } else {
            Log.d(TAG, "protect($fd) ok")
        }
    }

    override fun useProcFS(): Boolean = false
    override fun localDNSTransport(): LocalDNSTransport? = null

    /**
     * Resolves the app behind a connection so sing-box `package_name` rules can
     * match. ConnectivityManager is the only route available: SELinux blocks
     * /proc/net/tcp for untrusted_app, which is why useProcFS() stays false.
     *
     * Throwing (rather than returning null) is required: returning null crashes
     * the libbox wrapper — service.go:218 dereferences result.UserId without a
     * nil check. Throwing takes the (nil, err) path and the router treats the
     * owner as "not found" cleanly.
     *
     * Failure is fail-OPEN by construction: an unresolved owner matches no
     * package rule, so the traffic follows the general path rather than being
     * blocked. Two known holes, both accepted in the spec:
     * - API < 29: getConnectionOwnerUid doesn't exist (minSdk is 26). The UI
     *   disables the affected tabs there.
     * - UDP: only connected sockets are resolvable per the platform contract,
     *   so QUIC on an unconnected socket won't match.
     */
    override fun findConnectionOwner(
        ipProtocol: Int,
        sourceAddress: String?,
        sourcePort: Int,
        destinationAddress: String?,
        destinationPort: Int,
    ): ConnectionOwner {
        if (android.os.Build.VERSION.SDK_INT < android.os.Build.VERSION_CODES.Q) {
            throw UnsupportedOperationException("connection owner lookup needs API 29")
        }
        val cm = service.getSystemService(ConnectivityManager::class.java)
            ?: throw UnsupportedOperationException("no ConnectivityManager")
        val uid = cm.getConnectionOwnerUid(
            ipProtocol,
            InetSocketAddress(InetAddress.getByName(sourceAddress), sourcePort),
            InetSocketAddress(InetAddress.getByName(destinationAddress), destinationPort),
        )
        if (uid == Process.INVALID_UID) {
            throw UnsupportedOperationException("connection owner not found")
        }
        val packages = packagesForUid(uid)
            ?: throw UnsupportedOperationException("no packages for uid $uid")
        val owner = ConnectionOwner()
        owner.userId = uid
        owner.setAndroidPackageNames(PackageNameIterator(packages))
        return owner
    }

    /**
     * uid → package names, cached for the tunnel's lifetime. getConnectionOwnerUid
     * has to run per connection, but getPackagesForUid is the expensive half and
     * its answer only changes on install/uninstall — which tears the mapping down
     * with the next reconnect anyway.
     */
    private val packageCache = java.util.concurrent.ConcurrentHashMap<Int, Array<String>>()

    private fun packagesForUid(uid: Int): Array<String>? {
        packageCache[uid]?.let { return it }
        // Deliberately NOT MutableMap.getOrPut: that resolves to the non-atomic
        // get-then-put extension, which on this per-connection hot path lets
        // concurrent resolvers of the same new uid each pay for the expensive
        // getPackagesForUid — the exact cost the cache exists to avoid.
        // A null (uid gone, or not visible to us) is not cached: it may be
        // transient, and a poisoned entry would persist for the tunnel's life.
        val packages = service.packageManager.getPackagesForUid(uid) ?: return null
        return packageCache.putIfAbsent(uid, packages) ?: packages
    }

    override fun startDefaultInterfaceMonitor(listener: InterfaceUpdateListener?) {}
    override fun closeDefaultInterfaceMonitor(listener: InterfaceUpdateListener?) {}
    override fun getInterfaces(): NetworkInterfaceIterator = EmptyIterator
    override fun underNetworkExtension(): Boolean = false
    override fun includeAllNetworks(): Boolean = false
    override fun readWIFIState(): WIFIState? = null
    override fun systemCertificates(): StringIterator = EmptyStringIterator
    override fun clearDNSCache() {}
    override fun sendNotification(notification: Notification?) {}
}

private object EmptyIterator : NetworkInterfaceIterator {
    override fun hasNext(): Boolean = false
    override fun next() = throw NoSuchElementException()
}

private object EmptyStringIterator : StringIterator {
    override fun hasNext(): Boolean = false
    override fun next(): String = throw NoSuchElementException()
    override fun len(): Int = 0
}

private class PackageNameIterator(private val names: Array<String>) : StringIterator {
    private var i = 0
    override fun hasNext(): Boolean = i < names.size
    override fun next(): String = names[i++]
    override fun len(): Int = names.size
}

private class StubCommandHandler : CommandServerHandler {
    override fun serviceStop() { Log.i(TAG, "cmd: serviceStop") }
    override fun serviceReload() { Log.i(TAG, "cmd: serviceReload") }
    override fun getSystemProxyStatus(): SystemProxyStatus? = null
    override fun setSystemProxyEnabled(enabled: Boolean) {}
    override fun writeDebugMessage(message: String?) {
        Log.d(TAG, "libbox: $message")
        // Surface engine activity (connections, warnings, errors) in the
        // in-app log. EngineLog filters the raw stream down to the same class
        // of lines the desktop shows and dedups connections per host.
        EngineLog.ingest(message)
    }
}
