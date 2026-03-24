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

import React, { useState } from "react";
import {
  Power,
  Plus,
  Globe,
  Pencil,
  ChevronDown,
  Activity,
} from "lucide-react";
import { FlagIcon } from "../components/ui/FlagIcon";
import { SpeedChart } from "../components/ui/SpeedChart";
import { useConfigContext } from "../context/ConfigContext";
import { useConnectionContext } from "../context/ConnectionContext";
import { formatBytes, formatSpeed } from "../utils/formatters";
import { useTranslation } from "react-i18next";

export const HomeView = () => {
  const { t } = useTranslation();
  const { proxies, setEditingProxy, setActiveTab, settings } = useConfigContext();
  const {
    isConnected,
    isProxyDead,
    failedProxy,
    setFailedProxy,
    toggleConnection,
    activeProxy,
    stats,
    speedHistory,
    pings,
    selectAndConnect,
  } = useConnectionContext();

  const isError = !!failedProxy;
  const [isProxyListOpen, setIsProxyListOpen] = useState(false);
  const hasProxies = proxies.length > 0;
  const displayProxy = failedProxy || activeProxy || proxies.find(p => p.id === settings?.lastSelectedProxyId) || proxies[0];

  const goToBuy = () => setActiveTab("buy");
  const goToAdd = () => {
    setEditingProxy(null);
    setActiveTab("add");
  };
  const onEditProxy = (proxy) => {
    setEditingProxy(proxy);
    setActiveTab("add");
  };
  const goToProxyList = () => setActiveTab("list");

  return (
    <div className="flex flex-col items-center justify-center min-h-[75vh] space-y-10 animate-in fade-in zoom-in-95 duration-300">
      <div className="text-center space-y-2">
        <h2
          className={`text-3xl font-bold ${isConnected ? (isProxyDead ? "text-rose-500" : "text-[#007E3A]") : isError ? "text-rose-500" : "text-zinc-400"}`}
        >
          {isConnected
            ? isProxyDead
              ? t("home.status.lost")
              : t("home.status.protected")
            : isError
              ? t("home.status.error")
              : t("home.status.unprotected")}
        </h2>
        <p className="text-zinc-400">
          {isConnected
            ? isProxyDead
              ? t("home.desc.lost")
              : t("home.desc.protected")
            : isError
              ? t("home.desc.error")
              : t("home.desc.unprotected")}
        </p>
      </div>

      <div className="relative group my-8">
        <div
          className={`absolute inset-0 rounded-full blur-2xl transition-all duration-700 ${isConnected ? (isProxyDead ? "bg-rose-500/40 animate-pulse" : "bg-[#007E3A]/40") : isError ? "bg-rose-500/20 animate-pulse" : hasProxies ? "bg-zinc-800/10 group-hover:bg-zinc-800/20" : ""}`}
        ></div>
        <button
          disabled={!hasProxies && !isConnected}
          onClick={
            isError
              ? () => {
                  setFailedProxy(null);
                  toggleConnection();
                }
              : toggleConnection
          }
          className={`relative border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none flex items-center justify-center w-48 h-48 rounded-full transition-all duration-300 transform active:scale-95 ${
            !hasProxies && !isConnected
              ? "bg-zinc-900 border-4 border-zinc-800 text-zinc-600 opacity-50 cursor-not-allowed"
              : isConnected
                ? isProxyDead
                  ? "bg-rose-600 text-white shadow-2xl shadow-rose-500/50"
                  : "bg-[#007E3A] text-zinc-950 shadow-2xl shadow-[#007E3A]/50"
                : isError
                  ? "bg-zinc-900 border-4 border-rose-500/50 text-rose-500 shadow-2xl shadow-rose-500/20"
                  : "bg-gradient-to-br from-zinc-800 to-zinc-900 border-4 border-zinc-800 text-zinc-400 hover:border-[#007E3A] hover:text-[#007E3A] shadow-2xl"
          }`}
        >
          <Power
            className={`w-20 h-20 ${isConnected && !isProxyDead ? "drop-shadow-none" : isConnected || isError ? "drop-shadow-md" : ""}`}
          />
        </button>
      </div>

      {!hasProxies ? (
        <div className="w-full flex flex-col items-center animate-in fade-in duration-300">
          <p className="text-zinc-400 mb-4 text-center">
            {t("home.noProxies")}
            <span
              onClick={goToBuy}
              className="text-[#007E3A] hover:text-[#00A819] transition-colors cursor-pointer font-medium border-b border-transparent hover:border-[#00A819]"
            >
              {t("home.buyDiscount")}
            </span>
          </p>
          <div
            onClick={goToAdd}
            className="w-full bg-zinc-900 border border-dashed border-zinc-700 rounded-3xl p-8 flex flex-col items-center justify-center cursor-pointer hover:border-[#007E3A] hover:bg-zinc-800/50 transition-all group outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
          >
            <div className="bg-zinc-800 p-4 rounded-full mb-4 text-zinc-400 group-hover:text-[#007E3A] transition-colors">
              <Plus className="w-8 h-8" />
            </div>
            <p className="text-lg font-bold text-white mb-1">
              {t("home.addServer")}
            </p>
            <p className="text-sm text-zinc-500">{t("home.addManual")}</p>
          </div>
        </div>
      ) : (
        <div
          className={`w-full bg-zinc-900 rounded-3xl border flex flex-col overflow-hidden transition-all outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${(isProxyDead && isConnected) || isError ? "border-rose-500/30" : isProxyListOpen ? "border-zinc-700" : "border-zinc-800 hover:border-[#007E3A] hover:bg-zinc-800/50"}`}
        >
          <div
            onClick={() => setIsProxyListOpen(!isProxyListOpen)}
            className="p-5 flex items-center justify-between cursor-pointer transition-all group"
          >
            <div className="flex items-center space-x-5 min-w-0">
              <div
                className={`w-14 h-14 flex items-center justify-center rounded-2xl shrink-0 transition-colors ${isConnected ? (isProxyDead ? "bg-rose-500/20 text-rose-500" : "bg-[#007E3A]/20 text-[#007E3A]") : isError ? "bg-rose-500/10 text-rose-500" : "bg-zinc-800 text-zinc-500 group-hover:bg-zinc-700"}`}
              >
                {displayProxy ? (
                  <FlagIcon
                    code={displayProxy.country}
                    className="w-8 rounded-sm shadow-sm"
                  />
                ) : (
                  <Globe className="w-8 h-8" />
                )}
              </div>
              <div className="min-w-0">
                <p className="text-sm text-zinc-400 mb-1 truncate">
                  {t("home.currentServer")}
                </p>
                <p className="text-lg font-bold text-white truncate">
                  {displayProxy ? displayProxy.name : t("home.emptyServer")}
                </p>
                {displayProxy && (
                  <p className="text-sm text-zinc-500 font-mono mt-1 truncate">
                    {displayProxy.ip}:{displayProxy.port} ({displayProxy.type})
                  </p>
                )}
              </div>
            </div>

            <div className="flex items-center space-x-1 shrink-0 ml-4">
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  onEditProxy(displayProxy);
                }}
                className={`p-2 rounded-xl transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${(isProxyDead && isConnected) || isError ? "text-rose-500/50 hover:text-rose-500 hover:bg-rose-500/10" : "text-zinc-600 hover:text-[#007E3A] hover:bg-[#007E3A]/10"}`}
              >
                <Pencil className="w-5 h-5" />
              </button>
              <div
                className={`p-2 rounded-xl transition-colors text-zinc-500 group-hover:text-zinc-300`}
              >
                <ChevronDown
                  className={`w-5 h-5 transition-transform duration-300 ${isProxyListOpen ? "rotate-180" : ""}`}
                />
              </div>
            </div>
          </div>

          {isProxyListOpen && (
            <div className="bg-zinc-950/50 border-t border-zinc-800/50 p-2 max-h-[280px] overflow-y-auto scrollbar-hide space-y-2 animate-in slide-in-from-top-2 duration-200">
              {Object.entries(
                proxies.reduce((acc, proxy) => {
                  const countryCode = proxy.country || "Unknown";
                  if (!acc[countryCode]) acc[countryCode] = [];
                  acc[countryCode].push(proxy);
                  return acc;
                }, {}),
              )
                .sort(([countryA], [countryB]) =>
                  countryA.localeCompare(countryB),
                )
                .map(([country, countryProxies]) => (
                  <div key={country} className="space-y-1 mb-2 last:mb-0">
                    <div className="flex items-center px-3 py-1 space-x-2">
                      <FlagIcon
                        code={country}
                        className="w-5 h-auto rounded-[2px] opacity-70"
                      />
                      <span className="text-xs font-bold text-zinc-500 uppercase tracking-wider">
                        {country}
                      </span>
                    </div>
                    {countryProxies.map((proxy) => {
                      const isActive = activeProxy?.id === proxy.id;
                      return (
                        <div
                          key={proxy.id}
                          onClick={(e) => {
                            e.stopPropagation();
                            selectAndConnect(proxy);
                            setIsProxyListOpen(false);
                          }}
                          className={`flex items-center justify-between p-3 rounded-2xl cursor-pointer transition-colors outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${isActive ? "bg-[#007E3A]/10 border border-[#007E3A]/20" : "bg-zinc-900/50 border border-transparent hover:border-[#00A819]/50 hover:bg-zinc-800/80"}`}
                        >
                          <div className="flex items-center space-x-4 min-w-0">
                            <div className="shrink-0 flex items-center justify-center w-10 h-10 bg-zinc-800/50 rounded-lg border border-zinc-700/50">
                              <FlagIcon
                                code={proxy.country}
                                className="w-6 h-auto rounded-[2px]"
                              />
                            </div>
                            <div className="min-w-0">
                              <h4
                                className={`text-sm font-bold truncate transition-colors ${isActive ? "text-[#00A819]" : "text-white"}`}
                              >
                                {proxy.name}
                              </h4>
                              <p className="text-xs text-zinc-500 font-mono mt-0.5 truncate">
                                {proxy.ip}:{proxy.port}
                              </p>
                            </div>
                          </div>
                          <div className="flex items-center space-x-3 shrink-0 ml-3">
                            <div
                              className={`text-xs flex items-center ${pings[proxy.id] === "Timeout" || pings[proxy.id] === "Error" ? "text-rose-500" : "text-zinc-500"}`}
                            >
                              <Activity className="w-3 h-3 mr-1" />{" "}
                              {pings[proxy.id] || "..."}
                            </div>
                            {isActive ? (
                              <div className="w-2 h-2 rounded-full bg-[#00A819] shadow-[0_0_8px_rgba(0,168,25,0.8)]"></div>
                            ) : (
                              <div className="w-2 h-2 rounded-full bg-zinc-700"></div>
                            )}
                          </div>
                        </div>
                      );
                    })}
                  </div>
                ))}
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  goToProxyList();
                }}
                className="w-full mt-2 py-3 text-sm text-zinc-400 hover:text-white transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
              >
                {t("home.openList")}
              </button>
            </div>
          )}
        </div>
      )}

      {isError ? (
        <div className="flex space-x-4 w-full animate-in slide-in-from-bottom-4 fade-in duration-300">
          <button
            onClick={() => onEditProxy(displayProxy)}
            className="flex-1 border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none bg-zinc-900 border border-zinc-800 hover:border-zinc-700 text-white py-4 rounded-3xl font-bold transition-colors"
          >
            {t("home.editData")}
          </button>
          <button
            onClick={() => {
              setFailedProxy(null);
              goToProxyList();
            }}
            className="flex-1 border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none bg-rose-500/10 border border-rose-500/20 hover:bg-rose-500/20 text-rose-500 py-4 rounded-3xl font-bold transition-colors"
          >
            {t("home.chooseOther")}
          </button>
        </div>
      ) : (
        <div
          className={`w-full grid grid-cols-2 gap-6 transition-all duration-500 ${isConnected ? "opacity-100 translate-y-0" : "opacity-0 translate-y-4 pointer-events-none"}`}
        >
          <div className="bg-zinc-900 rounded-3xl p-6 border border-zinc-800 flex flex-col min-w-0 relative">
            <div className="flex justify-between items-start w-full">
              <p className="text-sm text-zinc-500 mb-2 truncate font-bold uppercase tracking-widest">
                {t("home.download")}
              </p>
              <p
                className={`text-[10px] font-bold ${isProxyDead ? "text-zinc-600" : "text-[#007E3A]"}`}
              >
                {formatSpeed(speedHistory.down[19])}
              </p>
            </div>
            <p
              className={`text-3xl font-bold truncate w-full ${isProxyDead ? "text-zinc-600" : "text-[#007E3A]"}`}
            >
              {formatBytes(stats.download)}
            </p>
            <SpeedChart
              data={speedHistory.down}
              color={isProxyDead ? "#52525b" : "#007E3A"}
            />
          </div>
          <div className="bg-zinc-900 rounded-3xl p-6 border border-zinc-800 flex flex-col min-w-0 relative">
            <div className="flex justify-between items-start w-full">
              <p className="text-sm text-zinc-500 mb-2 truncate font-bold uppercase tracking-widest">
                {t("home.upload")}
              </p>
              <p
                className={`text-[10px] font-bold ${isProxyDead ? "text-zinc-600" : "text-[#00A819]"}`}
              >
                {formatSpeed(speedHistory.up[19])}
              </p>
            </div>
            <p
              className={`text-3xl font-bold truncate w-full ${isProxyDead ? "text-zinc-600" : "text-[#00A819]"}`}
            >
              {formatBytes(stats.upload)}
            </p>
            <SpeedChart
              data={speedHistory.up}
              color={isProxyDead ? "#52525b" : "#00A819"}
            />
          </div>
        </div>
      )}
    </div>
  );
};
