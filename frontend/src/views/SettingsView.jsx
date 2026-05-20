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

import React, { useEffect, useMemo, useState } from "react";
import {
  Activity,
  ArrowLeft,
  Check,
  ChevronRight,
  Shield,
  Download,
  Upload,
  Lock,
} from "lucide-react";
import { SettingToggle } from "../components/ui/SettingToggle";
import AppSelect from "../components/ui/AppSelect";
import { encryptWithPassword, decryptWithPassword } from "../utils/crypto";
import { useConfigContext } from "../context/ConfigContext";
import { useTranslation } from "react-i18next";
import wailsAPI from "../utils/wailsAPI";
import { rebuildSubscriptionsFromProxies } from "../utils/proxyParser";

export const SettingsView = () => {
  const { t } = useTranslation();
  const {
    proxies,
    setProxies,
    routingRules,
    setRoutingRules,
    settings,
    setSettings,
    updateSetting,
    subscriptions,
    setSubscriptions,
  } = useConfigContext();
  const [pwdModal, setPwdModal] = useState({
    isOpen: false,
    mode: "",
    data: null,
  });
  const [pwdInput, setPwdInput] = useState("");
  const [notify, setNotify] = useState(null);
  const [localPortInput, setLocalPortInput] = useState(
    settings?.localPort ? String(settings.localPort) : "",
  );
  const [dnsInput, setDnsInput] = useState(
    Array.isArray(settings?.dnsServers) ? settings.dnsServers.join(", ") : "",
  );
  const [subscriptionIntervalInput, setSubscriptionIntervalInput] = useState(
    settings?.subscriptionUpdateIntervalHours
      ? String(settings.subscriptionUpdateIntervalHours)
      : "6",
  );
  const [subscriptionUserAgentInput, setSubscriptionUserAgentInput] = useState(
    settings?.subscriptionUserAgent || "",
  );
  const [dnsPreset, setDnsPreset] = useState("custom");
  const [lanIPs, setLanIPs] = useState([]);
  const [activeSection, setActiveSection] = useState(null);
  const DNS_PRESETS = useMemo(
    () => ({
      auto: [],
      google: ["8.8.8.8", "8.8.4.4"],
      cloudflare: ["1.1.1.1", "1.0.0.1"],
      quad9: ["9.9.9.9", "149.112.112.112"],
    }),
    [],
  );

  const showNotify = (msg, isError = false) => {
    setNotify({ msg, isError });
    setTimeout(() => setNotify(null), 4000);
  };

  const handleExportClick = () => {
    setPwdInput("");
    setPwdModal({ isOpen: true, mode: "export", data: null });
  };

  const executeExport = async () => {
    if (!pwdInput) return;

    const fullConfig = { proxies, routingRules, settings, subscriptions };
    const encrypted = await encryptWithPassword(fullConfig, pwdInput);
    if (!encrypted) {
      showNotify(t("settings.notify.read_error"), true);
      return;
    }
    const securePayload = {
      _isSecure: true,
      _version: 2,
      data: encrypted,
    };

    const dataStr =
      "data:text/json;charset=utf-8," +
      encodeURIComponent(JSON.stringify(securePayload, null, 2));
    const downloadAnchorNode = document.createElement("a");
    downloadAnchorNode.setAttribute("href", dataStr);
    downloadAnchorNode.setAttribute(
      "download",
      "resultv-secure-config.json",
    );
    document.body.appendChild(downloadAnchorNode);
    downloadAnchorNode.click();
    document.body.removeChild(downloadAnchorNode);

    setTimeout(() => {
      setPwdModal({ isOpen: false, mode: "", data: null });
    }, 100);
  };

  const handleImportClick = (e) => {
    const inputEl = e.target;
    const fileReader = new FileReader();
    if (!inputEl.files[0]) return;

    fileReader.readAsText(inputEl.files[0], "UTF-8");
    fileReader.onload = (event) => {
      try {
        const imported = JSON.parse(event.target.result);

        if (imported._isSecure && imported.data) {
          setPwdInput("");
          setPwdModal({ isOpen: true, mode: "import", data: imported.data });
        } else if (Array.isArray(imported)) {
          const parsedProxies = imported.map(p => ({ ...p, port: parseInt(p.port, 10) || 0 }));
          setProxies(parsedProxies);
          const nextSubs = rebuildSubscriptionsFromProxies(parsedProxies);
          setSubscriptions(nextSubs);
          wailsAPI.saveConfig({ proxies: parsedProxies, routingRules, settings, subscriptions: nextSubs }).catch(console.error);
          showNotify(t("settings.notify.import_old"));
        } else if (imported && typeof imported === "object") {
          let nextProxies = proxies;
          if (imported.proxies) {
            if (Array.isArray(imported.proxies)) {
              nextProxies = imported.proxies.map(p => ({ ...p, port: parseInt(p.port, 10) || 0 }));
              setProxies(nextProxies);
            } else {
               showNotify(t("settings.notify.invalid_format"), true);
               return;
            }
          }
          if (imported.routingRules) setRoutingRules(imported.routingRules);
          if (imported.settings) {
            if (imported.settings.autostart !== undefined)
              updateSetting("autostart", imported.settings.autostart);
            if (imported.settings.killswitch !== undefined)
              updateSetting("killswitch", imported.settings.killswitch);
            setSettings(imported.settings);
          }
          const nextSubs = Array.isArray(imported.subscriptions)
            ? imported.subscriptions
            : rebuildSubscriptionsFromProxies(nextProxies);
          setSubscriptions(nextSubs);
          wailsAPI.saveConfig({
            proxies: nextProxies,
            routingRules: imported.routingRules || routingRules,
            settings: imported.settings || settings,
            subscriptions: nextSubs,
          }).catch(console.error);
          showNotify(t("settings.notify.import_unsecured"));
        } else {
          showNotify(t("settings.notify.invalid_format"), true);
        }
      } catch (err) {
        showNotify(t("settings.notify.read_error"), true);
      }
      inputEl.value = "";
    };
  };

  const executeImport = async () => {
    if (!pwdInput || !pwdModal.data) return;
    const dec = await decryptWithPassword(pwdModal.data, pwdInput);
    if (dec) {
      const parsedProxies = (dec.proxies || []).map(p => ({ ...p, port: parseInt(p.port, 10) || 0 }));
      setProxies(parsedProxies);
      setRoutingRules(dec.routingRules || routingRules);
      setSettings(dec.settings || settings);
      const nextSubs = Array.isArray(dec.subscriptions)
        ? dec.subscriptions
        : rebuildSubscriptionsFromProxies(parsedProxies);
      setSubscriptions(nextSubs);
      wailsAPI.saveConfig({
        proxies: parsedProxies,
        routingRules: dec.routingRules || routingRules,
        settings: dec.settings || settings,
        subscriptions: nextSubs,
      }).catch(console.error);
      showNotify(t("settings.notify.decrypt_success"));
      setPwdModal({ isOpen: false, mode: "", data: null });
    } else {
      showNotify(t("settings.notify.decrypt_error"), true);
    }
  };

  useEffect(() => {
    setLocalPortInput(settings?.localPort ? String(settings.localPort) : "");
  }, [settings?.localPort]);

  useEffect(() => {
    setSubscriptionIntervalInput(
      settings?.subscriptionUpdateIntervalHours
        ? String(settings.subscriptionUpdateIntervalHours)
        : "6",
    );
  }, [settings?.subscriptionUpdateIntervalHours]);

  useEffect(() => {
    setSubscriptionUserAgentInput(settings?.subscriptionUserAgent || "");
  }, [settings?.subscriptionUserAgent]);

  useEffect(() => {
    const list = Array.isArray(settings?.dnsServers) ? settings.dnsServers : [];
    setDnsInput(list.join(", "));
    const toKey = (arr) => arr.join(",");
    const current = toKey(list);
    if (current === "") {
      setDnsPreset("auto");
      return;
    }
    if (current === toKey(DNS_PRESETS.google)) {
      setDnsPreset("google");
      return;
    }
    if (current === toKey(DNS_PRESETS.cloudflare)) {
      setDnsPreset("cloudflare");
      return;
    }
    if (current === toKey(DNS_PRESETS.quad9)) {
      setDnsPreset("quad9");
      return;
    }
    setDnsPreset("custom");
  }, [settings?.dnsServers, DNS_PRESETS]);

  useEffect(() => {
    if (!settings?.listenLan) return;
    wailsAPI.getLANIPs().then(setLanIPs).catch(() => setLanIPs([]));
  }, [settings?.listenLan]);

  const lanAddressText = useMemo(() => {
    if (!settings?.listenLan) return "";
    const port = Number(settings?.localPort || 0);
    if (!port) {
      return t("settings.lan_listen.addr_auto");
    }
    if (!lanIPs || lanIPs.length === 0) {
      return t("settings.lan_listen.addr_unknown", { port });
    }
    return lanIPs.map((ip) => `${ip}:${port}`).join(", ");
  }, [settings?.listenLan, settings?.localPort, lanIPs, t]);

  const commitLocalPort = async () => {
    const raw = String(localPortInput || "").trim();
    if (raw === "") {
      await updateSetting("localPort", 0);
      return;
    }
    const n = parseInt(raw, 10);
    if (!Number.isFinite(n) || n < 1 || n > 65535) {
      showNotify(t("settings.lan_listen.port_invalid"), true);
      setLocalPortInput(settings?.localPort ? String(settings.localPort) : "");
      return;
    }
    await updateSetting("localPort", n);
  };

  const commitSubscriptionInterval = async () => {
    const raw = String(subscriptionIntervalInput || "").trim();
    const n = parseInt(raw, 10);
    if (!Number.isFinite(n) || n < 1) {
      setSubscriptionIntervalInput(
        settings?.subscriptionUpdateIntervalHours
          ? String(settings.subscriptionUpdateIntervalHours)
          : "6",
      );
      return;
    }
    await updateSetting("subscriptionUpdateIntervalHours", n);
  };

  const commitSubscriptionUserAgent = async () => {
    await updateSetting(
      "subscriptionUserAgent",
      String(subscriptionUserAgentInput || "").trim(),
    );
  };

  const parseDNSInput = (raw) => {
    const parts = String(raw || "")
      .split(/[\s,]+/g)
      .map((s) => s.trim())
      .filter(Boolean);
    const seen = new Set();
    const result = [];
    for (const part of parts) {
      if (!seen.has(part)) {
        seen.add(part);
        result.push(part);
      }
    }
    return result;
  };

  const commitCustomDNS = async () => {
    const list = parseDNSInput(dnsInput);
    await updateSetting("dnsServers", list);
    setDnsInput(list.join(", "));
    setDnsPreset(list.length ? "custom" : "auto");
  };

  const applyDNSPreset = async (preset) => {
    setDnsPreset(preset);
    if (preset === "custom") {
      return;
    }
    const list = DNS_PRESETS[preset] || [];
    setDnsInput(list.join(", "));
    await updateSetting("dnsServers", list);
  };

  const SettingsSection = ({ children, className = "" }) => (
    <div className={`space-y-4 ${className}`}>
      <button
        type="button"
        onClick={() => setActiveSection(null)}
        className="inline-flex items-center gap-2 text-zinc-400 hover:text-white transition-colors"
      >
        <ArrowLeft className="w-4 h-4" />
        {t("settings.back")}
      </button>
      <div className="space-y-4">{children}</div>
    </div>
  );

  const selectFieldButtonClass =
    "w-full bg-zinc-950 border border-zinc-800 px-4 py-3 text-base hover:border-zinc-700 focus:ring-[#00A819]/35";

  const SettingsFieldCard = ({
    title,
    description,
    children,
    footer,
    inline = false,
  }) => {
    if (inline) {
      return (
        <div className="flex items-center justify-between gap-6 p-6 bg-zinc-900 rounded-3xl border border-zinc-800">
          <div className="pr-6 min-w-0">
            <h3 className="text-white font-bold text-lg">{title}</h3>
            {description ? (
              <p className="text-zinc-500 text-sm mt-1">{description}</p>
            ) : null}
          </div>
          <div className="shrink-0">{children}</div>
        </div>
      );
    }
    return (
      <div className="p-6 bg-zinc-900 rounded-3xl border border-zinc-800">
        <h3 className="text-white font-bold text-lg mb-2">{title}</h3>
        {description ? (
          <p className="text-zinc-500 text-sm mb-4">{description}</p>
        ) : null}
        {children}
        {footer ? <p className="text-zinc-500 text-sm mt-3">{footer}</p> : null}
      </div>
    );
  };

  const tunStackOptions = useMemo(
    () => [
      { value: "default", label: t("settings.tun_stack.default") },
      { value: "system", label: t("settings.tun_stack.system") },
      { value: "gvisor", label: t("settings.tun_stack.gvisor") },
    ],
    [t],
  );

  const SettingsMenuCard = ({ id, title, description }) => (
    <button
      type="button"
      onClick={() => setActiveSection(id)}
      className="w-full p-6 bg-zinc-900 hover:bg-zinc-800 rounded-3xl border border-zinc-800 hover:border-zinc-700 transition-colors text-left flex items-center justify-between gap-4"
    >
      <div>
        <h3 className="text-white font-bold text-lg mb-2">{title}</h3>
        <p className="text-zinc-500 text-sm">{description}</p>
      </div>
      <ChevronRight className="w-5 h-5 text-zinc-500 shrink-0" />
    </button>
  );

  const sectionHeader =
    {
      advanced: {
        title: t("settings.groups.advanced.title"),
        description: t("settings.groups.advanced.desc"),
      },
      subscriptions: {
        title: t("settings.groups.subscriptions.title"),
        description: t("settings.groups.subscriptions.desc"),
      },
      security: {
        title: t("settings.groups.security.title"),
        description: t("settings.groups.security.desc"),
      },
      network: {
        title: t("settings.groups.network.title"),
        description: t("settings.groups.network.desc"),
      },
    }[activeSection] || null;

  return (
    <div className="space-y-6 animate-in fade-in duration-300 relative">
      <div>
        <h2 className="text-3xl font-bold text-white">
          {sectionHeader?.title || t("settings.title")}
        </h2>
        <p className="text-zinc-400 mt-2">
          {sectionHeader?.description || t("settings.desc")}
        </p>
      </div>

      {notify && (
        <div
          className={`p-4 rounded-2xl border ${notify.isError ? "bg-rose-500/10 border-rose-500/20 text-rose-500" : "bg-[#007E3A]/10 border-[#007E3A]/20 text-[#00A819]"} flex items-center space-x-3 animate-in fade-in slide-in-from-top-4`}
        >
          {notify.isError ? (
            <Activity className="w-5 h-5 shrink-0" />
          ) : (
            <Check className="w-5 h-5 shrink-0" />
          )}
          <span className="font-medium">{notify.msg}</span>
        </div>
      )}

      {!activeSection && (
        <>
          <div className="space-y-4">
          <SettingsMenuCard
            id="advanced"
            title={t("settings.groups.advanced.title")}
            description={t("settings.groups.advanced.desc")}
          />
          <SettingsMenuCard
            id="subscriptions"
            title={t("settings.groups.subscriptions.title")}
            description={t("settings.groups.subscriptions.desc")}
          />
          <SettingsMenuCard
            id="security"
            title={t("settings.groups.security.title")}
            description={t("settings.groups.security.desc")}
          />
          <SettingsMenuCard
            id="network"
            title={t("settings.groups.network.title")}
            description={t("settings.groups.network.desc")}
          />
          </div>

          <div className="p-6 bg-zinc-900 rounded-3xl border border-zinc-800 mt-10">
            <h3 className="text-white font-bold text-lg mb-2">
              {t("settings.export_import.title")}
            </h3>
            <p className="text-zinc-500 text-sm mb-6">
              <Shield className="inline-block w-4 h-4 mr-1 text-[#00A819]" />
              {t("settings.export_import.desc")}
            </p>
            <div className="flex space-x-4">
              <button
                onClick={handleExportClick}
                className="flex items-center px-6 py-3 bg-zinc-800 hover:bg-zinc-700 text-white hover:text-[#00A819] rounded-xl font-medium transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
              >
                <Download className="w-5 h-5 mr-2" />{" "}
                {t("settings.export_import.export_btn")}
              </button>

              <input
                type="file"
                id="import-file"
                onChange={handleImportClick}
                accept=".json"
                className="hidden"
              />
              <button
                onClick={() => document.getElementById("import-file").click()}
                className="flex items-center px-6 py-3 bg-zinc-800 hover:bg-zinc-700 text-white hover:text-[#00A819] rounded-xl font-medium transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
              >
                <Upload className="w-5 h-5 mr-2" />{" "}
                {t("settings.export_import.import_btn")}
              </button>
            </div>
          </div>
        </>
      )}

      {activeSection === "advanced" && (
      <SettingsSection>
        <SettingsFieldCard
          title={t("settings.tun_stack.title")}
          description={t("settings.tun_stack.desc")}
        >
          <AppSelect
            value={settings?.tunStack || "default"}
            options={tunStackOptions}
            onChange={(v) =>
              updateSetting("tunStack", v === "default" ? "" : v)
            }
            ariaLabel={t("settings.tun_stack.title")}
            className="w-full"
            buttonClassName={selectFieldButtonClass}
            listClassName="w-full"
          />
        </SettingsFieldCard>
        <SettingToggle
          title={t("settings.autostart.title")}
          description={t("settings.autostart.desc")}
          isOn={settings.autostart}
          onToggle={() => updateSetting("autostart", !settings.autostart)}
        />
      </SettingsSection>
      )}

      {activeSection === "subscriptions" && (
      <SettingsSection>
        <SettingToggle
          title={t("settings.subscription_auto_update.title")}
          description={t("settings.subscription_auto_update.desc")}
          isOn={settings.subscriptionAutoUpdate !== false}
          onToggle={() =>
            updateSetting(
              "subscriptionAutoUpdate",
              !(settings.subscriptionAutoUpdate !== false),
            )
          }
        />
        <SettingsFieldCard
          inline
          title={t("settings.subscription_interval.title")}
          description={t("settings.subscription_interval.desc")}
        >
          <input
            type="number"
            min="1"
            step="1"
            className="w-40 bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:border-[#007E3A] transition-colors focus:ring-0"
            value={subscriptionIntervalInput}
            onChange={(e) => {
              const v = e.target.value;
              if (v === "" || /^[0-9]+$/.test(v)) {
                setSubscriptionIntervalInput(v);
                const n = parseInt(v, 10);
                if (Number.isFinite(n) && n >= 1) {
                  updateSetting("subscriptionUpdateIntervalHours", n);
                }
              }
            }}
            onBlur={() => commitSubscriptionInterval()}
          />
        </SettingsFieldCard>
        <SettingToggle
          title={t("settings.subscription_hwid.title")}
          description={t("settings.subscription_hwid.desc")}
          isOn={settings.subscriptionSendHWID !== false}
          onToggle={() =>
            updateSetting(
              "subscriptionSendHWID",
              !(settings.subscriptionSendHWID !== false),
            )
          }
        />
        <SettingsFieldCard
          title={t("settings.subscription_user_agent.title")}
          footer={t("settings.subscription_user_agent.desc")}
        >
          <input
            className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:border-[#007E3A] transition-colors focus:ring-0"
            placeholder={t("settings.subscription_user_agent.placeholder")}
            value={subscriptionUserAgentInput}
            onChange={(e) => setSubscriptionUserAgentInput(e.target.value)}
            onBlur={() => commitSubscriptionUserAgent()}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                commitSubscriptionUserAgent();
              }
            }}
          />
        </SettingsFieldCard>
      </SettingsSection>
      )}

      {activeSection === "security" && (
      <SettingsSection>
        <SettingToggle
          title={t("settings.killswitch.title")}
          description={t("settings.killswitch.desc")}
          isOn={settings.killswitch}
          onToggle={() => updateSetting("killswitch", !settings.killswitch)}
        />
        <SettingToggle
          title={t("settings.dns_leak_protection.title")}
          description={t("settings.dns_leak_protection.desc")}
          isOn={settings.dnsLeakProtection !== false}
          onToggle={() =>
            updateSetting(
              "dnsLeakProtection",
              !(settings.dnsLeakProtection !== false),
            )
          }
        />
      </SettingsSection>
      )}

      {activeSection === "network" && (
      <SettingsSection>
        <div className="p-6 bg-zinc-900 rounded-3xl border border-zinc-800">
          <h3 className="text-white font-bold text-lg mb-2">
            {t("settings.dns.title")}
          </h3>
          <p className="text-zinc-500 text-sm mb-4">{t("settings.dns.desc")}</p>

          <div className="grid sm:grid-cols-2 gap-3 mb-4">
            <button
              onClick={() => applyDNSPreset("auto")}
              className={`px-4 py-3 rounded-xl font-medium transition-colors border ${dnsPreset === "auto" ? "bg-[#007E3A] text-white border-[#007E3A]" : "bg-zinc-800 hover:bg-zinc-700 text-white border-zinc-700"}`}
            >
              {t("settings.dns.preset_auto")}
            </button>
            <button
              onClick={() => applyDNSPreset("google")}
              className={`px-4 py-3 rounded-xl font-medium transition-colors border ${dnsPreset === "google" ? "bg-[#007E3A] text-white border-[#007E3A]" : "bg-zinc-800 hover:bg-zinc-700 text-white border-zinc-700"}`}
            >
              {t("settings.dns.preset_google")}
            </button>
            <button
              onClick={() => applyDNSPreset("cloudflare")}
              className={`px-4 py-3 rounded-xl font-medium transition-colors border ${dnsPreset === "cloudflare" ? "bg-[#007E3A] text-white border-[#007E3A]" : "bg-zinc-800 hover:bg-zinc-700 text-white border-zinc-700"}`}
            >
              {t("settings.dns.preset_cloudflare")}
            </button>
            <button
              onClick={() => applyDNSPreset("quad9")}
              className={`px-4 py-3 rounded-xl font-medium transition-colors border ${dnsPreset === "quad9" ? "bg-[#007E3A] text-white border-[#007E3A]" : "bg-zinc-800 hover:bg-zinc-700 text-white border-zinc-700"}`}
            >
              {t("settings.dns.preset_quad9")}
            </button>
          </div>

          <div className="flex flex-wrap items-center gap-3">
            <input
              placeholder={t("settings.dns.custom_placeholder")}
              className="min-w-[260px] flex-1 bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:border-[#007E3A] transition-colors focus:ring-0"
              value={dnsInput}
              onChange={(e) => {
                setDnsInput(e.target.value);
                if (dnsPreset !== "custom") setDnsPreset("custom");
              }}
              onBlur={() => {
                if (dnsPreset === "custom") {
                  commitCustomDNS();
                }
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  commitCustomDNS();
                }
              }}
            />
            <button
              onClick={() => commitCustomDNS()}
              className="px-6 py-3 bg-zinc-800 hover:bg-zinc-700 text-white hover:text-[#00A819] rounded-xl font-medium transition-colors"
            >
              {t("settings.dns.apply")}
            </button>
          </div>
        </div>
        <div className="space-y-4">
          <SettingToggle
            title={t("settings.lan_listen.toggle_title")}
            description={t("settings.lan_listen.toggle_desc")}
            isOn={!!settings.listenLan}
            onToggle={() => updateSetting("listenLan", !settings.listenLan)}
          />

          <div className="p-6 bg-zinc-900 rounded-3xl border border-zinc-800">
            <h3 className="text-white font-bold text-lg mb-2">
              {t("settings.lan_listen.port_title")}
            </h3>
          <p className="text-zinc-500 text-sm mb-4">
            {t("settings.lan_listen.port_desc")}
          </p>

          <div className="flex items-center gap-3">
            <input
              inputMode="numeric"
              pattern="[0-9]*"
              placeholder={t("settings.lan_listen.port_placeholder")}
              className="w-40 bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:border-[#007E3A] transition-colors focus:ring-0"
              value={localPortInput}
              onChange={(e) => {
                const v = e.target.value;
                if (v === "" || /^[0-9]+$/.test(v)) setLocalPortInput(v);
              }}
              onBlur={() => commitLocalPort()}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  commitLocalPort();
                }
              }}
            />
            <button
              onClick={() => commitLocalPort()}
              className="px-6 py-3 bg-zinc-800 hover:bg-zinc-700 text-white hover:text-[#00A819] rounded-xl font-medium transition-colors"
            >
              {t("settings.lan_listen.port_apply")}
            </button>
          </div>

          {settings.listenLan && (
            <div className="mt-4 text-sm text-zinc-400">
              <div className="text-zinc-500 mb-1">
                {t("settings.lan_listen.addr_title")}
              </div>
              <div className="break-words">{lanAddressText}</div>
            </div>
          )}
          </div>
        </div>
      </SettingsSection>
      )}

      {pwdModal.isOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-zinc-950/80 backdrop-blur-sm p-4 animate-in fade-in duration-200">
          <div className="bg-zinc-900 border border-zinc-800 rounded-3xl p-6 max-w-md w-full shadow-2xl animate-in zoom-in-95 duration-200">
            <div className="flex items-center space-x-3 mb-4">
              <Lock className="w-6 h-6 text-[#007E3A]" />
              <h3 className="text-xl font-bold text-white">
                {pwdModal.mode === "export"
                  ? t("settings.modal.title_export")
                  : t("settings.modal.title_import")}
              </h3>
            </div>
            <p className="text-zinc-400 text-sm mb-6">
              {pwdModal.mode === "export"
                ? t("settings.modal.desc_export")
                : t("settings.modal.desc_import")}
            </p>
            <input
              type="password"
              placeholder={t("settings.modal.placeholder")}
              className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:border-[#007E3A] transition-colors mb-6 focus:ring-0"
              value={pwdInput}
              onChange={(e) => setPwdInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  pwdModal.mode === "export"
                    ? executeExport()
                    : executeImport();
                }
              }}
              autoFocus
            />
            <div className="flex space-x-3">
              <button
                onClick={() =>
                  setPwdModal({ isOpen: false, mode: "", data: null })
                }
                className="flex-1 bg-zinc-800 hover:bg-zinc-700 text-white font-bold py-3 rounded-xl transition-colors border-transparent outline-none focus:outline-none"
              >
                {t("settings.modal.cancel")}
              </button>
              <button
                onClick={
                  pwdModal.mode === "export" ? executeExport : executeImport
                }
                className="flex-1 bg-[#007E3A] hover:bg-[#005C2A] text-white font-bold py-3 rounded-xl transition-colors border-transparent outline-none focus:outline-none"
              >
                {pwdModal.mode === "export"
                  ? t("settings.modal.encrypt")
                  : t("settings.modal.open")}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
