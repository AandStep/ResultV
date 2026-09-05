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
 * Страница «Купить сервера», подключённая к приложению.
 *
 * Вид держит BuyPage, здесь только партнёры и три действия над ними: открыть
 * бота, открыть сайт и скопировать промокод. Ссылки уходят во внешний
 * браузер — открывать их внутри окна приложения незачем.
 */

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { BrowserOpenURL } from "../../../wailsjs/runtime/runtime";
import implogo from "../../assets/implogo.png";
import BuyPage from "./BuyPage";
import AppSidebar from "./AppSidebar";

/* Ссылки те же, что были на прежней странице покупки. */
const PARTNERS = [
  {
    id: "imp_vpn",
    botLink: "https://t.me/impVPNBot?start=NzQ3MDczMjUz",
    siteLink: "https://my.impio.space/?ref=NzQ3MDczMjUz",
    promo: "result",
    logo: implogo,
  },
];

/* Столько держится подпись «Скопировано!» на кнопке промокода. */
const COPIED_MS = 2000;

export default function BuyScreen() {
  const { t } = useTranslation();
  const [copiedId, setCopiedId] = useState("");
  const copiedTimer = useRef(0);

  useEffect(() => () => clearTimeout(copiedTimer.current), []);

  const copyPromo = (partner) => {
    navigator.clipboard.writeText(partner.promo);
    setCopiedId(partner.id);
    clearTimeout(copiedTimer.current);
    copiedTimer.current = setTimeout(() => setCopiedId(""), COPIED_MS);
  };

  const partners = PARTNERS.map((partner) => ({
    ...partner,
    title: t(`buy.${partner.id}.discount`),
    desc: t(`buy.${partner.id}.discount_desc`),
  }));

  return (
    <BuyPage
      title={t("buy.title")}
      subtitle={t("buy.subtitle")}
      partners={partners}
      copiedId={copiedId}
      onOpenBot={(partner) => BrowserOpenURL(partner.botLink)}
      onOpenSite={(partner) => BrowserOpenURL(partner.siteLink)}
      onCopyPromo={copyPromo}
      text={{
        bot: t("buy.goBot"),
        site: t("buy.goSite"),
        copyPromo: t("buy.copy"),
        copied: t("buy.copied"),
      }}
      sidebar={<AppSidebar />}
    />
  );
}
