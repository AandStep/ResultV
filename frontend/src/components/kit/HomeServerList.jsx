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


import "./HomeServerList.css";

/**
 * Карточка сервера на главной (Figma node 6504:3877).
 *
 * `header` — строка-заголовок (`ServerItem variant="header"`), `children` —
 * строки списка (`ServerItem variant="row"`), они видны только когда
 * карточка раскрыта. В макете раскрытая карточка показана с двумя строками.
 *
 * Раскрытая карточка разворачивает шеврон в шапке остриём вверх: так она
 * нарисована во фреймах главной (`MainPage (Servers list)`). В самом
 * компоненте кита шеврон остался смотреть вниз, см. docs/design/GAPS.md.
 *
 * Список внутри прокручивается: в макете он показан с двумя серверами и
 * свободно вылезает за низ окна, а в приложении серверов бывает сколько
 * угодно.
 */
export default function HomeServerList({
  open = false,
  header,
  status,
  children,
  className = "",
  ...rest
}) {
  return (
    <div
      className={`rv-home-server-list rv-border ${className}`}
      data-status={status}
      data-open={open}
      {...rest}
    >
      <div className="rv-home-server-list__inner">
        {header}
        {open && <div className="rv-home-server-list__rows">{children}</div>}
      </div>
    </div>
  );
}
