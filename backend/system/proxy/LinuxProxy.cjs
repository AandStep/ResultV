const BaseProxyManager = require("./BaseProxyManager.cjs");
const util = require("util");
const { execFile, execSync } = require("child_process");
const execFileAsync = util.promisify(execFile);

// Маркер-комментарий для идентификации наших правил iptables
const IPTABLES_COMMENT = "ResultProxy_KillSwitch";

class LinuxProxy extends BaseProxyManager {
  formatBypassList(whitelist) {
    let bypassArray = ["'localhost'", "'127.0.0.0/8'", "'::1'"];
    if (whitelist && whitelist.length > 0) {
      whitelist.forEach((domain) => {
        const clean = domain.replace(/\*/g, "");
        bypassArray.push(`'${clean}'`);
      });
    }
    return `[${bypassArray.join(", ")}]`;
  }

  async setSystemProxy(proxyIp, proxyPort, proxyType, whitelist) {
    const bypassStr = this.formatBypassList(whitelist);

    try {
      await execFileAsync("gsettings", [
        "set",
        "org.gnome.system.proxy",
        "mode",
        "'manual'",
      ]);
      await execFileAsync("gsettings", [
        "set",
        "org.gnome.system.proxy",
        "ignore-hosts",
        bypassStr,
      ]);

      if (proxyType === "SOCKS5") {
        await execFileAsync("gsettings", [
          "set",
          "org.gnome.system.proxy.socks",
          "host",
          `'${proxyIp}'`,
        ]);
        await execFileAsync("gsettings", [
          "set",
          "org.gnome.system.proxy.socks",
          "port",
          proxyPort.toString(),
        ]);
        await execFileAsync("gsettings", [
          "set",
          "org.gnome.system.proxy.http",
          "host",
          "''",
        ]);
        await execFileAsync("gsettings", [
          "set",
          "org.gnome.system.proxy.https",
          "host",
          "''",
        ]);
      } else {
        await execFileAsync("gsettings", [
          "set",
          "org.gnome.system.proxy.http",
          "host",
          `'${proxyIp}'`,
        ]);
        await execFileAsync("gsettings", [
          "set",
          "org.gnome.system.proxy.http",
          "port",
          proxyPort.toString(),
        ]);
        await execFileAsync("gsettings", [
          "set",
          "org.gnome.system.proxy.https",
          "host",
          `'${proxyIp}'`,
        ]);
        await execFileAsync("gsettings", [
          "set",
          "org.gnome.system.proxy.https",
          "port",
          proxyPort.toString(),
        ]);

        if (proxyType === "ALL") {
          await execFileAsync("gsettings", [
            "set",
            "org.gnome.system.proxy.socks",
            "host",
            `'${proxyIp}'`,
          ]);
          await execFileAsync("gsettings", [
            "set",
            "org.gnome.system.proxy.socks",
            "port",
            proxyPort.toString(),
          ]);
        } else {
          await execFileAsync("gsettings", [
            "set",
            "org.gnome.system.proxy.socks",
            "host",
            "''",
          ]);
        }
      }

      this.log(`[СИСТЕМА] Прокси применен к Linux (gsettings).`, "success");
    } catch (error) {
      this.log(
        `[СИСТЕМА Linux] Ошибка настройки gsettings: ${error.message} (возможно не GNOME среда)`,
        "info",
      );
    }
  }

  async disableSystemProxy() {
    this.log("[СИСТЕМА] Очистка настроек прокси Linux...", "info");

    // Всегда снимаем правила iptables kill switch
    await this.removeKillSwitchFirewall();

    try {
      await execFileAsync("gsettings", [
        "set",
        "org.gnome.system.proxy",
        "mode",
        "'none'",
      ]);
    } catch (error) {
      this.log(
        `[СИСТЕМА Linux] Ошибка сброса gsettings: ${error.message}`,
        "info",
      );
    }
  }

  disableSystemProxySync() {
    // Снимаем правила iptables синхронно
    try {
      execSync(
        `iptables -D OUTPUT -m comment --comment "${IPTABLES_COMMENT}" -j DROP`,
        { stdio: "ignore" },
      );
      execSync(
        `iptables -D INPUT -m comment --comment "${IPTABLES_COMMENT}" -j DROP`,
        { stdio: "ignore" },
      );
    } catch (e) {}

    try {
      execSync(`gsettings set org.gnome.system.proxy mode 'none'`, {
        stdio: "ignore",
      });
    } catch (e) {
      // Игнорируем ошибку
    }
  }

