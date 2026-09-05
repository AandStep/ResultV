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
import "./BigBtn.css";

/**
 * Крупная кнопка-плитка (Figma node 6551:2825).
 *
 * Иконка 48 всегда идёт в цвет подписи. В макете показана `UploadFile`,
 * но имя приходит пропом — такой же плиткой в макетах страниц набраны
 * «Из буфера», «Вручную» и другие.
 */
export default function BigBtn({ icon, label, state, disabled, className = "", ...rest }) {
  return (
    <button
      type="button"
      className={`rv-big-btn rv-border rv-press ${className}`}
      data-state={state}
      disabled={disabled ?? state === "disable"}
      {...rest}
    >
      {icon && <Icon name={icon} size={48} color="currentColor" />}
      <span>{label}</span>
    </button>
  );
}
