/*
 * Copyright (C) 2026 ResultProxy
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

import React from "react";
import {
  Activity,
  ShoppingCart,
  Plus,
  List,
  Split,
  Terminal,
  Settings,
} from "lucide-react";
import { useConfigContext } from "../../context/ConfigContext";
import { useConnectionContext } from "../../context/ConnectionContext";
import logo from "../../assets/logo.png";
import { useTranslation } from "react-i18next";
import { LanguageSwitcher } from "../ui/LanguageSwitcher";

const NavItem = ({ icon, label, isActive, onClick }) => (
  <button
    onClick={onClick}
    className={`w-full flex items-center space-x-3 px-4 py-3 rounded-xl transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${
      isActive
        ? "bg-[#007E3A]/10 text-[#007E3A]"
        : "text-zinc-400 hover:bg-zinc-800 hover:text-[#00A819]"
    }`}
  >
    {React.cloneElement(icon, { className: "w-5 h-5" })}
    <span className="font-medium">{label}</span>
  </button>
);

export const Sidebar = () => {
  const { activeTab, setActiveTab, setEditingProxy } = useConfigContext();
  const { daemonStatus } = useConnectionContext();
  const { t } = useTranslation();

  return (
    <div className="hidden md:flex flex-col w-64 bg-zinc-900 border-r border-zinc-800 shrink-0">
      <div className="p-6 flex items-center space-x-3">
        <img
          src={logo}
          alt="ResultProxy"
          className="w-8 h-8 drop-shadow-[0_0_10px_rgba(0,126,58,0.5)]"
        />
        <span className="text-xl font-bold text-white">ResultProxy</span>
      </div>

      {daemonStatus === "offline" && (
        <div className="mx-4 mb-2 p-2 bg-rose-500/10 border border-rose-500/20 rounded-lg flex items-center text-xs text-rose-400">
          <Activity className="w-4 h-4 mr-2 shrink-0" />
          <span>{t("sidebar.offline")}</span>
        </div>
      )}

      <nav className="flex-1 flex flex-col px-4 py-4 overflow-y-auto scrollbar-hide">
        <div className="space-y-1">
          <NavItem
            icon={<Activity />}
            label={t("sidebar.home")}
            isActive={activeTab === "home"}
            onClick={() => setActiveTab("home")}
          />
          <NavItem
            icon={<ShoppingCart />}
            label={t("sidebar.buy")}
            isActive={activeTab === "buy"}
            onClick={() => setActiveTab("buy")}
          />
          <NavItem
            icon={<Plus />}
            label={t("sidebar.add")}
            isActive={activeTab === "add"}
            onClick={() => {
              setEditingProxy(null);
              setActiveTab("add");
            }}
          />
          <NavItem
            icon={<List />}
            label={t("sidebar.list")}
            isActive={activeTab === "list"}
            onClick={() => setActiveTab("list")}
          />
          <NavItem
            icon={<Split />}
            label={t("sidebar.rules")}
            isActive={activeTab === "rules"}
            onClick={() => setActiveTab("rules")}
          />
          <NavItem
            icon={<Terminal />}
            label={t("sidebar.logs")}
            isActive={activeTab === "logs"}
            onClick={() => setActiveTab("logs")}
          />
        </div>

        <div className="mt-auto pt-4 pb-2 space-y-1">
          <NavItem
            icon={<Settings />}
            label={t("sidebar.settings")}
            isActive={activeTab === "settings"}
            onClick={() => setActiveTab("settings")}
          />
        </div>
      </nav>

      <div className="p-4 border-t border-zinc-800 flex items-center justify-between">
        <LanguageSwitcher />
        <span className="text-xs text-zinc-500">
          {t("sidebar.version", { version: "2.1.1" })}
        </span>
      </div>
    </div>
  );
};
