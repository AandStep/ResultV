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
import MenuBtn from "./MenuBtn";
import logoUrl from "../../assets/logo-resultv.svg";
import "./Sidebar.css";

/**
 * Боковое меню (Figma node 6492:1648).
 *
 * Пункты приходят списком `{ key, icon, label }`; подписи видны только в
 * развёрнутом виде. Нижний пункт (в макете «Настройки») передаётся отдельно —
 * он прижат к низу.
 *
 * `state` нужен витрине кита, чтобы показать вариант OpenHover без курсора.
 */
export default function Sidebar({
  opened = false,
  items = [],
  bottomItem,
  activeKey,
  onSelect,
  onToggle,
  state,
  className = "",
  ...rest
}) {
  const renderItem = (item) => (
    <MenuBtn
      key={item.key}
      icon={item.icon}
      label={item.label}
      showLabel={opened}
      active={item.key === activeKey}
      onClick={onSelect ? () => onSelect(item.key) : undefined}
    />
  );

  return (
    <nav
      className={`rv-sidebar rv-border rv-border--static ${className}`}
      data-opened={opened}
      data-state={state}
      {...rest}
    >
      <div className="rv-sidebar__top">
        <div className="rv-sidebar__logo-row">
          <button type="button" className="rv-sidebar__logo-btn" onClick={onToggle}>
            <span className="rv-sidebar__logo-stack">
              <img className="rv-sidebar__logo" src={logoUrl} alt="ResultV" />
              <Icon name="side" className="rv-sidebar__logo-toggle" />
            </span>
            {/* Название и кнопка свёртки всегда в разметке, видимость держит
                CSS — тем же ходом, что и подписи пунктов, чтобы всё
                появлялось разом и внутри уже раздвинутого сайдбара. */}
            <span className="rv-sidebar__wordmark" data-visible={opened}>
              ResultV
            </span>
          </button>
          <button
            type="button"
            className="rv-sidebar__collapse"
            data-visible={opened}
            tabIndex={opened ? undefined : -1}
            aria-hidden={opened ? undefined : true}
            onClick={onToggle}
          >
            <Icon name="side" />
          </button>
        </div>
        <div className="rv-sidebar__items">{items.map(renderItem)}</div>
      </div>
      {bottomItem && renderItem(bottomItem)}
    </nav>
  );
}
