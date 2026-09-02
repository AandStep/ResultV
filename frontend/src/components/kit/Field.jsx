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


import "./Field.css";

/**
 * Однострочное поле ввода (Figma node 6566:5077).
 * Ширина берётся от родителя; в макете это 588.
 *
 * `invalid` подсвечивает поле рамкой цвета ошибки. В макете такого состояния
 * нет — сделано по правилам кита, см. docs/design/GAPS.md.
 */
export function Input({ invalid = false, className = "", ...rest }) {
  return (
    <input
      type="text"
      className={`rv-input ${className}`}
      aria-invalid={invalid || undefined}
      {...rest}
    />
  );
}

/**
 * Многострочное поле (Figma node 6557:2093). Высота фиксированная — 156,
 * ручного изменения размера в макете нет.
 */
export function Textarea({ invalid = false, className = "", ...rest }) {
  return (
    <textarea
      className={`rv-textarea ${className}`}
      aria-invalid={invalid || undefined}
      {...rest}
    />
  );
}
