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
import "./SettingsItem.css";

/**
 * Строка настроек (Figma node 6636:3953).
 *
 * Иконка слева передаётся именем из кита и рисуется цветом Main-color —
 * так она задана в макете, а не белым 50 % как во фрейме Icons.
 * Значок внешней ссылки справа включается пропом `external`.
 */
export default function SettingsItem({
  icon,
  title,
  description,
  external = false,
  state,
  as: Tag = "div",
  className = "",
  ...rest
}) {
  return (
    <Tag className={`rv-settings-item rv-border ${className}`} data-state={state} {...rest}>
      <div className="rv-settings-item__row">
        <div className="rv-settings-item__head">
          {icon && (
            <div className="rv-settings-item__badge">
              <Icon name={icon} size={36} color="var(--rv-main-color)" />
            </div>
          )}
          <div className="rv-settings-item__text">
            <p className="rv-settings-item__title">{title}</p>
            {description && <p className="rv-settings-item__desc">{description}</p>}
          </div>
        </div>
        {external && <Icon name="externallink" />}
      </div>
    </Tag>
  );
}
