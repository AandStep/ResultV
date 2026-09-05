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
import { useTranslation } from "react-i18next";
import wailsAPI from "../../utils/wailsAPI";
import { isInsecureSubscriptionError } from "../../utils/subscriptionSecurity";
import {
  isSubscriptionURL,
  isEncryptedSubscription,
  subscriptionLabelFromURL,
} from "../../utils/proxyParser";
import { useConfigContext } from "../../context/ConfigContext";
import { Button, Dialog } from "../kit";
import AddConfirmDialog from "../../views/redesign/AddConfirmDialog";

const DeepLinkImportModal = () => {
  const { t } = useTranslation();
  const {
    pendingDeepLink,
    setPendingDeepLink,
    pendingDeepLinkSource,
    setPendingDeepLinkSource,
    handleBulkSaveProxies,
    setActiveTab,
    setSubscriptions,
    syncRoutingLists,
    showConfirmDialog,
  } = useConfigContext();

  const [stage, setStage] = useState("idle");
  const [pendingProxies, setPendingProxies] = useState([]);
  const [pendingRoutingLists, setPendingRoutingLists] = useState([]);
  /* Списки маршрутизации из подписки по умолчанию не берём — как в макете. */
  const [routing, setRouting] = useState(false);
  const [error, setError] = useState("");
  const reqId = useRef(0);

  useEffect(() => {
    if (!pendingDeepLink) return;
    const text = pendingDeepLink.trim();
    if (!text) {
      setPendingDeepLink("");
      setPendingDeepLinkSource("");
      return;
    }
    const myReq = ++reqId.current;
    setStage("loading");
    setError("");
    setPendingProxies([]);

    (async () => {
      try {
        // Browser-click flow gives us the already-decoded payload via the
        // "deeplink:received" event. Paste flow sets `pendingDeepLink` to the
        // raw resultv:// URL — decode it here so the rest of this effect
        // operates on the same shape (URL / RVSUB1 body / proxy URI list) in
        // both flows.
        let resolved = text;
        if (/^resultv:(\/\/)?/i.test(resolved)) {
          resolved = (await wailsAPI.decodeDeepLink(resolved)).trim();
        }
        let preview;
        if (isSubscriptionURL(resolved)) {
          try {
            preview = await wailsAPI.fetchSubscription(resolved);
          } catch (fetchErr) {
            if (!isInsecureSubscriptionError(fetchErr)) throw fetchErr;
            const ok = await showConfirmDialog({
              title: t("add.insecureSubscriptionTitle"),
              message: t("add.insecureSubscriptionMessage"),
              variant: "warning",
              confirmText: t("add.insecureSubscriptionConfirm"),
              cancelText: t("common.cancel"),
            });
            if (!ok) {
              if (myReq !== reqId.current) return;
              setError(t("add.insecureSubscriptionCancelled"));
              setStage("error");
              return;
            }
            preview = await wailsAPI.fetchSubscription(resolved, true);
          }
        } else if (isEncryptedSubscription(resolved)) {
          preview = await wailsAPI.parseSubscriptionText(resolved);
        } else {
          preview = await wailsAPI.parseSubscriptionText(resolved);
        }
        if (myReq !== reqId.current) return;
        const entries = preview?.proxies || [];
        const lists = preview?.routingLists || [];
        if (!entries || entries.length === 0) {
          setError(t("add.noProxiesFound") || "Серверы не найдены");
          setStage("error");
          return;
        }
        setPendingProxies(entries);
        setPendingRoutingLists(lists);
        setRouting(false);
        setStage("preview");
      } catch (e) {
        if (myReq !== reqId.current) return;
        setError(String(e?.message || e));
        setStage("error");
      }
    })();
  }, [pendingDeepLink, setPendingDeepLink, showConfirmDialog, t]);

  const close = () => {
    reqId.current++;
    setStage("idle");
    setPendingProxies([]);
    setPendingRoutingLists([]);
    setRouting(false);
    setError("");
    setPendingDeepLink("");
    setPendingDeepLinkSource("");
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
        let allowInsecure = false;
        for (;;) {
          try {
            entries = await wailsAPI.addSubscription(
              label,
              subURL,
              allowInsecure,
              pendingDeepLinkSource,
              routing ? [] : pendingRoutingLists.map((rl) => rl.url),
            );
            break;
          } catch (err) {
            const msg = String(err?.message || err || "");
            if (isInsecureSubscriptionError(err) && !allowInsecure) {
              const ok = await showConfirmDialog({
                title: t("add.insecureSubscriptionTitle"),
                message: t("add.insecureSubscriptionMessage"),
                variant: "warning",
                confirmText: t("add.insecureSubscriptionConfirm"),
                cancelText: t("common.cancel"),
              });
              if (!ok) {
                setError(t("add.insecureSubscriptionCancelled"));
                setStage("error");
                return;
              }
              allowInsecure = true;
              continue;
            }
            if (msg.includes("уже добавлена")) {
              const cfg = await wailsAPI.getConfig();
              const existing = cfg.subscriptions?.find((s) => s.url === subURL);
              if (!existing) throw err;
              entries = await wailsAPI.refreshSubscription(existing.id);
              break;
            }
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
        if (cfg?.routingRules?.routingLists) syncRoutingLists(cfg.routingRules.routingLists);
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

  /*
   * Найденное показывает то же окно, что и страница добавления: своего вида у
   * ссылки в макете нет, ход один и тот же.
   */
  if (stage === "preview") {
    return (
      <AddConfirmDialog
        proxies={pendingProxies}
        routingLists={pendingRoutingLists}
        routing={routing}
        onRoutingChange={setRouting}
        onCancel={close}
        /* Протокол ссылки несут в себе, подставлять нечего. */
        onConfirm={() => handleConfirm(null)}
      />
    );
  }

  if (stage === "error") {
    return (
      <Dialog
        variant="error"
        icon="alert"
        title={t("deeplink.errorTitle")}
        onClose={close}
        actions={
          <Button variant="red" onClick={close}>
            {t("common.close")}
          </Button>
        }
      >
        <p className="rv-dialog__text">{error}</p>
      </Dialog>
    );
  }

  /*
   * Ожидание. Кадра на него в макете нет, поэтому взято то же окно с щитом:
   * заголовок и строка о том, что происходит. Значок при этом дышит — так же
   * ждала прежняя версия окна, и без движения непонятно, живо ли приложение.
   *
   * Пока идёт запись, закрывать нечего: ход уже не отменить.
   */
  return (
    <Dialog
      icon="shield"
      title={t("deeplink.loadingTitle")}
      onClose={stage === "saving" ? undefined : close}
      showClose={false}
      className="rv-dialog--busy"
    >
      <p className="rv-dialog__text">
        {stage === "saving" ? t("deeplink.savingDesc") : t("deeplink.loadingDesc")}
      </p>
    </Dialog>
  );
};

export default DeepLinkImportModal;
