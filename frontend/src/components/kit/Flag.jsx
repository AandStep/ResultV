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


import "./Flag.css";

export const FLAG_STATUSES = ["default", "warning", "success", "error"];

/**
 * Подложка под флаг страны (Figma node 6504:3256).
 *
 * Сам флаг передаётся содержимым — в макете на его месте лежит заглушка,
 * поэтому конкретной картинки кит не навязывает.
 */
export default function Flag({ size = "md", status = "default", children, className = "", ...rest }) {
  return (
    <div className={`rv-flag ${className}`} data-size={size} data-status={status} {...rest}>
      {children}
    </div>
  );
}
