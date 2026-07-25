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
}
