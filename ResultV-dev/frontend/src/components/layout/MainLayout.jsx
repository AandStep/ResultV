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

import React, { useEffect, useRef } from "react";
import { Activity, ShoppingCart, Plus, List, Settings } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useConfigContext } from "../../context/ConfigContext";
import { Sidebar } from "./Sidebar";
import { MobileHeader } from "./MobileHeader";

const MobileNavItem = ({ icon, label, isActive, onClick }) => (
  <button
    onClick={onClick}
    className={`flex flex-col items-center p-2 min-w-[64px] border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${
      isActive ? "text-[#007E3A]" : "text-zinc-500 hover:text-[#00A819]"
    }`}
  >
    {React.cloneElement(icon, { className: "w-6 h-6 mb-1" })}
    <span className="text-[10px] font-medium">{label}</span>
  </button>
);

export const MainLayout = ({ children }) => {
  const { t } = useTranslation();
  const { activeTab, setActiveTab, setEditingProxy } = useConfigContext();
  const mainScrollRef = useRef(null);
  const prevTabRef = useRef(activeTab);

  useEffect(() => {
    if (
      activeTab === "list" &&
      prevTabRef.current === "add" &&
      mainScrollRef.current
    ) {
      mainScrollRef.current.scrollTop = 0;
    }
    prevTabRef.current = activeTab;
  }, [activeTab]);

  return (
    <div className="fixed inset-0 flex bg-zinc-950 text-zinc-200 font-sans overflow-hidden select-none">
      <style>{`
        * { 
          outline: none !important; 
          -webkit-tap-highlight-color: transparent !important; 
        }
        button { border-color: transparent; }
        button:hover, a:hover { 
          border-color: transparent;
        }
        button:focus, input:focus, a:focus { 
          outline: none !important; 
          box-shadow: none !important; 
        }
        :root { --bs-primary: transparent; }
        .scrollbar-hide::-webkit-scrollbar { display: none; }
      `}</style>

      <Sidebar />

      <div
        ref={mainScrollRef}
        className="flex-1 flex flex-col relative overflow-y-auto min-w-0 min-h-0 border-t border-zinc-800 [scrollbar-gutter:stable]"
      >
        <MobileHeader />

        <div className="flex min-h-full w-full max-w-[1600px] flex-col p-6 mx-auto">
          {children}
          {activeTab !== "home" && <div className="h-24 md:h-6 w-full shrink-0"></div>}
        </div>
      </div>

      <div className="md:hidden absolute bottom-0 w-full bg-zinc-900 border-t border-zinc-800 flex justify-around p-2 z-20 pb-safe">
        <MobileNavItem
          icon={<Activity />}
          label="Главная"
          isActive={activeTab === "home"}
          onClick={() => setActiveTab("home")}
        />
        <MobileNavItem
          icon={<ShoppingCart />}
          label={t("sidebar.buy")}
          isActive={activeTab === "buy"}
          onClick={() => setActiveTab("buy")}
        />
        <MobileNavItem
          icon={<Plus />}
          label="Добавить"
          isActive={activeTab === "add"}
          onClick={() => {
            setEditingProxy(null);
            setActiveTab("add");
          }}
        />
        <MobileNavItem
          icon={<List />}
          label="Прокси"
          isActive={activeTab === "list"}
          onClick={() => setActiveTab("list")}
        />
        <MobileNavItem
          icon={<Settings />}
          label="Настройки"
          isActive={activeTab === "settings"}
          onClick={() => setActiveTab("settings")}
        />
      </div>
    </div>
  );
};
