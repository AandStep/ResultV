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
import "./PowerButton.css";

export const POWER_VARIANTS = ["default", "warning", "success", "error"];

/**
 * Кнопка питания (Figma node 6481:7) — главный элемент домашнего экрана.
 *
 * Вариант отражает состояние подключения: `default` — выключено,
 * `warning` — подключается, `success` — подключено, `error` — ошибка.
 * `state` нужен витрине кита, обычно хватает CSS-ховера и нажатия.
 */
export default function PowerButton({ variant = "default", state, className = "", ...rest }) {
  return (
    <button
      type="button"
      className={`rv-power-btn ${className}`}
      data-variant={variant}
      data-state={state}
      {...rest}
    >
      {/* data-state дублируется на круге: по нему CSS отличает
          принудительно заданное состояние от настоящего ховера. */}
      <span className="rv-power-btn__circle rv-border" data-state={state}>
        <Icon name="power" size={110} color="currentColor" className="rv-power-btn__icon" />
      </span>
    </button>
  );
}