  async applyKillSwitch() {
    this.log(
      "[KILL SWITCH] Активирована полная блокировка интернета!",
      "error",
    );

    // Основной механизм: iptables (блокирует ВСЁ, не только приложения с gsettings)
    let firewallApplied = false;
    try {
      // Снимаем старые правила если остались
      await this.removeKillSwitchFirewall();

      // Разрешаем loopback
      await execFileAsync("iptables", [
        "-I",
        "OUTPUT",
        "-o",
        "lo",
        "-j",
        "ACCEPT",
        "-m",
        "comment",
        "--comment",
        IPTABLES_COMMENT,
      ]);
      await execFileAsync("iptables", [
        "-I",
        "INPUT",
        "-i",
        "lo",
        "-j",
        "ACCEPT",
        "-m",
        "comment",
        "--comment",
        IPTABLES_COMMENT,
      ]);
      // Блокируем всё остальное
      await execFileAsync("iptables", [
        "-A",
        "OUTPUT",
        "-j",
        "DROP",
        "-m",
        "comment",
        "--comment",
        IPTABLES_COMMENT,
      ]);
      await execFileAsync("iptables", [
        "-A",
        "INPUT",
        "-j",
        "DROP",
        "-m",
        "comment",
        "--comment",
        IPTABLES_COMMENT,
      ]);

      firewallApplied = true;
      this.log("[KILL SWITCH] iptables активирован.", "error");
    } catch (error) {
      this.log(
        `[KILL SWITCH] Не удалось применить iptables (нет прав root?): ${error.message}. Используем fallback.`,
        "warning",
      );
    }

    // Fallback: мёртвый прокси через gsettings
    try {
      await execFileAsync("gsettings", [
        "set",
        "org.gnome.system.proxy",
        "mode",
        "'manual'",
      ]);
      await execFileAsync("gsettings", [
        "set",
        "org.gnome.system.proxy.socks",
        "host",
        "'127.0.0.1'",
      ]);
      await execFileAsync("gsettings", [
        "set",
        "org.gnome.system.proxy.socks",
        "port",
        "65535",
      ]);
      await execFileAsync("gsettings", [
        "set",
        "org.gnome.system.proxy.http",
        "host",
        "'127.0.0.1'",
      ]);
      await execFileAsync("gsettings", [
        "set",
        "org.gnome.system.proxy.http",
        "port",
        "65535",
      ]);
    } catch (error) {
      this.log(
        `[СИСТЕМА Linux] Ошибка kill-switch (gsettings): ${error.message}`,
        "info",
      );
    }
  }

  async removeKillSwitchFirewall() {
    // Удаляем все наши правила iptables по комментарию
    try {
      // Удаляем правила OUTPUT
      while (true) {
        try {
          await execFileAsync("iptables", [
            "-D",
            "OUTPUT",
            "-m",
            "comment",
            "--comment",
            IPTABLES_COMMENT,
            "-j",
            "DROP",
          ]);
        } catch (e) {
          break;
        }
      }
      while (true) {
        try {
          await execFileAsync("iptables", [
            "-D",
            "OUTPUT",
            "-m",
            "comment",
            "--comment",
            IPTABLES_COMMENT,
            "-j",
            "ACCEPT",
          ]);
        } catch (e) {
          break;
        }
      }
      // Удаляем правила INPUT
      while (true) {
        try {
          await execFileAsync("iptables", [
            "-D",
            "INPUT",
            "-m",
            "comment",
            "--comment",
            IPTABLES_COMMENT,
            "-j",
            "DROP",
          ]);
        } catch (e) {
          break;
        }
      }
      while (true) {
        try {
          await execFileAsync("iptables", [
            "-D",
            "INPUT",
            "-m",
            "comment",
            "--comment",
            IPTABLES_COMMENT,
            "-j",
            "ACCEPT",
          ]);
        } catch (e) {
          break;
        }
      }
    } catch (e) {} // iptables может быть недоступен
  }
}

module.exports = LinuxProxy;
