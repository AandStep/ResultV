package com.resultv.android.vpn

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File

class RepositoryJsonFileTest {
    @get:Rule val folder = TemporaryFolder()

    @Test
    fun recoversLastReadableSnapshotAndKeepsItAfterAnotherWrite() {
        val file = File(folder.root, "profiles.json")
        val decode = { text: String -> JSONObject(text).getJSONArray("profiles").getString(0) }
        val old = """{"profiles":["old"]}"""
        val current = """{"profiles":["current"]}"""
        replaceRepositoryJson(file, old, decode)
        replaceRepositoryJson(file, current, decode)
        assertEquals("current", readRepositoryJson(file, decode) { throw it })

        file.writeText("""{"profiles":""")
        assertEquals("old", readRepositoryJson(file, decode) { throw it })

        replaceRepositoryJson(file, current, decode)
        file.writeText("{}")
        assertEquals("old", readRepositoryJson(file, decode) { throw it })
    }
}
