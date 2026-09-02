/*
 * Copyright (C) 2026 ResultV
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

import { useRef, useState } from "react";
import Icon from "./Icon";
import "./TagField.css";

/**
 * Поле-теги: с виду тот же Textarea, но введённое превращается в «таблетки».
 *
 * Figma "ResultV" -> App Design, ряд `SmartRules`. Три кадра показывают весь
 * путь одного значения: пустое поле с подсказкой (6586:3055), набранный текст
 * белым (6594:3120) и он же после Enter — тег с крестиком (6594:3260) и
 * каретка следом (6594:3253).
 *
 * Черновик живёт внутри: за пределами поля он ничего не значит, наружу уходят
 * только готовые значения. Совпадения гасит вызывающая сторона — она же знает,
 * как значение нормализуется (домен к нижнему регистру, приложению
 * дописывается расширение).
 */
export default function TagField({
  values = [],
  onAdd,
  onRemove,
  placeholder,
  disabled = false,
  removeLabel,
  className = "",
  ...rest
}) {
  const [draft, setDraft] = useState("");
  const inputRef = useRef(null);

  const commit = () => {
    const value = draft.trim();
    if (!value) return;
    onAdd?.(value);
    setDraft("");
  };

  const onKeyDown = (event) => {
    if (event.key !== "Enter") return;
    /* Enter здесь — это «добавить», а не отправка формы вокруг поля. */
    event.preventDefault();
    commit();
  };

  return (
    <div
      className={`rv-tag-field rv-scroll ${className}`}
      data-disabled={disabled || undefined}
      /* Клик по пустому месту поля ставит курсор в ввод — как у Textarea,
         где кликабельна вся площадь. */
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) {
          event.preventDefault();
          inputRef.current?.focus();
        }
      }}
      {...rest}
    >
      {values.map((value) => (
        <span key={value} className="rv-tag-field__tag">
          <span className="rv-tag-field__tag-label">{value}</span>
          <button
            type="button"
            className="rv-tag-field__remove"
            aria-label={removeLabel ? `${removeLabel}: ${value}` : value}
            disabled={disabled}
            onClick={() => onRemove?.(value)}
          >
            <Icon name="close" size={14} color="currentColor" />
          </button>
        </span>
      ))}
      <input
        ref={inputRef}
        type="text"
        className="rv-tag-field__input"
        /* Подсказка нарисована только у пустого поля: рядом с тегами ей негде
           встать, да и смысл её к этому моменту исчерпан. */
        placeholder={values.length === 0 ? placeholder : undefined}
        value={draft}
        disabled={disabled}
        onChange={(event) => setDraft(event.target.value)}
        onKeyDown={onKeyDown}
        /* Уход из поля не теряет набранное: в макете нарисован только Enter,
           но потерять строку молча — это не состояние, а потеря данных. */
        onBlur={commit}
      />
    </div>
  );
}
