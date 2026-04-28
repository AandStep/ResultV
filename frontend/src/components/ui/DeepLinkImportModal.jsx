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

import React, { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import { Link2 } from "lucide-react";
import wailsAPI from "../../utils/wailsAPI";
import {
  isSubscriptionURL,
  isEncryptedSubscription,
  parseProxies,
  subscriptionLabelFromURL,
} from "../../utils/proxyParser";
import { useConfigContext } from "../../context/ConfigContext";
import ProtocolSelectionModal from "./ProtocolSelectionModal";

const LoadingPanel = ({ t, label }) => (
  <div className="relative bg-zinc-900 border border-zinc-800 w-full max-w-md p-6 rounded-3xl shadow-2xl flex flex-col items-center text-center space-y-5 animate-in zoom-in-95 duration-300">
    <div className="w-16 h-16 bg-[#007E3A]/10 rounded-full flex items-center justify-center">
      <Link2 className="w-8 h-8 text-[#007E3A] animate-pulse" />
    </div>
    <div className="space-y-2">
      <h3 className="text-xl font-bold text-white">
        {t("deeplink.loadingTitle") || "Импорт по ссылке"}
      </h3>
      <p className="text-zinc-400 text-sm leading-relaxed">
        {label || t("deeplink.loadingDesc") || "Получаем данные подписки..."}
      </p>
    </div>
    <div className="w-full space-y-2">
      <div className="h-3 bg-zinc-800 rounded-md overflow-hidden">
        <div className="h-full w-2/3 bg-[#007E3A]/40 animate-pulse rounded-md" />
      </div>
      <div className="h-3 bg-zinc-800 rounded-md overflow-hidden">
        <div className="h-full w-1/2 bg-[#007E3A]/30 animate-pulse rounded-md" />
      </div>
      <div className="h-3 bg-zinc-800 rounded-md overflow-hidden">
        <div className="h-full w-3/4 bg-[#007E3A]/20 animate-pulse rounded-md" />
      </div>
    </div>
  </div>
);

const ErrorPanel = ({ t, message, onClose }) => (
  <div className="relative bg-zinc-900 border border-zinc-800 w-full max-w-md p-6 rounded-3xl shadow-2xl flex flex-col items-center text-center space-y-5 animate-in zoom-in-95 duration-300">
    <div className="w-16 h-16 bg-rose-500/10 rounded-full flex items-center justify-center">
      <Link2 className="w-8 h-8 text-rose-500" />
    </div>
    <div className="space-y-2">
      <h3 className="text-xl font-bold text-white">
        {t("deeplink.errorTitle") || "Не удалось импортировать"}
      </h3>
      <p className="text-zinc-400 text-sm leading-relaxed break-words">
        {message}
      </p>
    </div>
    <button
      onClick={onClose}
      className="w-full bg-zinc-800 hover:bg-zinc-700 text-white font-bold py-3 rounded-2xl transition-all"
    >
      {t("common.close") || "Закрыть"}
    </button>
  </div>
);

const DeepLinkImportModal = () => {
  const { t } = useTranslation();
  const {
    pendingDeepLink,
    setPendingDeepLink,
    handleBulkSaveProxies,
    setActiveTab,
    setSubscriptions,
  } = useConfigContext();

  const [stage, setStage] = useState("idle");
  const [pendingProxies, setPendingProxies] = useState([]);
  const [error, setError] = useState("");
  const reqId = useRef(0);

  useEffect(() => {
    if (!pendingDeepLink) return;
    const text = pendingDeepLink.trim();
    if (!text) {
      setPendingDeepLink("");
      return;
    }
    const myReq = ++reqId.current;
    setStage("loading");
    setError("");
    setPendingProxies([]);

    (async () => {
      try {
        let entries;
        if (isSubscriptionURL(text)) {
          entries = await wailsAPI.fetchSubscription(text);
        } else if (isEncryptedSubscription(text)) {
          entries = await wailsAPI.parseSubscriptionText(text);
        } else {
          entries = parseProxies(text);
        }
        if (myReq !== reqId.current) return;
        if (!entries || entries.length === 0) {
          setError(t("add.noProxiesFound") || "Серверы не найдены");
          setStage("error");
          return;
        }
        setPendingProxies(entries);
        setStage("preview");
      } catch (e) {
        if (myReq !== reqId.current) return;
        setError(String(e?.message || e));
        setStage("error");
      }
    })();
  }, [pendingDeepLink, setPendingDeepLink, t]);

  const close = () => {
    reqId.current++;
    setStage("idle");
    setPendingProxies([]);
    setError("");
    setPendingDeepLink("");
  };

  const handleConfirm = async (protocol) => {
    setStage("saving");
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
      setActiveTab("list");
      close();
    } catch (e) {
      setError(String(e?.message || e));
      setStage("error");
    }
  };

  if (stage === "idle") return null;

  if (stage === "preview") {
    return (
      <ProtocolSelectionModal
        isOpen
        proxies={pendingProxies}
        count={pendingProxies.length}
        onClose={close}
        onConfirm={handleConfirm}
      />
    );
  }

  return createPortal(
    <div className="fixed inset-0 z-[9999] flex items-center justify-center p-4">
      <div
        className="absolute inset-0 bg-black/70 backdrop-blur-md"
        onClick={stage === "saving" ? undefined : close}
      />
      {stage === "error" ? (
        <ErrorPanel t={t} message={error} onClose={close} />
      ) : (
        <LoadingPanel
          t={t}
          label={
            stage === "saving"
              ? t("deeplink.savingDesc") || "Сохраняем серверы..."
              : undefined
          }
        />
      )}
    </div>,
    document.body,
  );
};

export default DeepLinkImportModal;
