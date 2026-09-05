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


import "./ModeTumbler.css";

/**
 * Переключатель режима (Figma node 6488:48).
 *
 * `value` — выбранный сегмент, `options` — пары `{ value, label }`.
 * Подписи приходят снаружи: в макете это «Прокси» и «Туннель», в приложении
 * они идут через i18n.
 */
export default function ModeTumbler({ value, options = [], onChange, className = "", ...rest }) {
  return (
    <div
      className={`rv-mode-tumbler rv-border rv-border--static ${className}`}
      role="tablist"
      {...rest}
    >
      {options.map((option) => {
        const selected = option.value === value;
        return (
          <button
            key={option.value}
            type="button"
            role="tab"
            aria-selected={selected}
            className="rv-mode-tumbler__segment"
            data-selected={selected}
            onClick={selected || !onChange ? undefined : () => onChange(option.value)}
          >
            {option.label}
          </button>
        );
      })}
    </div>
  );
}
