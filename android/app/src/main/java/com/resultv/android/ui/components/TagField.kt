package com.resultv.android.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.resultv.android.R
import com.resultv.android.theme.RvColor
import com.resultv.android.theme.SegoeUi
import com.resultv.android.theme.RvRadius
import com.resultv.android.theme.RvSpace

/**
 * What one edit of a tag field's text box amounts to: the values it completed
 * and the draft left behind.
 */
internal data class TagDraftEdit(val draft: String, val commits: List<String>)

/**
 * Interpret freshly typed (or pasted) text as tag input.
 *
 * Whitespace is the separator, so a space commits what precedes it and a
 * pasted list commits every complete value in one go, leaving only the
 * unterminated tail in the draft. Runs of whitespace collapse rather than
 * committing empty values.
 *
 * Kept apart from the composable so the rule that decides what becomes a tag
 * is testable without a Compose harness.
 */
internal fun tagDraftEdit(text: String): TagDraftEdit {
    val cut = text.indexOfLast { it.isWhitespace() }
    if (cut < 0) return TagDraftEdit(text, emptyList())
    val committed = text.substring(0, cut + 1)
        .split(Regex("\\s+"))
        .filter { it.isNotEmpty() }
    return TagDraftEdit(text.substring(cut + 1), committed)
}

// Метрики скопированы из kit/TagField.css на ПК (Figma "ResultV" → App Design,
// ряд `SmartRules`), чтобы поле читалось одинаково на обеих платформах.
//
// Коробка: заливка Dark Grey, скругление 24, отступ 16, зазор 10, рамка
// фокуса Main-color 20 %. Рамки в покое нет — только заливка.
// Тег: заливка Light Gray, скругление 100, отступы 6/10/8/10, зазор 4,
// подпись белым 50 % (кегль — см. TagTextStyle), крестик 14.
private val FieldShape = RoundedCornerShape(RvRadius.card)
private val FieldPadding = RvSpace.nest1
private val FieldGap = RvSpace.nest3
private val ChipShape = RoundedCornerShape(100.dp)

// На ПК высота фиксирована (156px). На телефоне это съело бы шестую часть
// экрана, поэтому взят минимум: поле сразу читается коробкой, а не строкой,
// и растёт под теги.
private val FieldMinHeight = 112.dp

// Крестик рисуется в 14 dp, как на ПК, но мышиная цель там не годится для
// пальца — иконка живёт в прозрачном боксе побольше, а отступ тега справа
// уменьшен на ту же величину, чтобы видимая геометрия осталась прежней.
private val RemoveIconSize = 14.dp
private val RemoveTouchSize = 22.dp

// Единственное сознательное отступление от метрик ПК: там подпись 14, здесь 12.
// На мониторе теги стоят в широком поле, на телефоне тот же кегль рядом с
// остальным текстом секции читался крупно. Межстрочный оставлен в той же
// пропорции 1.4. Стиль один на все три текста поля — подпись тега, плейсхолдер
// и ввод, — чтобы они не разъезжались.
private val TagTextStyle = TextStyle(
    fontFamily = SegoeUi,
    fontSize = 12.sp,
    fontWeight = FontWeight.Medium,
    lineHeight = 16.8.sp,
)

