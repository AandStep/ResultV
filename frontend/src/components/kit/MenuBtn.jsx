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


import Icon from "./Icon";
import "./MenuBtn.css";

/**
 * Пункт бокового меню (Figma node 6491:1071).
 *
 * Иконка передаётся именем из кита и всегда наследует цвет подписи —
 * так она нарисована во всех четырёх состояниях.
 * `state` нужен только витрине кита, обычно хватает `active` плюс CSS-ховер.
 */
export default function MenuBtn({
  icon,
  label,
  active = false,
  showLabel = true,
  state,
  className = "",
  ...rest
}) {
  return (
    <button
      type="button"
      className={`rv-menu-btn ${className}`}
      data-active={active}
      data-state={state}
      {...rest}
    >
      {icon && <Icon name={icon} color="currentColor" />}
      {/*
        Подпись всегда в разметке, видимость держит CSS. Раньше она
        появлялась и исчезала монтированием — то есть мгновенно, когда
        сайдбар только начинал раздвигаться, и на доли секунды текст
        оказывался снаружи. Прозрачностью её можно провести вместе с
        шириной, а лишнюю ширину обрезает сама кнопка.
      */}
      <span className="rv-menu-btn__label" data-visible={showLabel}>
        {label}
      </span>
    </button>
  );
}
