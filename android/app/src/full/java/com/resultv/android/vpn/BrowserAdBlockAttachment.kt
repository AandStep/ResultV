package com.resultv.android.vpn

import android.content.Context
import android.os.Build
import android.util.Log
import com.resultv.android.R
import java.util.concurrent.Executor
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

private const val TAG = "ResultV/BrowserAdBlock"

/**
 * Браузерный ad-block (MITM): подъём локального прокси, самотест CA и
 * сторож живости.
 *
 * Вынесен из `ResultVpnService` в пару `src/full` / `src/play` ради ресурсов.
 * Строки, описывающие перехват TLS, лежат в `src/full/res` и в play-APK не
 * попадают — значит и упоминать их может только код, который туда не едет.
 * Go-слой в play уже без этой функции (тег `no_mitm`), так что заглушка
 * ничего не ломает.
 *
 * Сервис отдаёт сюда ровно то, что нужно, а не себя целиком: [reloadConfig]
 * пересобирает конфиг активного профиля (или возвращает null, когда профиля
 * нет), [worker] — та же однопоточная очередь, за которой стоят коннект и
 * reload-ы, [scope] — время жизни сервиса.
 */
class BrowserAdBlockAttachment(
    private val context: Context,
    private val worker: Executor,
    private val scope: CoroutineScope,
    private val reloadConfig: () -> String?,
) {

    @Volatile private var watchdog: FilterProxyWatchdog? = null

    /**
     * Attach the browser ad-block MITM proxy AFTER the tunnel is already up,
     * off the connect critical path. Starting it (urlfilter engine build + TLS
     * self-test) takes seconds; doing it before BoxModule.start() used to add
     * that to every connect. Here we start it on the worker (so it queues
     * behind the just-finished connect task) and, once the proxy is live, force
     * an in-place reload so openTun() re-runs and applies setHttpProxy — the
     * mirror image of onUnhealthy(), which reloads to REMOVE it.
     *
     * The tunnel stays up throughout; there's only a brief window right after
     * connect where the browser isn't yet filtered. The cached urlfilter engine
     * (Manager reuses it across connects) keeps that window short after the
     * first connect.
     *
     * Самотест CA, ответивший INCONCLUSIVE, повторяется: пауза отсчитывается
     * вне worker (иначе очередь reload-ов встала бы на всё это время), а сама
     * попытка снова встаёт в worker. См. [certSelfTestRetryDelayMs] о том,
     * почему «не знаю» — не повод выключать функцию до конца сессии.
     */
    fun attach(attemptsDone: Int = 0) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.Q) return
        if (!SettingsRepository.state.value.browserAdBlock) return
        worker.execute {
            val verdict = start()
            // Only reload if the proxy actually came up (trusted cert + healthy);
            // start() leaves filterProxyRunning=false otherwise.
            if (BoxModule.filterProxyRunning && BoxModule.isRunning) {
                val cfg = reloadConfig() ?: return@execute
                BoxModule.reload(cfg)
                return@execute
            }
            // CERT_UNTRUSTED — это ответ, а не помеха: повторять нечего, пока
            // пользователь не поставит сертификат. Повтора заслуживает только
            // INCONCLUSIVE.
            if (verdict != CertSelfTest.Result.INCONCLUSIVE) return@execute
            val attempts = attemptsDone + 1
            val retryIn = certSelfTestRetryDelayMs(attempts)
            if (retryIn == null) {
                AppLog.warning(context.getString(R.string.log_browser_adblock_selftest_inconclusive))
                return@execute
            }
            Log.i(TAG, "CA self-test inconclusive (attempt $attempts) - retrying in ${retryIn}ms")
            scope.launch {
                delay(retryIn)
                if (!BoxModule.isRunning) return@launch
                attach(attempts)
            }
        }
    }

    /**
     * Best-effort: browser ad-block is a bonus feature layered on top of
     * the VPN tunnel, never a reason to fail the whole connect. Runs AFTER
     * BoxModule.start() (see [attach]) and MUST leave
     * BoxModule.filterProxyRunning=false on any failure (no lists downloaded
     * yet, port in use, etc.) so the follow-up reload's openTun() never applies
     * setHttpProxy to a dead proxy and breaks Chrome's HTTPS traffic.
     *
     * Возвращает вердикт самотеста, чтобы вызывающая сторона отличила «не
     * знаю» (повторимо) от «не доверен» (нет), либо null, если до самотеста
     * не дошло.
     */
    private fun start(): CertSelfTest.Result? {
        BoxModule.filterProxyRunning = false
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.Q) return null
        if (!SettingsRepository.state.value.browserAdBlock) return null
        val dataDir = context.filesDir.absolutePath
        try {
            mobile.Mobile.startFilterProxy(dataDir, BROWSER_ADBLOCK_PORT.toLong())
            val verdict = CertSelfTest.run(BROWSER_ADBLOCK_PORT)
            when (verdict) {
                CertSelfTest.Result.PASS -> {
                    BoxModule.filterProxyRunning = true
                    SettingsRepository.setCertTrustState(CertTrustState.TRUSTED)
                    watchdog?.stop()
                    watchdog = FilterProxyWatchdog(dataDir) { onUnhealthy() }.also { it.start() }
                }
                CertSelfTest.Result.CERT_UNTRUSTED -> {
                    // Tear down; leave filterProxyRunning=false so setHttpProxy is NOT applied.
                    mobile.Mobile.stopFilterProxy()
                    SettingsRepository.setCertTrustState(CertTrustState.UNTRUSTED)
                    SettingsRepository.setBrowserAdBlock(false)
                    AppLog.warning(context.getString(R.string.log_browser_adblock_cert_untrusted))
                }
                CertSelfTest.Result.INCONCLUSIVE -> {
                    // Don't leave an unused proxy running; keep the toggle for next time.
                    // certTrustState is deliberately NOT touched here — a transient
                    // network hiccup must not overwrite the last known-good/known-bad
                    // determination (see CertTrustState's doc comment).
                    //
                    // В журнал пишет вызывающая сторона, и только когда повторы
                    // кончились: строка «проверить не удалось» на каждой
                    // попытке выглядела бы отказом там, где ещё идёт ожидание.
                    mobile.Mobile.stopFilterProxy()
                }
            }
            return verdict
        } catch (t: Throwable) {
            BoxModule.filterProxyRunning = false
            try { mobile.Mobile.stopFilterProxy() } catch (_: Throwable) {}
            Log.w(TAG, "browser ad-block proxy failed to start; Chrome will use normal routing", t)
            AppLog.warning(
                R.string.log_adblock_proxy_start_failed,
                t.message ?: t.javaClass.simpleName,
                source = AppLog.resolve(R.string.log_source_adblock),
            )
            return null
        }
    }

    fun stopWatchdog() {
        watchdog?.stop()
        watchdog = null
    }

    /**
     * Called from the watchdog's coroutine when the proxy dies mid-session.
     * Turns the feature off (so it doesn't silently keep failing on every
     * future reconnect) and forces a lightweight in-place reload — the same
     * technique reloadKillSwitch uses — so openTun() re-runs, re-reads
     * BoxModule.filterProxyRunning (now false), and drops setHttpProxy from
     * the live Builder. The sing-box config itself doesn't change; this is
     * purely to force a fresh TUN handover.
     */
    private fun onUnhealthy() {
        BoxModule.filterProxyRunning = false
        SettingsRepository.setBrowserAdBlock(false)
        AppLog.warning(context.getString(R.string.log_browser_adblock_disabled))
        val cfg = reloadConfig() ?: return
        worker.execute { BoxModule.reload(cfg) }
    }
}
