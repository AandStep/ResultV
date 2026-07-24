package com.resultv.android.vpn

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class CertStoreTest {

    @Test fun matchesOurCommonName() {
        assertTrue(CertStore.isResultVCommonName("CN=ResultV AdBlock Root CA,O=ResultV"))
        assertTrue(CertStore.isResultVCommonName("CN=ResultV AdBlock Root CA"))
    }

    @Test fun ignoresOtherAuthorities() {
        assertFalse(CertStore.isResultVCommonName("CN=DigiCert Global Root CA"))
        assertFalse(CertStore.isResultVCommonName("CN=Some ResultV Lookalike"))
        assertFalse(CertStore.isResultVCommonName(null))
        assertFalse(CertStore.isResultVCommonName(""))
    }
}
