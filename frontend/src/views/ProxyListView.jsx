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

import React, { useState, useMemo, useRef, useEffect } from "react";
import {
  Activity,
  ListTree,
  Pencil,
  Trash2,
  Search,
  Plus,
  ChevronDown,
  RefreshCw,
  Zap,
  Star,
} from "lucide-react";
import { FlagIcon } from "../components/ui/FlagIcon";
import { HoverMarquee } from "../components/ui/HoverMarquee";
import { useConfigContext } from "../context/ConfigContext";
import { useConnectionContext } from "../context/ConnectionContext";
import { useTranslation } from "react-i18next";
import {
  getProtocolLabel,
  isVpnType,
  formatProxyDisplayName,
  mergeSubscriptionRefreshCountries,
} from "../utils/proxyParser";
import wailsAPI from "../utils/wailsAPI";
import { parseExtra, getPingSortMetric } from "../utils/pingSort";
import impLogo from "../assets/implogo.png";

function formatTrafficBytes(n) {
  if (n == null || Number.isNaN(n)) return "0";
  const v = Number(n);
  if (v < 1024) return `${Math.round(v)} B`;
  const gb = v / 1024 ** 3;
  if (gb >= 1) return `${gb.toFixed(1)} GB`;
  const mb = v / 1024 ** 2;
  if (mb >= 1) return `${mb.toFixed(0)} MB`;
  const kb = v / 1024;
  return `${kb.toFixed(0)} KB`;
}

function subscriptionUsesImpLogo(name, source) {
  if (source === "rvsub") return true;
  return typeof name === "string" && name.toLowerCase().includes("impvpn");
}

function SubscriptionHeaderIcon({ url, subscriptionUrl, name, source }) {
  const useImpLogo = subscriptionUsesImpLogo(name, source);
  const [failed, setFailed] = useState(false);
  const [candidateIndex, setCandidateIndex] = useState(0);

  const candidates = useMemo(() => {
    const out = [];
    const add = (value) => {
      if (!value || out.includes(value)) return;
      out.push(value);
    };
    if (typeof url === "string" && url.startsWith("data:image/")) {
      add(url);
    }
    try {
      const u = new URL(subscriptionUrl || "");
      const base = `${u.protocol}//${u.host}`;
      add(`${base}/assets/favicon-32x32.png`);
      add(`${base}/assets/favicon.ico`);
      add(`${base}/favicon.ico`);
    } catch {}
    return out;
  }, [url, subscriptionUrl]);

  useEffect(() => {
    setFailed(false);
    setCandidateIndex(0);
  }, [url, subscriptionUrl, useImpLogo]);

  if (useImpLogo) {
    return (
      <div className="w-10 h-10 rounded-xl shrink-0 border border-zinc-700/50 bg-zinc-800 flex items-center justify-center">
        <img
          src={impLogo}
          alt=""
          className="w-7 h-7 rounded-lg object-contain"
        />
      </div>
    );
  }

  const src = candidates[candidateIndex] || "";
  if (!src) return null;
  return (
    <div className="w-10 h-10 rounded-xl shrink-0 border border-zinc-700/50 bg-zinc-800 flex items-center justify-center">
      {failed ? (
        <Activity className="w-5 h-5 text-zinc-500" />
      ) : (
        <img
          src={src}
          alt=""
          className="w-7 h-7 rounded-lg object-contain"
          onError={() => {
            if (candidateIndex + 1 < candidates.length) {
              setCandidateIndex((i) => i + 1);
            } else {
              setFailed(true);
            }
          }}
        />
      )}
    </div>
  );
}

