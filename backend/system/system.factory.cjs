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
    };
  }
}

module.exports = SystemFactory;
