class BaseProxyManager {
  constructor(loggerService) {
    this.loggerService = loggerService;
  }

  log(message, type = "info") {
    if (this.loggerService && typeof this.loggerService.log === "function") {
      this.loggerService.log(message, type);
    }
  }

  formatBypassList(whitelist) {
    throw new Error("Method formatBypassList must be implemented");
  }

  async setSystemProxy(proxyIp, proxyPort, proxyType, whitelist) {
    throw new Error("Method setSystemProxy must be implemented");
  }

  async disableSystemProxy() {
    throw new Error("Method disableSystemProxy must be implemented");
  }

  disableSystemProxySync() {
    throw new Error("Method disableSystemProxySync must be implemented");
  }

  async applyKillSwitch() {
    throw new Error("Method applyKillSwitch must be implemented");
  }

  async removeKillSwitchFirewall() {
    // По умолчанию ничего не делаем — переопределяется в наследниках
  }
}

module.exports = BaseProxyManager;
