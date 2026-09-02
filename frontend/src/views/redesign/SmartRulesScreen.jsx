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
 * Страница «Умные правила», подключённая к настоящей конфигурации.
 *
 * Вид держит SmartRulesPage, состояние — routingRules из ConfigContext.
 *
 * Каждый режим правит СВОЮ пару полей, и это не украшение, а разная логика:
 * в Global по умолчанию всё идёт в туннель и списки его РАЗГРУЖАЮТ
 * (`whitelist` / `appWhitelist` — «напрямую»), в Smart по умолчанию всё идёт
 * напрямую и те же два блока, наоборот, ДОБАВЛЯЮТ в туннель
 * (`customBlockedDomains` / `appForceVPN` — «через ВПН»). Поэтому у блоков
 * переворачиваются подписи, а переключение режима не рушит списки соседнего.
 */

import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useConfigContext } from "../../context/ConfigContext";
import { PickAppForWhitelist } from "../../../wailsjs/go/main/App";
import wailsAPI from "../../utils/wailsAPI";
import SmartRulesPage from "./SmartRulesPage";
import RoutingProfilesDialog from "./RoutingProfilesDialog";
import RoutingProfileEditor from "./RoutingProfileEditor";
import AppSidebar from "./AppSidebar";

export default function SmartRulesScreen() {
  const { t } = useTranslation();
  const {
    routingRules: rules,
    setRoutingRules: setRules,
    showConfirmDialog,
    showAlertDialog,
    platform,
  } = useConfigContext();

  const isWin =
    platform === "win32" || platform === "windows" || platform === "win64";
  const isSmart = rules.mode === "smart";

  /* --- Профили маршрутизации (только глобальный режим) -------------------- */

  const [profiles, setProfiles] = useState([]);
  const [activeId, setActiveId] = useState("");
  /* Списки маршрутизации от подписок. Хранятся отдельно от профилей, но по
     модели пользователя это та же маршрутизация, поэтому показываются в том
     же окне. */
  const [lists, setLists] = useState([]);
  const [profilesOpen, setProfilesOpen] = useState(false);
  /* null — закрыт; объект профиля — правка; {} — создание. */
  const [editing, setEditing] = useState(null);
  const [busy, setBusy] = useState(false);

  const reloadProfiles = useCallback(async () => {
    try {
      const res = await wailsAPI.getRoutingProfiles();
      const list = (res?.profiles || []).map((p) => ({
        ...p,
        counts: {
          direct: (p.directSites?.length || 0) + (p.directIp?.length || 0),
          proxy: (p.proxySites?.length || 0) + (p.proxyIp?.length || 0),
          block: (p.blockSites?.length || 0) + (p.blockIp?.length || 0),
        },
      }));
      setProfiles(list);
      setActiveId(res?.activeId || "");

      const cfg = await wailsAPI.getConfig();
      const subs = cfg?.subscriptions || [];
      const nameOf = (id) => subs.find((sub) => sub.id === id)?.name || "";
      setLists(
        (cfg?.routingRules?.routingLists || []).map((rl) => ({
          ...rl,
          /* Имя провайдера впереди: списков от одной подписки бывает
             несколько, и без него в окне три одинаковых «Встроен: direct». */
          name: [nameOf(rl.subscriptionId), rl.name || rl.url]
            .filter(Boolean)
            .join(" · "),
          counts: {
            [rl.action]: (rl.domainCount || 0) + (rl.cidrCount || 0),
          },
        }))
      );
    } catch (err) {
      console.error("getRoutingProfiles:", err);
    }
  }, []);

  /* Список нужен только когда окно открыто — до этого читать нечего. */
  useEffect(() => {
    if (profilesOpen) reloadProfiles();
  }, [profilesOpen, reloadProfiles]);

  const report = (err) =>
    showAlertDialog({
      title: t("common.error"),
      message: String(err?.message || err),
      variant: "danger",
    });

  const selectProfile = async (profile) => {
    /* Повторное нажатие по активному выключает профили, не удаляя их. */
    const next = profile.id === activeId ? "" : profile.id;
    try {
      await wailsAPI.setActiveRoutingProfile(next);
      await reloadProfiles();
    } catch (err) {
      report(err);
    }
  };

  const deleteProfile = async (profile) => {
    const ok = await showConfirmDialog({
      title: t("common.confirmAction"),
      message: t("routingProfiles.confirmDelete", { name: profile.name }),
      variant: "danger",
      confirmText: t("common.delete"),
      cancelText: t("common.cancel"),
    });
    if (!ok) return;
    try {
      await wailsAPI.deleteRoutingProfile(profile.id);
      await reloadProfiles();
    } catch (err) {
      report(err);
    }
  };

  const toggleList = async (list) => {
    try {
      await wailsAPI.updateRoutingList({ ...list, enabled: !list.enabled });
      await reloadProfiles();
    } catch (err) {
      report(err);
    }
  };

  const deleteList = async (list) => {
    const ok = await showConfirmDialog({
      title: t("common.confirmAction"),
      message: t("routingProfiles.confirmDelete", { name: list.name }),
      variant: "danger",
      confirmText: t("common.delete"),
      cancelText: t("common.cancel"),
    });
    if (!ok) return;
    try {
      await wailsAPI.deleteRoutingList(list.id);
      await reloadProfiles();
    } catch (err) {
      report(err);
    }
  };

  const saveProfile = async (draft) => {
    setBusy(true);
    try {
      await wailsAPI.saveRoutingProfile(draft);
      setEditing(null);
      await reloadProfiles();
    } catch (err) {
      report(err);
    } finally {
      setBusy(false);
    }
  };

  const importProfile = async () => {
    let url = "";
    try {
      url = await navigator.clipboard.readText();
    } catch (err) {
      console.error("clipboard:", err);
    }
    url = (url || "").trim();
    if (!url) {
      showAlertDialog({
        title: t("common.notice"),
        message: t("routingProfiles.importHint"),
        variant: "warning",
      });
      return;
    }
    setBusy(true);
    try {
      await wailsAPI.importRoutingDeepLink(url, true);
      await reloadProfiles();
    } catch (err) {
      report(err);
    } finally {
      setBusy(false);
    }
  };

  const siteField = isSmart ? "customBlockedDomains" : "whitelist";
  const appField = isSmart ? "appForceVPN" : "appWhitelist";
  const sites = rules[siteField] || [];
  const apps = rules[appField] || [];

  const put = (field, list) => setRules({ ...rules, [field]: list });

  const addTo = (field, list, value) => {
    if (!value || list.includes(value)) return;
    put(field, [...list, value]);
  };

  const clear = async (field, list, message) => {
    if (list.length === 0) return;
    const ok = await showConfirmDialog({
      title: t("common.confirmAction"),
      message,
      variant: "danger",
      confirmText: t("common.delete"),
      cancelText: t("common.cancel"),
    });
    if (ok) put(field, []);
  };

  /* Имя приложения приводится к тому виду, в котором его ждёт движок: нижний
     регистр и расширение платформы. Логика перенесена из RulesView. */
  const normalizeApp = (raw) => {
    let name = String(raw).toLowerCase().trim();
    if (isWin && !name.endsWith(".exe")) name += ".exe";
    if (!isWin && !name.includes(".")) name += ".app";
    return name;
  };

  const pickApp = async () => {
    try {
      const picked = await PickAppForWhitelist();
      if (picked) addTo(appField, apps, normalizeApp(picked));
    } catch (err) {
      console.error("PickAppForWhitelist err:", err);
    }
  };

  const text = {
    title: t("smartRules.title"),
    subtitle: t("smartRules.subtitle"),
    smart: t("smartRules.smart"),
    global: t("smartRules.global"),
    sitesTitle: t(isSmart ? "smartRules.sitesVpn" : "smartRules.sitesDirect"),
    sitesSubtitle: t(
      isSmart ? "smartRules.sitesVpnDesc" : "smartRules.sitesDirectDesc"
    ),
    sitesPlaceholder: t("smartRules.sitesPlaceholder"),
    appsTitle: t(isSmart ? "smartRules.appsVpn" : "smartRules.appsDirect"),
    appsSubtitle: t(
      isSmart ? "smartRules.appsVpnDesc" : "smartRules.appsDirectDesc"
    ),
    /* Пример в подсказке в макете свой у каждого режима, и это не случайность:
       в Smart в туннель заворачивают Discord (голос идёт мимо списков), в
       Global из туннеля выносят Steam. Расширение — по платформе. */
    appsPlaceholder: t("smartRules.appsPlaceholder", {
      example: (isSmart ? "discord" : "steam") + (isWin ? ".exe" : ".app"),
    }),
    pickFile: t("smartRules.pickFile"),
    clear: t("smartRules.clear"),
    remove: t("smartRules.remove"),
    profilesTitle: t("smartRules.profilesTitle"),
    profilesDesc: t("smartRules.profilesDesc"),
  };

  const rp = (key, opts) => t(`routingProfiles.${key}`, opts);
  const profilesText = {
    title: rp("title"),
    subtitle: rp("subtitle"),
    active: rp("active"),
    all: rp("all"),
    actions: rp("actions"),
    create: rp("create"),
    import: rp("import"),
    edit: rp("edit"),
    remove: rp("remove"),
    empty: rp("empty"),
    lists: rp("lists"),
    listOn: rp("listOn"),
    listOff: rp("listOff"),
  };
  const editorText = {
    createTitle: rp("createTitle"),
    createSubtitle: rp("createSubtitle"),
    editTitle: rp("editTitle"),
    editSubtitle: rp("editSubtitle"),
    name: rp("name"),
    namePlaceholder: rp("namePlaceholder"),
    direct: rp("direct"),
    proxy: rp("proxy"),
    block: rp("block"),
    domains: rp("domains"),
    ips: rp("ips"),
    domainsPlaceholder: rp("domainsPlaceholder"),
    ipsPlaceholder: rp("ipsPlaceholder"),
    strategy: rp("strategy"),
    strategyHint: rp("strategyHint"),
    geo: rp("geo"),
    geoip: rp("geoip"),
    geosite: rp("geosite"),
    urlPlaceholder: rp("urlPlaceholder"),
    actions: rp("actions"),
    reset: rp("reset"),
    save: rp("save"),
  };

  return (
    <>
      <SmartRulesPage
      mode={isSmart ? "smart" : "global"}
      onModeChange={(mode) => setRules({ ...rules, mode })}
      sites={sites}
      /* Домен приводится к нижнему регистру: в конфиге он ключ, и «VK.com»
         рядом с «vk.com» дал бы две записи вместо одной. */
      onAddSite={(value) => addTo(siteField, sites, value.toLowerCase())}
      onRemoveSite={(value) => put(siteField, sites.filter((i) => i !== value))}
      onClearSites={() => clear(siteField, sites, t("smartRules.confirmClearSites"))}
      apps={apps}
      onAddApp={(value) => addTo(appField, apps, normalizeApp(value))}
      onRemoveApp={(value) => put(appField, apps.filter((i) => i !== value))}
      onClearApps={() => clear(appField, apps, t("smartRules.confirmClearApps"))}
      onPickApp={pickApp}
      onOpenProfiles={() => setProfilesOpen(true)}
      sidebar={<AppSidebar />}
      text={text}
      />

      {/*
        Окно всегда одно: редактор открывается ПОВЕРХ списка, и оставленный
        под ним список давал вторую подложку — страница темнела вдвое, а
        Escape закрывал не то окно. Список прячется, пока идёт правка, и
        возвращается, когда редактор закрыт.
      */}
      {profilesOpen && !editing && (
        <RoutingProfilesDialog
          text={profilesText}
          profiles={profiles}
          activeId={activeId}
          lists={lists}
          onToggleList={toggleList}
          onDeleteList={deleteList}
          onSelect={selectProfile}
          onEdit={(p) => setEditing(p)}
          onDelete={deleteProfile}
          onCreate={() => setEditing({})}
          onImport={importProfile}
          onClose={() => setProfilesOpen(false)}
        />
      )}

      {editing && (
        <RoutingProfileEditor
          text={editorText}
          profile={editing.id ? editing : null}
          busy={busy}
          onSave={saveProfile}
          onClose={() => setEditing(null)}
        />
      )}
    </>
  );
}
