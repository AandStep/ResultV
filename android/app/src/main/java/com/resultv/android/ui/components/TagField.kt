package com.resultv.android.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
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
import androidx.compose.material3.IconButton
import androidx.compose.material3.LocalTextStyle
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
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
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import com.resultv.android.R
import com.resultv.android.theme.Brand

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

private val ChipShape = RoundedCornerShape(50)

/**
 * One field that looks like a text box and turns what is typed into chips —
 * the desktop's `TagField.jsx` (Figma "ResultV" → App Design, row `SmartRules`).
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
            .border(1.dp, Color.White.copy(alpha = 0.12f), RoundedCornerShape(16.dp))
            .background(Color.White.copy(alpha = 0.04f), RoundedCornerShape(16.dp))
            // A tap anywhere in the box focuses the input, the way the whole
            // area of an ordinary text field is clickable.
            .clickable(
                interactionSource = remember { MutableInteractionSource() },
                indication = null,
            ) { focusRequester.requestFocus() }
            .padding(horizontal = 10.dp, vertical = 8.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
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
                .onFocusChanged { if (!it.isFocused) commitDraft() }
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
            textStyle = LocalTextStyle.current.copy(color = MaterialTheme.colorScheme.onSurface),
            cursorBrush = SolidColor(MaterialTheme.colorScheme.onSurface),
            keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
            keyboardActions = KeyboardActions(onDone = {
                commitDraft()
                keyboard?.hide()
            }),
            decorationBox = { inner ->
                Row(verticalAlignment = Alignment.CenterVertically) {
                    // Only while the field is completely empty: next to chips
                    // there is nowhere to put it, and by then it has said all
                    // it had to say.
                    if (values.isEmpty() && draft.isEmpty()) {
                        Text(
                            placeholder,
                            style = MaterialTheme.typography.bodyMedium,
                            color = Brand.MutedText,
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
            .background(Color.White.copy(alpha = 0.07f), ChipShape)
            .border(1.dp, Color.White.copy(alpha = 0.09f), ChipShape)
            .padding(start = 14.dp, end = 6.dp, top = 6.dp, bottom = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        Text(label, style = MaterialTheme.typography.bodyMedium)
        IconButton(onClick = onRemove, modifier = Modifier.size(28.dp)) {
            Icon(
                Icons.Outlined.Close,
                contentDescription = stringResource(R.string.action_remove),
                tint = Brand.MutedText,
                modifier = Modifier.size(16.dp),
            )
        }
    }
}