export const ProxyListView = () => {
  const { t, i18n } = useTranslation();
  const [searchQuery, setSearchQuery] = useState("");
  const [sortBy, setSortBy] = useState("default");
  const [isSortOpen, setIsSortOpen] = useState(false);
  const [refreshingProvider, setRefreshingProvider] = useState(null);
  const [deletingSubId, setDeletingSubId] = useState(null);
  const [collapsedGroups, setCollapsedGroups] = useState({});
  const sortRef = useRef(null);

  const {
    proxies,
    setProxies,
    setEditingProxy,
    setActiveTab,
    subscriptions,
    setSubscriptions,
    showConfirmDialog,
    settings,
    toggleFavorite,
  } = useConfigContext();
  const favoriteIds = useMemo(
    () => new Set((settings?.favorites || []).map(String)),
    [settings?.favorites],
  );
  const {
    deleteProxy: performDelete,
    selectAndConnect,
    disconnectOnly,
    activeProxy,
    isConnected,
    isConnecting,
    failedProxy,
    pings,
    refreshPings,
    isPinging,
    isManualPinging,
    isPingPending,
  } = useConnectionContext();

  useEffect(() => {
    function handleClickOutside(event) {
      if (sortRef.current && !sortRef.current.contains(event.target)) {
        setIsSortOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  const autoMemberIds = useMemo(() => {
    const ids = new Set();
    proxies.forEach((p) => {
      if (p.type?.toUpperCase() === "AUTO") {
        const extra = parseExtra(p.extra);
        (extra?.members || []).forEach((id) => ids.add(String(id)));
      }
    });
    return ids;
  }, [proxies]);

  const filteredAndSortedProxies = useMemo(() => {
    let result = proxies.filter((p) => !autoMemberIds.has(String(p.id)));
    if (searchQuery) {
      const q = searchQuery.toLowerCase();
      result = result.filter(
        (p) =>
          p.name.toLowerCase().includes(q) || p.ip.toLowerCase().includes(q),
      );
    }
    if (sortBy === "country") {
      result.sort((a, b) => (a.country || "").localeCompare(b.country || ""));
    } else if (sortBy === "type") {
      result.sort((a, b) => (a.type || "").localeCompare(b.type || ""));
    } else if (sortBy === "newest") {
      result.reverse();
    } else if (sortBy === "provider") {
      result.sort((a, b) => (a.provider || "").localeCompare(b.provider || ""));
    } else if (sortBy === "ping") {
      result.sort(
        (a, b) => getPingSortMetric(a, pings) - getPingSortMetric(b, pings),
      );
    }
    return result;
  }, [proxies, searchQuery, sortBy, autoMemberIds, pings]);

  const groupedProxies = useMemo(() => {
    const groups = {};
    filteredAndSortedProxies.forEach((proxy) => {
      const key = proxy.provider || t("proxyList.myProxies") || "Мои прокси";
      if (!groups[key]) groups[key] = [];
      groups[key].push(proxy);
    });
    const myKey = t("proxyList.myProxies") || "Мои прокси";
    const entries = Object.entries(groups);
    entries.sort(([a], [b]) => {
      if (a === myKey) return 1;
      if (b === myKey) return -1;
      return a.localeCompare(b);
    });
    return entries.map(([name, list]) => {
      const sorted = [...list].sort((a, b) => {
        const af = favoriteIds.has(String(a.id)) ? 0 : 1;
        const bf = favoriteIds.has(String(b.id)) ? 0 : 1;
        return af - bf;
      });
      return [name, sorted];
    });
  }, [filteredAndSortedProxies, t, favoriteIds]);

  const hasProviders = useMemo(
    () => proxies.some((p) => p.provider),
    [proxies],
  );

  const myProxiesLabel = t("proxyList.myProxies") || "Мои прокси";

  const displayGroups = useMemo(() => {
    if (proxies.length === 0) {
      return [[myProxiesLabel, []]];
    }
    if (!hasProviders) {
      const sorted = [...filteredAndSortedProxies].sort((a, b) => {
        const af = favoriteIds.has(String(a.id)) ? 0 : 1;
        const bf = favoriteIds.has(String(b.id)) ? 0 : 1;
        return af - bf;
      });
      return [[myProxiesLabel, sorted]];
    }
    const entries = [...groupedProxies];
    if (!entries.some(([name]) => name === myProxiesLabel)) {
      entries.push([myProxiesLabel, []]);
    }
    return entries;
  }, [
    proxies.length,
    hasProviders,
    groupedProxies,
    filteredAndSortedProxies,
    myProxiesLabel,
    favoriteIds,
  ]);

  const editProxy = (proxy) => {
    setEditingProxy(proxy);
    setActiveTab("add");
  };

  const deleteProxy = (id) => performDelete(id, setProxies);

  const toggleGroup = (key) => {
    setCollapsedGroups((prev) => ({ ...prev, [key]: !prev[key] }));
  };

  const handleRefreshProvider = async (providerName, groupSubUrl) => {
    if (!subscriptions) return;
    const sub = groupSubUrl
      ? subscriptions.find((s) => s.url === groupSubUrl)
      : subscriptions.find((s) => s.name === providerName);
    if (!sub) return;

    setRefreshingProvider(sub.id);
    try {
      const updated = await wailsAPI.refreshSubscription(sub.id);
      if (updated?.length) {
        setProxies((prev) => {
          const filtered = prev.filter((p) => p.subscriptionUrl !== sub.url);
          const merged = mergeSubscriptionRefreshCountries(
            prev,
            updated,
            sub.url,
          );
          return [...filtered, ...merged];
        });
      }
      const cfg = await wailsAPI.getConfig();
      if (cfg?.subscriptions) setSubscriptions(cfg.subscriptions);
    } catch (err) {
      console.error("Refresh error:", err);
    } finally {
      setRefreshingProvider(null);
    }
  };

  const handleDeleteSubscription = async (subMeta) => {
    if (!subMeta?.id) return;
    const ok = await showConfirmDialog({
      title: t("common.confirmAction"),
      message: t("proxyList.confirmDeleteSubscription", {
        name: subMeta.name || subMeta.url,
      }),
      variant: "danger",
      confirmText: t("common.delete"),
      cancelText: t("common.cancel"),
    });
    if (!ok) return;
    setDeletingSubId(subMeta.id);
    try {
      await wailsAPI.deleteSubscription(subMeta.id);
      setProxies((prev) =>
        prev.filter((p) => p.subscriptionUrl !== subMeta.url),
      );
      const cfg = await wailsAPI.getConfig();
      if (cfg?.subscriptions) setSubscriptions(cfg.subscriptions);
    } catch (err) {
      console.error("Delete subscription error:", err);
    } finally {
      setDeletingSubId(null);
    }
  };

  const handleDeleteManualGroup = async (groupProxies) => {
    if (!groupProxies?.length) return;
    const ok = await showConfirmDialog({
      title: t("common.confirmAction"),
      message: t("proxyList.confirmDeleteMyProxies", {
        count: groupProxies.length,
      }),
      variant: "danger",
      confirmText: t("common.delete"),
      cancelText: t("common.cancel"),
    });
    if (!ok) return;
    const list = [...groupProxies];
    for (const p of list) {
      await deleteProxy(p.id);
    }
  };

  const handleCardConnect = (proxy) => {
    if (failedProxy && failedProxy.id === proxy.id) {
      disconnectOnly();
      return;
    }
    selectAndConnect(proxy);
  };

  const renderProxyCard = (proxy) => {
    const isActive = isConnected && activeProxy?.id === proxy.id;
    const isCardConnecting = isConnecting && activeProxy?.id === proxy.id;
    const isCardFailed = !!failedProxy && failedProxy.id === proxy.id;
    const isFromSubscription = Boolean(proxy.subscriptionUrl);
    const isAutoProxy = proxy.type?.toUpperCase() === "AUTO";
    const isSectionProxy = proxy.type?.toUpperCase() === "SECTION";
    const isFav = favoriteIds.has(String(proxy.id));
    const protocolInfo = isVpnType(proxy.type)
      ? getProtocolLabel(proxy)
      : proxy.type;
    const protocolLabel = isAutoProxy
      ? t("proxyList.autoType")
      : isSectionProxy
        ? t("proxyList.sectionType")
        : protocolInfo;

    // For AUTO cards: compute best ping from member servers.
    const autoBestPing = isAutoProxy
      ? (() => {
          const extra = parseExtra(proxy.extra);
          const memberIds = (extra?.members || []).map(String);
          const arr = memberIds
            .map((id) => pings[id])
            .filter((p) => p && /^\d+/.test(String(p)));
          return arr.length
            ? arr.sort((a, b) => parseInt(a, 10) - parseInt(b, 10))[0]
            : null;
        })()
      : null;

    const pingPending = isPingPending(proxy);

    const pingDisplay = pingPending
      ? t("proxyList.pinging")
      : isAutoProxy
        ? (autoBestPing ?? t("proxyList.pinging"))
        : pings[proxy.id] || t("proxyList.pinging");

    const pingIsError =
      !pingPending &&
      !isAutoProxy &&
      !isSectionProxy &&
      ["Timeout", "Error", "Unavailable", "Refused"].includes(pings[proxy.id]);

    const ipDisplay = isFromSubscription ? null : `${proxy.ip}:${proxy.port}`;

    const inactiveCardBorder = isSectionProxy
      ? "border-zinc-800 hover:border-zinc-600 hover:bg-zinc-800/20"
      : "border-zinc-800 hover:border-[#00A819] hover:bg-zinc-800/30";

    return (
      <div
        key={proxy.id}
        onClick={() => handleCardConnect(proxy)}
        className={`bg-zinc-900 p-4 rounded-[12px] border transition-all flex flex-col cursor-pointer group/card outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${isCardConnecting ? "border-amber-500 shadow-[0_0_20px_rgba(245,158,11,0.15)]" : isCardFailed ? "border-rose-500 shadow-[0_0_20px_rgba(244,63,94,0.15)]" : isActive ? "border-[#00A819] shadow-[0_0_20px_rgba(0,168,25,0.1)]" : inactiveCardBorder}`}
      >
        <div className="flex items-start gap-3 mb-4">
          <div
            className={`shrink-0 flex items-center justify-center w-10 h-10 rounded-xl border ${isSectionProxy ? "bg-violet-950/40 border-violet-800/50" : "bg-zinc-800/50 border-zinc-700/50"}`}
          >
            {isAutoProxy ? (
              <Zap className="w-5 h-5 text-[#00A819]" />
            ) : isSectionProxy ? (
              <ListTree className="w-5 h-5 text-violet-300/90" />
            ) : (
              <FlagIcon code={proxy.country} className="w-6 rounded-[2px]" />
            )}
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <h3
                className={`text-sm font-bold text-white transition-colors min-w-0 flex-1 ${isSectionProxy ? "group-hover/card:text-violet-200" : "group-hover/card:text-[#00A819]"}`}
              >
                <HoverMarquee
                  text={formatProxyDisplayName(proxy.name, proxy.country)}
                />
              </h3>
              <span className="text-[10px] font-medium px-1.5 py-0.5 rounded shrink-0 whitespace-nowrap bg-zinc-800 text-zinc-300 border border-zinc-700/60">
                {protocolLabel}
              </span>
            </div>
            {isSectionProxy ? (
              <p className="text-xs text-zinc-400 mt-1 leading-snug">
                {t("proxyList.sectionSubtitle")}
              </p>
            ) : (
              ipDisplay && (
                <p className="text-xs text-zinc-400 font-mono mt-1 truncate">
                  {ipDisplay}
                </p>
              )
            )}
          </div>
        </div>

        <div className="flex items-center justify-between mt-auto pt-2 gap-2">
          <div
            className={`text-xs flex items-center shrink-0 ${pingIsError ? "text-rose-500" : "text-zinc-500"}`}
            title={isSectionProxy ? undefined : t("proxyList.pingTooltip")}
          >
            {isSectionProxy ? (
              <ListTree
                className="w-3.5 h-3.5 shrink-0 text-violet-400/80"
                aria-label={t("proxyList.sectionType")}
              />
            ) : (
              <>
                <Activity className="w-3.5 h-3.5 mr-1 shrink-0" />
                {pingDisplay}
              </>
            )}
          </div>
          <div className="flex space-x-1.5 shrink-0">
            {!isSectionProxy && (
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  toggleFavorite(proxy.id);
                }}
                title={
                  isFav
                    ? t("proxyList.unfavoriteAria")
                    : t("proxyList.favoriteAria")
                }
                aria-label={
                  isFav
                    ? t("proxyList.unfavoriteAria")
                    : t("proxyList.favoriteAria")
                }
                className={`p-2 rounded-xl transition-colors shrink-0 border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${isFav ? "bg-amber-400/15 text-amber-400 hover:bg-amber-400/25" : "bg-zinc-800 text-zinc-400 hover:text-amber-400 hover:bg-amber-400/10"}`}
              >
                <Star
                  className={`w-3.5 h-3.5 ${isFav ? "fill-amber-400" : ""}`}
                />
              </button>
            )}
            {!isFromSubscription && (
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  editProxy(proxy);
                }}
                className="p-2 bg-zinc-800 text-zinc-400 hover:text-white hover:bg-zinc-700 rounded-xl transition-colors shrink-0 border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
              >
                <Pencil className="w-3.5 h-3.5" />
              </button>
            )}
            <button
              onClick={(e) => {
                e.stopPropagation();
                deleteProxy(proxy.id);
              }}
              className="p-2 bg-zinc-800 text-zinc-400 hover:text-rose-500 hover:bg-rose-500/10 rounded-xl transition-colors shrink-0 border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
            >
              <Trash2 className="w-3.5 h-3.5" />
            </button>
            {!isSectionProxy && (
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  handleCardConnect(proxy);
                }}
                className={`px-3 py-1.5 rounded-xl text-xs font-medium transition-colors shrink-0 border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${isCardConnecting ? "bg-amber-500/15 text-amber-400 font-bold" : isCardFailed ? "bg-rose-500/15 text-rose-400 font-bold hover:bg-rose-500/25" : isActive ? "bg-[#00A819] text-zinc-950 font-bold" : "bg-[#007E3A]/10 text-[#00A819] hover:bg-[#007E3A]/20"}`}
              >
                {isCardConnecting
                  ? t("proxyList.status.connecting")
                  : isCardFailed
                    ? t("proxyList.status.disconnect")
                    : isActive
                      ? t("proxyList.status.connected")
                      : t("proxyList.status.connect")}
              </button>
            )}
          </div>
        </div>
      </div>
    );
  };

  return (
    <div className="space-y-6 animate-in fade-in duration-300">
      <div>
        <h2 className="text-3xl font-bold text-white">
          {t("proxyList.title")}
        </h2>
        <p className="text-zinc-400 mt-2">{t("proxyList.desc")}</p>
      </div>

      {proxies.length > 0 && (
        <div className="flex flex-col sm:flex-row gap-4">
          <div className="relative flex-1 text-white">
            <Search className="absolute left-4 top-1/2 -translate-y-1/2 w-5 h-5 text-zinc-500" />
            <input
              type="text"
              placeholder={t("proxyList.searchPlaceholder")}
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="w-full bg-zinc-800 border-none text-white rounded-xl py-3 pl-12 pr-4 outline-none focus:ring-2 focus:ring-[#00A819]/50 transition-all placeholder:text-zinc-500"
            />
          </div>
          <div className="flex items-center gap-3 shrink-0" ref={sortRef}>
            <div className="relative">
              <button
                onClick={() => setIsSortOpen(!isSortOpen)}
                className="flex items-center justify-between bg-zinc-800 border-none text-white rounded-xl py-3 px-4 outline-none focus:ring-2 focus:ring-[#00A819]/50 transition-all cursor-pointer min-w-[160px]"
              >
                <span>{t(`proxyList.sort.${sortBy}`)}</span>
                <ChevronDown
                  className={`w-4 h-4 text-zinc-400 transition-transform ${isSortOpen ? "" : "-rotate-90"}`}
                />
              </button>

              {isSortOpen && (
                <div className="absolute top-full left-0 right-0 mt-2 bg-zinc-900 border border-zinc-700/50 rounded-xl shadow-xl overflow-hidden z-10 animate-in slide-in-from-top-2 duration-200">
                  {[
                    "default",
                    "newest",
                    "oldest",
                    "country",
                    "type",
                    ...(hasProviders ? ["provider"] : []),
                    "ping",
                  ].map((option) => (
                    <button
                      key={option}
                      onClick={() => {
                        setSortBy(option);
                        setIsSortOpen(false);
                      }}
                      className={`w-full text-left px-4 py-3 text-sm transition-colors ${sortBy === option ? "bg-[#00A819]/10 text-[#00A819]" : "text-zinc-300 hover:bg-zinc-800 hover:text-white"}`}
                    >
                      {t(`proxyList.sort.${option}`)}
                    </button>
                  ))}
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {proxies.length > 0 && filteredAndSortedProxies.length === 0 ? (
        <div className="text-center py-16 bg-zinc-900 rounded-3xl border border-zinc-800 border-dashed">
          <Search className="w-16 h-16 text-zinc-700 mx-auto mb-4" />
          <p className="text-zinc-400 text-lg">{t("proxyList.noResults")}</p>
        </div>
      ) : (
        <div className="space-y-6">
          {displayGroups.map(([groupName, groupProxies]) => {
            const isMyProxies = groupName === myProxiesLabel;
            const isCollapsed = collapsedGroups[groupName];
            const groupSubUrl = !isMyProxies
              ? groupProxies.find((p) => p.subscriptionUrl)?.subscriptionUrl
              : null;
            const subMeta = groupSubUrl
              ? subscriptions?.find((s) => s.url === groupSubUrl)
              : subscriptions?.find((s) => s.name === groupName);
            const isSub = Boolean(subMeta);
            const subBusy =
              isSub &&
              (refreshingProvider === subMeta?.id ||
                deletingSubId === subMeta?.id);

            const usedBytes =
              (subMeta?.trafficUpload ?? 0) + (subMeta?.trafficDownload ?? 0);
            const totalBytes = subMeta?.trafficTotal ?? 0;
            const usedTrafficStr = formatTrafficBytes(usedBytes);
            const totalTrafficStr =
              totalBytes > 0
                ? formatTrafficBytes(totalBytes)
                : t("proxyList.subUnlimited");
            let expireLine = null;
            if (isSub && subMeta?.expireUnix > 0) {
              const d = new Date(subMeta.expireUnix * 1000);
              const dd = String(d.getDate()).padStart(2, "0");
              const mm = String(d.getMonth() + 1).padStart(2, "0");
              const yy = String(d.getFullYear()).slice(-2);
              const hh = String(d.getHours()).padStart(2, "0");
              const min = String(d.getMinutes()).padStart(2, "0");
              expireLine = `до ${dd}.${mm}.${yy}. в ${hh}:${min}`;
            }

            return (
              <div key={groupName} className="space-y-4">
                <div className="flex flex-col gap-2 w-full">
                  <div className="flex items-stretch gap-3 w-full">
                    <button
                      type="button"
                      onClick={() => toggleGroup(groupName)}
                      className="group/hdr flex h-14 min-w-0 flex-1 items-center gap-3 overflow-hidden rounded-xl border border-zinc-700/60 bg-zinc-800/70 px-3 text-left outline-none transition-colors hover:border-zinc-600/80 focus:outline-none focus:ring-2 focus:ring-[#00A819]/25 focus-visible:outline-none"
                    >
                      {isSub && (
                        <SubscriptionHeaderIcon
                          key={`${subMeta?.id}-${subMeta?.iconUrl || ""}-${subMeta?.source || ""}-${subMeta?.name || ""}`}
                          url={subMeta?.iconUrl}
                          subscriptionUrl={subMeta?.url}
                          name={subMeta?.name}
                          source={subMeta?.source}
                        />
                      )}
                      <div className="flex min-w-0 flex-1 flex-col justify-center gap-0.5 overflow-hidden py-1">
                        <h3 className="truncate text-lg font-bold leading-tight text-white transition-colors group-hover/hdr:text-zinc-100">
                          {groupName}
                        </h3>
                        {isSub && expireLine && (
                          <span className="truncate text-[11px] leading-tight text-zinc-500">
                            {expireLine}
                          </span>
                        )}
                      </div>
                      <div className="flex shrink-0 items-center gap-2 pl-1">
                        {isSub && (
                          <span className="inline-flex shrink-0 items-center rounded-full border border-zinc-700/70 bg-zinc-900/80 px-2.5 py-0.5 text-xs font-medium whitespace-nowrap">
                            <span className="text-zinc-300">
                              {usedTrafficStr}
                            </span>
                            <span className="text-zinc-500">/</span>
                            <span className="text-zinc-400">
                              {totalTrafficStr}
                            </span>
                          </span>
                        )}
                        <span className="shrink-0 rounded-lg border border-zinc-700/50 bg-zinc-900/90 px-2 py-0.5 text-xs text-zinc-400">
                          {groupProxies.length}
                        </span>
                        <ChevronDown
                          className={`h-4 w-4 shrink-0 text-zinc-400 transition-transform group-hover/hdr:text-zinc-200 ${isCollapsed ? "-rotate-90" : ""}`}
                          aria-hidden
                        />
                      </div>
                    </button>
                    {isSub && (
                      <>
                        <button
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation();
                            handleRefreshProvider(groupName, groupSubUrl);
                          }}
                          disabled={subBusy}
                          title={t("proxyList.refreshSubAria")}
                          aria-label={t("proxyList.refreshSubAria")}
                          className="flex h-14 w-14 shrink-0 items-center justify-center rounded-xl border border-zinc-700/60 bg-zinc-800/50 text-zinc-300 hover:text-white hover:border-zinc-600 transition-colors outline-none focus:outline-none focus:ring-2 focus:ring-[#00A819]/25 disabled:opacity-50"
                        >
                          <RefreshCw
                            className={`h-5 w-5 text-[#00A819] ${refreshingProvider === subMeta?.id ? "animate-spin" : ""}`}
                          />
                        </button>
                        <button
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation();
                            refreshPings(groupProxies.map((p) => p.id));
                          }}
                          disabled={subBusy || isPinging}
                          title={t("proxyList.manualPingAria")}
                          aria-label={t("proxyList.manualPingAria")}
                          className="flex h-14 w-14 shrink-0 items-center justify-center rounded-xl border border-zinc-700/60 bg-zinc-800/50 text-zinc-300 hover:text-white hover:border-violet-500/40 transition-colors outline-none focus:outline-none focus:ring-2 focus:ring-violet-500/25 disabled:opacity-50"
                        >
                          <Activity
                            className={`h-5 w-5 text-violet-400 ${isManualPinging ? "animate-pulse" : ""}`}
                          />
                        </button>
                        <button
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation();
                            handleDeleteSubscription(subMeta);
                          }}
                          disabled={subBusy}
                          title={t("proxyList.deleteSubscriptionAria")}
                          aria-label={t("proxyList.deleteSubscriptionAria")}
                          className="flex h-14 w-14 shrink-0 items-center justify-center rounded-xl border border-rose-500/40 bg-rose-500/15 text-rose-400 hover:bg-rose-500/25 hover:text-rose-300 transition-colors outline-none focus:outline-none focus:ring-2 focus:ring-rose-500/30 disabled:opacity-50"
                        >
                          <Trash2 className="h-5 w-5" />
                        </button>
                      </>
                    )}
                    {isMyProxies && groupProxies.length > 0 && (
                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation();
                          handleDeleteManualGroup(groupProxies);
                        }}
                        title={t("proxyList.deleteManualGroupAria")}
                        aria-label={t("proxyList.deleteManualGroupAria")}
                        className="flex h-14 w-14 shrink-0 items-center justify-center rounded-xl border border-rose-500/40 bg-rose-500/15 text-rose-400 hover:bg-rose-500/25 hover:text-rose-300 transition-colors outline-none focus:outline-none focus:ring-2 focus:ring-rose-500/30"
                      >
                        <Trash2 className="h-5 w-5" />
                      </button>
                    )}
                  </div>
                  {isSub && (
                    <div className="flex flex-col gap-1.5 pl-0">
                      <p className="text-[11px] text-zinc-500 leading-snug">
                        {t("proxyList.subRefreshHint")}
                      </p>
                    </div>
                  )}
                </div>

                {!isCollapsed && (
                  <div className="grid gap-6 grid-cols-1 sm:grid-cols-[repeat(auto-fit,minmax(320px,1fr))]">
                    {isMyProxies && (
                      <div
                        onClick={() => setActiveTab("add")}
                        className="bg-zinc-900/50 p-6 rounded-3xl border border-zinc-800 hover:border-[#00A819] hover:bg-zinc-800/30 border-dashed transition-all flex flex-col items-center justify-center cursor-pointer group/card outline-none focus:outline-none focus:ring-0 focus-visible:outline-none min-h-[160px]"
                      >
                        <div className="w-12 h-12 rounded-xl bg-zinc-800/50 border border-zinc-700/50 flex items-center justify-center mb-4 group-hover/card:bg-[#00A819]/10 group-hover/card:border-[#00A819]/30 transition-colors">
                          <Plus className="w-6 h-6 text-zinc-400 group-hover/card:text-[#00A819] transition-colors" />
                        </div>
                        <h3 className="text-lg font-bold text-zinc-400 group-hover/card:text-white transition-colors">
                          {t("add.newServer")}
                        </h3>
                      </div>
                    )}
                    {groupProxies.map(renderProxyCard)}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
};
