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

import {
  AddRoutingList,
  UpdateRoutingList,
  DeleteRoutingList,
  RefreshRoutingList,
  ApplyMode,
  CancelConnect,
  Connect,
  Disconnect,
  DetectCountry,
  PingProxy,
  ResolveAutoCandidates,
  ReportAutoConnectOutcome,
  GetAutoGroupStatus,
  GetConfig,
  SaveConfig,
  ImportConfig,
  ExportConfig,
  GetStatus,
  GetPlatform,
  GetVersion,
  GetNetworkStatus,
  GetNetworkTraffic,
  GetLANIPs,
  GetLogs,
  GetMode,
  SetMode,
  IsAdmin,
  RestartAsAdmin,
  IsAutostartEnabled,
  SetAutostart,
  ToggleKillSwitch,
  UpdateRules,
  SyncProxies,
  FetchSubscription,
  ParseSubscriptionText,
  RefreshSubscription,
  AddSubscription,
  DeleteSubscription,
  UpdateSubscription,
  DecodeDeepLink,
  StartUpdate,
  CancelUpdate,
  GetLeftoverRecoveryReport,
  ResetLeftoverReport,
  GetChangelog,
  ShouldShowChangelog,
  AckChangelog,
} from '../../wailsjs/go/main/App';

export const wailsAPI = {
  
  connect: async (proxyStr, options, killSwitch) => {
    try {
      return await Connect(proxyStr, options, killSwitch);
    } catch (e) {
      console.error("wailsAPI.connect error:", e);
      throw e;
    }
  },

  cancelConnect: async () => {
    try {
      await CancelConnect();
    } catch (e) {
      console.error("wailsAPI.cancelConnect error:", e);
    }
  },

  disconnect: async () => {
    try {
      await Disconnect();
    } catch (e) {
      console.error("wailsAPI.disconnect error:", e);
      throw e;
    }
  },

  ping: async (host, port, proxyType) => {
    try {
      return await PingProxy(host, port, proxyType || "");
    } catch (e) {
      console.error("wailsAPI.ping error:", e);
      throw e;
    }
  },

  // Ranked AUTO-group connect candidates from the backend (App.ResolveAutoCandidates):
  // member lookup, dead-node filtering, two-phase probing, ranking and the
  // ≤5 cap all happen there, so both the tray and this UI path agree.
  async resolveAutoCandidates(proxyId) {
    try {
      return (await ResolveAutoCandidates(String(proxyId))) || [];
    } catch {
      return [];
    }
  },

  // Feeds a real connect result back into per-node statistics. The AUTO
  // head's id, the candidate's index in the list ResolveAutoCandidates
  // returned, the address and success/failure are sent: the backend rebuilds
  // the node key from its own cached candidate entry at that index (checked
  // against the address so a stale cache can't misattribute the outcome to
  // the wrong node — see app.go's ReportAutoConnectOutcome), and no failure
  // text crosses over — res.message is user-facing and can contain a
  // host:port, while node_stats.json is stored unencrypted.
  async reportAutoConnectOutcome(proxyId, candidateIndex, ip, port, ok) {
    try {
      await ReportAutoConnectOutcome(String(proxyId), candidateIndex, String(ip), port, !!ok);
    } catch {
      // Statistics are best-effort; never let them break a connect attempt.
    }
  },

  // Which member proxyId's AUTO group currently resolves to and its measured
  // RTT (App.GetAutoGroupStatus), so the row can show that node's RTT instead
  // of the group minimum. `{}` (known: undefined) reads the same as "unknown"
  // to autoRowPingLabel as an empty success response would.
  async getAutoGroupStatus(proxyId) {
    try {
      return (await GetAutoGroupStatus(String(proxyId))) || {};
    } catch {
      return {};
    }
  },

  getConfig: async () => {
    try {
      return await GetConfig();
    } catch (e) {
      console.error("wailsAPI.getConfig error:", e);
      throw e;
    }
  },

  saveConfig: async (configStr) => {
    try {
      return await SaveConfig(configStr);
    } catch (e) {
      console.error("wailsAPI.saveConfig error:", e);
      throw e;
    }
  },

  // Import an encrypted (RESULTPROXY2:) or legacy (RESULTPROXY:) export.
  //
  // Error semantics (returned from Go via Wails error string):
  //   - "export password is required" → show password prompt
  //   - "wrong export password or corrupted payload" → re-prompt
  //   - "import payload is from an older unencrypted export" → show warning,
  //     re-call with allowLegacy=true after user confirms
  // For legacy imports, pass password="".
  importConfig: async (configData, password = "", allowLegacy = false) => {
    try {
      return await ImportConfig(configData, password, allowLegacy);
    } catch (e) {
      console.error("wailsAPI.importConfig error:", e);
      throw e;
    }
  },

  // Export the current config as an encrypted RESULTPROXY2: payload. The
  // password must be at least 8 characters — shorter passwords return
  // "export password must be at least 8 characters".
  exportConfig: async (password) => {
    if (!password || password.length < 8) {
      throw new Error("export password must be at least 8 characters");
    }
    try {
      return await ExportConfig(password);
    } catch (e) {
      console.error("wailsAPI.exportConfig error:", e);
      throw e;
    }
  },

  
  getStatus: async () => {
    try {
      return await GetStatus(); 
    } catch (e) {
      console.error("wailsAPI.getStatus error:", e);
      throw e;
    }
  },

  getNetworkStatus: async () => {
    try {
      return await GetNetworkStatus();
    } catch (e) {
      console.error("wailsAPI.getNetworkStatus error:", e);
      return { online: false, latency: 0, checkedAt: 0 };
    }
  },

  getNetworkTraffic: async () => {
    try {
      return await GetNetworkTraffic();
    } catch (e) {
      console.error("wailsAPI.getNetworkTraffic error:", e);
      return { received: 0, sent: 0 };
    }
  },

  getLANIPs: async () => {
    try {
      return await GetLANIPs();
    } catch (e) {
      console.error("wailsAPI.getLANIPs error:", e);
      return [];
    }
  },

  getLogs: async (limit, level) => {
    try {
      return await GetLogs(limit, level);
    } catch (e) {
      console.error("wailsAPI.getLogs error:", e);
      return [];
    }
  },

  
  detectCountry: async (ip) => {
    try {
      return await DetectCountry(ip);
    } catch (e) {
      console.error("wailsAPI.detectCountry error:", e);
      return "Unknown";
    }
  },

  syncProxies: async (url) => {
    try {
      return await SyncProxies(url);
    } catch (e) {
      console.error("wailsAPI.syncProxies error:", e);
      throw e;
    }
  },

  
  getMode: async () => {
    try {
      return await GetMode();
    } catch (e) {
      console.error("wailsAPI.getMode error:", e);
      return "proxy";
    }
  },

  getPlatform: async () => {
    try {
      return await GetPlatform();
    } catch (e) {
      console.error("wailsAPI.getPlatform error:", e);
      return "windows";
    }
  },

  getVersion: async () => {
    try {
      return await GetVersion();
    } catch (e) {
      console.error("wailsAPI.getVersion error:", e);
      return "";
    }
  },

  setMode: async (mode) => {
    try {
      return await SetMode(mode);
    } catch (e) {
      console.error("wailsAPI.setMode error:", e);
      throw e;
    }
  },

  applyMode: async (mode) => {
    try {
      return await ApplyMode(mode);
    } catch (e) {
      console.error("wailsAPI.applyMode error:", e);
      throw e;
    }
  },

  // Ошибку пробрасываем, а не отдаём `false`: «не смогли спросить» и «прав
  // нет» — разные вещи, и подменять первое вторым значит блокировать запуск
  // из-за сбоя собственной диагностики. Ровно об этом предупреждает
  // комментарий у system.IsAdmin: принять администратора за обычного
  // пользователя хуже, чем не знать ответа.
  isAdmin: async () => {
    try {
      return await IsAdmin();
    } catch (e) {
      console.error("wailsAPI.isAdmin error:", e);
      throw e;
    }
  },

  restartAsAdmin: async () => {
    try {
      await RestartAsAdmin();
    } catch (e) {
      console.error("wailsAPI.restartAsAdmin error:", e);
      throw e;
    }
  },

  isAutostartEnabled: async () => {
    try {
      return await IsAutostartEnabled();
    } catch (e) {
      console.error("wailsAPI.isAutostartEnabled error:", e);
      return false;
    }
  },

  setAutostart: async (enabled) => {
    try {
      await SetAutostart(enabled);
    } catch (e) {
      console.error("wailsAPI.setAutostart error:", e);
      throw e;
    }
  },

  toggleKillSwitch: async (enabled) => {
    try {
      await ToggleKillSwitch(enabled);
    } catch (e) {
      console.error("wailsAPI.toggleKillSwitch error:", e);
      throw e;
    }
  },


  updateRules: async (url) => {
    try {
      return await UpdateRules(url);
    } catch (e) {
      console.error("wailsAPI.updateRules error:", e);
      throw e;
    }
  },

  // Routing-list subscriptions (user-managed domain/CIDR lists routed by a
  // single action: proxy | direct | block). Plaintext http:// is refused
  // unless allowInsecure=true — mirrors the subscription consent flow.
  addRoutingList: async (name, url, action, allowInsecure = false) => {
    try {
      return await AddRoutingList(name, url, action, allowInsecure);
    } catch (e) {
      console.error("wailsAPI.addRoutingList error:", e);
      throw e;
    }
  },

  updateRoutingList: async (routingList) => {
    try {
      return await UpdateRoutingList(routingList);
    } catch (e) {
      console.error("wailsAPI.updateRoutingList error:", e);
      throw e;
    }
  },

  deleteRoutingList: async (id) => {
    try {
      return await DeleteRoutingList(id);
    } catch (e) {
      console.error("wailsAPI.deleteRoutingList error:", e);
      throw e;
    }
  },

  refreshRoutingList: async (id) => {
    try {
      return await RefreshRoutingList(id);
    } catch (e) {
      console.error("wailsAPI.refreshRoutingList error:", e);
      throw e;
    }
  },


  // Fetch a subscription URL. Plaintext http:// is refused unless
  // allowInsecure=true is passed explicitly. The Go side returns the error
  // string "subscription URL uses plaintext HTTP — credentials and HWID
  // would travel unencrypted" — UI dispatches on this to show a warning
  // and re-call with allowInsecure=true. Insecure fetches also skip the
  // x-hwid header (HWID over plaintext defeats its own purpose).
  fetchSubscription: async (url, allowInsecure = false) => {
    try {
      return await FetchSubscription(url, allowInsecure);
    } catch (e) {
      console.error("wailsAPI.fetchSubscription error:", e);
      throw e;
    }
  },

  parseSubscriptionText: async (text) => {
    try {
      return await ParseSubscriptionText(text);
    } catch (e) {
      console.error("wailsAPI.parseSubscriptionText error:", e);
      throw e;
    }
  },

  // Ссылка, пришедшая до того, как фронт успел подписаться на события —
  // холодный старт. Дёргаем метод напрямую, а не через сгенерированный
  // модуль: биндинги обновляются только сборкой wails, и импорт отсутствующего
  // имени уронил бы бандл целиком.
  takePendingDeepLink: async () => {
    try {
      const take = window?.go?.main?.App?.TakePendingDeepLink;
      if (!take) return { payload: "", source: "" };
      return (await take()) || { payload: "", source: "" };
    } catch (e) {
      console.error("wailsAPI.takePendingDeepLink error:", e);
      return { payload: "", source: "" };
    }
  },

  decodeDeepLink: async (url) => {
    try {
      return await DecodeDeepLink(url);
    } catch (e) {
      console.error("wailsAPI.decodeDeepLink error:", e);
      throw e;
    }
  },

  refreshSubscription: async (subID) => {
    try {
      return await RefreshSubscription(subID);
    } catch (e) {
      console.error("wailsAPI.refreshSubscription error:", e);
      throw e;
    }
  },

  // Add a subscription. See fetchSubscription for the http:// consent flow.
  // The accepted-plaintext flag is persisted on the Subscription record so
  // refreshSubscription doesn't need to re-prompt.
  addSubscription: async (name, url, allowInsecure = false, subscriptionSource = "", disabledListURLs = []) => {
    try {
      return await AddSubscription(name, url, allowInsecure, subscriptionSource || "", disabledListURLs || []);
    } catch (e) {
      console.error("wailsAPI.addSubscription error:", e);
      throw e;
    }
  },

  deleteSubscription: async (subID) => {
    try {
      return await DeleteSubscription(subID);
    } catch (e) {
      console.error("wailsAPI.deleteSubscription error:", e);
      throw e;
    }
  },

  // Per-subscription settings from the servers page: display name, whether its
  // servers show up on the home screen, and its own refresh interval in
  // minutes (0 — never, negative — follow the global setting).
  updateSubscription: async (subID, name, showOnHome, updateIntervalMinutes) => {
    try {
      return await UpdateSubscription(subID, name, !!showOnHome, updateIntervalMinutes);
    } catch (e) {
      console.error("wailsAPI.updateSubscription error:", e);
      throw e;
    }
  },

  startUpdate: async () => {
    try {
      await StartUpdate();
    } catch (e) {
      console.error("wailsAPI.startUpdate error:", e);
      throw e;
    }
  },

  cancelUpdate: async () => {
    try {
      await CancelUpdate();
    } catch (e) {
      console.error("wailsAPI.cancelUpdate error:", e);
    }
  },

  // Report of OS-level leftovers (sing-tun adapter / system proxy / DNS /
  // kill-switch firewall) that startup recovery already cleaned after a prior
  // unclean exit. Returns + clears the stored report; used for a one-time notice.
  getLeftoverRecoveryReport: async () => {
    try {
      return await GetLeftoverRecoveryReport();
    } catch (e) {
      console.error("wailsAPI.getLeftoverRecoveryReport error:", e);
      return { proxy: false, dns: false, tun: false, firewall: false };
    }
  },

  resetLeftoverReport: async () => {
    try {
      await ResetLeftoverReport();
    } catch (e) {
      console.error("wailsAPI.resetLeftoverReport error:", e);
    }
  },

  // Release notes of the RUNNING build, read from the update.json embedded at
  // compile time — not from the network. Whether they are due this launch is
  // shouldShowChangelog's call, kept in Go so the policy has one home.
  getChangelog: async (lang) => {
    try {
      return await GetChangelog(lang || "");
    } catch (e) {
      console.error("wailsAPI.getChangelog error:", e);
      return null;
    }
  },

  shouldShowChangelog: async () => {
    try {
      return await ShouldShowChangelog();
    } catch (e) {
      console.error("wailsAPI.shouldShowChangelog error:", e);
      return false;
    }
  },

  ackChangelog: async () => {
    try {
      await AckChangelog();
    } catch (e) {
      console.error("wailsAPI.ackChangelog error:", e);
    }
  },

  /*
   * Профили маршрутизации.
   *
   * Идут через мост напрямую, а не через сгенерированные обёртки: файлы в
   * wailsjs/ пересобирает `wails build`, и до первой такой сборки импорт
   * несуществующего имени уронил бы весь бандл, а не одну кнопку. Имена и
   * порядок аргументов те же, что у методов App, так что после генерации
   * можно перевести на импорт, ничего не меняя в вызовах.
   */
  getRoutingProfiles: () => callApp("GetRoutingProfiles", [], { profiles: [], activeId: "" }),
  saveRoutingProfile: (profile) => callApp("SaveRoutingProfile", [profile]),
  deleteRoutingProfile: (id) => callApp("DeleteRoutingProfile", [id]),
  setActiveRoutingProfile: (id) => callApp("SetActiveRoutingProfile", [id]),
  compileRoutingProfile: (id, refreshGeo) => callApp("CompileRoutingProfile", [id, !!refreshGeo]),
  previewRoutingDeepLink: (url) => callApp("PreviewRoutingDeepLink", [url]),
  importRoutingDeepLink: (url, makeActive) =>
    callApp("ImportRoutingDeepLink", [url, !!makeActive]),
};

/*
 * Вызов метода App через мост. `fallback` возвращается только когда моста нет
 * вовсе (заглушка при проверке вида) — настоящую ошибку метода пробрасываем
 * наверх: интерфейсу есть что показать, а молчаливый успех прячет отказ.
 */
async function callApp(method, args = [], fallback) {
  const app = typeof window !== "undefined" ? window.go?.main?.App : undefined;
  const fn = app?.[method];
  if (typeof fn !== "function") {
    console.warn(`wailsAPI: метод ${method} недоступен — пересоберите биндинги`);
    if (fallback !== undefined) return fallback;
    throw new Error(`${method} недоступен`);
  }
  return fn(...args);
}

export default wailsAPI;
