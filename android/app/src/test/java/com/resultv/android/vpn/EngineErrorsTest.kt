package com.resultv.android.vpn

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class EngineErrorsTest {

    @Test fun matchesTheFieldError() {
        // The exact message sing-box surfaces when the cached rule-set blob is
        // corrupt — the failure that bricks ad-block startup.
        assertTrue(
            EngineErrors.isCorruptRuleSetCacheError(
                "initialize rule-set[1]: restore cached rule-set: read rule[0] zlib invalid checksum"
            )
        )
    }

    @Test fun matchesRuleSetWithZlibChecksumWording() {
        assertTrue(EngineErrors.isCorruptRuleSetCacheError("rule-set foo: zlib: invalid checksum"))
    }

    @Test fun caseInsensitive() {
        assertTrue(
            EngineErrors.isCorruptRuleSetCacheError("RESTORE CACHED RULE-SET: bad")
        )
    }

    @Test fun ignoresUnrelatedErrors() {
        assertFalse(EngineErrors.isCorruptRuleSetCacheError("bind: address already in use"))
        assertFalse(EngineErrors.isCorruptRuleSetCacheError("invalid checksum in tls handshake"))
        assertFalse(EngineErrors.isCorruptRuleSetCacheError(null))
        assertFalse(EngineErrors.isCorruptRuleSetCacheError(""))
    }
}
