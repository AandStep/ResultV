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
  ApplyMode,
  CancelConnect,
  Connect,
  Disconnect,
  DetectCountry,
  PingProxy,
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
  ToggleAdBlock,
  GetAdBlockStatus,
  UpdateAdBlockFilters,
  InstallAdBlockCA,
  IsAdBlockCAInstalled,
  UpdateRules,
  SyncProxies,
  FetchSubscription,
  ParseSubscriptionText,
  RefreshSubscription,
  AddSubscription,
  DeleteSubscription,
  DecodeDeepLink,
  StartUpdate,
  CancelUpdate,
} from '../../wailsjs/go/main/App';

export const wailsAPI = {
  
  connect: async (proxyStr, options, mode, processName) => {
    try {
      return await Connect(proxyStr, options, mode, processName);
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

  isAdmin: async () => {
    try {
      return await IsAdmin();
    } catch (e) {
      console.error("wailsAPI.isAdmin error:", e);
      return false;
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

  toggleAdBlock: async (enabled) => {
    try {
      await ToggleAdBlock(enabled);
    } catch (e) {
      console.error("wailsAPI.toggleAdBlock error:", e);
      throw e;
    }
  },

  getAdBlockStatus: async () => {
    try {
      return await GetAdBlockStatus();
    } catch (e) {
      console.error("wailsAPI.getAdBlockStatus error:", e);
      return null;
    }
  },

  updateAdBlockFilters: async () => {
    try {
      await UpdateAdBlockFilters();
    } catch (e) {
      console.error("wailsAPI.updateAdBlockFilters error:", e);
      throw e;
    }
  },

  installAdBlockCA: async () => {
    try {
      await InstallAdBlockCA();
    } catch (e) {
      console.error("wailsAPI.installAdBlockCA error:", e);
      throw e;
    }
  },

  isAdBlockCAInstalled: async () => {
    try {
      return await IsAdBlockCAInstalled();
    } catch (e) {
      console.error("wailsAPI.isAdBlockCAInstalled error:", e);
      return false;
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
  addSubscription: async (name, url, allowInsecure = false, subscriptionSource = "") => {
    try {
      return await AddSubscription(name, url, allowInsecure, subscriptionSource || "");
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
};

export default wailsAPI;
