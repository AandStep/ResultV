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

import React, { useState, useEffect } from "react";
import { createPortal } from "react-dom";
import { Lock, Link2 } from "lucide-react";
import { useConfigContext } from "../context/ConfigContext";
import { useConnectionContext } from "../context/ConnectionContext";
import { useTranslation } from "react-i18next";
import {
  parseProxies,
  isSubscriptionURL,
  isEncryptedSubscription,
  isVpnType,
  subscriptionLabelFromURL,
  parseProxyExtra,
  normalizeNetworkForSelect,
  normalizeSecurityForSelect,
  VPN_NETWORK_OPTIONS,
  sanitizeVpnExtraForEdit,
  readVpnTransportFieldsFromExtra,
  applyVpnTransportFieldsToExtra,
} from "../utils/proxyParser";
import { FileUp, ClipboardList } from "lucide-react";
import ProtocolSelectionModal from "../components/ui/ProtocolSelectionModal";
import AppSelect from "../components/ui/AppSelect";
import wailsAPI from "../utils/wailsAPI";
import { isInsecureSubscriptionError } from "../utils/subscriptionSecurity";

const PLAIN_TYPES = ["HTTP", "HTTPS", "SOCKS5"];
const VPN_TYPES_LIST = ["VLESS", "VMESS", "TROJAN", "SS", "WIREGUARD", "AMNEZIAWG", "HYSTERIA2", "NAIVEPROXY"];
const VPN_SECURITY_OPTIONS = ["none", "tls", "reality"];

const AMNEZIA_INT_KEYS = ["jc", "jmin", "jmax", "s1", "s2", "s3", "s4", "h1", "h2", "h3", "h4", "itime"];
const AMNEZIA_STR_KEYS = ["i1", "i2", "i3", "i4", "i5", "j1", "j2", "j3"];

const EMPTY_AMNEZIA_FIELDS = (() => {
  const o = {};
  for (const k of AMNEZIA_INT_KEYS) o[k] = "";
  for (const k of AMNEZIA_STR_KEYS) o[k] = "";
  return o;
})();

const amneziaFieldsFromObject = (raw) => {
  const out = { ...EMPTY_AMNEZIA_FIELDS };
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return out;
  for (const k of [...AMNEZIA_INT_KEYS, ...AMNEZIA_STR_KEYS]) {
    const v = raw[k];
    if (v === undefined || v === null || v === "") continue;
    out[k] = String(v);
  }
  return out;
};

const amneziaObjectFromFields = (fields) => {
  const obj = {};
  for (const k of AMNEZIA_INT_KEYS) {
    const s = String(fields?.[k] ?? "").trim();
    if (s === "") continue;
    const n = Number(s);
    if (Number.isFinite(n) && n >= 0) obj[k] = n;
  }
  for (const k of AMNEZIA_STR_KEYS) {
    const s = String(fields?.[k] ?? "").trim();
    if (s) obj[k] = s;
  }
  return obj;
};

const AmneziaEditor = ({ fields, onFieldsChange, useRaw, onUseRawChange, rawValue, onRawChange, t }) => {
  const inputCls =
    "w-full bg-zinc-950 border border-zinc-800 rounded-lg px-3 py-2 text-white font-mono text-xs outline-none focus:border-[#007E3A] transition-colors";

  const Fld = ({ k, label, placeholder = "" }) => (
    <div>
      <label className="block text-[11px] uppercase tracking-wider text-zinc-500 mb-1">{label}</label>
      <input
        type="text"
        inputMode={AMNEZIA_STR_KEYS.includes(k) ? "text" : "numeric"}
        className={inputCls}
        value={fields[k]}
        onChange={(e) => onFieldsChange({ ...fields, [k]: e.target.value })}
        placeholder={placeholder}
      />
    </div>
  );

  const Group = ({ title, children }) => (
    <div>
      <p className="text-[11px] font-medium text-zinc-500 uppercase tracking-wider mb-2">{title}</p>
      {children}
    </div>
  );

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <label className="text-sm font-medium text-zinc-400">
          {t("add.awgAmneziaTitle") || "AmneziaWG обфускация"}
        </label>
        <button
          type="button"
          onClick={() => onUseRawChange(!useRaw)}
          className="text-xs text-[#007E3A] hover:underline outline-none focus:outline-none"
        >
          {useRaw
            ? t("add.awgUseFields") || "Поля"
            : t("add.awgUseJSON") || "JSON"}
        </button>
      </div>

      {useRaw ? (
        <textarea
          className="w-full h-28 bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-xs outline-none focus:border-[#007E3A] transition-colors resize-none"
          value={rawValue}
          onChange={(e) => onRawChange(e.target.value)}
          placeholder='{"jc":4,"jmin":1,"jmax":3,"s1":15,"s2":13}'
        />
      ) : (
        <>
          <Group title={t("add.awgGroupHandshake") || "Junk пакеты handshake"}>
            <div className="grid grid-cols-3 gap-2">
              <Fld k="jc" label="Jc" placeholder="4" />
              <Fld k="jmin" label="Jmin" placeholder="1" />
              <Fld k="jmax" label="Jmax" placeholder="3" />
            </div>
          </Group>

          <Group title={t("add.awgGroupSizes") || "Размеры пакетов S1–S4"}>
            <div className="grid grid-cols-4 gap-2">
              <Fld k="s1" label="S1" />
              <Fld k="s2" label="S2" />
              <Fld k="s3" label="S3" />
              <Fld k="s4" label="S4" />
            </div>
          </Group>

          <Group title={t("add.awgGroupHeaders") || "Магические заголовки H1–H4"}>
            <div className="grid grid-cols-4 gap-2">
              <Fld k="h1" label="H1" />
              <Fld k="h2" label="H2" />
              <Fld k="h3" label="H3" />
              <Fld k="h4" label="H4" />
            </div>
          </Group>

          <Group title={t("add.awgGroupSpecial") || "AWG 2.0: special junk I1–I5 / Itime"}>
            <div className="grid grid-cols-2 gap-2">
              <Fld k="i1" label="I1" />
              <Fld k="i2" label="I2" />
              <Fld k="i3" label="I3" />
              <Fld k="i4" label="I4" />
              <Fld k="i5" label="I5" />
              <Fld k="itime" label="Itime" placeholder="сек" />
            </div>
          </Group>

          <Group title={t("add.awgGroupHandshakeJunk") || "AWG 2.0: handshake junk J1–J3"}>
            <div className="grid grid-cols-3 gap-2">
              <Fld k="j1" label="J1" />
              <Fld k="j2" label="J2" />
              <Fld k="j3" label="J3" />
            </div>
          </Group>
        </>
      )}
    </div>
  );
};

