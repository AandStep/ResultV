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

/*
 * Разбор вставленного текста и добавление серверов.
 *
 * Ход тот же, что был на старой странице добавления: ссылка на подписку
 * тянется с сервера, зашифрованная и base64-подписка разбираются ядром,
 * одиночные ссылки — своим парсером. Разобранное не сохраняется сразу, а
 * ложится в `preview`: по макету между разбором и записью стоит окно
 * «Добавление серверов».
 */

import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { useConfigContext } from "../context/ConfigContext";
import wailsAPI from "../utils/wailsAPI";
import {
  isEncryptedSubscription,
  isSubscriptionURL,
  parseProxies,
  subscriptionLabelFromURL,
} from "../utils/proxyParser";
import { isInsecureSubscriptionError } from "../utils/subscriptionSecurity";

export default function useProxyImport() {
  const { t } = useTranslation();
  const {
    handleBulkSaveProxies,
    setActiveTab,
    setSubscriptions,
    syncRoutingLists,
    showAlertDialog,
    showConfirmDialog,
    setPendingDeepLink,
    setPendingDeepLinkSource,
  } = useConfigContext();

  const [busy, setBusy] = useState(false);
  /* { proxies, routingLists } — то, что нашли и показываем в окне. */
  const [preview, setPreview] = useState(null);

  const notFound = useCallback(() => {
    showAlertDialog({
      title: t("common.notice"),
      message: t("add.noProxiesFound"),
      variant: "warning",
    });
  }, [showAlertDialog, t]);

  const failed = useCallback(
    (err, lead) => {
      showAlertDialog({
        title: t("common.error"),
        message: `${lead}: ${err?.message || err}`,
        variant: "danger",
      });
    },
    [showAlertDialog, t],
  );

  /* Согласие на подписку по http:// спрашиваем один раз и повторяем запрос. */
  const askInsecure = useCallback(async () => {
    const ok = await showConfirmDialog({
      title: t("add.insecureSubscriptionTitle"),
      message: t("add.insecureSubscriptionMessage"),
      variant: "warning",
      confirmText: t("add.insecureSubscriptionConfirm"),
      cancelText: t("common.cancel"),
    });
    if (!ok) {
      showAlertDialog({
        title: t("common.notice"),
        message: t("add.insecureSubscriptionCancelled"),
        variant: "warning",
      });
    }
    return ok;
  }, [showAlertDialog, showConfirmDialog, t]);

  const importText = useCallback(
    async (raw, sourceName = "") => {
      const text = (raw || "").trim();
      if (!text) return;

      /*
       * resultv:// — своя ссылка приложения; её ведёт DeepLinkImportModal,
       * чтобы вставка и переход из браузера шли одной дорогой.
       */
      if (/^resultv:(\/\/)?/i.test(text)) {
        setPendingDeepLinkSource(/^resultv:(\/\/)?rvsub\//i.test(text) ? "rvsub" : "");
        setPendingDeepLink(text);
        return;
      }

      if (isSubscriptionURL(text)) {
        setBusy(true);
        try {
          let sub;
          try {
            sub = await wailsAPI.fetchSubscription(text);
          } catch (err) {
            if (!isInsecureSubscriptionError(err)) throw err;
            if (!(await askInsecure())) return;
            sub = await wailsAPI.fetchSubscription(text, true);
          }
          const entries = sub?.proxies || [];
          if (entries.length === 0) {
            notFound();
            return;
          }
          setPreview({ proxies: entries, routingLists: sub?.routingLists || [] });
        } catch (err) {
          console.error("Subscription fetch error:", err);
          failed(err, t("add.subscriptionError"));
        } finally {
          setBusy(false);
        }
        return;
      }

      if (isEncryptedSubscription(text)) {
        setBusy(true);
        try {
          const sub = await wailsAPI.parseSubscriptionText(text);
          const entries = sub?.proxies || [];
          if (entries.length === 0) {
            notFound();
            return;
          }
          setPreview({ proxies: entries, routingLists: sub?.routingLists || [] });
        } catch (err) {
          console.error("Encrypted subscription parse error:", err);
          failed(err, t("add.subscriptionError"));
        } finally {
          setBusy(false);
        }
        return;
      }

      /*
       * Конфиг WireGuard приходит без имени, а имя файла как раз и есть имя
       * сервера — иначе в списке окажется «WireGuard» без опознавательных
       * знаков.
       */
      const parsed = parseProxies(text).map((proxy) => {
        const wg = proxy?.type === "WIREGUARD" || proxy?.type === "AMNEZIAWG";
        const noName = !proxy.name || proxy.name === "WireGuard" || proxy.name === "AmneziaWG";
        if (!wg || !noName || !sourceName.toLowerCase().endsWith(".conf")) return proxy;
        return { ...proxy, name: sourceName.replace(/\.[^.]+$/, "") || proxy.name };
      });

      if (parsed.length > 0) {
        setPreview({ proxies: parsed, routingLists: [] });
        return;
      }

      /*
       * Свой парсер знает только ссылки и списки строк. Подписка, вставленная
       * телом (base64 или JSON от sing-box/Clash), доходит сюда — её
       * разбирает ядро.
       */
      setBusy(true);
      try {
        const sub = await wailsAPI.parseSubscriptionText(text);
        const entries = sub?.proxies || [];
        if (entries.length > 0) {
          setPreview({ proxies: entries, routingLists: sub?.routingLists || [] });
          return;
        }
      } catch (err) {
        console.error("Subscription text parse error:", err);
      } finally {
        setBusy(false);
      }
      notFound();
    },
    [askInsecure, failed, notFound, setPendingDeepLink, setPendingDeepLinkSource, t],
  );

  const cancel = useCallback(() => setPreview(null), []);

  /*
   * Запись. `protocol` касается только голых `ip:port` — у ссылок протокол
   * свой. `routing` отвечает за списки маршрутизации из подписки: выключенный
   * тумблер отправляет их все в отключённые.
   */
  const confirm = useCallback(
    async ({ protocol, routing = true } = {}) => {
      const found = preview;
      if (!found) return;
      setPreview(null);
      setBusy(true);
      try {
        const stamp = () => `${t("add.newServer")} ${new Date().toLocaleTimeString()}`;
        const named = found.proxies.map((p) => ({ ...p, name: p.name || stamp() }));
        const disabled = routing ? [] : (found.routingLists || []).map((rl) => rl.url);

        const subURL = named[0]?.subscriptionUrl;
        const oneSubscription =
          subURL &&
          isSubscriptionURL(subURL) &&
          named.every((p) => p.subscriptionUrl === subURL);

        if (!oneSubscription) {
          await handleBulkSaveProxies(named, setActiveTab, protocol);
          return;
        }

        const label = subscriptionLabelFromURL(subURL);
        let entries;
        let allowInsecure = false;
        for (;;) {
          try {
            entries = await wailsAPI.addSubscription(label, subURL, allowInsecure, "", disabled);
            break;
          } catch (err) {
            if (isInsecureSubscriptionError(err) && !allowInsecure) {
              if (!(await askInsecure())) return;
              allowInsecure = true;
              continue;
            }
            /* Подписка уже заведена — обновляем её вместо повторного добавления. */
            if (String(err?.message || err || "").includes("уже добавлена")) {
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
          name: p.name || `${stamp()}-${i}`,
        }));
        await handleBulkSaveProxies(withNames, setActiveTab, protocol);

        const cfg = await wailsAPI.getConfig();
        if (cfg.subscriptions) setSubscriptions(cfg.subscriptions);
        if (cfg?.routingRules?.routingLists) syncRoutingLists(cfg.routingRules.routingLists);
      } catch (err) {
        console.error("Import failed:", err);
        failed(err, t("add.subscriptionError"));
      } finally {
        setBusy(false);
      }
    },
    [
      askInsecure,
      failed,
      handleBulkSaveProxies,
      preview,
      setActiveTab,
      setSubscriptions,
      syncRoutingLists,
      t,
    ],
  );

  return { busy, preview, importText, confirm, cancel };
}
