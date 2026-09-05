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

import { ICONS, ICON_STATE_COLOR, ICON_COLOR_OVERRIDE } from "./icons";

/**
 * Иконка UI-kit. Геометрию даёт Material Design Icons (@mdi/js), размеры,
 * сдвиги и цвета сняты с макета.
 *
 * Обёртка своя, а не <Icon> из @mdi/react, по одной причине: у двух иконок
 * макета есть дробный сдвиг внутри рамки (`settings`, `open`), а @mdi/react
 * умеет только size/color/rotate и выразить его не может. Здесь сдвиг уходит
 * в viewBox и совпадает попиксельно.
 *
 * Цвет идёт через currentColor, поэтому родитель может перекрыть его обычным
 * CSS — например ховером на строке меню.
 */
export default function Icon({ name, state = "default", size, color, className = "", style, ...rest }) {
  const icon = ICONS[name];
  if (!icon) {
    if (import.meta.env.DEV) console.warn(`[Icon] нет иконки "${name}" в UI-kit`);
    return null;
  }

  // Состояния, которого нет в макете, не выдумываем: откатываемся на default и
  // говорим об этом вслух, чтобы пробел попал в GAPS.md, а не растворился.
  let resolved = state;
  if (!icon.states.includes(state)) {
    if (import.meta.env.DEV) {
      console.warn(
        `[Icon] у "${name}" в макете нет состояния "${state}" ` +
          `(есть: ${icon.states.join(", ")}). Рисую "default".`
      );
    }
    resolved = "default";
  }

  const path = typeof icon.path === "string" ? icon.path : icon.path[resolved];
  /*
   * Внутри компонентов иконка иногда идёт фирменным цветом, а не белым из
   * фрейма Icons — тогда цвет задаёт вызывающая сторона.
   *
   * Особый случай — `currentColor`: он означает «цвет берёт родитель», и
   * тогда инлайн-стиль ставить нельзя. Инлайн перебивает любой класс, и
   * правило вида `.rv-power-btn__icon { color: ... }` до иконки уже не
   * доедет — она останется белой.
   */
  const inherit = color === "currentColor";
  const resolvedColor = inherit
    ? undefined
    : color ?? ICON_COLOR_OVERRIDE[name]?.[resolved] ?? ICON_STATE_COLOR[resolved];
  const [dx, dy] = icon.offset ?? [0, 0];
  const px = size ?? icon.size;

  return (
    <svg
      width={px}
      height={px}
      viewBox={`${dx} ${dy} 24 24`}
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={`rv-icon ${className}`}
      style={resolvedColor ? { color: resolvedColor, ...style } : style}
      aria-hidden="true"
      focusable="false"
      {...rest}
    >
      <path d={path} fill="currentColor" />
    </svg>
  );
}
