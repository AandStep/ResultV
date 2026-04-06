/**
 * Copyright (C) 2026 ResultProxy
 *
 * This file provides a centralized API wrapper for Wails backend methods.
 * It handles error catching and provides typed/named parameters for the frontend.
 */

import {
  ApplyMode,
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
  RefreshSubscription,
  AddSubscription,
  DeleteSubscription,
} from '../../wailsjs/go/main/App';

export const wailsAPI = {
  // --- Proxy Control ---
  connect: async (proxyStr, options, mode, processName) => {
    try {
      return await Connect(proxyStr, options, mode, processName);
    } catch (e) {
      console.error("wailsAPI.connect error:", e);
      throw e;
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

  ping: async (host, port) => {
    try {
      return await PingProxy(host, port);
    } catch (e) {
      console.error("wailsAPI.ping error:", e);
      return -1;
    }
  },

  // --- Configuration ---
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

  // --- Status & Logs ---
  getStatus: async () => {
    try {
      return await GetStatus(); // Returns running, connected, etc.
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

  getLogs: async (limit, level) => {
    try {
      return await GetLogs(limit, level);
    } catch (e) {
      console.error("wailsAPI.getLogs error:", e);
      return [];
    }
  },

  // --- Utilities ---
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

  // --- System/Mode Settings ---
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

  // --- Subscriptions ---
  fetchSubscription: async (url) => {
    try {
      return await FetchSubscription(url);
    } catch (e) {
      console.error("wailsAPI.fetchSubscription error:", e);
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
