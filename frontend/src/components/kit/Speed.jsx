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


import "./Speed.css";

/**
 * Плитка скорости (Figma node 6528:714).
 *
 * График передаётся содержимым: в макете там нарисован образец кривой, а в
 * приложении это живые данные. Если содержимого нет, рисуется плоская линия
 * из макета — состояние «трафика не было».
 *
 * Подписи и значения приходят пропами: в макете это «Загружено» / «Отправлено»,
 * «312 кб/с» и «312 Мб».
 */
export default function Speed({
  variant = "default",
  mode = "default",
  label,
  rate,
  total,
  children,
  className = "",
  ...rest
}) {
  return (
    <div className={`rv-speed ${className}`} data-variant={variant} data-mode={mode} {...rest}>
      <div className="rv-speed__data">
        <div className="rv-speed__row">
          <p>{label}</p>
          <p>{rate}</p>
        </div>
        <p className="rv-speed__total">{total}</p>
      </div>
      {children ? (
        <div className="rv-speed__chart">{children}</div>
      ) : (
        <div className="rv-speed__chart--empty" />
      )}
    </div>
  );
}
