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
  UpdateRules,
  SyncProxies,
  FetchSubscription,
  ParseSubscriptionText,
  RefreshSubscription,
  AddSubscription,
  DeleteSubscription,
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

  importConfig: async (configData) => {
    try {
      return await ImportConfig(configData);
    } catch (e) {
      console.error("wailsAPI.importConfig error:", e);
      throw e;
    }
  },

  exportConfig: async () => {
    try {
      return await ExportConfig();
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
      return false;
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

  updateRules: async (url) => {
    try {
      return await UpdateRules(url);
    } catch (e) {
      console.error("wailsAPI.updateRules error:", e);
      throw e;
    }
  },

  
  fetchSubscription: async (url) => {
    try {
      return await FetchSubscription(url);
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

  refreshSubscription: async (subID) => {
    try {
      return await RefreshSubscription(subID);
    } catch (e) {
      console.error("wailsAPI.refreshSubscription error:", e);
      throw e;
    }
  },

  addSubscription: async (name, url) => {
    try {
      return await AddSubscription(name, url);
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
};

export default wailsAPI;
