package com.resultv.android.vpn

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.os.Environment
import android.provider.DocumentsContract
import android.provider.MediaStore
import android.provider.OpenableColumns
import mobile.Mobile
import java.io.File

/** File name the install instructions tell the user to look for. */
const val CERT_FILE_NAME = "resultv-ca.crt"

/**
 * Hands the root CA to the system document picker so the user can pick it up
 * from Android's "Install a certificate" screen afterwards.
 *
 * Writing to Downloads silently via MediaStore was tried first and rejected on
 * device: nothing visible happens, so the user is left wondering whether the
 * step worked at all. The picker costs one tap and answers that — it opens
 * pre-filled with [CERT_FILE_NAME] and Downloads already selected, which is
 * also what AdGuard does.
 */
object CertExporter {

    /**
     * Intent for the system "save file" dialog, pre-filled so the user only
     * has to confirm.
     *
     * EXTRA_INITIAL_URI is a hint, honoured from Android 8 on and ignored
     * elsewhere; the picker still opens somewhere sensible without it.
     */
    fun buildSaveIntent(): Intent = Intent(Intent.ACTION_CREATE_DOCUMENT).apply {
        addCategory(Intent.CATEGORY_OPENABLE)
        type = "application/x-x509-ca-cert"
        putExtra(Intent.EXTRA_TITLE, CERT_FILE_NAME)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            putExtra(
                DocumentsContract.EXTRA_INITIAL_URI,
                downloadsTreeUri(),
            )
        }
    }

    /**
     * Copies the CA into the [destination] the picker returned, and reports
     * the name it actually landed under.
     *
     * The name is read back rather than assumed: when Downloads already holds
     * a `resultv-ca.crt`, the picker silently saves ours as `resultv-ca
     * (3).crt` and tells us only through the Uri. Instructions naming
     * [CERT_FILE_NAME] would then point at a different, older file — verified
     * on an Android 16 emulator.
     *
     * Blocking I/O — call from a background thread. Throws on failure so the
     * caller can surface an error rather than advance the wizard on a file
     * that isn't there.
     */
    fun writeTo(context: Context, destination: Uri, dataDir: String): String {
        context.contentResolver.openOutputStream(destination)?.use { out ->
            File(Mobile.filterCARootPath(dataDir)).inputStream().use { it.copyTo(out) }
        } ?: error("Couldn't open $destination for writing")
        return displayName(context, destination) ?: CERT_FILE_NAME
    }

    private fun displayName(context: Context, uri: Uri): String? = runCatching {
        context.contentResolver.query(
            uri,
            arrayOf(OpenableColumns.DISPLAY_NAME),
            null,
            null,
            null,
        )?.use { cursor ->
            if (cursor.moveToFirst()) {
                cursor.getString(cursor.getColumnIndexOrThrow(OpenableColumns.DISPLAY_NAME))
                    ?.takeIf { it.isNotBlank() }
            } else {
                null
            }
        }
    }.getOrNull()

    private fun downloadsTreeUri(): Uri =
        MediaStore.Files.getContentUri(MediaStore.VOLUME_EXTERNAL).buildUpon()
            .appendPath(Environment.DIRECTORY_DOWNLOADS)
            .build()
}
