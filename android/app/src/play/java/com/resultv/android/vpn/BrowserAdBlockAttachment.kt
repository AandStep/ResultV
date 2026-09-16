package com.resultv.android.vpn

import android.content.Context
import java.util.concurrent.Executor
import kotlinx.coroutines.CoroutineScope

/**
 * Заглушка для дистрибутива Play — по образцу [CertSelfTest], [CertStore] и
 * [FilterProxyWatchdog].
 *
 * Браузерного ad-block в этой сборке нет ни в .so (тег `no_mitm`), ни в
 * Kotlin. Класс вынесен из `ResultVpnService` ради ресурсов: строки про
 * перехват TLS лежат в `src/full/res` и в play-APK не попадают, а общий код
 * не может ссылаться на то, чего в сборке нет.
 */
@Suppress("UNUSED_PARAMETER")
class BrowserAdBlockAttachment(
    context: Context,
    worker: Executor,
    scope: CoroutineScope,
    reloadConfig: () -> String?,
) {
    fun attach(attemptsDone: Int = 0) {}

    fun stopWatchdog() {}
}
