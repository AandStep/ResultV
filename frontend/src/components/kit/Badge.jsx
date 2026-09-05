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


import "./Badge.css";

export const BADGE_VARIANTS = ["first", "second"];
export const BADGE_COLORS = ["default", "warning", "success", "error"];

/**
 * Бейдж из UI-kit (Figma node 6503:3035). Ширина — по содержимому.
 */
export default function Badge({ variant = "first", color = "default", children, className = "", ...rest }) {
  return (
    <span className={`rv-badge ${className}`} data-variant={variant} data-color={color} {...rest}>
      {children}
    </span>
  );
}
