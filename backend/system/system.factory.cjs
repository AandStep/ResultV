/*
 * Copyright (C) 2026 ResultProxy
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

const os = require("os");

const WindowsProcess = require("./process/WindowsProcess.cjs");
const WindowsProxy = require("./proxy/WindowsProxy.cjs");
const WindowsNetwork = require("./network/WindowsNetwork.cjs");

const MacProcess = require("./process/MacProcess.cjs");
const MacProxy = require("./proxy/MacProxy.cjs");
const MacNetwork = require("./network/MacNetwork.cjs");

const LinuxProcess = require("./process/LinuxProcess.cjs");
const LinuxProxy = require("./proxy/LinuxProxy.cjs");
const LinuxNetwork = require("./network/LinuxNetwork.cjs");

class SystemFactory {
  static getAdapter(loggerService) {
    const platform = os.platform();
    let ProcessMng, ProxyMng, NetworkMng;

    if (platform === "win32") {
      ProcessMng = WindowsProcess;
      ProxyMng = WindowsProxy;
      NetworkMng = WindowsNetwork;
    } else if (platform === "darwin") {
      ProcessMng = MacProcess;
      ProxyMng = MacProxy;
      NetworkMng = MacNetwork;
    } else if (platform === "linux") {
      ProcessMng = LinuxProcess;
      ProxyMng = LinuxProxy;
      NetworkMng = LinuxNetwork;
    } else {
      // Mock adapters for unsupported OS
      return {
        startProcessCacheInterval: () => {},
        getNetworkTraffic: async () => ({ received: 0, sent: 0 }),
        applyKillSwitch: async () =>
          loggerService.log(
            "Killswitch пока не поддерживается на этой ОС",
            "warning",
          ),
        removeKillSwitchFirewall: async () => {},
        disableSystemProxy: async () => {
          loggerService.log(
            "Очистка прокси пока не реализована для этой ОС",
            "warning",
          );
        },
        disableSystemProxySync: () => {},
        setSystemProxy: async (ip, port, type, wl) => {
          loggerService.log(
            `Авто-настройка не поддерживается. Настройте прокси вручную: ${ip}:${port}`,
            "warning",
          );
        },
        checkAppWhitelist: async () => false,
        setRunAsAdminFlag: async () => {},

        // Новые методы для планировщика задач
        enableTaskAutostart: async () => {},
        disableTaskAutostart: async () => {},

        // GPO
        setUserDataPath: () => {},
        restoreGpoOnStartup: () => false,
      };
    }

    const processManager = new ProcessMng();
    const proxyManager = new ProxyMng(loggerService);
    const networkManager = new NetworkMng();

    // Специальные утилиты для Windows (старая архитектура)
    const windowsUtils = platform === "win32" ? require("./windows.cjs") : null;

    // Возвращаем Facade, который реализует старый интерфейс для обратной совместимости
    // с остальным бекендом, пока он не переписан полностью.
    return {
      startProcessCacheInterval: (getState) =>
        processManager.startProcessCacheInterval(getState),
      stopProcessCacheInterval: () => processManager.stopProcessCacheInterval(),
      getNetworkTraffic: () => networkManager.getNetworkTraffic(),
      setSystemProxy: (ip, port, type, wl, logCb) =>
        proxyManager.setSystemProxy(ip, port, type, wl),
      disableSystemProxy: (logCb) => proxyManager.disableSystemProxy(),
      disableSystemProxySync: () => proxyManager.disableSystemProxySync(),
      applyKillSwitch: (logCb) => proxyManager.applyKillSwitch(),
      removeKillSwitchFirewall: () => proxyManager.removeKillSwitchFirewall(),
      checkAppWhitelist: (port, wl, host, logCb) =>
        processManager.checkAppWhitelist(port, wl, host, logCb),
      setRunAsAdminFlag: (enable) =>
        windowsUtils
          ? windowsUtils.setRunAsAdminFlag(enable)
          : Promise.resolve(),

      // Новые методы для планировщика задач (Windows)
      enableTaskAutostart: (exePath, args) =>
        windowsUtils
          ? windowsUtils.enableTaskAutostart(exePath, args)
          : Promise.resolve(),
      disableTaskAutostart: () =>
        windowsUtils ? windowsUtils.disableTaskAutostart() : Promise.resolve(),

      // GPO: передаём userDataPath в proxyManager для хранения backup
      setUserDataPath: (userDataPath) => {
        if (typeof proxyManager.setUserDataPath === "function") {
          proxyManager.setUserDataPath(userDataPath);
        }
      },

      // GPO: восстановление настроек при запуске (после аварийного завершения)
      restoreGpoOnStartup: (userDataPath, logCallback) => {
        if (platform === "win32") {
          return WindowsProxy.restoreGpoOnStartup(userDataPath, logCallback);
        }
        return false;
      },
    };
  }
}

module.exports = SystemFactory;