/**
 * One field that looks like a text box and turns what is typed into chips —
 * the desktop's `TagField.jsx` / `TagField.css`.
 *
 * Shared behaviour with the desktop: the placeholder shows only while there is
 * nothing at all, a tap on empty space puts the caret in the input, and losing
 * focus commits rather than silently dropping what was typed.
 *
 * Touch-specific on top of it: space commits and keeps the keyboard up (the
 * natural way to type a list on a phone), Enter commits and dismisses it, and
 * backspace on an empty draft removes the last chip.
 *
 * The draft is hoisted rather than held inside, because callers show live hints
 * about what is being typed — the Rules screen warns that a half-typed pattern
 * would shadow an existing one, or that another tab already holds it.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun TagField(
    values: List<String>,
    draft: String,
    onDraftChange: (String) -> Unit,
    onCommit: (String) -> Unit,
    onRemove: (String) -> Unit,
    placeholder: String,
    modifier: Modifier = Modifier,
) {
    val keyboard = LocalSoftwareKeyboardController.current
    val focusRequester = remember { FocusRequester() }
    var focused by remember { mutableStateOf(false) }

    // Committing on Enter and on focus loss both funnel through here so the
    // trim/empty rule lives in exactly one place.
    val commitDraft = {
        val value = draft.trim()
        if (value.isNotEmpty()) {
            onCommit(value)
            onDraftChange("")
        }
    }

    FlowRow(
        modifier = modifier
            .fillMaxWidth()
            .heightIn(min = FieldMinHeight)
            // На ПК три ступени: страница #141414 → поле #171717 → тег #1f1f1f.
            // Здесь шторка настроек уже залита тем же серым, что и RvColor.Grey,
            // поэтому взять его же под поле значило бы слить коробку с фоном:
            // ступень задаётся подъёмом белым, а тег поднимается до RvColor.LightGray.
            .background(Color.White.copy(alpha = 0.04f), FieldShape)
            // Рамка только в фокусе, как на ПК: в покое коробку держит заливка.
            .border(
                width = 1.dp,
                color = if (focused) RvColor.Main.copy(alpha = 0.2f) else Color.Transparent,
                shape = FieldShape,
            )
            // A tap anywhere in the box focuses the input, the way the whole
            // area of an ordinary text field is clickable.
            .clickable(
                interactionSource = remember { MutableInteractionSource() },
                indication = null,
            ) { focusRequester.requestFocus() }
            .padding(FieldPadding),
        horizontalArrangement = Arrangement.spacedBy(FieldGap),
        verticalArrangement = Arrangement.spacedBy(FieldGap),
    ) {
        values.forEach { value ->
            TagChip(label = value, onRemove = { onRemove(value) })
        }
        BasicTextField(
            value = draft,
            onValueChange = { text ->
                val edit = tagDraftEdit(text)
                edit.commits.forEach(onCommit)
                onDraftChange(edit.draft)
            },
            modifier = Modifier
                // No explicit width: FlowRow hands the input whatever is left
                // on the row, so the caret continues after the last chip and
                // wraps to a fresh line only when the remainder gets too thin.
                .widthIn(min = 120.dp)
                .align(Alignment.CenterVertically)
                .focusRequester(focusRequester)
                .onFocusChanged {
                    focused = it.isFocused
                    if (!it.isFocused) commitDraft()
                }
                .onPreviewKeyEvent { event ->
                    val backspaceOnEmpty = event.type == KeyEventType.KeyDown &&
                        event.key == Key.Backspace &&
                        draft.isEmpty()
                    if (backspaceOnEmpty && values.isNotEmpty()) {
                        onRemove(values.last())
                        true
                    } else {
                        false
                    }
                },
            singleLine = true,
            textStyle = TagTextStyle.copy(color = Color.White),
            cursorBrush = SolidColor(Color.White.copy(alpha = 0.5f)),
            keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
            keyboardActions = KeyboardActions(onDone = {
                commitDraft()
                keyboard?.hide()
            }),
            decorationBox = { inner ->
                // Подсказка лежит ПОД вводом, а не рядом с ним: в ряду она
                // занимала бы место и отодвигала каретку вправо от себя.
                Box(contentAlignment = Alignment.CenterStart) {
                    if (values.isEmpty() && draft.isEmpty()) {
                        Text(
                            placeholder,
                            style = TagTextStyle,
                            color = Color.White.copy(alpha = 0.2f),
                        )
                    }
                    inner()
                }
            },
        )
    }
}

@Composable
private fun TagChip(label: String, onRemove: () -> Unit) {
    Row(
        modifier = Modifier
            .background(RvColor.LightGray, ChipShape)
            .padding(
                start = RvSpace.nest3,
                // Прозрачное поле крестика шире самой иконки на 4 dp с каждой
                // стороны, поэтому отступ тега справа меньше отступа слева —
                // компенсация под шкалу RvSpace, а не точный пиксельный расчёт,
                // как было на ПК.
                end = RvSpace.xs,
                top = RvSpace.xs,
                bottom = RvSpace.nest3,
            ),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(0.dp),
    ) {
        Text(
            label,
            style = TagTextStyle,
            color = Color.White.copy(alpha = 0.5f),
        )
        Box(
            modifier = Modifier
                .size(RemoveTouchSize)
                .clickable(
                    interactionSource = remember { MutableInteractionSource() },
                    indication = null,
                    onClick = onRemove,
                ),
            contentAlignment = Alignment.Center,
        ) {
            Icon(
                Icons.Outlined.Close,
                contentDescription = stringResource(R.string.action_remove),
                tint = Color.White.copy(alpha = 0.5f),
                modifier = Modifier.size(RemoveIconSize),
            )
        }
    }
}
