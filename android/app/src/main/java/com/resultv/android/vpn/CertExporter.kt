package com.resultv.android.vpn

import android.content.ContentValues
import android.content.Context
import android.net.Uri
import android.os.Build
import android.os.Environment
import android.provider.MediaStore
import androidx.annotation.RequiresApi
import mobile.Mobile
import java.io.File

/** File name the install instructions tell the user to look for. */
const val CERT_FILE_NAME = "resultv-ca.crt"

/**
 * Drops our root CA into the public Downloads folder so the user can pick it
 * up from Android's "Install a certificate" screen.
 *
 * Q+ only, which is the same floor as browser ad-block itself
 * (VpnService.Builder.setHttpProxy) — so MediaStore's scoped-storage API is
 * always available and no storage permission is needed.
 *
 * Deliberately no picker and no share sheet: the previous flow made the user
 * choose a target and then hunt for what they'd saved, which is exactly the
 * friction the wizard exists to remove.
 */
object CertExporter {

    /**
     * Writes [CERT_FILE_NAME] to Downloads, replacing an earlier copy we wrote.
     *
     * Without the delete, MediaStore silently renames the new file to
     * `resultv-ca (1).crt` and the on-screen instructions stop matching what
     * the user sees in the file list. The query is scoped to
     * `MediaStore.Downloads` by exact display name, so it can only match a
     * download of ours.
     *
     * Throws on I/O failure — callers should surface an error rather than
     * silently advance the wizard.
     */
    @RequiresApi(Build.VERSION_CODES.Q)
    fun saveToDownloads(context: Context, dataDir: String): Uri {
        val resolver = context.contentResolver
        val collection = MediaStore.Downloads.EXTERNAL_CONTENT_URI

        resolver.query(
            collection,
            arrayOf(MediaStore.Downloads._ID),
            "${MediaStore.Downloads.DISPLAY_NAME} = ?",
            arrayOf(CERT_FILE_NAME),
            null,
        )?.use { cursor ->
            val idColumn = cursor.getColumnIndexOrThrow(MediaStore.Downloads._ID)
            while (cursor.moveToNext()) {
                runCatching {
                    resolver.delete(
                        MediaStore.Downloads.EXTERNAL_CONTENT_URI.buildUpon()
                            .appendPath(cursor.getLong(idColumn).toString())
                            .build(),
                        null,
                        null,
                    )
                }
            }
        }

        val values = ContentValues().apply {
            put(MediaStore.Downloads.DISPLAY_NAME, CERT_FILE_NAME)
            put(MediaStore.Downloads.MIME_TYPE, "application/x-x509-ca-cert")
            put(MediaStore.Downloads.RELATIVE_PATH, Environment.DIRECTORY_DOWNLOADS)
            put(MediaStore.Downloads.IS_PENDING, 1)
        }
        val uri = resolver.insert(collection, values)
            ?: error("MediaStore refused to create $CERT_FILE_NAME in Downloads")

        try {
            resolver.openOutputStream(uri)?.use { out ->
                File(Mobile.filterCARootPath(dataDir)).inputStream().use { it.copyTo(out) }
            } ?: error("Couldn't open an output stream for $CERT_FILE_NAME")
        } catch (t: Throwable) {
            // Leaving the row IS_PENDING would strand an invisible placeholder
            // in the user's Downloads that later saves would then collide with.
            runCatching { resolver.delete(uri, null, null) }
            throw t
        }

        resolver.update(
            uri,
            ContentValues().apply { put(MediaStore.Downloads.IS_PENDING, 0) },
            null,
            null,
        )
        return uri
    }
}
