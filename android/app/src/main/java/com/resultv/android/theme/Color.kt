package com.resultv.android.theme

/**
 * Псевдонимы на новые токены — строительные леса на время перевода экранов.
 *
 * Существуют ровно для того, чтобы все 23 файла продолжали собираться, пока
 * экраны переводятся по одному, а не разом. Удаляются последней задачей плана,
 * и это удаление служит доказательством полноты перевода: если `Brand` уходит
 * и сборка проходит, значит на старые значения не осталось ни одной ссылки.
 *
 * НОВЫЙ КОД СЮДА НЕ ПИШЕТСЯ. Берите RvColor.
 */
@Deprecated("Строительные леса перевода на RvColor; удаляются в конце", ReplaceWith("RvColor"))
object Brand {
    val Green = RvColor.Main
    val GreenLight = RvColor.Second
    val GreenDark = RvColor.mainA50

    val Danger = RvColor.Errors
    val Warning = RvColor.Warning
    val Favorite = RvColor.Warning

    val Bg = RvColor.Black
    val Surface = RvColor.Grey
    val SurfaceHigh = RvColor.LightGray
    val SurfaceBorder = RvColor.whiteA10

    val MutedText = RvColor.whiteA50
    val SecondaryText = RvColor.whiteA50
}
