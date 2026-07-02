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

    /**
     * Builds a share intent for the CA certificate file itself, via
     * FileProvider + ACTION_SEND. This is the actually-functional path for
     * getting the certificate onto the device where the user can pick it:
     * KeyChain.createInstallIntent() (above) only shows an informational
     * "install this in Settings" dialog on Android 7+, it never installs
     * directly — the user needs a real file to select in that Settings flow,
     * which this share sheet provides (save to Files, email, cloud storage,
     * etc., then pick it back up from Settings -> Encryption & credentials).
     *
     * Copies ca.crt into a dedicated cacheDir/share_cert/ subdirectory before
     * building the FileProvider URI, rather than exposing files/filter/
     * directly — that directory also holds ca.key (the CA private key) and
     * cached filter lists, which must never become reachable through this
     * mechanism. (Pointing <files-path> at the exact ca.crt filename instead
     * of a directory was tried and rejected: it crashes AndroidX
     * FileProvider's SimplePathStrategy.getUriForFile with a
     * StringIndexOutOfBoundsException — confirmed via live device test.
     * <files-path>/<cache-path> entries must name a directory.)
     *
     * Throws under the same conditions as [buildInstallIntent] — callers
     * should catch and show an error rather than crash.
     */
    fun buildShareIntent(context: Context, dataDir: String): android.content.Intent {
        val certPath = Mobile.filterCARootPath(dataDir)
        val shareDir = File(context.cacheDir, "share_cert").apply { mkdirs() }
        val shareFile = File(shareDir, "resultv-adblock-ca.crt")
        File(certPath).copyTo(shareFile, overwrite = true)
        val uri = androidx.core.content.FileProvider.getUriForFile(
            context,
            "${context.packageName}.fileprovider",
            shareFile,
        )
        val sendIntent = android.content.Intent(android.content.Intent.ACTION_SEND).apply {
            type = "application/x-x509-ca-cert"
            putExtra(android.content.Intent.EXTRA_STREAM, uri)
            addFlags(android.content.Intent.FLAG_GRANT_READ_URI_PERMISSION)
        }
        return android.content.Intent.createChooser(sendIntent, "ResultV AdBlock Root CA")
    }
}
