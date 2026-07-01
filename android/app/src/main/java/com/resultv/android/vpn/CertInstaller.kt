package com.resultv.android.vpn

import android.content.Context
import android.security.KeyChain
import mobile.Mobile
import java.io.File
import java.security.cert.CertificateFactory

/**
 * Wraps the Android system dialog for trusting our locally generated MITM
 * root CA. There is no silent/programmatic install path outside of a
 * device-owner (MDM) context — this always shows Android's own warning
 * dialog, which is the point: the user must knowingly consent.
 */
object CertInstaller {
    /**
     * Builds the system "install certificate" intent. Reads the CA as PEM
     * (java.security.cert.CertificateFactory accepts PEM directly) and
     * passes its DER encoding, since KeyChain.EXTRA_CERTIFICATE requires
     * DER bytes.
     *
     * Throws if the Go side hasn't generated the CA yet or the file can't
     * be parsed — callers should catch and show an error rather than crash,
     * since this touches the filesystem and a third-party crypto library.
     */
    fun buildInstallIntent(context: Context, dataDir: String): android.content.Intent {
        val certPath = Mobile.filterCARootPath(dataDir)
        val der = CertificateFactory.getInstance("X.509")
            .generateCertificate(File(certPath).inputStream())
            .encoded
        return KeyChain.createInstallIntent().apply {
            putExtra(KeyChain.EXTRA_CERTIFICATE, der)
            putExtra(KeyChain.EXTRA_NAME, "ResultV AdBlock Root CA")
        }
    }
}
