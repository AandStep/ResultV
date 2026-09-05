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
 * Страница «Настройки», подключённая к приложению.
 *
 * Вид держит SettingsPage, здесь всё остальное: перевод названий настроек в
 * ключи конфига, адреса прокси в локальной сети и экспорт с импортом
 * конфигурации. Всё это перенесено с прежней страницы как есть: менялся
 * только вид.
 */

import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useConfigContext } from "../../context/ConfigContext";
import { encryptWithPassword, decryptWithPassword } from "../../utils/crypto";
import { rebuildSubscriptionsFromProxies } from "../../utils/proxyParser";
import wailsAPI from "../../utils/wailsAPI";
import AppSidebar from "./AppSidebar";
import ConfigPasswordDialog from "./ConfigPasswordDialog";
import SettingsPage, {
  DNS_PRESETS,
  SUBSCRIPTION_HOURS,
} from "./SettingsPage";

/* Имя файла резервной копии — то же, что было на прежней странице. */
const BACKUP_FILE = "resultv-secure-config.json";

/* Разбор строки DNS: адреса через запятую или пробел, без повторов. */
const parseDNS = (raw) => {
  const parts = String(raw || "")
    .split(/[\s,]+/g)
    .map((part) => part.trim())
    .filter(Boolean);
  return [...new Set(parts)];
};

