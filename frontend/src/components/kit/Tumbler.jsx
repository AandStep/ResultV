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


import "./Tumbler.css";

/**
 * Тумблер из UI-kit (Figma node 6566:5073).
 *
 * Анимации перехода между состояниями в макете нет, поэтому её здесь тоже
 * нет — см. docs/design/GAPS.md B-2.
 */
export default function Tumbler({ checked = false, onChange, className = "", ...rest }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      className={`rv-tumbler rv-border ${className}`}
      data-state={checked ? "active" : "default"}
      onClick={onChange ? () => onChange(!checked) : undefined}
      {...rest}
    >
      <span className="rv-tumbler__knob" />
    </button>
  );
}
