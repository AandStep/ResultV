package com.resultv.android.vpn

import java.io.File
import java.nio.file.Files
import java.nio.file.StandardCopyOption.ATOMIC_MOVE
import java.nio.file.StandardCopyOption.REPLACE_EXISTING

internal fun <T> readRepositoryJson(
    file: File,
    decode: (String) -> T,
    onRecovered: (Throwable) -> Unit = {},
    onUnreadable: (Throwable) -> Unit,
): T? {
    val backup = repositoryBackup(file)
    if (!file.exists() && !backup.exists()) return null

    val primary = runCatching { decode(file.readText()) }
    return primary.getOrElse { error ->
        runCatching { decode(backup.readText()) }
            .onSuccess { onRecovered(error) }
            .getOrElse {
                onUnreadable(error)
                null
            }
    }
}

internal fun replaceRepositoryJson(file: File, data: String, decode: (String) -> Any) {
    val previous = runCatching { file.readText().also { decode(it) } }.getOrNull()
    if (previous != null) writeAtomically(repositoryBackup(file), previous)
    writeAtomically(file, data)
}

private fun repositoryBackup(file: File) = File(file.parentFile, "${file.name}.bak")

private fun writeAtomically(file: File, data: String) {
    val temporary = File.createTempFile("rv-", ".tmp", file.parentFile)
    try {
        temporary.outputStream().use { output ->
            output.write(data.toByteArray(Charsets.UTF_8))
            output.fd.sync()
        }
        Files.move(temporary.toPath(), file.toPath(), ATOMIC_MOVE, REPLACE_EXISTING)
    } finally {
        temporary.delete()
    }
}
