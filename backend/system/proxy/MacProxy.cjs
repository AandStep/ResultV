const BaseProxyManager = require("./BaseProxyManager.cjs");
const util = require("util");
const { execFile, execSync } = require("child_process");
const execFileAsync = util.promisify(execFile);
const fs = require("fs");
const os = require("os");
const path = require("path");

// Путь к конфигурации pf для kill switch
const PF_CONF_PATH = path.join(os.tmpdir(), "resultproxy_killswitch.conf");

// Вспомогательная функция для получения активных сетевых адаптеров Mac
async function getActiveServices() {
  try {
    const { stdout } = await execFileAsync("networksetup", [
      "-listallnetworkservices",
    ]);
    const services = stdout
      .split("\n")
      .map((l) => l.trim())
      .filter(
        (l) =>
          l &&
          !l.includes("*") &&
          l !== "An asterisk (*) denotes that a network service is disabled.",
      );
    return services.length > 0 ? services : ["Wi-Fi", "Ethernet"];
  } catch (err) {
    return ["Wi-Fi", "Ethernet"];
  }
}

class MacProxy extends BaseProxyManager {
  formatBypassList(whitelist) {
    let bypassStr = "*.local 169.254/16 127.0.0.1 localhost";
    if (whitelist && whitelist.length > 0) {
      bypassStr +=
        " " + whitelist.map((d) => (d.includes("*") ? d : `*${d}*`)).join(" ");
    }
    return bypassStr;
  }

  async _runCommandsForServices(services, commandsPerService) {
    for (const service of services) {
      for (const cmd of commandsPerService(service)) {
        await execFileAsync("networksetup", cmd);
      }
    }
  }

  async setSystemProxy(proxyIp, proxyPort, proxyType, whitelist) {
    const services = await getActiveServices();
    const bypassStr = this.formatBypassList(whitelist);

    try {
      await this._runCommandsForServices(services, (service) => {
        const bypassArgs = [
          "-setproxybypassdomains",
          service,
          ...bypassStr.split(" "),
        ];
        let typeArgs = [];

        if (proxyType === "SOCKS5") {
          typeArgs = [
            ["-setsocksfirewallproxy", service, proxyIp, proxyPort.toString()],
            ["-setsocksfirewallproxystate", service, "on"],
            ["-setwebproxystate", service, "off"],
            ["-setsecurewebproxystate", service, "off"],
          ];
        } else {
          typeArgs = [
            ["-setwebproxy", service, proxyIp, proxyPort.toString()],
            ["-setsecurewebproxy", service, proxyIp, proxyPort.toString()],
            ["-setwebproxystate", service, "on"],
            ["-setsecurewebproxystate", service, "on"],
          ];
          if (proxyType === "ALL") {
            typeArgs.push([
              "-setsocksfirewallproxy",
              service,
              proxyIp,
              proxyPort.toString(),
            ]);
            typeArgs.push(["-setsocksfirewallproxystate", service, "on"]);
          } else {
            typeArgs.push(["-setsocksfirewallproxystate", service, "off"]);
          }
        }
        return [bypassArgs, ...typeArgs];
      });
      this.log(
        `[СИСТЕМА] Прокси применен к macOS (Интерфейсы: ${services.join(", ")}).`,
        "success",
      );
    } catch (error) {
      this.log(
        `[ОШИБКА macOS] Не удалось применить настройки: ${error.message}`,
        "error",
      );
      throw error;
    }
  }

  async disableSystemProxy() {
    this.log("[СИСТЕМА] Очистка настроек прокси macOS...", "info");
    const services = await getActiveServices();

    // Всегда снимаем правила pf kill switch
    await this.removeKillSwitchFirewall();

    try {
      await this._runCommandsForServices(services, (service) => [
        ["-setwebproxystate", service, "off"],
        ["-setsecurewebproxystate", service, "off"],
        ["-setsocksfirewallproxystate", service, "off"],
        ["-setproxybypassdomains", service, "Empty"],
      ]);
    } catch (err) {
      this.log(
        `[ОШИБКА macOS] Ошибка очистки настроек прокси: ${err.message}`,
        "error",
      );
    }
  }

  disableSystemProxySync() {
    // Снимаем правила pf синхронно
    try {
      execSync("pfctl -d", { stdio: "ignore" });
    } catch (e) {}
    try {
      if (fs.existsSync(PF_CONF_PATH)) {
        fs.unlinkSync(PF_CONF_PATH);
      }
    } catch (e) {}

    try {
      let services = ["Wi-Fi", "Ethernet"];
      try {
        const stdout = execSync("networksetup -listallnetworkservices", {
          encoding: "utf8",
        });
        services = stdout
          .split("\n")
          .map((l) => l.trim())
          .filter(
            (l) =>
              l &&
              !l.includes("*") &&
              l !==
                "An asterisk (*) denotes that a network service is disabled.",
          );
        if (services.length === 0) services = ["Wi-Fi", "Ethernet"];
      } catch (e) {}

      let commands = [];
      for (const service of services) {
        commands.push(`networksetup -setwebproxystate "${service}" off`);
        commands.push(`networksetup -setsecurewebproxystate "${service}" off`);
        commands.push(
          `networksetup -setsocksfirewallproxystate "${service}" off`,
        );
      }
      execSync(commands.join(" ; "), { stdio: "ignore" });
    } catch (e) {
      // Игнорируем ошибки при синхронном сбросе
    }
  }

  async applyKillSwitch() {
    this.log(
      "[KILL SWITCH] Активирована полная блокировка интернета!",
      "error",
    );

    // Основной механизм: pf (packet filter) — блокирует ВСЁ на уровне ядра
    let firewallApplied = false;
    try {
      // Создаём конфиг pf: разрешаем только loopback
      const pfRules = [
        "# ResultProxy Kill Switch",
        "set skip on lo0",
        "block all",
      ].join("\n");

      fs.writeFileSync(PF_CONF_PATH, pfRules);

      await execFileAsync("pfctl", ["-e", "-f", PF_CONF_PATH]);

      firewallApplied = true;
      this.log("[KILL SWITCH] macOS pf файрвол активирован.", "error");
    } catch (error) {
      this.log(
        `[KILL SWITCH] Не удалось применить pf (нет прав?): ${error.message}. Используем fallback.`,
        "warning",
      );
    }

    // Fallback: мёртвый прокси через networksetup
    const services = await getActiveServices();
    try {
      await this._runCommandsForServices(services, (service) => [
        ["-setwebproxy", service, "127.0.0.1", "65535"],
        ["-setsecurewebproxy", service, "127.0.0.1", "65535"],
        ["-setsocksfirewallproxy", service, "127.0.0.1", "65535"],
        ["-setwebproxystate", service, "on"],
        ["-setsecurewebproxystate", service, "on"],
        ["-setsocksfirewallproxystate", service, "on"],
      ]);
    } catch (err) {
      this.log(
        `[ОШИБКА macOS] Ошибка применения kill switch: ${err.message}`,
        "error",
      );
    }
  }

  async removeKillSwitchFirewall() {
    try {
      await execFileAsync("pfctl", ["-d"]);
    } catch (e) {} // pf мог быть не активен
    try {
      if (fs.existsSync(PF_CONF_PATH)) {
        fs.unlinkSync(PF_CONF_PATH);
      }
    } catch (e) {}
  }
}

module.exports = MacProxy;
