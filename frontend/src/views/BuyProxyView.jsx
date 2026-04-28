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

import React, { useState } from "react";
import { BrowserOpenURL } from "../../wailsjs/runtime/runtime";
import { ExternalLink, Check, Copy } from "lucide-react";
import { useTranslation } from "react-i18next";
import p6logo from "../assets/p6logo.png";
import pmlogo from "../assets/pmlogo.png";
import implogo from "../assets/implogo.png";
import psellerlogo from "../assets/pseller.png";

const VPN_PARTNERS = [
  {
    id: "imp_vpn",
    link: "https://t.me/impVPNBot?start=NzQ3MDczMjUz",
    promoCode: "result",
    logo: implogo,
    isBot: true,
  },
];

const PROXY_PARTNERS = [
  {
    id: "proxy_seller",
    link: "https://proxy-seller.com/?partner=4TZ2AXZ85WHSQT",
    promoCode: "YZDPUK_1131267",
    logo: psellerlogo,
  },
  {
    id: "proxy6",
    link: "https://proxy6.net/?r=833290",
    promoCode: "resultproxy",
    logo: p6logo,
  },
  {
    id: "proxy_market",
    link: "https://ru.dashboard.proxy.market/?ref=resultproxy",
    promoCode: "resultproxy",
    logo: pmlogo,
  },
];

export const BuyProxyView = () => {
  const { t } = useTranslation();
  const [copiedLink, setCopiedLink] = useState(null);
  const [copiedPromo, setCopiedPromo] = useState(null);

  const handleCopyAndGo = (link, partnerId) => {
    navigator.clipboard.writeText(link);
    setCopiedLink(partnerId);
    setTimeout(() => setCopiedLink(null), 2000);
    BrowserOpenURL(link);
  };

  const handleCopyPromo = (promoCode, partnerId) => {
    navigator.clipboard.writeText(promoCode);
    setCopiedPromo(partnerId);
    setTimeout(() => setCopiedPromo(null), 2000);
  };

  const renderPartner = (partner) => (
    <div
      key={partner.id}
      className="group relative bg-zinc-900/40 backdrop-blur-xl p-6 rounded-3xl border border-zinc-800/50 hover:border-[#007E3A]/30 transition-all duration-300 flex flex-col gap-6"
    >
      <div className="absolute inset-0 bg-gradient-to-br from-[#007E3A]/5 to-transparent opacity-0 group-hover:opacity-100 transition-opacity rounded-3xl"></div>

      <div className="flex items-center gap-6 z-10">
        <div className="relative shrink-0 bg-zinc-950 p-4 rounded-2xl border border-zinc-800 group-hover:border-[#007E3A]/20 transition-colors shadow-2xl">
          <img
            src={partner.logo}
            alt={partner.id}
            className="w-12 h-12 object-contain filter grayscale group-hover:grayscale-0 transition-all duration-500"
          />
        </div>

        <div className="flex-1 min-w-0">
          <h3 className="text-lg font-bold text-white mb-1 group-hover:text-[#00A819] transition-colors">
            {t(`buy.${partner.id}.discount`)}
          </h3>
          <p className="text-zinc-500 text-sm line-clamp-3 leading-relaxed">
            {t(`buy.${partner.id}.discount_desc`)}
          </p>
        </div>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 w-full z-10">
        <button
          onClick={() => handleCopyAndGo(partner.link, partner.id)}
          className="flex items-center justify-center space-x-2 px-5 py-3.5 bg-[#007E3A] hover:bg-[#00A819] text-white rounded-xl font-bold transition-all active:scale-[0.98] shadow-lg shadow-[#007E3A]/20"
        >
          {copiedLink === partner.id ? (
            <Check className="w-4 h-4" />
          ) : (
            <ExternalLink className="w-4 h-4" />
          )}
          <span>
            {copiedLink === partner.id
              ? t("buy.copied")
              : partner.isBot
                ? t("buy.goBot")
                : t("buy.go")}
          </span>
        </button>

        <button
          onClick={() => handleCopyPromo(partner.promoCode, partner.id)}
          className="flex items-center justify-center space-x-2 px-5 py-3.5 bg-zinc-800 hover:bg-zinc-700 text-zinc-300 rounded-xl font-bold transition-all border border-zinc-700/50 active:scale-[0.98]"
        >
          {copiedPromo === partner.id ? (
            <Check className="w-4 h-4 text-[#00A819]" />
          ) : (
            <Copy className="w-4 h-4" />
          )}
          <div className="flex flex-col items-start leading-none gap-1">
            <span className="text-[10px] text-zinc-500 font-medium tracking-tight">
              {t(`buy.${partner.id}.promo_title`)}
            </span>
            <span className="text-xs font-mono uppercase tracking-widest font-bold">
              {copiedPromo === partner.id
                ? t("buy.copied")
                : partner.promoCode}
            </span>
          </div>
        </button>
      </div>
    </div>
  );

  return (
    <div className="space-y-8 animate-in fade-in slide-in-from-bottom-4 duration-500">
      <div className="space-y-4">
        <div className="space-y-1">
          <h2 className="text-3xl font-bold text-white">
            {t("buy.vpn_section_title")}
          </h2>
          <p className="text-zinc-400 text-sm leading-relaxed">
            {t("buy.vpn_section_desc")}
          </p>
        </div>
        <div className="grid gap-4">{VPN_PARTNERS.map(renderPartner)}</div>
      </div>

      <div className="space-y-4">
        <div className="space-y-1">
          <h2 className="text-3xl font-bold text-white">
            {t("buy.proxy_section_title")}
          </h2>
          <p className="text-zinc-400 text-sm leading-relaxed">
            {t("buy.proxy_section_desc")}
          </p>
        </div>
        <div className="grid gap-4">{PROXY_PARTNERS.map(renderPartner)}</div>
      </div>
    </div>
  );
};
