package com.resultv.android.vpn

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SmartListSnapshotTest {

    @Test fun builtinDoesNotOverwriteRealList() {
        // A transient antizapret failure yields the small builtin list; it must
        // not replace an already-loaded remote/cache list (Problem #2: collapses
        // the Smart allowlist and kills traffic).
        assertFalse(shouldReplaceSmartSnapshot(curSource = "remote", curEmpty = false, nextSource = "builtin"))
        assertFalse(shouldReplaceSmartSnapshot(curSource = "cache", curEmpty = false, nextSource = "builtin"))
    }

    @Test fun builtinAcceptedWhenNoRealListYet() {
        // Cold start: some blocking beats none.
        assertTrue(shouldReplaceSmartSnapshot(curSource = "", curEmpty = true, nextSource = "builtin"))
        assertTrue(shouldReplaceSmartSnapshot(curSource = "builtin", curEmpty = false, nextSource = "builtin"))
    }

    @Test fun realListAlwaysReplaces() {
        assertTrue(shouldReplaceSmartSnapshot(curSource = "cache", curEmpty = false, nextSource = "remote"))
        assertTrue(shouldReplaceSmartSnapshot(curSource = "remote", curEmpty = false, nextSource = "cache"))
        assertTrue(shouldReplaceSmartSnapshot(curSource = "builtin", curEmpty = false, nextSource = "remote"))
    }

    @Test
    fun `snapshot is empty only when it has no entries`() {
        assertTrue(SmartListRepository.Snapshot().isEmpty)
        assertFalse(SmartListRepository.Snapshot(count = 42, ready = true).isEmpty)
    }

    @Test
    fun `a ready seed with no fetch is not empty`() {
        // The bundled seed installs an SRS without any successful fetch, so
        // readiness must come from the SRS on disk, not from the fetch count.
        val seeded = SmartListRepository.Snapshot(count = 0, ready = true)
        assertFalse(seeded.isEmpty)
    }

    @Test
    fun `staleness is driven by fetchedAt`() {
        assertTrue(SmartListRepository.Snapshot(count = 1, ready = true, fetchedAt = 0L).isStale)
        assertFalse(
            SmartListRepository.Snapshot(
                count = 1, ready = true, fetchedAt = System.currentTimeMillis(),
            ).isStale
        )
    }
}
