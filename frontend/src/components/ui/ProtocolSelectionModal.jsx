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

import React, { useState, useMemo } from "react";
import { createPortal } from "react-dom";
import { Info, Check, Shield } from "lucide-react";
import { useTranslation } from "react-i18next";
import { VPN_TYPES } from "../../utils/proxyParser";

// Mirrors RulesView's ROUTING_ACTION_STYLES — kept as a local copy rather
// than a cross-view import so this modal stays a self-contained component.
const ROUTING_ACTION_STYLES = {
  proxy: "bg-[#007E3A]/10 text-[#00A819] border-[#007E3A]/20",
  direct: "bg-zinc-800 text-zinc-300 border-zinc-700",
  block: "bg-rose-500/10 text-rose-400 border-rose-500/20",
};

const ProtocolSelectionModal = ({
  isOpen,
  onClose,
  onConfirm,
  count,
  proxies = [],
  routingLists = [],
  disabledListUrls,
  onToggleListDisabled,
}) => {
  const { t } = useTranslation();
  const [selectedType, setSelectedType] = useState("HTTP");
  const disabledSet =
    disabledListUrls instanceof Set ? disabledListUrls : new Set();

  const { plainProxies, vpnProxies, vpnSummary, visibleCount } = useMemo(() => {
    const autoMemberIds = new Set();
    proxies.forEach((p) => {
      if (p.type === "AUTO" && p.extra?.members) {
        p.extra.members.forEach((id) => autoMemberIds.add(id));
      }
    });

    const visibleProxies = proxies.filter(p => !autoMemberIds.has(p.id));
    const plain = visibleProxies.filter(
        (p) =>
            !VPN_TYPES.includes(p.type) &&
            p.type !== "AUTO" &&
            p.type !== "SECTION",
    );
    const vpn = visibleProxies.filter((p) => VPN_TYPES.includes(p.type) || p.type === "AUTO");
    
    const summary = {};
    vpn.forEach((p) => {
      summary[p.type] = (summary[p.type] || 0) + 1;
    });
    
    return { 
      plainProxies: plain, 
      vpnProxies: vpn, 
      vpnSummary: summary, 
      visibleCount: visibleProxies.length 
    };
  }, [proxies]);

  if (!isOpen) return null;

  const hasPlain = plainProxies.length > 0;
  const hasVpn = vpnProxies.length > 0;
  const onlyVpn = hasVpn && !hasPlain;

  const handleConfirm = () => {
    onConfirm(onlyVpn ? null : selectedType);
  };

  return createPortal(
    <div className="fixed inset-0 z-[9999] flex items-center justify-center p-4 animate-in fade-in duration-300">
      <div
        className="absolute inset-0 bg-black/70 backdrop-blur-md"
        onClick={onClose}
      />
      <div className="relative bg-zinc-900 border border-zinc-800 w-full max-w-md p-6 rounded-3xl shadow-2xl animate-in zoom-in-95 duration-300 flex flex-col space-y-6">
        <div className="flex flex-col items-center text-center space-y-4">
          <div className="w-16 h-16 bg-[#007E3A]/10 rounded-full flex items-center justify-center">
            <Info className="w-8 h-8 text-[#007E3A]" />
          </div>
          <div className="space-y-2">
            <h3 className="text-xl font-bold text-white">
              {t("add.protocolSelectionTitle")}
            </h3>
            {hasPlain && (
              <p className="text-zinc-400 text-sm leading-relaxed">
                {t("add.protocolSelectionDesc")}
              </p>
            )}
          </div>
        </div>

        {hasVpn && (
          <div className="bg-zinc-950 rounded-2xl p-4 border border-zinc-800 space-y-3">
            <div className="flex items-center space-x-2 text-[#007E3A]">
              <Shield className="w-5 h-5" />
              <span className="text-sm font-bold">
                {t("add.vpnDetected", { count: vpnProxies.length }) ||
                  `Обнаружено ${vpnProxies.length} VPN-серверов`}
              </span>
            </div>
            <div className="flex flex-wrap gap-2">
              {Object.entries(vpnSummary).map(([type, cnt]) => (
                <span
                  key={type}
                  className="text-xs px-3 py-1 rounded-lg bg-[#007E3A]/10 text-[#00A819] font-bold"
                >
                  {type} ({cnt})
                </span>
              ))}
            </div>
            <p className="text-zinc-500 text-xs">
              {t("add.vpnKeepTypes") ||
                "VPN-протоколы определены автоматически из ссылок"}
            </p>
          </div>
        )}

        {hasPlain && (
          <div className="space-y-3">
            {hasVpn && (
              <p className="text-sm text-zinc-400 font-medium">
                {t("add.plainProxiesProtocol", { count: plainProxies.length }) ||
                  `Выберите протокол для ${plainProxies.length} обычных прокси:`}
              </p>
            )}
            <div className="grid grid-cols-1 gap-3">
              {["HTTP", "HTTPS", "SOCKS5"].map((type) => (
                <button
                  key={type}
                  onClick={() => setSelectedType(type)}
                  className={`flex items-center justify-between px-6 py-4 rounded-2xl border transition-all ${
                    selectedType === type
                      ? "bg-[#007E3A]/10 border-[#007E3A] text-white shadow-lg shadow-[#007E3A]/5"
                      : "bg-zinc-950 border-zinc-800 text-zinc-400 hover:border-zinc-700"
                  }`}
                >
                  <span className="font-bold">{type}</span>
                  {selectedType === type && (
                    <Check className="w-5 h-5 text-[#007E3A]" />
                  )}
                </button>
              ))}
            </div>
          </div>
        )}

        {routingLists.length > 0 && (
          <div className="space-y-3 pt-2 border-t border-zinc-800">
            <p className="text-sm text-zinc-400 font-medium pt-4">
              {t("routingLists.subscriptionAddsTitle", {
                count: routingLists.length,
              })}
            </p>
            <div className="space-y-2 max-h-48 overflow-y-auto pr-1">
              {routingLists.map((rl) => {
                const enabled = !disabledSet.has(rl.url);
                return (
                  <div
                    key={rl.url}
                    className="flex items-center gap-3 p-3 bg-zinc-950 rounded-xl border border-zinc-800"
                  >
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="text-white text-sm font-bold truncate">
                          {rl.name || rl.url}
                        </span>
                        <span
                          className={`shrink-0 text-xs font-medium px-2.5 py-0.5 rounded-full border ${ROUTING_ACTION_STYLES[rl.action] || ROUTING_ACTION_STYLES.direct}`}
                        >
                          {t(
                            `routingLists.action${rl.action?.charAt(0).toUpperCase()}${rl.action?.slice(1)}`,
                          )}
                        </span>
                      </div>
                    </div>
                    <div
                      className={`relative w-12 h-7 rounded-2xl transition-colors duration-300 ease-in-out shrink-0 cursor-pointer ${enabled ? "bg-[#007E3A]" : "bg-zinc-700"}`}
                      onClick={() => onToggleListDisabled?.(rl.url, enabled)}
                      role="button"
                      aria-pressed={enabled}
                    >
                      <div
                        className={`absolute top-1 left-1 bg-white w-5 h-5 rounded-full transition-transform duration-300 ease-in-out ${enabled ? "transform translate-x-5" : ""}`}
                      ></div>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        )}

        <div className="pt-2 flex space-x-3">
          <button
            onClick={onClose}
            className="flex-1 bg-zinc-800 hover:bg-zinc-700 text-white font-bold py-4 rounded-2xl transition-all"
          >
            {t("add.cancel")}
          </button>
          <button
            onClick={handleConfirm}
            className="flex-[2] bg-[#007E3A] hover:bg-[#005C2A] text-white font-bold py-4 rounded-2xl transition-all shadow-lg shadow-[#007E3A]/20"
          >
            {t("add.confirmImport", { count: visibleCount })}
          </button>
        </div>
      </div>
    </div>,
    document.body
  );
};

export default ProtocolSelectionModal;
