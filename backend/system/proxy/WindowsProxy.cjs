const BaseProxyManager = require("./BaseProxyManager.cjs");
const util = require("util");
const { execFile, execSync } = require("child_process");
const execFileAsync = util.promisify(execFile);

const FIREWALL_RULE_NAME = "ResultProxy_KillSwitch";

class WindowsProxy extends BaseProxyManager {
  formatBypassList(whitelist) {
    let override = "<local>";
    if (whitelist && whitelist.length > 0) {
      const bypassStr = whitelist.map((d) => `*.${d};*${d}*`).join(";");
      override = `${bypassStr};<local>`;
    }
    return override;
  }

  async setSystemProxy(proxyIp, proxyPort, proxyType, whitelist) {
    let proxyStr = "";
    if (proxyType === "SOCKS5") {
      proxyStr = `socks=${proxyIp}:${proxyPort}`;
    } else if (proxyType === "ALL") {
      proxyStr = `${proxyIp}:${proxyPort}`;
    } else {
      proxyStr = `http=${proxyIp}:${proxyPort};https=${proxyIp}:${proxyPort}`;
    }

    const override = this.formatBypassList(whitelist);

    try {
      await execFileAsync("reg", [
        "add",
        "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
        "/v",
        "ProxyEnable",
        "/t",
        "REG_DWORD",
        "/d",
        "1",
        "/f",
      ]);
      await execFileAsync("reg", [
        "add",
        "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
        "/v",
        "ProxyServer",
        "/t",
        "REG_SZ",
        "/d",
        proxyStr,
        "/f",
      ]);
      await execFileAsync("reg", [
        "add",
        "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
        "/v",
        "ProxyOverride",
        "/t",
        "REG_SZ",
        "/d",
        override,
        "/f",
      ]);
      try {
        await execFileAsync("reg", [
          "delete",
          "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
          "/v",
          "AutoConfigURL",
          "/f",
        ]);
      } catch (e) {} // Игнорируем, если ключа нет
      await execFileAsync("ipconfig", ["/flushdns"]);

      this.log(`[СИСТЕМА] Прокси применен к Windows успешно.`, "success");
    } catch (error) {
      this.log(
        `[ОШИБКА СИСТЕМЫ] Ошибка установки прокси: ${error.message}`,
        "error",
      );
      throw error;
    }
  }

  async disableSystemProxy() {
    this.log("[СИСТЕМА] Очистка настроек прокси из реестра Windows...", "info");

    // Всегда снимаем правила файрвола kill switch
    await this.removeKillSwitchFirewall();

    try {
      await execFileAsync("reg", [
        "add",
        "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
        "/v",
        "ProxyEnable",
        "/t",
        "REG_DWORD",
        "/d",
        "0",
        "/f",
      ]);
      try {
        await execFileAsync("reg", [
          "delete",
          "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
          "/v",
          "ProxyServer",
          "/f",
        ]);
      } catch (e) {}
      try {
        await execFileAsync("reg", [
          "delete",
          "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
          "/v",
          "ProxyOverride",
          "/f",
        ]);
      } catch (e) {}
      try {
        await execFileAsync("reg", [
          "delete",
          "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
          "/v",
          "AutoConfigURL",
          "/f",
        ]);
      } catch (e) {}
      await execFileAsync("ipconfig", ["/flushdns"]);
    } catch (error) {
      this.log(
        `[ОШИБКА СИСТЕМЫ] Ошибка очистки прокси: ${error.message}`,
        "error",
      );
    }
  }

  disableSystemProxySync() {
    // Файрвольные правила очищаются ТОЛЬКО через async removeKillSwitchFirewall()
    // Не вызываем netsh advfirewall здесь — синхронный вызов может показать интерактивный промпт

    try {
      execSync(
        'reg add "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyEnable /t REG_DWORD /d 0 /f',
        { stdio: "ignore" },
      );
      execSync(
        'reg delete "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyServer /f',
        { stdio: "ignore" },
      );
      execSync(
        'reg delete "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyOverride /f',
        { stdio: "ignore" },
      );
    } catch (e) {
      // Игнорируем ошибки при синхронном закрытии
    }
  }

  async applyKillSwitch() {
    this.log(
      "[KILL SWITCH] Активирована полная блокировка интернета!",
      "error",
    );

    // Основной механизм: файрвол Windows (блокирует ВСЁ, включая приложения, не использующие системный прокси)
    let firewallApplied = false;
    try {
      // Удаляем старые правила, если остались
      try {
        await execFileAsync("netsh", [
          "advfirewall",
          "firewall",
          "delete",
          "rule",
          `name=${FIREWALL_RULE_NAME}`,
        ]);
      } catch (e) {}

      // Блокировка всего исходящего трафика
      await execFileAsync("netsh", [
        "advfirewall",
        "firewall",
        "add",
        "rule",
        `name=${FIREWALL_RULE_NAME}`,
        "dir=out",
        "action=block",
        "enable=yes",
        "profile=any",
        "protocol=any",
      ]);
      // Блокировка всего входящего трафика
      await execFileAsync("netsh", [
        "advfirewall",
        "firewall",
        "add",
        "rule",
        `name=${FIREWALL_RULE_NAME}_in`,
        "dir=in",
        "action=block",
        "enable=yes",
        "profile=any",
        "protocol=any",
      ]);

      firewallApplied = true;
      this.log("[KILL SWITCH] Файрвол Windows активирован.", "error");
    } catch (error) {
      this.log(
        `[KILL SWITCH] Не удалось применить файрвол (нет прав администратора?): ${error.message}. Используем fallback.`,
        "warning",
      );
    }

    // Fallback: мёртвый прокси через реестр
    try {
      await execFileAsync("reg", [
        "add",
        "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
        "/v",
        "ProxyEnable",
        "/t",
        "REG_DWORD",
        "/d",
        "1",
        "/f",
      ]);
      await execFileAsync("reg", [
        "add",
        "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
        "/v",
        "ProxyServer",
        "/t",
        "REG_SZ",
        "/d",
        "127.0.0.1:65535",
        "/f",
      ]);
      try {
        await execFileAsync("reg", [
          "delete",
          "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
          "/v",
          "ProxyOverride",
          "/f",
        ]);
      } catch (e) {}
      await execFileAsync("ipconfig", ["/flushdns"]);
    } catch (error) {
      this.log(`[ОШИБКА KILL SWITCH] ${error.message}`, "error");
    }
  }

  async removeKillSwitchFirewall() {
    try {
      await execFileAsync("netsh", [
        "advfirewall",
        "firewall",
        "delete",
        "rule",
        `name=${FIREWALL_RULE_NAME}`,
      ]);
    } catch (e) {} // Правило могло не существовать
    try {
      await execFileAsync("netsh", [
        "advfirewall",
        "firewall",
        "delete",
        "rule",
        `name=${FIREWALL_RULE_NAME}_in`,
      ]);
    } catch (e) {}
  }
}

module.exports = WindowsProxy;
