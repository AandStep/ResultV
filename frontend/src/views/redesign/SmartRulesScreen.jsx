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

import { useTranslation } from "react-i18next";
import { useConfigContext } from "../../context/ConfigContext";
import { PickAppForWhitelist } from "../../../wailsjs/go/main/App";
import SmartRulesPage from "./SmartRulesPage";
import AppSidebar from "./AppSidebar";

export default function SmartRulesScreen() {
  const { t } = useTranslation();
  const {
    routingRules: rules,
    setRoutingRules: setRules,
    showConfirmDialog,
    platform,
  } = useConfigContext();

  const isWin =
    platform === "win32" || platform === "windows" || platform === "win64";
  const isSmart = rules.mode === "smart";

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
  };

  return (
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
      sidebar={<AppSidebar />}
      text={text}
    />
  );
}
