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
 * Заглушки для витрины. Повторяют то, что стоит на их месте в макете:
 * флаг — триколор, логотип провайдера — квадрат, графики скорости — те самые
 * кривые, что нарисованы в Figma. В приложении на их месте живые данные.
 */

import curveMain from "../assets/design-samples/speed-curve-main.svg";
import curveSecond from "../assets/design-samples/speed-curve-second.svg";

export const FlagStub = () => <span className="kit__flag" />;
export const LogoStub = () => <span className="kit__logo" />;

export const ChartMain = () => (
  <img className="rv-main-page__chart" src={curveMain} alt="" />
);

/* У второй плитки в макете та же кривая, отражённая по вертикали. */
export const ChartSecond = () => (
  <img
    className="rv-main-page__chart"
    style={{ transform: "scaleY(-1)" }}
    src={curveSecond}
    alt=""
  />
);

export const MENU_ITEMS = [
  { key: "home", icon: "home", label: "Главная" },
  { key: "add", icon: "add", label: "Добавить сервер" },
  { key: "list", icon: "list", label: "Список серверов" },
  { key: "routing", icon: "routing", label: "Умные правила" },
  { key: "buy", icon: "buy", label: "Купить сервер" },
  { key: "logs", icon: "logs", label: "Журнал логов" },
];

export const SETTINGS_ITEM = { key: "settings", icon: "settings", label: "Настройки" };
