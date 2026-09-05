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
 * Боковое меню нового дизайна, подключённое к вкладкам приложения.
 * Состав и порядок пунктов — из макета; страницы берут его как есть.
 */

import { useTranslation } from "react-i18next";
import { useConfigContext } from "../../context/ConfigContext";
import { Sidebar } from "../../components/kit";

export const MENU = [
  { key: "home", icon: "home", label: "sidebar.home" },
  { key: "add", icon: "add", label: "sidebar.add" },
  { key: "list", icon: "list", label: "sidebar.list" },
  { key: "rules", icon: "routing", label: "sidebar.rules" },
  { key: "buy", icon: "buy", label: "sidebar.buy" },
  { key: "logs", icon: "logs", label: "sidebar.logs" },
];

export default function AppSidebar() {
  const { t } = useTranslation();
  /*
   * Раскрытость хранится в контексте, а не здесь: каждая страница рисует свой
   * экземпляр меню, и своё состояние схлопывалось бы на каждом переходе.
   */
  const { activeTab, setActiveTab, setEditingProxy, sidebarOpen, setSidebarOpen } =
    useConfigContext();

  /* «Добавить» — это всегда новый сервер: правку начинают из списка. */
  const select = (key) => {
    if (key === "add") setEditingProxy(null);
    setActiveTab(key);
  };

  return (
    <Sidebar
      opened={sidebarOpen}
      onToggle={() => setSidebarOpen((v) => !v)}
      items={MENU.map((item) => ({ ...item, label: t(item.label) }))}
      bottomItem={{ key: "settings", icon: "settings", label: t("sidebar.settings") }}
      activeKey={activeTab}
      onSelect={select}
    />
  );
}
