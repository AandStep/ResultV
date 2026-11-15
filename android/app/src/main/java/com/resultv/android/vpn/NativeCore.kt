package com.resultv.android.vpn

import android.util.Log
import mobile.Mobile

internal object NativeCore {
    val available: Boolean by lazy {
        try {
            Mobile.isSubscriptionURL("", false)
            true
        } catch (error: LinkageError) {
            Log.e("ResultV/NativeCore", "VPN core could not load for this ABI", error)
            false
        }
    }
}