export default function SettingsScreen() {
  const { t, i18n } = useTranslation();
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
    showAlertDialog,
  } = useConfigContext();

  /* Пустая строка — список пунктов; иначе открыта страница этой группы. */
  const [section, setSection] = useState("");
  /* `mode` — что делает окно пароля: шифрует выгрузку или открывает файл.
     `data` — зашифрованное содержимое выбранного файла. */
  const [pwdDialog, setPwdDialog] = useState({ mode: "", data: null });
  const [lanIPs, setLanIPs] = useState([]);

  /* Escape уводит со страницы пункта назад к списку — тем же путём, каким
     он закрывает любое окно приложения. */
  useEffect(() => {
    if (!section) return undefined;
    const onKey = (event) => {
      if (event.key === "Escape") setSection("");
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [section]);

  useEffect(() => {
    if (!settings?.listenLan) return;
    wailsAPI
      .getLANIPs()
      .then(setLanIPs)
      .catch(() => setLanIPs([]));
  }, [settings?.listenLan]);

  const lanAddress = useMemo(() => {
    if (!settings?.listenLan) return "";
    const port = Number(settings?.localPort || 0);
    if (!port) return t("settings.lan_listen.addr_auto");
    if (!lanIPs || lanIPs.length === 0) {
      return t("settings.lan_listen.addr_unknown", { port });
    }
    return lanIPs.map((ip) => `${ip}:${port}`).join(", ");
  }, [settings?.listenLan, settings?.localPort, lanIPs, t]);

  /*
   * Интервал обновления подписок в прежних настройках был числом, а не
   * выбором из готовых, поэтому в конфиге может стоять что угодно. Значение
   * не из набора становится дополнительной кнопкой в начале ряда — иначе
   * отмеченной не была бы ни одна и текущий интервал пропал бы из виду.
   */
  const intervalHours = settings?.subscriptionUpdateIntervalHours || 6;
  const intervals = useMemo(() => {
    const label = (hours) =>
      t("settings.subscription_interval.hours", { hours });
    const preset = SUBSCRIPTION_HOURS.map((item) => ({
      hours: item.hours,
      label: label(item.hours),
    }));
    if (preset.some((item) => item.hours === intervalHours)) return preset;
    return [{ hours: intervalHours, label: label(intervalHours) }, ...preset];
  }, [intervalHours, t]);

  const values = {
    tunStack: settings?.tunStack || "default",
    autostart: !!settings?.autostart,
    language: i18n.language?.startsWith("en") ? "en" : "ru",
    subAutoUpdate: settings?.subscriptionAutoUpdate !== false,
    subIntervalHours: intervalHours,
    subHwid: settings?.subscriptionSendHWID !== false,
    subUserAgent: settings?.subscriptionUserAgent || "",
    killSwitch: !!settings?.killswitch,
    dnsLeak: settings?.dnsLeakProtection !== false,
    dnsServers: Array.isArray(settings?.dnsServers) ? settings.dnsServers : [],
    ipv6: !!settings?.enableIPv6,
    listenLan: !!settings?.listenLan,
    localPort: Number(settings?.localPort || 0),
  };

  const change = (key, value) => {
    switch (key) {
      case "tunStack":
        /* «По умолчанию» — это пустое поле конфига, а не слово в нём. */
        return updateSetting("tunStack", value === "default" ? "" : value);
      case "autostart":
        return updateSetting("autostart", value);
      case "language":
        return i18n.changeLanguage(value);
      case "subAutoUpdate":
        return updateSetting("subscriptionAutoUpdate", value);
      case "subIntervalHours":
        return updateSetting("subscriptionUpdateIntervalHours", value);
      case "subHwid":
        return updateSetting("subscriptionSendHWID", value);
      case "subUserAgent":
        return updateSetting("subscriptionUserAgent", value);
      case "killSwitch":
        return updateSetting("killswitch", value);
      case "dnsLeak":
        return updateSetting("dnsLeakProtection", value);
      case "dnsServers":
        /* Кнопка набора отдаёт готовый список, поле — строку. */
        return updateSetting(
          "dnsServers",
          Array.isArray(value) ? value : parseDNS(value),
        );
      case "ipv6":
        return updateSetting("enableIPv6", value);
      case "listenLan":
        return updateSetting("listenLan", value);
      case "localPort": {
        const raw = String(value || "").trim();
        if (raw === "") return updateSetting("localPort", 0);
        const port = parseInt(raw, 10);
        if (!Number.isFinite(port) || port < 1 || port > 65535) {
          showAlertDialog({
            title: t("settings.lan_listen.port_title"),
            message: t("settings.lan_listen.port_invalid"),
            variant: "danger",
          });
          /* `false` — отказ: поле вернёт набранное к сохранённому порту. */
          return false;
        }
        return updateSetting("localPort", port);
      }
      default:
        return undefined;
    }
  };

  /* --- Экспорт и импорт --------------------------------------------------- */

  const notify = (message, isError = false) =>
    showAlertDialog({
      title: t("settings.export_import.title"),
      message,
      variant: isError ? "danger" : "info",
    });

  const runExport = async (password) => {
    const encrypted = await encryptWithPassword(
      { proxies, routingRules, settings, subscriptions },
      password,
    );
    if (!encrypted) {
      setPwdDialog({ mode: "", data: null });
      notify(t("settings.notify.read_error"), true);
      return;
    }
    const payload = { _isSecure: true, _version: 2, data: encrypted };
    const link = document.createElement("a");
    link.setAttribute(
      "href",
      `data:text/json;charset=utf-8,${encodeURIComponent(
        JSON.stringify(payload, null, 2),
      )}`,
    );
    link.setAttribute("download", BACKUP_FILE);
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    setPwdDialog({ mode: "", data: null });
  };

  const runImport = async (password) => {
    const decoded = await decryptWithPassword(pwdDialog.data, password);
    if (!decoded) {
      notify(t("settings.notify.decrypt_error"), true);
      return;
    }
    const nextProxies = (decoded.proxies || []).map((proxy) => ({
      ...proxy,
      port: parseInt(proxy.port, 10) || 0,
    }));
    const nextRules = decoded.routingRules || routingRules;
    const nextSettings = decoded.settings || settings;
    const nextSubs = Array.isArray(decoded.subscriptions)
      ? decoded.subscriptions
      : rebuildSubscriptionsFromProxies(nextProxies);

    setProxies(nextProxies);
    setRoutingRules(nextRules);
    setSettings(nextSettings);
    setSubscriptions(nextSubs);
    await wailsAPI
      .saveConfig({
        proxies: nextProxies,
        routingRules: nextRules,
        settings: nextSettings,
        subscriptions: nextSubs,
      })
      .catch(console.error);

    setPwdDialog({ mode: "", data: null });
    notify(t("settings.notify.decrypt_success"));
  };

  /*
   * Файл выбирается скрытым `input`, который живёт ровно один выбор: держать
   * его в разметке страницы незачем — кнопка импорта и есть его вид.
   */
  const pickFile = () => {
    const input = document.createElement("input");
    input.type = "file";
    input.accept = ".json";
    input.onchange = () => {
      const file = input.files?.[0];
      if (!file) return;
      const reader = new FileReader();
      reader.onload = (event) => readBackup(event.target.result);
      reader.readAsText(file, "UTF-8");
    };
    input.click();
  };

  /* Форматов три: зашифрованный, голый объект конфига и самый старый —
     просто список серверов. Все три открывались и раньше. */
  const readBackup = async (raw) => {
    let imported;
    try {
      imported = JSON.parse(raw);
    } catch {
      notify(t("settings.notify.read_error"), true);
      return;
    }

    if (imported?._isSecure && imported.data) {
      setPwdDialog({ mode: "import", data: imported.data });
      return;
    }

    if (Array.isArray(imported)) {
      const nextProxies = imported.map((proxy) => ({
        ...proxy,
        port: parseInt(proxy.port, 10) || 0,
      }));
      const nextSubs = rebuildSubscriptionsFromProxies(nextProxies);
      setProxies(nextProxies);
      setSubscriptions(nextSubs);
      await wailsAPI
        .saveConfig({
          proxies: nextProxies,
          routingRules,
          settings,
          subscriptions: nextSubs,
        })
        .catch(console.error);
      notify(t("settings.notify.import_old"));
      return;
    }

    if (!imported || typeof imported !== "object") {
      notify(t("settings.notify.invalid_format"), true);
      return;
    }

    let nextProxies = proxies;
    if (imported.proxies) {
      if (!Array.isArray(imported.proxies)) {
        notify(t("settings.notify.invalid_format"), true);
        return;
      }
      nextProxies = imported.proxies.map((proxy) => ({
        ...proxy,
        port: parseInt(proxy.port, 10) || 0,
      }));
      setProxies(nextProxies);
    }
    if (imported.routingRules) setRoutingRules(imported.routingRules);
    if (imported.settings) {
      if (imported.settings.autostart !== undefined) {
        await updateSetting("autostart", imported.settings.autostart);
      }
      if (imported.settings.killswitch !== undefined) {
        await updateSetting("killswitch", imported.settings.killswitch);
      }
      setSettings(imported.settings);
    }
    const nextSubs = Array.isArray(imported.subscriptions)
      ? imported.subscriptions
      : rebuildSubscriptionsFromProxies(nextProxies);
    setSubscriptions(nextSubs);
    await wailsAPI
      .saveConfig({
        proxies: nextProxies,
        routingRules: imported.routingRules || routingRules,
        settings: imported.settings || settings,
        subscriptions: nextSubs,
      })
      .catch(console.error);
    notify(t("settings.notify.import_unsecured"));
  };

  const group = (key) => ({
    title: t(`settings.groups.${key}.title`),
    items: t(`settings.groups.${key}.items`),
    desc: t(`settings.groups.${key}.desc`),
  });

  const row = (key, extra = {}) => ({
    title: t(`settings.${key}.title`),
    desc: t(`settings.${key}.desc`),
    ...extra,
  });

  return (
    <>
      <SettingsPage
        sidebar={<AppSidebar />}
        section={section}
        onOpenSection={setSection}
        onBack={() => setSection("")}
        values={values}
        onChange={change}
        lanAddress={lanAddress}
        intervals={intervals}
        tunStacks={[
          { value: "default", label: t("settings.tun_stack.default") },
          { value: "system", label: t("settings.tun_stack.system") },
          { value: "gvisor", label: t("settings.tun_stack.gvisor") },
        ]}
        languages={[
          { value: "ru", label: t("settings.language.ru") },
          { value: "en", label: t("settings.language.en") },
        ]}
        dnsPresets={DNS_PRESETS.map((preset) => ({
          ...preset,
          label: t(`settings.dns.preset_${preset.id}`),
        }))}
        onExport={() => setPwdDialog({ mode: "export", data: null })}
        onImport={pickFile}
        text={{
          title: t("settings.title"),
          subtitle: t("settings.desc"),
          back: t("settings.back"),
          groups: {
            advanced: group("advanced"),
            subscriptions: group("subscriptions"),
            security: group("security"),
            network: group("network"),
          },
          exportImport: {
            title: t("settings.export_import.title"),
            desc: t("settings.export_import.desc"),
            exportBtn: t("settings.export_import.export_btn"),
            importBtn: t("settings.export_import.import_btn"),
          },
          rows: {
            tunStack: row("tun_stack"),
            autostart: row("autostart"),
            language: row("language"),
            subAutoUpdate: row("subscription_auto_update"),
            subInterval: row("subscription_interval"),
            subHwid: row("subscription_hwid"),
            subUserAgent: row("subscription_user_agent", {
              placeholder: t("settings.subscription_user_agent.placeholder"),
            }),
            killSwitch: row("killswitch"),
            dnsLeak: row("dns_leak_protection"),
            dns: row("dns", {
              customLabel: t("settings.dns.custom_label"),
              placeholder: t("settings.dns.custom_placeholder"),
            }),
            ipv6: row("enable_ipv6"),
            listenLan: {
              title: t("settings.lan_listen.toggle_title"),
              desc: t("settings.lan_listen.toggle_desc"),
            },
            port: {
              title: t("settings.lan_listen.port_title"),
              desc: t("settings.lan_listen.port_desc"),
              placeholder: t("settings.lan_listen.port_placeholder"),
              addrTitle: t("settings.lan_listen.addr_title"),
            },
          },
        }}
      />

      <ConfigPasswordDialog
        open={pwdDialog.mode !== ""}
        title={t(
          pwdDialog.mode === "export"
            ? "settings.modal.title_export"
            : "settings.modal.title_import",
        )}
        message={t(
          pwdDialog.mode === "export"
            ? "settings.modal.desc_export"
            : "settings.modal.desc_import",
        )}
        placeholder={t("settings.modal.placeholder")}
        cancelLabel={t("settings.modal.cancel")}
        submitLabel={t(
          pwdDialog.mode === "export"
            ? "settings.modal.encrypt"
            : "settings.modal.open",
        )}
        onSubmit={pwdDialog.mode === "export" ? runExport : runImport}
        onClose={() => setPwdDialog({ mode: "", data: null })}
      />
    </>
  );
}
