package com.resultv.android.vpn

import android.content.Context
import android.content.Intent
import android.os.Build
import android.security.KeyChain
import mobile.Mobile
import java.io.File
import java.security.cert.CertificateFactory

/**
 * The one-tap install path — available only below Android 11.
 *
 * Up to and including Android 10, `KeyChain.createInstallIntent()` carrying an
 * EXTRA_CERTIFICATE really does install it: the system asks the user to name
 * the certificate and confirm, and that's the whole flow. Android 11 closed
 * this off — the intent no longer installs CA certificates, and there is no
 * deep link to the Settings screen that does, which is why [CertExporter] plus
 * the wizard's manual instructions exist for API 30+.
 *
 * See https://httptoolkit.com/blog/android-11-trust-ca-certificates/
 *
 * Browser ad-block requires API 29 (VpnService.Builder.setHttpProxy), so in
 * practice "below 11" here means exactly Android 10.
 */
object CertInstaller {

    /** True when [buildInstallIntent] can actually install rather than just inform. */
    fun canInstallDirectly(): Boolean = Build.VERSION.SDK_INT < Build.VERSION_CODES.R

    /**
     * Builds the system install intent for our root CA.
     *
     * KeyChain.EXTRA_CERTIFICATE wants DER, while the Go side writes PEM —
     * CertificateFactory reads the PEM and re-encodes.
     *
     * Throws if the CA hasn't been generated yet or can't be parsed; callers
     * should surface an error rather than crash.
     */
    fun buildInstallIntent(context: Context, dataDir: String): Intent {
        val der = File(Mobile.filterCARootPath(dataDir)).inputStream().use { stream ->
            CertificateFactory.getInstance("X.509").generateCertificate(stream).encoded
        }
        return KeyChain.createInstallIntent().apply {
            putExtra(KeyChain.EXTRA_CERTIFICATE, der)
            putExtra(KeyChain.EXTRA_NAME, "ResultV AdBlock Root CA")
        }
    }
}
