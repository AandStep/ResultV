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

import React, { useEffect, useMemo, useState, useRef } from "react";
import {
  Plus,
  Trash2,
  HelpCircle,
  RefreshCw,
  Pencil,
  ListPlus,
} from "lucide-react";
import { useConfigContext } from "../context/ConfigContext";
import { useTranslation } from "react-i18next";
import { PickAppForWhitelist } from "../../wailsjs/go/main/App";
import wailsAPI from "../utils/wailsAPI";
import RoutingListModal from "../components/ui/RoutingListModal";

const ROUTING_ACTION_STYLES = {
  proxy: "bg-[#007E3A]/10 text-[#00A819] border-[#007E3A]/20",
  direct: "bg-zinc-800 text-zinc-300 border-zinc-700",
  block: "bg-rose-500/10 text-rose-400 border-rose-500/20",
};

// Embedded routing lists ship inside a subscription payload (url: "embedded:<action>")
// rather than being fetched from their own URL. `rl.action` is already populated
// for these rows, so no need to re-derive it from the url here.
const isEmbeddedList = (rl) =>
  typeof rl.url === "string" && rl.url.startsWith("embedded:");

export const RulesView = () => {
  const { t, i18n } = useTranslation();
  const {
    routingRules: rules,
    setRoutingRules: setRules,
    syncRoutingLists,
    settings,
    updateSetting,
    showConfirmDialog,
    platform,
    subscriptions,
  } = useConfigContext();
  const [newDomain, setNewDomain] = useState("");
  const [newApp, setNewApp] = useState("");

  const appFileInputRef = useRef(null);

  const routingLists = rules.routingLists || [];
  const subscriptionNameById = useMemo(() => {
    const map = new Map();
    (subscriptions || []).forEach((s) => map.set(s.id, s.name || s.url));
    return map;
  }, [subscriptions]);
  const [listModal, setListModal] = useState({ isOpen: false, mode: "add", initial: null });
  const [refreshingListId, setRefreshingListId] = useState(null);
  const [busyListId, setBusyListId] = useState(null);
  // Optimistic toggle: the switch flips instantly and shows an "applying"
  // spinner while the tunnel reconnects, instead of a dead greyed-out control.
  const [applyingListId, setApplyingListId] = useState(null);
  const [optimisticEnabled, setOptimisticEnabled] = useState({});
  const [intervalInput, setIntervalInput] = useState(
    settings?.routingListUpdateHours
      ? String(settings.routingListUpdateHours)
      : "24",
  );

  useEffect(() => {
    setIntervalInput(
      settings?.routingListUpdateHours
        ? String(settings.routingListUpdateHours)
        : "24",
    );
  }, [settings?.routingListUpdateHours]);

  const commitInterval = async () => {
    const raw = String(intervalInput || "").trim();
    const n = parseInt(raw, 10);
    if (!Number.isFinite(n) || n < 1) {
      setIntervalInput(
        settings?.routingListUpdateHours
          ? String(settings.routingListUpdateHours)
          : "24",
      );
      return;
    }
    await updateSetting("routingListUpdateHours", n);
  };

  const refreshRoutingListsFromConfig = async () => {
    try {
      const cfg = await wailsAPI.getConfig();
      if (cfg?.routingRules?.routingLists) {
        syncRoutingLists(cfg.routingRules.routingLists);
      }
    } catch (err) {
      console.error("Refresh routing lists error:", err);
    }
  };

  const openAddListModal = () =>
    setListModal({ isOpen: true, mode: "add", initial: null });
  const openEditListModal = (rl) =>
    setListModal({ isOpen: true, mode: "edit", initial: rl });
  const closeListModal = () =>
    setListModal({ isOpen: false, mode: "add", initial: null });

  const handleListModalSubmit = async (payload) => {
    if (listModal.mode === "edit" && listModal.initial) {
      await wailsAPI.updateRoutingList({
        ...listModal.initial,
        name: payload.name,
        action: payload.action,
      });
    } else {
      await wailsAPI.addRoutingList(
        payload.name,
        payload.url,
        payload.action,
        payload.allowInsecure,
      );
    }
    await refreshRoutingListsFromConfig();
  };

  const handleToggleListEnabled = async (rl) => {
    const next = !rl.enabled;
    // Flip the switch immediately (optimistic) and lock the row while the
    // backend rebuilds the route and reconnects; reconcile to the real config
    // afterwards so a failed apply visibly snaps back.
    setOptimisticEnabled((m) => ({ ...m, [rl.id]: next }));
    setApplyingListId(rl.id);
    setBusyListId(rl.id);
    try {
      await wailsAPI.updateRoutingList({ ...rl, enabled: next });
    } catch (err) {
      console.error("Toggle routing list error:", err);
    } finally {
      await refreshRoutingListsFromConfig();
      setOptimisticEnabled((m) => {
        const copy = { ...m };
        delete copy[rl.id];
        return copy;
      });
      setApplyingListId((cur) => (cur === rl.id ? null : cur));
      setBusyListId((cur) => (cur === rl.id ? null : cur));
    }
  };

  const handleRefreshList = async (rl) => {
    setRefreshingListId(rl.id);
    try {
      await wailsAPI.refreshRoutingList(rl.id);
    } catch (err) {
      console.error("Refresh routing list error:", err);
    } finally {
      await refreshRoutingListsFromConfig();
      setRefreshingListId(null);
    }
  };

  const handleDeleteList = async (rl) => {
    const ok = await showConfirmDialog({
      title: t("common.confirmAction"),
      message: rl.subscriptionId
        ? t("routingLists.confirmDeleteProvider")
        : t("routingLists.confirmDelete", { name: rl.name || rl.url }),
      variant: "danger",
      confirmText: t("common.delete"),
      cancelText: t("common.cancel"),
    });
    if (!ok) return;
    setBusyListId(rl.id);
    try {
      await wailsAPI.deleteRoutingList(rl.id);
    } catch (err) {
      console.error("Delete routing list error:", err);
    } finally {
      await refreshRoutingListsFromConfig();
      setBusyListId(null);
    }
  };

  const formatUpdatedAgo = useMemo(() => {
    return (updatedAt) => {
      if (!updatedAt) return t("routingLists.updatedNever");
      const deltaSec = Math.max(
        0,
        Math.floor(Date.now() / 1000) - updatedAt,
      );
      let value, unit;
      if (deltaSec < 60) {
        value = -deltaSec;
        unit = "second";
      } else if (deltaSec < 3600) {
        value = -Math.floor(deltaSec / 60);
        unit = "minute";
      } else if (deltaSec < 86400) {
        value = -Math.floor(deltaSec / 3600);
        unit = "hour";
      } else {
        value = -Math.floor(deltaSec / 86400);
        unit = "day";
      }
      try {
        const rtf = new Intl.RelativeTimeFormat(i18n.language, {
          numeric: "auto",
        });
        return t("routingLists.updatedAgo", { time: rtf.format(value, unit) });
      } catch {
        return t("routingLists.updatedAgo", { time: `${-value} ${unit}` });
      }
    };
  }, [t, i18n.language]);

  const isWin =
    platform === "win32" || platform === "windows" || platform === "win64";

  // The two lists change meaning with the routing mode. In Global they are
  // exceptions (bypass the VPN): whitelist + appWhitelist. In Smart everything
  // is direct by default, so the same blocks invert — they ADD to the VPN:
  // customBlockedDomains + appForceVPN. Each mode edits its own config field,
  // so switching modes shows the other list (Smart's start empty) without
  // destroying the Global exceptions.
  const isSmart = rules.mode === "smart";
  const domainField = isSmart ? "customBlockedDomains" : "whitelist";
  const appField = isSmart ? "appForceVPN" : "appWhitelist";
  const domainList = rules[domainField] || [];
  const appList = rules[appField] || [];

  const addDomain = () => {
    const d = isSmart ? newDomain.trim().toLowerCase() : newDomain;
    if (d && !domainList.includes(d)) {
      setRules({ ...rules, [domainField]: [...domainList, d] });
      setNewDomain("");
    }
  };

  const removeDomain = (d) => {
    setRules({ ...rules, [domainField]: domainList.filter((i) => i !== d) });
  };

  const addSpecificDomain = (domain) => {
    if (!domainList.includes(domain)) {
      setRules({ ...rules, [domainField]: [...domainList, domain] });
    }
  };

  const normalizeAppName = (raw) => {
    let appName = raw.toLowerCase().trim();
    if (isWin && !appName.endsWith(".exe")) appName += ".exe";
    if (!isWin && !appName.includes(".")) appName += ".app";
    return appName;
  };

  const addAppName = (appName) => {
    if (!appName) return;
    if (!appList.includes(appName)) {
      setRules({ ...rules, [appField]: [...appList, appName] });
    }
  };

  const addApp = () => {
    if (!newApp) return;
    addAppName(normalizeAppName(newApp));
    setNewApp("");
  };

  const removeApp = (appName) => {
    setRules({ ...rules, [appField]: appList.filter((i) => i !== appName) });
  };

  const handleFileSelect = (e) => {
    const file = e.target.files[0];
    if (file) {
      addAppName(normalizeAppName(file.name));
      e.target.value = "";
    }
  };

  const pickAppNative = async () => {
    try {
      const appName = await PickAppForWhitelist();
      if (!appName) return;
      addAppName(appName);
    } catch (err) {
      // Fallback to the legacy HTML file picker if the native dialog fails.
      if (appFileInputRef.current) appFileInputRef.current.click();
    }
  };

  const popularTlds = ["*.ru", "*.рф", "*.su", "*.by", "*.kz"];
  const appExt = isWin ? ".exe" : ".app";
  const appPlaceholder = isWin ? "steam.exe" : "safari.app";

  const popularApps = isWin
    ? ["steam.exe", "discord.exe", "telegram.exe", "epicgameslauncher.exe"]
    : ["safari.app", "discord.app", "telegram.app"];

  const domainTitle = isSmart
    ? t("rules.forceDomains.title")
    : t("rules.domains.title");
  const domainDesc = isSmart
    ? t("rules.forceDomains.desc")
    : t("rules.domains.desc");
  const domainPlaceholder = isSmart
    ? t("rules.forceDomains.placeholder")
    : t("rules.domains.placeholder");

  const appTitle = isSmart
    ? t("rules.forceApps.title")
    : t("rules.apps.title");

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

      <div className="bg-zinc-900 p-6 rounded-3xl border border-zinc-800 mt-6">
        <div className="flex items-center space-x-2 mb-2">
          <h3 className="text-white font-bold text-lg">{domainTitle}</h3>
          {!isSmart && (
            <div className="relative group">
              <HelpCircle className="w-5 h-5 text-zinc-500 hover:text-zinc-300 cursor-help transition-colors" />
              <div className="absolute bottom-full left-0 mb-2 px-3 py-2 bg-zinc-800 text-zinc-200 text-xs rounded-lg opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none w-64 z-10 border border-zinc-700 shadow-xl">
                {t("rules.domains.tooltip")}
              </div>
            </div>
          )}
        </div>

        <p className="text-zinc-500 text-sm mb-6">{domainDesc}</p>
        <div className="flex space-x-3 mb-6">
          <input
            type="text"
            placeholder={domainPlaceholder}
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
        <div className="flex flex-wrap gap-2">
          {domainList.map((d) => (
            <div
              key={d}
              className="bg-zinc-800 px-4 py-2 rounded-lg flex items-center space-x-3 border border-zinc-700"
            >
              <span className="text-sm text-zinc-300">{d}</span>
              <button
                onClick={() => removeDomain(d)}
                className="text-zinc-500 hover:text-rose-500 transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
              >
                <Trash2 className="w-4 h-4" />
              </button>
            </div>
          ))}
        </div>

        {!isSmart && (
          <div className="pt-6 mt-8 border-t border-zinc-800">
            <p className="text-sm font-medium text-zinc-400 mb-4">
              {t("rules.domains.fast_add")}
            </p>
            <div className="flex flex-wrap gap-3">
              {popularTlds.map((tld) => {
                const isAdded = domainList.includes(tld);
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
        )}
      </div>

      <div className="bg-zinc-900 p-6 rounded-3xl border border-zinc-800 mt-6">
        <h3 className="text-white font-bold text-lg mb-2">
          {appTitle} ({isWin ? ".EXE" : ".APP"})
        </h3>
        <p className="text-zinc-500 text-sm mb-6">
          {isSmart ? (
            t("rules.forceApps.desc")
          ) : (
            <>
              {t("rules.apps.desc1")}
              <strong>{t("rules.apps.desc2")}</strong>
            </>
          )}
        </p>
        <div className="flex flex-wrap gap-3 mb-6">
          <input
            type="text"
            placeholder={`${t("rules.apps.placeholder")}${appPlaceholder}`}
            className="flex-1 min-w-[180px] bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:outline-none focus:ring-0 focus-visible:outline-none focus:border-[#007E3A] transition-colors"
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
            onClick={pickAppNative}
            className="bg-[#007E3A] hover:bg-[#00A819] text-white px-6 font-bold rounded-xl transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none shrink-0"
          >
            {t("rules.apps.choose_file")}
            {appExt}
          </button>
        </div>
        <div className="flex flex-wrap gap-2">
          {appList.map((appName) => (
            <div
              key={appName}
              className="bg-zinc-800 px-4 py-2 rounded-lg flex items-center space-x-3 border border-zinc-700"
            >
              <span className="text-sm text-zinc-300 font-mono">{appName}</span>
              <button
                onClick={() => removeApp(appName)}
                className="text-zinc-500 hover:text-rose-500 transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
              >
                <Trash2 className="w-4 h-4" />
              </button>
            </div>
          ))}
        </div>

        {!isSmart && (
          <div className="pt-6 mt-8 border-t border-zinc-800">
            <p className="text-sm font-medium text-zinc-400 mb-4">
              {t("rules.apps.popular")}
            </p>
            <div className="flex flex-wrap gap-3">
              {popularApps.map((appName) => {
                const isAdded = appList.includes(appName);
                return (
                  <button
                    key={appName}
                    onClick={() => addAppName(appName)}
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
        )}
      </div>

      <div className="bg-zinc-900 p-6 rounded-3xl border border-zinc-800 mt-6">
        <div className="flex flex-wrap items-start justify-between gap-4 mb-2">
          <div>
            <h3 className="text-white font-bold text-lg">
              {t("routingLists.title")}
            </h3>
            <p className="text-zinc-500 text-sm mt-1">
              {t("routingLists.subtitle")}
            </p>
          </div>
          <button
            onClick={openAddListModal}
            className="flex items-center px-6 py-3 bg-[#007E3A] hover:bg-[#00A819] text-white font-bold rounded-xl transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none shrink-0"
          >
            <ListPlus className="w-5 h-5 mr-2" />
            {t("routingLists.add")}
          </button>
        </div>

        <div className="flex items-center gap-3 mb-6 mt-4">
          <label className="text-sm font-medium text-zinc-400 shrink-0">
            {t("routingLists.updateInterval")}
          </label>
          <input
            type="number"
            min="1"
            step="1"
            className="w-28 bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-2 text-white outline-none focus:border-[#007E3A] transition-colors focus:ring-0"
            value={intervalInput}
            onChange={(e) => {
              const v = e.target.value;
              if (v === "" || /^[0-9]+$/.test(v)) setIntervalInput(v);
            }}
            onBlur={commitInterval}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                commitInterval();
              }
            }}
          />
        </div>

        {routingLists.length === 0 ? (
          <p className="text-zinc-500 text-sm">{t("routingLists.empty")}</p>
        ) : (
          <div className="space-y-3">
            {routingLists.map((rl) => {
              const busy = busyListId === rl.id;
              const refreshing = refreshingListId === rl.id;
              const applying = applyingListId === rl.id;
              const shownEnabled =
                rl.id in optimisticEnabled ? optimisticEnabled[rl.id] : rl.enabled;
              const embedded = isEmbeddedList(rl);
              const actionLabel = t(
                `routingLists.action${rl.action?.charAt(0).toUpperCase()}${rl.action?.slice(1)}`,
              );
              const displayName = embedded
                ? t("routingLists.embeddedName", { action: actionLabel })
                : rl.name || rl.url;
              return (
                <div
                  key={rl.id}
                  className="p-4 bg-zinc-800 rounded-xl border border-zinc-700 flex flex-wrap items-center gap-4"
                >
                  <div className="flex items-center gap-2 shrink-0">
                    <div
                      className={`relative w-12 h-7 rounded-2xl transition-colors duration-300 ease-in-out ${applying ? "pointer-events-none" : "cursor-pointer"} ${shownEnabled ? "bg-[#007E3A]" : "bg-zinc-700"}`}
                      onClick={() => !applying && handleToggleListEnabled(rl)}
                      role="button"
                      aria-pressed={shownEnabled}
                    >
                      <div
                        className={`absolute top-1 left-1 bg-white w-5 h-5 rounded-full transition-transform duration-300 ease-in-out ${shownEnabled ? "transform translate-x-5" : ""}`}
                      ></div>
                    </div>
                    {applying && (
                      <span
                        className="flex items-center gap-1.5 text-xs text-[#00A819]"
                        aria-live="polite"
                      >
                        <RefreshCw className="w-3.5 h-3.5 animate-spin" />
                        {t("routingLists.applying")}
                      </span>
                    )}
                  </div>

                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-white font-bold truncate">
                        {displayName}
                      </span>
                      <span
                        className={`shrink-0 text-xs font-medium px-2.5 py-0.5 rounded-full border ${ROUTING_ACTION_STYLES[rl.action] || ROUTING_ACTION_STYLES.direct}`}
                      >
                        {actionLabel}
                      </span>
                      {rl.subscriptionId && (
                        <span className="shrink-0 text-xs font-medium px-2.5 py-0.5 rounded-full border bg-zinc-800 text-zinc-400 border-zinc-700">
                          {t("routingLists.fromSubscription", {
                            name:
                              subscriptionNameById.get(rl.subscriptionId) ||
                              t("routingLists.fromSubscriptionUnknown"),
                          })}
                        </span>
                      )}
                      {embedded && (
                        <span className="shrink-0 text-xs font-medium px-2.5 py-0.5 rounded-full border bg-zinc-800 text-zinc-400 border-zinc-700">
                          {t("routingLists.embeddedBadge")}
                        </span>
                      )}
                    </div>
                    <p className="text-zinc-500 text-xs mt-1 truncate">
                      {rl.url}
                    </p>
                    <p className="text-zinc-500 text-xs mt-1">
                      {formatUpdatedAgo(rl.updatedAt)} ·{" "}
                      {t("routingLists.counts", {
                        domains: rl.domainCount || 0,
                        cidrs: rl.cidrCount || 0,
                      })}
                    </p>
                    {rl.lastError && (
                      <p className="text-rose-500 text-xs mt-1 break-words">
                        {t("routingLists.errorLabel", { error: rl.lastError })}
                      </p>
                    )}
                  </div>

                  <div className="flex items-center gap-2 shrink-0">
                    {!embedded && (
                      <button
                        type="button"
                        onClick={() => handleRefreshList(rl)}
                        disabled={busy || refreshing}
                        title={t("routingLists.refreshAria")}
                        aria-label={t("routingLists.refreshAria")}
                        className="p-2 bg-zinc-800 text-zinc-400 hover:text-white hover:bg-zinc-700 rounded-xl transition-colors shrink-0 border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none disabled:opacity-50"
                      >
                        <RefreshCw
                          className={`w-3.5 h-3.5 ${refreshing ? "animate-spin" : ""}`}
                        />
                      </button>
                    )}
                    {!rl.subscriptionId && (
                      <button
                        type="button"
                        onClick={() => openEditListModal(rl)}
                        disabled={busy}
                        title={t("routingLists.editAria")}
                        aria-label={t("routingLists.editAria")}
                        className="p-2 bg-zinc-800 text-zinc-400 hover:text-white hover:bg-zinc-700 rounded-xl transition-colors shrink-0 border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none disabled:opacity-50"
                      >
                        <Pencil className="w-3.5 h-3.5" />
                      </button>
                    )}
                    <button
                      type="button"
                      onClick={() => handleDeleteList(rl)}
                      disabled={busy}
                      title={t("routingLists.deleteAria")}
                      aria-label={t("routingLists.deleteAria")}
                      className="p-2 bg-zinc-800 text-zinc-400 hover:text-rose-500 hover:bg-rose-500/10 rounded-xl transition-colors shrink-0 border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none disabled:opacity-50"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      <RoutingListModal
        isOpen={listModal.isOpen}
        mode={listModal.mode}
        initial={listModal.initial}
        onClose={closeListModal}
        onSubmit={handleListModalSubmit}
      />
    </div>
  );
};
