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

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { BrowserOpenURL, ClipboardSetText } from "../../../wailsjs/runtime/runtime";
import { useConfigContext } from "../../context/ConfigContext";
import { PAGE_BUY, useScrollMemory } from "../../hooks/usePageMemory";
import wailsAPI from "../../utils/wailsAPI";
import AppSidebar from "./AppSidebar";
import BuyPage from "./BuyPage";

const BOT_URL = "https://t.me/result_vpn_bot";
const SUBSCRIPTION_ORIGINS = new Set(["https://result-service.ru", "https://cdn.result-proxy.ru:8443"]);
const COPIED_MS = 2000;

function isResultVSubscription(subscription) {
  try {
    const { origin, pathname } = new URL(subscription.url);
    return SUBSCRIPTION_ORIGINS.has(origin) && /^\/s\/[^/]+/.test(pathname);
  } catch {
    return false;
  }
}

export default function BuyScreen() {
  const { t, i18n } = useTranslation();
  const { subscriptions, proxies } = useConfigContext();
  const [copyFeedback, setCopyFeedback] = useState(null);
  const [connectedURL, setConnectedURL] = useState("");
  const [connecting, setConnecting] = useState(false);
  const [connectionError, setConnectionError] = useState("");
  const copiedTimer = useRef(0);
  const contentRef = useScrollMemory(PAGE_BUY);

  useEffect(() => () => clearTimeout(copiedTimer.current), []);

  const officialSubscriptions = subscriptions.filter((item) =>
    isResultVSubscription(item) &&
    proxies.some((entry) => entry.subscriptionUrl === item.url && entry.ip && entry.type !== "SECTION"),
  );
  const now = Date.now() / 1000;
  const subscription = officialSubscriptions.find((item) => item.url === connectedURL)
    ?? officialSubscriptions.find((item) => !item.expireUnix || item.expireUnix > now)
    ?? officialSubscriptions[0];
  const url = subscription?.url;
  const copyMessage = copyFeedback && copyFeedback.url === url ? t(copyFeedback.key) : "";
  const canCancel = connecting || connectionError === "telegram_connect_busy";

  const connectTelegram = async () => {
    setConnectionError("");
    setConnecting(true);
    try {
      setConnectedURL(await wailsAPI.connectTelegramSubscription(i18n.resolvedLanguage || "en"));
    } catch (err) {
      setConnectionError(String(err?.message || err));
    } finally {
      setConnecting(false);
    }
  };

  const cancelTelegram = async () => {
    await wailsAPI.cancelTelegramSubscription();
    setConnectionError((current) => current === "telegram_connect_busy" ? "" : current);
  };

  const copySubscription = async () => {
    let copied = false;
    try {
      copied = await ClipboardSetText(url);
    } catch {
      // The system clipboard can be temporarily unavailable.
    }
    clearTimeout(copiedTimer.current);
    setCopyFeedback({ url, key: copied ? "buy.copied" : "buy.copyFailed" });
    copiedTimer.current = setTimeout(() => setCopyFeedback(null), COPIED_MS);
  };

  return (
    <BuyPage
      subscriptionURL={url}
      copyMessage={copyMessage}
      connecting={connecting}
      canCancel={canCancel}
      onConnectTelegram={connectTelegram}
      onCancelTelegram={cancelTelegram}
      onOpenBot={() => BrowserOpenURL(BOT_URL)}
      onCopySubscription={copySubscription}
      text={{
        title: t("buy.title"),
        subtitle: t("buy.subtitle"),
        offerTitle: t("buy.offerTitle"),
        offerDescription: t("buy.offerDescription"),
        connect: t("buy.connect"),
        connecting: t("buy.connecting"),
        cancel: t("buy.cancel"),
        connectionError: connectionError ? t(`buy.connectionErrors.${connectionError}`, {
          defaultValue: t("buy.connectionErrors.telegram_unavailable"),
        }) : "",
        purchase: t("buy.purchase"),
        subscriptionTitle: t("buy.subscriptionTitle", {
          name: subscription?.name || "ResultV",
        }),
        subscriptionDescription: t("buy.subscriptionAdded"),
        copySubscription: t("buy.copySubscription"),
      }}
      sidebar={<AppSidebar />}
      contentRef={contentRef}
    />
  );
}
