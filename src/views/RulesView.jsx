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

import React, { useState, useRef } from "react";
import { Plus, Trash2, HelpCircle } from "lucide-react";
import { useConfigContext } from "../context/ConfigContext";
import { useTranslation } from "react-i18next";

export const RulesView = () => {
  const { t } = useTranslation();
  const {
    routingRules: rules,
    setRoutingRules: setRules,
    platform,
  } = useConfigContext();
  const [newDomain, setNewDomain] = useState("");
  const [newApp, setNewApp] = useState("");

  const appFileInputRef = useRef(null);

  const addDomain = () => {
    if (newDomain && !rules.whitelist.includes(newDomain)) {
      setRules({ ...rules, whitelist: [...rules.whitelist, newDomain] });
      setNewDomain("");
    }
  };

  const addSpecificDomain = (domain) => {
    if (!rules.whitelist.includes(domain)) {
      setRules({ ...rules, whitelist: [...rules.whitelist, domain] });
    }
  };

  const addApp = () => {
    if (newApp) {
      let appName = newApp.toLowerCase().trim();
      const isWin = platform === "win32";
      if (isWin && !appName.endsWith(".exe")) appName += ".exe";
      if (!isWin && !appName.includes(".")) appName += ".app";

      const currentList = rules.appWhitelist || [];
      if (!currentList.includes(appName)) {
        setRules({ ...rules, appWhitelist: [...currentList, appName] });
        setNewApp("");
      }
    }
  };

  const addSpecificApp = (appName) => {
    const currentList = rules.appWhitelist || [];
    if (!currentList.includes(appName)) {
      setRules({ ...rules, appWhitelist: [...currentList, appName] });
    }
  };

  const handleFileSelect = (e) => {
    const file = e.target.files[0];
    if (file) {
      let appName = file.name.toLowerCase();
      const isWin = platform === "win32";
      if (isWin && !appName.endsWith(".exe")) appName += ".exe";
      if (!isWin && !appName.includes(".")) appName += ".app";
      const currentList = rules.appWhitelist || [];
      if (!currentList.includes(appName)) {
        setRules({ ...rules, appWhitelist: [...currentList, appName] });
      }
      e.target.value = "";
    }
  };

  const popularTlds = ["*.ru", "*.рф", "*.su", "*.by", "*.kz"];
  const isWin = platform === "win32";
  const appExt = isWin ? ".exe" : ".app";
  const appPlaceholder = isWin ? "steam.exe" : "safari.app";

  const popularApps = isWin
    ? ["steam.exe", "discord.exe", "telegram.exe", "epicgameslauncher.exe"]
    : ["safari.app", "discord.app", "telegram.app"];
  const safeAppWhitelist = rules.appWhitelist || [];

  return (
    <div className="space-y-6 animate-in fade-in duration-300">
      <div>
        <h2 className="text-3xl font-bold text-white">{t("rules.title")}</h2>
        <p className="text-zinc-400 mt-2">{t("rules.desc")}</p>
      </div>

      <div className="grid gap-4">
        <div
          onClick={() => setRules({ ...rules, mode: "global" })}
          className={`p-6 rounded-3xl border cursor-pointer transition-colors outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${rules.mode === "global" ? "border-[#007E3A] bg-[#007E3A]/10" : "border-zinc-800 bg-zinc-900 hover:border-zinc-700"}`}
        >
          <h4 className="text-white font-bold text-lg">
            {t("rules.modes.global")}
          </h4>
          <p className="text-zinc-500 text-sm mt-1">
            {t("rules.modes.global_desc")}
          </p>
        </div>
        <div
          onClick={() => setRules({ ...rules, mode: "smart" })}
          className={`p-6 rounded-3xl border cursor-pointer transition-colors outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${rules.mode === "smart" ? "border-[#00A819] bg-[#00A819]/10" : "border-zinc-800 bg-zinc-900 hover:border-zinc-700"}`}
        >
          <h4 className="text-white font-bold text-lg">
            {t("rules.modes.smart")}
          </h4>
          <p className="text-zinc-500 text-sm mt-1">
            {t("rules.modes.smart_desc")}
          </p>
        </div>
      </div>

      <div className="bg-zinc-900 p-8 rounded-3xl border border-zinc-800 mt-6">
        <div className="flex items-center space-x-2 mb-2">
          <h3 className="text-white font-bold text-lg">
            {t("rules.domains.title")}
          </h3>
          <div className="relative group">
            <HelpCircle className="w-5 h-5 text-zinc-500 hover:text-zinc-300 cursor-help transition-colors" />
            <div className="absolute bottom-full left-0 mb-2 px-3 py-2 bg-zinc-800 text-zinc-200 text-xs rounded-lg opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none w-64 z-10 border border-zinc-700 shadow-xl">
              {t("rules.domains.tooltip")}
            </div>
          </div>
        </div>

        <p className="text-zinc-500 text-sm mb-6">{t("rules.domains.desc")}</p>
        <div className="flex space-x-3 mb-6">
          <input
            type="text"
            placeholder={t("rules.domains.placeholder")}
            className="flex-1 bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:outline-none focus:ring-0 focus-visible:outline-none focus:border-[#007E3A] transition-colors"
            value={newDomain}
            onChange={(e) => setNewDomain(e.target.value)}
            onKeyPress={(e) => e.key === "Enter" && addDomain()}
          />
          <button
            onClick={addDomain}
            className="bg-[#007E3A] hover:bg-[#00A819] text-white px-6 font-bold rounded-xl transition-colors border-none outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
          >
            {t("rules.domains.add_btn")}
          </button>
        </div>
        <div className="flex flex-wrap gap-2 mb-8">
          {rules.whitelist.map((d) => (
            <div
              key={d}
              className="bg-zinc-800 px-4 py-2 rounded-lg flex items-center space-x-3 border border-zinc-700"
            >
              <span className="text-sm text-zinc-300">{d}</span>
              <button
                onClick={() =>
                  setRules({
                    ...rules,
                    whitelist: rules.whitelist.filter((i) => i !== d),
                  })
                }
                className="text-zinc-500 hover:text-rose-500 transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
              >
                <Trash2 className="w-4 h-4" />
              </button>
            </div>
          ))}
        </div>

        <div className="pt-6 border-t border-zinc-800">
          <p className="text-sm font-medium text-zinc-400 mb-4">
            {t("rules.domains.fast_add")}
          </p>
          <div className="flex flex-wrap gap-3">
            {popularTlds.map((tld) => {
              const isAdded = rules.whitelist.includes(tld);
              return (
                <button
                  key={tld}
                  onClick={() => addSpecificDomain(tld)}
                  disabled={isAdded}
                  className={`flex items-center px-4 py-2 rounded-xl text-sm font-medium transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${
                    isAdded
                      ? "bg-[#007E3A]/10 text-[#007E3A] border-[#007E3A]/20 cursor-default"
                      : "bg-zinc-800 text-zinc-300 border-zinc-700 hover:border-[#00A819] hover:text-white cursor-pointer"
                  }`}
                >
                  {!isAdded && <Plus className="w-4 h-4 mr-2" />}
                  {tld}
                </button>
              );
            })}
          </div>
        </div>
      </div>

      <div className="bg-zinc-900 p-8 rounded-3xl border border-zinc-800 mt-6">
        <h3 className="text-white font-bold text-lg mb-2">
          {t("rules.apps.title")} ({isWin ? ".EXE" : ".APP"})
        </h3>
        <p className="text-zinc-500 text-sm mb-6">
          {t("rules.apps.desc1")}
          <strong>{t("rules.apps.desc2")}</strong>
        </p>
        <div className="flex space-x-3 mb-6">
          <input
            type="text"
            placeholder={`${t("rules.apps.placeholder")}${appPlaceholder}`}
            className="flex-1 bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:outline-none focus:ring-0 focus-visible:outline-none focus:border-[#007E3A] transition-colors"
            value={newApp}
            onChange={(e) => setNewApp(e.target.value)}
            onKeyPress={(e) => e.key === "Enter" && addApp()}
          />
          <button
            onClick={addApp}
            className="bg-zinc-800 hover:bg-zinc-700 text-white px-6 font-bold rounded-xl transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none shrink-0"
          >
            {t("rules.apps.add_manual")}
          </button>

          <input
            type="file"
            accept={appExt}
            className="hidden"
            ref={appFileInputRef}
            onChange={handleFileSelect}
          />
          <button
            onClick={() => appFileInputRef.current.click()}
            className="bg-[#007E3A] hover:bg-[#00A819] text-white px-6 font-bold rounded-xl transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none shrink-0"
          >
            {t("rules.apps.choose_file")}
            {appExt}
          </button>
        </div>
        <div className="flex flex-wrap gap-2 mb-8">
          {safeAppWhitelist.map((appName) => (
            <div
              key={appName}
              className="bg-zinc-800 px-4 py-2 rounded-lg flex items-center space-x-3 border border-zinc-700"
            >
              <span className="text-sm text-zinc-300 font-mono">{appName}</span>
              <button
                onClick={() =>
                  setRules({
                    ...rules,
                    appWhitelist: safeAppWhitelist.filter((i) => i !== appName),
                  })
                }
                className="text-zinc-500 hover:text-rose-500 transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
              >
                <Trash2 className="w-4 h-4" />
              </button>
            </div>
          ))}
        </div>

        <div className="pt-6 border-t border-zinc-800">
          <p className="text-sm font-medium text-zinc-400 mb-4">
            {t("rules.apps.popular")}
          </p>
          <div className="flex flex-wrap gap-3">
            {popularApps.map((appName) => {
              const isAdded = safeAppWhitelist.includes(appName);
              return (
                <button
                  key={appName}
                  onClick={() => addSpecificApp(appName)}
                  disabled={isAdded}
                  className={`flex items-center px-4 py-2 rounded-xl text-sm font-medium transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${
                    isAdded
                      ? "bg-[#007E3A]/10 text-[#007E3A] border-[#007E3A]/20 cursor-default"
                      : "bg-zinc-800 text-zinc-300 border-zinc-700 hover:border-[#00A819] hover:text-white cursor-pointer"
                  }`}
                >
                  {!isAdded && <Plus className="w-4 h-4 mr-2" />}
                  <span className="font-mono">{appName}</span>
                </button>
              );
            })}
          </div>
        </div>
      </div>
    </div>
  );
};
