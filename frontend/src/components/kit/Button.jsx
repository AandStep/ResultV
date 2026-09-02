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

import "./Button.css";

export const BUTTON_VARIANTS = ["default", "green", "yellow", "red"];
export const BUTTON_MODES = ["default", "hover", "active", "idle", "disable"];

/**
 * Кнопка из UI-kit (Figma node 6523:385).
 *
 * Ховер и нажатие работают сами, от CSS. Проп `mode` нужен только там, где
 * состояние надо показать принудительно — например на витрине кита; он же
 * включает `idle`, у которого нет своего CSS-триггера.
 *
 * Иконка передаётся слотом и наследует цвет подписи, как в макете.
 */
export default function Button({
  variant = "default",
  mode,
  icon,
  children,
  className = "",
  disabled,
  ...rest
}) {
  return (
    <button
      type="button"
      className={`rv-btn rv-border rv-press ${className}`}
      data-variant={variant}
      data-mode={mode}
      disabled={disabled ?? mode === "disable"}
      {...rest}
    >
      {icon}
      {children != null && <span>{children}</span>}
    </button>
  );
}
