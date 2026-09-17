package com.resultv.android.vpn

import android.util.Log
import com.resultv.android.R
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import libbox.CommandServer
import mobile.Mobile

/**
 * Пишет в журнал счётчики WireGuard-устройства и его gVisor-стека, пока жив
 * туннель и включён подробный журнал.
 *
 * Зачем: ядро логирует рукопожатия и keepalive, а данные — нет. Сессия,
 * переставшая возить трафик при живом с виду устройстве, выглядит в журнале
 * полной тишиной, и отличить «пакеты не дошли до устройства» от «дошли, но
 * соединения не встают» больше нечем.
 *
 * Только при подробном журнале: строка длинная и идёт раз в пять секунд — в
 * обычной сессии она вытеснила бы из журнала всё остальное.
 */
internal object WgDiagSampler {

    private const val TAG = "WgDiagSampler"
    private const val INTERVAL_MS = 5_000L

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private var job: Job? = null

    @Synchronized
    fun start(server: CommandServer) {
        stop()
        if (SettingsRepository.state.value.logLevel != "debug") return
        job = scope.launch {
            while (isActive) {
                delay(INTERVAL_MS)
                val line = try {
                    Mobile.wgDiagLine(server)
                } catch (t: Throwable) {
                    // Сообщается один раз и на этом всё: у не-WireGuard узла
                    // считать нечего, а повтор каждые пять секунд похоронил бы
                    // тот самый журнал, ради которого это заведено.
                    Log.i(TAG, "counters unavailable: ${t.message}")
                    AppLog.info(
                        R.string.log_wg_diag_unavailable,
                        t.message ?: t.javaClass.simpleName,
                        source = AppLog.resolve(R.string.log_source_wg),
                    )
                    return@launch
                }
                AppLog.info(line, source = AppLog.resolve(R.string.log_source_wg))
            }
        }
    }

    @Synchronized
    fun stop() {
        job?.cancel()
        job = null
    }
}
