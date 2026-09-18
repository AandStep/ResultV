package com.resultv.android.ui.components

import com.resultv.android.vpn.VpnStatus
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Вид главной под состояние и задержки волны — перенос BY_STATUS и блока
 * `--rv-wave` из `ResultVPC/frontend/src/views/redesign/MainPage.{jsx,css}`.
 */
class HomeStatusTest {

    @Test fun eachStatusMapsToItsLook() {
        assertEquals(HomeLook.Idle, homeLook(VpnStatus.Idle))
        assertEquals(HomeLook.Processing, homeLook(VpnStatus.Connecting))
        assertEquals(HomeLook.Success, homeLook(VpnStatus.Connected(0L)))
        assertEquals(HomeLook.Error, homeLook(VpnStatus.Error("boom")))
    }

    // Подключение: волна идёт от кнопки наружу.
    @Test fun connectingWaveRunsOutwardFromTheButton() {
        assertEquals(0, waveDelayMillis(WaveStep.Power, connected = true))
        assertEquals(70, waveDelayMillis(WaveStep.Title, connected = true))
        assertEquals(140, waveDelayMillis(WaveStep.Time, connected = true))
        assertEquals(210, waveDelayMillis(WaveStep.Card, connected = true))
        assertEquals(280, waveDelayMillis(WaveStep.Speed, connected = true))
    }

    // Отключение: гаснет сначала то, что зажглось последним.
    @Test fun disconnectingWaveRunsBackward() {
        assertEquals(0, waveDelayMillis(WaveStep.Speed, connected = false))
        assertEquals(70, waveDelayMillis(WaveStep.Card, connected = false))
        assertEquals(140, waveDelayMillis(WaveStep.Time, connected = false))
        assertEquals(210, waveDelayMillis(WaveStep.Title, connected = false))
        assertEquals(280, waveDelayMillis(WaveStep.Power, connected = false))
    }

    // Вся волна укладывается в 280 мс — иначе она читалась бы как задержка.
    @Test fun waveFitsInTwoHundredEighty() {
        val longest = WaveStep.entries.maxOf { waveDelayMillis(it, connected = true) }
        assertEquals(280, longest)
    }
}
