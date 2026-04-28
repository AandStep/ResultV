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

import React, { useState, useMemo, useEffect } from "react";
import {
  Power,
  Plus,
  Globe,
  Pencil,
  ChevronDown,
  Activity,
  Zap,
  Star,
} from "lucide-react";
import { FlagIcon } from "../components/ui/FlagIcon";
import { SpeedChart } from "../components/ui/SpeedChart";
import { useConfigContext } from "../context/ConfigContext";
import { useConnectionContext } from "../context/ConnectionContext";
import { formatBytes, formatSpeed } from "../utils/formatters";
import { useTranslation } from "react-i18next";
import {
  getProtocolLabel,
  isVpnType,
  formatProxyDisplayName,
} from "../utils/proxyParser";

export const HomeView = () => {
  const { t } = useTranslation();
  const { proxies, setEditingProxy, setActiveTab, settings, updateSetting, toggleFavorite, showAlertDialog, isApplyingMode } = useConfigContext();
  const {
    isConnected,
    isConnecting,
    isProxyDead,
    failedProxy,
    setFailedProxy,
    disconnectOnly,
    toggleConnection,
    cancelConnect,
    activeProxy,
    stats,
    speedHistory,
    pings,
    selectAndConnect,
  } = useConnectionContext();

  const isError = !!failedProxy;
  const [isProxyListOpen, setIsProxyListOpen] = useState(false);
  const hasProxies = proxies.length > 0;
  const displayProxy =
    failedProxy ||
    activeProxy ||
    proxies.find((p) => String(p.id) === String(settings?.lastSelectedProxyId)) ||
    proxies[0];

  useEffect(() => {
    const isEndpointProtocol = displayProxy && ["WIREGUARD", "AMNEZIAWG"].includes(String(displayProxy.type).toUpperCase());
    if (isEndpointProtocol && settings?.mode !== "tunnel") {
      updateSetting("mode", "tunnel");
    }
  }, [displayProxy, settings?.mode, updateSetting]);

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

  const autoMemberIds = useMemo(() => {
    const ids = new Set();
    proxies.forEach((p) => {
      if (p.type?.toUpperCase() === "AUTO") {
        let extra = {};
        if (typeof p.extra === "string") {
            try { extra = JSON.parse(p.extra); } catch {}
        } else if (p.extra) {
            extra = p.extra;
        }
        (extra?.members || []).forEach((id) => ids.add(String(id)));
      }
    });
    return ids;
  }, [proxies]);

  const filteredProxies = useMemo(() => {
      return proxies.filter(p => !autoMemberIds.has(String(p.id)));
  }, [proxies, autoMemberIds]);

  const favoriteIds = useMemo(
    () => new Set((settings?.favorites || []).map(String)),
    [settings?.favorites],
  );

  const favoriteProxies = useMemo(
    () => filteredProxies.filter((p) => favoriteIds.has(String(p.id))),
    [filteredProxies, favoriteIds],
  );

  const nonFavoriteProxies = useMemo(
    () => filteredProxies.filter((p) => !favoriteIds.has(String(p.id))),
    [filteredProxies, favoriteIds],
  );

  const groupedByProvider = useMemo(() => {
    const groups = {};
    nonFavoriteProxies.forEach((proxy) => {
      const providerKey = proxy.provider || t("proxyList.myProxies") || "Мои прокси";
      if (!groups[providerKey]) groups[providerKey] = {};
      const countryCode = proxy.type?.toUpperCase() === "AUTO" ? "Auto" : (proxy.country || "Unknown");
      if (!groups[providerKey][countryCode]) groups[providerKey][countryCode] = [];
      groups[providerKey][countryCode].push(proxy);
    });

    const myKey = t("proxyList.myProxies") || "Мои прокси";
    return Object.entries(groups)
      .sort(([a], [b]) => {
        if (a === myKey) return 1;
        if (b === myKey) return -1;
        return a.localeCompare(b);
      })
      .map(([provider, countries]) => ({
        provider,
        countries: Object.entries(countries).sort(([a], [b]) => {
          if (a === "Auto") return -1;
          if (b === "Auto") return 1;
          return a.localeCompare(b);
        }),
      }));
  }, [nonFavoriteProxies, t]);

  const hasProviders = proxies.some((p) => p.provider);
  
  const statsFillRemainingHeight = isConnected && !(hasProxies && isProxyListOpen);

  return (
    <div className="flex min-h-full w-full flex-col items-center gap-5 animate-in fade-in zoom-in-95 duration-300">
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
        {!(isConnected && !isProxyDead) && (
          <p className="text-zinc-400">
            {isConnected
              ? t("home.desc.lost")
              : isError
                ? t("home.desc.error")
                : t("home.desc.unprotected")}
          </p>
        )}
      </div>

      <div className="relative group my-3">
        <div
          className={`absolute inset-0 rounded-full blur-2xl transition-all duration-700 ${isConnected ? (isProxyDead ? "bg-rose-500/40 animate-pulse" : "bg-[#007E3A]/40") : isError ? "bg-rose-500/20 animate-pulse" : hasProxies ? "bg-zinc-800/10 group-hover:bg-zinc-800/20" : ""}`}
        ></div>
        <button
          disabled={!hasProxies && !isConnected && !isConnecting}
          onClick={
            isConnecting
              ? cancelConnect
              : isError
              ? disconnectOnly
              : toggleConnection
          }
          className={`relative border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none flex items-center justify-center w-48 h-48 rounded-full transition-all duration-300 transform active:scale-95 ${
            isConnecting
              ? "bg-gradient-to-br from-zinc-800 to-zinc-900 border-4 border-amber-500 text-amber-400 shadow-2xl shadow-amber-500/30 scale-95 hover:border-rose-500 hover:text-rose-400 cursor-pointer"
              : !hasProxies && !isConnected
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

      {}
      <div className="flex items-center bg-zinc-900 rounded-full p-1 border border-zinc-800">
        <button
          disabled={isApplyingMode}
          onClick={() => {
            const isEndpointProtocol = displayProxy && ["WIREGUARD", "AMNEZIAWG"].includes(String(displayProxy.type).toUpperCase());
            if (isEndpointProtocol) {
              showAlertDialog({
                title: t("common.notice") || "Внимание",
                message: "Протоколы WireGuard и AmneziaWG не поддерживают Proxy-режим. Доступен только Туннель.",
                variant: "warning",
              });
            } else {
              updateSetting("mode", "proxy");
            }
          }}
          className={`px-6 py-2 rounded-full text-sm font-bold transition-all border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${
            isApplyingMode ? "opacity-60 cursor-wait " : ""
          }${
            displayProxy && ["WIREGUARD", "AMNEZIAWG"].includes(String(displayProxy.type).toUpperCase())
              ? "text-zinc-600 cursor-not-allowed"
              : settings.mode === "proxy"
                ? "bg-[#007E3A] text-white"
                : "text-zinc-400 hover:text-white"
          }`}
        >
          {t("home.modeProxy") || "ПРОКСИ"}
        </button>
        <button
          disabled={isApplyingMode}
          onClick={() => updateSetting("mode", "tunnel")}
          className={`px-6 py-2 rounded-full text-sm font-bold transition-all border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${
            isApplyingMode ? "opacity-60 cursor-wait " : ""
          }${
            settings.mode === "tunnel"
              ? "bg-[#007E3A] text-white"
              : "text-zinc-400 hover:text-white"
          }`}
        >
          {t("home.modeTunnel") || "ТУННЕЛЬ"}
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
            className="w-full bg-zinc-900 border border-dashed border-zinc-700 rounded-3xl p-6 flex flex-col items-center justify-center cursor-pointer hover:border-[#007E3A] hover:bg-zinc-800/50 transition-all group outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
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
          className={`w-full bg-zinc-900 rounded-3xl border flex flex-col transition-all outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${isProxyListOpen ? "overflow-visible" : "overflow-hidden"} ${(isProxyDead && isConnected) || isError ? "border-rose-500/30" : isProxyListOpen ? "border-zinc-700" : "border-zinc-800 hover:border-[#007E3A] hover:bg-zinc-800/50"}`}
        >
          <div
            onClick={() => setIsProxyListOpen(!isProxyListOpen)}
            className="p-5 flex items-center justify-between cursor-pointer transition-all group"
          >
            <div className="flex items-center space-x-5 min-w-0">
              <div
                className={`w-12 h-12 flex items-center justify-center rounded-2xl shrink-0 transition-colors ${isConnected ? (isProxyDead ? "bg-rose-500/20 text-rose-500" : "bg-[#007E3A]/20 text-[#007E3A]") : isError ? "bg-rose-500/10 text-rose-500" : "bg-zinc-800 text-zinc-500 group-hover:bg-zinc-700"}`}
              >
                {displayProxy ? (
                  displayProxy.type?.toUpperCase() === "AUTO" ? (
                    <Zap className="w-[1.65rem] h-[1.65rem] text-[#00A819]" />
                  ) : (
                    <FlagIcon
                      code={displayProxy.country}
                      className="w-[1.65rem] rounded-sm shadow-sm"
                    />
                  )
                ) : (
                  <Globe className="w-[1.65rem] h-[1.65rem]" />
                )}
              </div>
              <div className="min-w-0">
                <p className="text-xs text-zinc-400 mb-1 truncate leading-tight">
                  {t("home.currentServer")}
                </p>
                <p className="text-base font-bold text-white truncate leading-tight">
                  {displayProxy
                    ? formatProxyDisplayName(displayProxy.name, displayProxy.country)
                    : t("home.emptyServer")}
                </p>
                {displayProxy && !displayProxy.subscriptionUrl && (
                  <p className="text-xs text-zinc-500 font-mono mt-1 truncate leading-tight">
                    {displayProxy.ip}:{displayProxy.port} ({isVpnType(displayProxy.type) ? getProtocolLabel(displayProxy) : displayProxy.type})
                  </p>
                )}
              </div>
            </div>

            <div className="flex items-center space-x-1 shrink-0 ml-4">
              {displayProxy && !displayProxy.subscriptionUrl && (
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    onEditProxy(displayProxy);
                  }}
                  className={`p-2 rounded-xl transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${(isProxyDead && isConnected) || isError ? "text-rose-500/50 hover:text-rose-500 hover:bg-rose-500/10" : "text-zinc-600 hover:text-[#007E3A] hover:bg-[#007E3A]/10"}`}
                >
                  <Pencil className="w-[1.05rem] h-[1.05rem]" />
                </button>
              )}
              <div
                className={`p-2 rounded-xl transition-colors text-zinc-500 group-hover:text-zinc-300`}
              >
                <ChevronDown
                  className={`w-[1.05rem] h-[1.05rem] transition-transform duration-300 ${isProxyListOpen ? "rotate-180" : ""}`}
                />
              </div>
            </div>
          </div>

          {isProxyListOpen && (
            <div className="space-y-2 border-t border-zinc-800/50 bg-zinc-950/50 p-2 animate-in slide-in-from-top-2 duration-200">
              {favoriteProxies.length > 0 && (
                <div className="space-y-1 mb-2">
                  <div className="flex items-center px-3 py-1 space-x-2">
                    <Star className="w-4 h-4 text-amber-400 fill-amber-400" />
                    <span className="text-[10px] font-bold text-zinc-500 uppercase tracking-wider">
                      {t("home.favorites")}
                    </span>
                  </div>
                  {favoriteProxies.map((proxy) => renderDropdownItem(proxy))}
                </div>
              )}
              {hasProviders ? (
                groupedByProvider.map(({ provider, countries }) => (
                  <div key={provider} className="space-y-1 mb-2 last:mb-0">
                    <div className="px-3 py-1">
                      <span className="text-xs font-bold text-zinc-400 uppercase tracking-wider">
                        {provider}
                      </span>
                    </div>
                    {countries.map(([country, countryProxies]) => (
                      <div key={`${provider}-${country}`} className="space-y-1">
                        <div className="flex items-center px-3 py-0.5 space-x-2">
                          {country === "Auto" ? (
                            <Zap className="w-4 h-4 text-[#00A819]" />
                          ) : (
                            <FlagIcon
                              code={country}
                              className="w-4 h-auto rounded-[2px] opacity-70"
                            />
                          )}
                          <span className="text-[10px] font-bold text-zinc-500 uppercase tracking-wider">
                            {country}
                          </span>
                        </div>
                        {countryProxies.map((proxy) => renderDropdownItem(proxy))}
                      </div>
                    ))}
                  </div>
                ))
              ) : (
                Object.entries(
                  nonFavoriteProxies.reduce((acc, proxy) => {
                    const countryCode = proxy.type?.toUpperCase() === "AUTO" ? "Auto" : (proxy.country || "Unknown");
                    if (!acc[countryCode]) acc[countryCode] = [];
                    acc[countryCode].push(proxy);
                    return acc;
                  }, {}),
                )
                  .sort(([countryA], [countryB]) => {
                    if (countryA === "Auto") return -1;
                    if (countryB === "Auto") return 1;
                    return countryA.localeCompare(countryB);
                  })
                  .map(([country, countryProxies]) => (
                    <div key={country} className="space-y-1 mb-2 last:mb-0">
                      <div className="flex items-center px-3 py-1 space-x-2">
                        {country === "Auto" ? (
                          <Zap className="w-5 h-5 text-[#00A819]" />
                        ) : (
                          <FlagIcon
                            code={country}
                            className="w-5 h-auto rounded-[2px] opacity-70"
                          />
                        )}
                        <span className="text-xs font-bold text-zinc-500 uppercase tracking-wider">
                          {country}
                        </span>
                      </div>
                      {countryProxies.map((proxy) => renderDropdownItem(proxy))}
                    </div>
                  ))
              )}
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
          {displayProxy && !displayProxy.subscriptionUrl && (
            <button
              onClick={() => onEditProxy(displayProxy)}
              className="flex-1 border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none bg-zinc-900 border border-zinc-800 hover:border-zinc-700 text-white py-4 rounded-3xl font-bold transition-colors"
            >
              {t("home.editData")}
            </button>
          )}
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
          className={
            statsFillRemainingHeight
              ? "flex min-h-0 w-full flex-1 flex-col"
              : isConnected
                ? "flex w-full shrink-0 flex-col"
                : "w-full"
          }
        >
          <div
            className={`grid min-h-0 w-full transition-[grid-template-rows] duration-500 ease-in-out motion-reduce:transition-none ${
              isConnected
                ? statsFillRemainingHeight
                  ? "min-h-0 flex-1 grid-rows-[1fr]"
                  : "grid-rows-[1fr]"
                : "grid-rows-[0fr]"
            }`}
          >
            <div
              className={`flex min-h-0 flex-col ${statsFillRemainingHeight ? "h-full overflow-visible" : "overflow-visible"}`}
            >
              <div
                className={`flex w-full gap-6 transition-[opacity,transform] duration-500 ease-out motion-reduce:transition-none ${
                  statsFillRemainingHeight
                    ? "min-h-0 flex-1"
                    : isConnected
                      ? "min-h-0 items-start"
                      : "min-h-0"
                } ${
                  isConnected
                    ? "translate-y-0 opacity-100"
                    : "pointer-events-none -translate-y-2 opacity-0"
                }`}
              >
                <div
                  className={`flex min-w-0 flex-1 flex-col rounded-3xl border border-zinc-800 bg-zinc-900 p-6 ${statsFillRemainingHeight ? "min-h-[10rem]" : "self-start"}`}
                >
                  <div className="flex w-full shrink-0 justify-between items-start">
                    <p className="text-xs text-zinc-500 mb-2 truncate font-bold uppercase tracking-widest leading-tight">
                      {t("home.download")}
                    </p>
                    <p
                      className={`text-[9px] font-bold leading-tight ${isProxyDead ? "text-zinc-600" : "text-[#007E3A]"}`}
                    >
                      {formatSpeed(speedHistory.down[19])}
                    </p>
                  </div>
                  <p
                    className={`text-2xl font-bold truncate w-full leading-tight shrink-0 ${isProxyDead ? "text-zinc-600" : "text-[#007E3A]"}`}
                  >
                    {formatBytes(stats.download)}
                  </p>
                  <div
                    className={
                      statsFillRemainingHeight
                        ? "flex min-h-0 flex-1 flex-col"
                        : "shrink-0 flex flex-col"
                    }
                  >
                    <SpeedChart
                      data={speedHistory.down}
                      color={isProxyDead ? "#52525b" : "#007E3A"}
                      fillHeight={statsFillRemainingHeight}
                    />
                  </div>
                </div>
                <div
                  className={`flex min-w-0 flex-1 flex-col rounded-3xl border border-zinc-800 bg-zinc-900 p-6 ${statsFillRemainingHeight ? "min-h-[10rem]" : "self-start"}`}
                >
                  <div className="flex w-full shrink-0 justify-between items-start">
                    <p className="text-xs text-zinc-500 mb-2 truncate font-bold uppercase tracking-widest leading-tight">
                      {t("home.upload")}
                    </p>
                    <p
                      className={`text-[9px] font-bold leading-tight ${isProxyDead ? "text-zinc-600" : "text-[#00A819]"}`}
                    >
                      {formatSpeed(speedHistory.up[19])}
                    </p>
                  </div>
                  <p
                    className={`text-2xl font-bold truncate w-full leading-tight shrink-0 ${isProxyDead ? "text-zinc-600" : "text-[#00A819]"}`}
                  >
                    {formatBytes(stats.upload)}
                  </p>
                  <div
                    className={
                      statsFillRemainingHeight
                        ? "flex min-h-0 flex-1 flex-col"
                        : "shrink-0 flex flex-col"
                    }
                  >
                    <SpeedChart
                      data={speedHistory.up}
                      color={isProxyDead ? "#52525b" : "#00A819"}
                      fillHeight={statsFillRemainingHeight}
                    />
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );

  function renderDropdownItem(proxy) {
    const isActive = activeProxy?.id === proxy.id;
    const isFav = favoriteIds.has(String(proxy.id));
    return (
      <div
        key={proxy.id}
        onClick={(e) => {
          e.stopPropagation();
          selectAndConnect(proxy);
          setIsProxyListOpen(false);
        }}
        className={`flex items-center justify-between p-3 rounded-[12px] cursor-pointer transition-colors outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${isActive ? "bg-[#007E3A]/10 border border-[#007E3A]/20" : "bg-zinc-900/50 border border-transparent hover:border-[#00A819]/50 hover:bg-zinc-800/80"}`}
      >
        <div className="flex items-center space-x-4 min-w-0">
          <div className="shrink-0 flex items-center justify-center w-10 h-10 bg-zinc-800/50 rounded-lg border border-zinc-700/50">
            {proxy.type?.toUpperCase() === "AUTO" ? (
              <Zap className="w-5 h-5 text-[#00A819]" />
            ) : (
              <FlagIcon
                code={proxy.country}
                className="w-6 h-auto rounded-[2px]"
              />
            )}
          </div>
          <div className="min-w-0">
            <h4
              className={`text-sm font-bold truncate transition-colors ${isActive ? "text-[#00A819]" : "text-white"}`}
            >
              {formatProxyDisplayName(proxy.name, proxy.country)}
            </h4>
            <p className="text-xs text-zinc-500 font-mono mt-0.5 truncate">
              {proxy.subscriptionUrl ? (
                <span className="text-[#007E3A]">
                  {isVpnType(proxy.type) ? getProtocolLabel(proxy) : proxy.type}
                </span>
              ) : (
                <>
                  {proxy.ip}:{proxy.port}
                  {isVpnType(proxy.type) && (
                    <span className="ml-1 text-[#007E3A]">
                      {" "}({getProtocolLabel(proxy)})
                    </span>
                  )}
                </>
              )}
            </p>
          </div>
        </div>
        <div className="flex items-center space-x-3 shrink-0 ml-3">
          <button
            onClick={(e) => {
              e.stopPropagation();
              toggleFavorite(proxy.id);
            }}
            title={isFav ? t("home.unfavorite") : t("home.favorite")}
            aria-label={isFav ? t("home.unfavorite") : t("home.favorite")}
            className={`p-1 rounded-md transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${isFav ? "text-amber-400 hover:text-amber-300" : "text-zinc-600 hover:text-amber-400"}`}
          >
            <Star className={`w-4 h-4 ${isFav ? "fill-amber-400" : ""}`} />
          </button>
          <div
            className={`text-xs flex items-center ${pings[proxy.id] === "Timeout" || pings[proxy.id] === "Error" || pings[proxy.id] === "Unavailable" ? "text-rose-500" : "text-zinc-500"}`}
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
  }
};
