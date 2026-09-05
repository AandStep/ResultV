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
import "./Header.css";

export const HEADER_VARIANTS = ["default", "processing", "success", "error"];

/**
 * Шапка главного экрана (Figma node 6521:263).
 *
 * Подпись и время приходят пропами: в макете они записаны по-русски, а в
 * приложении идут через i18n. Значения из макета — «Не подключено»,
 * «Подключение...», «Защищено», «Что-то пошло не так» и время «12:23».
 *
 * Иконки справа умеют ховер, поэтому состоянием управляет CSS родителя:
 * `state` им здесь не передаётся.
 */
export default function Header({
  variant = "default",
  title,
  time,
  onSite,
  onTelegram,
  className = "",
  ...rest
}) {
  return (
    <div className={`rv-header ${className}`} data-variant={variant} {...rest}>
      <div className="rv-header__time">
        <Icon name="clock" />
        <span>{time}</span>
      </div>
      <p className="rv-header__title">{title}</p>
      <div className="rv-header__links">
        {/* Цвет наведения у каждой ссылки свой, поэтому его держит кнопка, а
            иконка берёт его через currentColor. */}
        <button
          type="button"
          className="rv-header__link rv-header__link--site"
          onClick={onSite}
        >
          <Icon name="site" color="currentColor" />
        </button>
        <button
          type="button"
          className="rv-header__link rv-header__link--tg"
          onClick={onTelegram}
        >
          <Icon name="tg" color="currentColor" />
        </button>
      </div>
    </div>
  );
}
