package com.resultv.android.vpn

import android.util.Log
import mobile.Mobile
import java.io.File
import java.security.KeyStore
import java.security.cert.CertificateFactory
import java.security.cert.X509Certificate

private const val TAG = "ResultV/CertStore"

/**
 * Reads the system trust store to answer "has the user installed our MITM root
 * CA yet?" — offline, with no VPN and no proxy involved.
 *
 * This complements [CertSelfTest] rather than replacing it. The self-test
 * proves interception actually works, but it can only run once the tunnel is
 * up. The install wizard needs an answer the instant the user tabs back from
 * Android's Settings app, which is what this provides.
 *
 * "AndroidCAStore" exposes both system CAs (`system:*` aliases) and
 * user-installed ones (`user:*`). Only the latter can be ours, so system
 * aliases are skipped — that also keeps the comparison loop short.
 */
object CertStore {

    /**
     * True when a certificate byte-identical to our generated root CA is
     * present among the user-installed CAs.
     *
     * Compares encoded form rather than subject DN: a DN match would also
     * accept some other certificate claiming our name, and we specifically
     * want to know that *this* key is trusted.
     *
     * Blocking KeyStore + filesystem I/O — call from a background thread.
     * Returns false on any error (missing CA file, unreadable store) since an
     * unanswerable question is not an install.
     */
    fun isInstalled(dataDir: String): Boolean = try {
        val ours = loadOurCa(dataDir)
        val store = KeyStore.getInstance("AndroidCAStore").apply { load(null, null) }
        store.aliases().asSequence()
            .filter { it.startsWith("user:") }
            .any { alias ->
                (store.getCertificate(alias) as? X509Certificate)?.encoded.contentEquals(ours)
            }
    } catch (t: Throwable) {
        Log.w(TAG, "Couldn't determine CA install state", t)
        false
    }

    private fun loadOurCa(dataDir: String): ByteArray =
        File(Mobile.filterCARootPath(dataDir)).inputStream().use { stream ->
            CertificateFactory.getInstance("X.509").generateCertificate(stream).encoded
        }
}
