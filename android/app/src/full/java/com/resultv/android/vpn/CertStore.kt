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

    /** Subject CommonName of our root CA — must match the Go side's caCN. */
    private const val CA_COMMON_NAME = "ResultV AdBlock Root CA"

    /**
     * Records the stable device seed so the Go side regenerates the exact same
     * root CA after a reinstall. Safe to call repeatedly; forwards a blank seed
     * unchanged (Go then keeps random generation).
     */
    fun applySeed(dataDir: String, seed: String) {
        runCatching { Mobile.setFilterCASeed(dataDir, seed) }
            .onFailure { Log.w(TAG, "Couldn't set CA seed", it) }
    }

    /**
     * How many user-installed CAs carry our CommonName but are NOT byte-equal
     * to the CA we currently use. These are stale leftovers from earlier random
     * generations, piling up in the system trust store across reinstalls.
     *
     * Blocking KeyStore + filesystem I/O — call from a background thread.
     * Returns 0 on any error, matching [isInstalled]'s fail-safe stance.
     */
    fun staleEntryCount(dataDir: String): Int = try {
        val ours = loadOurCa(dataDir)
        val store = KeyStore.getInstance("AndroidCAStore").apply { load(null, null) }
        store.aliases().asSequence()
            .filter { it.startsWith("user:") }
            .count { alias ->
                val cert = store.getCertificate(alias) as? X509Certificate
                cert != null &&
                    isResultVCommonName(cert.subjectX500Principal.name) &&
                    !cert.encoded.contentEquals(ours)
            }
    } catch (t: Throwable) {
        Log.w(TAG, "Couldn't count stale CA entries", t)
        0
    }

    /**
     * True when an X.500 subject DN names our root CA. Pure and testable — the
     * trust-store iteration around it is Android-only and untested, like
     * [isInstalled].
     */
    fun isResultVCommonName(subjectDn: String?): Boolean =
        subjectDn?.split(",")?.any { it.trim() == "CN=$CA_COMMON_NAME" } ?: false

    private fun loadOurCa(dataDir: String): ByteArray =
        File(Mobile.filterCARootPath(dataDir)).inputStream().use { stream ->
            CertificateFactory.getInstance("X.509").generateCertificate(stream).encoded
        }
}
