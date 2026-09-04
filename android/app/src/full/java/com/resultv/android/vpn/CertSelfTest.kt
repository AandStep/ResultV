package com.resultv.android.vpn

import android.util.Log
import java.io.IOException
import java.net.InetSocketAddress
import java.net.Proxy
import java.net.SocketTimeoutException
import java.net.URL
import java.security.cert.CertificateException
import javax.net.ssl.HttpsURLConnection
import javax.net.ssl.SSLHandshakeException

private const val TAG = "ResultV/CertSelfTest"
private const val PROBE_URL = "https://example.com/"
private const val TIMEOUT_MS = 5000

/**
 * Blocking TLS self-test — must be called from a background/worker thread,
 * never the main thread.
 *
 * Before enabling setHttpProxy() on the VpnService.Builder, we need to know
 * whether the user actually trusted our MITM root CA (KeyChain's
 * install-cert dialog reports no success/failure back to the app). We can't
 * read the system user-CA trust store directly, so instead we behave like
 * Chrome for one probe request: connect to a host through the freshly
 * started local MITM proxy and see if the leaf it presents validates.
 *
 * `example.com` is deliberately NOT in the Go defaultMITMExceptions() list
 * (internal/filter/mitm/server.go), so the proxy DOES intercept it and
 * present a leaf signed by our root CA — that's what makes this a real
 * trust test, not a pass-through. The default SSLSocketFactory is used so
 * that res/xml/network_security_config.xml's scoped domain-config (which
 * trusts user-installed CAs only for this host) governs the handshake.
 */
object CertSelfTest {

    enum class Result {
        PASS,
        CERT_UNTRUSTED,
        INCONCLUSIVE,
    }

    fun run(port: Int): Result {
        var connection: HttpsURLConnection? = null
        return try {
            val proxy = Proxy(Proxy.Type.HTTP, InetSocketAddress("127.0.0.1", port))
            val url = URL(PROBE_URL)
            connection = (url.openConnection(proxy) as HttpsURLConnection).apply {
                connectTimeout = TIMEOUT_MS
                readTimeout = TIMEOUT_MS
                requestMethod = "GET"
            }
            // Any response code means the TLS handshake succeeded — our
            // leaf validated, so the CA is trusted.
            connection.responseCode
            Result.PASS
        } catch (t: Throwable) {
            if (isCertTrustFailure(t)) {
                Log.w(TAG, "CA self-test: certificate untrusted", t)
                Result.CERT_UNTRUSTED
            } else if (t is IOException || t is SocketTimeoutException) {
                Log.w(TAG, "CA self-test: inconclusive (network error)", t)
                Result.INCONCLUSIVE
            } else {
                Log.w(TAG, "CA self-test: inconclusive (unexpected error)", t)
                Result.INCONCLUSIVE
            }
        } finally {
            connection?.disconnect()
        }
    }

    /** Unwraps causes so SSLHandshakeException wrapping a CertPathValidatorException is caught too. */
    private fun isCertTrustFailure(t: Throwable): Boolean {
        var cause: Throwable? = t
        while (cause != null) {
            if (cause is SSLHandshakeException || cause is CertificateException) return true
            cause = cause.cause
        }
        return false
    }
}