export const AddProxyView = () => {
  const { t } = useTranslation();
  const {
    handleSaveProxy,
    handleBulkSaveProxies,
    editingProxy,
    setEditingProxy,
    setActiveTab,
    setSubscriptions,
    syncRoutingLists,
    showAlertDialog,
    showConfirmDialog,
    setPendingDeepLink,
    setPendingDeepLinkSource,
  } = useConfigContext();
  const {
    activeProxy,
    failedProxy,
    setFailedProxy,
    setActiveProxy,
    isConnected,
    selectAndConnect,
  } = useConnectionContext();

  const onCancel = () => {
    setEditingProxy(null);
    setActiveTab("list");
  };

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
  const [pendingRoutingLists, setPendingRoutingLists] = useState([]);
  const [disabledListUrls, setDisabledListUrls] = useState(new Set());
  const [showSelectionModal, setShowSelectionModal] = useState(false);
  const [vpnUri, setVpnUri] = useState("");
  const [vpnLoading, setVpnLoading] = useState(false);

  const handleClipboardImport = async () => {
    try {
      const text = await navigator.clipboard.readText();
      if (!text) {
        showAlertDialog({
          title: t("common.notice"),
          message: t("add.clipboardEmpty") || "Clipboard is empty",
          variant: "warning",
        });
        return;
      }
      setBulkText(text);
      await processImport(text);
    } catch (err) {
      console.error("Failed to read clipboard:", err);
      showAlertDialog({
        title: t("common.error"),
        message:
          t("add.clipboardError") ||
          "Could not read from clipboard. Please paste manually.",
        variant: "danger",
      });
      setImportMode("bulk");
    }
  };

  const processImport = async (text, sourceName = "") => {
    const trimmed = (text || "").trim();
    // resultv:// deep links: hand off to DeepLinkImportModal so paste and
    // browser-click flows share one code path (fetchSubscription with the
    // insecure-http consent prompt, then AddSubscription). Without this,
    // pasted deep links would skip the subscription header creation that
    // the browser flow performs.
    if (/^resultv:(\/\/)?/i.test(trimmed)) {
      setPendingDeepLinkSource(
        /^resultv:(\/\/)?rvsub\//i.test(trimmed) ? "rvsub" : "",
      );
      setPendingDeepLink(trimmed);
      setBulkText("");
      setImportMode("single");
      return;
    }
    if (isSubscriptionURL(text)) {
      setIsImporting(true);
      try {
        let preview;
        try {
          preview = await wailsAPI.fetchSubscription(text.trim());
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
            showAlertDialog({
              title: t("common.notice"),
              message: t("add.insecureSubscriptionCancelled"),
              variant: "warning",
            });
            return;
          }
          preview = await wailsAPI.fetchSubscription(text.trim(), true);
        }
        const entries = preview?.proxies || [];
        const lists = preview?.routingLists || [];
        if (!entries || entries.length === 0) {
          showAlertDialog({
            title: t("common.notice"),
            message: t("add.noProxiesFound") || "No proxies found.",
            variant: "warning",
          });
          return;
        }
        setPendingProxies(entries);
        setPendingRoutingLists(lists);
        setDisabledListUrls(new Set());
        setShowSelectionModal(true);
      } catch (err) {
        console.error("Subscription fetch error:", err);
        showAlertDialog({
          title: t("common.error"),
          message: `${t("add.subscriptionError") || "Subscription fetch error"}: ${err}`,
          variant: "danger",
        });
      } finally {
        setIsImporting(false);
      }
      return;
    }

    if (isEncryptedSubscription(text)) {
      setIsImporting(true);
      try {
        const preview = await wailsAPI.parseSubscriptionText(text.trim());
        const entries = preview?.proxies || [];
        const lists = preview?.routingLists || [];
        if (!entries || entries.length === 0) {
          showAlertDialog({
            title: t("common.notice"),
            message: t("add.noProxiesFound") || "No proxies found.",
            variant: "warning",
          });
          return;
        }
        setPendingProxies(entries);
        setPendingRoutingLists(lists);
        setDisabledListUrls(new Set());
        setShowSelectionModal(true);
      } catch (err) {
        console.error("Encrypted subscription parse error:", err);
        const detail = err?.message || String(err);
        showAlertDialog({
          title: t("common.error"),
          message: `${t("add.subscriptionError") || "Failed to decrypt subscription."}\n\n${detail}`,
          variant: "danger",
        });
      } finally {
        setIsImporting(false);
      }
      return;
    }

    const proxiesToImport = parseProxies(text).map((proxy) => {
      if (
        (proxy?.type === "WIREGUARD" || proxy?.type === "AMNEZIAWG") &&
        sourceName &&
        sourceName.toLowerCase().endsWith(".conf") &&
        (!proxy.name || proxy.name === "WireGuard" || proxy.name === "AmneziaWG")
      ) {
        const base = sourceName.replace(/\.[^.]+$/, "");
        return { ...proxy, name: base || proxy.name };
      }
      return proxy;
    });
    if (proxiesToImport.length === 0) {
      // The client-side parser only handles single links and plaintext lists.
      // A base64-encoded subscription body or a JSON sing-box/Clash export
      // pasted directly lands here — defer to the backend parser, which decodes
      // base64 and JSON. Without this, the most common real subscription format
      // (a bare base64 blob) silently reported "no proxies".
      setIsImporting(true);
      try {
        const preview = await wailsAPI.parseSubscriptionText(text.trim());
        const entries = preview?.proxies || [];
        const lists = preview?.routingLists || [];
        if (entries && entries.length > 0) {
          setPendingProxies(entries);
          setPendingRoutingLists(lists);
          setDisabledListUrls(new Set());
          setShowSelectionModal(true);
          return;
        }
      } catch (err) {
        console.error("Subscription text parse error:", err);
      } finally {
        setIsImporting(false);
      }
      showAlertDialog({
        title: t("common.notice"),
        message: t("add.noProxiesFound") || "No proxies found.",
        variant: "warning",
      });
      return;
    }

    setPendingProxies(proxiesToImport);
    setPendingRoutingLists([]);
    setDisabledListUrls(new Set());
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
        let allowInsecure = false;
        for (;;) {
          try {
            entries = await wailsAPI.addSubscription(
              label,
              subURL,
              allowInsecure,
              "",
              Array.from(disabledListUrls),
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
                showAlertDialog({
                  title: t("common.notice"),
                  message: t("add.insecureSubscriptionCancelled"),
                  variant: "warning",
                });
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

      setBulkText("");
      setImportMode("single");
      setPendingProxies([]);
      setPendingRoutingLists([]);
      setDisabledListUrls(new Set());
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
      await processImport(event.target.result, file.name || "");
    };
    reader.readAsText(file);
    e.target.value = "";
  };

  const handleVpnUriSubmit = async (overrideText) => {
    const override = typeof overrideText === "string" ? overrideText : null;
    let text = (override ?? vpnUri).trim();
    if (!text) return;

    if (isSubscriptionURL(text)) {
      setVpnLoading(true);
      try {
        let preview;
        try {
          preview = await wailsAPI.fetchSubscription(text);
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
            showAlertDialog({
              title: t("common.notice"),
              message: t("add.insecureSubscriptionCancelled"),
              variant: "warning",
            });
            return;
          }
          preview = await wailsAPI.fetchSubscription(text, true);
        }
        const entries = preview?.proxies || [];
        const lists = preview?.routingLists || [];
        if (!entries || entries.length === 0) {
          showAlertDialog({
            title: t("common.notice"),
            message: t("add.noProxiesFound") || "No proxies found.",
            variant: "warning",
          });
          return;
        }
        setPendingProxies(entries);
        setPendingRoutingLists(lists);
        setDisabledListUrls(new Set());
        setShowSelectionModal(true);
      } catch (err) {
        console.error("Subscription fetch error:", err);
        showAlertDialog({
          title: t("common.error"),
          message: `${t("add.subscriptionError") || "Subscription fetch error"}: ${err}`,
          variant: "danger",
        });
      } finally {
        setVpnLoading(false);
      }
      return;
    }

    if (isEncryptedSubscription(text)) {
      setVpnLoading(true);
      try {
        const preview = await wailsAPI.parseSubscriptionText(text);
        const entries = preview?.proxies || [];
        const lists = preview?.routingLists || [];
        if (!entries || entries.length === 0) {
          showAlertDialog({
            title: t("common.notice"),
            message: t("add.noProxiesFound") || "No proxies found.",
            variant: "warning",
          });
          return;
        }
        setPendingProxies(entries);
        setPendingRoutingLists(lists);
        setDisabledListUrls(new Set());
        setShowSelectionModal(true);
      } catch (err) {
        console.error("Encrypted subscription parse error:", err);
        const detail = err?.message || String(err);
        showAlertDialog({
          title: t("common.error"),
          message: `${t("add.subscriptionError") || "Failed to decrypt subscription."}\n\n${detail}`,
          variant: "danger",
        });
      } finally {
        setVpnLoading(false);
      }
      return;
    }

    const parsed = parseProxies(text);
    if (parsed.length === 0) {
      showAlertDialog({
        title: t("common.notice"),
        message: t("add.noProxiesFound") || "No proxies found.",
        variant: "warning",
      });
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
      setPendingRoutingLists([]);
      setDisabledListUrls(new Set());
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

  const [vpnNetwork, setVpnNetwork] = useState("tcp");
  const [vpnSecurity, setVpnSecurity] = useState("none");
  const [vpnUuid, setVpnUuid] = useState("");
  const [vpnSsMethod, setVpnSsMethod] = useState("aes-256-gcm");
  const [vpnTf, setVpnTf] = useState({
    transPath: "/",
    transHost: "",
    grpcService: "",
    httpHost: "",
    httpPath: "/",
    xhttpMode: "auto",
  });
  const [vpnNetworkBaseline, setVpnNetworkBaseline] = useState("tcp");

  const [hy2, setHy2] = useState({
    password: "",
    sni: "",
    alpn: "h3",
    insecure: false,
    upMbps: "",
    downMbps: "",
    obfsType: "",
    obfsPassword: "",
  });

  const [naive, setNaive] = useState({
    sni: "",
    insecure: false,
    listen: "",
    log: "",
  });

  const [wg, setWg] = useState({
    address: "10.0.0.2/32",
    privateKey: "",
    publicKey: "",
    preSharedKey: "",
    allowedIps: "0.0.0.0/0",
    reserved: "",
    keepalive: "",
    system: false,
    name: "",
    mtu: "",
    amnezia: { ...EMPTY_AMNEZIA_FIELDS },
    amneziaJSON: "",
    amneziaUseRaw: false,
  });

  const handleAmneziaFieldsChange = (next) => setWg((w) => ({ ...w, amnezia: next }));
  const handleAmneziaRawChange = (next) => setWg((w) => ({ ...w, amneziaJSON: next }));
  const handleAmneziaUseRawChange = (next) => {
    setWg((w) => {
      if (next) {
        const obj = amneziaObjectFromFields(w.amnezia);
        const json = Object.keys(obj).length ? JSON.stringify(obj, null, 2) : w.amneziaJSON;
        return { ...w, amneziaUseRaw: true, amneziaJSON: json };
      }
      const raw = (w.amneziaJSON || "").trim();
      if (raw) {
        try {
          const obj = JSON.parse(raw);
          if (obj && typeof obj === "object" && !Array.isArray(obj)) {
            return { ...w, amneziaUseRaw: false, amnezia: amneziaFieldsFromObject(obj) };
          }
        } catch {}
      }
      return { ...w, amneziaUseRaw: false };
    });
  };

  const isVpnMode = VPN_TYPES_LIST.includes(formData.type);
  const isVpnEditMode =
    editingProxy != null &&
    VPN_TYPES_LIST.includes(String(editingProxy.type || "").toUpperCase());

  useEffect(() => {
    if (editingProxy) {
      setFormData(editingProxy);
      if (
        VPN_TYPES_LIST.includes(String(editingProxy.type || "").toUpperCase())
      ) {
        const ex = parseProxyExtra(editingProxy.extra);
        setVpnNetwork(normalizeNetworkForSelect(ex.network));
        setVpnSecurity(normalizeSecurityForSelect(ex.security));
        setVpnUuid(ex.uuid || "");
        setVpnSsMethod(ex.method || "aes-256-gcm");
        setVpnTf(readVpnTransportFieldsFromExtra(editingProxy.extra));
        setVpnNetworkBaseline(normalizeNetworkForSelect(ex.network));

        setHy2({
          password: String(ex.password || editingProxy.password || ""),
          sni: String(ex.sni || ex.server_name || ""),
          alpn: String(ex.alpn || "h3"),
          insecure: Boolean(ex.insecure),
          upMbps: ex.up_mbps != null ? String(ex.up_mbps) : (ex.upMbps != null ? String(ex.upMbps) : ""),
          downMbps: ex.down_mbps != null ? String(ex.down_mbps) : (ex.downMbps != null ? String(ex.downMbps) : ""),
          obfsType: String(ex.obfs_type || ex.obfsType || ""),
          obfsPassword: String(ex.obfs_password || ex.obfsPassword || ""),
        });

        if (String(editingProxy.type || "").toUpperCase() === "NAIVEPROXY") {
          setNaive({
            sni: String(ex.sni || ex.server_name || ""),
            insecure: Boolean(ex.insecure),
            listen: String(ex.naive_listen || ""),
            log: String(ex.naive_log || ""),
          });
        } else {
          setNaive({ sni: "", insecure: false, listen: "", log: "" });
        }

        const wgAddr = Array.isArray(ex.address) ? ex.address.join(",") : String(ex.address || "");
        const wgAllowed = Array.isArray(ex.allowed_ips) ? ex.allowed_ips.join(",") : String(ex.allowed_ips || "");
        const amRaw = ex.amnezia;
        const amObj = amRaw && typeof amRaw === "object" && !Array.isArray(amRaw) ? amRaw : null;
        setWg({
          address: wgAddr || "10.0.0.2/32",
          privateKey: String(ex.private_key || ex.privateKey || ""),
          publicKey: String(ex.public_key || ex.publicKey || ""),
          preSharedKey: String(ex.pre_shared_key || ex.preSharedKey || ""),
          allowedIps: wgAllowed || "0.0.0.0/0",
          reserved: Array.isArray(ex.reserved) ? ex.reserved.join(",") : String(ex.reserved || ""),
          keepalive: ex.persistent_keepalive_interval != null ? String(ex.persistent_keepalive_interval) : (ex.persistentKeepaliveInterval != null ? String(ex.persistentKeepaliveInterval) : ""),
          system: Boolean(ex.system),
          name: String(ex.name || ""),
          mtu: ex.mtu != null ? String(ex.mtu) : "",
          amnezia: amneziaFieldsFromObject(amObj),
          amneziaJSON: amObj ? JSON.stringify(amObj, null, 2) : "",
          amneziaUseRaw: false,
        });
      }
    } else {
      setFormData({
        name: "",
        ip: "",
        port: "",
        type: "HTTP",
        username: "",
        password: "",
        country: "\u{1F310}",
      });
      setVpnNetwork("tcp");
      setVpnSecurity("none");
      setVpnUuid("");
      setVpnSsMethod("aes-256-gcm");
      setVpnTf({
        transPath: "/",
        transHost: "",
        grpcService: "",
        httpHost: "",
        httpPath: "/",
        xhttpMode: "auto",
      });
      setVpnNetworkBaseline("tcp");

      setHy2({
        password: "",
        sni: "",
        alpn: "h3",
        insecure: false,
        upMbps: "",
        downMbps: "",
        obfsType: "",
        obfsPassword: "",
      });

      setNaive({ sni: "", insecure: false, listen: "", log: "" });

      setWg({
        address: "10.0.0.2/32",
        privateKey: "",
        publicKey: "",
        preSharedKey: "",
        allowedIps: "0.0.0.0/0",
        reserved: "",
        keepalive: "",
        system: false,
        name: "",
        mtu: "",
        amnezia: { ...EMPTY_AMNEZIA_FIELDS },
        amneziaJSON: "",
        amneziaUseRaw: false,
      });
    }
  }, [editingProxy]);

  const handleSubmit = (e) => {
    e.preventDefault();
    if (!formData.ip || !formData.port) return;

    const tUpper = String(formData.type || "").toUpperCase();
    const isManualVpn = ["WIREGUARD", "AMNEZIAWG", "HYSTERIA2", "NAIVEPROXY"].includes(tUpper);

    if (isVpnEditMode) {
      const vpnType = String(formData.type || "").toUpperCase();
      let ex;
      if (vpnType === "HYSTERIA2") {
        const base = parseProxyExtra(formData.extra);
        ex = {
          ...base,
          password: (hy2.password || "").trim(),
          sni: (hy2.sni || "").trim(),
          alpn: (hy2.alpn || "").trim(),
          insecure: Boolean(hy2.insecure),
          up_mbps: hy2.upMbps ? parseInt(hy2.upMbps, 10) || 0 : undefined,
          down_mbps: hy2.downMbps ? parseInt(hy2.downMbps, 10) || 0 : undefined,
          obfs_type: (hy2.obfsType || "").trim(),
          obfs_password: (hy2.obfsPassword || "").trim(),
        };
        if (!ex.up_mbps) delete ex.up_mbps;
        if (!ex.down_mbps) delete ex.down_mbps;
        if (!ex.obfs_type) {
          delete ex.obfs_type;
          delete ex.obfs_password;
        }
      } else if (vpnType === "NAIVEPROXY") {
        const base = parseProxyExtra(formData.extra);
        ex = {
          ...base,
          sni: (naive.sni || "").trim(),
          insecure: Boolean(naive.insecure),
        };
        if ((naive.listen || "").trim()) ex.naive_listen = naive.listen.trim();
        if ((naive.log || "").trim()) ex.naive_log = naive.log.trim();
        delete ex.fp;
      } else if (vpnType === "WIREGUARD" || vpnType === "AMNEZIAWG") {
        const base = parseProxyExtra(formData.extra);
        const addr = (wg.address || "")
          .split(/[,;\n\r\t]+/g)
          .map((s) => s.trim())
          .filter(Boolean);
        const allowed = (wg.allowedIps || "")
          .split(/[,;\n\r\t]+/g)
          .map((s) => s.trim())
          .filter(Boolean);
        const reserved = (wg.reserved || "")
          .split(/[,;\n\r\t]+/g)
          .map((s) => parseInt(s.trim(), 10))
          .filter((n) => Number.isFinite(n));
        ex = {
          ...base,
          address: addr,
          private_key: (wg.privateKey || "").trim(),
          public_key: (wg.publicKey || "").trim(),
          pre_shared_key: (wg.preSharedKey || "").trim(),
          allowed_ips: allowed,
          reserved: reserved.length ? reserved : undefined,
          persistent_keepalive_interval: wg.keepalive ? parseInt(wg.keepalive, 10) || 0 : undefined,
          system: Boolean(wg.system),
          name: (wg.name || "").trim(),
          mtu: wg.mtu ? parseInt(wg.mtu, 10) || 0 : undefined,
        };
        if (!ex.pre_shared_key) delete ex.pre_shared_key;
        if (!ex.reserved) delete ex.reserved;
        if (!ex.persistent_keepalive_interval) delete ex.persistent_keepalive_interval;
        if (!ex.name) delete ex.name;
        if (!ex.mtu) delete ex.mtu;
        if (vpnType === "AMNEZIAWG") {
          let amObj;
          if (wg.amneziaUseRaw) {
            const raw = (wg.amneziaJSON || "").trim();
            if (raw) {
              try {
                const parsed = JSON.parse(raw);
                if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
                  amObj = parsed;
                }
              } catch {}
            }
          } else {
            amObj = amneziaObjectFromFields(wg.amnezia);
          }
          if (amObj && Object.keys(amObj).length > 0) ex.amnezia = amObj;
          else delete ex.amnezia;
        } else {
          delete ex.amnezia;
        }
      } else {
        const base = sanitizeVpnExtraForEdit(formData.extra, {
          type: vpnType,
          network: vpnNetwork,
          security: vpnSecurity,
          uuid: vpnUuid.trim(),
          ssMethod: vpnSsMethod,
        });
        ex = applyVpnTransportFieldsToExtra(base, vpnNetwork, vpnTf);
      }
      saveProxyWrapper({
        ...formData,
        name: formData.name || t("add.newServer"),
        extra: ex,
      });
      return;
    }

    if (isManualVpn) {
      const extra = {};
      if (tUpper === "HYSTERIA2") {
        extra.password = (hy2.password || "").trim();
        extra.sni = (hy2.sni || "").trim();
        extra.alpn = (hy2.alpn || "").trim();
        extra.insecure = Boolean(hy2.insecure);
        if (hy2.upMbps) extra.up_mbps = parseInt(hy2.upMbps, 10) || 0;
        if (hy2.downMbps) extra.down_mbps = parseInt(hy2.downMbps, 10) || 0;
        if ((hy2.obfsType || "").trim()) {
          extra.obfs_type = (hy2.obfsType || "").trim();
          extra.obfs_password = (hy2.obfsPassword || "").trim();
        }
      } else if (tUpper === "NAIVEPROXY") {
        extra.sni = (naive.sni || "").trim();
        extra.insecure = Boolean(naive.insecure);
        if ((naive.listen || "").trim()) extra.naive_listen = naive.listen.trim();
        if ((naive.log || "").trim()) extra.naive_log = naive.log.trim();
      } else {
        extra.address = (wg.address || "")
          .split(/[,;\n\r\t]+/g)
          .map((s) => s.trim())
          .filter(Boolean);
        extra.private_key = (wg.privateKey || "").trim();
        extra.public_key = (wg.publicKey || "").trim();
        if ((wg.preSharedKey || "").trim()) extra.pre_shared_key = (wg.preSharedKey || "").trim();
        extra.allowed_ips = (wg.allowedIps || "")
          .split(/[,;\n\r\t]+/g)
          .map((s) => s.trim())
          .filter(Boolean);
        const reserved = (wg.reserved || "")
          .split(/[,;\n\r\t]+/g)
          .map((s) => parseInt(s.trim(), 10))
          .filter((n) => Number.isFinite(n));
        if (reserved.length) extra.reserved = reserved;
        if (wg.keepalive) extra.persistent_keepalive_interval = parseInt(wg.keepalive, 10) || 0;
        extra.system = Boolean(wg.system);
        if ((wg.name || "").trim()) extra.name = (wg.name || "").trim();
        if (wg.mtu) extra.mtu = parseInt(wg.mtu, 10) || 0;
        if (tUpper === "AMNEZIAWG") {
          let amObj;
          if (wg.amneziaUseRaw) {
            const raw = (wg.amneziaJSON || "").trim();
            if (raw) {
              try {
                const parsed = JSON.parse(raw);
                if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
                  amObj = parsed;
                }
              } catch {}
            }
          } else {
            amObj = amneziaObjectFromFields(wg.amnezia);
          }
          if (amObj && Object.keys(amObj).length > 0) extra.amnezia = amObj;
        }
      }
      saveProxyWrapper({
        ...formData,
        name: formData.name || t("add.newServer"),
        extra,
      });
      return;
    }

    saveProxyWrapper({ ...formData, name: formData.name || t("add.newServer") });
  };

  const vpnEditType = String(formData.type || "").toUpperCase();
  const showVpnTransport = ["VLESS", "VMESS", "TROJAN"].includes(vpnEditType);
  const showVpnSecurity = ["VLESS", "VMESS"].includes(vpnEditType);
  const showManualVpn = ["WIREGUARD", "AMNEZIAWG", "NAIVEPROXY"].includes(vpnEditType);
  const vpnNetworkChangedFromImport =
    isVpnEditMode &&
    showVpnTransport &&
    vpnNetwork !== vpnNetworkBaseline;

  const subscriptionPlaceholderByType = {
    VLESS: t("add.subscriptionPlaceholderVless") || "https://example.com/sub или vless://...",
    VMESS: t("add.subscriptionPlaceholderVmess") || "https://example.com/sub или vmess://...",
    TROJAN: t("add.subscriptionPlaceholderTrojan") || "https://example.com/sub или trojan://...",
    SS: t("add.subscriptionPlaceholderSs") || "https://example.com/sub или ss://...",
    HYSTERIA2: t("add.subscriptionPlaceholderHy2") || "https://example.com/sub или hy2://...",
    NAIVEPROXY:
      t("add.subscriptionPlaceholderNaive") ||
      "naive+https://... или JSON {\"listen\":\"...\",\"proxy\":\"https://...\"}",
    WIREGUARD: t("add.subscriptionPlaceholderWireguard") || "https://example.com/sub или wireguard://...",
    AMNEZIAWG: t("add.subscriptionPlaceholderAmneziawg") || "https://example.com/sub или amneziawg://...",
  };
  const subscriptionPlaceholder =
    subscriptionPlaceholderByType[vpnEditType] ||
    t("add.subscriptionPlaceholderVless") ||
    "https://example.com/sub или vless://...";

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
          <label className="cursor-pointer bg-zinc-900/50 hover:bg-zinc-800/80 text-white font-bold py-5 !rounded-3xl transition-all border border-zinc-800 flex flex-col items-center justify-center gap-2 group hover:border-[#007E3A]/50 hover:shadow-lg hover:shadow-[#007E3A]/5">
            <FileUp className="w-6 h-6 text-[#007E3A] group-hover:scale-110 transition-transform" />
            <span className="text-sm">{t("add.fromFile")}</span>
            <input
              type="file"
              accept=".txt,.csv,.conf,.json"
              className="hidden"
              onChange={handleFileImport}
            />
          </label>
          <button
            onClick={handleClipboardImport}
            className="bg-zinc-900/50 hover:bg-zinc-800/80 text-white font-bold py-5 !rounded-3xl transition-all border border-zinc-800 flex flex-col items-center justify-center gap-2 group hover:border-[#007E3A]/50 hover:shadow-lg hover:shadow-[#007E3A]/5"
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
          {!isVpnEditMode && (
            <>
              <details className="bg-zinc-950 border border-zinc-800 rounded-2xl overflow-hidden" open>
                <summary className="cursor-pointer select-none px-4 py-3 text-sm font-bold text-white">
                  {t("add.proxyProtocolsTitle") || "Прокси протоколы"}
                </summary>
                <div className="px-4 pb-4 pt-1">
                  <div className="grid grid-cols-3 gap-3">
                    {PLAIN_TYPES.map((type) => (
                      <button
                        type="button"
                        key={type}
                        onClick={() => setFormData({ ...formData, type })}
                        className={`h-[46px] rounded-xl text-sm font-bold border transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${
                          formData.type === type
                            ? "bg-[#007E3A] text-white border-[#007E3A]"
                            : "bg-zinc-900 text-zinc-300 border-zinc-800 hover:border-[#00A819]"
                        }`}
                      >
                        {type}
                      </button>
                    ))}
                  </div>
                </div>
              </details>

              {!(editingProxy && !isVpnMode) && (
                <details className="bg-zinc-950 border border-zinc-800 rounded-2xl overflow-hidden" open>
                  <summary className="cursor-pointer select-none px-4 py-3 text-sm font-bold text-white">
                    {t("add.vpnProtocolsTitle") || "ВПН протоколы"}
                  </summary>
                  <div className="px-4 pb-4 pt-1">
                    <div className="grid grid-cols-4 gap-3">
                      {VPN_TYPES_LIST.map((type) => (
                        <button
                          type="button"
                          key={type}
                          onClick={() => setFormData({ ...formData, type })}
                          className={`h-[46px] rounded-xl text-[11px] font-bold border transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${
                            formData.type === type
                              ? "bg-[#007E3A] text-white border-[#007E3A]"
                              : "bg-zinc-900 text-zinc-300 border-zinc-800 hover:border-[#00A819]"
                          }`}
                        >
                          {type}
                        </button>
                      ))}
                    </div>
                  </div>
                </details>
              )}
            </>
          )}

          {isVpnEditMode ? (
            <>
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

              <div className="grid grid-cols-3 gap-6">
                <div className="col-span-2">
                  <label className="block text-sm font-medium text-zinc-400 mb-2">
                    {t("add.ip")}
                  </label>
                  <input
                    type="text"
                    required
                    placeholder="example.com"
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
                    placeholder="443"
                    className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:outline-none focus:ring-0 focus-visible:outline-none focus:border-[#007E3A] transition-colors"
                    value={formData.port || ""}
                    onChange={(e) =>
                      setFormData({ ...formData, port: e.target.value })
                    }
                  />
                </div>
              </div>

              <div>
                <p className="text-xs font-medium text-zinc-500 uppercase tracking-wider mb-2">
                  {t("add.vpnProtocolType")}
                </p>
                <div className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-bold">
                  {vpnEditType}
                </div>
              </div>

              {(vpnEditType === "VLESS" || vpnEditType === "VMESS") && (
                <div>
                  <label className="block text-sm font-medium text-zinc-400 mb-2">
                    {t("add.vpnUuid")}
                  </label>
                  <input
                    type="text"
                    required
                    className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                    value={vpnUuid}
                    onChange={(e) => setVpnUuid(e.target.value)}
                    autoComplete="off"
                  />
                </div>
              )}

              {(vpnEditType === "TROJAN" || vpnEditType === "SS") && (
                <div>
                  <label className="block text-sm font-medium text-zinc-400 mb-2">
                    {t("add.passPlaceholder")}
                  </label>
                  <input
                    type="password"
                    className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:border-[#007E3A] transition-colors"
                    value={formData.password || ""}
                    onChange={(e) =>
                      setFormData({ ...formData, password: e.target.value })
                    }
                  />
                </div>
              )}

              {vpnEditType === "HYSTERIA2" && (
                <div className="space-y-3">
                  <div>
                    <label className="block text-sm font-medium text-zinc-400 mb-2">
                      {t("add.hy2Password") || "Пароль"}
                    </label>
                    <input
                      type="password"
                      className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:border-[#007E3A] transition-colors"
                      value={hy2.password}
                      onChange={(e) => setHy2({ ...hy2, password: e.target.value })}
                    />
                  </div>
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="block text-sm font-medium text-zinc-400 mb-2">
                        {t("add.hy2Sni") || "SNI"}
                      </label>
                      <input
                        type="text"
                        className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                        value={hy2.sni}
                        onChange={(e) => setHy2({ ...hy2, sni: e.target.value })}
                        placeholder="example.com"
                      />
                    </div>
                    <div>
                      <label className="block text-sm font-medium text-zinc-400 mb-2">
                        {t("add.hy2Alpn") || "ALPN"}
                      </label>
                      <input
                        type="text"
                        className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                        value={hy2.alpn}
                        onChange={(e) => setHy2({ ...hy2, alpn: e.target.value })}
                        placeholder="h3"
                      />
                    </div>
                  </div>
                  <label className="flex items-center gap-3 text-sm text-zinc-200">
                    <input
                      type="checkbox"
                      className="accent-[#007E3A]"
                      checked={hy2.insecure}
                      onChange={(e) => setHy2({ ...hy2, insecure: e.target.checked })}
                    />
                    {t("add.hy2Insecure") || "Insecure (skip verify)"}
                  </label>
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="block text-sm font-medium text-zinc-400 mb-2">
                        {t("add.hy2Up") || "Up Mbps"}
                      </label>
                      <input
                        type="number"
                        className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:border-[#007E3A] transition-colors"
                        value={hy2.upMbps}
                        onChange={(e) => setHy2({ ...hy2, upMbps: e.target.value })}
                      />
                    </div>
                    <div>
                      <label className="block text-sm font-medium text-zinc-400 mb-2">
                        {t("add.hy2Down") || "Down Mbps"}
                      </label>
                      <input
                        type="number"
                        className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:border-[#007E3A] transition-colors"
                        value={hy2.downMbps}
                        onChange={(e) => setHy2({ ...hy2, downMbps: e.target.value })}
                      />
                    </div>
                  </div>
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="block text-sm font-medium text-zinc-400 mb-2">
                        {t("add.hy2ObfsType") || "Obfs type"}
                      </label>
                      <input
                        type="text"
                        className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                        value={hy2.obfsType}
                        onChange={(e) => setHy2({ ...hy2, obfsType: e.target.value })}
                        placeholder="salamander"
                      />
                    </div>
                    <div>
                      <label className="block text-sm font-medium text-zinc-400 mb-2">
                        {t("add.hy2ObfsPassword") || "Obfs password"}
                      </label>
                      <input
                        type="password"
                        className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:border-[#007E3A] transition-colors"
                        value={hy2.obfsPassword}
                        onChange={(e) => setHy2({ ...hy2, obfsPassword: e.target.value })}
                      />
                    </div>
                  </div>
                </div>
              )}

              {vpnEditType === "NAIVEPROXY" && (
                <div className="space-y-3">
                  <p className="text-xs text-zinc-500">{t("add.naiveTlsHint")}</p>
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="block text-sm font-medium text-zinc-400 mb-2">
                        {t("add.naiveUser") || "Username"}
                      </label>
                      <input
                        type="text"
                        className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                        value={formData.username || ""}
                        onChange={(e) => setFormData({ ...formData, username: e.target.value })}
                        autoComplete="off"
                      />
                    </div>
                    <div>
                      <label className="block text-sm font-medium text-zinc-400 mb-2">
                        {t("add.naivePass") || "Password"}
                      </label>
                      <input
                        type="password"
                        className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:border-[#007E3A] transition-colors"
                        value={formData.password || ""}
                        onChange={(e) => setFormData({ ...formData, password: e.target.value })}
                      />
                    </div>
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-zinc-400 mb-2">
                      {t("add.naiveSni") || "SNI (TLS server name)"}
                    </label>
                    <input
                      type="text"
                      className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                      value={naive.sni}
                      onChange={(e) => setNaive({ ...naive, sni: e.target.value })}
                      placeholder="example.com"
                    />
                  </div>
                  <label className="flex items-center gap-3 text-sm text-zinc-200">
                    <input
                      type="checkbox"
                      className="accent-[#007E3A]"
                      checked={naive.insecure}
                      onChange={(e) => setNaive({ ...naive, insecure: e.target.checked })}
                    />
                    {t("add.naiveInsecure") || "Insecure (not supported for naive in sing-box)"}
                  </label>
                  <div>
                    <label className="block text-sm font-medium text-zinc-400 mb-2">
                      {t("add.naiveListen") || "Listen (reference, optional)"}
                    </label>
                    <input
                      type="text"
                      className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-xs outline-none focus:border-[#007E3A] transition-colors"
                      value={naive.listen}
                      onChange={(e) => setNaive({ ...naive, listen: e.target.value })}
                      placeholder="socks://127.0.0.1:1080"
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-zinc-400 mb-2">
                      {t("add.naiveLog") || "Log (reference, optional)"}
                    </label>
                    <input
                      type="text"
                      className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-xs outline-none focus:border-[#007E3A] transition-colors"
                      value={naive.log}
                      onChange={(e) => setNaive({ ...naive, log: e.target.value })}
                    />
                  </div>
                </div>
              )}

              {(vpnEditType === "WIREGUARD" || vpnEditType === "AMNEZIAWG") && (
                <div className="space-y-3">
                  <div>
                    <label className="block text-sm font-medium text-zinc-400 mb-2">
                      {t("add.wgAddress") || "Адрес (CIDR, можно несколько через запятую)"}
                    </label>
                    <input
                      type="text"
                      className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                      value={wg.address}
                      onChange={(e) => setWg({ ...wg, address: e.target.value })}
                      placeholder="10.0.0.2/32"
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-zinc-400 mb-2">
                      {t("add.wgPrivateKey") || "Private key"}
                    </label>
                    <input
                      type="text"
                      className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                      value={wg.privateKey}
                      onChange={(e) => setWg({ ...wg, privateKey: e.target.value })}
                      autoComplete="off"
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-zinc-400 mb-2">
                      {t("add.wgPeerPublicKey") || "Peer public key"}
                    </label>
                    <input
                      type="text"
                      className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                      value={wg.publicKey}
                      onChange={(e) => setWg({ ...wg, publicKey: e.target.value })}
                      autoComplete="off"
                    />
                  </div>
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="block text-sm font-medium text-zinc-400 mb-2">
                        {t("add.wgPsk") || "Pre-shared key (опционально)"}
                      </label>
                      <input
                        type="text"
                        className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                        value={wg.preSharedKey}
                        onChange={(e) => setWg({ ...wg, preSharedKey: e.target.value })}
                        autoComplete="off"
                      />
                    </div>
                    <div>
                      <label className="block text-sm font-medium text-zinc-400 mb-2">
                        {t("add.wgKeepalive") || "Keepalive (сек)"}
                      </label>
                      <input
                        type="number"
                        className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:border-[#007E3A] transition-colors"
                        value={wg.keepalive}
                        onChange={(e) => setWg({ ...wg, keepalive: e.target.value })}
                      />
                    </div>
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-zinc-400 mb-2">
                      {t("add.wgAllowedIps") || "Allowed IPs"}
                    </label>
                    <input
                      type="text"
                      className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                      value={wg.allowedIps}
                      onChange={(e) => setWg({ ...wg, allowedIps: e.target.value })}
                      placeholder="0.0.0.0/0"
                    />
                  </div>
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="block text-sm font-medium text-zinc-400 mb-2">
                        {t("add.wgReserved") || "Reserved (байты через запятую)"}
                      </label>
                      <input
                        type="text"
                        className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                        value={wg.reserved}
                        onChange={(e) => setWg({ ...wg, reserved: e.target.value })}
                        placeholder="0,0,0"
                      />
                    </div>
                    <div className="space-y-3">
                      <div>
                        <label className="block text-sm font-medium text-zinc-400 mb-2">
                          {t("add.wgMtu") || "MTU"}
                        </label>
                        <input
                          type="number"
                          className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:border-[#007E3A] transition-colors"
                          value={wg.mtu}
                          onChange={(e) => setWg({ ...wg, mtu: e.target.value })}
                          placeholder="1408"
                        />
                      </div>
                    </div>
                  </div>
                  <label className="flex items-center gap-3 text-sm text-zinc-200">
                    <input
                      type="checkbox"
                      className="accent-[#007E3A]"
                      checked={wg.system}
                      onChange={(e) => setWg({ ...wg, system: e.target.checked })}
                    />
                    {t("add.wgSystem") || "System interface"}
                  </label>
                  <div>
                    <label className="block text-sm font-medium text-zinc-400 mb-2">
                      {t("add.wgName") || "Interface name"}
                    </label>
                    <input
                      type="text"
                      className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                      value={wg.name}
                      onChange={(e) => setWg({ ...wg, name: e.target.value })}
                      placeholder="wg0"
                    />
                  </div>
                  {vpnEditType === "AMNEZIAWG" && (
                    <AmneziaEditor
                      fields={wg.amnezia}
                      onFieldsChange={handleAmneziaFieldsChange}
                      useRaw={wg.amneziaUseRaw}
                      onUseRawChange={handleAmneziaUseRawChange}
                      rawValue={wg.amneziaJSON}
                      onRawChange={handleAmneziaRawChange}
                      t={t}
                    />
                  )}
                </div>
              )}

              {vpnEditType === "SS" && (
                <div>
                  <label className="block text-sm font-medium text-zinc-400 mb-2">
                    {t("add.ssMethod")}
                  </label>
                  <input
                    type="text"
                    className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                    value={vpnSsMethod}
                    onChange={(e) => setVpnSsMethod(e.target.value)}
                    placeholder="aes-256-gcm"
                  />
                </div>
              )}

              {showVpnTransport && (
                <div className="space-y-2">
                  <p className="text-xs font-medium text-zinc-500 uppercase tracking-wider">
                    {t("add.vpnTransport")}
                  </p>
                  <label className="flex items-center justify-between gap-4 w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 cursor-pointer">
                    <span className="text-sm text-white shrink-0">
                      {t("add.vpnNetwork")}
                    </span>
                    <AppSelect
                      value={vpnNetwork}
                      options={VPN_NETWORK_OPTIONS}
                      onChange={setVpnNetwork}
                      ariaLabel={t("add.vpnNetwork")}
                      className="max-w-[55%] min-w-[120px]"
                      buttonClassName="bg-transparent border-0 p-0 hover:border-0 focus:ring-0 justify-end text-right"
                      listClassName="w-full"
                    />
                  </label>
                  <p className="text-xs text-zinc-500 mt-1">{t("add.vpnTransportParamsHint")}</p>
                  <p className="text-xs text-zinc-500 mt-2">{t("add.vpnTcpLinkHint")}</p>
                  {vpnNetworkChangedFromImport && (
                    <div className="mt-2 rounded-xl border border-amber-500/35 bg-amber-500/10 px-3 py-2 text-xs text-amber-100/95 leading-snug">
                      {t("add.vpnNetworkChangedWarning")}
                    </div>
                  )}
                  {vpnNetwork === "ws" && (
                    <div className="space-y-3 pt-2">
                      <div>
                        <label className="block text-sm font-medium text-zinc-400 mb-2">
                          {t("add.vpnTransportPath")}
                        </label>
                        <input
                          type="text"
                          className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                          value={vpnTf.transPath}
                          onChange={(e) =>
                            setVpnTf({ ...vpnTf, transPath: e.target.value })
                          }
                          placeholder="/"
                        />
                      </div>
                      <div>
                        <label className="block text-sm font-medium text-zinc-400 mb-2">
                          {t("add.vpnTransportHost")}
                        </label>
                        <input
                          type="text"
                          className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                          value={vpnTf.transHost}
                          onChange={(e) =>
                            setVpnTf({ ...vpnTf, transHost: e.target.value })
                          }
                          placeholder="cdn.example.com"
                        />
                      </div>
                    </div>
                  )}
                  {vpnNetwork === "grpc" && (
                    <div className="pt-2">
                      <label className="block text-sm font-medium text-zinc-400 mb-2">
                        {t("add.vpnGrpcService")}
                      </label>
                      <input
                        type="text"
                        className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                        value={vpnTf.grpcService}
                        onChange={(e) =>
                          setVpnTf({ ...vpnTf, grpcService: e.target.value })
                        }
                      />
                    </div>
                  )}
                  {(vpnNetwork === "http" || vpnNetwork === "h2") && (
                    <div className="space-y-3 pt-2">
                      <div>
                        <label className="block text-sm font-medium text-zinc-400 mb-2">
                          {t("add.vpnHttpHost")}
                        </label>
                        <input
                          type="text"
                          className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                          value={vpnTf.httpHost}
                          onChange={(e) =>
                            setVpnTf({ ...vpnTf, httpHost: e.target.value })
                          }
                        />
                      </div>
                      <div>
                        <label className="block text-sm font-medium text-zinc-400 mb-2">
                          {t("add.vpnHttpPath")}
                        </label>
                        <input
                          type="text"
                          className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                          value={vpnTf.httpPath}
                          onChange={(e) =>
                            setVpnTf({ ...vpnTf, httpPath: e.target.value })
                          }
                          placeholder="/"
                        />
                      </div>
                    </div>
                  )}
                  {vpnNetwork === "xhttp" && (
                    <div className="space-y-3 pt-2">
                      <div>
                        <label className="block text-sm font-medium text-zinc-400 mb-2">
                          {t("add.vpnTransportPath")}
                        </label>
                        <input
                          type="text"
                          className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                          value={vpnTf.transPath}
                          onChange={(e) =>
                            setVpnTf({ ...vpnTf, transPath: e.target.value })
                          }
                          placeholder="/"
                        />
                      </div>
                      <div>
                        <label className="block text-sm font-medium text-zinc-400 mb-2">
                          {t("add.vpnTransportHost")}
                        </label>
                        <input
                          type="text"
                          className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                          value={vpnTf.transHost}
                          onChange={(e) =>
                            setVpnTf({ ...vpnTf, transHost: e.target.value })
                          }
                        />
                      </div>
                      <div>
                        <label className="block text-sm font-medium text-zinc-400 mb-2">
                          {t("add.vpnXhttpMode")}
                        </label>
                        <input
                          type="text"
                          className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                          value={vpnTf.xhttpMode}
                          onChange={(e) =>
                            setVpnTf({ ...vpnTf, xhttpMode: e.target.value })
                          }
                          placeholder="auto"
                        />
                      </div>
                    </div>
                  )}
                </div>
              )}

              {showVpnSecurity && (
                <div className="space-y-2">
                  <p className="text-xs font-medium text-zinc-500 uppercase tracking-wider">
                    {t("add.vpnSecurity")}
                  </p>
                  <label className="flex items-center justify-between gap-4 w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 cursor-pointer">
                    <span className="text-sm text-white shrink-0">
                      {t("add.vpnEncryption")}
                    </span>
                    <AppSelect
                      value={vpnSecurity}
                      options={VPN_SECURITY_OPTIONS}
                      onChange={setVpnSecurity}
                      ariaLabel={t("add.vpnEncryption")}
                      className="max-w-[55%] min-w-[120px]"
                      buttonClassName="bg-transparent border-0 p-0 hover:border-0 focus:ring-0 justify-end text-right"
                      listClassName="w-full"
                    />
                  </label>
                </div>
              )}

              {vpnEditType === "TROJAN" && (
                <p className="text-xs text-zinc-500">{t("add.trojanTlsHint")}</p>
              )}

              <div className="pt-5 flex space-x-4">
                <button
                  type="button"
                  onClick={onCancel}
                  className="flex-1 bg-zinc-800 hover:bg-zinc-700 text-white font-bold py-3 rounded-xl transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
                >
                  {t("add.cancel")}
                </button>
                <button
                  type="submit"
                  className="flex-[2] bg-[#007E3A] hover:bg-[#005C2A] text-white font-bold py-3 rounded-xl transition-colors shadow-lg shadow-[#007E3A]/20 border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
                >
                  {t("add.saveChanges")}
                </button>
              </div>
            </>
          ) : (
            <>
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

              {isVpnMode ? (
                <div className="space-y-4">
                  {showManualVpn ? (
                    <>
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

                      <div className="grid grid-cols-3 gap-6">
                        <div className="col-span-2">
                          <label className="block text-sm font-medium text-zinc-400 mb-2">
                            {t("add.ip")}
                          </label>
                          <input
                            type="text"
                            required
                            placeholder="example.com"
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
                            placeholder="443"
                            className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:outline-none focus:ring-0 focus-visible:outline-none focus:border-[#007E3A] transition-colors"
                            value={formData.port || ""}
                            onChange={(e) =>
                              setFormData({ ...formData, port: e.target.value })
                            }
                          />
                        </div>
                      </div>

                      {vpnEditType === "HYSTERIA2" && (
                        <div className="space-y-3">
                          <div>
                            <label className="block text-sm font-medium text-zinc-400 mb-2">
                              {t("add.hy2Password") || "Пароль"}
                            </label>
                            <input
                              type="password"
                              className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:border-[#007E3A] transition-colors"
                              value={hy2.password}
                              onChange={(e) => setHy2({ ...hy2, password: e.target.value })}
                            />
                          </div>
                          <div className="grid grid-cols-2 gap-3">
                            <div>
                              <label className="block text-sm font-medium text-zinc-400 mb-2">
                                {t("add.hy2Sni") || "SNI"}
                              </label>
                              <input
                                type="text"
                                className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                                value={hy2.sni}
                                onChange={(e) => setHy2({ ...hy2, sni: e.target.value })}
                                placeholder="example.com"
                              />
                            </div>
                            <div>
                              <label className="block text-sm font-medium text-zinc-400 mb-2">
                                {t("add.hy2Alpn") || "ALPN"}
                              </label>
                              <input
                                type="text"
                                className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                                value={hy2.alpn}
                                onChange={(e) => setHy2({ ...hy2, alpn: e.target.value })}
                                placeholder="h3"
                              />
                            </div>
                          </div>
                          <label className="flex items-center gap-3 text-sm text-zinc-200">
                            <input
                              type="checkbox"
                              className="accent-[#007E3A]"
                              checked={hy2.insecure}
                              onChange={(e) => setHy2({ ...hy2, insecure: e.target.checked })}
                            />
                            {t("add.hy2Insecure") || "Insecure (skip verify)"}
                          </label>
                          <div className="grid grid-cols-2 gap-3">
                            <div>
                              <label className="block text-sm font-medium text-zinc-400 mb-2">
                                {t("add.hy2Up") || "Up Mbps"}
                              </label>
                              <input
                                type="number"
                                className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:border-[#007E3A] transition-colors"
                                value={hy2.upMbps}
                                onChange={(e) => setHy2({ ...hy2, upMbps: e.target.value })}
                              />
                            </div>
                            <div>
                              <label className="block text-sm font-medium text-zinc-400 mb-2">
                                {t("add.hy2Down") || "Down Mbps"}
                              </label>
                              <input
                                type="number"
                                className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:border-[#007E3A] transition-colors"
                                value={hy2.downMbps}
                                onChange={(e) => setHy2({ ...hy2, downMbps: e.target.value })}
                              />
                            </div>
                          </div>
                          <div className="grid grid-cols-2 gap-3">
                            <div>
                              <label className="block text-sm font-medium text-zinc-400 mb-2">
                                {t("add.hy2ObfsType") || "Obfs type"}
                              </label>
                              <input
                                type="text"
                                className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                                value={hy2.obfsType}
                                onChange={(e) => setHy2({ ...hy2, obfsType: e.target.value })}
                                placeholder="salamander"
                              />
                            </div>
                            <div>
                              <label className="block text-sm font-medium text-zinc-400 mb-2">
                                {t("add.hy2ObfsPassword") || "Obfs password"}
                              </label>
                              <input
                                type="password"
                                className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:border-[#007E3A] transition-colors"
                                value={hy2.obfsPassword}
                                onChange={(e) => setHy2({ ...hy2, obfsPassword: e.target.value })}
                              />
                            </div>
                          </div>
                        </div>
                      )}

                      {vpnEditType === "NAIVEPROXY" && (
                        <div className="space-y-3">
                          <p className="text-xs text-zinc-500">{t("add.naiveTlsHint")}</p>
                          <div className="grid grid-cols-2 gap-3">
                            <div>
                              <label className="block text-sm font-medium text-zinc-400 mb-2">
                                {t("add.naiveUser") || "Username"}
                              </label>
                              <input
                                type="text"
                                className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                                value={formData.username || ""}
                                onChange={(e) => setFormData({ ...formData, username: e.target.value })}
                                autoComplete="off"
                              />
                            </div>
                            <div>
                              <label className="block text-sm font-medium text-zinc-400 mb-2">
                                {t("add.naivePass") || "Password"}
                              </label>
                              <input
                                type="password"
                                className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:border-[#007E3A] transition-colors"
                                value={formData.password || ""}
                                onChange={(e) => setFormData({ ...formData, password: e.target.value })}
                              />
                            </div>
                          </div>
                          <div>
                            <label className="block text-sm font-medium text-zinc-400 mb-2">
                              {t("add.naiveSni") || "SNI (TLS server name)"}
                            </label>
                            <input
                              type="text"
                              className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                              value={naive.sni}
                              onChange={(e) => setNaive({ ...naive, sni: e.target.value })}
                              placeholder="example.com"
                            />
                          </div>
                          <label className="flex items-center gap-3 text-sm text-zinc-200">
                            <input
                              type="checkbox"
                              className="accent-[#007E3A]"
                              checked={naive.insecure}
                              onChange={(e) => setNaive({ ...naive, insecure: e.target.checked })}
                            />
                            {t("add.naiveInsecure") || "Insecure (not supported for naive in sing-box)"}
                          </label>
                          <div>
                            <label className="block text-sm font-medium text-zinc-400 mb-2">
                              {t("add.naiveListen") || "Listen (reference, optional)"}
                            </label>
                            <input
                              type="text"
                              className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-xs outline-none focus:border-[#007E3A] transition-colors"
                              value={naive.listen}
                              onChange={(e) => setNaive({ ...naive, listen: e.target.value })}
                              placeholder="socks://127.0.0.1:1080"
                            />
                          </div>
                          <div>
                            <label className="block text-sm font-medium text-zinc-400 mb-2">
                              {t("add.naiveLog") || "Log (reference, optional)"}
                            </label>
                            <input
                              type="text"
                              className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-xs outline-none focus:border-[#007E3A] transition-colors"
                              value={naive.log}
                              onChange={(e) => setNaive({ ...naive, log: e.target.value })}
                            />
                          </div>
                        </div>
                      )}

                      {(vpnEditType === "WIREGUARD" || vpnEditType === "AMNEZIAWG") && (
                        <div className="space-y-3">
                          <div>
                            <label className="block text-sm font-medium text-zinc-400 mb-2">
                              {t("add.wgAddress") || "Адрес (CIDR, можно несколько через запятую)"}
                            </label>
                            <input
                              type="text"
                              className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                              value={wg.address}
                              onChange={(e) => setWg({ ...wg, address: e.target.value })}
                              placeholder="10.0.0.2/32"
                            />
                          </div>
                          <div>
                            <label className="block text-sm font-medium text-zinc-400 mb-2">
                              {t("add.wgPrivateKey") || "Private key"}
                            </label>
                            <input
                              type="text"
                              className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                              value={wg.privateKey}
                              onChange={(e) => setWg({ ...wg, privateKey: e.target.value })}
                              autoComplete="off"
                            />
                          </div>
                          <div>
                            <label className="block text-sm font-medium text-zinc-400 mb-2">
                              {t("add.wgPeerPublicKey") || "Peer public key"}
                            </label>
                            <input
                              type="text"
                              className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                              value={wg.publicKey}
                              onChange={(e) => setWg({ ...wg, publicKey: e.target.value })}
                              autoComplete="off"
                            />
                          </div>
                          <div className="grid grid-cols-2 gap-3">
                            <div>
                              <label className="block text-sm font-medium text-zinc-400 mb-2">
                                {t("add.wgPsk") || "Pre-shared key (опционально)"}
                              </label>
                              <input
                                type="text"
                                className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                                value={wg.preSharedKey}
                                onChange={(e) => setWg({ ...wg, preSharedKey: e.target.value })}
                                autoComplete="off"
                              />
                            </div>
                            <div>
                              <label className="block text-sm font-medium text-zinc-400 mb-2">
                                {t("add.wgKeepalive") || "Keepalive (сек)"}
                              </label>
                              <input
                                type="number"
                                className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:border-[#007E3A] transition-colors"
                                value={wg.keepalive}
                                onChange={(e) => setWg({ ...wg, keepalive: e.target.value })}
                              />
                            </div>
                          </div>
                          <div>
                            <label className="block text-sm font-medium text-zinc-400 mb-2">
                              {t("add.wgAllowedIps") || "Allowed IPs"}
                            </label>
                            <input
                              type="text"
                              className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                              value={wg.allowedIps}
                              onChange={(e) => setWg({ ...wg, allowedIps: e.target.value })}
                              placeholder="0.0.0.0/0"
                            />
                          </div>
                          <div className="grid grid-cols-2 gap-3">
                            <div>
                              <label className="block text-sm font-medium text-zinc-400 mb-2">
                                {t("add.wgReserved") || "Reserved (байты через запятую)"}
                              </label>
                              <input
                                type="text"
                                className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                                value={wg.reserved}
                                onChange={(e) => setWg({ ...wg, reserved: e.target.value })}
                                placeholder="0,0,0"
                              />
                            </div>
                            <div>
                              <label className="block text-sm font-medium text-zinc-400 mb-2">
                                {t("add.wgMtu") || "MTU"}
                              </label>
                              <input
                                type="number"
                                className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:border-[#007E3A] transition-colors"
                                value={wg.mtu}
                                onChange={(e) => setWg({ ...wg, mtu: e.target.value })}
                                placeholder="1408"
                              />
                            </div>
                          </div>
                          <label className="flex items-center gap-3 text-sm text-zinc-200">
                            <input
                              type="checkbox"
                              className="accent-[#007E3A]"
                              checked={wg.system}
                              onChange={(e) => setWg({ ...wg, system: e.target.checked })}
                            />
                            {t("add.wgSystem") || "System interface"}
                          </label>
                          <div>
                            <label className="block text-sm font-medium text-zinc-400 mb-2">
                              {t("add.wgName") || "Interface name"}
                            </label>
                            <input
                              type="text"
                              className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors"
                              value={wg.name}
                              onChange={(e) => setWg({ ...wg, name: e.target.value })}
                              placeholder="wg0"
                            />
                          </div>
                          {vpnEditType === "AMNEZIAWG" && (
                            <AmneziaEditor
                              fields={wg.amnezia}
                              onFieldsChange={handleAmneziaFieldsChange}
                              useRaw={wg.amneziaUseRaw}
                              onUseRawChange={handleAmneziaUseRawChange}
                              rawValue={wg.amneziaJSON}
                              onRawChange={handleAmneziaRawChange}
                              t={t}
                            />
                          )}
                        </div>
                      )}

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
                  ) : (
                    <>
                      <div>
                        <label className="block text-sm font-medium text-zinc-400 mb-2">
                          {t("add.subscriptionUrl") || "Ссылка на подписку или URI"}
                        </label>
                        <textarea
                          className="w-full h-28 bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white font-mono text-sm outline-none focus:border-[#007E3A] transition-colors resize-none"
                          placeholder={subscriptionPlaceholder}
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
                    </>
                  )}
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

      {isImporting && importMode !== "bulk" && createPortal(
        <div className="fixed inset-0 bg-black/50 backdrop-blur-sm z-[9999] flex items-center justify-center">
          <div className="bg-zinc-900 border border-zinc-800 p-6 rounded-3xl flex flex-col items-center space-y-4 shadow-2xl">
            <div className="w-12 h-12 border-4 border-[#007E3A]/30 border-t-[#007E3A] rounded-full animate-spin" />
            <p className="text-white font-medium">
              {t("add.importing") || "Импорт прокси..."}
            </p>
          </div>
        </div>,
        document.body
      )}

      <ProtocolSelectionModal
        isOpen={showSelectionModal}
        proxies={pendingProxies}
        count={pendingProxies.length}
        routingLists={pendingRoutingLists}
        disabledListUrls={disabledListUrls}
        onToggleListDisabled={(url, disabled) => {
          setDisabledListUrls((prev) => {
            const next = new Set(prev);
            if (disabled) next.add(url);
            else next.delete(url);
            return next;
          });
        }}
        onClose={() => {
          setShowSelectionModal(false);
          setPendingProxies([]);
          setPendingRoutingLists([]);
          setDisabledListUrls(new Set());
        }}
        onConfirm={handleConfirmImport}
      />
    </div>
  );
};
