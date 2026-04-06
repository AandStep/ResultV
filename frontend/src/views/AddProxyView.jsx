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

import React, { useState, useEffect } from "react";
import { Lock, Link2 } from "lucide-react";
import { useConfigContext } from "../context/ConfigContext";
import { useConnectionContext } from "../context/ConnectionContext";
import { useTranslation } from "react-i18next";
import {
  parseProxies,
  isSubscriptionURL,
  isVpnType,
  subscriptionLabelFromURL,
} from "../utils/proxyParser";
import { FileUp, ClipboardList } from "lucide-react";
import ProtocolSelectionModal from "../components/ui/ProtocolSelectionModal";
import wailsAPI from "../utils/wailsAPI";

const PLAIN_TYPES = ["HTTP", "HTTPS", "SOCKS5"];
const VPN_TYPES_LIST = ["VLESS", "VMESS", "TROJAN", "SS"];

export const AddProxyView = () => {
  const { t } = useTranslation();
  const {
    handleSaveProxy,
    handleBulkSaveProxies,
    editingProxy,
    setEditingProxy,
    setActiveTab,
    setSubscriptions,
  } = useConfigContext();
  const {
    activeProxy,
    failedProxy,
    setFailedProxy,
    setActiveProxy,
    isConnected,
    selectAndConnect,
  } = useConnectionContext();

  const onCancel = () => setActiveTab("list");

  const saveProxyWrapper = (proxyData) => {
    handleSaveProxy(
      proxyData,
      activeProxy,
      failedProxy,
      setFailedProxy,
      setActiveProxy,
      isConnected,
      selectAndConnect,
      setActiveTab,
      setEditingProxy,
    );
  };

  const [importMode, setImportMode] = useState("single");
  const [bulkText, setBulkText] = useState("");
  const [isImporting, setIsImporting] = useState(false);
  const [pendingProxies, setPendingProxies] = useState([]);
  const [showSelectionModal, setShowSelectionModal] = useState(false);
  const [vpnUri, setVpnUri] = useState("");
  const [vpnLoading, setVpnLoading] = useState(false);

  const handleClipboardImport = async () => {
    try {
      const text = await navigator.clipboard.readText();
      if (!text) {
        alert(t("add.clipboardEmpty") || "Clipboard is empty");
        return;
      }
      setBulkText(text);
      await processImport(text);
    } catch (err) {
      console.error("Failed to read clipboard:", err);
      alert(
        t("add.clipboardError") ||
          "Could not read from clipboard. Please paste manually.",
      );
      setImportMode("bulk");
    }
  };

  const processImport = async (text) => {
    if (isSubscriptionURL(text)) {
      setIsImporting(true);
      try {
        const entries = await wailsAPI.fetchSubscription(text.trim());
        if (!entries || entries.length === 0) {
          alert(t("add.noProxiesFound") || "No proxies found.");
          return;
        }
        setPendingProxies(entries);
        setShowSelectionModal(true);
      } catch (err) {
        console.error("Subscription fetch error:", err);
        alert(t("add.subscriptionError") || `Error: ${err}`);
      } finally {
        setIsImporting(false);
      }
      return;
    }

    const proxiesToImport = parseProxies(text);
    if (proxiesToImport.length === 0) {
      alert(t("add.noProxiesFound") || "No proxies found.");
      return;
    }

    setPendingProxies(proxiesToImport);
    setShowSelectionModal(true);
  };

  const handleConfirmImport = async (protocol) => {
    setShowSelectionModal(false);
    setIsImporting(true);
    try {
      const namedProxies = pendingProxies.map((p) => ({
        ...p,
        name:
          p.name || `${t("add.newServer")} ${new Date().toLocaleTimeString()}`,
      }));

      const subURL = namedProxies[0]?.subscriptionUrl;
      const allSameSubscription =
        subURL &&
        isSubscriptionURL(subURL) &&
        namedProxies.every((p) => p.subscriptionUrl === subURL);

      if (allSameSubscription) {
        const label = subscriptionLabelFromURL(subURL);
        let entries;
        try {
          entries = await wailsAPI.addSubscription(label, subURL);
        } catch (err) {
          const msg = String(err?.message || err || "");
          if (msg.includes("уже добавлена")) {
            const cfg = await wailsAPI.getConfig();
            const existing = cfg.subscriptions?.find((s) => s.url === subURL);
            if (!existing) throw err;
            entries = await wailsAPI.refreshSubscription(existing.id);
          } else {
            throw err;
          }
        }
        const withNames = entries.map((p, i) => ({
          ...p,
          name:
            p.name ||
            `${t("add.newServer")} ${new Date().toLocaleTimeString()}-${i}`,
        }));
        await handleBulkSaveProxies(withNames, setActiveTab, protocol);
        const cfg = await wailsAPI.getConfig();
        if (cfg.subscriptions) setSubscriptions(cfg.subscriptions);
      } else {
        await handleBulkSaveProxies(namedProxies, setActiveTab, protocol);
      }

      setBulkText("");
      setImportMode("single");
      setPendingProxies([]);
    } catch (error) {
      console.error("Import failed:", error);
    } finally {
      setIsImporting(false);
    }
  };

  const handleBulkImport = () => processImport(bulkText);

  const handleFileImport = (e) => {
    const file = e.target.files[0];
    if (!file) return;

    const reader = new FileReader();
    reader.onload = async (event) => {
      await processImport(event.target.result);
    };
    reader.readAsText(file);
    e.target.value = "";
  };

  const handleVpnUriSubmit = async () => {
    const text = vpnUri.trim();
    if (!text) return;

    if (isSubscriptionURL(text)) {
      setVpnLoading(true);
      try {
        const entries = await wailsAPI.fetchSubscription(text);
        if (!entries || entries.length === 0) {
          alert(t("add.noProxiesFound") || "No proxies found.");
          return;
        }
        setPendingProxies(entries);
        setShowSelectionModal(true);
      } catch (err) {
        console.error("Subscription fetch error:", err);
        alert(t("add.subscriptionError") || `Error: ${err}`);
      } finally {
        setVpnLoading(false);
      }
      return;
    }

    const parsed = parseProxies(text);
    if (parsed.length === 0) {
      alert(t("add.noProxiesFound") || "No proxies found.");
      return;
    }
    if (parsed.length === 1 && isVpnType(parsed[0].type)) {
      const p = parsed[0];
      saveProxyWrapper({
        ...p,
        name: p.name || t("add.newServer"),
      });
    } else {
      setPendingProxies(parsed);
      setShowSelectionModal(true);
    }
    setVpnUri("");
  };

  const [formData, setFormData] = useState({
    name: "",
    ip: "",
    port: "",
    type: "HTTP",
    username: "",
    password: "",
    country: "\u{1F310}",
  });

  const isVpnMode = VPN_TYPES_LIST.includes(formData.type);

  useEffect(() => {
    if (editingProxy) setFormData(editingProxy);
    else
      setFormData({
        name: "",
        ip: "",
        port: "",
        type: "HTTP",
        username: "",
        password: "",
        country: "\u{1F310}",
      });
  }, [editingProxy]);

  const handleSubmit = (e) => {
    e.preventDefault();
    if (formData.ip && formData.port)
      saveProxyWrapper({
        ...formData,
        name: formData.name || t("add.newServer"),
      });
  };

  return (
    <div className="space-y-6 animate-in fade-in duration-300">
      <div>
        <div className="flex justify-between items-end mb-2">
          <div>
            <h2 className="text-3xl font-bold text-white">
              {editingProxy ? t("add.titleEdit") : t("add.titleAdd")}
            </h2>
            <p className="text-zinc-400 mt-2">
              {editingProxy ? t("add.descEdit") : t("add.descAdd")}
            </p>
          </div>
        </div>
      </div>

      {!editingProxy && (
        <div className="grid grid-cols-2 gap-4 animate-in fade-in slide-in-from-top-4 duration-500 delay-150">
          <label className="cursor-pointer bg-zinc-900/50 hover:bg-zinc-800/80 text-white font-bold py-5 rounded-3xl transition-all border border-zinc-800 flex flex-col items-center justify-center gap-2 group hover:border-[#007E3A]/50 hover:shadow-lg hover:shadow-[#007E3A]/5">
            <FileUp className="w-6 h-6 text-[#007E3A] group-hover:scale-110 transition-transform" />
            <span className="text-sm">{t("add.fromFile")}</span>
            <input
              type="file"
              accept=".txt,.csv"
              className="hidden"
              onChange={handleFileImport}
            />
          </label>
          <button
            onClick={handleClipboardImport}
            className="bg-zinc-900/50 hover:bg-zinc-800/80 text-white font-bold py-5 rounded-3xl transition-all border border-zinc-800 flex flex-col items-center justify-center gap-2 group hover:border-[#007E3A]/50 hover:shadow-lg hover:shadow-[#007E3A]/5"
          >
            <ClipboardList className="w-6 h-6 text-[#007E3A] group-hover:scale-110 transition-transform" />
            <span className="text-sm">{t("add.fromClipboard")}</span>
          </button>
        </div>
      )}

      {importMode === "single" ? (
        <form
          onSubmit={handleSubmit}
          className="bg-zinc-900 p-6 rounded-3xl border border-zinc-800 space-y-5"
        >
          {!isVpnMode && (
            <div>
              <label className="block text-sm font-medium text-zinc-400 mb-2">
                {t("add.profileName")}
              </label>
              <input
                type="text"
                placeholder={t("add.profilePlaceholder")}
                className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:outline-none focus:ring-0 focus-visible:outline-none focus:border-[#007E3A] transition-colors"
                value={formData.name || ""}
                onChange={(e) =>
                  setFormData({ ...formData, name: e.target.value })
                }
              />
            </div>
          )}

          {!isVpnMode && (
            <div className="grid grid-cols-3 gap-6">
              <div className="col-span-2">
                <label className="block text-sm font-medium text-zinc-400 mb-2">
                  {t("add.ip")}
                </label>
                <input
                  type="text"
                  required
                  placeholder="192.168.1.1"
                  className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:outline-none focus:ring-0 focus-visible:outline-none focus:border-[#007E3A] transition-colors"
                  value={formData.ip || ""}
                  onChange={(e) =>
                    setFormData({ ...formData, ip: e.target.value })
                  }
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-zinc-400 mb-2">
                  {t("add.port")}
                </label>
                <input
                  type="number"
                  required
                  placeholder="8000"
                  className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:outline-none focus:ring-0 focus-visible:outline-none focus:border-[#007E3A] transition-colors"
                  value={formData.port || ""}
                  onChange={(e) =>
                    setFormData({ ...formData, port: e.target.value })
                  }
                />
              </div>
            </div>
          )}

          <div>
            <label className="block text-sm font-medium text-zinc-400 mb-3">
              {t("add.protocol")}
            </label>
            <div className="grid grid-cols-3 gap-3 mb-3">
              {PLAIN_TYPES.map((type) => (
                <button
                  type="button"
                  key={type}
                  onClick={() => setFormData({ ...formData, type })}
                  className={`py-3 rounded-xl text-sm font-bold border transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${formData.type === type ? "bg-[#007E3A] text-white border-[#007E3A]" : "bg-zinc-950 text-zinc-400 border-zinc-800 hover:border-[#00A819]"}`}
                >
                  {type}
                </button>
              ))}
            </div>
            <div className="grid grid-cols-4 gap-3">
              {VPN_TYPES_LIST.map((type) => (
                <button
                  type="button"
                  key={type}
                  onClick={() => setFormData({ ...formData, type })}
                  className={`py-3 rounded-xl text-xs font-bold border transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${formData.type === type ? "bg-[#007E3A] text-white border-[#007E3A]" : "bg-zinc-950 text-zinc-400 border-zinc-800 hover:border-[#00A819]"}`}
                >
                  {type}
                </button>
              ))}
            </div>
          </div>

          {isVpnMode ? (
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-zinc-400 mb-2">
                  {t("add.subscriptionUrl") || "Ссылка на подписку или URI"}
                </label>
                <textarea
                  className="w-full h-28 bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors resize-none"
                  placeholder={t("add.subscriptionPlaceholder") || "https://example.com/sub или vless://..."}
                  value={vpnUri}
                  onChange={(e) => setVpnUri(e.target.value)}
                />
              </div>
              <button
                type="button"
                onClick={handleVpnUriSubmit}
                disabled={vpnLoading || !vpnUri.trim()}
                className="w-full bg-[#007E3A] hover:bg-[#005C2A] disabled:bg-zinc-800 disabled:text-zinc-600 text-white font-bold py-3 rounded-xl transition-all shadow-lg shadow-[#007E3A]/20 flex items-center justify-center space-x-2"
              >
                {vpnLoading ? (
                  <div className="w-5 h-5 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                ) : (
                  <Link2 className="w-5 h-5" />
                )}
                <span>
                  {vpnLoading
                    ? t("add.importing") || "Загрузка..."
                    : t("add.loadSubscription") || "Загрузить"}
                </span>
              </button>
            </div>
          ) : (
            <>
              <div className="pt-5 border-t border-zinc-800">
                <p className="text-sm font-medium text-zinc-400 mb-3 flex items-center">
                  <Lock className="w-4 h-4 mr-2" /> {t("add.auth")}
                </p>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
                  <input
                    type="text"
                    placeholder={t("add.loginPlaceholder")}
                    className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:outline-none focus:ring-0 focus-visible:outline-none focus:border-[#007E3A] transition-colors"
                    value={formData.username || ""}
                    onChange={(e) =>
                      setFormData({ ...formData, username: e.target.value })
                    }
                  />
                  <input
                    type="password"
                    placeholder={t("add.passPlaceholder")}
                    className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:outline-none focus:ring-0 focus-visible:outline-none focus:border-[#007E3A] transition-colors"
                    value={formData.password || ""}
                    onChange={(e) =>
                      setFormData({ ...formData, password: e.target.value })
                    }
                  />
                </div>
              </div>
              <div className="pt-5 flex space-x-4">
                {editingProxy && (
                  <button
                    type="button"
                    onClick={onCancel}
                    className="flex-1 bg-zinc-800 hover:bg-zinc-700 text-white font-bold py-3 rounded-xl transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
                  >
                    {t("add.cancel")}
                  </button>
                )}
                <button
                  type="submit"
                  className="flex-[2] bg-[#007E3A] hover:bg-[#005C2A] text-white font-bold py-3 rounded-xl transition-colors shadow-lg shadow-[#007E3A]/20 border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
                >
                  {editingProxy ? t("add.saveChanges") : t("add.saveProxy")}
                </button>
              </div>
            </>
          )}
        </form>
      ) : (
        <div className="bg-zinc-900 p-6 rounded-3xl border border-zinc-800 space-y-5">
          <div className="flex flex-col space-y-4">
            <div className="flex justify-between items-center">
              <label className="block text-sm font-medium text-zinc-400">
                {t("add.bulkInputLabel") ||
                  "Ручной ввод прокси (CSV или IP:Port:User:Pass)"}
              </label>
              <button
                onClick={() => setImportMode("single")}
                className="text-xs text-zinc-500 hover:text-white transition-colors"
              >
                {t("add.backToSingle") || "Вернуться к форме"}
              </button>
            </div>
            <textarea
              className="w-full h-48 bg-zinc-950 border border-zinc-800 rounded-2xl p-4 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors resize-none"
              placeholder={`ip,port,login,password\n85.195.81.139,13449,user,pass\n\n- ИЛИ -\n\n85.195.81.139:13449:user:pass`}
              value={bulkText}
              onChange={(e) => setBulkText(e.target.value)}
            />
            <button
              onClick={handleBulkImport}
              disabled={isImporting || !bulkText.trim()}
              className="w-full bg-[#007E3A] hover:bg-[#005C2A] disabled:bg-zinc-800 disabled:text-zinc-600 text-white font-bold py-4 rounded-xl transition-all shadow-lg shadow-[#007E3A]/10 flex items-center justify-center space-x-2"
            >
              {isImporting ? (
                <div className="w-5 h-5 border-2 border-white/30 border-t-white rounded-full animate-spin" />
              ) : (
                <ClipboardList className="w-5 h-5" />
              )}
              <span>
                {isImporting
                  ? t("add.importing") || "Импорт..."
                  : t("add.importProxies") || "Добавить прокси"}
              </span>
            </button>
          </div>
        </div>
      )}

      {isImporting && importMode !== "bulk" && (
        <div className="fixed inset-0 bg-black/50 backdrop-blur-sm z-50 flex items-center justify-center">
          <div className="bg-zinc-900 border border-zinc-800 p-8 rounded-3xl flex flex-col items-center space-y-4 shadow-2xl">
            <div className="w-12 h-12 border-4 border-[#007E3A]/30 border-t-[#007E3A] rounded-full animate-spin" />
            <p className="text-white font-medium">
              {t("add.importing") || "Импорт прокси..."}
            </p>
          </div>
        </div>
      )}

      <ProtocolSelectionModal
        isOpen={showSelectionModal}
        proxies={pendingProxies}
        count={pendingProxies.length}
        onClose={() => {
          setShowSelectionModal(false);
          setPendingProxies([]);
        }}
        onConfirm={handleConfirmImport}
      />
    </div>
  );
};
