package com.resultv.android.ui.components

import com.resultv.android.vpn.VpnStatus

/**
 * Вид главной под состояние подключения — перенос таблицы BY_STATUS из
 * `ResultVPC/frontend/src/views/redesign/MainPage.jsx`.
 *
 * Одно состояние задаёт разом четыре вещи: вариант шапки, вариант кнопки
 * питания, подсветку карточки сервера и режим плиток скорости. Поэтому это
 * одно перечисление, а не четыре набора `when` по `VpnStatus`, разъехавшихся
 * бы при первой же правке.
 */
enum class HomeLook { Idle, Processing, Success, Error }

fun homeLook(status: VpnStatus): HomeLook = when (status) {
    is VpnStatus.Idle -> HomeLook.Idle
    is VpnStatus.Connecting -> HomeLook.Processing
    is VpnStatus.Connected -> HomeLook.Success
    is VpnStatus.Error -> HomeLook.Error
}

/**
 * Ступени волны в порядке от кнопки наружу. Порядок объявления — и есть
 * порядок волны; добавлять ступени только на своё место.
 */
enum class WaveStep { Power, Title, Time, Card, Speed }

/** Шаг волны из макета. */
const val WAVE_STEP_MILLIS = 70

/**
 * Задержка ступени.
 *
 * Подключение меняет разом полстраницы: заливку кнопки, цвет заголовка,
 * плашку времени, подсветку карточки, кривые скорости. Пущенное одним
 * проблеском, это читается как мигание всего экрана. Волна разносит
 * изменение во времени: первой отзывается кнопка, последними плитки.
 *
 * При отключении порядок обратный — гаснет сначала то, что зажглось
 * последним. Задержку каждый раз берёт то состояние, в которое переходим.
 */
fun waveDelayMillis(step: WaveStep, connected: Boolean): Int {
    val order = if (connected) step.ordinal else WaveStep.entries.size - 1 - step.ordinal
    return order * WAVE_STEP_MILLIS
}
