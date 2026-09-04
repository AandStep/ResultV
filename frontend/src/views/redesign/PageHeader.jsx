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

/*
 * Заголовок страницы: название и строка под ним.
 *
 * В Figma это не компонент, а одинаковый фрейм `Header` на каждой внутренней
 * странице (Add page 6533:2003, ServersPage 6557:3439, Logs 6618:4066,
 * Settings 6628:3211). Компонент кита `Header` — другое: это шапка главного
 * экрана с состоянием подключения.
 */

import Icon from "../../components/kit/Icon";
import "./PageHeader.css";

/**
 * `actions` — кнопки-иконки справа от названия. Они есть только у страницы
 * серверов (6557:3439): замер задержки и порядок списка.
 *
 * `onBack` — стрелка слева от названия. Её в макете нет: страницы внутри
 * пунктов настроек не нарисованы, а уходить с них надо. См. GAPS.md.
 */
export default function PageHeader({
  title,
  subtitle,
  actions,
  onBack,
  backLabel,
  className = "",
  ...rest
}) {
  return (
    <div
      className={`rv-page-header ${className}`}
      data-actions={actions ? "true" : undefined}
      {...rest}
    >
      {onBack && (
        <button
          type="button"
          className="rv-page-header__back"
          title={backLabel}
          aria-label={backLabel}
          onClick={onBack}
        >
          <Icon name="back" color="currentColor" />
        </button>
      )}
      {/* Название с подписью — одна колонка, чтобы подпись равнялась по
          названию, а не по стрелке. */}
      <div className="rv-page-header__text">
        <div className="rv-page-header__row">
          <h1 className="rv-page-header__title">{title}</h1>
          {actions && <div className="rv-page-header__actions">{actions}</div>}
        </div>
        {subtitle && <p className="rv-page-header__subtitle">{subtitle}</p>}
      </div>
    </div>
  );
}
